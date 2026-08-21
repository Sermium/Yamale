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

## Written and passing, not yet merged

**The tiered netting layer.** `x/netting` in the worktree at
`.claude/worktrees/agent-a1b7c8556d3a8c96b` — keeper, msg server, ABCI,
autocli, generated reference docs, and ten test files including property,
security, settlement and compression tests. The whole tree builds and those
tests pass.

It is uncommitted because the agent writing it was killed by a spend limit
before it could commit, not because anything is wrong with it. What it has not
had is the verification the rest of this went through: four tag combinations,
the five drift guards, and a mutation pass. Until then it is *promising*, not
*trusted* — and it matters more than most, because the decision to get payment
confidentiality from architecture rather than cryptography rests on it.

The one thing to read first when reviewing it is the **settlement-failure
design**. A netting layer without an answer for a participant who cannot cover
its position is the most dangerous thing in this repository, and the brief asked
for queuing, collateral, partial settlement or bilateral limits rather than a
naive recompute.

## Designed, documented, not built

**Roles and `AssertScope`.** The jurisdiction registry exists and is queryable;
nothing consumes it. Until every authority action routes through one check, the
perimeter is a fact the chain knows and does not enforce. See
[roles-and-perimeter.md](roles-and-perimeter.md).

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

## Open decisions, §8

None of these can be answered from inside the code, and each changes scope:

1. **Which beachhead** — UEMOA or an Afreximbank partnership.
2. **Whether cross-chain collateral is a commercial requirement**, which alone
   decides whether IBC goes on the critical path.
3. **Threshold key custody: build or buy.**
4. **Vendor with support obligations, or reference implementation** a systems
   integrator productises. This one gates the LTS branches, the backport policy,
   the certification and the warranties — most of the cost in §6.

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

## The one test nothing has run

**Three of the five custodian keys moving something, and two failing to.**

Every step so far proves the group *exists*: it is in genesis, in state, with
five members and a threshold of three, and it is the recovery destination. None
of it proves the group *works*. "The foundation cannot actually spend" is the
worst thing to discover after a deployment, and it is the only claim in this
document that no other step exercises.

The foundation's voting period is seven days, so a proposal raised today decides
next week. That is right for five custodians in different timezones and
inconvenient for a rehearsal; the alternative is a throwaway second group with a
short window, which proves the mechanism without weakening the real one.
