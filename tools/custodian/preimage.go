package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	markettypes "yamale/blockchain/x/market/types"
	tokenisationtypes "yamale/blockchain/x/tokenisation/types"
)

// What the custodian is actually being asked to sign.
//
// # The hole this closes
//
// The sign request used to carry `amount` as a number the CALLER supplied, and
// the field's own comment said what was wrong with it: "Asserted by the caller
// today, which means a thief asserts zero." A second-factor threshold on a
// figure the attacker chooses is not a control. It is a formality that the one
// person it exists to stop is free to skip.
//
// So the caller now sends the PREIMAGE — the marshalled SignDoc whose SHA-256
// is the digest — and the service checks the hash itself. What it reads out of
// those bytes is not asserted by anybody: if the preimage hashes to the digest,
// it IS the transaction being signed, and the value in it is the value that
// will move.
//
// # What it costs, which is not nothing
//
// The custodian previously learned nothing about a payment: 32 opaque bytes in,
// a signature out. It now sees the recipient and the amount of everything it
// co-signs. That is a real privacy loss and it is the price of the threshold
// meaning anything. It is mitigated only by what the service does NOT do with
// it: the preimage is parsed and discarded, never stored, never logged, and the
// account record gains nothing from it.
//
// # Failing toward more proof
//
// The service can value a bank send. It cannot value a market order without
// reading the book, and it cannot value a message type it has never heard of —
// which includes every message added after this binary was built. Every one of
// those makes the value UNKNOWN, and unknown demands the second factor.
//
// That is the only safe direction. A service that assumed an unrecognised
// message was small would be one an attacker beats by wrapping the payment in
// anything this file does not list.

// ErrPreimageMismatch is a preimage that does not hash to the digest offered.
var ErrPreimageMismatch = errors.New(
	"the preimage does not hash to the digest; one of them is not this transaction")

// Valued is what the service could work out about a transaction.
type Valued struct {
	// PerDenom is the outbound value by denomination, in base units.
	PerDenom map[string]uint64
	// Known is false when ANY message could not be valued, in which case
	// PerDenom is a floor and must not be treated as the total.
	Known bool
	// Why names the first message that could not be valued, for the log line a
	// person reads when their payment unexpectedly demanded a code.
	Why string
}

// Highest is the largest single denomination's value, which is what the
// threshold is compared against.
//
// Per denomination rather than summed, because summing uusd and uyml would add
// numbers that are not the same kind of thing. Crude — a threshold of 1,000,000
// means one YML and one dollar alike — and safe in the direction that matters,
// since it can only ever demand the factor more often than a currency-aware
// rule would.
func (v Valued) Highest() uint64 {
	var top uint64
	for _, amount := range v.PerDenom {
		if amount > top {
			top = amount
		}
	}
	return top
}

// ValueOfPreimage checks the preimage against the digest and reads what it moves.
func ValueOfPreimage(preimage, digest []byte) (Valued, error) {
	sum := sha256.Sum256(preimage)
	if subtle.ConstantTimeCompare(sum[:], digest) != 1 {
		return Valued{}, ErrPreimageMismatch
	}

	var doc txtypes.SignDoc
	if err := doc.Unmarshal(preimage); err != nil {
		return Valued{}, fmt.Errorf("the preimage is not a SignDoc: %w", err)
	}
	var body txtypes.TxBody
	if err := body.Unmarshal(doc.BodyBytes); err != nil {
		return Valued{}, fmt.Errorf("the SignDoc's body does not decode: %w", err)
	}

	out := Valued{PerDenom: map[string]uint64{}, Known: true}
	for _, msg := range body.Messages {
		if msg == nil {
			out.Known, out.Why = false, "a message with no type"
			continue
		}
		if err := valueMessage(msg.TypeUrl, msg.Value, out.PerDenom); err != nil {
			// One unvaluable message makes the WHOLE transaction unvaluable.
			// Anything else would let an attacker hide a payment behind a
			// message this binary has never heard of.
			out.Known = false
			if out.Why == "" {
				out.Why = fmt.Sprintf("%s: %v", msg.TypeUrl, err)
			}
		}
	}

	// The fee leaves the account too, and a transaction whose fee is the
	// interesting part is a transaction worth a second factor.
	var authInfo txtypes.AuthInfo
	if err := authInfo.Unmarshal(doc.AuthInfoBytes); err == nil && authInfo.Fee != nil {
		for _, coin := range authInfo.Fee.Amount {
			add(out.PerDenom, coin.Denom, coin.Amount.Uint64())
		}
	}

	return out, nil
}

// errUnvaluable is returned for a message this service cannot put a number on.
var errUnvaluable = errors.New("this service cannot value this message type")

// valueMessage adds one message's outbound value, or refuses to.
//
// Only the messages that MOVE VALUE OUT of the signer are listed. A message
// that receives — a claim, a listing's proceeds — is not a risk the second
// factor exists for, and treating a receipt as a large outbound would demand a
// code for being paid.
func valueMessage(typeURL string, raw []byte, into map[string]uint64) error {
	switch typeURL {
	case "/cosmos.bank.v1beta1.MsgSend":
		var m banktypes.MsgSend
		if err := m.Unmarshal(raw); err != nil {
			return err
		}
		for _, coin := range m.Amount {
			add(into, coin.Denom, coin.Amount.Uint64())
		}
		return nil

	case "/cosmos.bank.v1beta1.MsgMultiSend":
		var m banktypes.MsgMultiSend
		if err := m.Unmarshal(raw); err != nil {
			return err
		}
		for _, in := range m.Inputs {
			for _, coin := range in.Coins {
				add(into, coin.Denom, coin.Amount.Uint64())
			}
		}
		return nil

	case "/blockchain.market.v1.MsgBid":
		// The money is escrowed in this block, so it leaves now.
		var m markettypes.MsgBid
		if err := m.Unmarshal(raw); err != nil {
			return err
		}
		add(into, m.Amount.Denom, m.Amount.Amount.Uint64())
		return nil

	case "/blockchain.market.v1.MsgBuy":
		// The price is on the LISTING, not in the message. Valuing this would
		// mean reading chain state the custodian deliberately does not have, so
		// it is honestly unvaluable rather than guessed at zero.
		return fmt.Errorf("%w: the price is on the listing, not in the message", errUnvaluable)

	case "/blockchain.market.v1.MsgPlaceOrder":
		var m markettypes.MsgPlaceOrder
		if err := m.Unmarshal(raw); err != nil {
			return err
		}
		if m.Side != markettypes.SIDE_BID {
			// An ask sends shares, not money. Shares are not denominated in
			// anything the threshold is set in, and an ask's proceeds arrive
			// rather than leave.
			return nil
		}
		// quantity * unit_price, the keeper's own arithmetic. The denomination
		// is the vehicle's settlement currency, which this message does not
		// carry — so the value is real and its denomination is not, and that
		// makes it unvaluable against a per-denomination threshold.
		return fmt.Errorf(
			"%w: a bid's settlement currency is on the vehicle, not in the message", errUnvaluable)

	case "/blockchain.tokenisation.v1.MsgFundVault":
		var m tokenisationtypes.MsgFundVault
		if err := m.Unmarshal(raw); err != nil {
			return err
		}
		add(into, m.Amount.Denom, m.Amount.Amount.Uint64())
		return nil

	case
		// Receipts and administration: nothing leaves the signer's balance, so
		// nothing here is what a second factor is protecting.
		"/blockchain.tokenisation.v1.MsgClaim",
		"/blockchain.tokenisation.v1.MsgExchangeDividend",
		"/blockchain.market.v1.MsgCancelListing",
		"/blockchain.market.v1.MsgCancelAuction",
		"/blockchain.market.v1.MsgCancelOrder",
		"/blockchain.market.v1.MsgSettleAuction":
		return nil

	default:
		return fmt.Errorf("%w: %s", errUnvaluable, typeURL)
	}
}

func add(into map[string]uint64, denom string, amount uint64) {
	if denom == "" {
		return
	}
	into[denom] += amount
}
