# What Yamale is

A blockchain for moving money between institutions that know who each other are.

That constraint is the design. Most chains are built for anonymous participants
and treat permissioning as something applications bolt on afterwards. Yamale
inverts it: who may validate, who may issue a currency, and who may route a
payment are all decisions the chain itself enforces, decided by governance and
visible to anyone.

The trade is deliberate. You give up permissionless participation at the
infrastructure layer, and you get a network where a payment's counterparties are
accountable entities rather than addresses.

---

## The ideas the rest of the documentation assumes

### Money is integers

Every amount on the chain is a whole number in a base unit. `uyml` is the base
unit of YML at six decimal places, so `12500000uyml` is 12.5 YML. There are no
fractional base units and no floating point anywhere near a balance.

Interfaces convert only when displaying. If you are writing one, do the same:
compute in base units, convert at the last moment, and truncate rather than
round, so a number you show is never larger than the number that exists.

### Permission is on-chain, and so is the decision

Four things on this chain require governance approval: becoming a validator,
issuing a currency, routing payments as an institution, and receiving a share of
transaction fees as a developer.

Each follows the same shape. Anyone may apply — that is a permissionless
message that records a pending application. Approval is a separate message whose
only valid signer is the governance account, which means it can only ever happen
as the payload of a proposal that passed. There is no path where an applicant
approves themselves, and every decision leaves a record of who voted for it.

### The validator set is closed, but the ledger is not

Only approved validators produce blocks. Everything else — holding funds,
sending payments, trading, opening a treasury — is open to any account. Reading
is open to everyone: there are no private balances and no permissioned queries.

### Supply is capped and decays toward it

Instead of open-ended inflation, new YML is issued on a fixed schedule that
halves toward a ceiling. Every block mints slightly less than the last, and the
total approaches a fixed number rather than growing forever.

### Commitments are stronger than policies

The treasury module draws a line most systems do not. When funds are committed
to somebody — a vesting grant, a scheduled disbursement — they leave the
treasury's spendable balance entirely. No administrator, no governance proposal
and no group of signers clearing their threshold can spend them.

This is enforced by where the money sits rather than by a rule that checks it,
which is the difference between a commitment and an intention.

### A value too old to trust is not a value

Every price the chain holds carries the moment it was observed, and every answer
carries how old it is. Nothing is silently frozen at its last known number: a feed
that stops reporting becomes explicitly unusable rather than quietly wrong, and
the paths that act on a price immediately — lending, liquidation — refuse a stale
one rather than guess.

The two kinds of value get there differently. A currency's rate is discovered:
validators each report what they observe and the chain takes the median weighted
by stake, so moving it costs the same as attacking consensus. A building's value
is attested: an appointed, independent party inspects it and signs a number, and
what the chain can guarantee is not that the number is right but that it is
attributable — who signed it, what they were admitted to value, and against which
report.

---

## What the chain does

| | |
| --- | --- |
| **Payments** | ISO 20022-shaped credit transfers between approved institutions, each leaving a queryable statement entry with its reference and purpose. |
| **Currencies** | Fiat-referenced tokens with a single governance-approved issuer each, who alone may mint and redeem. |
| **Treasuries** | Shared funds with roles, spending limits, time locks and vesting. |
| **Trading** | Constant-product liquidity pools; anyone may open one or provide liquidity. |
| **Staking** | Standard proof-of-stake economics over the approved validator set. |
| **Governance** | Proposals, deposits and voting, plus the four approval flows above. |
| **Valuation** | Exchange rates agreed by stake-weighted validator vote, and real-world assets valued by governance-appointed independent parties. |

Not built yet: lending against real-world assets. The valuation layer it needs is
in place — see below — but the credit logic itself is a later phase.

---

## Where to go next

- Try it: [Run a local chain](local-devnet.md).
- See the strongest idea in practice: [Set up a treasury](treasury.md).
- Feed the chain prices: [Price feeds and asset valuations](oracle.md).
- Look something up: the [reference](../reference/) covers every message,
  query, parameter and error code, generated from the source.
