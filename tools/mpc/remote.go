package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"yamale/blockchain/mpc"
)

// Signing with a share this process does not have.
//
// # Why this exists
//
// Every other path in this tool loads two share files and signs. That is the
// right shape for a ceremony and the wrong shape for a person: their custodian
// share lives on a server precisely so that losing their phone is survivable,
// and a client that needs a local copy of it has undone the arrangement.
//
// So this drives the custodian's session API. The device holds one share, the
// service holds the other, the two exchange protocol messages, and both end up
// computing the same signature — which is the property that lets the device
// check what it is about to broadcast rather than trust what it was handed.
//
// # What the service can refuse, and what it cannot
//
// It can refuse to start: wrong password, frozen account, an amount over the
// second-factor threshold, a recovery completed in the last 24 hours. Those
// refusals are the product, and they arrive here as the service's own wording
// because it is written for the person reading it.
//
// It cannot refuse the transaction. It is handed 32 opaque bytes and has no
// idea what they commit to. Anything that needs to be true of the payment has
// to be checked on the device, before this is called.

// remoteSigner is one signature, produced jointly with the custodian service.
type remoteSigner struct {
	base     string
	email    string
	password string
	http     *http.Client
}

func newRemoteSigner(base, email, password string) *remoteSigner {
	return &remoteSigner{
		base:     strings.TrimRight(base, "/"),
		email:    email,
		password: password,
		// Generous for the same reason enrolment's is: the far side is doing
		// real cryptography inside the request. Signing is much faster than key
		// generation, and a bound that only just fits is one that fails on a
		// loaded host rather than never.
		http: &http.Client{Timeout: 2 * time.Minute},
	}
}

// sign produces a 64-byte signature over digest, using the device's share here
// and the custodian's share there.
func (r *remoteSigner) sign(digest []byte, deviceShare mpc.Share, amount uint64) ([]byte, string, error) {
	if len(digest) != 32 {
		return nil, "", fmt.Errorf("expected a 32-byte digest, got %d", len(digest))
	}

	device, err := mpc.NewSigningParty(
		mpc.RoleDevice, digest, deviceShare,
		[]string{mpc.RoleDevice, mpc.RoleCustodian},
	)
	if err != nil {
		return nil, "", err
	}

	var start struct {
		Session  string         `json:"session"`
		Address  string         `json:"address"`
		Outbound []mpc.Outbound `json:"outbound"`
	}
	if err := r.post("/v1/sign/start", map[string]any{
		"email": r.email, "password": r.password,
		"digest": base64.StdEncoding.EncodeToString(digest),
		"amount": amount,
	}, &start); err != nil {
		return nil, "", err
	}

	pending := start.Outbound
	deadline := time.Now().Add(2 * time.Minute)

	for {
		progressed := false

		// Anything the custodian sent goes into the device's party.
		for _, m := range pending {
			if err := device.Handle(m); err != nil {
				return nil, "", fmt.Errorf("this device handling a message from the custodian: %w", err)
			}
			progressed = true
		}
		pending = nil

		// And anything the device wants to send goes over.
		out, err := device.Outbound()
		if err != nil {
			return nil, "", err
		}
		for _, m := range out {
			var res struct {
				Outbound []mpc.Outbound `json:"outbound"`
			}
			if err := r.post("/v1/sign/message", map[string]any{
				"session": start.Session, "message": m,
			}, &res); err != nil {
				return nil, "", fmt.Errorf("the custodian handling a message: %w", err)
			}
			pending = append(pending, res.Outbound...)
			progressed = true
		}

		if sig, done := device.Signature(); done {
			// Checked against the custodian's own copy rather than taken on
			// trust. Both parties compute the identical bytes, so a service
			// handing back a different signature from the one it helped produce
			// is caught here — before anything is broadcast.
			var res struct {
				Signature string `json:"signature"`
				Pending   bool   `json:"pending"`
			}
			if err := r.post("/v1/sign/result", map[string]any{"session": start.Session}, &res); err != nil {
				return nil, "", err
			}
			if !res.Pending && res.Signature != "" {
				theirs, err := base64.StdEncoding.DecodeString(res.Signature)
				if err != nil {
					return nil, "", fmt.Errorf("the custodian's signature does not decode: %w", err)
				}
				if !bytes.Equal(theirs, sig) {
					return nil, "", fmt.Errorf(
						"the custodian returned a different signature from the one this device computed; " +
							"nothing has been broadcast")
				}
			}
			return sig, start.Address, nil
		}

		if !progressed {
			if time.Now().After(deadline) {
				return nil, "", fmt.Errorf("the signature did not complete within two minutes")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (r *remoteSigner) post(path string, body, into any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := r.http.Post(r.base+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(payload, &e) == nil && e.Error != "" {
			// The service's own wording. These refusals are written for the
			// person who hit them and lose their point if restated here.
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("%s said %s", r.base, resp.Status)
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(payload, into)
}

// ------------------------------------------------------- the sign-remote command

// runSignRemote signs with the device's share here and the custodian's there.
//
// Kept as its own command rather than a flag on `sign`, because the two do
// genuinely different things and conflating them hides which one ran. `sign`
// needs two share files and proves the mathematics; this needs one share file
// and a password, and proves the deployment.
func runSignRemote(args []string) error {
	fs := flag.NewFlagSet("sign-remote", flag.ExitOnError)
	digestB64 := fs.String("digest", "", "32-byte digest to sign, base64")
	sharePath := fs.String("share", "", "this device's share file")
	custodian := fs.String("custodian", "", "base URL of the custodian service")
	email := fs.String("email", "", "the account's email")
	password := fs.String("password", "", "the account's password")
	amount := fs.Uint64("amount", 0, "the amount being moved, so the service can apply its second-factor rule")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sharePath == "" || *custodian == "" || *email == "" || *password == "" {
		return fmt.Errorf("--share, --custodian, --email and --password are all required")
	}
	digest, err := base64.StdEncoding.DecodeString(*digestB64)
	if err != nil {
		return fmt.Errorf("--digest is not base64: %w", err)
	}

	raw, err := os.ReadFile(*sharePath)
	if err != nil {
		return err
	}
	var share mpc.Share
	if err := json.Unmarshal(raw, &share); err != nil {
		return fmt.Errorf("reading the share: %w", err)
	}
	if share.Role != mpc.RoleDevice {
		// A device share, specifically. Handing this command a custodian share
		// would ask the service to sign with a second custodian share, which
		// tss-lib refuses in a way that reads like a protocol fault rather than
		// like the mistake it is.
		return fmt.Errorf(
			"that is a %s share; this command signs with the DEVICE's share and asks the service "+
				"for the other", share.Role)
	}

	sig, address, err := newRemoteSigner(*custodian, *email, *password).sign(digest, share, *amount)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(sig))
	fmt.Fprintf(os.Stderr, "signed for %s by device + custodian, and the custodian's share never moved\n", address)
	return nil
}
