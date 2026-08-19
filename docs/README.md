# Yamale documentation

Three layers, split by what you are trying to do. They are deliberately not
blended: mixing conceptual explanation into a task guide makes the task longer,
and mixing tasks into a reference makes the reference incomplete.

## [Learn](guides/concepts.md)

What this chain is, what it is for, and the handful of ideas the rest of the
documentation assumes. Read this if you are deciding whether Yamale solves a
problem you have. No commands.

## [Do](guides/)

Task-shaped walkthroughs with a stated starting point and an end state you can
verify. Every command in these pages has been run against a real node before
publishing.

- [Run a local chain](guides/local-devnet.md) — a single-node network on your
  machine in about two minutes.
- [Make a key](guides/wallet.md) — a recovery phrase, its addresses, and an
  encrypted keystore, generated on a machine with no network.
- [Send a payment](guides/payments.md) — approved institutions, registered
  customers, and the statement entry a payment leaves behind.
- [Set up a treasury](guides/treasury.md) — shared funds, roles, spending
  limits, and a vesting schedule.
- [Price feeds and asset valuations](guides/oracle.md) — running a feeder,
  delegating a hot key, and having a real-world asset valued.
- [Currencies Yamale carries](guides/currencies.md) — the 42 African currencies,
  their denoms, and the YML-hub pool model (generated from the currency table).
- [Issue a currency](guides/stablecoin.md) — registering, being approved, and
  minting a fiat-referenced token.
- [Trade and provide liquidity](guides/amm.md) — pools, swaps, slippage, and
  what providing liquidity actually earns.
- [Freeze a scammer and recover the funds](guides/enforcement.md) — the case,
  the vote, the seizure, and what stops the same power being aimed at you.
- [Run a validator](guides/validator.md) — applying, being approved, bonding,
  and feeding prices.
- [IBC: what is exposed, and what is turned off](guides/ibc.md) — the surface
  reviewed, the interchain-accounts decision, and what is still untested.

## [Reference](reference/)

Every message, query, state type, parameter and error code, **generated** from
the protobuf definitions, each module's registered errors, and its own
`DefaultParams()`.

These pages are not written by hand and must not be edited. A reference that
drifts from the code is worse than no reference, because somebody trusts it and
finds out in production. Regenerate with:

```bash
make docs
```

`static/openapi.json` describes the same REST surface as a machine-readable
Swagger document, merged from the protobuf annotations by `make openapi`. It is
generated on the same terms and guarded the same way — it had drifted two whole
modules behind the chain before that check existed.

`make docs-check` fails the build if the committed output has fallen behind the
code, so drift is caught in CI rather than by a reader.

## What is missing

Honest list, so nobody hunts for something that does not exist yet:

- No API tutorial for the `@yamale/chain` client; see
  [clients/README.md](../clients/README.md) for what it does.
- The explorer signs transfers, staking and governance votes. The SDK can now
  sign this chain's own messages too — payments, treasury spends, swaps, price
  submissions — but the explorer does not yet expose screens for them, so the
  guides above still describe those flows through the CLI.
- IBC has never been exercised: no connection, channel, relayer or packet, and
  no tests. The surface is reviewed in [the IBC guide](guides/ibc.md) and the
  ceremony disables interchain accounts, but nothing about it has been seen
  working.
- Accounts have no human-readable identity. `yml1chm…` is the only way to name
  someone. The replacement — a chain-assigned user ID and a private address book
  — is designed in [User IDs and the address book](guides/identity.md) and not
  yet built.
- Outside assets cannot come in. There is no bridge and there will not be one —
  the model is custodial issuance, decided and written up in
  [Bringing outside assets in](guides/custody.md), but nothing is built. Read it
  as a design note, not a guide: it has no commands, because there is nothing yet
  to run.
- Nothing consumes the oracle's prices yet. The `PriceSource` interface and its
  refuse-when-stale behaviour are in place and tested, but lending against
  real-world assets — the thing that would use them — is a later phase.
