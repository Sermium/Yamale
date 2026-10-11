# What is left

Checked against the tree and the running chain rather than against a task list —
a task here was once marked complete while its artefact did not exist, so the
list is not evidence.

**Chain state** re-verified 2026-09-13, against `yamale-devnet-2` at block
**320,662**, over `https://pay.yamalelegal.com/api/rpc/`. This sweep re-read the
open audit and configuration rows and nothing else, so **a figure here carries
the date it was read**: rows dated 2026-09-13 are current, rows dated 2026-09-05
(block 196,559) or 2026-09-01 (block 141,500) are older and say so, and a figure
with no date is older still and should be re-read before it is relied on. What
the 2026-09-13 sweep changed is recorded in *"What moved between 2026-09-05 and
2026-09-13"* immediately below.

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

## The move to yamale-us, 2026-10-10

**Live and checked against the network on 2026-10-10:**

- **The master validator runs on yamale-us** (OVH) since height
  **774153**. Its key moved from the Oracle VM; the chain halted about three
  minutes and missed no blocks. The VM runs a non-signing node with a new key.
- **Public endpoints:** rpc.yamaletech.com and api.yamaletech.com, through the
  same nginx gates the VM ran (unsafe RPC 403, unlisted REST 401).
- **Every chain app on its own subdomain** of yamaletech.com, 17 of them (app is
  pay., livestock is ladoum.) plus chain. for the hub and docs, all 200 with
  /api/ reaching the chain. Sign-in from explorer.yamaletech.com opens
  wallet.yamaletech.com, the one wallet origin.
- **Faucet, custodian and the master's feeder** run on yamale-us, each with its
  own user and only the key it needs. The faucet answers through all three entry
  points (pay.yamaletech.com, pay.yamalelegal.com, the funnel).
- **yamaletech.com** serves the Lovable company site (repository
  `yamaletech-site`).

**Open:**

- ~~The VM is not a validator yet~~ **Closed 2026-10-10**: admitted by proposal
  30 and created at block 778617 as `oracle`, 5,000 YML (2.8%), signing, with
  its own feeder and auto-unjail. The master keeps 68.6%, so it can still make
  blocks alone.
- ~~yamale-us is not on the tailnet~~ **Closed 2026-10-10**: it joined as
  100.76.214.3, and wallet.yamaletech.com reaches the recovery service on the Pi.
  The dashboard (status.yamaletech.com) shows all 31 checks green, including
  both validators signing; pi-2 is unjailed and has an auto-unjail.
- ~~The ops signer is still on the VM~~ **Closed 2026-10-11**: it runs on
  yamale-us, and governance.yamaletech.com's buttons reach both signers (pi
  locally, pi-2 over the tailnet) behind the console password.
- ~~pi-2 is jailed again~~ **Closed 2026-10-10**: unjailed at 776268, its
  feeder back on. `yamale-auto-unjail` on the Pi now unjails it through an
  authz grant (MsgUnjail only) when safe, and the dashboard alerts within
  seconds when any validator stops signing.
- **Accounts created at the old addresses stay there.** A wallet account lives
  in the browser storage of the wallet's origin, so pay.yamalelegal.com/wallet
  and wallet.yamaletech.com are different wallets to a browser.
- **Caddy's apt repository answers 402 Payment Required** (Cloudsmith), so the
  server cannot update Caddy until the source is replaced.
- **The company site's contact form sends nothing**: its handler shows the
  thank-you and discards the message, on Lovable's preview as well.
- **Shared logins (yamalelegal and yamaletech) are not built.** yamalelegal's
  accounts live in Clerk; sharing them across a second domain means changing the
  production Clerk configuration, which is a decision, not a deployment.

## What moved between 2026-09-05 and 2026-09-13

Re-read from the live chain at block **320,662** on 2026-09-13. Governance was
active in the interval; five things that this document listed as open or
undeployed are now closed on the chain, and are moved into the table below:

- **The RPC method gate is deployed.** `POST /api/rpc/` with
  `{"method":"net_info"}` (and `unconfirmed_txs`) now answers *"not available on
  this endpoint"* — `tools/rpcgate` is on both hosts, so the JSON-RPC bypass that
  "Two the audit did not find" recorded as *not deployed* is shut.
- **The security headers are deployed.** `pay.yamalelegal.com` returns HSTS,
  `X-Frame-Options: DENY`, `X-Content-Type-Options`, `Referrer-Policy` and a CSP
  with `connect-src 'self'` — the row under "Operational loose ends" that read
  *in the repo, not on the hosts* is closed.
- **The ombudsman is set** to `yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr`
  (a foundation group account), so `MsgOmbudsmanVeto` has a holder — the
  "Configuration, measured in minutes" bullet is closed.
- **The rolling seizure cap covers all 48 denoms**, not `uyml` alone — the same
  bullet's second half is closed.
- **The three commercial modules are in the deployed binary.**
  `abci_query` on `/blockchain.herd.v1.Query/Params`,
  `/blockchain.syndicate.v1.Query/Params` and
  `/blockchain.selfcustody.v1.Query/Params` all return code 0, so the upgrade
  that "Merged, but not yet running" describes has been applied.

**Superseded the same day.** The qualification above — "present, not exercised"
— was written in the morning of 2026-09-13 and stopped being true that
afternoon. Re-read at block 321,363: `x/herd` holds **2 registry offices and 9
animals**, `x/syndicate` holds **1 admitted operator and 2 deals** (one OPEN and
below its minimum, one CLOSED and paying out), and `x/selfcustody` publishes
parameters whose acknowledgement hash matches the binary's embedded text. Both
consoles answer 200 **on the funnel**, which is the host the public reaches:
`yamale.tail4355e8.ts.net/livestock/` and `/syndicate/`. So all three modules move
out of "Merged, but not yet running" and into the table above.

Two limits on that claim, stated because the section heading is a strong one.
The 48-hour disclosure wait has still not been waited out on any chain, so
`x/selfcustody`'s confirm-and-record half remains proved only by its tests. And
the herd "refusals" stage was not run on devnet-2 — it writes nothing on
success, every declaration being meant to fail, so it populates no state and its
absence changes no row.

**The funnel was dark for 21 hours and is back.** Between roughly 2026-09-12
10:00Z and 2026-09-13 the Pi answered on no port: its SSD had stopped
enumerating, so it fell back to an SD-card install that carries the kiosk and no
web stack at all, and Tailscale was logged out on the OS that did boot. Three
consequences worth recording, because each one contradicts something this
document has previously asserted:

- The Pi is **two machines**, not one. `BOOT_ORDER=0xf14` tries USB first and
  falls back to the SD card, and the two disks carry different operating
  systems: the SSD is the Yamale host (nginx, `/opt/yamale`, the validator), the
  SD card is `borne-vacances-0001`, a SmartAX kiosk. "The Pi is down" and "the
  Pi booted the other disk" are different faults with the same symptom.
- Its crash loop was **not** the validator. Measured: the kiosk browser holds
  ~2.2 GB of RSS+swap on a 1.8 GB box, swap lived on the SD card, and thrashing
  drove iowait to 55% and the load average to 31 until `watchdog` reset the
  machine — `error 253 = 'load average too high'`, in its own log. Stopping the
  validator changed nothing. Moving swap to zram (zstd, ~5:1) dropped the load
  to 0.2 and it has been stable since.
- `pi-2` **cannot block-sync and never could**. The VM rejects it with
  `invalid LastCommitRound 0 (must be -1 for initial height 265844)`, so it
  connects, is dropped with EOF, and retries for ever at 0 peers. It has to be
  re-seeded by state sync; done 2026-09-13, 265,843 → head in about four
  minutes, and it is bonded again.

**The four refusals, run on devnet-2 2026-09-13.** Each declaration is one a
paper certificate would carry without complaint, and each was refused by
consensus with the chain's own reason, read out of the execution events rather
than inferred — a group proposal whose message is refused still rides a
SUCCESSFUL transaction, so the tx result would have reported every one of them
as an acceptance.

| declaration | the chain's reason |
| --- | --- |
| a dam that is male | `animal 4 is SEX_MALE` |
| a sire born after his own offspring | `sire 7 was born 2023-06-14 and…` |
| a parent the register does not hold | `sire 9041: the parent named is…` |
| a certification with no laboratory | `animal 12: a certified animal…` |

Kept as `scripts/devnet/herd-refusals.sh`, because the seeder's own `refusals`
stage can no longer be used for this: its probes name animals that are now
mid-transfer or disputed, so they would be refused for not being ACTIVE rather
than for the pedigree being impossible — which that script's own comment
correctly calls worse than showing nothing.

**One probe was mislabelled and is worth recording rather than quietly fixing.**
"An animal as its own ancestor" IS refused, but by the BIRTH-DATE rule, not the
cycle check: a dated cycle always implies a descendant older than its ancestor,
so dates catch it first. That is why x/herd's own cycle tests use undated
animals. It was replaced with a parent the register does not hold, which fails
for a reason nothing else covers.

**Seeded to show the state machines, 2026-09-13.** Both consoles were showing
one frame of each module. Re-seeded so a demonstration can walk the whole range:

- `x/herd` now carries **five of six transfer states** — PROPOSED, ACCEPTED,
  ATTESTED, OBJECTED and CANCELLED, on five different animals. COMPLETED is
  absent and cannot be seeded today: it needs the seven-day challenge window to
  run after ATTESTED, which is the point of the window. The objection carries a
  real reason and left its animal DISPUTED, which is the module's whole
  argument — an objection anybody may raise and an office must answer.
- `x/herd` now carries **cattle**. A second herd at Dahra Djolof, 60 head
  declared and three Gobra zebu identified individually under `/B` identifiers
  against the sheep's `/L`. The 57 unidentified are the module's economics
  rather than an omission: a record costs what it proves, so a transhumant herd
  is registered as a herd and only the animals that will be sold, pledged or
  exported are worth identifying one by one. Until this, every animal on a
  cattle register was ovine.
- `x/syndicate` now carries **all four deal states** — OPEN, CLOSED, ABORTED and
  REALISED. The abort repaid the committed 900,000 in full, fees included,
  measured on the investor's balance before and after; the realisation was a
  final distribution, which stops trading because every token is then a claim on
  a known fixed sum.
- `x/syndicate` now carries **a syndicate-shaped book**. Four more investors
  across SN, NG, KE and MA, onboarded in one batched group proposal, and a live
  raise at 8.10 of 12.00 YML against a 7.00 minimum spread over six cheques from
  2.80 down to 0.40. Two investors is a co-investment; the spread is the product,
  and one investor who can write the whole cheque does not need a syndicate.
- **The console now shows the return.** `distributed / raised` at the deal level
  — exact, because both are fields of the deal — as a percentage of capital and
  as a multiple, on the deal list, on a card, and in a ladder with the four
  things it is not: not an annual rate (an IRR needs the date of every cash
  flow, and the chain carries the distribution total rather than its schedule),
  not a valuation, not any one holder's return, and not a forecast of the other
  deals. The holding view attributes a share of it to a position, and says in
  the same breath that this is exact only for a position that has not traded:
  the index accrues per position and keeps no register of payments per holder,
  which is what makes a distribution to thousands of them cost one write. On
  `syn-alice` that caveat is visible rather than theoretical — she can claim
  more than a position her current size has earned, because she held more of
  the deal earlier.

**Deal documents, live on devnet-2 since 2026-09-14.** `Deal` carried
`reference`, a free-text string, and nothing else — no term sheet, no
subscription agreement, no shareholders' agreement. `x/syndicate` now has
`MsgAttachDocument` and `Query/Documents`: an admitted operator group records a
SHA-256 digest, a kind, a title and a location, in any deal state including
REALISED, and amends by supersession so the earlier version stays readable. The
documents themselves are never written to the chain — a subscription agreement
names its investor and their cheque.

Proposal 18 passed (37.0% of bonded, no votes against) and the plan
`syndicate-documents` applied at height **335,762** on both validators.
`Query/Documents` answers `code 0`. Deals 2, 4 and 5 carry real papers whose
digests were verified against `sha256sum` of the files that produced them. 18
keeper tests, 8 console tests, both build profiles, reference docs and the
OpenAPI spec regenerated.

**Proposal 17, the first attempt, was REJECTED with zero votes** — the
30-minute window ran out while nobody was at the keyboard. Worth recording
because the failure was clean: an unvoted upgrade proposal never schedules its
plan, so the chain sailed past 333,108 without halting and nothing needed
undoing.

**Two mistakes of my own, both on the chain for good.** The seeder hard-coded
deal ids from creation order; deal 1 is SYN-2026-FAIL and not SYN-2026-001, so
seven papers describing Sahel Logistics were filed against Kigali Fintech.
There is no delete — by design — so the correction went forward: six Kigali
Fintech papers now supersede them and the wrong ones stay visible. Ids resolve
through the reference now. Separately, the binary-swap script refused a node
that had genuinely halted, because it grepped 200 journal lines for
`UPGRADE NEEDED` and thirty-second pex chatter had scrolled it away; it tests
the height against the scheduled plan instead. Both fixes are in
`scripts/devnet/`.

Runbook: [upgrade-devnet-2-syndicate-documents.md](../guides/upgrade-devnet-2-syndicate-documents.md).

**A positioning error, corrected 2026-09-13.** The console was `clients/ladoum`,
titled "Registre du Ladoum", and its certificate carried a hardcoded section
header reading "Mouton" — so a general-purpose register whose first species is
cattle, offered against a tender for exactly that, introduced itself as a
sheep-breed registry. `x/herd` itself was never breed-specific: five species,
`SPECIES_CATTLE` first, and "Ladoum" only ever in comments as the worked example
the identifier format was drawn from. Renamed to `/livestock/`, "Registre du
cheptel", and the species is now read from the herd rather than asserted —
verified on devnet-2, where Classico's certificate reads "Ovin" and a cattle
record would read "Bovin" from the same code path. The old `/ladoum/` path is
gone from both hosts and was never shared publicly.

**A finding that qualifies the two-validator rows.** `pi-2` is bonded and
counted live — `missed_blocks_counter` is 0 — but it precommits **nil** on
most blocks, committing roughly 3 in 14. The cause is structural rather than a
fault: the VM holds 70.21%, its own supermajority, so it completes a round alone
in ~150–230ms while the Pi needs ~250ms merely to execute the block. `pi-2`
receives block and commit together and never sees a live round to vote in. This
is the same arithmetic as "A validator above two thirds silently excludes every
other validator" below, which was fixed once by rebalancing and has drifted
back. You cannot have both "the VM can run alone" and "pi-2 meaningfully
validates".

Everything under "The one that makes the rest conditional" (the 887.9M-YML
premine, C-3), "The governance electorate is a feeder key" (C-4), the treasury-2
admin rotation (H-4), `max_gas = -1` (K-3), the `abci_query`-past-REST bypass
("The gate that stops at the wall"), and the 100% concentration ceilings (H-5)
was **re-confirmed still open** on 2026-09-13 and is unchanged below.

## Live on the chain, and verified there

| | |
|---|---|
| Constitutional layer | 13 invariants fixed at genesis; `MsgUpdateParams` refuses them |
| Public RPC gate | `tools/rpcgate` deployed both hosts — JSON-RPC POST bypass shut; verified 2026-09-13 |
| Web security headers | HSTS, frame-deny, nosniff, referrer-policy, CSP `connect-src 'self'` on both hosts; 2026-09-13 |
| Enforcement oversight, configured | ombudsman set and seizure cap over all 48 denoms; 2026-09-13 |
| Commercial modules in the binary | `x/herd`, `x/syndicate`, `x/selfcustody` answer `abci_query` — upgrade applied; present, not yet exercised on devnet-2; 2026-09-13 |
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

**Updated 2026-09-13.** The binary caveat below — *"the deployed binary predates
the module"* — is no longer true for `x/herd`, `x/syndicate` and `x/selfcustody`:
the upgrade landed and all three answer `abci_query` on devnet-2 at block
320,662. What is still true is the second, weaker claim: none has been
*exercised* there (no office, animal or deal seeded), and their consoles are not
in the public gate. Read these rows as "present in the binary, not run on
devnet-2" rather than "not on devnet-2 at all".

| | | |
|---|---|---|
| Country enrolment | `ceremony country` - offices, grants, jurisdictions, approved by the foundation | tool complete, **never run**: `RoleHolders` is empty for every country asked |
| Enrolment for a consumer account | `tools/custodian` serves `POST /v1/enrol`: one deposit per service, signed by the account key so nothing in the middle can substitute the holding or the address | **closed 2026-09-22.** An end-to-end test enrols against two live services, loses the device holding, drives a full recovery through both, and signs with the reassembled key under the original public key |
| Distributed key generation | `mpc.KeygenParty` — one participant per process, share computed locally and never transmitted | **closed 2026-09-03.** A test captures every byte the three parties exchange and asserts no party's secret scalar appears in it |
| Enrolment over HTTP | `tools/custodian` `/v1/enrol/*`, driven by the device against two deployments (`--role custodian` and `--role recovery`) | built and tested; never run against a real browser or a deployed pair |
| The recovery process of Part 5 | two approvers from different teams, 72-hour delay, notice that aborts on failure, 24-hour outbound freeze enforced on every signature, recorded proof, aggregate publication | built and tested; never exercised by a real operator |
| `x/herd` — the livestock registry | three levels of record, pedigree refused at consensus, four-party supervised transfer with an answerable objection, accredited agents | 44 tests; **exercised end to end on a local devnet 2026-09-10** — two offices admitted by a real governance vote, ten animals, seven certified lines four generations deep, a sale carried to ATTESTED with two independent attestations, four impossible pedigrees refused. **Run on devnet-2 2026-09-13**: two offices admitted by governance (proposal 15), 9 animals, Classico's line walking three generations, and Bathie's sale carried to ATTESTED with two independent attestations |
| The livestock console, on `x/herd` | certificate with the species read from the herd, evidence level, chain-side pedigree walk, provenance including refused sales | 43 browser tests; **rendered against that local chain**, 14 of 14 ancestors resolved. **Deployed 2026-09-13**, answering 200 on the funnel |
| `x/syndicate` — the syndication layer | escrowed commitments, close-or-refund, three fee points, KYC gate as a bank send restriction, pro-rata distribution index, private and open secondary listings | 37 tests, both build profiles. **Exercised end to end on a local devnet 2026-09-12**, all four stages of `scripts/devnet/seed-syndicate.sh`: an operator admitted by a real governance vote, a raise that missed its minimum, one that closed at 3.00 YML with 3% taken at entry, a private sale between two distributions, and three holders paid correctly across it with nobody reconciling anything. Running it is what found the panic below. **Run on devnet-2 2026-09-13**: operator admitted by governance (proposal 16), a raise short of its minimum, one closed at 3.00 YML, and a private sale followed by a distribution that paid the secondary buyer 144,000 without anybody reconciling anything |
| The Syndicate console, on `x/syndicate` | the book against both its thresholds, the fee ladder in money, entitlement with its working, the secondary market, and the commands with the account that signs each | 62 browser tests; **rendered against that local chain** across all four views. Read-only: it holds no key and composes no transaction. **Deployed 2026-09-13**, answering 200 on the funnel |
| `x/selfcustody` — the key-disclosure pathway | request, chain-enforced wait, confirm, all three signed by the account itself; the warning pinned by hash; a permanent record and a published count but no roll | 22 tests. Linked into every build profile, unlike the commercial modules: the arrangement exists on a settlement deployment too, and a binary carrying the custody but not the door out of it would be the arrangement the acknowledgement promises this chain does not operate. **Exercised on a local devnet 2026-09-12**: a request recorded with its two deadlines computed from the parameters in force, a confirmation refused with "172800 seconds to go", and a cancellation that left nothing behind. The 48-hour wait has not been waited out on any chain, so the confirm-and-record half is proved only by the tests |
| The field application, on `x/herd` | identify a beast from a national identifier, a transponder or a tattoo; the herd offline in a scoped replica; a walkable pedigree; an iris matcher that refuses to guess | **Deployed 2026-09-22** at `/herd/`, in five languages, verified against devnet-2: `221/682/000102/L` resolves to Nianing with its line walked three generations. 228 tests. **No hardware has ever been attached** — the IriShield's USB protocol and the stick reader's GATT service are unconfirmed, so those capabilities report which vendor answers are missing rather than offering a dead button. The matcher's thresholds are the human-iris literature's and MUST be re-derived from field captures before anybody relies on them |
| `x/market` — the secondary market | a fixed-price listing, an auction by deadline, and a resting order in a book; shares escrowed behind an ask, money behind a bid; every fill writes the buyer's cost basis into `x/tokenisation` | **Applied on devnet-2 at height 378,800, 2026-09-17** (proposal 22). **Exercised there the same day** by `scripts/devnet/seed-market.sh`: a parcel of 100,000 shares listed at $60,000 and taken, an auction of 50,000 with its $25,000 reserve met at $26,000, and a two-sided book at 0.55 against 0.65. The fill wrote the first cost basis this chain has ever held — 60,000,000,000 uusd over 100,000 shares, which is $0.60 each. **Interface built** in the tokenisation app as the vehicle page's Market folder, in all five languages, deployed and answering on the funnel |
| Pro rata temporis, and a recorded cost basis | entitlement apportioned by time held rather than by who holds when a dividend is declared, in constant time whatever the gap; `cost_basis` and `cost_qty` so a holding can be partly covered | 3 bugs found by the tests and fixed: 600,000 of a 900,000 distribution stranded after the last touch, a holder owed exactly 100,000 paid 99,999, and a zero-length period losing the money entirely. **Applied on devnet-2 at height 376,100, 2026-09-16** (proposal 21). Every entitlement carried across at the old rule, read back off the chain: 11,700,000 / 1,350,000 / 330,000 USD against a 15,000,000 vault, for 78% / 9% / 2.2%. `cost_basis` and `cost_qty` are 0 on every migrated position, which means NOT RECORDED — nobody ever told the chain what these holders paid, and the console says so rather than showing a cost of zero. The field populates on a subscription or a market fill, neither of which has happened |

**A note on the rows above.** Several say "exercised on a LOCAL devnet", and
that is a weaker claim than the section heading implies — deliberately so. A
local chain proves the state machine; it does not prove the deployment, the
proxy, the funnel, or that anybody but the author has run it.

**What running it was worth, measured.** `x/syndicate` sat in this table from
2026-09-10 to 2026-09-12 as "not run on any chain", with 37 passing tests, both
build profiles and a clean `buf lint`. On 2026-09-12 it was linked into a binary
that was actually started, and the binary did not start:

    $ blockchaind version
      panic: open-deal flag redefined: fees

`MsgOpenDeal.fees` collided with the SDK's universal `--fees` transaction flag,
so pflag refused the duplicate while the root command was being built — killing
`version`, `start` and every other subcommand, not merely `tx syndicate
open-deal`. No test constructs the CLI, so nothing in the suite could have
caught it. Renamed to `fee_schedule`.

Rendering the console against the seeded chain found three more: a book bar
whose fill colour was never applied, so every raise drew as empty including a
fully funded one; a shortfall printed as `1600000` in a sentence next to
"1.40 YML raised"; and a deal-token quantity with no unit a reader thinks in.
All three are units-and-pixels faults, which is the class of defect that passes
every arithmetic test there is.

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

**Superseded 2026-09-22: the threshold protocol is replaced by sealed secret
sharing, and tss-lib is no longer on the signing path.** Everything in the two
paragraphs below is history from here on — it describes a scheme that ran, that
signed on this chain, and that the architecture decision of 2026-09-18 removed
the reason for.

That decision — a deal is between a buyer and a seller, and the services appear
only in a litigation or a recovery — means nothing co-signs. Threshold ECDSA
exists so several parties can sign without any of them holding the key, which is
precisely the property that was no longer wanted. What is left to do is put a
lost key back together, and that is secret sharing.

The replacement is `recovery`: the device generates an ordinary secp256k1 key,
seals it under the account holder's password with Argon2id and XChaCha20, and
splits the SEALED blob 2-of-3 over GF(2^8). Sealing before splitting is the
whole design — split the raw key and the custodian plus the recovery service,
both run by the platform, reconstruct it between them.

What went with it, measured rather than estimated:

| | Before | After |
| --- | --- | --- |
| Enrolment on a phone | ~9.5 minutes, and it needed a Web Worker to avoid freezing the tab | one HTTP request |
| Enrolment endpoints | 3, with a session store and a 10-minute TTL | 1, stateless |
| Shipped to the browser | a 19 MB `mpc.wasm` | nothing; the arithmetic is small enough to write in the client |
| `tools/custodian` | — | 1,253 lines removed |
| Package test time | 148s (`mpc`) | 12s (`recovery`) |
| Third-party crypto dependency | tss-lib, with a withdrawn `agl/ed25519` in its module graph | none |

What was GIVEN UP, and it is not nothing: with both services breached, the
account is exposed to offline password guessing at Argon2id cost. The threshold
scheme offered no such avenue. It is the same trade decision A already made
everywhere else — nothing co-signs and nothing can freeze, so a stolen phone and
password is a stolen account regardless. The custodian can no longer refuse a
payment, and a frozen account can still spend; a freeze now blocks a RECOVERY,
which is a narrower promise. The 24-hour post-recovery hold moved from "access
restored, spending frozen" to "the holding is released 24 hours later", because
releasing it is the only lever this service still has.

Closed 2026-09-22: `clients/keys` was rewritten. The demonstrator no longer runs
a threshold protocol or ships the 19 MB binary — it seals nothing in the page
(that is Argon2id at 128 MiB) but splits and recombines the REAL sealed values
from `recovery/testdata/vectors.json` in about a millisecond, and its third copy
of the sharing arithmetic is held to those same vectors by its own tests. The
directory went from 19 MB to 141 KB. The two on-chain payments stay, relabelled
as history: they were made under the threshold scheme.

**Deleted 2026-09-22:** `mpc/`, `mpc/wasm`, `tools/mpc/`, `spike/dkls23`, the
`github.com/bnb-chain/tss-lib/v2` dependency, and the `replace` directive for the
withdrawn `github.com/agl/ed25519` that existed only to make tss-lib's module
graph resolve. `docs/guides/mpc.md` is replaced by
[recovery.md](../guides/recovery.md). Nothing in the repository imports a
threshold library any more.

**Deployed 2026-09-22.** The frontend is live on the funnel and verified there:
`/keys/` runs the split-and-recombine demonstration (the two service parts
rebuild the sealed blob, and it is visibly not the key), the explorer and the
investments console sign through the wallet, and the wallet carries the
protection panel.

**The two services are DEPLOYED, 2026-09-23.** `custodian` on the VM at
`100.117.244.96:8099`, `recovery` on the Pi at `127.0.0.1:8100`, reached from
the internet as `/api/custodian/` and `/api/recovery/`. Never both on one host:
a host holding two of the three parts is one that can attempt a recovery alone.

Verified by enrolling a real account through the public hostname with the
wallet's own code: sealed and split in 656 ms, both services accepted the
deposit, and both independently derived the SAME address from the public key
they each verified. `tools/notify` is installed on both and both report
`"recovery":"ready"`.

Two things found while installing it. The sealing keys and peppers are raw
32-byte files and `readSecret` trims whitespace, so roughly one draw in twenty
produces a 31-byte key and a service that refuses to start for a key that was
generated correctly. Fixed at the source 2026-09-30: `readSecret` decodes hex
(the documented form, `openssl rand -hex 32`), takes exactly 32 raw bytes
untouched, and refuses anything else with that command in the message. The
test for it fails against the old code on a raw key ending in `\n`. And `sites-enabled` held a
`yamale.before-account` backup, so nginx was loading every server block twice
and the right one won only on alphabetical order; it has been moved to
`/etc/nginx/backups/`.

Still open: accounts enrolled under the old protocol cannot be read by the new
services and must re-enrol — a migration is
possible in principle (the device part plus one service part reconstruct the old
key, on the device, which could then be sealed and re-split) but is not built,
and on a demonstrator chain it is probably not worth building.

**The share protocol is built, and it has signed on this chain.** `mpc` is
2-of-3 threshold ECDSA over secp256k1 on tss-lib: three shares named `device`,
`custodian` and `recovery`, any two of which sign, none of which signs alone. It
was documented in `mpc.md`, now replaced by [recovery.md](../guides/recovery.md). `tools/custodian` holds the second
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
built on 2026-09-03. What remains untouched around it is an **identity check
at enrolment**. The rest of what this sentence used to list has happened:
second-factor enrolment (an authenticator, confirmed before it counts), a client
(the wallet enrols through the public hostname), and **a per-account limit on
the password check, deployed 2026-09-30** -- five free mistakes, then a wait
that doubles from a minute to an hour, checked before the password so a guess
during it is refused unread, and applied identically to an address with no
account. nginx's per-address limit could not do this: an attacker has as many
addresses as they rent.

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
section is what the audit found that **code cannot close** —
[audit-remediation.md](audit-remediation.md) has the commands for it, in order — — every row here
needs a transaction, a vote or a key ceremony, and none of it has been done.

**Verified still live on 2026-09-05** against the funnel at block 196,559.

### Running the tests the audits could not, 2026-09-06

The audit's "not covered" list named fuzzing, simulation and differential
testing, and said plainly that *"running them at length would likely surface
invariant violations this review could not reach by reading."* It was right, and
the first thing it surfaced was why nobody had run them.

**The simulation suite had never been able to start.** Not skipped — impossible.
It died in `InitChain`, before block one:

    panic: failed to initialize constitution genesis state: constitution genesis
    is invalid, refusing to start: enforcement_recovery_destination must name the
    foundation account

`x/constitution` has no simulation package, so the simulator built its genesis
from `DefaultGenesis()`, and `DefaultInvariants()` leaves the recovery
destination empty on purpose — no address compiled into a binary is anybody's
foundation. Correct for a real launch, fatal for the simulator. `x/enforcement`
had already hit exactly this and seeded its own destination, with a comment
saying why; the constitutional copy of the same field was missed. Behind it,
`x/validatorgov`'s generator predated the `Declaration` field and produced
approved validators with no `legal_entity_id`, which its own genesis validation
refuses.

Both fixed. All four simulation tests now pass, including at the Makefile's own
settings — 100 blocks × 200 operations, 20,000 operations in a run — across five
seeds, plus import/export, simulation-after-import and state determinism.

**And then the simulation immediately earned its keep, twice, on this month's
own fixes.** Both were operations that had not been updated to match a handler
that had:

- `x/paymsg` sent a payment from a customer the participant had only *claimed*,
  which M-3 now refuses until the account confirms. Fixed by filtering the
  debtor pool to confirmed customers and adding a `MsgConfirmParticipant`
  operation — without one the simulated chain accumulates claims no payment can
  ever use, and the payment path stops being exercised at all.
- `x/amm` submitted a swap too small to buy a single unit, which L-2 now refuses
  rather than settling for nothing. The operation now runs the handler's own
  arithmetic, fee and truncation included, and declines instead.

Neither was a defect in the fix. Both were the simulation doing the job the
audit said it would.

**A test that expired.** `TestRecoveryOverHTTPHoldsPaymentsAfterwards` in
`tools/custodian` had been failing since **2026-09-04** with nobody noticing.
The fixture injects a fixed clock into the recoveries store, so `EligibleAt` was
stamped 2026-09-04; the HTTP handlers called `nowUTC()` and read wall time. The
assertion about the 72-hour notice period therefore held only while real time
was behind the fixture's date plus 72 hours, and then quietly began asserting
the approvals rule instead. Two clocks where there should be one. The server now
carries the clock and the fixture shares it — production behaviour unchanged,
since nil still means wall time.

**Dependencies: ten reachable vulnerabilities, seven now closed.** `govulncheck`
is already wired into `make test`, which is how ten went unnoticed — there is no
`make` on the development machine. Fixed by upgrading `google.golang.org/grpc`
(xDS RBAC and HTTP/2), `github.com/hashicorp/go-getter` (arbitrary file reads),
`github.com/ulikunitz/xz` (memory leak on corrupted archives) and the
`aws-sdk-go-v2` modules the grpc bump pulled forward.

What remains, and neither is fixable in this repository:

- **Six are the Go standard library**, all fixed in **Go 1.26.6**; the toolchain
  here is 1.26.5. `net/url`, `html/template`, `crypto/tls`, `net/http` (×2) and
  `encoding/asn1`. A toolchain upgrade, not a code change.
- **One has no fix at all**: `golang.org/x/crypto`'s `openpgp` package is
  unmaintained and reachable through the Cosmos SDK's own keyring armour
  (`crypto.EncryptArmorPrivKey`). Upstream, and an accepted risk until the SDK
  moves off it.

**Fuzzing: the repository had no targets.** Three now, over the inputs an
attacker actually chooses, run by `make test-fuzz`:

| Target | Surface | Result |
|---|---|---|
| `FuzzPermit` | the public RPC gate's method filter | 2.2M execs, no crash, no bypass |
| `FuzzMethodsOf` | its JSON-RPC parser, raw network bytes | 1.4M execs, clean |
| `FuzzValidateMsgTypeURL` | the permissionless store key from H-6 | 0.5M execs, clean |

They assert the boundary, not just the absence of a crash: nothing outside the
allow-list may be permitted, and nothing that survives validation may be a store
key the module cannot live with. A fuzzer that only looked for panics would be
satisfied by a gate that returned true for everything.

### The reference had been publishing summaries instead of reasoning, 2026-09-06

`buf lint` wants a comment on every rpc. Writing them was meant to be
housekeeping. Regenerating `docs/reference` afterwards deleted documentation
across fourteen modules.

A message is documented twice — once on the rpc that carries it and once on the
message itself — and `docgen` published `firstNonEmpty(rpcDoc, msgDoc)`. The
rpc comment won. That had been invisible for as long as the tool has existed,
because most rpcs had no comment and the message comment came through by
default. Giving every rpc the one-liner the linter asks for turned that default
off, and each new one-line service-index entry replaced the paragraphs written
on the message it names. `x/treasury`'s escrow lost *"The buyer confirming is
the whole condition. Nobody else can confirm on their behalf — not the seller,
not the moderator, not the treasury admin."* and got *"ReleaseEscrow pays the
beneficiary"* in its place.

**Reversing the preference is not the fix, and trying it is how the size of the
problem became clear.** Preferring the message comment deleted **138 sentences**
of rpc comment instead — `x/enforcement`'s sweep being permissionless and
repeatable, `x/validatorgov`'s recovery being gated by the same account that
admits validators, `x/land`'s authorisation being what `x/tokenisation` refuses
without. Both comments are written by hand and both say things the other does
not. Choosing between them is the bug.

`mergeDocs` now publishes both: the message comment leads, and what the rpc line
adds follows it. What the rpc line restates is dropped, matched on the words
carrying the meaning rather than on the wording, because one fact written twice
by one hand is written two ways. An rpc comment that is a second telling of the
whole message comment is dropped entire rather than trimmed to the sentences
that happen not to match — that produces prose beginning mid-argument.
Scaffolding on either side ("CreatePool defines the CreatePool RPC.",
"MsgApproveBuilder is the Msg/ApproveBuilder request type.") is not
documentation and no longer displaces the side that is. A message left with
nothing now prints nothing, which at least makes an undocumented message look
undocumented.

**A second defect surfaced underneath it.** Once the message comments were
reaching the page, `cleanComment` was seen rejoining every wrapped line with a
space — right for prose, wrong for the bullets, numbered items and headings
inside it. Three numbered consequences in `x/alias`'s MsgGrantRole arrived as
one run-on paragraph, and a `#` section marker inside a comment rendered as an
h1 outranking the page it sat on. Block-level Markdown now keeps the line it was
written on, and headings are demoted below the `###` a message is printed under.

Verified by regenerating and diffing every `###` section against its committed
version, sentence by sentence: **no sentence the reference used to publish has
stopped being published**, excepting one piece of framing ("Three failure modes
went with the parameter") whose three facts are all still there. `x/alias`'s
UpdateParams was the one message documented twice at essay length; the two facts
that existed only in the rpc telling were moved onto the message, which is where
this repository's reasoning is supposed to live. `tools/docgen/merge_test.go`
pins the six cases the merge has to get right.

The reference is now **276 lines longer** than before the lint cleanup
began — not because anything was written for it, but because it had never been
printing what was already there.

### The gate and the headers are actually on the hosts now, 2026-09-06

Both had been written, committed and copied to both hosts for a day, and
neither was doing anything. A snippet nobody includes is a file on a disk, and
a binary nobody installed is a directory in the repository. Three things had to
be wrong at once for that to be invisible, and all three were.

**The RPC method gate is live on both hosts.** `tools/rpcgate` is built for
each architecture, installed at `/opt/yamale/bin/rpcgate`, running under
systemd on `127.0.0.1:26659`, and `/api/rpc/` on both hosts now points at it
instead of at the node. The path deny-list it replaces has been removed rather
than left beside it: a rule that never blocked anything is worse than no rule,
because it reads as the protection.

Measured on the public funnel, before and after:

| POST method | before | after |
|---|---|---|
| `net_info` | full peer map | refused |
| `dump_consensus_state` | answered | refused |
| `consensus_state` | answered | refused |
| `unconfirmed_txs` | answered | refused |
| `broadcast_tx_commit` | answered | refused |

All twelve methods the consoles actually call — `abci_info`, `abci_query`,
`block`, `blockchain`, `broadcast_tx_async`, `broadcast_tx_sync`, `commit`,
`health`, `status`, `tx`, `tx_search`, `validators` — still reach the node.
Checked by distinguishing the gate's `-32601` from the node's own errors, since
a method that fails for its own reasons looks identical to a blocked one if you
only test for the absence of a result.

**The committed systemd unit could not have started on either host.** It named
`User=yamale`, and there is no `yamale` user on either machine; it depended on
`yamale-node.service`, and the node here is `yamale-devnet.service`. The service
account now comes from a per-host drop-in, which is what the faucet, feeder and
custodian units already do — the two hosts run these under different logins and
there is no shared account to name.

**The security headers are live, and the obvious install would have been
half-wrong.** `add_header` REPLACES rather than merges: a location that sets one
header drops every header the server block set. The Pi includes
`yamale-cache.conf`, which sets `Cache-Control` in an exact `location = /` and
in two regexes covering every bundle and every `.html`. Including the headers at
server level would therefore have covered `/keys/` and `/explorer/` and covered
neither the home page nor a single script on the site — and `deploy.sh` probed
`/keys/`, so it would have reported the install as done. The cache snippet now
carries the headers into each of its three locations, and the check probes three
URL shapes: a path with no location of its own, the exact-match location, and a
hashed bundle.

The VM had been serving these headers since 2026-09-05 and the Pi had not, from
any block at all — which is the same trap as every other entry here, in that the
host being checked was not the host the public reaches.

**One aside, from getting it wrong once.** The first attempt backed each site
file up as `<file>.before-headers` *inside* `sites-enabled/`. nginx includes
`sites-enabled/*` with no extension filter, so the backup became a second live
config and `nginx -t` failed on a duplicate default server — from a file whose
entire purpose was to not be used. Backups now go to `sites-retired/`, which is where this host already kept the
`.bak` files that caused the same failure the first time.
`nginx -t` caught it before any reload, which is the only reason this is an
anecdote rather than an outage.

**Still open, and unchanged by this:** `abci_query` reaches modules that are
closed on REST, because the gate filters the method and not the query path
inside it. That is a disclosure decision rather than a bug in the gate, and
`deploy.sh` reports it on every run so that it changes deliberately instead of
drifting.

### The offensive assessment, 2026-09-05

A red-team follow-on to the audit approached the same target as an adversary —
not "is each handler correct" but "what do I walk away with" — and found one
thing the module-by-module reads could not: a defect that lives in the seam
between two modules that are each correct alone. Three kill chains.

**K-1 — pool poisoning by realisation (CRITICAL, was new, now closed in the
tree).** This one was ours to own. An AMM pool pays both reserve legs in a
single `SendCoins`; the tokenisation send restriction halts every transfer of a
realised vehicle's fraction token that does not touch the tokenisation module
account; a pool is a different module account. So: pool a fraction denom against
YML, let honest LPs add liquidity, then realise the vehicle — and the exit
refuses the fraction leg, takes the YML leg down with it, and every LP's
counter-asset is locked in the pool forever. Not stolen, destroyed. A malicious
sponsor uses it as a rug; an honest one arms it the first time a pooled vehicle
is sold.

The blind spot was a comment this repository wrote on 2026-09-05, defending the
AMM's permissionlessness under M-11: it read the tokenisation restriction as
something that "sits there" stopping laundering on the way in. On the way out it
is the trap. The reviewer quoted the comment back as the exact place the
reasoning went wrong, and they were right.

Closed by refusing a fraction denom at `CreatePool` — the one point the pool
does not yet exist. Exempting the AMM account from the halt instead would let a
realised token trade at the pool's stale price, which is the free lunch the
restriction exists to prevent, so the trap and the thing the restriction guards
are two ends of one rope. The AMM learns which denoms are fraction denoms
through a read-only classifier the tokenisation module implements, wired after
the graph is built the way x/enforcement receives its concentration check.
Proven both ways: `TestAuditRealisedFractionLocksThePoolsCounterAsset` drives
the full trap on the unwired AMM with the real bank and the real restriction and
shows the LP's YML trapped; `TestTheFixRefusesToPoolAFractionDenom` shows the
same setup refused at creation. A source-level test pins the wiring line, because
the send restriction it sits beside was itself dead and unregistered for weeks.

**No trap is armed on the live chain today.** The two live pools are
`uyml/ungn` and `uyml/uzar` — fiat pairs, not fraction denoms — and both
fractionalised vehicles (`tok/2/KIN518`, `tok/3/KIN777`) are still
`STATUS_ACTIVE`. K-1 is a latent capability that the shipped binary permits, not
a current loss. It becomes a live loss the first time anyone pools a fraction
denom, which is a permissionless action requiring no privilege.

**K-2 — the mempool is a public feed (HIGH, live-verified).** This is the RPC
gate finding [[see above]] read from the attacker's chair: `unconfirmed_txs`
answering over POST is not a hardening gap, it is a data source. On a chain
whose pitch is confidential payments, the mempool is the deanonymisation oracle
the on-chain salted hashes were meant to prevent — `MsgSendPayment` carries
debtor, creditor and participants in cleartext in the envelope. Add the no-op
mempool's first-come ordering and it is also a live MEV feed against the two
pools, and a liveness beacon on the franchise key, which broadcasts on a
sixty-second timer. The reviewer pulled a live `MsgSubmitExchangeRates` from the
feeder key this way, read-only, one request.

`tools/rpcgate` closes the disclosure. It does **not** close the MEV: on a
two-validator FIFO chain, ordering is not adversary-resistant regardless of who
can read the pool, and if value ever routes through the AMM that is a design
question rather than a config one. Chained with the `abci_query` raw-store reads
still open, the disclosure goes further than the mempool: `/store/alias/...`
exposes the jurisdiction-to-address map directly — who-banks-where plus the
payment graph, from two unauthenticated endpoints.

**K-3 — no ceiling on work (MEDIUM, live-verified).** `max_gas = -1` is the
backstop missing behind every unbounded-work finding. Per-transaction gas still
charges the attacker, so it is an amplifier rather than a free halt — but it is
what lets a slow permissionless loop (the netting end blocker before M-7, a
spray of 1-unit fraction transfers each writing a permanent position row) cross
from "late block" into "halted chain" on a two-node set that must both stay up.

Set to a finite `100000000` in `scripts/testnet/canonical-genesis.json` and in
`init-devnet.sh`, so no chain built from the tooling launches uncapped. The
**live chain is still `-1`** and needs a consensus-parameter change — the single
cheapest defense in either report, and it backstops M-7 and the state-growth
vectors at once. See [audit-remediation.md](audit-remediation.md).

**What the assessment attacked and could not break**, worth recording because a
report that only lists wins is marketing: the first-depositor/donation inflation
attack (the pool module is a blocked account and `JoinPool` reads reserves from
state, not bank balance); the per-swap rounding leak (every division truncates
toward the pool); permissionless chain-halt via arithmetic (every divisor
guarded at the point of use); send-restriction re-entrancy (`Settle` makes no
bank call); and self-service alias/role escalation (`AssertScope` fails closed
on a missing registry). These are the controls the first audit credited under
"what is right", confirmed from the other side of the glass.

### Two the audit did not find, from its own reply

The auditor checked this response rather than accepting it, and turned up two
things neither side had. Both verified against the funnel on **2026-09-05**.

**The RPC deny list has never blocked anything.** Not "the proposed fix would
not have worked" — the rule that was already there does not work either. It is
an nginx location regex, so it matches the URL path, and CometBFT takes the
method in the POST body:

| Method | `GET /api/rpc/<m>` | `POST /api/rpc/` with `{"method":"<m>"}` |
|---|---|---|
| `net_info` | 403 | peer node id, moniker, listen addr, remote ip |
| `dump_consensus_state` | 403 | full round state |
| `unconfirmed_txs` | 403 | the mempool |
| `broadcast_tx_commit` | 403 | accepted, holds the connection |

So every argument written into that rule — the mempool being everything needed
to front-run a payment, `dump_consensus_state` serialising unbounded state per
call, `broadcast_tx_commit` exhausting the connection pool — has been true and
unmitigated for as long as the rule has existed.

One correction in the other direction, because the severity should be right:
the remote address `net_info` discloses is `100.117.244.96`, which is Tailscale
CGNAT and not globally routable. The claim in the old comment that this is "the
validator's real network location, which is the one thing the tunnel exists to
hide" does not hold — what leaks is the tailnet topology, node ids and
monikers. The serious three are the denial-of-service and mempool methods, not
the address.

Closed in the tree by `tools/rpcgate`, an allow-list filter that parses the
JSON-RPC envelope and applies the same decision whichever way the method
arrives, including inside a batch. Eleven tests, of which 44 assertions fail
against a permissive gate.

**Deployed, verified 2026-09-13.** `POST /api/rpc/` with `{"method":"net_info"}`
and `{"method":"unconfirmed_txs"}` now both return *"not available on this
endpoint"* — the gate is on both hosts. This closes the disclosure only, not the
MEV, and not the `abci_query`-past-REST reads, which remain open (see "The gate
that stops at the wall").

**The governance electorate of this chain is an oracle feeder key.**

    tx_search message.action='/cosmos.gov.v1.MsgVote'
      -> 11 txs, 20 votes, heights 12,724-181,430, every one signed by
         yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg

    oracle/v1/feeder/ymlvaloper1m9xhc...  -> yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg
    keys show alice --keyring-backend test -> yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg

That address holds the only third-party delegation on the chain — 65,000 YML,
**37.16% of bonded stake** — has cast every governance vote ever cast, and is
`alice` in an **unencrypted keyring** on the VM, signing an oracle price
automatically every 60 seconds on a network-connected validator host.

This is H-4 with the asset swapped. H-4 flagged the *other* feeder key because
it administers a treasury holding 700 YML. This one carries the franchise that
approves issuers, appoints validators, creates collections and resolves
disputes. Whoever reads `/opt/yamale/node/keyring-test/` on the VM does not
need C-3's rewards to control the chain; they already have a plurality of the
vote and the only voter's key.

It also reframes C-3. The rewards make the thresholds *purchasable*; this makes
them **already bought**, by a key whose entire documented purpose is that it is
disposable.

Nothing in the tree fixes this. It is a rotation, and it is the one item on
this page that is neither a decision nor a delay — see step 1 of
[audit-remediation.md](audit-remediation.md).

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

**One claim in the audit did not survive being checked.** It reports that most
of the second validator's rewards accrue to the `validatorgov` module account,
which has no key to sign a withdrawal, and are therefore permanently
unreachable. On 2026-09-05 that module account
(`yml1rrxxm2cz5ch90srswv5el7y95exvmj3lfxc44u`) holds **no delegation and no
rewards at all**. Nothing is stranded there, which means the figure is worse
than the audit's, not better: every uyml below is reachable by a key somebody
holds.

The whole of it, read on 2026-09-05:

| Holder | Delegator rewards | Commission | Total |
|---|---:|---:|---:|
| `yml1m9xhc…htmqms` — operator of `pi`, 100,000 YML bonded | 799,146,452.40 | 88,794,050.27 | **887,940,502.67** |
| `yml1cgguvt0…v5see4` — operator of `pi-2`, 74,900 YML bonded | 79,913,548.25 | 8,879,283.18 | **88,792,831.43** |
| `yml1rxtapck…wa6szg` — 65,000 YML delegated to `pi-2` | 0.37 | — | **0.37** |
| | | | **976,733,334.47** |

Against a total supply of 997,766,566.66 YML that is **97.9% of every token on
the chain, held by two operator keys**, and 89.0% of it by one. The 65,000 YML
delegation that carried all eight governance votes has earned 0.37 YML, because
it was made after emission had already truncated to zero — so the votes were
cast by an account with none of the money.

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

- ~~**The ombudsman is unset.**~~ **Closed 2026-09-13.** `enforcement/v1/params`
  now names `yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr` (a
  foundation group account) as ombudsman, so `MsgOmbudsmanVeto` — the only
  message that can stop a seizure that has passed and is waiting out its delay —
  has a holder.

- ~~**The rolling seizure cap covers one denomination out of forty-eight.**~~
  **Closed 2026-09-13.** `seizure_window_cap` now lists all 48 denoms —
  `amm/pool/*` and `tok/*` at lower caps, the currencies at 500 YML — so a
  seizure is no longer bounded by `max_seizures_per_window` alone for anything
  but `uyml`.

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
  given what it handles. It is **no longer served without a CSP** (headers
  deployed 2026-09-05/09-06, re-confirmed 2026-09-13) — but the deployed policy
  allows `script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'`, and
  `'unsafe-inline'` on the one page that generates key material is exactly the
  gap a dedicated review should close.

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
- **USDC and EURC are LIVE on the faucet (2026-09-30), after three gates.** Each
  was found only after the one before it was cleared. (1) No application had
  ever been filed: `register-currency` files an application rather than
  creating a currency, and the script that should have filed these two sent its
  output to `/dev/null`. (2) The foundation was not their issuer: proposal 24.
  (3) The supply ceiling was zero -- the default is 0 and only `uusd` had its
  own: proposal 26 set 10^15 each (25 had expired unvoted). Then
  `scripts/currencies/onchain.sh` minted 10^13 of each and opened pools 16
  (uyml/uusdc) and 17 (uyml/ueurc), exactly the four transactions its dry run
  predicted. Verified by claiming both through the public funnel: HTTP 200,
  1,000 of each on the account. The faucet's "retrying will not help" message
  (committed 2026-09-23) was deployed the same day; the binary it replaced dated
  from 2026-08-17 and is kept as `yamale-faucet.prev-20260817`.
- **pi-2 was jailed by power cuts on the Pi, and is unjailed but not fixed.**
  Jailed at 528648 (2026-09-26 07:44 UTC) after the Pi lost power mid-block;
  six boots in three days end with no shutdown, one logged 12 under-voltage
  lines, and `vcgencmd get_throttled` read `0x50000`. Its third downtime
  jailing (tokens/shares = 0.99^3). Unjailed 2026-09-30 at 601220 and signing
  again. **Supply replaced 2026-09-30 ~15:10 CEST**; the swap cost 4 missed
  blocks and no jailing, and the Pi read `throttled=0x0` with no under-voltage
  ten minutes after boot. Not yet proven: the flags are cumulative per boot, so
  re-check `vcgencmd get_throttled` after a few days under load. While pi-2 was
  out, only `pi-operator` could carry a vote.
- **Every currency now has a pool (2026-09-30): 44 pools, one per currency, no
  duplicates.** The currency script had never opened most of the pools it
  claimed to skip -- its "already pooled" check ran `query amm pool`, which is
  not a command -- and it could not fund them: the foundation held about 798,000
  of most currencies and a 20,000-YML pool needs up to 172 million. Proposal 27
  set a mint ceiling for all 44 (one billion dollars' worth at the indicative
  rate; `uusd` kept, EURC moved to 920 million) and moved 600,000 YML from the
  community pool to the foundation. The rewritten script then made 44 top-up
  mints and opened 27 pools with no failures, exactly its dry run. Proven by a
  GNF claim through the public faucet and a swap of it in pool 32: 860 GNF in,
  0.099699 YML out -- 0.1 at 8,600 less the 0.3% fee.
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
