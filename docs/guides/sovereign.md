# A chain a country operates

**Status: framing, not built.** The intended deployment: an African state — a
central bank or a designated authority — runs the network, issues the currency,
and custodies consumer accounts. Citizens use it as an everyday payment system
and never see the chain.

That changes the design questions. Not the code, mostly — the code already fits.
What changes is which decisions are political rather than technical, and which
defaults become dangerous in different hands.

---

## What already fits

The chain was built permissioned, and that turns out to be the right shape here
rather than a compromise:

- **One approved issuer per currency** (`x/stablecoin`) is exactly a central bank
  issuing its own money. The permission is a governance record on the chain, not
  a configuration file — an auditor can see who was authorised and when.
- **Approved participants** (`x/paymsg`) is a licensed-institutions register:
  banks, mobile money operators, microfinance. Payments carry ISO 20022
  references, which is what supervisors and reconciliation already speak.
- **Fee sponsorship** (`x/feegrant`) means a citizen holding only local currency
  can transact without holding the network token. On a system meant for
  financial inclusion this is not a convenience; it is the difference between
  usable and not.
- **The 42 African currencies** are already seeded and priced.

---

## The decision that matters most: who holds the validators

A chain whose validators are all run by one ministry is not a network. It is a
database with extra steps, and it inherits none of the properties that make a
ledger worth using — no independent verification, no protection against a
single administrator, nothing an opposition or an auditor can check.

If the answer is "the state runs all of them", be honest that a replicated
database would be cheaper and simpler. The chain earns its place only when the
validator set is **plural**: the central bank, several licensed banks, a
telecommunications regulator, a university, an auditor-general's office. Then
"two thirds must agree" means something.

**The regional case is the strongest one.** XOF and XAF are each used by eight
and six countries, issued by BCEAO and BEAC. A shared chain across a monetary
union is a genuinely better fit than a national one: the validator set is
naturally multi-party because the members do not answer to one another, and
cross-border settlement inside the union stops being a correspondent-banking
problem. That is the deployment to lead with.

---

## The position

Stated plainly, because the rest of this document is the reasoning behind it.

**Why decentralised rather than a PIX.** PIX works, at enormous scale, and it
cannot be regional infrastructure: it is operated by Brazil, for Brazil. A
neighbouring state can accept the method but cannot build sovereignty on a
system another government runs. Cross-border is the argument, and it is
decisive — a shared ledger between states only earns its complexity because no
single state may own it.

**Why a shared chain for a shared currency.** XOF is one currency across eight
states with one monetary authority. Splitting it across eight ledgers and
reconciling it back would recreate the correspondent-banking problem the project
exists to remove. Where the currency is already shared, the ledger should be.
National currencies stay on national chains, joined by IBC.

**Why the validator set answers the control objection.** Eight states with equal
power means no state controls anything: three would have to combine to halt, six
to change consensus. And the set is not restricted to states. Licensed banks,
mobile money operators, a telecommunications regulator, a university, an
auditor-general's office — every non-state validator pushes those thresholds
further out of reach of any political bloc. **Dilution is the mechanism, and it
is deliberate.**

The honest residuals, stated rather than discovered:

- With eight equal validators, **three can halt** and **six can change or
  seize**. Adding non-state validators raises both bars; the numbers should be
  recomputed for the actual set and published.
- **An exit procedure should exist before anyone needs it.** A member's citizens
  hold balances in shared state; how that sub-state is exported, settled and
  detached is far easier to agree while nobody wants to leave.

This is a bet on the region's trajectory rather than its history. That is a
legitimate bet to make, and it should be made explicitly — the architecture
assumes members who disagree without sabotaging, and says so.

## Sovereignty and a common core pull against each other

The stated goal is both: each country controls its own traffic and its own
frontier, **and** the core is shared so neighbours interoperate. Those are the
right goals and they cannot both be maximised on one chain. The shape of the
compromise is the most consequential decision in the whole design.

### "Validators specialised in one currency" is not possible on a single chain

This needs correcting before it gets built on. In CometBFT every validator
executes **every** transaction and votes on **every** block. There is no way to
have a validator that only validates XOF transfers: consensus is over the whole
state machine, not over a subset of it. A validator that ignored some
transactions would simply compute a different app hash and be slashed or halt.

So on one shared chain:

- Every country's validators validate every other country's payments.
- **One third of the total voting power can halt the entire union**, including
  the domestic payments of countries that had nothing to do with the dispute.
- A country cannot upgrade, pause, or change a parameter for itself. Every
  change is a change for everyone.

That is not sovereignty. It is a shared central system with the word blockchain
attached.

### The architecture that gives both

Cosmos was designed for exactly this question, and the answer is **one chain per
country, connected by IBC**:

- Each country runs its own chain with its own validator set, its own
  parameters, its own upgrade schedule. It can halt, fork, or change rules
  without asking anyone. That is the sovereignty in point 1, and it is real
  rather than nominal.
- Currencies move between chains over **IBC** — the transfer is verified by
  light clients on both sides, with no bridge operator and no multisig to trust.
  That is the interoperability in point 2, and it is stronger than a shared
  chain because it survives one member misbehaving.
- The "common core" becomes the **shared codebase, shared standards and shared
  governance of the specification** — every country runs the same software, the
  same ISO 20022 message shapes, the same identity format. Not one shared
  ledger.
- A monetary union like BCEAO can additionally run **one chain for its shared
  currency**, with the member states as its validator set. XOF belongs on a
  union chain because the currency itself is union-wide; national currencies do
  not.

The cost is honest: IBC is real work, it has never been exercised on this
codebase (see [ibc.md](ibc.md) — the surface is reviewed, nothing has been
relayed), and a relayer has to be operated. That is the price of the sovereignty
being genuine.

**Decided: lead with the monetary union.** BCEAO first — one chain for XOF with
the eight member states as its validator set, joined later by licensed banks and
other non-state validators. National chains for national currencies come after,
connected by IBC, once the union chain has proved the model with a real
validator set rather than an argument about one.

Leading with the union rather than a single country is what makes the
sovereignty claim survive its first serious question. A one-country deployment
has to answer "so the ministry controls it?" with a promise. A union deployment
answers it with arithmetic.

**Recommendation: one chain per country, one chain per monetary union, IBC
between them, shared code and standards as the "core".** Present it that way and
the sovereignty claim survives scrutiny from a finance ministry that asks who
can stop their payments.

## On replacing the incumbents rather than joining them

Point 4 — a government-sponsored system that supplants mobile money and card
networks rather than complementing them — is the strategy with the clearest
precedent, and it is not encouraging.

**Nigeria's eNaira** is the closest comparison: a central-bank digital currency,
government-backed, launched 2021. After a year it had been used by well under
one percent of the population, while cash and existing mobile money continued
untouched. The technology worked. Adoption did not follow the mandate.

What that suggests, and it is worth planning around rather than arguing with:

- **A mandate creates availability, not usage.** People move money where the
  people they pay already are.
- **Distribution is the incumbents.** M-Pesa, MTN MoMo, Orange Money and Wave
  own the agent networks — the human being in the village who turns cash into
  balance. Replacing them means rebuilding that network; joining them means
  inheriting it.
- **Merchant acceptance decides consumer adoption**, and merchants adopt what
  their customers already carry. Outside the card networks, acceptance has to be
  built from zero.

None of this argues against the goal. It argues for sequencing: **be
interoperable first and dominant later**. A system that can receive from M-Pesa
on day one starts with users; one that asks people to leave M-Pesa starts with
none. The `x/paymsg` participant model already lets a mobile money operator join
as an approved participant rather than be excluded — that door should stay open
even if the long-term intent is to replace them.

## "Public ledger, no risk of tampering" — two corrections

Both halves need qualifying, and the second one matters a great deal for this
deployment.

**Tamper-evident, not tamper-proof.** History cannot be rewritten *unnoticed* —
but two thirds of voting power can rewrite it by consensus, and the property
holds only in proportion to how independent the validators are. If one ministry
runs two thirds, the guarantee is a promise, not mathematics. This is the same
point as the validator set above, arriving from a different direction: the
integrity claim and the sovereignty claim both rest on plurality.

**"Public" is a stronger word than it sounds for a national payment system.** A
public ledger means every payment every citizen makes is permanently visible to
anyone, forever, and correlatable. That is a mass-surveillance property, and it
is the opposite of what a bank statement is today.

It also sits directly against the identity design: [accounts.md](accounts.md)
keeps names and identity numbers off-chain precisely so the ledger cannot be
resolved to people. That firewall holds only while it holds. Anything that
publishes the mapping — a national ID on-chain, a public directory, a leaked
custodian database — retroactively deanonymises the entire history, and nothing
can be deleted afterwards.

### Decided: split the visibility by whose money it is

The trust argument comes almost entirely from **public money**, not from
citizens' payments. So the two are separated:

| | Visibility |
|---|---|
| Treasury spends, procurement, disbursements | **Public.** Anyone, no account, forever |
| Issuance and redemption of the currency | **Public** — how much exists, and who authorised it |
| Validator governance, votes, parameter changes | **Public** |
| Enforcement cases, evidence, every vote cast | **Public** |
| Citizen-to-citizen payments | **Not world-readable.** Supervisors, auditors and courts, on authenticated access |

That keeps the entire anti-corruption argument — *where did the money go* is
answerable by any citizen with a browser — and drops the mass-surveillance
property. It is also a far better answer when someone asks whether an opposition
politician's transactions can be traced by whoever governs next.

**What this actually means technically, stated honestly.** Every validator
executes every transaction, so **every validator sees everything**. There is no
cryptographic privacy here and this document should not imply one. "Not
world-readable" means:

1. The **public explorer and public API** expose government flows, supply,
   governance and enforcement in full, and do not expose citizen payment detail.
2. Supervisors, auditors and courts reach the rest through **authenticated
   access**, and those requests are logged.
3. The confidentiality guarantee is therefore **the composition of the validator
   set plus access control** — not mathematics. It is the same guarantee a
   national payment system already gives, and it is worth saying so rather than
   overclaiming.

If genuine cryptographic confidentiality is ever required, that is encrypted
amounts and parties on-chain — a different and much larger project, and one that
trades away the auditability this design is being sold on.

**Not yet built.** The explorer and REST proxy currently expose everything to
anyone. Implementing this is an authorisation layer in front of the API and a
split in the explorer between a public view and an authenticated one.

## `x/enforcement` in state hands is a different tool

This is the part that needs saying plainly rather than being left as an
implementation detail.

The module lets a validator freeze an account in one block and a two-thirds
validator vote seize its balance to a governance-set address. It was designed
against theft: a scam drains a wallet in minutes and a vote takes hours, so
freezing has to be fast and taking has to be slow.

**In the hands of a government, the same mechanism is asset forfeiture without a
court.** Everything that made it a good anti-fraud tool — speed, finality,
irreversibility — makes it a good instrument of coercion. A political opponent's
account, a journalist's, a protest fund's: the code cannot tell the difference,
and it was never designed to.

What the design already does, and why it matters more here:

- Every case is public, with its grounds and its evidence hash on the chain.
- Every validator's vote is recorded and attributed.
- Cases that failed stay on the record too, so a pattern of attempted seizures
  is visible even when none succeed.
- Seizure needs two thirds — which is only a protection if the validator set is
  plural. With one operator it is a formality.

What should be decided before any deployment:

- **Should seizure exist at all in this deployment?** Freezing pending a court
  order is a defensible power. Taking, by vote, may not be — and the module can
  ship with seizure disabled and freezing retained.
- **Should a judicial reference be mandatory** in the case record, so a seizure
  without one is visibly irregular rather than merely unusual?
- **Who holds the recovery destination**, and what is the process for returning
  funds when a case was wrong?

A country deploying this should be told what the power is, in these words,
rather than discovering it later. A vendor who does not raise it is selling
something they do not understand.

---

## Custody by the state: what changes

Model A (server-held keys) was chosen for consumer accounts. With a state as
custodian:

- **The licensing question dissolves** — the authority is the regulator. What
  replaces it is a governance question: what stops the custodian spending
  balances it holds, and who audits that? The solvency query in
  [custody](custody.md) becomes a public accountability tool, not an internal
  one.
- **Data residency is now a sovereignty requirement.** Citizen names, national
  identity numbers and transaction histories cannot sit in a foreign cloud
  region. The encrypted profile store from [accounts](accounts.md) has to run
  where the law requires, which constrains the whole architecture — and is a
  reason to keep the chain and the identity store separable.
- **Decided: link to the government's own population database, in one
  direction only.** The state already knows its people. Duplicating that onto a
  ledger would be redundant and unfixable, so the join is a key exchanged
  between the two systems — which is right. The direction is what matters:

  > The chain's user ID goes **into the government database as a column**.
  > The national ID never goes onto the chain.

  Same link, opposite direction, and the difference is total. `CAQ3-C04Z-M` on
  chain means nothing to anyone without the state's own records. A national ID
  on chain makes every payment permanently attributable to a named citizen — to
  anyone, forever, including whoever governs next — and cannot be deleted
  afterwards. `x/alias` was built to hold an address and nothing else precisely
  so this is the easy direction to take.

---

## What financial inclusion actually requires

The interface built so far assumes a smartphone and a browser. For the intended
population that is the minority case, and the gap is not cosmetic:

- **Feature phones and USSD.** A large share of mobile money in West and East
  Africa runs over USSD menus on phones with no data connection. A payment
  system that requires an app excludes exactly the people it claims to serve.
  This is a whole second interface, and it is the one that decides adoption.
- **Intermittent connectivity.** Settlement can be online; *acceptance* often
  cannot. Offline-capable payment with later settlement is a hard problem and a
  real requirement.
- **Tiered identity.** Many users have no formal identity document. A system
  that requires full KYC to open an account excludes them; one that requires
  none is a laundering vehicle. Tiered limits — small balances with light
  verification, higher limits with more — is the established answer and needs to
  be in the account model from the start.
- **Interoperability with existing mobile money.** M-Pesa, MTN MoMo, Orange
  Money and Wave already have the users. A new system that cannot receive from
  them starts with nobody. This is an integration and a commercial negotiation,
  not a protocol feature, and it is probably the single largest determinant of
  whether any of this is used.

---

## What to say when presenting it

Three things that are true and worth leading with:

1. **The permissioning is the product.** Every participant approved by vote,
   every issuer authorised on the record, every decision auditable — that is
   what a public blockchain cannot offer a finance ministry.
2. **Sovereignty is real here.** The state runs the validators, holds the keys,
   sets the parameters, and can leave. Nothing depends on a foreign company's
   continued goodwill.
3. **Settlement is seconds, and the reference travels with the money** — which
   is the actual daily cost of correspondent banking, not the headline fee.

And one that should be said before anyone asks:

> This is pre-testnet software with no independent security audit. The design is
> complete and much of it is running; none of it has been reviewed by anyone
> outside the project, and it should not hold public money until it has.
