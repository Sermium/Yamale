# What is left

Checked against the tree and the running chain rather than against a task list —
a task here was once marked complete while its artefact did not exist, so the
list is not evidence.

Last verified 2026-08-31, against `yamale-devnet-2` at block 124,691 — queried
over `https://pay.yamalelegal.com/api/rpc/`, and every figure below that reads
as chain state was read from that node on that day.

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

## Merged, but not yet running

The distinction this document previously lost. Everything here is in `main` and
has never been exercised against the network the validators are running, so a
query against the live chain returns nothing for any of it.

| | | |
|---|---|---|
| Country enrolment | `ceremony country` - offices, grants, jurisdictions, approved by the foundation | tool complete, **never run**: `RoleHolders` is empty for every country asked |
| Enrolment for a threshold account | `tools/custodian --import` takes a share file an operator produced with `tools/mpc keygen` | there is no path by which a member of the public gets an account |
| Distributed key generation | `mpc.Keygen` runs all three parties in one process | correct for a ceremony on one machine; in production the device's share must be generated on the device and never leave it |

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
| | 118,885 | `A8F18CAB…B15C3C` | `threshold signed: device + custodian` |
| | 118,968 | `6C784D06…4AFCFE` | `after a password reset: new shares, same address` |

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

**The service around it is not built, and that is still the commercial critical
path.** No enrolment over HTTP, no recovery workflow, no second factor, no
notification, no distributed key generation, no pre-parameter pool. An account is
created by an operator running `keygen` and `--import`. Everything in Part 5 of
[accounts.md](../guides/accounts.md) — two approvers from different teams, the
72-hour delay, notice to email and every enrolled device, the 24-hour outbound
freeze, the standard of proof — is specified and none of it exists. `Reshare` is
the mechanism a reset would use and nothing calls it in anger.

**And the consumer app still ships the model the design rejected.** `clients/app`
holds a CosmJS password-wrapped key in `localStorage`.
`clients/app/src/account.ts` says so in its own header and calls itself not
acceptable in production, which is the right way to carry a proof of concept —
but it means a forgotten password is a lost account today, and the threshold work
above is not wired into any interface a person could use.

Two divergences worth recording rather than leaving only in the code:

- **The blind index in `clients/app` is not one.** `emailKey()` is a bare
  `SHA-256` of the address. The design specifies `HMAC(email, pepper)` with the
  pepper held outside the store, and the difference is the whole property: a bare
  hash can be tested against any list of email addresses, so a dump yields the
  membership it was supposed to hide. Local-only storage makes it less severe
  than the same mistake server-side; it does not make the code's claim true.
  **`tools/custodian` now does it correctly** — HMAC with a pepper the service
  refuses to start without, refuses below 32 bytes, and refuses to read from
  inside the directory it protects. So the divergence is now between two places
  in this repository rather than between the code and the spec, which makes it
  cheaper to close and no less wrong until somebody does.
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

**The account service**, which the scope calls the actual commercial critical
path. Its key custody has its own section above, because it is now built rather
than merely specified. What remains untouched around it is most of the service:
enrolment for a member of the public, the recovery workflow, second-factor
enrolment, notification, and rate limiting or lockout on the custodian's password
check. `tools/custodian` covers authentication and the decision to co-sign, and
nothing else on that list. Plus USSD and feature-phone access, without which most
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

## Known defects

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

- **The oracle has never agreed a price on this chain, and nothing said so.**
  Found 2026-08-31. `Query/ExchangeRates` returns an empty set, and
  `Query/ExchangeRate` for every denomination asked answers *"no rate has ever
  been agreed"* — not a stale rate, never one. `Query/MissCounters` says why:
  both validators have missed **every** window they have been eligible for,
  10,188 of 10,188 and 10,394 of 10,394. No feeder is running. The vote period is
  12 blocks and the threshold 50%, so with two validators a single feeder cannot
  produce a rate on its own; two feeders are needed and there are none.

  This is worth more than its size. `/markets/` is listed above as live and it
  is — the console renders, the AMM pools behind it are real, and its oracle
  panel is correctly showing that there is nothing to show. But "the module
  works" and "the module has ever produced its output on this network" are
  different claims, and only the first of them was being made. Anything that
  consumes a rate — fee conversion, the appointed-valuer path, a stablecoin
  peg check — has never been exercised end to end here.

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
