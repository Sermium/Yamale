# What is left

Checked against the tree and the running chain rather than against a task list —
a task here was once marked complete while its artefact did not exist, so the
list is not evidence.

Last verified 2026-08-21, against `yamale-devnet-2`.

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

## Designed, documented, not built

**Browser signing for the foundation.** The console at `/foundation/` reads the
chain and composes the commands, and a custodian runs them. It cannot sign,
because `clients/wallet` decodes a signing request only as far as each message's
type URL — so an in-browser vote would mean hand-rolled protobuf that nothing
checks, on the account that receives every seizure. The page would say "pay
5,000 to Amara" and the wallet would say `MsgSubmitProposal` whether the encoding
was right, wrong or hostile. Decoding message contents in the wallet is the
prerequisite; after that, the three actions a custodian signs personally —
`MsgVote`, `MsgExec`, `MsgSubmitProposal` — are worth revisiting.

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

5. **Whether a netting reserve is seizable.** `x/enforcement` seizes
   `SpendableCoins` from a bank account; the uncommitted part of a posted
   reserve is plainly the participant's own money and sits in the netting module
   account, out of reach. A freeze blocks posting and withdrawing, so value
   cannot escape — but a seizure cannot reach it either.

## Known defects

- **`clients/app`'s Pay screen renders a success receipt without broadcasting**,
  and neither the app nor the wallet ever constructs `MsgSendPayment`. So
  `x/paymsg` — and with it every confidentiality field — is unreachable from any
  user interface.
- **`x/tokenisation` served under `/blockchain/` for a while** and is now on
  `/yamale/`; the nginx visibility allowlist names it under neither, so it falls
  through to deny-by-default. Correct today, a trap when somebody opens it up.
- **The OpenAPI merge collapses every module's `Params` into one definition**,
  currently holding only `x/validatorgov`'s fields.
- **`TestGenesisRoundTrips` in `x/alias` still has a vacuous `Owners` check** —
  the same shape as three others that were fixed.

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
- **The hosted ceremony process is still up** on the VM. Its work is finished
  and it holds a coordinator token that can issue invites. Stop it.
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

## The one test nothing had run

**Three of the five custodian keys moving something, and two failing to.**

Now exercised, though not yet on the staging chain. Against a 3-of-5 built by the
real `ceremony group` tool on a local chain:

- **Three signatures executed, twice.** A restitution paid out, with the
  `MsgExec` sent by a custodian who had never voted — which is the property
  worth having, since the third signature and the person who presses the button
  need not be the same. And a custodian swap, run from the console's own
  composed command with `--exec 1`: one member out, one in, count still five,
  atomically.
- **Two signatures did not execute, twice.** `EventExec NOT_RUN`, status still
  SUBMITTED, balance unchanged. A third proposal took three refusals and was
  rejected.
- **The constitution refused what it should.** A bare removal is rejected at
  submission — "would leave the foundation group with 4 custodians; the
  constitution fixes it at 5" — and so is `MsgLeaveGroup`.

What remains is doing it on `yamale-devnet-2`, with its seven-day voting period,
through the console at `/foundation/`. That is a test of the *process* rather
than of the mechanism: five people in five countries, a deadline a week out, and
an interface instead of a command line.
