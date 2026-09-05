# What is left

Checked against the tree and the running chain rather than against a task list —
a task here was once marked complete while its artefact did not exist, so the
list is not evidence.

**Chain state** last verified 2026-09-05, against `yamale-devnet-2` at block
196,559, queried over `https://yamale.tail4355e8.ts.net/api/rpc/`. That sweep
covered the audit rows below and nothing else, so **a figure here carries the
date it was read** — most of the rest were read on 2026-09-01 at block 141,500
and say so. A figure with no date is older than both and should be re-read
before it is relied on.

**Repository state** last verified 2026-09-05, which is a weaker claim and is
labelled separately on purpose: it means the code exists and its tests pass, not
that anything has run on the network. The audit-remediation rows and the
account-service rows are both of this kind.

**The hostname in that sentence is itself a correction.** Every previous
verification queried `pay.yamalelegal.com`, which is the VM — and the VM is not
the host the public reaches. The funnel terminates on the Pi. So a document
written to stop claims being made about the tree instead of the chain was, for
the client half of its rows, checking the wrong machine. See
[deploy/README.md](../../deploy/README.md).

**Three states, not one.** This document once headed a single table "built,
merged, and live on the chain", and that phrasing hid a real gap for two days:
rows that were merged sat beside rows that were running, and a row reading "land
registry - module, CLI, client" concealed that `x/tokenisation` had no CLI at all
and the land client could send two of twelve messages. So the states are now
separated, and a row may only move down the page when somebody has checked the
running chain rather than the tree.

---

## Live on the chain, and verified there

| | |
|---|---|
| Constitutional layer | 13 invariants fixed at genesis; `MsgUpdateParams` refuses them |
| Concentration caps | beneficial-ownership registry, epoch enforcement, demotion by jailing |
| Enforcement oversight | legal instrument, delays scaling with amount, ombudsman veto, rolling window cap |
| Foundation | 3-of-5 `x/group`, in genesis, as `recovery_destination` |
| Key ceremony | air-gapped CLI, loopback page, and hosted multi-device with in-browser keygen |
| Payment confidentiality — metadata | salted hash on-chain, payload off-chain |
| Payment confidentiality — schema | fields 10–13 reserved; commitment refused until verification ships |
| Encrypted payload store | X25519 + ChaCha20-Poly1305, three recipients, erasure demonstrated |
| Jurisdictional registry | country on the account, country prefix in the `x/alias` identifier |
| Build profiles | `settlement` compiles out the token and five modules; IBC opt-in |
| Fees in issued currency | ante gate on denom, swept to a treasury operating account |
| Validator key rotation | planned rotation, recovery quorum, veto-by-signing |
| Land registry - the module | 12 messages, four-party transfer, 31 tests; `x/tokenisation` refuses unauthorised fractionalisation |
| Tiered netting | `x/netting`: collateral posted first, hold-and-retry, no recompute path |
| Foundation console | `/foundation/` — the 3-of-5 has an interface, with the limits below |
| Roles and the perimeter | `x/alias` role grants, and `AssertScope` consulted by four modules |
| Coordinated upgrade | proposed, voted, halted at height, binaries swapped, applied — on the live chain |
| Signing-request decoding | the wallet reads a `TxBody` and says what it does, instead of naming a type URL |
| Visual system | `clients/shared/yamale.css` — real typefaces, a scale, elevation, semantic colour |
| All five roles confer something | `roles-that-do-something` applied at **95,400**; two chain-wide `ROLE_FOUNDATION_ADMINISTRATOR` grants exist, made at **95,758**, both pinned (3-of-5 and 3-of-4) |
| Threshold consumer accounts | `mpc` — 2-of-3 ECDSA. Two payments from one such account at **118,885** and **118,968**, the second after a reshare, under a byte-identical public key |
| The custodian service | `tools/custodian` — holds one share, authenticates, co-signs, refuses. Not deployed against anybody's money |
| `x/tokenisation` pays its shareholders | `income-that-arrives` applied at **119,900**; a vehicle minted after it owes 18 YML to its majority holder where the one minted before owes nobody anything against a 72 YML vault |
| Four more client surfaces | `/keys/`, `/markets/`, `/oversight/`, `/demo/` — all answering 200 on 2026-08-31 |
| The oracle agrees a price | first ever at **141,264** on 2026-09-01: 48 denominations, both validators reporting, `voting_power_bps` 10,000. Two feeders under systemd on two different sources |
| Structuring and stuck slices | `structuring-and-stuck-slices` applied at **132,700**, identical app hashes on both hosts. Both new parameters default to zero, so nothing changes until governance sets them |

## Merged, but not yet running

The distinction this document previously lost. Everything here is in `main` and
has never been exercised against the network the validators are running, so a
query against the live chain returns nothing for any of it.

| | | |
|---|---|---|
| Country enrolment | `ceremony country` - offices, grants, jurisdictions, approved by the foundation | tool complete, **never run**: `RoleHolders` is empty for every country asked |
| Enrolment for a threshold account | `tools/custodian --import` takes a share file an operator produced with `tools/mpc keygen` | there is no path by which a member of the public gets an account |
| Distributed key generation | `mpc.KeygenParty` — one participant per process, share computed locally and never transmitted | **closed 2026-09-03.** A test captures every byte the three parties exchange and asserts no party's secret scalar appears in it |
| Enrolment over HTTP | `tools/custodian` `/v1/enrol/*`, driven by the device against two deployments (`--role custodian` and `--role recovery`) | built and tested; never run against a real browser or a deployed pair |
| The recovery process of Part 5 | two approvers from different teams, 72-hour delay, notice that aborts on failure, 24-hour outbound freeze enforced on every signature, recorded proof, aggregate publication | built and tested; never exercised by a real operator |

**Three rows left this table on 2026-08-31, and one of them had been wrong for
four days.** `roles-that-do-something` was applied at height **95,400** — all
five roles confer something, the retired parameter lists are grants, and
`required_shape` is checked on every authority action. Administrator appointment
was listed as "tool complete, never run" and had in fact been run:
`ChainWideGrants` returns two `ROLE_FOUNDATION_ADMINISTRATOR` grants made at
height **95,758**, both to `x/group` accounts and both pinned, at 3-of-5 and
3-of-4. `x/tokenisation`'s CLI rides the same binary and is therefore live too.

That is the same failure this document was restructured to prevent, in the
opposite direction: a row stayed pessimistic because nobody re-read the chain
after the upgrade landed. **Reading the chain has to happen in both directions.**

**How this was found, and it is the reason for the split.** On 2026-08-27 the
running binary on both hosts was four days old. `query alias chain-wide-grants`
returned `{}`, so no account held `ROLE_FOUNDATION_ADMINISTRATOR` - not because a
grant was missing but because the state machine had no such role. The symptom
surfaced two modules away: an account created in the payments app could send
money and could never be addressed, because `MsgRegisterAlias` needs a recorded
jurisdiction and only an approved participant or a foundation administrator can
record one. **A binary's build date is now part of verifying this document.**

## The account model - the key is built, the service around it is not

Separated out because it keeps being asked about. This section previously read
"decided, specified, and the largest unbuilt thing", and the first half of that
stopped being true on 2026-08-31.

**Decided 2026-08-20: threshold key custody, built in house rather than bought.**
The design is complete in [accounts.md](../guides/accounts.md): the key is split,
the server holds one share and the device the other, and neither can sign alone -
so "the operator cannot move your money" is a statement about mathematics rather
than about policy. Operator custody was chosen first and reversed, because on a
state-operated system "the authority can spend any citizen's balance" is a very
different political object from "the authority runs the payment rails". Recovery
is specified to the same depth: two approvers from different teams, a 72-hour
delay with notice to email and every enrolled device, outbound payments frozen
for 24 hours afterwards, proof that is not public knowledge, and recoveries
published in aggregate so an unusual rate is visible without exposing who.

**The share protocol is built, and it has signed on this chain.** `mpc` is
2-of-3 threshold ECDSA over secp256k1 on tss-lib: three shares named `device`,
`custodian` and `recovery`, any two of which sign, none of which signs alone. It
is documented in [mpc.md](../guides/mpc.md). `tools/custodian` holds the second
share and decides whether to co-sign — [custodian.md](../guides/custodian.md).
`mpc/wasm` is the device's half compiled for the browser, and `clients/keys` runs
the protocol in front of an audience at `/keys/`.

What was verified on the chain, and it is the claim the design was chosen for:

| | Height | Transaction | Memo |
|---|---|---|---|
| Before the reset | 118,885 | `A8F18CAB…B15C3C` | `threshold signed: device + custodian` |
| After it | 118,968 | `6C784D06…4AFCFE` | `after a password reset: new shares, same address` |

Both `code 0`, both `MsgSend` from
`yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97`, sequences 0 and 1 — and both
carrying the **byte-identical** compressed public key
`0320ddc3…3c34`, which derives to that address. The second was signed by shares
that did not exist when the first was signed. A Cosmos multisig could not have
done that: rotating a member changes the address, which retires the account's
`x/alias` identifier and silently breaks every saved payee.

Two honest limits on that. The chain sees an ordinary single-signature account
and cannot tell you the signature was produced jointly — that
indistinguishability is deliberate, and it is also why the transaction alone is
not proof of the arrangement. And the shares in that rehearsal were three files
on one machine driven by `tools/mpc`, which is precisely the arrangement the
design exists to avoid; the CLI's own header says so.

**Most of the service now exists, built 2026-09-03.** Enrolment over HTTP,
distributed key generation with no process ever holding two shares, a
pre-parameter pool, and the whole of Part 5 — two approvers from different teams,
the 72-hour delay, notice that aborts the recovery if it fails, the 24-hour
outbound freeze enforced on every signature, a recorded standard of proof, and
publication in aggregate. Thirty-three tests in `tools/custodian`, five more in
`mpc`.

Enrolment runs `mpc.KeygenParty` in three places at once: the device in the
browser, and two independent deployments of the same binary as `custodian` and
`recovery`, with separate sealing keys and separate directories. The store is
constructed for exactly one role and refuses to write or hand back any other, so
"no single service can sign" is enforced rather than configured.

**What is still missing is smaller but not small.** No identity check at
enrolment — the service can refuse a duplicate email and verify that everybody
generated the same key, and it cannot verify who anybody is; that belongs to
enrolment policy and is the first thing a deployment must answer. No second
factor. No notice to enrolled devices, only to one email, and device enrolment
does not exist. No rate limit or lockout on the password check. And the reshare
after a recovery is deliberately left to the customer's device, which means a
recovery is not finished when the endpoint says completed.

**None of it has run outside a test.** No browser has driven an enrolment, no
operator has approved a recovery, and the two deployments have never been stood
up as two deployments. That is the next thing, and it is the same distinction
this document exists for: built is not running.

**And the consumer app still ships the model the design rejected.** `clients/app`
holds a CosmJS password-wrapped key in `localStorage`.
`clients/app/src/account.ts` says so in its own header and calls itself not
acceptable in production, which is the right way to carry a proof of concept —
but it means a forgotten password is a lost account today, and the threshold work
above is not wired into any interface a person could use. **That wiring is now
the gap**: the service it would talk to exists, and nothing talks to it.

Two divergences worth recording rather than leaving only in the code:

- **The blind index in `clients/app` was not one. Fixed 2026-09-01.** It was a
  bare `SHA-256` of the email, which can be tested against any word list, so a
  dump yielded exactly the membership the hash was meant to hide. There is no
  server here to hold a pepper, so the key is now the user's own password through
  PBKDF2, salted by the email: a dump without it yields nothing testable. The
  cost is stated rather than hidden — an account can no longer be found by email
  alone, so a wrong password and an unknown email are now the same outcome.

  The larger hole was beside it and is the part worth remembering: the plain
  email was stored under a fixed key to remember who signed in last, so one dump
  answered "who uses this" **without touching the index at all**. The index was
  decorative however it was computed. That record is gone; a masked hint was
  considered and rejected, because on a national payments system the domain is
  most of the answer.
- **`crypto.subtle` is undefined outside a secure context**, so on the plain-HTTP
  tailnet host account creation throws and reaches the user as "That password is
  not right."

## Designed, documented, not built

**Browser signing for the foundation.** The console at `/foundation/` reads the
chain and composes commands a custodian runs; it still cannot sign.

**Its stated prerequisite has since shipped.** The wallet no longer stops at a
message's type URL — `clients/sdk/src/signrequest.ts` decodes a `TxBody` through
the generated encoders and describes what it does, following nested `Any`
payloads so a proposal describes the act rather than the envelope, and refusing
to describe a type it does not recognise. So the argument against in-browser
signing is now weaker than the argument for it, and the three actions a custodian
signs personally — `MsgVote`, `MsgExec`, `MsgSubmitProposal` — are the ones to
revisit first.

What has not changed is why it matters: on the account that receives every
seizure, a page that said "pay 5,000 to Amara" while the wallet said
`MsgSubmitProposal` would be worse than a command line.

**Confidential amounts.** Deliberately deferred, with the measurements recorded
in [confidentiality.md](confidentiality.md): no audited Go library, ~6
transactions per second with the only credible Bulletproofs implementation
(which is itself broken on three of five curves), ~130 with gnark. The reserved
field numbers are the part that mattered and they are done.

**The seat token for a no-native-token profile.** `-tags settlement` removes the
token's *issuance*, not its *denomination*: `sdk.DefaultBondDenom` is still
`uyml`, and staking, gov deposits, slashing and the oracle's accepted denoms all
follow it. Equal seats needs a non-inflating, non-transferable seat token with
equal genesis balances, delegation blocked and slash fractions at zero.

## Untouched

**Scope §6, workstream three.** Batch payment messages with per-item failure
isolation, mempool lanes prioritising settlement, a published state-growth model
with pruning and a tested restore, an institution registry carrying LEI or BIC,
oracle hardening with deviation circuit breakers.

**Scope §6, workstream four — assurance, which gates everything commercial.**
Two independent audits with non-overlapping scopes, property-based and fuzz
testing on every value-moving path, written invariant specifications produced
*before* audit so reviewers have something to check against, a key ceremony with
HSM custody, a documented upgrade rollback.

**The account service** has its own section above and is mostly no longer
untouched: enrolment, distributed key generation and the recovery workflow were
built on 2026-09-03. What remains untouched around it is **second-factor
enrolment**, **rate limiting or lockout on the custodian's password check**, an
**identity check at enrolment**, and any **client** that speaks to it.

Plus USSD and feature-phone access, without which most African transaction
volume is unreachable, and agent-network and mobile-money integration. Those are
still entirely absent and are the larger commercial gap now.

**A legal entity able to sign an indemnity.** A `LICENSE` now exists —
proprietary, with reading, compiling, running on a test network and publishing
security findings explicitly permitted, and production or distribution requiring
a written agreement. That removes the "no licence at all" blocker that stopped a
counterparty's legal team from opening the repository, and it is not the same
thing as being able to contract. The file itself flags what is unsettled: the
registered entity, named "Sermium" on the basis of the repository's own naming,
and the governing law, which it deliberately does not assert. It was drafted for
clarity rather than by a lawyer and has not been reviewed by one.

## Open decisions

**Answered 2026-08-20.** Threshold key custody is **built in house**. The product
is a **vendor with support obligations**, not a reference implementation — which
makes the §6 assurance workstream, the LTS branches, the backport policy and a
legal entity able to sign a warranty into owned scope rather than somebody
else's problem. **Cross-chain collateral is not a requirement**, so IBC stays
compiled out and outside audit scope.

**Still open, and the one with the shortest fuse:**

1. **Which beachhead** — UEMOA, or an Afreximbank partnership. The two imply
   different first roadmaps. The recommendation on file is Afreximbank first: a
   vendor sells to a buyer, and one counterparty answers faster than eight
   governments whether anybody buys this at all.

2. **What happens to an open netting window when netting is switched off.**
   Setting `cycle_blocks = 0` returns before closing anything, so the open
   window never settles, held slices stop being retried, and every participant
   in it has an exposure with no settlement date until a second proposal passes.
   The choice is between refusing the proposal and closing the open window
   immediately; both are defensible and it is a settlement-policy call, not a
   coding one. Documented in `params.proto` and
   [settlement.md](../guides/settlement.md), deliberately not decided.

**Answered 2026-08-23.** **Every role is granted by the foundation**, and
chain-wide `*` scope remains governance's alone. A country authority may *not*
grant roles inside its own country. That is what the code already enforces, so
nothing changed — but it was a live question and the reasoning is worth keeping:
delegating national appointment to a national office means one compromised
office yields every power in that country, because the payments authority could
appoint the enforcement authority that freezes accounts. The cost accepted in
exchange is that three custodians sign every national appointment, which is a
bottleneck and a rubber-stamping risk. Revisit by giving offices the power to
*propose* rather than to grant, if the bottleneck turns out to be drafting
rather than approval.

**Answered 2026-08-25.** **The retired emergency authority is carried across
chain-wide, and that widens it.** `emergency_authority` was one address with no
territorial limit, so the `roles-that-do-something` upgrade grants the address it
held `ROLE_ENFORCEMENT_AUTHORITY` at `*`. Granting it a country instead would
have been the upgrade choosing to narrow an authority nobody voted to narrow, and
choosing which country on top of it; granting nothing would have removed the
emergency path from a running chain with no one noticing until it was needed.

What the chain-wide grant adds, stated because collapsing two mechanisms into one
always adds something: that account can now open an ordinary case as well,
including a seizure accusation, which `emergency_authority` could not do. It
still cannot decide one — a seizure needs two thirds of bonded voting power, and
this account has no vote unless it is also a validator. The first thing a
deployment should do after the upgrade is revoke the chain-wide grant and issue
country ones, which is one foundation proposal per country.

3. **Whether a bonded validator should still be able to freeze anything.**
   `AssertScope` gates `x/enforcement`'s `OpenCase`, which changes the module's
   central property from *any bonded validator can freeze* to *any bonded
   validator governance has placed in that country's perimeter*. That is what
   [roles-and-perimeter.md](roles-and-perimeter.md) asks for and it is the point
   of having a perimeter — but it is a real narrowing of who can act in an
   emergency, and the narrower alternative was to scope only `EmergencyFreeze`
   and leave ordinary validators chain-wide. Worth confirming deliberately
   rather than discovering.

   **Still open, and less pressing than it was.** `OpenCase` now also accepts a
   holder of `ROLE_ENFORCEMENT_AUTHORITY`, so a country that has enrolled an
   enforcement office is no longer relying on a validator being both awake and
   granted. The question the decision turns on is unchanged: who can stop money
   in a country that has NOT enrolled one.

4. **Whether the count of chain-wide `*` grants belongs in the constitution.**
   A validator set wanting to act outside its perimeter would grant itself `*`,
   and today an ordinary governance vote is all that stands in the way — which
   is exactly the test for constitutional-ness. The argument against is that it
   is a count rather than a threshold, and `x/alias` has no `EndBlocker`, so it
   would be checked in `GrantRole` and `InitGenesis` the way the
   foundation-administrator cap already is. Not added, because the invariant set
   is customer-visible.

**Answered 2026-08-23.** **An office's shape is pinned on its grant and checked
on every action**, not stamped once at grant time. `assertGroupAccount` refused a
role holder that was not an `x/group` policy and asked nothing about the
arrangement inside, so a one-of-one satisfied it and a proper three-of-five could
vote itself down to one afterwards with nothing notified. `RoleGrant` now carries
an optional `required_shape` and both perimeter functions refuse an office that
has fallen below it. Two decisions inside that are worth recording because they
could reasonably have gone the other way:

- **Absent means no requirement**, so grants made before the field existed are
  unchanged in effect and their holders can still shrink to a single key. The
  alternative — treating absence as "must be at least something" — would disable
  every existing authority on the upgrade block. Closing it is one foundation
  proposal per grant, and the runbook says how.
- **A missing `x/group` keeper refuses**, unlike `assertGroupAccount`, which
  skips. The asymmetry is which way the bypass runs: there a missing keeper can
  only produce a grant the perimeter will refuse to act on, here it would produce
  an action by an office whose shape nobody read.

5. **Whether a netting reserve is seizable.** `x/enforcement` seizes
   `SpendableCoins` from a bank account; the uncommitted part of a posted
   reserve is plainly the participant's own money and sits in the netting module
   account, out of reach. A freeze blocks posting and withdrawing, so value
   cannot escape — but a seizure cannot reach it either.

## The devnet-2 audit, and what it left for the chain

An independent security and functional audit was performed against
`yamale-devnet-2` on **2026-09-03**, at blocks 170,022-170,225 and repository
commit `dd23f06`. Read-only throughout. Its own summary is the fair one:

> Unusually disciplined module code, running on a network whose configuration
> undoes most of what that code protects.

3 critical, 6 high, 11 medium, 8 low, 7 functional. The code findings were
closed on **2026-09-05** and are listed under "Known defects" below. This
section is what the audit found that **code cannot close** — every row here
needs a transaction, a vote or a key ceremony, and none of it has been done.

**Verified still live on 2026-09-05** against the funnel at block 196,559.

### The one that makes the rest of the list conditional

**887,940,502.67 YML is claimable by one operator with a single
`MsgWithdrawDelegatorReward`.** Bonded stake is 174,900 YML, so that is 5,077
times the entire bonded total, and a `MsgDelegate` of any fraction of it puts
one account above 99.9% of voting power in one block. That defeats governance
quorum (33.4%), the enforcement supermajority (66.67%) and the constitutional
amendment threshold (80%). Every one of the eight proposals this chain has
passed carried 65,000 YML of yes votes and nothing else.

This is not a code defect. Every module correctly asks whether the signer holds
two thirds of power; the answer is purchasable because power is measured against
a bonded total four orders of magnitude smaller than the float.

Where it came from: `genesis_provisions_per_block` is 3,333,333,333,333 uyml —
3.33 million YML a block — decaying by a third every 100 blocks. The schedule
minted 997.8 million YML in roughly the first four thousand blocks and then
truncated to zero. `current_provisions_per_block` reads `0` today, so **the
exposure is fixed rather than growing**, which is the only good news in this
row: supply and outstanding rewards have not moved since the audit read them.

What has to happen, and in this order:

1. Withdraw the outstanding rewards and either burn them or bond them, so the
   bonded total is a meaningful fraction of the float. Nothing else in this
   list matters while every threshold is purchasable.
2. Set `min_commission_rate` and revisit the emission schedule before any
   future chain is launched from this genesis.
3. Separate the governance franchise from raw bonded stake. On a permissioned
   validator set, one-seat-one-vote through x/group — or an equal-power
   invariant — is the model the rest of this design already assumes.

Most of the second validator's rewards accrue to the `validatorgov` module
account, which delegates the seat bonds and has no key to sign a withdrawal.
Those are permanently unreachable and should be counted as burned.

### Configuration, measured in minutes

- **The oracle feeder key is the admin of a funded treasury.**
  `yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas` is both the delegated feeder for
  `ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j` and the admin of treasury
  2, "Lagos Field Operations" — 900 NGN, 650 XOF, 700 YML and a 400 YML vesting
  lock. The shipped unit file said `FEEDER_KEYRING=test`, an unencrypted keyring
  on the validator host, and the trust model written beside it says a
  compromised feeder "cannot touch the stake". It cannot. It can empty a
  treasury: the 50 YML per-spend cap is not a bound against the admin, because
  `SetSpendPolicy` is admin-only. **The example file is fixed; the treasury
  admin is not.** Rotate it to a separate key, ideally an x/group account.

- **The ombudsman is unset**, so `MsgOmbudsmanVeto` — the only message that can
  stop a seizure that has passed and is waiting out its delay — has no holder.
  What remains is a 79-minute or 5-hour delay whose only recourse is a
  governance `MsgReverseCase`, which on this chain is the same voter that could
  have passed the seizure.

- **The rolling seizure cap covers one denomination out of forty-eight.**
  `seizure_window_cap` lists only `uyml` at 500 YML. A seizure of any of the 43
  fiat currencies, or of an `amm/pool/*` or `tok/*` denom, is bounded only by
  `max_seizures_per_window`: 5 per 31.6 hours, at any size. The code comment
  anticipated exactly this; the configuration has not followed it.

- **No payment participant is approved**, so `MsgSendPayment` cannot succeed for
  anybody and `x/netting` is in the same state. The payments product the public
  hostname is named for has no live rail behind it.

- **Only CD has a national authority granted.** NG, ZA and CI are empty, and
  because `AssertScope` fails closed, every message that accepts "governance or
  a scoped authority" is governance-only outside CD — which on this chain means
  the single voter above.

- **The oracle's vote threshold is met by one validator alone.**
  `vote_threshold_bps` is 5,000 and the larger validator holds 5,717, so it
  satisfies the threshold by itself and its own rate is the weighted median. On
  a two-validator set the threshold has to exceed 5,717 for the median to mean
  anything.

- **The oracle has no on-chain consumer.** `types.ValueOf` and `types.IsStale`
  are called nowhere outside `x/oracle`. The rates are published and healthy;
  nothing on the chain prices anything against them, which is also why oracle
  manipulation is not rated higher.

- **Both validators must be online for the chain to produce blocks**, and the
  larger one alone can halt it. The public hostname rests on one consumer
  tunnel and two hosts that must both stay up. See
  [three-validator-launch-facts].

### The one that cannot be done quickly

**The concentration ceilings are 100% and cover no validator.** All three
invariants read 10,000 basis points, and `Invariants.Validate()` permits that
while refusing zero — so a genesis can declare ceilings that bind nobody and
still pass validation. Even a real ceiling would apply to nobody:
`activeSeatHolders` needs an `ApprovedValidator` declaration and the live query
returns an empty set, because both validators were onboarded through the gentx
ceremony. The founding set was never declared, which is what the code comment
says the remedy is. The same emptiness disarms `assertWithinCaps`, the
precondition x/enforcement runs before every freeze.

Because these are constitutional invariants rather than params, correcting them
needs a proposed amendment, 80% ratification and a 120,960-block delay: **9.2
days minimum, whenever it starts**. `Invariants.Validate()` should also refuse a
ceiling of exactly `BasisPoints` with the same directness it refuses zero — a
ceiling that binds nobody is as much a contradiction as one that binds
everybody. That part is a code change and is **not** done, because it belongs in
the same amendment as the figures.

### The gate that stops at the wall

`abci_query` on `/api/rpc/` reaches every registered query service, so the REST
allow-list is a lock on a door in a wall that stops there. The decision taken on
2026-09-05 was to **open `x/land` and `paymsg` params on REST and say so**,
since the data was already reachable — see `deploy/nginx/yamale-visibility.conf`.

**Payment records and `x/netting` stay closed, and closing them there does not
close them.** The deny list matches on the URL path while the JSON-RPC form
carries the method in the POST body, so a path regex cannot fix it: the real
options are `njs` body inspection with an allow-list of `abci_query` paths, or
moving the client reads to REST. `deploy.sh` now probes the bypass on every
deploy and prints a warning while it stands. It costs nothing today because no
payment has ever been made on this chain. It must be closed before one is.

### Two things the audit did not cover

- `tools/custodian`, `tools/mpc`, the feeder and the payload store were read
  only where they touch the chain. **The threshold-signature implementation in
  `mpc/` warrants its own cryptographic review** and was deliberately left
  alone, being under active modification at the time.
- The in-browser key-generation page at `/keys/` deserves a dedicated review
  given what it handles — and it is served with no Content-Security-Policy. See
  the security-headers row under "Operational loose ends".

---

## Known defects

### Closed on 2026-09-05, from the devnet-2 audit

All in the tree, none of them yet running on the chain — the same distinction
the paragraph below draws, and the reason it is drawn. Every one has a
regression test that fails on the code as it was.

- **`x/treasury`: an escrow and a lock could be issued the same id.**
  `OpenEscrow` took the next sequence value and then incremented it while
  `CreateLock` did not, so the next ordinary lock created on the chain took the
  escrow's id and overwrote the record — leaving the depositor's money in the
  module account with no owner and no handler that would release it. The
  increment is gone; `InitGenesis` has always seeded the sequence at one, which
  is where a guard against id zero belongs. The keeper fixture went through
  `Params.Set` alone, so it ran against a state no chain is ever in; it now
  initialises genesis, which is what exposed where the guard was living.

- **`x/tokenisation`: a vehicle that funded nothing could drain another's
  vault.** `FinaliseSale` credited the whole reported sale price to the index
  while moving no coins, and one module account serves every vehicle. There is
  now a per-vehicle ledger of what the module holds on whose behalf: nothing
  reaches an index the vehicle does not already hold, and no payout exceeds it.
  Proceeds arrive through `MsgPaySaleProceeds`, permissionless so a sponsor who
  goes quiet cannot strand the holders. The repository already half-knew — the
  exit test called it "the proceeds hole" and asserted the broken behaviour so
  that whoever fixed it would have to look.

- **`x/tokenisation`: the sponsor's share was taken and never paid back.**
  `FundVault` transferred the whole payment into the module account and credited
  only `holder_share_bps` to the index. On a vehicle with a 2,500 bps share
  three quarters of every rent payment was stranded. It now collects the
  holders' share and leaves the rest where the message always said it stayed.

- **`x/tokenisation`: a dispute bond had no way out.** It went into the shared
  module account and no message returned it or awarded it. It now goes back to
  the challenger when the price is corrected and forfeits whole to the holders
  when it is not — whole, because a penalty for delaying the holders is not
  income from the asset and the sponsor has no claim on it. Not in the audit;
  found while building the ledger above.

- **`x/stablecoin` and `x/builderfee`: a rejected application killed its key
  forever.** Both registrations are permissionless and keyed by a permanent
  identifier, both refuse a second application for a key that already has one,
  and neither had a withdrawal, expiry or clearing path — so one transaction fee
  bought the right to stop anybody ever registering `uusd`, or ever claiming the
  builder fee on a message type. Rejection now removes the record.
  `msg_type_url` was additionally unbounded and unvalidated, making it an
  arbitrary-length attacker-chosen store key; it is now bounded in shape and
  length and must name a message this chain can route.

- **`x/stablecoin`: mint was unbounded and an issuer could not be removed.**
  Being the recorded issuer was the whole authorisation — no cap, no period
  limit, no reserve check — on a chain where one key is the approved issuer for
  all 43 currencies. Minting is now bounded by a supply ceiling in params, and
  an unset ceiling means **no minting** rather than unlimited minting, so a
  chain upgraded past this waits for governance to state a figure. And
  `MsgRevokeIssuer` did not exist: `ApproveIssuer` refuses an application that
  is no longer pending, so a compromised issuer key could not be answered
  without a chain upgrade.

- **`x/custody`: two attestors to credit a deposit, one to settle a redemption
  or to state the reserve.** Settlement now takes the threshold, with the first
  attestor's `settled_ref` standing as a proposal a second must confirm rather
  than contradict. The published reserve is derived from per-attestor reports —
  enough fresh ones, and the lowest counts, because a reserve moves and a rule
  demanding exact agreement would deadlock the module rather than secure it. And
  `credit` now checks issuance against that figure, which nothing did:
  `solvencyOf` was called only from the query server, so the chain would mint
  past its reserves and report the shortfall in a number nobody was obliged to
  read.

- **`x/custody`: fee revenue was unrecoverable.** `credit` mints the full
  deposit and forwards the net, deliberately — the claim outstanding has to
  equal the reserve held. The fee therefore accumulated as claim tokens in a
  module account with no key and no message that spent them.
  `MsgWithdrawFees` pays them out as claims, so the custodian's own revenue
  leaves by the same door as everybody else's money.

- **`x/validatorgov`: anyone could block a governance decision by delegating.**
  `SetValidatorPower` compared the seat target against `validator.Tokens` —
  every delegation, not just the module reserve's — and delegation is
  permissionless. One `MsgDelegate` of any size made `releaseSeats` fail, and
  governance could not lower that validator's power until the stranger chose to
  unbond. The mirror was worse: after a power was raised against an inflated
  figure, the stranger undelegating dropped the validator silently below what it
  was granted. Seats are now measured against the reserve's own delegation. Note
  the consequence: a seat count is what governance has staked, so a validator's
  total power is its own bond plus its seats.

- **`x/validatorgov`: x/group execution bypassed the ante gate.** The decorator
  descends into `MsgExec`, which closes the authz route; a passed group proposal
  dispatches straight through the message router after the ante chain has run,
  and both chain-wide foundation administrators are x/group accounts. The gate
  is now also a staking hook, delivered as a `StakingHooksWrapper` because
  x/staking sets its hooks from a depinject invoker and calling `SetHooks`
  afterwards panics.

- **`x/enforcement`: a freeze did not follow the staking rewards.** A send
  restriction fires on the sender, and the sender on a reward payout is the
  distribution module account — so a frozen account could point its withdraw
  address at an unfrozen one and take out everything accruing during the freeze,
  which on this chain is the overwhelming majority of what an account controls.
  The freeze now resets the withdraw address, which is a state change rather
  than a gate: nothing routes around it, and it covers a redirect set long
  before the case was opened.

- **`x/emission`: the block reward was minted against a package global.**
  `sdk.DefaultBondDenom` is a variable `app/config.go`'s `init()` rewrites, so
  it was correct only while the app package happened to be linked in, and a
  governance change to `bond_denom` would have left emission minting the old
  denomination into the fee collector indefinitely. It asks the staking keeper
  now, as x/validatorgov and x/enforcement already did.

- **`x/paymsg`: a participant could claim any account as its customer.**
  Registration is signed by the participant alone and one participant per
  customer is enforced, so the first institution to claim an address owned the
  relationship: it could be named as instructing agent on that account's
  payments, and only it could let the account go. A claim is now only a claim —
  `MsgConfirmParticipant` is signed by the account, `assertInstructedBy`
  requires the confirmation, and refusing frees the account to bank elsewhere.

- **`x/netting`: the end blocker walked unbounded collections.** `retryHeldSlices`
  walked every held slice and, for each, every position in its cycle;
  `escalateOldHolds` walked all of `HeldSince`. Neither had a per-block bound,
  consensus `max_gas` is -1, and an error out of an end blocker halts the chain
  rather than failing a message. Both are now bounded at 256 a block and resume
  from a cursor — a bound without one would have retried the same first slices
  forever.

- **`x/amm`: three ways a pool ended up unusable.** A complete exit left
  reserves and shares at zero, and the next join divided by a zero reserve; a
  swap whose output truncated to zero took payment and returned nothing, settled
  rather than refused, on any call passing `min_amount_out` of zero; and the
  reserves were parsed with the error discarded, which yields a `math.Int` with
  a nil inner value that panics on first use. All three refused now.

- **`x/land`: transfer ids started at zero.** `RegisterParcel` carried an
  explicit guard and `ProposeTransfer` did not, so the first transfer on any
  chain was transfer 0 — and a client omitting `transfer_id` from a
  `MsgValidateTransfer` would have addressed it rather than being refused.

- **`x/custody`: a store error was read as not-found.** Branching on `err != nil`
  alone built a fresh pending deposit over whatever was really there, dropping
  the attestations already counted against it.

- **`x/treasury`: an escrow with a lost moderator had no exit.** The argument
  against a deadline stands — an automatic release rewards the seller who
  shipped nothing and waited — but a dispute opened against a moderator whose
  key is then lost could not be settled by any message. Governance can now
  decide a case somebody opened, and only such a case: a quiet lock is still
  nobody's but the depositor's.

- **`x/alias` and `x/custody` compared the governance authority as a string.**
  Bech32 is case-insensitive, so this refused a signer it should accept — the
  safe direction, and no exploit follows, which is a poor reason to keep
  comparing the wrong thing. Ten other modules decode and use `bytes.Equal`.

- **`x/amm` is permissionless on a chain where nothing else that moves value
  is.** Left open, and now argued for in the handler rather than left as an
  omission — see `CreatePool`. A deployment that needs the perimeter to cover
  pools will have to gate creation the way x/stablecoin gates minting.

---

**Closed in the tree is not closed on the chain.** The five `x/tokenisation`
defects struck through below were fixed on 2026-08-27 and were still being
executed in their broken form by the validators for four more days. They became
live with the `income-that-arrives` upgrade at height **119,900**, verified
against `AppliedPlan`. A row here now says both dates when they differ.

- ~~**No shareholder in x/tokenisation could ever be paid anything.**~~ Closed
  2026-08-27, live at 119,900. `SendRestrictionFn` settles both sides of a share transfer, which
  the income index requires — a holder earns the movement in the index across
  the period they held, so a position has to be settled the moment a balance
  changes. It was written, commented, and **registered nowhere**: `app.go`
  appended only the enforcement restriction, and a repository-wide search found
  the function referenced by nothing at all, not even a test. So no transfer
  settled, no position was created by one, and every entitlement read zero.
  Found against a live vehicle holding 72 YML against 1,000,000 shares which the
  chain's own query said was owed to nobody. Second dead load-bearing function
  in this module after `FinaliseSale`, and both were found the same way: by
  driving the thing rather than reading it.

- **`mpc.Sign` panics when handed shares from two different sharings.** Found
  2026-08-31 while verifying the reshare claim by running the tool. An account
  was generated, resharded from `custodian + recovery` without the device share
  — the password-reset path — and the resulting address was identical, which is
  the property working as designed. Signing with the **old** device share and the
  **new** custodian share then crashes the process:

      panic: BuildLocalSaveDataSubset: unable to find a signer party in the
      local save data
      … tss-lib/ecdsa/keygen/save_data.go:92
      … mpc.Sign at mpc/mpc.go:296

  The refusal is correct — shares from two sharings must not combine — but a
  panic is the wrong way to deliver it, and the input is not exotic: a stale
  device share is exactly what a customer's phone holds after a reset it did not
  finish. `Sign` validates the share count and the digest length and then hands
  the set to tss-lib without checking that every share agrees on the same `Ks`,
  which is the check that would turn this into an error somebody can read.

  **Scope, stated because it was measured and not reasoned:** this is `Sign`, the
  all-in-one-process path used by `tools/mpc` and the tests.
  `NewSigningParty` — the production path, and the one `tools/custodian` uses —
  builds its committee from its *own* share's `Ks`, so it does not reach this
  construction with a mixed set and cannot panic here. What it does instead when
  its peer is on a different sharing was **not tested**, and should be: the
  plausible answers are a protocol that hangs and a signature that verifies
  against nothing, and neither is a good thing for a service to do on input a
  stranger controls.

- **x/tokenisation credits a sale's proceeds that never arrive.** `FinaliseSale`
  puts the whole reported price through the income index while moving no coins,
  and the only message that does move coins — `FundVault` — accrues them a second
  time on the way in. So the proceeds of a sale have no funding path that does
  not double-count, every holder is credited money the vault does not hold, and
  redemption fails with `insufficient funds` for everybody after the first.
  Found 2026-08-27 while writing the pipeline's first test. **Not fixed, because
  the fix is a decision rather than a mechanism:** either finalising pulls the
  price from the reporter — which makes a reported price binding, and closes the
  report-low-and-keep-the-difference attack a second way — or funding stops
  accruing once a sale is reported and the proceeds are simply the last
  `FundVault`. The first is stronger and assumes the sponsor holds the money on
  chain; the second is weaker and assumes nothing.
  `TestAVehicleCanBeExited` asserts the broken behaviour deliberately, so
  whoever decides this will see it fail and have to look.

- **A sender is not obliged to seal a payload to the readers the chain names.**
  `ROLE_SUPERVISOR` now confers an entitlement — a holder covering a country is
  a viewing-key recipient for every payload settling there, published by
  `Query/PayloadReaders` — but the envelope is built off chain and the chain
  holds only a hash of the plaintext. A sender that ignores the published set
  produces a payload the supervisor can never open, and nothing on chain
  detects it. That is a limit of where the ciphertext lives rather than a gap in
  the registry, and closing it would mean putting the recipient key ids on the
  payment record and checking them, which is a design decision nobody has taken.
- **A regulator appointed before the supervisor rule existed keeps the
  appointment.** `MsgAppointRegulator` refuses an appointee that holds no
  `ROLE_SUPERVISOR` covering the country, checked on the write and not re-read
  afterwards. So a country appointed under the old rule, or one whose regulator's
  grant is later revoked, has a sitting regulator holding no role — visible in
  `role-holders` and `regulator` disagreeing, and fixed by appointing again.
  Re-reading it on every payment was rejected because an appointment that could
  evaporate would leave a country's payments sealed to an authority the chain no
  longer lists, with nothing saying when it stopped.
- **Role grants made before `required_shape` are unpinned**, so their holders
  can still reduce themselves to a single key. Absent means no requirement, by
  design: the alternative disabled every authority on the upgrade block. On a
  chain still in development the fix is to re-make the grants; on one holding
  value it would need a migration somebody decided on deliberately.
- **`x/land`'s own admin path has the same weakness `required_shape` closes.**
  It asks whether a registry office is a group, once, and never again.
- **Every foundation administrator carried across by the upgrade is unpinned and
  may not be a group.** `Migrate2to3` writes each address from the retired
  parameter as a chain-wide grant with no `required_shape` and without asking
  whether the holder is an `x/group` account, because neither was true of a
  parameter entry and a migration that applied today's rule would be deleting the
  one authority that can correct a country rather than carrying it. Both are
  closed by re-granting deliberately, per grant, and the guide says how. Until
  then this is the same class of defect as the unpinned grants above.
- ~~**No payment has been pushed through the Pay screen end to end.**~~ Closed
  2026-08-26: 12.50 XOF at block 84,121, tx `336F01BA…`, code 0, carrying
  `ym1;e2e=YML-20260826-PM1SJCMM;purp=GDDS;rmt=Invoice 4471` on the ledger, and
  the reference decoded back onto the history row.
- **No account created in the app can be paid, and for a while nothing said so.**
  `MsgRegisterAlias` lands in a block and fails there — `this account has no
  recorded jurisdiction`, codespace `alias`, code 9 — because the chain has
  neither an approved participant nor a foundation administrator. The app now
  says this on the account screen instead of returning null and carrying on. The
  fix is chain state, not client code.
- **`clients/app` sends `MsgSend`, not `MsgSendPayment`.** `x/paymsg` requires
  both named participants to be governance-approved and the debtor to be a
  registered customer of the one it names, and this chain has **zero** approved
  participants. So the app sends a real transfer with the reference in the memo
  and says which rails carried it. The ISO message stays unreachable from any
  interface until a country is enrolled.

  Re-measured 2026-08-31: `ListApprovedParticipant` and `ListPaymentRecord` both
  return an empty page, and `RoleHolders` is empty for every country asked. **So
  the payment at height 84,121 is not an ISO 20022 payment and must never be
  described as one.** It is a `/cosmos.bank.v1beta1.MsgSend` of 12,500,000
  `uxof` carrying `ym1;e2e=YML-20260826-PM1SJCMM;purp=GDDS;rmt=Invoice 4471` in
  the memo — read back from the transaction, not from the release note. The ISO
  fields travel in a memo string; `x/paymsg` was not involved and holds nothing.
- **The OpenAPI merge collapses every module's `Params` into one definition**,
  currently holding only `x/validatorgov`'s fields.
- **`TestGenesisRoundTrips` in `x/alias` still has a vacuous `Owners` check** —
  the same shape as three others that were fixed.
- **`x/tokenisation` is served under `/yamale/` and the nginx allowlist names it
  under neither prefix**, so it falls through to deny-by-default. Correct today,
  a trap when somebody opens it up.
- ~~**`FinaliseSale` has no caller.**~~ Closed 2026-08-27, live at 119,900. It was a keeper method
  nothing invoked — no message, no EndBlocker, not one test — so no asset reached
  `STATUS_REALISED` and `Redeem`, which requires it, could never succeed for
  anybody: every fractionalised vehicle was a one-way door. `MsgFinaliseSale` is
  the caller, permissionless because a crank only the sponsor could turn is one
  the sponsor can decline to turn.
- ~~**`AttestSale` never checked who was attesting.**~~ Closed 2026-08-27, live
  at 119,900.
  `ErrNotAttestor` was registered as code 17 and returned from nowhere, and
  `Collection` carried no register to check against, so a sponsor met any
  threshold with fresh addresses at the cost of the gas — leaving the guide's
  own "the sale price is the attack" defended by nothing. Collections now carry
  an attestor register that **governance** appoints, not the seller.
- ~~**A holder who never transferred was paid nothing.**~~ Closed 2026-08-27, live
  at 119,900.
  `Fractionalise` created the vault and no position for the owner, so `Settle`
  treated them as a first-time holder on the way out and started them at an
  index that had already moved. Hidden because any transfer settles both sides,
  so the ordinary issue-then-distribute path created the position by accident.
- ~~**`DisputeSale` returned `ErrStillInWindow` for a window that had closed.**~~
  Closed 2026-08-27, live at 119,900; `ErrWindowClosed` is code 34.

## Operational loose ends

- **The oracle had never agreed a price, and now has.** Found 2026-08-31, closed
  2026-09-01. Both validators had missed **every** window since genesis — 11,771
  of 11,771 for `pi`. Nothing was broken: nobody had ever nominated a feeder, and
  nominating one needs the validator's operator key, which on both hosts lives in
  a password-protected `keyring-file` that only the operator can open.

  With one delegation per validator the oracle agreed 48 rates at **141,264**,
  and both validators now report — `voting_power_bps` 10,000 — from two
  *different* sources, because the aggregate is a stake-weighted median and two
  feeders on one endpoint is one source counted twice.

  One number is worth keeping: `pi` alone is 5,717 bps against a 5,000 bps
  threshold, so a single feeder does produce a price, with no median at all
  behind it. That is a property of the current split rather than a safeguard.

  What this closes is narrower than it looks, and the distinction is the one this
  document exists for: a rate now exists, so anything consuming one *can* be
  exercised. Fee conversion, the appointed-valuer path and a stablecoin peg check
  still have not been.

- **A validator above two thirds silently excludes every other validator.**
  Found and fixed 2026-08-21. `pi-2` signed 5 of 40 blocks, was jailed for
  downtime and slashed 1%, on a 21ms direct link with clocks four seconds apart
  and a load average of 0.5 — nothing wrong with it. A validator holding more
  than two thirds finishes consensus with its own precommit before gossip
  reaches anybody, so the other node gets the commit before it has the block and
  cannot vote for what it has not seen. Rebalancing to **64.56% / 35.44%** took
  it from 5 of 40 to **25 of 25** with nothing else changed. The cost is that
  neither node can now commit alone, so either one's outage stops the chain —
  which two validators could never avoid. Four is the number that gets both
  properties. The split has drifted since and reads **57.18% / 42.82%** on
  2026-08-31 — 100,000 and 74,900 of 174,900 bonded. Neither holds two thirds,
  which is the property that matters, and the chain was not catching up when
  read.
- **The public host is the Pi, and nothing checked that.** Found 2026-09-01.
  Consoles were deployed to the VM for a day while the funnel served the Pi's
  month-old copy; every check passed, because every check asked the VM. Worse,
  the Pi's REST allow-list had drifted a revision behind and was missing
  `cosmos/auth/.../accounts/`, which every client reads before it can sign — so
  signing was broken for every public visitor, and it failed as a `401` with a
  `WWW-Authenticate` challenge, which a browser renders as a login box on an app
  that has no login. `deploy/deploy.sh` now verifies against the funnel hostname
  and exits non-zero when the public site is not what the repo builds.
- **Ten of the fourteen consoles were linked from nowhere.** Found 2026-09-01,
  when the land register was asked about. It was deployed, working and answering
  200 — and reachable only by typing the URL, along with the guided tour,
  markets, vehicles, governance, oversight, foundation, validator and threshold
  keys. This is the client-side twin of the oracle row above: served is not
  reachable, the same way merged is not running.
- **A validator operator passphrase is in `~/.bash_history` on the VM**, in the
  clear, and appears to be a reused personal password. Mode `0600`, so it is one
  `sudo`, one backup or one host compromise away. It unlocks the key for the
  majority validator. Nothing needs it again — both feeder delegations are made —
  so shredding the history and rotating the value costs nothing.
- **The ops signing service is still running** with two htpasswd files. It was
  always a devnet crutch; the plan is client-side signing through
  `@yamale/connect`, then delete `/api/ops/`, both credential files and both
  copies of `opsd.py`, and move the consoles onto the tailnet.
- **The hosted ceremony process is gone**, killed by the VM's reboot on
  2026-08-21. It had been holding a coordinator token that could still issue
  invites. Do not restart it between ceremonies.
- **There were two faucet units and only one could ever work.** A faucet needs a
  funded key, and duplicating it duplicates custody of that key for no benefit.
  The Pi's copy was inactive and still configured for `yamale-devnet-1`, so the
  public funnel answered 502 while the real faucet answered fine on the other
  hostname — which reads as "the faucet is broken" rather than as "you are asking
  the wrong host". The funnel now proxies to the one real faucet over the tailnet
  and the stale unit is disabled.
- **A deploy used to be invisible.** nginx sent no `Cache-Control` at all, so
  browsers applied heuristic freshness and an old page after a deploy looked
  identical to a fix that did not work. That is the diagnosis that burns hours,
  not the wait. Now `no-cache` with ETag revalidation for HTML, CSS and JS, and
  `immutable` for the content-hashed bundles under `/assets/`.
- **The Pi's operator key lives on the Pi.** Correct while rehearsing; the
  validator guide says to take it off the node once genesis is done, because a
  node needs only its consensus key to produce blocks.
- **The staging chain's custodians are one person.** Its 3-of-5 is a 1-of-1 with
  four extra steps, which is fine for exercising the process and must never be
  carried into a deployment holding value.

## No fault tolerance, and the arithmetic that fixes it

A chain survives losing a validator only if the ones left hold more than two
thirds — so **every validator must hold less than a third**, and no set of two
can manage it. With 100,000 against 10,000 the majority node is a single point
of failure; with 55,000 each, either node's outage halts the chain. Two
validators can be arranged to tolerate the *joining* node's outages, which is
what [launch.md](../guides/launch.md) recommends and is right for a rehearsal,
but they cannot be arranged to tolerate both.

There is a second cost, and it is the one that bites first. A validator above
two thirds commits without waiting for anybody, so the minority node is
structurally unable to get its votes counted and is jailed for downtime while
being entirely healthy. A lopsided pair does not buy one strong validator and
one weak one; it buys one validator and one node being punished.

**Four equal validators is the minimum that tolerates one loss**: each holds
25%, any three hold 75%. That is also exactly what the equal-seats decision
produces, so this is an argument for seating four rather than for weighting
three.

## The test that had never run, and has now

**Three of five custodian keys moving something, and two failing to.**

Exercised repeatedly, on local chains built by the real `ceremony group` tool:

- **Three signatures executed.** A restitution paid out with the `MsgExec` sent
  by a custodian who had never voted — the property worth having, since the third
  signature and the person who presses the button need not be the same. And a
  custodian swap: one out, one in, count still five, atomically.
- **Two signatures did not.** `EventExec NOT_RUN`, status still SUBMITTED,
  balance unchanged.
- **The constitution refused what it should** — a bare removal rejected at
  submission, and `MsgLeaveGroup` likewise.
- **A country was enrolled end to end**: the foundation granted an office
  `PAYMENTS_AUTHORITY` in SN on three signatures, the office admitted a bank
  inside SN and was refused outside it, a non-foundation group granting was
  refused, and the foundation granting itself `*` was refused with chain-wide
  grants still empty.
- **An office that shrank lost its authority.** Granted as 3-of-5, it voted
  itself to 1-of-5 and the same action was refused; it restored itself on one
  signature and worked again with no re-grant.

**What remains is doing it on `yamale-devnet-2`**, with its seven-day voting
period, through the console at `/foundation/`. That is a test of the *process*
rather than of the mechanism: five people in five countries, a deadline a week
out, and an interface instead of a command line.

## The measurement that keeps being worth repeating

Every claim in this document that reads as a fact was measured. The ones that
cost the most to learn, in the order they bit:

1. **Broadcast `code: 0` means accepted, not executed.** Five separate bugs.
   The fifth was subtler than the rest: an office acts through its own group, so
   an `x/group` proposal that fails in execution still produces a transaction
   with code 0, and the refusal is inside `EventExec`'s logs.
2. **proto3 cannot distinguish 0 from unset.** Four separate bugs. `OfficeShape`
   is a nullable message rather than two integers for exactly this reason.
3. **A policy address derives from the group's sequence number alone.** A live
   ceremony predicted an office address and got *the foundation's own*, because
   both were policy sequence 1. Never predict one; read it back.
4. **x/group counts weight, not heads.** A threshold of 3 over member weights
   3,1,1,1,1 is a 1-of-5. Any interface displaying an office's shape must
   compute the fewest members who can reach the threshold, or it will show
   3-of-5 for what is actually a single key.
