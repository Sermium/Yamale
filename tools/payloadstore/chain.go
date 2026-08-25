package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// chainReader answers the two questions the store cannot answer for itself:
// who is a party to a payment, and which key does an account read with.
//
// Both come from chain state rather than from this service's configuration.
// That is the point of publishing them on-chain: a store with its own list of
// who may read would be a second answer to a question the chain already
// answers, and the second answer is the one that goes stale — here, by leaving
// a revoked regulator able to read after the appointment moved.
type chainReader struct {
	rest string
	http *http.Client
}

func newChainReader(rest string) *chainReader {
	return &chainReader{
		rest: strings.TrimSuffix(rest, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// paymentRecord is the subset of the on-chain record entitlement depends on.
type paymentRecord struct {
	Debtor                 string `json:"debtor"`
	Creditor               string `json:"creditor"`
	SettlementJurisdiction string `json:"settlement_jurisdiction"`
	MetadataHash           string `json:"metadata_hash"`
}

func (c *chainReader) get(path string, out any) error {
	resp, err := c.http.Get(c.rest + path)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chain returned %d for %s", resp.StatusCode, path)
	}
	return json.Unmarshal(body, out)
}

func (c *chainReader) paymentRecord(participant, endToEndID string) (paymentRecord, error) {
	q := url.Values{}
	q.Set("instructing_participant", participant)
	q.Set("end_to_end_id", endToEndID)

	var out struct {
		PaymentRecord paymentRecord `json:"payment_record"`
	}
	if err := c.get("/yamale/blockchain/paymsg/v1/payment_record_by_id?"+q.Encode(), &out); err != nil {
		return paymentRecord{}, err
	}
	if out.PaymentRecord.Debtor == "" {
		return paymentRecord{}, errors.New("no such payment")
	}
	return out.PaymentRecord, nil
}

// latestLiveViewingKey returns the newest key an account may still be sealed to.
//
// Live only. Sealing a challenge to a revoked key would hand the puzzle to
// whoever the revocation was declared because of, and the challenge is the
// gate on reading every payload this account is entitled to.
func (c *chainReader) latestLiveViewingKey(address string) ([]byte, error) {
	var out struct {
		Keys []struct {
			Version         string `json:"version"`
			PublicKey       string `json:"public_key"`
			RevokedAtHeight string `json:"revoked_at_height"`
		} `json:"keys"`
	}
	if err := c.get("/yamale/blockchain/alias/v1/viewing_keys/"+url.PathEscape(address), &out); err != nil {
		return nil, err
	}

	// The query answers newest first, but the order is not relied on: a
	// response that arrived reversed would otherwise hand back a superseded key
	// and every challenge would be sealed to the wrong half, which looks like
	// the caller holding a bad key.
	var best []byte
	var bestVersion uint64
	for _, k := range out.Keys {
		if k.RevokedAtHeight != "" && k.RevokedAtHeight != "0" {
			continue
		}
		version, err := strconv.ParseUint(k.Version, 10, 64)
		if err != nil {
			continue
		}
		if best != nil && version <= bestVersion {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(k.PublicKey)
		if err != nil {
			continue
		}
		best, bestVersion = raw, version
	}
	if best == nil {
		return nil, errors.New("no live viewing key")
	}
	return best, nil
}

// entitled resolves whether one account may read one payment's payload.
//
// Four ways in, and no fifth. The payer and the payee because it is their
// payment; the regulator of the *declared settlement jurisdiction* because that
// single declaration is what settles which authority has standing over a
// cross-border payment; and a live auditor because aggregate checks cross
// accounts and no party's own keys can perform them.
//
// The instructing participant is deliberately not on the list. It operates this
// store and can read the ciphertext off its own disk, but it is not sealed into
// the envelope — so it cannot decrypt what it holds, and this service must not
// pretend otherwise by granting it a route it could not use.
func (c *chainReader) entitled(account string, record paymentRecord) (bool, error) {
	if account == record.Debtor || account == record.Creditor {
		return true, nil
	}

	// One question, not two. The chain resolves who may open a payload settling
	// in a country — the appointed regulator, and every holder of
	// ROLE_SUPERVISOR covering it, including chain-wide holders — and answers in
	// a single call. This store previously asked only about the regulator, so a
	// supervisor the chain named as entitled was refused by the service holding
	// the ciphertext: the entitlement existed on chain and nowhere else.
	//
	// Asking the chain's own question also means the expiry and scope rules live
	// in one place. Two implementations of "may this account read" eventually
	// disagree, and the one that disagrees permissively is a supervisor reading a
	// country it was never granted.
	if record.SettlementJurisdiction != "" {
		var out struct {
			Readers []struct {
				Address string `json:"address"`
			} `json:"readers"`
		}
		err := c.get("/yamale/blockchain/alias/v1/payload_readers/"+url.PathEscape(record.SettlementJurisdiction), &out)
		// A country with nobody entitled is not an error. The payment is simply
		// readable by its two parties and any auditor, which is a real and
		// expected state before an authority has been named.
		if err == nil {
			for _, reader := range out.Readers {
				if reader.Address == account {
					return true, nil
				}
			}
		}
	}

	var auditors struct {
		Auditors []struct {
			Grant struct {
				Address string `json:"address"`
			} `json:"grant"`
		} `json:"auditors"`
	}
	// The chain filters expired grants out of this response, so the store does
	// not re-implement the expiry rule. Two implementations of "is this grant
	// live" would eventually disagree, and the one that disagrees in the
	// permissive direction is a lapsed auditor still reading.
	if err := c.get("/yamale/blockchain/alias/v1/auditors", &auditors); err != nil {
		return false, err
	}
	for _, a := range auditors.Auditors {
		if a.Grant.Address == account {
			return true, nil
		}
	}
	return false, nil
}
