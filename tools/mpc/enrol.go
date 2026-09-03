package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// enrol is the DEVICE's half of creating an account.
//
// # What this stands in for
//
// A browser. The real client is `mpc/wasm` running in a page, and this is the
// same protocol driven from a terminal so that enrolment can be exercised
// against real, separately deployed services without a browser in the loop.
// Everything it does, a page does.
//
// # The one thing to understand about this command
//
// It speaks to TWO services and it is the only participant that does. Each of
// them computes a share this process never sees, and this process computes one
// they never see. Nobody, at any point, holds two.
//
// So the share this writes to disk is not a copy of something the server has —
// it is the only copy in existence, and losing it means the account can be
// signed for only with the recovery service's help.
type enrolClient struct {
	base    string
	role    string
	session string
	http    *http.Client
}

func runEnrol(args []string) error {
	fs := flag.NewFlagSet("enrol", flag.ExitOnError)
	custodian := fs.String("custodian", "", "base URL of the custodian service")
	recovery := fs.String("recovery", "", "base URL of the recovery service")
	email := fs.String("email", "", "the account's email")
	password := fs.String("password", "", "the account's password (at least 12 characters)")
	out := fs.String("out", ".", "directory to write the device share into")
	timeout := fs.Duration("timeout", 10*time.Minute, "how long to allow the whole exchange")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *custodian == "" || *recovery == "" || *email == "" || *password == "" {
		return fmt.Errorf("--custodian, --recovery, --email and --password are all required")
	}
	if *custodian == *recovery {
		// Refused rather than allowed with a warning. Enrolling both roles
		// against one service would put two shares in one process, which is the
		// single thing this design exists to prevent — and it would still
		// produce a working account, so nothing later would notice.
		return fmt.Errorf(
			"--custodian and --recovery are the same service (%s). Two shares in one process is "+
				"exactly what splitting the key was for", *custodian)
	}

	fmt.Fprintln(os.Stderr, "generating this device's parameters; this takes a few minutes")
	pre, err := mpc.GeneratePreParams(mpc.KeygenTimeout)
	if err != nil {
		return fmt.Errorf("generating pre-parameters: %w", err)
	}

	device, err := mpc.NewKeygenParty(mpc.RoleDevice, pre)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	services := map[string]*enrolClient{
		mpc.RoleCustodian: {base: strings.TrimRight(*custodian, "/"), role: mpc.RoleCustodian, http: client},
		mpc.RoleRecovery:  {base: strings.TrimRight(*recovery, "/"), role: mpc.RoleRecovery, http: client},
	}

	pending := map[string][]mpc.Outbound{}
	for _, role := range []string{mpc.RoleCustodian, mpc.RoleRecovery} {
		svc := services[role]
		var res struct {
			Session  string         `json:"session"`
			Role     string         `json:"role"`
			Outbound []mpc.Outbound `json:"outbound"`
		}
		if err := svc.post("/v1/enrol/start", map[string]any{
			"email": *email, "password": *password,
		}, &res); err != nil {
			return fmt.Errorf("starting enrolment with the %s: %w", role, err)
		}
		if res.Role != role {
			// The service says which role it holds, and it must be the one we
			// addressed. Two services both configured as `custodian` would
			// otherwise produce an account with no recovery share and nothing
			// would say so until the day somebody needed it.
			return fmt.Errorf(
				"the service at %s reports role %q, not %q — check its --role", svc.base, res.Role, role)
		}
		svc.session = res.Session
		pending[role] = res.Outbound
	}
	fmt.Fprintln(os.Stderr, "both services have started their party; exchanging messages")

	deliver := func(from string, msgs []mpc.Outbound) error {
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
						return fmt.Errorf("this device handling a message from %s: %w", from, err)
					}
					continue
				}
				svc, ok := services[to]
				if !ok {
					return fmt.Errorf("a message was addressed to %q, which is nobody here", to)
				}
				var res struct {
					Outbound []mpc.Outbound `json:"outbound"`
					Done     bool           `json:"done"`
				}
				if err := svc.post("/v1/enrol/message", map[string]any{
					"session": svc.session, "message": m,
				}, &res); err != nil {
					return fmt.Errorf("the %s handling a message from %s: %w", to, from, err)
				}
				pending[to] = append(pending[to], res.Outbound...)
			}
		}
		return nil
	}

	var deviceShare mpc.Share
	done := map[string]bool{}
	deadline := time.Now().Add(*timeout)

	for {
		progressed := false

		out, err := device.Outbound()
		if err != nil {
			return err
		}
		if len(out) > 0 {
			if err := deliver(mpc.RoleDevice, out); err != nil {
				return err
			}
			progressed = true
		}

		for _, role := range []string{mpc.RoleCustodian, mpc.RoleRecovery} {
			svc := services[role]
			queued := pending[role]
			pending[role] = nil
			if len(queued) > 0 {
				if err := deliver(role, queued); err != nil {
					return err
				}
				progressed = true
			}
			// The poll. A round's messages can become available after the
			// response that triggered them was written, so a client that never
			// asks again stalls one round short with nothing logged.
			var res struct {
				Outbound []mpc.Outbound `json:"outbound"`
				Done     bool           `json:"done"`
			}
			if err := svc.post("/v1/enrol/message", map[string]any{"session": svc.session}, &res); err != nil {
				return fmt.Errorf("polling the %s: %w", role, err)
			}
			if len(res.Outbound) > 0 {
				pending[role] = append(pending[role], res.Outbound...)
				progressed = true
			}
			if res.Done && !done[role] {
				done[role] = true
				progressed = true
			}
		}

		if deviceShare.Data.ShareID == nil {
			if s, ok := device.Share(); ok {
				deviceShare = s
				progressed = true
				fmt.Fprintln(os.Stderr, "this device has its share")
			}
		}

		// Every party, not just this one. The device knowing its own share says
		// nothing about whether its peers know theirs, and /finish refuses a
		// generation that has not completed.
		if deviceShare.Data.ShareID != nil && done[mpc.RoleCustodian] && done[mpc.RoleRecovery] &&
			len(pending[mpc.RoleCustodian]) == 0 && len(pending[mpc.RoleRecovery]) == 0 {
			break
		}
		if !progressed {
			if err := device.Err(); err != nil {
				return err
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("enrolment stalled after %s", *timeout)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	pub, err := deviceShare.PublicKey()
	if err != nil {
		return err
	}
	address, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}

	// The share is written BEFORE the services are told to commit. If the
	// commit fails the account does not exist and this file is harmless; if the
	// write failed after committing, the account would exist and nobody could
	// ever sign for it.
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	path := filepath.Join(*out, "device.json")
	raw, err := json.MarshalIndent(deviceShare, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}

	for _, role := range []string{mpc.RoleCustodian, mpc.RoleRecovery} {
		svc := services[role]
		var res struct {
			Address string `json:"address"`
			Role    string `json:"role"`
		}
		if err := svc.post("/v1/enrol/finish", map[string]any{
			"session": svc.session, "address": address,
		}, &res); err != nil {
			return fmt.Errorf("the %s refused to commit: %w", role, err)
		}
		if res.Address != address {
			return fmt.Errorf("the %s committed %s, this device computed %s", role, res.Address, address)
		}
	}

	fmt.Printf("account   %s\n", address)
	fmt.Printf("share     %s   (the only copy; the services have neither)\n", path)
	fmt.Printf("signs     device + custodian, or device + recovery — never the two services alone\n")
	return nil
}

func (c *enrolClient) post(path string, body any, into any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.base+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// The service's own wording, not a status code. These messages are
		// written to be read by whoever is enrolling.
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(payload, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("%s said %s: %s", c.base, resp.Status, strings.TrimSpace(string(payload)))
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(payload, into)
}
