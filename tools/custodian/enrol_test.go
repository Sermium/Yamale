package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// Enrolment, driven the way a browser drives it: one device, two independent
// services, and no process anywhere holding two shares.
//
// These tests are the honest version of the claim in the package comment. It
// would be easy to assert that the endpoints return 200; what is asserted here
// is that the account which comes out the other end can actually sign, that the
// device's share never reached a server, and that the refusals refuse.

// enrolService is one deployment — custodian or recovery — with its own store,
// its own sealing key and its own pepper.
type enrolService struct {
	t      *testing.T
	role   string
	srv    *httptest.Server
	server *server
	// session is this service's enrolment id, assigned at start.
	session string
}

func newEnrolService(t *testing.T, role string, pre *keygen.LocalPreParams) *enrolService {
	t.Helper()
	dir := t.TempDir()
	// Distinct sealing keys per role, because sharing one would make a stolen
	// key open both stores, which is the situation two deployments exist to
	// avoid.
	store, err := NewStore(dir, role, []byte(strings.Repeat(role[:1], 32)))
	if err != nil {
		t.Fatalf("store for %s: %v", role, err)
	}
	index, err := NewBlindIndex(strings.Repeat("p", 32) + role)
	if err != nil {
		t.Fatalf("index for %s: %v", role, err)
	}

	s := &server{
		store: store, sessions: NewSessions(), index: index,
		role: role, enrolments: NewEnrolments(), started: time.Now(),
		// A pool of exactly one, pre-filled with material the test generated
		// once. Generating safe primes per service per test would put this file
		// at half an hour.
		preParams: newFixedPool(pre),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/enrol/start", s.handleEnrolStart)
	mux.HandleFunc("POST /v1/enrol/message", s.handleEnrolMessage)
	mux.HandleFunc("POST /v1/enrol/finish", s.handleEnrolFinish)
	mux.HandleFunc("POST /v1/sign/start", s.handleStart)
	mux.HandleFunc("POST /v1/sign/message", s.handleMessage)
	mux.HandleFunc("POST /v1/sign/result", s.handleResult)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &enrolService{t: t, role: role, srv: srv, server: s}
}

// newFixedPool is a pool that hands out one prepared set and then refuses,
// which is also how the real pool behaves when it is empty.
func newFixedPool(pre *keygen.LocalPreParams) *PreParamPool {
	p := NewPreParamPool(1, func() (*keygen.LocalPreParams, error) {
		// Never called: the shelf below is stocked before anybody asks. Present
		// because the pool requires a generator and a nil one would reach for
		// real safe primes.
		return pre, nil
	})
	return p
}

func (e *enrolService) post(t *testing.T, path string, body any, into any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	resp, err := http.Post(e.srv.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("posting to %s: %v", path, err)
	}
	defer resp.Body.Close()
	if into != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
	}
	return resp.StatusCode
}

// enrol runs a full three-party key generation over HTTP and returns the
// device's own share plus the address every party agreed on.
func enrol(
	t *testing.T, custodian, recovery *enrolService,
	device *mpc.KeygenParty, email, password string,
) (mpc.Share, string) {
	t.Helper()

	services := map[string]*enrolService{
		mpc.RoleCustodian: custodian,
		mpc.RoleRecovery:  recovery,
	}

	// Both services start their party. The device's is already running.
	pending := map[string][]mpc.Outbound{}
	for role, svc := range services {
		var out enrolStartResponse
		if code := svc.post(t, "/v1/enrol/start",
			enrolStartRequest{Email: email, Password: password}, &out); code != http.StatusOK {
			t.Fatalf("%s refused to start enrolment: %d", role, code)
		}
		svc.session = out.Session
		pending[role] = out.Outbound
	}

	deliver := func(from string, msgs []mpc.Outbound) {
		for _, m := range msgs {
			targets := m.To
			if m.Broadcast {
				targets = nil
				for _, r := range mpc.Roles {
					if r != from {
						targets = append(targets, r)
					}
				}
			}
			for _, to := range targets {
				if to == mpc.RoleDevice {
					if err := device.Handle(m); err != nil {
						t.Fatalf("device handling a message from %s: %v", from, err)
					}
					continue
				}
				svc := services[to]
				var out enrolMessageResponse
				code := svc.post(t, "/v1/enrol/message",
					enrolMessageRequest{Session: svc.session, Message: &m}, &out)
				if code != http.StatusOK {
					t.Fatalf("%s refused a message from %s: %d", to, from, code)
				}
				pending[to] = append(pending[to], out.Outbound...)
			}
		}
	}

	var deviceShare mpc.Share
	// Tracked per service, and the loop runs until EVERY party is finished
	// rather than until the device is.
	//
	// The first version stopped as soon as the device had its share, which
	// looked right and was not: the services were still a round short, and
	// /finish answered "key generation has not finished yet". A real client
	// would have had exactly this bug, so the rule is worth stating — the
	// device knowing its own share says nothing about whether its peers know
	// theirs.
	done := map[string]bool{}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		progressed := false

		// The device's own party.
		out, err := device.Outbound()
		if err != nil {
			t.Fatalf("device: %v", err)
		}
		if len(out) > 0 {
			deliver(mpc.RoleDevice, out)
			progressed = true
		}

		// Each service, drained through the poll form of /message.
		for _, role := range []string{mpc.RoleCustodian, mpc.RoleRecovery} {
			svc := services[role]
			queued := pending[role]
			pending[role] = nil
			if len(queued) > 0 {
				deliver(role, queued)
				progressed = true
			}
			var pollOut enrolMessageResponse
			if code := svc.post(t, "/v1/enrol/message",
				enrolMessageRequest{Session: svc.session}, &pollOut); code != http.StatusOK {
				t.Fatalf("%s refused a poll: %d", role, code)
			}
			if len(pollOut.Outbound) > 0 {
				pending[role] = append(pending[role], pollOut.Outbound...)
				progressed = true
			}
			if pollOut.Done && !done[role] {
				done[role] = true
				progressed = true
			}
		}

		if s, ok := device.Share(); ok && deviceShare.Data.ShareID == nil {
			deviceShare = s
			progressed = true
		}
		if deviceShare.Data.ShareID != nil &&
			done[mpc.RoleCustodian] && done[mpc.RoleRecovery] &&
			len(pending[mpc.RoleCustodian]) == 0 && len(pending[mpc.RoleRecovery]) == 0 {
			break
		}
		if !progressed {
			if err := device.Err(); err != nil {
				t.Fatalf("device: %v", err)
			}
			if time.Now().After(deadline) {
				t.Fatal("enrolment stalled")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	pub, err := deviceShare.PublicKey()
	if err != nil {
		t.Fatalf("device public key: %v", err)
	}
	address, err := mpccosmos.Address(pub)
	if err != nil {
		t.Fatalf("device address: %v", err)
	}

	for role, svc := range services {
		var fin enrolFinishResponse
		if code := svc.post(t, "/v1/enrol/finish",
			enrolFinishRequest{Session: svc.session, Address: address}, &fin); code != http.StatusOK {
			t.Fatalf("%s refused to finish: %d", role, code)
		}
		if fin.Address != address {
			t.Fatalf("%s committed a different address", role)
		}
	}
	return deviceShare, address
}
