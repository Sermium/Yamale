# What is left

Checked against the tree and the running chain rather than against a task list —
a task here was once marked complete while its artefact did not exist, so the
list is not evidence.

Last verified 2026-08-24, against `yamale-devnet-2` at block 52,356.

---

## Built, merged, and live on the chain

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
| Land registry | module, CLI, client; tokenisation refuses unauthorised fractionalisation |
| Tiered netting | `x/netting`: collateral posted first, hold-and-retry, no recompute path |
| Foundation console | `/foundation/` — the 3-of-5 has an interface, with the limits below |
| Roles and the perimeter | `x/alias` role grants, and `AssertScope` consulted by four modules |
| An office's M-of-N | recorded on the grant as `required_shape`, re-checked on every authority action |
| Country enrolment | `ceremony country` — offices, grants and jurisdictions, approved by the foundation |
| Administrator appointment | `ceremony administrators` plus a governance console that composes it |
| Coordinated upgrade | proposed, voted, halted at height, binaries swapped, applied — on the live chain |
| Signing-request decoding | the wallet reads a `TxBody` and says what it does, instead of naming a type URL |
| Visual system | `clients/shared/yamale.css` — real typefaces, a scale, elevation, semantic colour |

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

**The account service**, which the scope calls the actual commercial critical
path: authentication, threshold key custody, recovery with two approvers and a
delay, second factor. Plus USSD and feature-phone access, without which most
African transaction volume is unreachable, and agent-network and mobile-money
integration.

**A legal entity able to sign an indemnity.**

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

3. **Whether a bonded validator should still be able to freeze anything.**
   `AssertScope` now gates `x/enforcement`'s `OpenCase`, which changes the
   module's central property from *any bonded validator can freeze* to *any
   bonded validator governance has placed in that country's perimeter*. That is
   what [roles-and-perimeter.md](roles-and-perimeter.md) asks for and it is the
   point of having a perimeter — but it is a real narrowing of who can act in an
   emergency, and the narrower alternative was to scope only `EmergencyFreeze`
   and leave ordinary validators chain-wide. Worth confirming deliberately
   rather than discovering.

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

## Known defects

- **Two of the five roles do nothing an office can use.**
  `ROLE_SUPERVISOR` has no consumer anywhere in the tree, and
  `ROLE_ENFORCEMENT_AUTHORITY` cannot be exercised by an office because
  `OpenCase` additionally requires a bonded validator and `EmergencyFreeze`
  requires being the emergency-authority parameter. Enrolment grants both
  anyway — appointing an office later is harder than granting a role that is
  waiting for its message — and the runbook says plainly not to expect them to
  work. This is a gap between the design and the modules rather than a bug in
  either.
- **Role grants made before `required_shape` are unpinned**, so their holders
  can still reduce themselves to a single key. Absent means no requirement, by
  design: the alternative disabled every authority on the upgrade block. On a
  chain still in development the fix is to re-make the grants; on one holding
  value it would need a migration somebody decided on deliberately.
- **`x/land`'s own admin path has the same weakness `required_shape` closes.**
  It asks whether a registry office is a group, once, and never again.
- **`x/alias`'s `Params.Validate()` accepts any non-empty string as a foundation
  administrator.** It refuses duplicates and a ninth entry but never asks whether
  an entry is an address, so a typo passes a vote, consumes one of the eight
  places and grants the power to nobody. The governance console checks the
  bech32 checksum itself precisely because the chain does not.
- **No payment has been pushed through the Pay screen end to end.** The screen no
  longer forges a receipt — it signs, waits for the block, and reports from
  execution rather than acceptance — but that path is verified by reading and by
  tests, not by moving money.
- **`clients/app` sends `MsgSend`, not `MsgSendPayment`.** `x/paymsg` requires
  both named participants to be governance-approved and the debtor to be a
  registered customer of the one it names, and this chain has **zero** approved
  participants. So the app sends a real transfer with the reference in the memo
  and says which rails carried it. The ISO message stays unreachable from any
  interface until a country is enrolled.
- **The OpenAPI merge collapses every module's `Params` into one definition**,
  currently holding only `x/validatorgov`'s fields.
- **`TestGenesisRoundTrips` in `x/alias` still has a vacuous `Owners` check** —
  the same shape as three others that were fixed.
- **`x/tokenisation` is served under `/yamale/` and the nginx allowlist names it
  under neither prefix**, so it falls through to deny-by-default. Correct today,
  a trap when somebody opens it up.

## Operational loose ends

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
  properties.
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
