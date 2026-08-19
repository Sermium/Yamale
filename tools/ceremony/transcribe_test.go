package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// scripted builds a console whose input is a fixed list of answers, so the
// transcription check can be driven end to end without a terminal.
func scripted(answers ...string) (*console, *bytes.Buffer) {
	var out bytes.Buffer
	input := strings.Join(answers, "\n") + "\n"
	return &console{in: bufio.NewReader(strings.NewReader(input)), out: &out, tty: -1}, &out
}

func testSecret() *secret {
	words := make([]string, 24)
	for i := range words {
		words[i] = fmt.Sprintf("w%02d", i+1)
	}
	return &secret{buf: []byte(strings.Join(words, " "))}
}

func TestPickPositionsAlwaysIncludesTheLastWord(t *testing.T) {
	// The last word is the one people leave off the sheet. Run it enough times
	// that a sampler which merely usually included it would fail.
	for i := 0; i < 200; i++ {
		positions, err := pickPositions(24, challengeCount)
		if err != nil {
			t.Fatal(err)
		}
		if len(positions) != challengeCount {
			t.Fatalf("got %d positions, want %d", len(positions), challengeCount)
		}
		if positions[len(positions)-1] != 24 {
			t.Fatalf("the last word was not asked for: %v", positions)
		}
		seen := map[int]bool{}
		for _, p := range positions {
			if p < 1 || p > 24 {
				t.Fatalf("position %d is outside the phrase", p)
			}
			if seen[p] {
				t.Fatalf("position %d asked for twice: %v", p, positions)
			}
			seen[p] = true
		}
	}
}

func TestPickPositionsVariesBetweenRuns(t *testing.T) {
	// A fixed sample would let anyone who has watched a few ceremonies write
	// only those words down carefully.
	first, err := pickPositions(24, challengeCount)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		next, err := pickPositions(24, challengeCount)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(next) != fmt.Sprint(first) {
			return
		}
	}
	t.Fatal("fifty runs produced the same positions, so the sample is not random")
}

func TestPickPositionsHandlesShortPhrases(t *testing.T) {
	positions, err := pickPositions(3, challengeCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 3 {
		t.Fatalf("got %v, want every position of a three-word phrase", positions)
	}
	if _, err := pickPositions(0, challengeCount); err == nil {
		t.Fatal("an empty phrase was accepted")
	}
}

func TestAskBackAcceptsCorrectAnswers(t *testing.T) {
	s := testSecret()
	positions := []int{1, 7, 24}

	answers := make([]string, len(positions))
	for i, p := range positions {
		answers[i] = string(s.word(p))
	}
	c, _ := scripted(answers...)

	wrong, err := askBack(c, s, positions)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatalf("correct answers were rejected at %v", wrong)
	}
}

func TestAskBackRejectsWrongAnswers(t *testing.T) {
	s := testSecret()
	positions := []int{1, 7, 24}

	// Word 7 answered with word 8: a plausible off-by-one, which is what a
	// sheet written a line out of step actually looks like.
	c, _ := scripted(string(s.word(1)), string(s.word(8)), string(s.word(24)))

	wrong, err := askBack(c, s, positions)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 1 || wrong[0] != 7 {
		t.Fatalf("wrong = %v, want [7]", wrong)
	}
}

func TestAskBackReportsEveryWrongPositionNotJustTheFirst(t *testing.T) {
	s := testSecret()
	positions := []int{1, 7, 24}
	c, _ := scripted("nonsense", "nonsense", "nonsense")

	wrong, err := askBack(c, s, positions)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 3 {
		t.Fatalf("wrong = %v, want all three positions; stopping at the first would send the custodian back twice", wrong)
	}
}

func TestAskBackToleratesCaseAndSurroundingSpace(t *testing.T) {
	s := testSecret()
	positions := []int{2}
	c, _ := scripted("  " + strings.ToUpper(string(s.word(2))) + "  ")

	wrong, err := askBack(c, s, positions)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatal("a correctly transcribed word was rejected for its capitalisation")
	}
}

func TestAskBackRejectsAPrefixOfTheWord(t *testing.T) {
	// BIP-39 words are identified by their first four letters, and a tool that
	// accepted a prefix would pass a sheet that recovers nothing.
	s := &secret{buf: []byte("abandon ability able about")}
	c, _ := scripted("aban")

	wrong, err := askBack(c, s, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 1 {
		t.Fatal("a four-letter prefix was accepted in place of the whole word")
	}
}

// fixedPositions replaces the random sampler so a run can be scripted. The
// randomness is tested separately, in TestPickPositions*; what is being
// exercised here is the show/clear/ask/retry sequence around it.
func fixedPositions(positions ...int) picker {
	return func(words, count int) ([]int, error) { return positions, nil }
}

// correctAnswersFor is the script a custodian with a perfect sheet produces:
// the pause after writing it down, then one word per question.
func correctAnswersFor(s *secret, positions ...int) []string {
	answers := []string{""}
	for _, p := range positions {
		answers = append(answers, string(s.word(p)))
	}
	return answers
}

func TestVerifyTranscriptionPassesOnACorrectSheet(t *testing.T) {
	s := testSecret()
	positions := []int{3, 9, 24}
	c, out := scripted(correctAnswersFor(s, positions...)...)

	if err := verifyTranscriptionWith(c, s, fixedPositions(positions...)); err != nil {
		t.Fatalf("a correct sheet was rejected: %v", err)
	}
	if !strings.Contains(out.String(), "Transcription verified") {
		t.Fatal("a passing check did not say so")
	}
}

func TestVerifyTranscriptionRetriesAfterAWrongAnswerAndThenAccepts(t *testing.T) {
	s := testSecret()
	positions := []int{3, 9, 24}

	// Pass one: word 9 wrong. Then the pause before the phrase is shown again,
	// then a clean pass.
	answers := []string{
		"", string(s.word(3)), "wrong", string(s.word(24)),
		"",
	}
	answers = append(answers, correctAnswersFor(s, positions...)...)
	c, out := scripted(answers...)

	if err := verifyTranscriptionWith(c, s, fixedPositions(positions...)); err != nil {
		t.Fatalf("the second, correct pass was rejected: %v", err)
	}
	if !strings.Contains(out.String(), "Wrong at word 9") {
		t.Fatalf("the custodian was not told which word was wrong:\n%s", out.String())
	}
}

func TestVerifyTranscriptionGivesUpAfterThreeFailedPasses(t *testing.T) {
	s := testSecret()
	positions := []int{3, 9, 24}

	// Three passes of nonsense, with the pause between each. The tool must
	// stop rather than let a room keep guessing until it happens to pass.
	var answers []string
	for pass := 0; pass < maxAttempts; pass++ {
		answers = append(answers, "", "nonsense", "nonsense", "nonsense", "")
	}
	c, _ := scripted(answers...)

	err := verifyTranscriptionWith(c, s, fixedPositions(positions...))
	if err == nil {
		t.Fatal("an unreadable sheet was accepted")
	}
	if !strings.Contains(err.Error(), "generate a new key") {
		t.Fatalf("the failure does not tell the room to start again: %v", err)
	}
}

func TestVerifyTranscriptionClearsTheScreenBeforeAsking(t *testing.T) {
	s := testSecret()
	positions := []int{3, 9, 24}
	c, out := scripted(correctAnswersFor(s, positions...)...)

	if err := verifyTranscriptionWith(c, s, fixedPositions(positions...)); err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	clearAt := strings.Index(rendered, "\033[3J")
	if clearAt < 0 {
		t.Fatal("the scrollback was never cleared, so the phrase stays one scroll away")
	}
	askAt := strings.Index(rendered, "  word ")
	if askAt < 0 || askAt < clearAt {
		t.Fatal("the first question was asked before the screen was cleared, so the custodian is reading the monitor rather than their sheet")
	}
	// No word of the phrase may survive on screen after the clear, or the
	// custodian answers from the display instead of the paper.
	for i := 1; i <= s.wordCount(); i++ {
		if strings.Contains(rendered[clearAt:], string(s.word(i))) {
			t.Fatalf("word %d of the phrase appears after the screen clear", i)
		}
	}
}

func TestDisplayPhraseNumbersEveryWord(t *testing.T) {
	s := testSecret()
	var out bytes.Buffer
	c := &console{in: bufio.NewReader(strings.NewReader("")), out: &out, tty: -1}

	displayPhrase(c, s)
	rendered := out.String()

	// Numbered, because a phrase transcribed out of order recovers nothing.
	for i := 1; i <= s.wordCount(); i++ {
		if !strings.Contains(rendered, fmt.Sprintf("%2d. %s", i, s.word(i))) {
			t.Fatalf("word %d is not numbered in the display:\n%s", i, rendered)
		}
	}
}
