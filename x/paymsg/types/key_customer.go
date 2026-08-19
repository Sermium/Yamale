package types

import "cosmossdk.io/collections"

// CustomerKey maps an account to the approved participant that acts for it.
//
// Keyed by the customer rather than the participant because that is the
// direction every lookup goes: a payment names an instructing participant and
// the chain has to answer whether this debtor is theirs. It also enforces the
// one-participant-per-customer rule structurally — an account cannot bank with
// two institutions at once, so there is no ambiguity about who instructed a
// payment.
var CustomerKey = collections.NewPrefix("customer/value/")
