<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/herd

## Transactions

### MsgAcceptTransfer

`/blockchain.herd.v1.MsgAcceptTransfer`

Signed by the `buyer` field.

MsgAcceptTransfer is the buyer agreeing to receive the animal.

A transfer nobody accepted is a transfer that never happened, and requiring it is what stops a keeper assigning an animal to somebody who never asked for it.

| Field | Type | Description |
| --- | --- | --- |
| `buyer` | string |  |
| `transfer_id` | uint64 |  |

### MsgAccreditAgent

`/blockchain.herd.v1.MsgAccreditAgent`

Signed by the `authority` field.

MsgAccreditAgent admits one person as an attestor, or strikes them off.

Signed by an OFFICE, not by governance, and that is the point. Attestation used to require the attestor to be an Authority — an x/group account admitted by a governance proposal — which would have made every field veterinarian a multi-signature institution with a parliamentary vote behind them. The transfer quorum is two, so a single lawful sale would have needed two of those to exist first.

Accreditation is the office's own act. A national veterinary order admits and strikes off its own members; it does not ask a legislature. What governance still controls is which offices exist at all, which is the level where the question actually belongs.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | The accrediting office. Its jurisdiction becomes the agent's. |
| `agent` | string | The person. A plain key, deliberately: an agent signs alone in a field with no signal, and an attestation is one person's statement that they looked at the animal. Making it M-of-N would not make it more true. |
| `name` | string |  |
| `active` | bool | False strikes the agent off. The record stays — every attestation they made names them, and the question "was this person accredited when they signed" has to stay answerable. |

### MsgAttestTransfer

`/blockchain.herd.v1.MsgAttestTransfer`

Signed by the `attestor` field.

MsgAttestTransfer is an accredited agent swearing the animal is the animal.

This is the only claim in the system the chain cannot check for itself, which is why it needs a named person answering for it.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `transfer_id` | uint64 |  |
| `evidence_hash` | string | What the attestor actually checked — the tag read back, a photograph taken at the time. Recorded so that a false attestation is a specific claim by a named person rather than a shrug. |

### MsgCancelTransfer

`/blockchain.herd.v1.MsgCancelTransfer`

Signed by the `seller` field.

MsgCancelTransfer withdraws a proposal.

The seller only, and only before anybody has attested. After an attestation the buyer has relied on the sale and an accredited agent has spent a journey on it, so a seller who could still walk away would hold a veto over a completed bargain — which is exactly what CompleteTransfer being permissionless exists to deny.

Without it an animal whose buyer simply never answers is frozen against any further sale for good, and the seller has done nothing wrong.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `transfer_id` | uint64 |  |
| `reason` | string |  |

### MsgCertifyAnimal

`/blockchain.herd.v1.MsgCertifyAnimal`

Signed by the `creator` field.

MsgCertifyAnimal raises an animal to level 3.

The pedigree is given as record ids, not names, and that is the whole difference from the certificate the sector issues today. A line of names can be neither walked nor contradicted; a line of references can be both.

CertifyAnimal raises an animal to level 3: a DNA profile and photographic biometrics, and a pedigree that refers to records rather than naming names. The registry refuses a line it can prove impossible.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `animal_id` | uint64 |  |
| `sire_id` | uint64 | Zero means unknown, which is honest and common: a founder animal has no recorded parents. It does not mean animal zero, which is never issued. |
| `dam_id` | uint64 |  |
| `dna_hash` | string |  |
| `photo_hash` | string |  |

### MsgCompleteTransfer

`/blockchain.herd.v1.MsgCompleteTransfer`

Signed by the `creator` field.

MsgCompleteTransfer moves the animal.

CompleteTransfer moves the animal, once the window has run and the attestations are in.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | Permissionless, deliberately. Every condition has already been checked by the chain, so whoever pays the gas is irrelevant — and requiring a particular signer would let that signer sit on a completed sale. |
| `transfer_id` | uint64 |  |

### MsgEnrolHerd

`/blockchain.herd.v1.MsgEnrolHerd`

Signed by the `creator` field.

MsgEnrolHerd records a herd, and it is the message that has to be cheap.

EnrolHerd records a herd at level 1 — the level that can actually cover twenty-one million head, because it is costed per herd rather than per animal.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | An active registry office. |
| `reference` | string | Unique. Two records claiming to be the same file makes reconciliation with the paper world guesswork. |
| `keeper` | string |  |
| `species` | Species |  |
| `declared_size` | uint32 |  |
| `locality` | string |  |
| `location_hash` | string |  |

### MsgFreezeAnimal

`/blockchain.herd.v1.MsgFreezeAnimal`

Signed by the `authority` field.

MsgFreezeAnimal stops an animal moving: a fraud inquiry, a court order, a sanitary hold.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `animal_id` | uint64 |  |
| `reason` | string |  |

### MsgIdentifyAnimal

`/blockchain.herd.v1.MsgIdentifyAnimal`

Signed by the `creator` field.

MsgIdentifyAnimal records one animal at level 2.

IdentifyAnimal records one animal at level 2, bound to its record by an ISO 11784/11785 transponder fitted by an accredited agent.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `herd_id` | uint64 |  |
| `national_id` | string | The national identifier, in the form the sector already issues: 221/682/021029/L. Unique. |
| `tag` | string | The ISO 11784/11785 transponder code. Unique when present. |
| `name` | string |  |
| `sex` | Sex |  |
| `coat` | string |  |
| `born_on` | string |  |
| `birth_type` | string |  |

### MsgObjectTransfer

`/blockchain.herd.v1.MsgObjectTransfer`

Signed by the `objector` field.

MsgObjectTransfer stops a transfer.

Open to anybody, because the person best placed to notice a stolen animal being sold is rarely a party to the sale.

| Field | Type | Description |
| --- | --- | --- |
| `objector` | string | Anybody. The person best placed to notice a stolen animal being sold is rarely a party to the sale, and a registry that only heard from the parties would hear nothing. |
| `transfer_id` | uint64 |  |
| `reason` | string |  |

### MsgProposeTransfer

`/blockchain.herd.v1.MsgProposeTransfer`

Signed by the `creator` field.

MsgProposeTransfer opens a supervised handover.

Signed by the current keeper, and it freezes the animal against a second transfer.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string | The current keeper of the animal. |
| `animal_id` | uint64 |  |
| `to` | string |  |
| `price` | string |  |

### MsgRecordHealthEvent

`/blockchain.herd.v1.MsgRecordHealthEvent`

Signed by the `agent` field.

MsgRecordHealthEvent records a treatment against a herd.

RecordHealthEvent is what turns a vaccination campaign into verifiable coverage rather than a count of doses administered.

| Field | Type | Description |
| --- | --- | --- |
| `agent` | string | The accredited agent or veterinarian who administered it. Signed by them rather than by the office, because the signature is the attestation. |
| `herd_id` | uint64 |  |
| `disease` | string |  |
| `batch` | string |  |
| `treated` | uint32 |  |

### MsgRegisterAuthority

`/blockchain.herd.v1.MsgRegisterAuthority`

Signed by the `authority` field.

MsgRegisterAuthority admits a registry office.

Governance only, and the office must be a group account: enrolling a herd, certifying parentage and validating a transfer are each a single signature otherwise.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | x/gov. Offices are admitted by the chain's governance, never by each other. |
| `office` | string |  |
| `name` | string |  |
| `jurisdiction` | string |  |
| `active` | bool |  |

### MsgReleaseAnimal

`/blockchain.herd.v1.MsgReleaseAnimal`

Signed by the `authority` field.

MsgReleaseAnimal ends a hold.

The counterpart to every stop in this module, and the reason the stops are safe to make quickly. A hold an office can impose but nothing can lift is a confiscation, and it would eventually be used as one.

ReleaseAnimal ends a hold and returns the animal to circulation.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `animal_id` | uint64 |  |
| `reason` | string |  |

### MsgResolveObjection

`/blockchain.herd.v1.MsgResolveObjection`

Signed by the `authority` field.

MsgResolveObjection is an office answering an objection.

This message exists because the objection is open to anybody. That openness is right — the person who recognises a stolen animal is rarely a party to the sale — but it means an objection costs nothing and can be made by anyone, so a stop that nothing could lift would be a denial-of-service on the national herd rather than a protection for it.

The office does not adjudicate ownership; a chain cannot. It decides whether this sale proceeds, and that decision is recorded against its name.

ResolveObjection is an office deciding whether an objection had substance.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | A registry office, which is a group account — so this is already an M-of-N decision inside the office rather than one official's signature. |
| `transfer_id` | uint64 |  |
| `uphold` | bool | True to uphold: the sale is cancelled and the animal is held pending whatever inquiry or court follows. False to dismiss: the transfer returns to where it was, and its challenge window starts again from the decision, so that dismissing an objection cannot be used to complete a sale the same minute nobody is left able to stop. |
| `reason` | string |  |

### MsgRetireAnimal

`/blockchain.herd.v1.MsgRetireAnimal`

Signed by the `creator` field.

MsgRetireAnimal records that an animal is gone.

RetireAnimal records a death, a slaughter or a sale out of the country. The record stays: a pedigree refers to its ancestors, and deleting one would silently break every descendant's line.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `animal_id` | uint64 |  |
| `reason` | string |  |

### MsgUpdateParams

`/blockchain.herd.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the module parameters. Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Agents

`GET /yamale/blockchain/herd/v1/agents`

Agents lists the accredited people an office answers for, struck-off ones included: "was this person accredited when they signed" is a question about the past.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | Optional. When set, only the agents this office accredited; when empty, every accredited agent on the register. |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `agents` | repeated Agent |  |
| `pagination` | PageResponse |  |

### Animal

`GET /yamale/blockchain/herd/v1/animal/{id}`

Animal returns one animal by its chain id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `animal` | Animal |  |

### AnimalByNationalID

`GET /yamale/blockchain/herd/v1/animal_by_national_id`

AnimalByNationalID is the certificate lookup: the identifier printed on the document, resolved to the record it refers to.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `national_id` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `animal` | Animal |  |

### AnimalByTag

`GET /yamale/blockchain/herd/v1/animal_by_tag`

AnimalByTag resolves a transponder code read in the field. This is the query an agent with a reader makes, and the one that answers "whose animal is this".

Request:

| Field | Type | Description |
| --- | --- | --- |
| `tag` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `animal` | Animal |  |

### Animals

`GET /yamale/blockchain/herd/v1/animals`

Animals lists the animals in one herd.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `herd_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `animals` | repeated Animal |  |
| `pagination` | PageResponse |  |

### Authorities

`GET /yamale/blockchain/herd/v1/authorities`

Authorities lists the registry offices.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `authorities` | repeated Authority |  |
| `pagination` | PageResponse |  |

### HealthEvents

`GET /yamale/blockchain/herd/v1/health_events`

HealthEvents lists what a herd has been treated for, which is the difference between coverage and a count of doses.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `herd_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `health_events` | repeated HealthEvent |  |
| `pagination` | PageResponse |  |

### Herd

`GET /yamale/blockchain/herd/v1/herd/{id}`

Herd returns one herd by its chain id.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `herd` | Herd |  |

### HerdByRef

`GET /yamale/blockchain/herd/v1/herd_by_ref`

HerdByRef returns one herd by the registry's own reference, which is what an officer holding a paper file actually has in front of them.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `reference` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `herd` | Herd |  |

### Params

`GET /yamale/blockchain/herd/v1/params`

Params returns the module parameters.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### Pedigree

`GET /yamale/blockchain/herd/v1/pedigree/{animal_id}`

Pedigree walks the line upward from one animal.

The endpoint that the free-text certificate cannot have. Depth is bounded by the caller and by the module, because an unbounded walk over a graph somebody else populated is a denial of service with a polite name.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `animal_id` | uint64 |  |
| `depth` | uint32 | How many generations to walk. Bounded by the module regardless of what is asked for: a pedigree is a graph a stranger populated, and an unbounded walk over one is a denial of service. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `nodes` | repeated PedigreeNode |  |
| `truncated` | bool | True when the walk stopped because it hit the depth bound rather than because it ran out of recorded ancestors. Without this a truncated line is indistinguishable from a founder, and a buyer would read "no further ancestors" where the truth is "we stopped looking". |

### Transfer

`GET /yamale/blockchain/herd/v1/transfer/{id}`

Transfer returns one transfer.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `transfer` | Transfer |  |

### TransfersByAnimal

`GET /yamale/blockchain/herd/v1/transfers_by_animal/{animal_id}`

TransfersByAnimal is the provenance of one animal: every supervised handover it has been through, including the ones that were objected to.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `animal_id` | uint64 |  |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `transfers` | repeated Transfer |  |
| `pagination` | PageResponse |  |

## State

### Agent

Agent is one accredited person: a veterinarian, a livestock officer, the individual who physically stood in front of the animal.

Separate from Authority, and the separation was forced by trying to run a sale. Attestation used to require the attestor to be an Authority — but an Authority must be an x/group account admitted by chain governance, so every field veterinarian would have to be an M-of-N group with a governance proposal behind them. In a pastoral zone that is not a high bar, it is an impossible one, and the transfer quorum of two would have needed two such groups before a single animal could lawfully change hands.

So an agent is a PERSON with a plain key, accredited by an office rather than by governance — which is how accreditation actually works: a national veterinary order strikes off its own members, and it does not need a parliament to do it.

The office that accredited an agent is recorded, because that is what the independence check turns on. An attestor accredited by the same office that holds the animal's record is not independent, and the four-party shape collapses back to one office deciding.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `name` | string |  |
| `authority` | string | The office that accredited this agent and can strike them off. |
| `jurisdiction` | string | Copied from the accrediting office. An agent cannot be accredited into a jurisdiction their office does not hold. |
| `active` | bool | Struck off agents keep their record — every attestation they ever made names them, and deleting the agent would leave those attestations pointing at nothing — but they stop being able to act. |
| `accredited_at` | int64 |  |

### Animal

Animal is what levels 2 and 3 record. One animal, one record.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `herd_id` | uint64 | The herd this animal belongs to. Every animal has one: an animal with no herd has no keeper to answer for it, and the transfer rules would have nobody to ask. |
| `national_id` | string | The national identifier, human-readable, in the form the Ladoum sector already issues: 221/682/021029/L — country dialling code, flock, serial, breed letter. It is a good format and it generalises, so all three levels use it. Unique and indexed. This is the key a certificate is looked up by. |
| `tag` | string | The ISO 11784/11785 transponder code, when one is fitted. Unique when present and empty at level 1. A tag is cheap, quick and REMOVABLE, which is why it is not the only marker at level 3. A registry that treated a tag as proof of identity would be a registry that can be defeated with a pair of pliers. |
| `level` | Level |  |
| `name` | string |  |
| `sex` | Sex |  |
| `coat` | string |  |
| `born_on` | string | Birth date as a day, not a timestamp. A pastoral birth is known to the day at best, and a second-precision field would invite a precision nobody has. |
| `birth_type` | string | Single, twin, triplet. Recorded because it bears on the coherence checks: two animals declared to the same dam days apart are either twins or a false declaration, and the registry cannot tell which unless this says. |
| `sire_id` | uint64 | The pedigree, as references rather than names. This is the difference between the certificate the sector issues today and one that can be checked. Today a line reads "Classico / Atomique / Palmarin" — names, in free text, which nobody can walk and nobody can contradict. Here an ancestor is a record, so a line can be walked in one query, an inbreeding coefficient computed, and an impossible declaration refused. Zero means unknown, which is honest and common: a founder animal has no recorded parents. It does NOT mean animal zero, which is never issued. |
| `dam_id` | uint64 |  |
| `dna_hash` | string | Hash of the DNA profile — a panel of microsatellite markers. Level 3 only, and the only real proof that a declared sire is the sire. The profile itself stays with the laboratory; the hash proves which profile this record refers to. |
| `photo_hash` | string | Hash of the standardised photographic biometrics, captured with timestamp and position. Level 3 only. |
| `holder` | string | The current keeper. Moves only through a four-party transfer, which is what makes the resale of a stolen animal impossible rather than merely traceable. |
| `authority` | string |  |
| `status` | Status |  |
| `registered_at` | int64 |  |

### Attestation

Attestation is one accredited agent swearing that the animal presented is the animal on the record.

This is the only claim in the whole system that the chain cannot check for itself, and the reason the four-party shape exists: seller, buyer, registry and somebody who physically looked at the animal.

| Field | Type | Description |
| --- | --- | --- |
| `attestor` | string |  |
| `authority` | string | The office the attestor answers to, captured at attestation time. Recorded rather than looked up later because an attestor can be struck off, and the question "was this independent when it was made" has to stay answerable. |
| `evidence_hash` | string | Hash of what they actually checked — the tag read back, a photograph taken at the time. A false attestation should be a specific claim by a named person, not a shrug. |
| `attested_at` | int64 |  |

### Authority

Authority is a registry office.

It must be an x/group account, for the same reason x/land requires it: an office that is one key is one bribe, and enrolling a herd, certifying parentage and validating a transfer are each a single signature otherwise.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |
| `name` | string |  |
| `jurisdiction` | string |  |
| `active` | bool |  |

### HealthEvent

HealthEvent is what turns a vaccination campaign into verifiable coverage.

A campaign without a registry produces a number of doses administered. That is not coverage, and coverage is the only figure eradication needs. This records which herd was treated, when, by whom, and with which batch — at level 1, therefore at the lowest cost, therefore across the whole herd.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `herd_id` | uint64 |  |
| `disease` | string | The disease treated against. PPR, CBPP, and so on. |
| `batch` | string | The vaccine batch. Recorded because a batch that fails has to be traceable to every herd that received it, and that is a recall rather than a rumour. |
| `treated` | uint32 | How many animals were treated. May differ from the herd's declared size — that difference is itself information, and hiding it by forcing them equal would destroy the only signal that a declaration is drifting. |
| `agent` | string | The accredited agent or veterinarian who administered it and signed for it. |
| `recorded_at` | int64 |  |

### Herd

Herd is what level 1 records, and it is costed per herd rather than per head.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 | Assigned by the chain. Never reused, never reassigned. |
| `reference` | string | The registry's own human reference, so this record can be reconciled with the paper world it will coexist with for a very long time. Unique, and indexed: two records claiming to be the same file makes reconciliation guesswork. |
| `keeper` | string | The person or group answering for these animals. Exactly one account — co-ownership is a legal arrangement between people, expressed by this being a group account rather than by the chain holding several keepers and having to rank them. |
| `authority` | string | The office whose jurisdiction this herd falls in. |
| `species` | Species |  |
| `declared_size` | uint32 | What the keeper says the herd contains, per species. DECLARED, not counted, and the field name says so on purpose: level 1 buys a registry of who keeps roughly how many animals where, which is what a vaccination campaign and an index insurer need. Anything that requires a verified count is level 2, and costs accordingly. |
| `locality` | string | Where the herd is, in words a person uses. Free text because a pastoral herd moves and a point would be a lie with more decimal places. |
| `location_hash` | string | Hash of a surveyed location or a GeoJSON extent, when one exists — a fixed holding, a designated grazing area. Optional, because for a transhumant herd there is nothing honest to put here. |
| `jurisdiction` | string |  |
| `status` | Status |  |
| `registered_at` | int64 |  |

### PedigreeNode

PedigreeNode is one ancestor and where it sits in the line.

| Field | Type | Description |
| --- | --- | --- |
| `animal` | Animal |  |
| `generation` | uint32 | 0 for the animal asked about, 1 for its parents, and so on. |
| `path` | string | The path taken to reach it — "sire", "dam", "sire.dam" — so a caller can render the tree without re-deriving which side each ancestor came from. |

### Transfer

Transfer is a supervised handover of one animal.

The mechanism is the same one x/land uses for a parcel, and for the same reason: it is not there to trace a theft afterwards, it is there to make the lawful resale of a stolen animal impossible, which is what destroys its value. A thief can still take the beast. He cannot sell it with a title.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `animal_id` | uint64 |  |
| `from` | string |  |
| `to` | string |  |
| `price` | string | Free text, and not enforced by the chain. The registry records what the parties say they agreed; it does not hold the money and does not pretend to have witnessed the payment. |
| `state` | TransferState |  |
| `attestations` | repeated Attestation |  |
| `attested_at` | int64 | When the attestation quorum was reached. The challenge window runs from this, not from the proposal — otherwise a transfer could be proposed quietly, left to age, and completed the moment it was attested. |
| `objected_by` | string |  |
| `objection_reason` | string |  |
| `proposed_at` | int64 |  |
| `resolved_by` | string | Who resolved the objection, and what they decided. An objection can be raised by anybody, so it has to be answerable by somebody — and the answer is recorded rather than merely applied, because the interesting pattern is not one dismissal, it is an office that dismisses every objection raised against one seller. |
| `resolution_reason` | string |  |
| `objection_upheld` | bool | True where the office found the objection had substance. Kept as a field rather than inferred from the state, because a dismissed objection returns the transfer to the state it was in and would otherwise leave no trace that it had ever been raised. |
| `accepted_at` | int64 | When the buyer agreed. Needed to put a transfer back where it was after an objection is dismissed: the state alone was overwritten by OBJECTED, and guessing from the attestation count cannot tell a proposal nobody accepted from one that was accepted and not yet attested. |

## Value types

### Level

Level is how strongly a record is tied to a living animal, and it is the whole design.

Senegal has 21,185,146 head excluding poultry. At 1,000 FCFA per electronic tag fitted — counting neither the reader, nor the agent's journey, nor the data entry — identifying every animal individually costs 21.2 billion FCFA, which is 96.6% of the entire PNDIES programme in tags alone. So a national scheme cannot mean "identify every animal", and any registry that only knows how to hold individuals is a registry that can only ever cover a rounding error of the herd.

Three levels instead, each costing what it makes possible, and the level is stated on the record so that a buyer, a veterinarian or a banker knows exactly what they are looking at.

| Value | Meaning |
| --- | --- |
| `LEVEL_UNSPECIFIED` | Never stored. The zero value is what a message that forgot to set a level looks like, and the keeper refuses it rather than defaulting — defaulting here would silently record a herd as a certified animal. |
| `LEVEL_HERD` | The herd, its keeper, its declared size, where it is, and the health campaigns it received. Nothing individual, and nothing verified beyond the agent's attestation that they stood in front of it. Costed per herd rather than per head, which is the only reason it can apply to the whole national herd. It is not a cut-price level: verifiable vaccination coverage, transhumance and index insurance all need this and nothing more. |
| `LEVEL_INDIVIDUAL` | One animal, one record, bound by an ISO 11784/11785 transponder fitted by an accredited agent. For animals that move commercially, are exported, or are pledged as security. |
| `LEVEL_CERTIFIED` | The tag, plus a DNA profile with parentage actually tested, plus standardised photographic biometrics. For élite breeding stock, where the value of one animal justifies a laboratory. |

### Sex

Sex, which the pedigree checks turn on: a sire must be male and a dam female, and a registry that did not hold this could not refuse the one declaration that is impossible on its face.

| Value | Meaning |
| --- | --- |
| `SEX_UNSPECIFIED` | Unset. Permitted at level 1, where nothing individual is recorded, and refused on any animal offered as a parent. |
| `SEX_MALE` | Male. |
| `SEX_FEMALE` | Female. |

### Species

Species, because the arithmetic differs by an order of magnitude between them and so does the reason for identifying them. Small ruminants are the PPR target population — 15,153,137 of them — and cattle are where the collateral argument lives.

| Value | Meaning |
| --- | --- |
| `SPECIES_UNSPECIFIED` | Unset. Refused on a message; it is what a caller who did not choose looks like, and a herd of unspecified animals cannot be vaccinated against anything in particular. |
| `SPECIES_CATTLE` | Cattle. Where the collateral argument lives: a pastoral household's herd is its entire wealth and worth nothing to a bank. |
| `SPECIES_SHEEP` | Sheep. With goats, the 15,153,137 small ruminants that are the PPR target population. |
| `SPECIES_GOAT` | Goats. |
| `SPECIES_HORSE` | Horses. Senegal microchipped equines in Thies from 2016 against theft, so this is the one species with an identification precedent here. |
| `SPECIES_CAMEL` | Camels. |

### Status

Status is where a record is in its life. A record that is not ACTIVE cannot begin a transfer, which is what makes the other states effective: they are stops rather than warnings.

| Value | Meaning |
| --- | --- |
| `STATUS_UNSPECIFIED` | Never stored. The zero value only appears on a message that forgot to set one, and the keeper refuses it rather than defaulting to ACTIVE. |
| `STATUS_ACTIVE` | Live, held by exactly one keeper, and movable. |
| `STATUS_TRANSFER_PENDING` | A transfer is under way. No second transfer may start against it. |
| `STATUS_DISPUTED` | Somebody objected. Terminal for the transfer and an entry point for a court; the chain preserves the evidence and does not adjudicate. |
| `STATUS_FROZEN` | Stopped by an office. A fraud inquiry, a court order, a sanitary hold. |
| `STATUS_RETIRED` | Dead, slaughtered or otherwise gone. Kept rather than deleted: a pedigree refers to its ancestors, and deleting an ancestor would silently break every descendant's line. |

### TransferState

TransferState is where a supervised handover has got to.

| Value | Meaning |
| --- | --- |
| `TRANSFER_STATE_UNSPECIFIED` | Never stored on a transfer that exists. A transfer is created PROPOSED. |
| `TRANSFER_STATE_PROPOSED` | Opened by the keeper, waiting for the buyer to agree. |
| `TRANSFER_STATE_ACCEPTED` | The buyer has agreed; waiting for attestations. |
| `TRANSFER_STATE_ATTESTED` | Enough accredited agents have sworn the animal is the animal. The challenge window runs from here. |
| `TRANSFER_STATE_OBJECTED` | Somebody objected. The transfer is stopped and an office must now decide whether the objection has substance. Not terminal, and it must not be: an objection is open to anybody, so a state nobody could ever leave would let one adversary with a script freeze the national herd for the price of gas. |
| `TRANSFER_STATE_COMPLETED` | The animal has moved. |
| `TRANSFER_STATE_CANCELLED` | Withdrawn by the seller before anybody attested, or ended by an office upholding an objection. Terminal, and the record stays: a sale somebody opened and abandoned is part of an animal's provenance. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotAuthority` | signer is not a registry office |
| 10 | `ErrNoNationalID` | a national identifier is required |
| 11 | `ErrNationalIDTaken` | this national identifier is already used |
| 12 | `ErrTagTaken` | this tag is already fitted to another animal |
| 13 | `ErrNoAnimal` | no such animal |
| 14 | `ErrAnimalNotActive` | this animal is not in a state that allows it |
| 15 | `ErrNoTag` | an animal must carry a tag before it can be certified |
| 16 | `ErrNoDNA` | a certified animal needs a DNA profile; without one the parentage is the breeder's word |
| 17 | `ErrParentMissing` | the parent named is not a record here |
| 18 | `ErrSireNotMale` | the animal named as sire is not male |
| 19 | `ErrDamNotFemale` | the animal named as dam is not female |
| 2 | `ErrAuthorityInactive` | this registry office is not active |
| 20 | `ErrParentYounger` | the parent named was born after this animal |
| 21 | `ErrPedigreeCycle` | that would make the animal its own ancestor |
| 22 | `ErrBirthTooClose` | this dam already has a birth recorded too recently for another |
| 23 | `ErrNoDisease` | a disease is required |
| 24 | `ErrNoTreated` | the number treated must be positive |
| 25 | `ErrNoHealthEvent` | no such health event |
| 26 | `ErrNotKeeper` | only the keeper may propose a transfer |
| 27 | `ErrInvalidRecipient` | the recipient is not a valid account |
| 28 | `ErrSelfTransfer` | the recipient is already the keeper |
| 29 | `ErrNoTransfer` | no such transfer |
| 3 | `ErrInvalidJurisdiction` | that is not a country this chain recognises |
| 30 | `ErrTransferClosed` | this transfer is already finished |
| 31 | `ErrNotBuyer` | only the named buyer may accept |
| 32 | `ErrNotAccepted` | the buyer has not accepted yet |
| 33 | `ErrAlreadyAttested` | this attestor has already attested |
| 34 | `ErrNotIndependent` | an attestor from the proposing office is not independent |
| 35 | `ErrQuorumNotMet` | not enough attestations yet |
| 36 | `ErrWindowOpen` | the challenge window has not closed |
| 37 | `ErrTransferObjected` | somebody objected to this transfer |
| 38 | `ErrInvalidLevel` | a level is required |
| 39 | `ErrNotSeller` | only the seller may withdraw a proposal |
| 4 | `ErrOfficeNotGroup` | a registry office must be a group account, so its decisions need several signatures |
| 40 | `ErrNotObjected` | there is no objection on this transfer to resolve |
| 41 | `ErrTransferAttested` | an attested transfer can no longer be withdrawn; the buyer and an agent have acted on it |
| 42 | `ErrNotHeld` | this animal is not under a hold |
| 43 | `ErrNoReason` | a reason is required; a stop or a release with no reason recorded is unreviewable |
| 44 | `ErrWrongJurisdiction` | this animal is not in your office's jurisdiction |
| 45 | `ErrNotAgent` | you are not an accredited agent; an attestation is a named person's statement |
| 46 | `ErrAgentInactive` | this agent has been struck off and can no longer attest |
| 47 | `ErrInvalidAgent` | the agent is not a valid account |
| 5 | `ErrNoReference` | a herd reference is required |
| 6 | `ErrRefTaken` | this herd reference is already used |
| 7 | `ErrInvalidKeeper` | the keeper is not a valid account |
| 8 | `ErrNoSpecies` | a species is required |
| 9 | `ErrNoHerd` | no such herd |
