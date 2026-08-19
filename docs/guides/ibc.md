# IBC: what is exposed, and what is turned off

Yamale is built with IBC compiled in. This page says exactly what that opens,
what the launch genesis turns off, and what has to be decided before any of it
is switched on — because on a permissioned chain those are policy decisions, not
configuration.

**Status:** IBC has never been exercised on this chain. No connection, channel
or relayer has been set up, and there are no tests. What follows is a review of
the surface, not a report of it working.

---

## What is compiled in

| | |
| --- | --- |
| **Core** | Client, connection and channel handshakes; Tendermint and solo-machine light clients. |
| **Transfer (ICS-20)** | Sending and receiving fungible tokens across a channel. |
| **Interchain accounts, host** | Lets an account on another chain drive an account **here**. |
| **Interchain accounts, controller** | Lets an account here drive an account on another chain. |

None of it can be reached without a connection, and a connection requires a
relayer that somebody deliberately runs. On day one the surface is inert.

## What the launch genesis turns off, and why

`scripts/testnet/02-build-canonical-genesis.sh` disables **both interchain
account submodules**, and refuses to produce a genesis in which they are on.

The reason is specific. An interchain account executes messages through the
message router:

```go
handler := k.msgRouter.Handler(msg)   // icahost/keeper/relay.go
```

The ante chain is never involved. This chain's validator permissioning is an
**ante decorator**, so it does not apply to anything arriving that way. With the
host enabled — and the module's default genesis enables it with
`allow_messages: ["*"]` — anyone able to open a channel could execute
`MsgCreateValidator` through an interchain account and join a permissioned
validator set without a vote. Every other message on the chain is reachable by
the same route.

That is the general lesson, worth carrying to anything added later: **an ante
handler only guards messages that arrive as transactions.** `authz.MsgExec`
reaches the router from inside a transaction, which the validator gate now
handles by descending into nested messages. Interchain accounts reach it from
outside one, which no ante decorator can see.

**Transfer is left enabled**, because it cannot execute messages — it can only
move tokens, and only once a channel exists.

## Before enabling interchain accounts

Turning the host on is a governance decision that needs an explicit
`allow_messages` list, never a wildcard. Work through, at minimum:

- **Anything the ante chain guards must be excluded.** Today that is
  `MsgCreateValidator` (the validator gate) — check the ante handler in
  `app/ante.go` for what has been added since.
- **Anything authority-gated is already safe**: the `Approve*` messages check
  the governance module account as their signer, and an interchain account is
  not it.
- **Value-moving messages** — `MsgSend`, `MsgSendPayment`, treasury spends —
  are only as safe as the counterparty chain's own security. An interchain
  account is controlled by whoever controls the account on the other side.

## The question transfer raises

An inbound ICS-20 transfer creates a voucher denom, `ibc/<hash>`, held like any
other token. It can then be sent, pooled on the AMM, held by a treasury, and
named as the amount on a payment record.

That sits awkwardly with the chain's central claim. `x/stablecoin` exists so
that every currency has exactly one governance-approved issuer, recorded on the
chain — and a voucher is a currency nobody here approved, backed by an escrow
account on a chain this one has no view of.

Nothing about that is broken; it is a decision that has not been made yet. The
options are the usual ones:

- **Leave it open.** Any counterparty chain's assets can arrive. Simple, and
  entirely dependent on which chains relayers connect.
- **Restrict channels.** Only open connections to counterparties governance has
  approved, which is the same shape as every other permission on this chain.
- **Restrict denoms downstream.** Let vouchers exist but keep them out of the
  places where "currency" carries a claim — the payment records especially.

Decide it before a relayer is running, not after somebody has an `ibc/…` balance.

## What has been verified

- **Blocked module accounts are refused as IBC receivers.** The transfer keeper
  is constructed with this chain's bank keeper, and ibc-go checks
  `IsBlockedAddr` on both send and receive — so a transfer to a treasury or pool
  module account fails rather than stranding funds, exactly as a local
  `MsgSend` does. Those accounts are covered by `app/blocked_accounts_test.go`.
- **The ceremony genesis has the ICA host and controller off.** Tested in both
  directions: a normal run produces `host_enabled: false`, and a run where that
  edit is removed is refused with a named reason and writes nothing.

## What has not

Everything else. No handshake, no channel, no relayer, no packet has been
exercised on this chain — not a transfer in either direction, not a timeout, not
a channel closure. Before IBC is used for anything real, it needs the treatment
the other modules got: a live counterparty, a relayer, and the failure cases
driven deliberately.

---

**Full reference:** IBC's own messages and queries are not in
[the generated reference](../reference/), which covers this chain's modules
only. See [ibc-go's documentation](https://ibc.cosmos.network/) for those.
