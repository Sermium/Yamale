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
//	yamaleMPC.outbound(session)                           -> {outbound}
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
// It DOES now do key generation, which this build once did not. The cost is
// stated rather than hidden: the safe primes behind this device's Paillier key
// take minutes, they are generated here because a device share generated
// anywhere else was known anywhere else, and the API is asynchronous so a page
// can say so instead of appearing to hang.
//
// What it still does not do is survive a page reload mid-keygen. A session is
// process state; closing the tab half way through loses it and the enrolment
// starts again. That is a real limitation and it is the caller's to explain.
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
		"outbound":  js.FuncOf(outbound),
		"signature": js.FuncOf(signature),
		"publicKey": js.FuncOf(publicKey),
		"close":     js.FuncOf(closeSession),

		// Key generation. Asynchronous on purpose: the safe primes behind this
		// device's key take minutes, and a synchronous call would freeze the
		// page with no way to tell a slow phone from a hung one.
		"startKeygen":    js.FuncOf(startKeygen),
		"keygenReady":    js.FuncOf(keygenReady),
		"keygenHandle":   js.FuncOf(keygenHandle),
		"keygenOutbound": js.FuncOf(keygenOutbound),
		"keygenShare":    js.FuncOf(keygenShare),
		"keygenClose":    js.FuncOf(keygenClose),
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

// outbound polls for messages the party has produced on its own.
//
// Necessary, and its absence deadlocked the first browser run. A tss-lib party
// does its work in a goroutine, and under WebAssembly goroutines are scheduled
// cooperatively on one thread — so at the moment startSign returns, the party
// has not run at all and has nothing to send. A caller that only ever collected
// messages as replies to handle() would sit forever waiting to be given the
// first one.
func outbound(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("outbound needs a session")
	}
	party, ok := lookup(args[0].String())
	if !ok {
		return fail("no such session")
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

/* ========================================================================== */
/*  Key generation                                                            */
/* ========================================================================== */
//
// The half this build deliberately did without, and the reason a browser could
// sign for an account but not create one: without it the device share has to be
// generated somewhere else, and a share generated somewhere else was known to
// somewhere else. "The operator cannot move your money" stops being true of
// that account forever.
//
// # What it costs, stated rather than discovered
//
// `startKeygen` generates this device's Paillier safe primes, and that takes
// MINUTES rather than milliseconds — on a phone, potentially several. There is
// no way around it that keeps the property: the custodian keeps a pool of
// pre-parameters precisely because it can, and the device cannot take from that
// pool without the custodian having known the primes behind the device's key.
//
// So it is asynchronous, and it reports progress. `startKeygen` returns
// immediately with a session whose state is `generating`; the caller polls
// `keygenReady` until it turns `ready`, which is the point at which there are
// messages to carry.
//
// # THIS MUST RUN IN A WEB WORKER
//
// Not a recommendation. Go's wasm runtime is single-threaded and its scheduler
// is cooperative, and the prime search does not yield — so the goroutine
// `startKeygen` spawns takes the one thread and does not give it back. Measured
// on the main thread, the tab stopped answering script entirely: not slow,
// frozen, with even a page-text read timing out.
//
// The asynchronous shape above is still right, because it is what lets a worker
// report progress to the page. But the page must not be the thing running it.
// An earlier version of this comment said the work was "broken up enough that
// the page keeps responding". It is not, and saying so cost a frozen tab to
// find out.

var (
	kmu     sync.Mutex
	keygens = map[string]*keygenSession{}
	kseq    int
)

// keygenSession is a party that may not exist yet.
//
// The party is nil while the primes are being generated, because
// NewKeygenParty does not return until they are. Holding the state here rather
// than blocking is what lets the page stay answerable.
type keygenSession struct {
	role    string
	party   *mpc.KeygenParty
	err     error
	started bool
}

// startKeygen begins this device's half of creating an account.
//
// Returns at once, and then the goroutine it spawns holds this runtime's only
// thread until the primes are found. In a Web Worker that is harmless and is
// the intended arrangement. On a page's main thread it freezes the tab — see
// the note above the session state, which was written after measuring exactly
// that.
func startKeygen(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("startKeygen needs a role")
	}
	role := strings.TrimSpace(args[0].String())

	sess := &keygenSession{role: role}

	kmu.Lock()
	kseq++
	id := fmt.Sprintf("k%d", kseq)
	keygens[id] = sess
	kmu.Unlock()

	go func() {
		// nil pre-parameters: generated here, on this device, which is the
		// whole point. See the note above on why the custodian's pool is not
		// available to us.
		party, err := mpc.NewKeygenParty(role, nil)
		kmu.Lock()
		sess.party, sess.err, sess.started = party, err, true
		kmu.Unlock()
	}()

	return map[string]any{"session": id, "state": "generating"}
}

// keygenReady says whether the primes are done, without blocking on them.
func keygenReady(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("keygenReady needs a session")
	}
	kmu.Lock()
	sess, ok := keygens[args[0].String()]
	kmu.Unlock()
	if !ok {
		return fail("no such session")
	}

	kmu.Lock()
	defer kmu.Unlock()
	switch {
	case sess.err != nil:
		return fail(sess.err.Error())
	case !sess.started:
		return map[string]any{"state": "generating"}
	default:
		out, err := sess.party.Outbound()
		if err != nil {
			return fail(err.Error())
		}
		return map[string]any{"state": "ready", "outbound": encode(out)}
	}
}

// keygenHandle feeds one message from a service in and returns what this device
// wants to send back.
func keygenHandle(this js.Value, args []js.Value) any {
	if len(args) < 2 {
		return fail("keygenHandle needs a session and a message")
	}
	party, err := keygenParty(args[0].String())
	if err != nil {
		return fail(err.Error())
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

// keygenOutbound drains anything this party has produced since it was last
// asked, without feeding anything in.
func keygenOutbound(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("keygenOutbound needs a session")
	}
	party, err := keygenParty(args[0].String())
	if err != nil {
		return fail(err.Error())
	}
	out, err := party.Outbound()
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"outbound": encode(out)}
}

// keygenShare returns this device's share, once there is one.
//
// THE ONLY COPY. Neither service has it and neither can reconstruct it. Sealing
// it under the user's password and storing it is the caller's job, for the same
// reason this module does not touch localStorage — and an application that gets
// this value and then loses it has destroyed the account's device half, which
// only the recovery service can stand in for.
func keygenShare(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("keygenShare needs a session")
	}
	party, err := keygenParty(args[0].String())
	if err != nil {
		return fail(err.Error())
	}
	share, ok := party.Share()
	if !ok {
		if e := party.Err(); e != nil {
			return fail(e.Error())
		}
		return map[string]any{"pending": true}
	}
	encoded, err := json.Marshal(share)
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"share": string(encoded)}
}

// keygenClose forgets a session. A half-finished keygen left in the map is a
// Paillier private key the page can no longer reach and has not released.
func keygenClose(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return fail("keygenClose needs a session")
	}
	kmu.Lock()
	delete(keygens, args[0].String())
	kmu.Unlock()
	return map[string]any{"closed": true}
}

func keygenParty(id string) (*mpc.KeygenParty, error) {
	kmu.Lock()
	defer kmu.Unlock()
	sess, ok := keygens[id]
	if !ok {
		return nil, fmt.Errorf("no such session")
	}
	if sess.err != nil {
		return nil, sess.err
	}
	if !sess.started {
		return nil, fmt.Errorf("this device is still generating its parameters")
	}
	return sess.party, nil
}
