package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFiveCustodianCeremony runs a whole ceremony — five custodians, the
// transcription check on each, the group assembly — and leaves behind what a
// chain needs to import the result.
//
// It is skipped unless CEREMONY_LIVE_DIR names an output directory, because it
// is the harness for verifying the tool against a running chain rather than a
// unit test. It exists at all because the production entry points read from a
// terminal, and a room's keyboard cannot be scripted: everything below is the
// tool's real code — preflightWith, verifyTranscription with the real
// crypto/rand sampler, identityOf, writeIdentity, buildGroup, writeArmorTo —
// driven through a console whose reader is a scripted custodian rather than a
// person.
//
// The armor export is how a key legitimately leaves the ceremony machine, and
// it is the only reason this harness can hand the devnet something to sign
// with. In a paper ceremony no custodian gets one; see the note on writeArmor.
// No phrase is written anywhere here either — each secret is zeroed on the way
// out of its iteration, exactly as runKeyCeremony does.
func TestFiveCustodianCeremony(t *testing.T) {
	dir := os.Getenv("CEREMONY_LIVE_DIR")
	if dir == "" {
		t.Skip("set CEREMONY_LIVE_DIR to run the ceremony harness against a chain")
	}
	configureAddresses()

	const passphrase = "ceremony-rehearsal-passphrase"
	names := []string{
		"Amara Okafor", "Bernard Kouassi", "Chipo Mwale", "Dalia Haddad", "Eshe Njoroge",
	}

	paths := make([]string, 0, len(names))
	for i, name := range names {
		id, err := oneCustodian(t, dir, name, passphrase, i)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		paths = append(paths, filepath.Join(dir, fmt.Sprintf("custodian-%s.json", slug(name))))
		t.Logf("custodian %d: %s", i+1, id.describe())
	}

	people, err := readIdentities(paths)
	if err != nil {
		t.Fatal(err)
	}

	documents, err := buildGroup(people, foundationPurpose(), 3, 7*24*time.Hour, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"group-members.json":    documents.members,
		"group-policy.json":     documents.policy,
		"group-create-msg.json": documents.msg,
		"group-genesis.json":    documents.genesis,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "policy-address"), []byte(documents.policyAddr+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Logf("group policy address: %s", documents.policyAddr)
}

// oneCustodian is runKeyCeremony with a scripted room instead of a real one.
func oneCustodian(t *testing.T, dir, name, passphrase string, index int) (identity, error) {
	t.Helper()

	s, err := newSecret()
	defer s.zero()
	if err != nil {
		return identity{}, err
	}

	// A custodian who transcribed correctly: whatever position is asked for,
	// the right word comes back. The sampler is the real one, so the positions
	// differ on every run.
	var transcript bytes.Buffer
	sheet := &perfectSheet{s: s, transcript: &transcript}
	c := &console{in: bufio.NewReader(sheet), out: &transcript, tty: -1}

	if _, err := preflightWith(c, "", nil); err != nil {
		return identity{}, err
	}
	if err := verifyTranscription(c, s); err != nil {
		return identity{}, err
	}

	priv, path, err := s.derive(0)
	if err != nil {
		return identity{}, err
	}
	defer zero(priv.Key)

	id, err := identityOf(name, roleCustodian, priv, path, time.Now())
	if err != nil {
		return identity{}, err
	}
	id.Ceremony = "rehearsal"

	recordPath := filepath.Join(dir, fmt.Sprintf("custodian-%s.json", slug(name)))
	_ = os.Remove(recordPath)
	if err := writeIdentity(recordPath, id); err != nil {
		return identity{}, err
	}

	armorPath := filepath.Join(dir, fmt.Sprintf("custodian-%d.asc", index+1))
	if err := writeArmorTo(armorPath, priv, passphrase); err != nil {
		return identity{}, err
	}
	return id, nil
}

// perfectSheet is a custodian who copied their phrase correctly.
//
// It answers by reading the prompt the console just printed rather than by
// replaying a fixed script, which is what lets it work against the real
// crypto/rand sampler: whichever five positions the check picks, the right word
// comes back. Everything it does is something a person would do — say yes to a
// checklist item, press return, read word nineteen off a sheet.
type perfectSheet struct {
	s          *secret
	transcript *bytes.Buffer
	read       int
	pending    []byte
}

var wordPrompt = regexp.MustCompile(`word\s+(\d+):\s*$`)

func (p *perfectSheet) Read(buf []byte) (int, error) {
	if len(p.pending) == 0 {
		p.pending = p.answer(p.lastPrompt())
	}
	n := copy(buf, p.pending)
	p.pending = p.pending[n:]
	return n, nil
}

// lastPrompt is everything the console has printed since the previous answer.
// The prompt is always the tail of it, because readLine prints and then reads.
func (p *perfectSheet) lastPrompt() string {
	all := p.transcript.String()
	if p.read > len(all) {
		p.read = 0
	}
	prompt := all[p.read:]
	p.read = len(all)
	return prompt
}

func (p *perfectSheet) answer(prompt string) []byte {
	if match := wordPrompt.FindStringSubmatch(prompt); match != nil {
		position, err := strconv.Atoi(match[1])
		if err != nil {
			return []byte("\n")
		}
		return append(append([]byte{}, p.s.word(position)...), '\n')
	}
	if strings.Contains(prompt, "[type yes]") {
		return []byte("yes\n")
	}
	return []byte("\n")
}
