# Land title deeds on Yamale

How a parcel of land becomes a record that cannot be owned twice, and how it
changes hands without anybody being able to buy the outcome.

**Status:** design. The module (`x/land`) is specified here and not yet
implemented. This document is the specification the implementation must satisfy.

## The problem this solves

Land registries in much of the world fail in four specific ways, and each one is
a design requirement rather than a vague aspiration:

1. **The same parcel is sold twice.** Two paper deeds exist for one field,
   issued years apart by offices that never spoke. Both buyers paid.
2. **A record is quietly altered.** A boundary moves, a name changes, and the
   only evidence of the original is a ledger controlled by whoever altered it.
3. **One official can be bought.** A single signature transfers a family's land.
   The cost of theft is the price of one bribe.
4. **The dispossessed cannot prove anything.** The person who lost the land has
   no copy, no timestamp, and no standing.

A blockchain does not fix corruption. What it can do is make the *cost of
corruption* legible and high: force collusion instead of a single bribe, make
alteration detectable rather than deniable, and give the loser a receipt.

## What a title is here

A **parcel** is a record with an identity that cannot be duplicated, because the
chain refuses to create a second one over the same ground:

- `parcel_id` — assigned by the chain, never reused, never reassigned.
- `geometry_hash` — the hash of the surveyed boundary (GeoJSON, or the cadastral
  reference). The survey itself is too large and too sensitive for a block; the
  hash proves which survey the title refers to.
- `cadastral_ref` — the registry's own human reference, so this record can be
  reconciled against the paper world it has to coexist with.
- `holder` — exactly one account. Not a list.
- `authority` — the registry office whose jurisdiction this parcel falls in.
- `status` — `REGISTERED`, `TRANSFER_PENDING`, `DISPUTED`, `FROZEN`.
- `encumbrances` — mortgages, liens, rights of way. Recorded, because a title
  without them is a lie that gets somebody's house taken.
- `deeds` — the chain of title as token metadata: each document's kind, hash,
  reference and where the registry serves it from.
- `restrictions` — what may lawfully be done with this land.
- `vehicle_id` — the tokenisation vehicle opened over it, if any. The title
  itself never moves into the vehicle.

**Single ownership is enforced at registration, not by convention.** The keeper
keeps an index on `geometry_hash`; registering a parcel whose hash already exists
fails. Overlap of *different* geometries is a survey problem the chain cannot
see, so the module makes the honest move: the geometry hash is unique, and a
claimed overlap is raised as a dispute by a human, not detected by code.

## Why a transfer needs more than a signature

A transfer is the moment land is stolen, so it is the moment that gets the
weight. Four things must happen, and no one party controls two of them:

1. **The holder consents.** They sign `MsgProposeTransfer` naming the recipient
   and the price. Without this nothing starts — an authority cannot move land on
   its own.
2. **The authority in charge validates.** The registry office for that
   jurisdiction signs. This is the office holding the paper file, which can check
   the seller is who they say they are.
3. **M-of-N registrars attest.** A quorum of *independent* registrars, drawn from
   a set that does **not** include the proposing authority, each sign
   `MsgAttestTransfer`. This is the anti-bribery mechanism: to steal a parcel you
   must buy the holder's key *and* the local office *and* a quorum of officials in
   other offices with no relationship to the buyer. One bribe is not enough, and
   every additional conspirator is a person who can defect or leak.
4. **A challenge window elapses.** Between quorum and completion the transfer is
   public and objectable. Anybody may file `MsgObject` with a reason; an objection
   moves the parcel to `DISPUTED` and stops the transfer dead.

Only when all four hold does `MsgCompleteTransfer` move the holder. Completion is
mechanical — it checks conditions and applies them — so no official holds a
discretionary final step they can sell.

### Parameters worth arguing about

| Parameter | Default | Why |
|---|---|---|
| `attestation_quorum` | 3 | Below three, two colluding officials suffice. |
| `challenge_window` | 14 days | Long enough for word to reach a family member in another city; short enough that legitimate sales are not strangled. |
| `same_authority_attestation` | false | An attestor from the proposing office is not independent; allowing it collapses the mechanism back to one bribe. |

These belong to governance, not to code, and the defaults are a starting position
rather than a finding.

## Every office is a group, not a person

A registry office is admitted as an **x/group account**, never a plain key, and
the keeper refuses anything else (`ErrOfficeNotGroup`). This matters because the
cross-office quorum only guards *transfers* — registration, validation,
restrictions and freezes are each done by one office, and if that office were a
single key, each of them would cost exactly one bribe.

With group accounts there are two independent layers:

- **Inside an office:** M-of-N registrars must agree before the office signs
  anything at all. This comes free from x/group; the keeper still sees one
  signer, but that signer is a policy.
- **Across offices:** a transfer additionally needs the independent quorum
  described above.

The check happens once, at admission, rather than on every message — it costs a
single lookup and it cannot be forgotten later. Trusting governance to only ever
admit group addresses would put the whole intra-office protection in a human
review step that will eventually be rushed.

Two consequences worth stating plainly:

- **First registration is the cheapest remaining attack.** Creating a title over
  ground nobody has claimed needs one office (M-of-N inside it) and no
  cross-office quorum. That is deliberate for seeding a registry, and arguably
  wrong afterwards — extending the attestation quorum to registration is the
  next thing to decide.
- **Freezing needs only the office.** With a group account that is already
  several people, but a captured office can still freeze land and extort its
  owner. The remedy in that case is `MsgObject` and a court, not the chain.

## What this deliberately does not do

- **It does not decide who owns land today.** Seeding the registry is a political
  act performed by the state that adopts it. The chain records the seeding as
  first registration by a named authority, timestamped, so the initial allocation
  is as auditable as everything after it.
- **It does not resolve disputes.** `DISPUTED` is a terminal state for the
  transfer and an entry point for a court. The chain's job is to stop the
  transfer and preserve the evidence, not to adjudicate.
- **It does not publish personal data.** Holders are accounts; names live in the
  registry's own records. A public list of who owns what, with names attached, is
  a targeting list.
- **It cannot detect a fraudulent survey.** If two honest-looking geometries
  describe overlapping ground, only a surveyor can say so. The module gives them
  a way to say it (`MsgObject`) and refuses to pretend it knows.

## Messages

| Message | Signer | Effect |
|---|---|---|
| `MsgRegisterAuthority` | governance | Admits a registry office, with its jurisdiction. |
| `MsgRegisterParcel` | authority | First registration. Fails if `geometry_hash` exists. |
| `MsgProposeTransfer` | holder | Opens a transfer to a named recipient. |
| `MsgValidateTransfer` | authority in charge | The jurisdiction's validation. |
| `MsgAttestTransfer` | registrar (another office) | One attestation toward quorum. |
| `MsgObject` | anyone | Halts the transfer, sets `DISPUTED`. |
| `MsgCompleteTransfer` | anyone | Mechanical: applies a transfer that has met every condition. |
| `MsgRecordEncumbrance` | authority | Adds or releases a lien or right of way. |
| `MsgFreezeParcel` | authority | Stops all movement — a court order, a fraud investigation. |
| `MsgAttachDeed` | authority | Adds a document to the chain of title. |
| `MsgSetRestriction` | authority | Imposes or lifts a limit on what may be done with the land. |
| `MsgAuthoriseFractionalisation` | authority | Permits a tokenisation vehicle over the parcel, with a ceiling and an expiry. |

`MsgCompleteTransfer` being open to anyone is deliberate: if only an official
could finalise, an official could refuse to, and refusal is leverage.

## A parcel is an NFT, and it can be fractionalised later — supervised

A parcel is one indivisible token with one holder. The deed documents ride with
it as metadata: each `Deed` carries the kind, the hash of the document the
registry holds, a URI to fetch it, and its reference in the paper world. The
scans stay with the registry — a 1974 grant is megabytes and usually contains
somebody's personal details — but the hash proves which paper the title means.

**Fractionalising land is legitimate and useful.** An owner may want to sell an
exploitation right in shares and collect rent from it; that is real financing for
people whose only asset is land they cannot borrow against. The danger is not
fractionalisation itself, it is fractionalisation the registry cannot see:
selling around a restriction, or moving control of the land without a transfer
ever being recorded.

So the bridge to `x/tokenisation` is supervised, and built on one rule: **the
title never leaves this module.**

- The authority signs `MsgAuthoriseFractionalisation`, naming the *right* being
  sold (exploitation, lease, revenue share — never the title), a ceiling
  `max_share_bps`, and an expiry.
- `x/tokenisation` refuses to open a vehicle over a parcel without a live
  authorisation, and refuses to issue beyond the ceiling.
- The parcel stays here, held by the same account, with `vehicle_id` pointing at
  what was opened over it. The vehicle sells rights that *reference* the parcel.

That separation is what lets a land service still answer the only two questions
that matter — *who owns this, and is what is being sold over it lawful* — after
the asset has been financialised. Withdrawing an authorisation stops new
issuance; it does not expropriate existing holders, because that is a taking and
belongs to a court, not to a registry office.

`Restriction` entries are the standing instruction all of this obeys:
`agricultural_use_only`, `heritage_protected`, `foreign_ownership_capped`,
`minimum_parcel_size`, `customary_tenure`. They are data rather than code
because land law differs by country, and a chain that hard-codes one country's
rules is a chain only that country can use.

## Nobody sees a wallet

The same abstraction the payments app enforces applies here, and matters more: a
farmer proving they own their field should never meet the words *address*,
*key*, *token* or *gas*.

- A holder is a **person or an office with a name**, resolved through the user
  ID system. Addresses appear nowhere in the interface.
- A parcel is found by its **cadastral reference** — the number on the paper
  somebody is holding — not by a chain id.
- Signing a transfer is **"approve this transfer"**, with the fee sponsored, so
  a citizen never needs to hold YML to defend their own title.
- The deed history reads as *who did what, when* — not as transactions.

## The application

A separate client (`clients/land`), because its audience is not the audience for
a wallet:

- **Search** by cadastral reference or parcel id — read-only, no account needed.
  A citizen must be able to check a claim before paying anybody.
- **A parcel page** showing holder, status, encumbrances, and the full transfer
  history with every signature and timestamp. This is the receipt the
  dispossessed currently do not have.
- **A registrar console** for proposing, validating and attesting, which shows an
  official exactly what they are signing and who else has signed.
- **An objection form** needing no credentials beyond an account, because the
  person being robbed is often not the person with the official relationships.
