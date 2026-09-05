<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/paymsg

ISO 20022-shaped credit transfers between institutions that governance has approved, each leaving a queryable statement entry.

## Transactions

### MsgApplyParticipant

`/blockchain.paymsg.v1.MsgApplyParticipant`

Signed by the `creator` field.

ApplyParticipant defines the ApplyParticipant RPC.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `code` | string |  |
| `name` | string |  |

### MsgApproveParticipant

`/blockchain.paymsg.v1.MsgApproveParticipant`

Signed by the `authority` field.

ApproveParticipant defines the ApproveParticipant RPC. It is authority-gated (the x/gov module account) and approves or rejects a pending participant application submitted via MsgApplyParticipant.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `participant` | string |  |
| `approve` | bool |  |

### MsgConfirmParticipant

`/blockchain.paymsg.v1.MsgConfirmParticipant`

Signed by the `customer` field.

MsgConfirmParticipant is the account's own word on who banks it.

Signed by the customer, which is the point: without it a participant's claim was the whole of the record, and the account it named had no say and no way out. Confirming turns a claim into a relationship a payment can rely on; refusing removes it and frees the account to bank elsewhere.

| Field | Type | Description |
| --- | --- | --- |
| `customer` | string |  |
| `participant` | string | The participant being confirmed. Named so that a confirmation cannot be replayed against a different claim than the one the account read. |
| `confirm` | bool | false removes the record, whether it was confirmed or not. An account may always leave. |

### MsgRegisterCustomer

`/blockchain.paymsg.v1.MsgRegisterCustomer`

Signed by the `participant` field.

RegisterCustomer records that an account banks with the signing participant, which is what entitles a payment from that account to name the participant as its instructing agent.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string | participant is the approved institution, and the only signer that may claim or disclaim an account as its customer. |
| `customer` | string |  |
| `registered` | bool | registered false removes the relationship, so a participant can end one without governance being involved. |

### MsgSendPayment

`/blockchain.paymsg.v1.MsgSendPayment`

Signed by the `debtor` field.

SendPayment defines the SendPayment RPC.

| Field | Type | Description |
| --- | --- | --- |
| `debtor` | string |  |
| `end_to_end_id` | string |  |
| `instructing_participant` | string |  |
| `instructed_participant` | string |  |
| `creditor` | string |  |
| `denom` | string |  |
| `amount` | string |  |
| `purpose_code` | string | purpose_code and remittance_information are ISO 20022 free text, and are superseded by metadata_hash. A sender that sets metadata_hash must leave both empty. They keep their numbers rather than being removed or reused: a deployment already holds payments encoded with 8 and 9 as strings, and every ledger entry, statement export and client that has ever read one depends on that staying true. Emptying them is a client-side change; renumbering them would silently corrupt history. The reason to stop writing them is not tidiness. This is where operators put customer names in practice, and the module writes them to a ledger that has no erasure path — which is the exposure under the NDPA, Ghana's DPA, POPIA and the GDPR. |
| `remittance_information` | string |  |
| `amount_commitment` | bytes | amount_commitment is the Pedersen commitment C = aG + rH that will replace the plaintext amount above. Not yet populated and not yet verified: commitments are their own workstream with their own audit. It is numbered now because the switch is exactly the moment the chain is holding balances that cannot be re-encoded. |
| `amount_range_proof` | bytes | amount_range_proof proves the committed amount is non-negative. Not yet populated. It travels with amount_commitment and is not optional once that is live: a commitment the chain accepts without a range proof lets a negative amount balance the sums, which is an inflation bug wearing a privacy feature's clothes. |
| `metadata_hash` | bytes | metadata_hash is SHA-256 over the canonical metadata payload held off-chain — the "encrypted payload hash" of docs/scope/confidentiality.md. It hashes the payload itself, not the ciphertext, because the check that matters is a party who holds the payload proving it is the one recorded. Encryption is randomised, so a hash over ciphertext could be verified by nobody: the payer, the payee and the regulator all decrypt to the same payload and all get different bytes back if they re-encrypt. The payload carries a random salt for a reason that is easy to miss: a purpose code is four characters from a published list, so an unsalted hash of one is not a fingerprint, it is a lookup table. Hashing without the salt would publish the field it was meant to hide. |
| `settlement_jurisdiction` | string | settlement_jurisdiction is the ISO 3166-1 alpha-2 country whose authority settles this payment. Unlike the three above it means something today. A cross-border payment touches two perimeters, and both endpoint authorities may see it, but only this one may act on it — so without the declaration there is a contest over standing that the chain cannot resolve, and the record cannot show which authority had the right to act. See docs/scope/roles-and-perimeter.md. The same declaration names the regulator who holds the third viewing key over the payload metadata_hash commits to. That is why it has to be stated when the payment is sent rather than decided afterwards: the payload is encrypted to that regulator's key at that moment, and a payment sent without a jurisdiction is one no regulator can ever open. Accepted empty for now, and required once Params.require_settlement_jurisdiction is turned on. Refusing it outright today would refuse payments that were valid when they were included, and a node syncing from block 0 re-runs every one of them. |

### MsgSetPayloadStore

`/blockchain.paymsg.v1.MsgSetPayloadStore`

Signed by the `participant` field.

SetPayloadStore records where an approved participant serves the encrypted payloads of the payments it instructed.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string |  |
| `url` | string | url is an absolute http or https base URL, or empty to withdraw. Validated for scheme and length here rather than left to the clients. A value that is not a URL is not a store, and the failure it produces otherwise lands on the payee — who sees detail they are entitled to read reported as missing, with nothing to tell them the reason is a typo in somebody else's registration. |

### MsgUpdateParams

`/blockchain.paymsg.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams defines a (governance) operation for updating the module parameters. The authority defaults to the x/gov module account.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the address that controls the module (defaults to x/gov unless overwritten). |
| `params` | Params | NOTE: All parameters must be supplied. |

## Queries

### GetApprovedParticipant

`GET /yamale/blockchain/paymsg/v1/approved_participant/{participant}`

ListApprovedParticipant Queries a list of ApprovedParticipant items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_participant` | ApprovedParticipant |  |

### GetParticipantApplication

`GET /yamale/blockchain/paymsg/v1/participant_application/{creator}`

ListParticipantApplication Queries a list of ParticipantApplication items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `participant_application` | ParticipantApplication |  |

### GetPaymentRecord

`GET /yamale/blockchain/paymsg/v1/payment_record_by_id`

ListPaymentRecord Queries a list of PaymentRecord items.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `end_to_end_id` | string |  |
| `instructing_participant` | string | instructing_participant scopes the id. End-to-end ids are unique per instructing party in ISO 20022, not globally, so the id alone does not identify a payment. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `payment_record` | PaymentRecord |  |

### ListApprovedParticipant

`GET /yamale/blockchain/paymsg/v1/approved_participant`

ListApprovedParticipant defines the ListApprovedParticipant RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `approved_participant` | repeated ApprovedParticipant |  |
| `pagination` | PageResponse |  |

### ListParticipantApplication

`GET /yamale/blockchain/paymsg/v1/participant_application`

ListParticipantApplication defines the ListParticipantApplication RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `participant_application` | repeated ParticipantApplication |  |
| `pagination` | PageResponse |  |

### ListPaymentRecord

`GET /yamale/blockchain/paymsg/v1/payment_record`

ListPaymentRecord defines the ListPaymentRecord RPC.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `payment_record` | repeated PaymentRecord |  |
| `pagination` | PageResponse |  |

### Params

`GET /yamale/blockchain/paymsg/v1/params`

Parameters queries the parameters of the module.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params | params holds all the parameters of this module. |

## State

### ApprovedParticipant

ApprovedParticipant defines the ApprovedParticipant message.

| Field | Type | Description |
| --- | --- | --- |
| `participant` | string |  |
| `code` | string |  |
| `name` | string |  |
| `payload_store_url` | string | payload_store_url is where this participant serves the encrypted payloads of the payments it instructed. A directory fact, not key material: it names a host, and everything behind it is already encrypted to keys this participant does not hold. It is on the chain rather than in each client's configuration because the payee is the party that has to find it, and the only thing the payee is guaranteed to have is the payment record — which names the instructing participant and nothing else. A configuration file would make "which store holds this payment's detail" a question every client answers separately and some answer stale, and a stale answer here renders as detail that has been lost. Empty is the honest default and means this participant runs no store. A client must show that as detail being unavailable, never as a payment with no detail. |

### Customer

Customer is an account an approved participant acts for.

One participant per customer, mirroring the arrangement it describes: an account holder's payment service provider is a single institution, and allowing several would reintroduce the ambiguity about who instructed a payment that this record exists to remove.

| Field | Type | Description |
| --- | --- | --- |
| `customer` | string |  |
| `participant` | string |  |
| `confirmed` | bool | confirmed is the account's own signature on the relationship. Registration is signed by the participant alone, and one participant per customer means the first to claim an account owns it: no second institution may claim it, and only the claiming one could release it. The comment defending that said it prevented impersonation, and it does prevent the SECOND participant impersonating — it did nothing about the first. An approved institution could attach itself to any account on the chain, be named as the instructing participant on that account's payments, and lock it out of banking anywhere else. So a claim is now only a claim. The account confirms it with MsgConfirmParticipant, and assertInstructedBy — the check that makes a PaymentRecord mean anything — requires the confirmation, not the claim. |

### ParticipantApplication

ParticipantApplication defines the ParticipantApplication message.

| Field | Type | Description |
| --- | --- | --- |
| `creator` | string |  |
| `status` | string |  |
| `code` | string |  |
| `name` | string |  |

### PayloadEnvelope

PayloadEnvelope is the encrypted form of a PaymentMetadata payload.

It is never sent to the chain and never stored by it — the chain holds only the SHA-256 of the *plaintext* payload, in MsgSendPayment.metadata_hash. The envelope lives in a participant's payload store. It is declared here, in the same schema as the payload it wraps, for exactly the reason PaymentMetadata is: the payer's wallet, the payee's wallet, a regulator's Go tooling and the store itself all have to agree on the bytes, and three hand-written framings in three languages agree until the first field somebody adds.

The construction is the standard multi-recipient KEM/DEM composition — the one age uses, and the one RFC 9180 formalises as DHKEM(X25519, HKDF-SHA256) with ChaCha20-Poly1305:

1. a fresh 32-byte content key encrypts the padded payload once, under ChaCha20-Poly1305; 2. for each recipient, a fresh ephemeral X25519 key agrees a shared secret with that recipient's registered viewing key, HKDF-SHA256 turns it into a wrapping key, and the content key is sealed under it.

Nothing here is novel, and that is the point. The payload is the ISO 20022 detail of somebody's payment; it is not the place to find out whether a new scheme holds.

What is deliberately *not* in this message: any hint of who the recipients are beyond an eight-byte key fingerprint, and any length signal. See RecipientBlock.key_id and ciphertext below.

| Field | Type | Description |
| --- | --- | --- |
| `version` | uint32 | version is the envelope format, not the key version. Present from the first byte because the alternative is discovering, on the day the format has to change, that every stored envelope is a bare blob whose framing can only be guessed at from its length. A reader refuses a version it does not implement rather than parsing it optimistically. |
| `recipients` | repeated RecipientBlock | recipients is one wrapped copy of the content key per party entitled to read: the payer, the payee, the regulator of the declared settlement jurisdiction, and every auditor whose grant was live when the payment was sent. Order is not significant and must not be relied on. A reader tries the blocks whose key_id matches a key it holds, and falls back to trying all of them — key_id is a lookup hint, never an access control. |
| `nonce` | bytes | nonce is 12 bytes, fresh per envelope, for the content cipher. Fresh per envelope rather than derived from the payment, because a nonce reused under one key with ChaCha20-Poly1305 does not degrade gracefully: it leaks the XOR of two plaintexts and the authentication key with it. The content key is also fresh per envelope, so this is belt and braces — which is the correct amount for a value whose reuse is unrecoverable. |
| `ciphertext` | bytes | ciphertext is ChaCha20-Poly1305 over the padded payload. Padded to a multiple of 256 bytes before encryption, so the length of the ciphertext says nothing about the length of the remittance line. Without padding the store — and anyone who can see a response size — could tell a one-word reference from a full name and address, which is a meaningful part of what the payload was moved off-chain to protect. The associated data is the payment's identity: the domain string, the instructing participant and the end-to-end id. That binds the ciphertext to one payment, so a store cannot serve the payload of one payment under the key of another — a substitution that would otherwise decrypt cleanly and then fail only against the on-chain hash, if the reader remembered to check it. |

### PaymentMetadata

PaymentMetadata is the ISO 20022 detail that used to travel in MsgSendPayment's purpose_code and remittance_information fields.

It is never sent to the chain and is never stored by it. The chain sees only SHA-256 over this message's encoding, in MsgSendPayment.metadata_hash; the message itself is held off-chain by the parties. It is declared here, rather than in each client, so that the payer, the payee, the regulator and the node all hash the same bytes — three hand-written serialisers in three languages would agree until the first field anybody added, and a hash that disagrees proves nothing while looking exactly like a hash that works.

Encoding is the protobuf encoding: fields in ascending number, defaults omitted. That is the same determinism the chain already stakes transaction signing on, so it is not a new assumption.

| Field | Type | Description |
| --- | --- | --- |
| `salt` | bytes | salt is 32 random bytes, regenerated for every payment. Without it this whole exercise publishes what it set out to hide. A purpose code is four characters drawn from a published list, so the unsalted hash of one is not a fingerprint — it is a lookup table anybody can build in a second. Remittance text is barely better: invoice numbers and names are guessable, and the ledger is public and permanent, so a guess can be tested for as long as the chain exists. Regenerated per payment, not per account, because a reused salt means two payments carrying the same detail hash to the same value, which tells an observer they are the same detail without ever revealing what it is. |
| `purpose_code` | string |  |
| `remittance_information` | string |  |

### PaymentRecord

PaymentRecord defines the PaymentRecord message.

This is the camt.053-style statement entry, and it is what participants reconcile against, so its numbering is as unreclaimable as the message's: records already written decode by field number and nothing can re-encode them. Fields 11-13 are added while that is still cheap.

| Field | Type | Description |
| --- | --- | --- |
| `end_to_end_id` | string |  |
| `instructing_participant` | string |  |
| `instructed_participant` | string |  |
| `debtor` | string |  |
| `creditor` | string |  |
| `denom` | string |  |
| `amount` | string |  |
| `purpose_code` | string | Kept, and kept at 8 and 9, for records written before metadata_hash existed. Payments sent since carry the hash and leave these empty; nothing rewrites the old ones, because a statement entry that changes after the fact is not a statement entry. |
| `remittance_information` | string |  |
| `block_height` | uint64 |  |
| `amount_commitment` | bytes | amount_commitment mirrors the field on MsgSendPayment. Not yet populated. |
| `metadata_hash` | bytes | metadata_hash pins the off-chain payload this payment's ISO 20022 detail was recorded in, so a party holding the payload can prove it is the one the chain saw and cannot substitute another afterwards. The range proof that will accompany amount_commitment is deliberately not stored here. It is checked once, at execution, and is worth nothing afterwards — keeping kilobytes of it against every payment forever is state bloat priced at one transaction fee. |
| `settlement_jurisdiction` | string | settlement_jurisdiction is recorded because the perimeter check has to run against the payment, not against a memory of it: which authority may act on this payment is a question asked long after the block that carried it. |

### RecipientBlock

RecipientBlock is one entitled party's wrapped copy of the content key.

| Field | Type | Description |
| --- | --- | --- |
| `key_id` | bytes | key_id is the first 8 bytes of SHA-256 over the recipient's X25519 public key. A hint, so a reader holding one key does not have to attempt every block, and truncated so that the block does not simply publish the recipient's public key to whoever fetches the envelope. Eight bytes is not a commitment and must never be treated as one: two keys can collide, so a reader that fails to open a matching block must try the rest. |
| `ephemeral_public_key` | bytes | ephemeral_public_key is 32 bytes of X25519, fresh for this block. Per recipient rather than one shared across the envelope. It costs 32 bytes each and buys two things: the blocks are unlinkable to each other, and a party who later holds the content key can add a recipient — an auditor appointed after the fact, a rotated regulator key — without needing the original ephemeral secret, which nobody kept. |
| `wrapped_key` | bytes | wrapped_key is the 32-byte content key sealed under the agreed wrapping key, so 48 bytes with the Poly1305 tag. Its associated data names the recipient's key_id, so a block cannot be moved between envelopes or reassigned to a different recipient's slot without the tag failing. |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `require_settlement_jurisdiction` | `false` | require_settlement_jurisdiction refuses a payment that does not declare the country whose authority settles it. It has to be a parameter rather than a rule compiled into the binary, for a reason specific to how a chain is replayed: a node syncing from block 0 re-executes every historical payment through today's handler, so a rule that refuses what was once accepted produces a different app hash and the node stops. A parameter is read from state at the height being replayed, so blocks from before the switch replay under the value that was in force when they were made, and blocks after it under the new one. Default false, because the deployments that exist hold payments with no jurisdiction. Governance turns it on once every sender populates the field, and it must be on before the metadata payload is encrypted for real: the declared jurisdiction names the regulator holding the third viewing key, so a payment sent without one is a payment no regulator can ever open. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1100 | `ErrInvalidSigner` | expected gov account as only signer for proposal message |
| 1101 | `ErrApplicationExists` | a participant application already exists for this address |
| 1102 | `ErrApplicationNotFound` | participant application not found |
| 1103 | `ErrApplicationNotPending` | participant application is not pending |
| 1104 | `ErrNotApprovedParticipant` | address is not an approved participant |
| 1105 | `ErrPaymentExists` | a payment with this end-to-end id already exists |
| 1106 | `ErrInvalidAmount` | invalid coin amount |
| 1107 | `ErrNotACustomer` | the debtor does not bank with the instructing participant |
| 1108 | `ErrInvalidPaymentField` | payment field is missing or outside its ISO 20022 limit |
| 1109 | `ErrInvalidSettlementJurisdiction` | settlement jurisdiction is missing or is not an ISO 3166-1 alpha-2 code |
| 1110 | `ErrInvalidMetadata` | payment metadata payload or its hash is malformed |
| 1111 | `ErrConfidentialAmountUnavailable` | confidential amounts are reserved but not yet verified by this chain |
| 1112 | `ErrInvalidPayloadStore` | payload store url must be an absolute http or https base url, or empty |
