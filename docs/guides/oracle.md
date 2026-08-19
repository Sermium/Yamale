# Price feeds and asset valuations

How Yamale answers "what is this worth" — for currencies, which are priced by
validator vote, and for real-world assets, which are valued by an appointed
independent party.

**You need:** a running chain. [Run a local chain](local-devnet.md) gets you one
in about two minutes. Every command below was run against a node built from this
repository.

**You will end with:** an agreed exchange rate on the chain, a hot key voting on
your validator's behalf, and an appraiser application awaiting governance.

---

## Why two mechanisms

Nobody knows the price of a currency on their own, so validators each report what
they observe and the chain takes the median weighted by stake. Moving that median
requires controlling half the stake — the same thing that securing the chain
requires — which is what stops a minority from setting the price.

Nobody discovers the price of a building by polling exchanges. An appointed valuer
inspects it and signs a number. There is no median to take and no crowd to appeal
to; the value is exactly as good as the named party who signed it, so the chain's
job is to record precisely who that was, when, and against what report.

Both answer through one interface, because a lending module asking "what is this
collateral worth" should not have to care which kind it is. And both obey the rule
that matters most: **a value too old to trust is not a value.**

---

## 1. Look at what the chain expects

```bash
blockchaind query oracle params
```

```yaml
params:
  accepted_denoms:
  - uyml
  - uusd
  - uchf
  - ueur
  - ugbp
  - ujpy
  max_appraisal_age_seconds: "8640000"
  max_class_ids_per_appraiser: "50"
  max_rate_age_seconds: "900"
  quote_symbol: USD
  vote_period: "12"
  vote_threshold_bps: "5000"
```

`vote_period: 12` means a rate is agreed every twelve blocks — about once a
minute at five-second blocks. `vote_threshold_bps: 5000` means half the stake has
to report or no rate is agreed at all.

The two ages differ by four orders of magnitude on purpose. `max_rate_age_seconds`
is fifteen minutes: long enough to survive a validator restart, short enough that
a feed which has genuinely stopped is refused before anyone lends against it.
`max_appraisal_age_seconds` is a hundred days, covering a quarterly valuation
cycle with room for the report to arrive.

Before any votes, asking for a price is an explicit failure rather than a zero:

```bash
blockchaind query oracle rate uusd
```

```
rpc error: code = NotFound desc = no rate has ever been agreed for uusd
```

That is different from a rate that exists and has gone stale, and the difference
matters: the first is a configuration gap, the second is an emergency.

## 2. Vote

Every bonded validator may report. By default it votes with its own account —
the same bytes as the operator address, in account form:

```bash
blockchaind query oracle feeder $(blockchaind keys show alice --bech val -a --keyring-backend test)
```

```yaml
feeder: yml16v5yy9dgnw9h3g85pgvcw3d2ylg86gndadv7c5
```

Submit prices. Each `--rates` is one denom, repeated as many times as you have
prices; the rate prices one **display** unit — 1 YML, not 1 uyml:

```bash
blockchaind tx oracle submit-rates $(blockchaind keys show alice --bech val -a --keyring-backend test) \
  --rates '{"denom":"uusd","rate":"1.00"}' \
  --rates '{"denom":"uyml","rate":"0.4213"}' \
  --from alice --chain-id oracle-devnet-1 --keyring-backend test --fees 500uyml --yes
```

> `--rates` takes one JSON object per occurrence, not a JSON array. An array is
> rejected with `proto: syntax error (line 1:1): unexpected token [`.

As always, the `code` printed by the broadcast only means the node accepted the
transaction for inclusion. Check what it actually did:

```bash
blockchaind query tx <txhash>
```

Nothing is agreed yet. Votes sit until the round closes, so no one can see the
others' prices before choosing their own. At the next height divisible by
`vote_period`, the tally runs:

```bash
blockchaind query oracle rates
```

```yaml
rates:
- denom: uyml
  rate: "0.421300000000000000"
  updated_at: "1786378422"
  updated_height: "24"
  voting_power_bps: "10000"
- denom: uusd
  rate: "1.000000000000000000"
  updated_at: "1786378422"
  updated_height: "24"
  voting_power_bps: "10000"
```

`voting_power_bps: 10000` is the share of stake that contributed, in basis
points — here a single-validator devnet, so all of it. A rate agreed by a bare
quorum deserves less trust than one agreed by everybody, and this is how a
consumer tells them apart.

Asking for one denom also reports its age:

```bash
blockchaind query oracle rate uusd
```

```yaml
age_seconds: "16"
rate:
  denom: uusd
  rate: "1.000000000000000000"
  updated_at: "1786378422"
  updated_height: "24"
  voting_power_bps: "10000"
```

Once `age_seconds` passes `max_rate_age_seconds`, the rate is still returned —
hiding it would leave a caller unable to tell "no feed" from "feed stopped" — but
it is flagged stale, and the lending paths refuse to act on it.

**Correcting a vote.** Submitting again in the same round replaces the earlier
report rather than counting twice, so a feeder that spots its own mistake can fix
it before the tally.

**Absence is recorded.**

```bash
blockchaind query oracle misses
```

```yaml
counters:
- misses: "1"
  validator: ymlvaloper16v5yy9dgnw9h3g85pgvcw3d2ylg86gndmhl9ml
  windows: "2"
```

Two rounds have closed and one of them had no vote. Nothing is slashed for this.
On a small permissioned network an automatic penalty mostly punishes the operator
whose machine rebooted; governance can add one later using exactly this record,
and cannot easily undo one that fired wrongly.

## 3. Move voting to a hot key

Voting every minute with the operator key means keeping online the one key whose
compromise costs the most. Delegate it:

```bash
blockchaind tx oracle delegate-feeder \
  $(blockchaind keys show alice --bech val -a --keyring-backend test) \
  $(blockchaind keys show feeder -a --keyring-backend test) \
  --from alice --chain-id oracle-devnet-1 --keyring-backend test --fees 500uyml --yes
```

The delegate can only vote. It cannot move stake, change commission, or do
anything else on the validator's behalf. And the change is one-directional: only
the operator account may sign this message, so a compromised feeder cannot
nominate its own successor to keep itself alive.

From here the operator key is refused:

```bash
blockchaind query tx <txhash-of-a-vote-signed-by-alice>
```

```
code: 1102
raw_log: 'failed to execute message; message index: 0: ... may not vote for ...
```

and the hot key works:

```bash
blockchaind tx oracle submit-rates $(blockchaind keys show alice --bech val -a --keyring-backend test) \
  --rates '{"denom":"uusd","rate":"1.05"}' \
  --from feeder --chain-id oracle-devnet-1 --keyring-backend test --fees 500uyml --yes
```

To take voting back, delegate to the operator account again — that removes the
delegation rather than storing a self-delegation, so what is on the chain means
exactly one thing: a hot key is in use.

**Running this for real.** A feeder is a small daemon: read prices from wherever
you source them, submit once per vote period, and alert on your own miss counter.
The chain does not ship one. What it does guarantee is that the rules a client
applies to decide whether to show a price are the same rules the chain applies to
decide whether to lend against it.

## 4. Have a real-world asset valued

An asset is represented by an NFT — one token per asset, because two invoices
from the same issuer are not interchangeable and a fungible price for them would
be a fiction. The valuation is bound to that token.

Applying is open to anyone and grants nothing:

```bash
blockchaind tx oracle apply-appraiser \
  "Alpine Valuation SA" \
  "RICS 12345 / FINMA-registered" \
  realestate \
  --from valuer --chain-id oracle-devnet-1 --keyring-backend test --fees 500uyml --yes
```

```bash
blockchaind query oracle appraiser $(blockchaind keys show valuer -a --keyring-backend test)
```

```yaml
appraiser:
  address: yml1jk3vecg9un9yldj5x725n3s7d68tzaj7ctl5zg
  class_ids:
  - realestate
  credentials: RICS 12345 / FINMA-registered
  name: Alpine Valuation SA
  status: APPRAISER_STATUS_PENDING
```

The application exists so that what governance approved — the name, the
credentials it relied on, the scope asked for — sits on the chain next to the
decision, rather than in a forum post that can be edited after the vote.

Pending grants nothing. Trying to value something now fails:

```
code: 1105
raw_log: 'failed to execute message; message index: 0: ... is pending: address is not an approved appraiser
```

Admission is a governance decision (`MsgApproveAppraiser`, signed by the gov
module account), submitted as a proposal. Governance may narrow the scope the
applicant asked for without making them reapply; leaving `class_ids` empty on the
approval keeps what they requested, so approving cannot widen a scope by accident.

Once approved, a valuation is submitted against the asset's NFT:

```bash
blockchaind tx oracle submit-appraisal \
  realestate building-1 2500000000 uusd 1786300000 "RICS Red Book" \
  --report-uri "ipfs://QmExample" --report-hash "9f2b..." \
  --from valuer --chain-id oracle-devnet-1 --keyring-backend test --fees 500uyml --yes
```

Four things about that command are the point of the module:

- **The date is the valuation date, not today.** A monthly NAV published late is
  still a month-end number, and staleness is measured from when the valuation
  describes the world — not from when it was typed in.
- **The report is pinned.** `--report-uri` and `--report-hash` mean the on-chain
  number and the off-chain document cannot drift apart unnoticed.
- **The asset must exist.** A valuation of a token that was never minted is
  refused, because a lending module reading it would extend credit against
  nothing.
- **Scope is enforced.** A valuer admitted for property cannot value invoices.

Revaluing supersedes rather than overwrites:

```bash
blockchaind query oracle appraisal realestate building-1
blockchaind query oracle appraisal-history realestate building-1
```

The current query also reports `appraiser_still_approved`. If a valuer's
authority is later withdrawn, their existing valuations stay exactly as they are —
they were validly signed when they were made, and deleting them would rewrite the
record rather than correct it — but a consumer can see that the chain no longer
stands behind the signer.

---

## Things worth knowing

**A round that was open when the chain stopped is abandoned.** Votes are not
carried in an exported genesis: they describe a moment that has passed, and
resuming them would agree a price on reports nobody currently stands behind. The
next round starts empty.

**A stale price is returned, not hidden.** Queries always answer with the value
plus its age and a staleness flag. The paths that act immediately on a
price — lending, liquidation — use the stricter form that refuses instead, because
there the safe failure is to stop.

**No commit-reveal.** Votes are visible in the mempool, so a validator could in
principle copy another's report instead of observing prices itself. On a
permissioned network with a known, accountable validator set the cost of that is
social rather than economic, and the two-phase alternative doubles the traffic and
adds a whole class of "revealed late" failures. Both this and the absence of a
miss penalty are parameters governance can revisit; neither is baked in.

**Full reference:** [x/oracle](../reference/oracle.md) — every message, query,
parameter and error code, generated from the source.
