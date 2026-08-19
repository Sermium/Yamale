package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// challengeCount is how many words are read back, including the last one.
//
// Four sampled positions plus the last. The number is a trade, and the trade is
// not against an attacker — it is against the one failure that actually happens,
// which is a person copying twenty-four words by hand and getting one wrong.
// Sampling five of twenty-four catches a single error about one time in five,
// which sounds poor until you notice what it is really for: it forces the
// custodian to read their own sheet back, in order, under someone else's eye,
// before the room moves on. Asking for all twenty-four would be better and
// nobody would do it twice.
const challengeCount = 5

// maxAttempts is how many complete passes are allowed before the tool stops.
//
// Not unlimited. A custodian who cannot read their own sheet back three times
// running has a sheet with something wrong on it, and the correct response is
// to destroy the key and start again — not to keep guessing until the tool
// happens to be satisfied.
const maxAttempts = 3

// displayPhrase writes the phrase to the screen, numbered, four to a line.
//
// Numbered because a phrase transcribed out of order recovers nothing and
// people do transcribe out of order. This is the only function in the program
// that writes phrase bytes anywhere, which is what makes the "never reaches
// disk" claim checkable by reading one file.
func displayPhrase(c *console, s *secret) {
	c.println("Recovery phrase. Write it down now, in order, on the sheet in front of you.")
	c.println("It is shown once. Nothing on this machine will be able to show it again.")
	c.println()
	for i := 1; i <= s.wordCount(); i++ {
		c.printf("%2d. %-10s", i, s.word(i))
		if i%4 == 0 {
			c.println()
		}
	}
	if s.wordCount()%4 != 0 {
		c.println()
	}
	c.println()
}

// pickPositions chooses which words will be asked for.
//
// The last word is always among them. It is the one people leave off — the
// sheet runs out of room, the phone rings on word twenty-three, the writer
// assumes twenty-four is the same as the twelve they have seen before — and a
// sample that could miss it would be sampling everything except the most likely
// mistake.
//
// crypto/rand rather than math/rand, so nobody who has watched a few ceremonies
// can predict which four they will be asked for and write only those down
// carefully.
func pickPositions(words, count int) ([]int, error) {
	if words < 1 {
		return nil, errors.New("an empty phrase has no words to check")
	}
	if count > words {
		count = words
	}

	chosen := map[int]bool{words: true}
	for len(chosen) < count {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(words-1)))
		if err != nil {
			return nil, err
		}
		chosen[int(n.Int64())+1] = true
	}

	positions := make([]int, 0, len(chosen))
	for i := 1; i <= words; i++ {
		if chosen[i] {
			positions = append(positions, i)
		}
	}
	return positions, nil
}

// askBack puts the questions and reports which positions came back wrong.
//
// Every position is asked even after the first mistake. Stopping at the first
// wrong answer would tell the custodian one word is wrong and leave them to
// find out about the second on the next pass; a list of every position that
// disagreed is what lets somebody fix a sheet in one go.
func askBack(c *console, s *secret, positions []int) ([]int, error) {
	var wrong []int
	for _, position := range positions {
		answer, err := c.readLine(fmt.Sprintf("  word %2d: ", position))
		if err != nil {
			return nil, err
		}
		// Case-folded and trimmed, because BIP-39 words are lowercase and a
		// custodian who typed "Abandon" has transcribed the word correctly.
		// Nothing else is normalised: a phrase is right or it is not.
		if !bytes.Equal([]byte(strings.ToLower(strings.TrimSpace(answer))), s.word(position)) {
			wrong = append(wrong, position)
		}
	}
	return wrong, nil
}

// verifyTranscription is the whole check: show, clear, ask, repeat until right.
//
// The clear between showing and asking is the part that makes this a check
// rather than a formality. With the phrase still on the screen the custodian is
// reading it off the monitor, the sheet in their hand is never consulted, and
// the tool has confirmed only that the screen agrees with itself.
func verifyTranscription(c *console, s *secret) error {
	return verifyTranscriptionWith(c, s, pickPositions)
}

// picker chooses which words are asked for. Injected so a test can fix the
// positions and script the answers; production always passes pickPositions,
// which chooses them with crypto/rand.
type picker func(words, count int) ([]int, error)

func verifyTranscriptionWith(c *console, s *secret, pick picker) error {
	for attempt := 1; ; attempt++ {
		displayPhrase(c, s)
		if err := c.pause("Press return when it is written down and you have checked it against the screen. "); err != nil {
			return err
		}

		c.clear()
		c.println("Screen cleared. Read the following back from your sheet, not from memory.")
		c.println()

		positions, err := pick(s.wordCount(), challengeCount)
		if err != nil {
			return err
		}

		wrong, err := askBack(c, s, positions)
		if err != nil {
			return err
		}
		if len(wrong) == 0 {
			c.println()
			c.println("Transcription verified.")
			return nil
		}

		c.println()
		c.printf("Wrong at %s.\n", positionList(wrong))
		c.println("Your sheet does not match. That is what this check is for — a phrase with one")
		c.println("wrong word derives a different, empty account, and you would not have found out")
		c.println("until the day it mattered.")
		c.println()

		if attempt >= maxAttempts {
			return fmt.Errorf(
				"transcription failed %d times.\n"+
					"Stop. Destroy the sheet, run this command again and generate a new key.\n"+
					"Do not keep retrying: a sheet that cannot be read back three times is a\n"+
					"sheet with something wrong on it, and the cost of a fresh key is five minutes",
				maxAttempts)
		}

		c.println("Correct your sheet against the phrase, which will be shown again now.")
		if err := c.pause("Press return. "); err != nil {
			return err
		}
		c.clear()
	}
}

func positionList(positions []int) string {
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = fmt.Sprintf("word %d", p)
	}
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}
