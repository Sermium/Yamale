package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestASignedRequestNamesItsSigner(t *testing.T) {
	s := testServer(t)
	k := testKey("a")
	r := sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix())
	got, err := s.signer(r, "file", func(string) string { return "abc" })
	require.NoError(t, err)
	require.Equal(t, addrOf(k), got)
}

func TestASignatureIsBoundToPurposeSubjectAndTime(t *testing.T) {
	s := testServer(t)
	k := testKey("a")
	subject := func(string) string { return "abc" }
	cases := map[string]func() error{
		"another purpose": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "profile", "abc", testNow.Unix()), "file", subject)
			return err
		},
		"another subject": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abd", testNow.Unix()), "file", subject)
			return err
		},
		"six minutes old": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix()-360), "file", subject)
			return err
		},
		"six minutes ahead": func() error {
			_, err := s.signer(sign(t, request("POST", "/x", nil), k, "file", "abc", testNow.Unix()+360), "file", subject)
			return err
		},
		"unsigned": func() error {
			_, err := s.signer(request("POST", "/x", nil), "file", subject)
			return err
		},
	}
	for name, run := range cases {
		require.Error(t, run(), name)
	}
}
