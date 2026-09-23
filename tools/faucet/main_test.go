package main

import (
	"errors"
	"testing"
)

// What the block said, and what the faucet tells the person who asked.
//
// This exists because of a real failure on devnet-2 on 2026-09-23. `uusdc` and
// `ueurc` were added to the faucet's currency list and to the wallet's claim
// screen, and never issued on the chain — zero supply, no approved issuer. So
// every claim broadcast a send of a denom that does not exist, the block
// refused it with code 5, and the faucet answered:
//
//	"The faucet could not send that right now. Please try again shortly."
//
// Which was false in the only way that matters. Nothing about it was going to
// change, and somebody read it and tried again.
//
// The faucet had the chain's own verdict in hand — the code AND the log naming
// the denom and the amount — and threw both away to print a guess.

func TestAnEmptyBalanceIsNotATransientFailure(t *testing.T) {
	err := classify(codeInsufficientFunds,
		"failed to execute message; message index: 0: spendable balance 0uusdc is smaller than 1000000000uusdc: insufficient funds")
	if err == nil {
		t.Fatal("an insufficient balance was reported as success")
	}

	var permanent permanentError
	if !errors.As(err, &permanent) {
		t.Fatalf("an empty balance must be permanent, got %T: %v", err, err)
	}
	// The denom survives into the operator's log. It is the one fact that says
	// WHICH currency is missing, and the handler deliberately does not repeat
	// it to the internet.
	if !contains(err.Error(), "uusdc") {
		t.Errorf("the reason does not name the denom: %v", err)
	}
}

func TestEverythingElseStaysTransient(t *testing.T) {
	// The default matters as much as the special case. A sequence collision
	// between two requests in one block is by far the most common failure and
	// genuinely does clear on a retry — classifying it as permanent would tell
	// somebody to give up on a faucet that was about to work.
	err := classify(32, "account sequence mismatch, expected 41, got 40")
	if err == nil {
		t.Fatal("a sequence mismatch was reported as success")
	}
	var permanent permanentError
	if errors.As(err, &permanent) {
		t.Fatal("a sequence mismatch was classified as permanent; the caller would stop retrying")
	}
}

func TestCodeZeroIsSuccess(t *testing.T) {
	if err := classify(0, ""); err != nil {
		t.Fatalf("a successful block was reported as a failure: %v", err)
	}
}

func TestTheClassificationIsOnTheCodeAndNotTheWords(t *testing.T) {
	// Matched on the code because the text is a human-readable string the SDK
	// is free to reword, and a matcher keyed to its wording breaks silently on
	// an upgrade — reverting the faucet to telling people to retry forever.
	err := classify(codeInsufficientFunds, "")
	var permanent permanentError
	if !errors.As(err, &permanent) {
		t.Fatal("classification depends on the log text rather than the code")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
