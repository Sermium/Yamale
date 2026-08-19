# Yamale — Revised Scope Specification and Path Forward

*Authoritative, recorded as issued. See [README](README.md) for what it
supersedes and [gaps.md](gaps.md) for what the tree does not yet meet.*

## 1. Purpose and status of this document

This supersedes the enhancement specification issued earlier in this discussion. It reflects two decisions taken since: that payments and land are separate deployments sharing only infrastructure, and that validator admission remains a governance vote rather than being replaced by automated constraints. Both are adopted here, with one amendment to the second.

The assessment underlying it is based on the public documentation only — the guides and the generated reference for the nine modules. No source code was reviewed, so nothing here speaks to implementation quality. The chain remains pre-testnet, unaudited, and undeployed by the project's own account, and that framing should be preserved in every external communication, because against an incumbent with African Union backing, candour is the only asset a small team can offer that a large one cannot.

## 2. The decision that reframes everything: platform, not network

Separating payments from land forces a conclusion worth stating plainly, because it changes the business rather than just the architecture. Land is the most sovereign asset class there is; no state will place its cadastre on a ledger validated by other states, and none should be asked to. Payments are inherently multilateral. The two therefore cannot share a validator set, which means the shared component is the stack, not the chain.

Yamale is consequently defined here as a distribution — a permissioned financial-ledger platform, shipped as profiles, deployed by others. Revenue is licensing, integration and support. There is no network for Yamale to operate, no token economics to defend, and no continental rail to be granted permission to build. This dissolves the single hardest problem in the previous framing, which was that proposing a parallel continental rail asks central banks to defect from an AU directive. A vendor does not compete with PAPSS. A vendor sells to whoever needs to run a ledger, PAPSS included, and the PACM build with Interstellar is direct evidence that Afreximbank buys blockchain infrastructure from vendors.

It also imposes obligations the project does not yet have: long-term support branches, a versioning and backport policy, upgrade tooling, certification of deployments, and a legal entity capable of signing warranties. These are now in scope.

## 3. Product definition

The platform consists of a common core and three profiles built from it.

The core is consensus, the ante and permissioning chain, key management and HSM procedures, the constitutional and governance layer, the explorer and indexer, deployment and genesis ceremony tooling, and operational runbooks. This is the layer that gets audited once and amortised across every deployment, and it is where the efficiency argument for a shared stack actually lives.

The settlement profile targets a monetary union or an institutional consortium. It carries x/paymsg, x/stablecoin, x/treasury, x/oracle in its FX role, and governance. It has no native token: fees are denominated in the issued currency and routed to a treasury-governed operating account, with validators compensated by service contract. x/emission is compiled out.

The registry profile targets a national lands commission or equivalent. It carries the asset registry, the appointed-valuer oracle, and governance. It is a national deployment with a national validator set — the central bank, the lands commission, the judiciary, an auditor-general, a university. It shares no consensus with any settlement network.

A consortium profile retains the token and market modules for private-sector deployments where a native asset is appropriate. It is not offered to sovereign buyers.

The two sovereign profiles do not interoperate at launch, and that is a deliberate acceptance. Cross-chain collateral — land pledged against credit on a settlement network — requires IBC, which has never been exercised on this chain and is disabled at genesis for a well-documented reason. If that capability is ever required commercially, IBC moves onto the critical path with a full testing programme, a relayer, deliberate failure-case exercises, and an ante-equivalent authorisation check inside the interchain-accounts host path. Until then it stays excluded by build tag rather than by configuration, so it is absent from the binary and from audit scope.

## 4. Governance: admission votes plus codified caps

The position that a plural validator set will refuse to admit too many nodes from one state is accepted. Admission voting is retained as the mechanism for judging competence, licensing status and institutional fitness, because those require human judgement and cannot be automated. The amendment is that voting alone does not discharge the concentration risk, for reasons that are structural rather than a matter of confidence in the voters.

Admission is an event; concentration is a state. Nothing in an admission process triggers when a state-owned bank acquires a participant, when two members merge, when an operator is nationalised, or when beneficial ownership changes quietly. Power concentrates with no application to vote on. Separately, a set empowered to refuse entrants has a standing incentive to refuse them, since new members dilute both fee share and veto power, and cartel formation is harder to detect than capture because it presents as prudence. And the founding set is unelected by construction, so whatever bias exists at genesis propagates through every subsequent admission.

The decisive argument, though, is commercial rather than theoretical. Open accession and discretionary admission are in tension: if balance is maintained by refusing applications, the network must eventually tell a sovereign government no, and that refusal is a diplomatic act that will be read as exclusion. Arithmetic caps remove the need. With per-entity, per-beneficial-owner and per-jurisdiction ceilings set below the freeze-relevant threshold and enforced at every epoch, accession can stay open and depoliticised — nobody is refused, they are admitted at a bounded weight, and drift between votes is corrected automatically rather than requiring a confrontation.

Scope therefore includes a beneficial-ownership registry in x/validatorgov, continuous epoch-level enforcement with automatic power reduction on breach, and a constitutional layer holding the caps, the seizure threshold, the recovery destination and the enforcement delays as genesis-fixed invariants amendable only by supermajority plus a multi-week public delay. A chain that can vote to lower its own seizure threshold does not have one.

Enforcement oversight is retained from the previous specification and remains critical for sovereign sale: every seizure case must carry the hash and identifier of an external legal instrument, delays must scale with the amount, an ombudsman appointed outside the validator set holds a veto but never an initiation power, and aggregate seizures are capped in a rolling window. The existing fast-freeze design is sound and unchanged.

## 5. Positioning and claims

Three changes to how this is presented, each of which removes a claim that would end a meeting.

Remittance pricing comes out of the pitch. Sending two hundred dollars to Sub-Saharan Africa costs roughly 7.8 percent, and the settlement rail is a minority of that. The balance is FX spread, cash-out agent commissions, per-transaction compliance, dual-corridor licensing and pre-funded destination liquidity. Stellar and Ripple both made this promise and both discovered the corridor still costs six percent when the rail is free. Repeating it signals that the corridor economics were not done.

The PIX comparison comes out as a claim and stays only as an illustration. PIX succeeded because one central bank had statutory authority over every bank in one jurisdiction, mandated participation and set pricing. No continental equivalent of that authority exists, which is precisely why PAPSS uptake has been uneven across 28 countries despite AU backing.

The relationship to PAPSS is stated explicitly and early, in the first minutes of any conversation, framed as complementary infrastructure. The first question in any room with an African central banker will be how this differs, and a crisp, respectful answer is worth more than any architecture material.

What replaces those claims is the capability nobody else offers: programmable settlement. PAPSS moves money; it does not do conditional disbursement, escrow with automatic refund, vesting, or spend policies enforced by where funds physically sit. x/treasury — commitments that leave spendable balance entirely and cannot be redirected by any administrator, governance proposal or signer threshold — is the most differentiated component in the system and the easiest to sell, because donor disbursement, subsidy programmes, public payroll and trade-finance escrow are problems people will pay to solve today without any monetary-sovereignty argument.

The second differentiator, and one that runs against instinct, is capital-control compliance as a designed protocol feature: per-corridor limits, convertibility windows, programmable policy enforcement. States that ration foreign exchange do so deliberately, and a rail that makes value movement frictionless reads to their finance ministry as a capital-flight vector. Nobody has built this, and it converts the principal objection into a selling point.

## 6. Technical workstreams, in priority order

First, and before any deployment carries real balances, payment confidentiality. ISO 20022 fields carry personal data, and plaintext on an append-only ledger has no erasure path under Nigeria's NDPA, Ghana's DPA, POPIA or GDPR. Every participant also currently sees every competitor's payment graph. The fix is a commitment on-chain with the payload held off-chain under viewing keys for the two participants and the supervisor. This is a schema change now and effectively impossible later. Alongside it, the constitutional layer, since it is an authority change with the same property.

Second, the governance package above, the sovereign build profile with the token removed, and enforcement oversight. Together these answer the only question a central bank board actually asks, which is who can stop or take our money.

Third, the engineering that makes a pilot survivable: batch payment messages with per-item failure isolation, mempool lanes prioritising settlement over all other traffic, a published state-growth model with pruning, state sync, archival offload and a tested restore, an institution registry carrying LEI or BIC and supervisory authority, oracle hardening with an authority-signed rate for the sovereign profile and deviation circuit breakers, and an explicit tiered architecture in which participants run retail ledgers and the chain settles net positions and high-value items. CometBFT gives seconds to finality at low thousands of transactions per second, which is right for interbank and two orders of magnitude short of national retail; the answer is architectural honesty, not chasing throughput on the base layer.

Fourth, assurance, which gates everything commercial: two independent audits with non-overlapping scopes, property-based and fuzz testing on every value-moving path, written formal invariant specifications for treasury and enforcement produced before audit so reviewers have something to check against, a rehearsed key ceremony with HSM custody across two independent hosting providers, and a documented upgrade rollback and recovery objective. Audit reports published in full, including unresolved findings.

Running throughout, and commercially the actual critical path: the account service — authentication, threshold key custody, recovery with two approvers and a mandatory delay, second factor — which the project's own documentation correctly identifies as the larger half of the remaining product; USSD and feature-phone access, without which most African transaction volume is unreachable; agent-network and mobile-money integration; and formation of a legal entity able to sign an indemnity.

## 7. Sequencing

Lead with one monetary union rather than the continent. UEMOA under BCEAO across eight states, or CEMAC under BEAC across six, gives one currency, one monetary authority, one regulator and a naturally plural validator set because member states do not answer to one another. In parallel, open a partnership conversation with Afreximbank on the infrastructure position, since that is a single counterparty rather than fifty-four and the precedent already exists. Land is pursued as a separate national sale to a lands commission, timed after the first settlement deployment, and never presented in the same document as payments.

A continental claim is made only after one live deployment exists. Leading with all of Africa without a single production network inverts the sequence and invites the comparison that cannot be won.

## 8. Open decisions requiring an answer before build

Four things cannot be resolved from the outside and determine scope. Which beachhead is pursued first, since UEMOA and an Afreximbank partnership imply different roadmaps and different first customers. Whether cross-chain collateral between registry and settlement networks is a commercial requirement, since that alone decides whether IBC is on the critical path. Whether threshold key custody is built in-house or procured from a vendor such as Turnkey or Privy, which the account design note correctly names as the largest open question in the product. And whether the intent is genuinely to become a software vendor with support obligations, or to remain a design and reference implementation that a systems integrator productises — a legitimate choice, and a materially smaller undertaking.

## 9. Residual risk

Nothing above closes three exposures. Adoption in this category fails on trust and value proposition rather than architecture, as the eNaira demonstrated with technically adequate infrastructure and sub-one-percent uptake. The incumbent has an AU mandate, twenty-eight countries and a live blockchain marketplace. And the seizure capability, however well constrained in code, may appeal to a buyer for precisely the reasons that make it dangerous, since no protocol constraint binds a government that can also change the law.

The engineering remains well above the threshold that justifies serious investment. The scope defined here is narrower than the original ambition in every dimension except one — it addresses more of the continent, by not trying to run it.

---

## Decisions taken since issue

**Recovery destination (§4).** Seized assets go to the **foundation address** —
the foundation being the trust body that administers the deployment. It holds
them so that they can be restituted to the parties they were taken from, rather
than being extinguished or landing anywhere a case could be brought to benefit.
This makes the empty `recovery_destination` found on the devnet a defect to fix,
not a question still open.

**Jurisdictional perimeter (§3, §4).** Profiles are not only a build question.
Stakeholders hold roles that are bounded by country: a national authority may act
on its own jurisdiction's accounts and records and on no others. The perimeter
must therefore be a first-class, enforced property of an account rather than a
convention in a user interface. See [roles-and-perimeter.md](roles-and-perimeter.md).

**Validator power (§3, §4).** Power is **equal seats**: every admitted validator
carries the same weight, because admission is already a governance vote and
stake-weighting would add a second, money-based channel nobody intended. In a
monetary union it would also hand the whole set to the central bank, which is the
one institution able to bond unlimited domestic currency — destroying the plural
validator set that is the sovereign pitch.

Governance may vote an individual validator's power where there is reason to, but
**a governance-set power is still bound by the concentration caps**. A proposal
that raises a validator above a ceiling is trimmed by the same epoch check that
catches growth and mergers; otherwise the constitutional layer is decorative.

This makes the caps countable — "no jurisdiction holds more than N of M seats" —
rather than arithmetic over balances that drifts every block, which is what a
supervisor can actually audit.

Two thresholds are computed from voting power today and become seat counts:
`x/oracle`'s rate agreement and `x/enforcement`'s two-thirds. Nothing changes on
the running devnet: validator 1 keeps its current dominant power and
`threshold_bps` stays at 6667. Equal seats is how a new deployment's genesis is
built, not a retro-change to a live chain.
