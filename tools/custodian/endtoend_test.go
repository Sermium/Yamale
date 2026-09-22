package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/recovery"
	"yamale/blockchain/recovery/chain"
)

// The whole thing, across two services, with nothing stubbed.
//
// Every other test here checks one rule. This one asks the only question that
// matters to a person: if I enrol, lose my phone, and go through a recovery,
// do I get my account back — the same account, able to sign?
//
// It is modelled on the `pay` command of the operator tool it replaces, whose
// comment said the quiet part: everything above it can be satisfied by a
// library that is subtly wrong; a transaction the chain accepts cannot. There
// is no chain in this test, but the equivalent holds — a signature that
// verifies under the ORIGINAL public key cannot be produced by a recovery that
// reassembled the wrong bytes.
//
// Two servers, because one process holding both holdings is the arrangement the
// whole design exists to avoid, and a test that used one would be testing a
// deployment nobody runs.

type service struct {
	server *server
	srv    *httptest.Server
	clock  *time.Time
}

func newService(t *testing.T, role string) *service {
	t.Helper()
	dir := t.TempDir()
	// Different sealing keys per deployment, as in production. A test that gave
	// both the same key would pass while hiding the case where a holding
	// written by one service is readable by the other.
	key := []byte(strings.Repeat(role[:1], 32))
	store, err := NewStore(dir, role, key)
	require.NoError(t, err)

	index, err := NewBlindIndex(strings.Repeat("pepper-", 8))
	require.NoError(t, err)

	clock := new(time.Time)
	*clock = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	recoveries, err := NewRecoveries(t.TempDir())
	require.NoError(t, err)
	recoveries.now = func() time.Time { return *clock }

	s := &server{
		store: store, index: index, role: role,
		recoveries: recoveries, notifier: &fakeNotifier{},
		started: time.Now(),
		now:     func() time.Time { return *clock },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrol", s.handleEnrol)
	mux.HandleFunc("POST /v1/recovery/initiate", s.handleRecoveryInitiate)
	mux.HandleFunc("POST /v1/recovery/approve", s.handleRecoveryApprove)
	mux.HandleFunc("POST /v1/recovery/complete", s.handleRecoveryComplete)
	mux.HandleFunc("POST /v1/recovery/claim", s.handleRecoveryClaim)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &service{server: s, srv: srv, clock: clock}
}

func (sv *service) call(t *testing.T, path string, body, into any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(sv.srv.URL+path, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	defer resp.Body.Close()
	if into != nil && resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(into))
	}
	return resp.StatusCode
}

func TestAnAccountSurvivesLosingThePhone(t *testing.T) {
	const email = "amara@example.org"
	const password = "seven horses eating apples slowly"

	custodian := newService(t, recovery.RoleCustodian)
	recoveryService := newService(t, recovery.RoleRecovery)

	// ---------------------------------------------------------------- the device

	// An ordinary key. This is the whole saving: under the scheme this replaces
	// there was no such object at any point, and producing the shares of one
	// took about nine and a half minutes in a browser.
	key, err := secp256k1.GeneratePrivateKey()
	require.NoError(t, err)
	wanted, err := chain.Address(key.PubKey().SerializeCompressed())
	require.NoError(t, err)

	holdings, err := recovery.Protect(key.Serialize(), password)
	require.NoError(t, err)

	byRole := map[string]recovery.Holding{}
	for _, h := range holdings {
		byRole[h.Role] = h
	}
	// The device keeps its own and posts the other two. It is the only
	// participant that speaks to both services.
	require.NotEmpty(t, byRole[recovery.RoleDevice].Share.Y,
		"the device was given no holding of its own")

	for _, target := range []struct {
		svc  *service
		role string
	}{{custodian, recovery.RoleCustodian}, {recoveryService, recovery.RoleRecovery}} {
		h := byRole[target.role]
		idx := target.svc.server.index.Of(email)
		sig, err := recovery.SignDeposit(key, idx, target.role, h)
		require.NoError(t, err)

		var out enrolResponse
		code := target.svc.call(t, "/v1/enrol", Deposit{
			Email: email, Password: password,
			PubKey:  key.PubKey().SerializeCompressed(),
			Holding: h, Signature: sig,
		}, &out)
		require.Equal(t, http.StatusOK, code, "enrolling with %s", target.role)
		require.Equal(t, wanted, out.Address,
			"the service derived a different address from the one the device holds")
	}

	// Enrolling twice is refused, which is what stops an enrolment overwriting
	// somebody's account.
	h := byRole[recovery.RoleCustodian]
	idx := custodian.server.index.Of(email)
	sig, err := recovery.SignDeposit(key, idx, recovery.RoleCustodian, h)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, custodian.call(t, "/v1/enrol", Deposit{
		Email: email, Password: password,
		PubKey: key.PubKey().SerializeCompressed(), Holding: h, Signature: sig,
	}, nil))

	// A holding meant for the other service is refused by this one.
	wrong := byRole[recovery.RoleRecovery]
	wrongSig, err := recovery.SignDeposit(key, idx, recovery.RoleCustodian, wrong)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, custodian.call(t, "/v1/enrol", Deposit{
		Email: "someone.else@example.org", Password: password,
		PubKey: key.PubKey().SerializeCompressed(), Holding: wrong, Signature: wrongSig,
	}, nil))

	// ------------------------------------------------- the phone goes in a river

	// The device holding is gone. Nothing below this line touches it, which is
	// the whole point — a recovery that quietly used it would prove nothing,
	// and zeroing the variable here to look thorough would prove nothing
	// either. What is left is two services and a password.

	claimed := make([]recovery.Holding, 0, 2)
	for _, svc := range []*service{custodian, recoveryService} {
		var rec recoveryView
		require.Equal(t, http.StatusOK, svc.call(t, "/v1/recovery/initiate",
			recoveryInitiateRequest{
				Email: email, Operator: "amina", Team: "support",
				Proof: "in person at an agent, with a national identity card",
			}, &rec))
		require.Equal(t, http.StatusOK, svc.call(t, "/v1/recovery/approve",
			recoveryApproveRequest{ID: rec.ID, Operator: "boubacar", Team: "risk"}, nil))
		require.Equal(t, http.StatusOK, svc.call(t, "/v1/recovery/approve",
			recoveryApproveRequest{ID: rec.ID, Operator: "chinwe", Team: "compliance"}, nil))

		*svc.clock = svc.clock.Add(RecoveryDelay + time.Minute)
		require.Equal(t, http.StatusOK,
			svc.call(t, "/v1/recovery/complete", recoveryIDRequest{ID: rec.ID}, nil))

		// Still withheld for the post-recovery window.
		require.Equal(t, http.StatusConflict, svc.call(t, "/v1/recovery/claim",
			claimRequest{ID: rec.ID, Email: email, Password: password}, nil))

		*svc.clock = svc.clock.Add(PostRecoveryFreeze + time.Minute)
		var out claimResponse
		require.Equal(t, http.StatusOK, svc.call(t, "/v1/recovery/claim",
			claimRequest{ID: rec.ID, Email: email, Password: password}, &out))
		require.Equal(t, wanted, out.Address)
		claimed = append(claimed, out.Holding)
	}

	// --------------------------------------------- back on a new phone, with two

	require.Len(t, claimed, 2)

	// THE POINT. Two holdings and the password rebuild the key, and the key is
	// the same key — proved by a signature verifying under the ORIGINAL public
	// key, not by comparing bytes, because comparing bytes would also pass if
	// both sides were wrong in the same way.
	restored, err := recovery.Restore(claimed, password)
	require.NoError(t, err)

	recovered := secp256k1.PrivKeyFromBytes(restored)
	again, err := chain.Address(recovered.PubKey().SerializeCompressed())
	require.NoError(t, err)
	require.Equal(t, wanted, again, "the recovered key controls a different account")

	message := []byte("a transaction this account is paying for, digested")
	digest := make([]byte, 32)
	copy(digest, message)
	sigAfter := ecdsa.Sign(recovered, digest)
	require.True(t, sigAfter.Verify(digest, key.PubKey()),
		"the recovered key cannot sign for the account it recovered")

	// And the wrong password rebuilds nothing, from the same two holdings.
	_, err = recovery.Restore(claimed, "the wrong password entirely")
	require.ErrorIs(t, err, recovery.ErrWrongSeal)
}

// The claim that the product is sold on, checked against what the operator can
// actually reach.
//
// Everything both services store for this account, with no password: it must
// not contain the key, and the two holdings together must not open.
func TestTheOperatorHoldingBothServicesStillCannotOpenTheAccount(t *testing.T) {
	const email = "bakary@example.org"
	const password = "seven horses eating apples slowly"

	custodian := newService(t, recovery.RoleCustodian)
	recoveryService := newService(t, recovery.RoleRecovery)

	key, err := secp256k1.GeneratePrivateKey()
	require.NoError(t, err)
	holdings, err := recovery.Protect(key.Serialize(), password)
	require.NoError(t, err)

	both := make([]recovery.Holding, 0, 2)
	for _, target := range []struct {
		svc  *service
		role string
	}{{custodian, recovery.RoleCustodian}, {recoveryService, recovery.RoleRecovery}} {
		var h recovery.Holding
		for _, c := range holdings {
			if c.Role == target.role {
				h = c
			}
		}
		idx := target.svc.server.index.Of(email)
		sig, err := recovery.SignDeposit(key, idx, target.role, h)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, target.svc.call(t, "/v1/enrol", Deposit{
			Email: email, Password: password,
			PubKey: key.PubKey().SerializeCompressed(), Holding: h, Signature: sig,
		}, nil))

		// Read straight out of the store, which is what an operator with the
		// sealing key and the disk has.
		account, err := target.svc.server.store.Get(idx)
		require.NoError(t, err)
		stored, err := target.svc.server.store.Holding(account)
		require.NoError(t, err)
		both = append(both, stored)
	}

	secret := key.Serialize()
	for _, h := range both {
		require.NotContains(t, string(h.Share.Y), string(secret),
			"a stored holding contains the key")
	}

	// Both of them, which is the collusion case.
	_, err = recovery.Restore(both, "a guess at the password")
	require.ErrorIs(t, err, recovery.ErrWrongSeal,
		"two services together opened an account without the password")

	// And with the password they DO open it — otherwise the test above would
	// pass against a scheme that simply never works.
	restored, err := recovery.Restore(both, password)
	require.NoError(t, err)
	require.True(t, recovery.Equal(restored, secret))
}
