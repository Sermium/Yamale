//go:build js && wasm

// Command wasm is the device's half of threshold signing, compiled for the
// browser.
//
// The device share must be generated on the device and must never leave it.
// That is not a preference — it is the entire security property. A share that
// is generated server-side and sent down, however carefully, was known to the
// server for a moment, and "the operator cannot move your money" stops being
// true of that account forever.
//
// So the protocol has to run where the user is, which on a phone means a
// browser, which means WebAssembly. This is that build.
//
// # What it exposes
//
// One object, `yamaleMPC`, holding a session API:
//
//	yamaleMPC.startSign(digestB64, shareJSON, signersCSV) -> {session, outbound}
//	yamaleMPC.handle(session, outboundJSON)               -> {outbound}
//	yamaleMPC.signature(session)                          -> {signature} | {pending:true}
//	yamaleMPC.publicKey(session)                          -> {x, y}
//	yamaleMPC.close(session)
//
// Every call is local. Nothing here opens a socket: the caller carries the
// outbound envelopes to the custodian and feeds the replies back, because the
// transport is where authentication lives and a crypto module that chose its
// own peer would be one nobody can audit apart from the network it happened to
// trust.
//
// # The mistake this file used to make
//
// An earlier version exposed `sign(digest, shares)` — every share, in the
// browser, at once. It worked, and it would have been a whole key in all but
// name: a page holding both the device and custodian shares can sign alone, so
// the property the entire design exists for would have been false for every
// account that ever used it. The session API below is the fix, and the reason
// mpc.SigningParty exists.
//
// # What it deliberately does not do
//
// It does not store anything. Sealing the share under the user's password and
// putting it somewhere is the caller's job, because the browser's storage APIs
// and the password KDF both belong to the application, and a crypto module that
// reached for localStorage would be one nobody could test.
//
// It does not do key generation. That is three parties and minutes of safe
// primes; it needs the custodian's pre-parameter pool and a session that
// survives a page reload, and it is the next piece rather than this one.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"syscall/js"

	"yamale/blockchain/mpc"
)

// Sessions are held here rather than handed to JavaScript, so that a share and
// a half-finished protocol state never cross into a heap the page can read.
// JavaScript gets an opaque handle and nothing else.
var (
	mu       sync.Mutex
	sessions = map[string]*mpc.SigningParty{}
	seq      int
)

func main() {
	js.Global().Set("yamaleMPC", js.ValueOf(map[string]any{
		"version":   js.FuncOf(version),
		"startSign": js.FuncOf(startSign),
		"handle":    js.FuncOf(handle),
		"signature": js.FuncOf(signature),
		"publicKey": js.FuncOf(publicKey),
		"close":     js.FuncOf(closeSession),
	}))
	// A wasm main that returns exits the module, taking the exported functions
	// with it, so it blocks forever on purpose.
	select {}
}

func version(this js.Value, args []js.Value) any {
	return map[string]any{
		"roles":     len(mpc.Roles),
		"threshold": mpc.Threshold,
		"signers":   mpc.Threshold + 1,
	}
}

// startSign begins this device's half of a signature.
//
// share is the device's own share and nothing else. Handing this function a
// second share would not make it faster; it would make the account custodial,
// and the party it builds accepts exactly one.
func startSign(this js.Value, args []js.Value) any {
	if len(args) < 3 {
		return fail("startSign needs a digest, a share and the signer list")
	}
	digest, err := base64.StdEncoding.DecodeString(args[0].String())
	if err != nil {
		return fail(fmt.Sprintf("the digest is not base64: %v", err))
	}
	var share mpc.Share
	if err := json.Unmarshal([]byte(args[1].String()), &share); err != nil {
		return fail(fmt.Sprintf("the share does not decode: %v", err))
	}
	signers := strings.Split(args[2].String(), ",")
	for i := range signers {
		signers[i] = strings.TrimSpace(signers[i])
	}

	party, err := mpc.NewSigningParty(share.Role, digest, share, signers)
	if err != nil {
		return fail(err.Error())
	}
	out, err := party.Outbound()
	if err != nil {
		return fail(err.Error())
	}

	mu.Lock()
	seq++
	id := fmt.Sprintf("s%d", seq)
	sessions[id] = party
	mu.Unlock()

	return map[string]any{"session": id, "outbound": encode(out)}
}

// handle feeds one message from the custodian in, and returns whatever this
// party wants to send back.
func handle(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return fail("handle needs a session and a message")
	}
	party, ok := lookup(args[0].String())
	if !ok {
		return fail("no such session")
	}
	var in mpc.Outbound
	if err := json.Unmarshal([]byte(args[1].String()), &in); err != nil {
		return fail(fmt.Sprintf("the message does not decode: %v", err))
	}
	if err := party.Handle(in); err != nil {
		return fail(err.Error())
	}
	out, err := party.Outbound()
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"outbound": encode(out)}
}

// signature reports the finished signature, or that the protocol is still
// running. Pending is a state and not an error: a caller that treated it as one
// would abandon a signature two rounds from done.
func signature(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("signature needs a session")
	}
	party, ok := lookup(args[0].String())
	if !ok {
		return fail("no such session")
	}
	sig, done := party.Signature()
	if !done {
		return map[string]any{"pending": true}
	}
	return map[string]any{"signature": base64.StdEncoding.EncodeToString(sig)}
}

// publicKey reports the account key this session signs for, computed from the
// device's own share.
//
// Local on purpose, and bech32 is left to the caller: encoding it here would
// mean importing cosmos-sdk, which drags in the store stack and does not
// compile for wasm at all. See mpc/cosmos. A device that took its address from
// the custodian's word for it is a device that can be told to show somebody
// else's balance.
func publicKey(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("publicKey needs a session")
	}
	party, ok := lookup(args[0].String())
	if !ok {
		return fail("no such session")
	}
	pub, err := party.PublicKey()
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"x": pub.X.String(), "y": pub.Y.String()}
}

// closeSession drops the party and its protocol state.
//
// Worth calling even on the happy path. A finished session still holds the
// share it was given and the intermediate values of a threshold protocol, and
// leaving those in a long-lived page is how a share outlives the moment it was
// needed.
func closeSession(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("close needs a session")
	}
	mu.Lock()
	delete(sessions, args[0].String())
	mu.Unlock()
	return map[string]any{"closed": true}
}

func lookup(id string) (*mpc.SigningParty, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := sessions[id]
	return p, ok
}

// encode renders outbound messages as JSON. js.ValueOf cannot carry a []byte,
// and a silently dropped protocol message is a protocol that hangs.
func encode(out []mpc.Outbound) string {
	if len(out) == 0 {
		return "[]"
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func fail(msg string) any {
	return map[string]any{"error": msg}
}
