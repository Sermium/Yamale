<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/alias

## Transactions

### MsgAppointRegulator

`/blockchain.alias.v1.MsgAppointRegulator`

Signed by the `authority` field.

MsgAppointRegulator names the authority that holds the third viewing key over payments settling in one country.

Authority-gated, because this is the single most powerful grant the confidentiality design makes: the appointee can read the ISO 20022 detail of every payment that names their country from the moment they are appointed. An account that could appoint itself regulator of Ghana would have granted itself that by sending one message.

The appointee must additionally hold ROLE_SUPERVISOR covering that country — granted in it, or chain-wide. That is the one place where this module's two answers to "who is watching this country" were reconciled rather than left to agree by convention: an appointment is an entry in one map and a role is an entry in a registry, and before this check they could name different accounts with nothing objecting. Requiring the grant makes the registry the source and the appointment the choice of which of its holders also has standing to act.

It also puts the appointment behind the office rules the registry enforces: a role holder must be an x/group account, so a regulator is now M-of-N rather than possibly one key, and an office that has fallen below the shape its grant records cannot be newly appointed.

An appointment already recorded is not revisited. This is a check on the write, not a condition re-read on every payment — a country whose regulator was appointed before the rule existed keeps it, and revoking the supervisor grant of a sitting regulator does not silently unappoint them. Removing a regulator is appointing a different one, visibly, which is what the record should show.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `country` | string | country is an ISO 3166-1 alpha-2 code. Checked against the assigned list for the same reason a jurisdiction is: a mistyped NX would appoint a regulator of nowhere, and every payment declaring NG would go on being encrypted to nobody while the appointment sat in state looking done. |
| `address` | string | address is the appointed authority, expected to be an x/group account wherever the decision to open a payload should be M-of-N rather than one official. The chain does not require that; it records who was named. |

### MsgGrantAuditor

`/blockchain.alias.v1.MsgGrantAuditor`

Signed by the `authority` field.

MsgGrantAuditor grants the time-boxed cross-account reading role.

Authority-gated and bounded in three ways at once — it expires by height, the number of live grants is capped, and every grant is recorded with who made it. All three exist because this role is the one that reads payment detail belonging to people who have no relationship with the holder.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `address` | string |  |
| `expires_at_height` | int64 | expires_at_height must be in the future. There is no unbounded form of this grant, and no zero-means-forever: a role that can become permanent by leaving a field unset is not time-boxed, it is time-boxed by convention. |

### MsgGrantRole

`/blockchain.alias.v1.MsgGrantRole`

Signed by the `authority` field.

MsgGrantRole grants a role inside one jurisdiction.

Two signers, and which one is allowed depends on the scope being granted:

- a **country** scope may be granted by governance or by the foundation;
- the **chain-wide** scope may be granted by governance and nobody else.

"The foundation" here means one specific account: the address x/constitution pins as enforcement_recovery_destination, which is the 3-of-5 group produced by the key ceremony and the account every seized asset on the chain is sent to. It is deliberately NOT a holder of ROLE_FOUNDATION_ADMINISTRATOR, and the difference matters. That role is granted by this very message, so reading the foundation out of it would make "who may appoint a country's authorities" a set that this message could append to — the same circularity a parameter list had, wearing a grant's clothes. An invariant cannot be changed without a constitutional amendment.

#### Why the foundation, when this used to be governance alone

It was governance-only, and the argument was that there is a difference between using a power and deciding who holds one: an administrator naming a country's regulator is operating the rail governance already put them on, whereas an administrator able to grant roles could grant themselves the chain-wide scope and then grant it to anyone. Every widening of who may act went through a vote, in public, one grant at a time.

What that did not survive was enrolling a country. Bringing one country onto the rail is not one grant: it is an M-of-N group per office, two to five role grants across those offices, and the jurisdiction records for the offices' own accounts — a sequence that has to land in a particular order. Under governance-only, each part was a separate proposal that could pass, fail or time out on its own, so the friction was not being paid once per widening of authority but several times over for one decision. The outcome of that is a bundle proposal nobody reads, or a deployment that seeds its grants in genesis and never revisits them.

So admitting a country is the foundation's act: three of five custodians from five organisations, attributable on chain in granted_by. What was given up is publicity and delay — the validator set no longer has a veto over who administers a perimeter, and the appointment no longer sits in public for a voting period first.

Three residual consequences, stated rather than discovered. None of them lets the foundation reach a state governance could not, and all three are bounded by the chain-wide scope staying closed:

1. The foundation can grant one office the same role in every country, one grant at a time, arriving where a chain-wide grant would by a longer road. Bounded by the assigned country list rather than prevented, and enumerable: every grant is listed by RoleGrants and RoleHolders, and the chain-wide ones have an endpoint of their own.
2. It can revoke a country grant GOVERNANCE made — a reduction of the validator set's power, kept because the alternative makes an emergency the expensive case. See MsgRevokeRole.
3. It can grant itself a country role. That is permitted on purpose: a country admitted before its offices exist needs an interim authority, and refusing holder == authority would block that while preventing nothing, since the foundation can appoint any group it controls. Re-granting an existing triple also rewrites granted_by, so that field names the last authority to write the grant rather than the first; the events carry the history.

The holder must be an x/group account. A role is only worth the office that holds it, and an office that is one key is one bribe — which is the same reasoning that makes x/land refuse a registry office that is not a group. That applies to the foundation's grants exactly as it applies to governance's: the widening is about who may sign and about nothing else.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance account, or the foundation — the address x/constitution pins as enforcement_recovery_destination. Any other signer is refused, and so is the foundation when the jurisdiction below is "*". |
| `holder` | string | holder is the account being granted the role. |
| `role` | Role | role is what they may do. ROLE_UNSPECIFIED is refused: a grant whose role was left unset must never be honoured, and proto3 cannot tell that from a role that happens to be numbered zero. |
| `jurisdiction` | string | jurisdiction is where: an assigned ISO 3166-1 alpha-2 code, or "*" for chain-wide. The reserved code the foundation's own identifiers carry is refused — it marks the absence of a perimeter, so a grant over it would confer nothing while reading like everything. One role refuses a country here: ROLE_FOUNDATION_ADMINISTRATOR is chain-wide or nothing. What it exempts is the ABSENCE of a national perimeter, so an administrator of one country would be an account claiming an exemption from a rule it is already inside — and, because chain-wide is governance-only, the refusal is also what keeps that appointment exactly as governance-only as the parameter list it replaced. This field also decides who may sign the message, so it is validated before the signer is checked. "*" narrows the acceptable authority to governance alone, and that narrowing happens before the constitution is read: a store failure while resolving the foundation must not be the thing that decides whether the chain-wide scope was allowed. |
| `required_shape` | OfficeShape | required_shape is the M-of-N the holder's office must keep in order to keep this role. Omit it and no requirement is recorded, which is what every grant made before this field existed carries. Recorded on the grant and re-checked on every action the grant permits, so an office that reduces itself below the shape it was granted under loses the authority automatically rather than waiting for somebody to notice and revoke. See OfficeShape for why both numbers are floors and why the count of signatures is not the same thing as the policy's threshold. Checked here as well, when the group keeper is available: a grant requiring three-of-five is refused outright to a one-of-one office, rather than being written and then failing on first use. Recording a requirement the holder does not meet is how an office ends up holding a grant that reads correct in every query and permits nothing. The requirement is decided before the ceremony rather than read off whoever turned up to it. `ceremony country` takes it per office in the enrolment config and refuses to assemble an office whose signed group file does not meet it — see docs/guides/country-enrolment.md. A requirement captured from the group that happened to be created would be no requirement at all: it would ratify a one-of-one as readily as a three-of-five. |

### MsgRegisterAlias

`/blockchain.alias.v1.MsgRegisterAlias`

Signed by the `account` field.

MsgRegisterAlias claims an identifier for the sending account.

It carries no identifier field on purpose, and no country field either. The chain derives the identifier from the address and takes the country prefix from the jurisdiction already recorded against the account, so there is nothing for the sender to choose, nothing for anyone else to squat, and no way to be issued a prefix that says somewhere you are not. An account with no recorded jurisdiction is refused rather than given a default.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

### MsgRegisterViewingKey

`/blockchain.alias.v1.MsgRegisterViewingKey`

Signed by the `account` field.

MsgRegisterViewingKey publishes the sending account's X25519 public key.

Self-signed, unlike the jurisdiction beside it, and the difference is not an oversight. A jurisdiction is a claim about somebody that they must not be able to make for themselves; a viewing key is a claim only about which key can decrypt payloads addressed to the sender, and an account that publishes a key it does not hold has locked itself out of its own payment detail and nobody else out of anything.

Sending it again rotates: the previous version stays queryable so historical envelopes remain openable, and new envelopes wrap to the new version. See ViewingKey.version.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `public_key` | bytes | public_key is 32 bytes of X25519. The private half never leaves the holder, and nothing on this chain has any use for it. |

### MsgRevokeRole

`/blockchain.alias.v1.MsgRevokeRole`

Signed by the `authority` field.

MsgRevokeRole removes one grant, named exactly.

The same signers as MsgGrantRole, and deliberately not a narrower set: whoever may appoint a country's authority may also remove it, and only governance may touch a chain-wide grant in either direction.

Keeping revocation governance-only while granting was widened was considered and rejected, because it puts the slow path on the wrong action. The reason to revoke in a hurry is that an office's keys are compromised or its authority is being abused, and a rule under which the foundation can appoint a national enforcement authority with one 3-of-5 vote but needs a full governance cycle to remove one makes the emergency the expensive case. Granting authority is the act that wants friction; taking it away is the act that has to be possible on a Sunday.

The jurisdiction is part of what is revoked rather than implied, because a holder may hold the same role in several countries and revoking "their enforcement role" would be ambiguous between removing one perimeter and removing all of them. The signer says which.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance account, or the foundation. Same rule as MsgGrantRole, including that "*" is governance alone. |
| `holder` | string | holder is the account losing the role. |
| `role` | Role | role is which of the holder's roles to remove. ROLE_UNSPECIFIED is refused here as well as on the grant: "revoke whatever role was left unset" has no meaning, and a message that resolved it to one would revoke something nobody named. |
| `jurisdiction` | string | jurisdiction is which perimeter to remove it in. Naming one that was never granted is an error rather than a quiet success — "nothing to revoke" is how a proposal that named the wrong country passes while leaving the authority it meant to remove in place. As on the grant, this decides who may sign: "*" is governance alone. |

### MsgRevokeViewingKey

`/blockchain.alias.v1.MsgRevokeViewingKey`

Signed by the `account` field.

MsgRevokeViewingKey marks one of the sender's key versions compromised.

It does not delete the key and does not pretend the payloads wrapped to it became unreadable — ciphertext that has been distributed cannot be recalled, and a message that implied otherwise would be worse than none. What it does is stop senders wrapping to it and let a reader see that a payload they are holding was addressed to a key somebody else may also hold. Erasing the payload itself is the store's job, not the chain's.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `version` | uint64 | version is which registration to mark. Named explicitly rather than defaulting to the newest, because the key an operator wants to revoke is usually the old one they have just rotated away from. |

### MsgRotateAlias

`/blockchain.alias.v1.MsgRotateAlias`

Signed by the `account` field.

MsgRotateAlias retires the sender's current identifier and issues a new one.

For an account whose key was compromised: the old identifier is tombstoned and never issued again, so a payment sent to the handle somebody memorised arrives nowhere rather than in a stranger's account. That is the whole reason retiring is allowed and repointing is not.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

### MsgSetJurisdiction

`/blockchain.alias.v1.MsgSetJurisdiction`

Signed by the `recorder` field.

MsgSetJurisdiction records or corrects the country an account belongs to.

The first recording is made by the approved participant that onboarded the account, because that is the party that performed the KYC and therefore the only one that knows the answer. It is never self-declared: an account that could name its own perimeter could name the one with no authority watching it.

A correction — somebody moves, a firm redomiciles — is a foundation administrator's act, not the participant's. A participant that could rewrite a country it had already recorded could move a customer out from under an authority mid-investigation.

| Field | Type | Description |
| --- | --- | --- |
| `recorder` | string | recorder is the approved participant that onboarded the account, or a foundation administrator, or governance. |
| `account` | string | account is whose perimeter this is. |
| `country` | string | country is an ISO 3166-1 alpha-2 code. It must be one ISO has actually assigned: the reserved codes, ZZ among them, are refused here so that the marker the foundation administrators carry cannot be handed to an ordinary account and read as chain-wide authority. |

### MsgUpdateParams

`/blockchain.alias.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the module parameters. Governance only.

It used to be the only way a foundation administrator was appointed or removed, which made it the most consequential message in this module. That appointment is now MsgGrantRole with ROLE_FOUNDATION_ADMINISTRATOR and the chain-wide scope — still governance and nobody else, because a chain-wide grant is governance's alone. What is left here is one parameter.

#### It replaces the whole Params object

`params` is a message, not a field mask, and there is no partial update. So "appoint one administrator" is really "read the current parameters, add one address, and resubmit every parameter" — and the failure mode is not an error. A proposal composed without reading the current values passes, and silently drops the administrators already appointed or resets payload_length to its default. Nothing here catches that: Params.Validate() bounds the list and refuses duplicates, and a shorter list than before is a perfectly valid list.

Two consequences worth knowing before composing one by hand:

- Read `Query/Params` first and carry every field across. A field left out of the message is a field set to its protobuf default, not a field left alone.
- A `payload_length` that reads back as 0 means the value is UNKNOWN, not zero: proto3 cannot tell a zero from a field nobody filled in. Resubmitting a guess would re-parameterise the chain. Validate() refuses a zero, so a chain never actually holds one.

clients/governance composes this message with the whole object shown before and after, and refuses rather than defaulting when the current parameters cannot be read.

Note that this message no longer appoints anybody. Field 2 held foundation_administrators and is reserved; the appointment is MsgGrantRole with ROLE_FOUNDATION_ADMINISTRATOR and the chain-wide scope, which is governance-only for the same reason every chain-wide grant is. The authority behind the act did not move — a chain-wide grant is governance's alone. What moved is where it is recorded, and that is the point: the appointment and the role registry are one mechanism now rather than two lists that happened to share a name.

Two things were wrong with the list beyond the whole-object hazard above, and they are what the move was for. Validate() never checked that an entry was an address, so a mistyped one passed a vote, occupied one of the eight places and granted the exemption to nobody; MsgGrantRole decodes the holder. And an administrator was a bare address, where a role holder must be an x/group account — so the exemption is now held M-of-N like every other authority on this chain.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance module account. Any other signer is refused, so a proposal naming the wrong address passes its vote and is then refused when it executes — reported in a transaction log nobody is watching. |
| `params` | Params | params is the COMPLETE parameter set, replacing whatever is there. See above: every field omitted is a field reset, not a field preserved. |

## Queries

### Alias

`GET /yamale/blockchain/alias/v1/alias/{id}`

Alias resolves an identifier to an address. The hot path.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | string | id in any form the client has it: hyphenated, lower case, with the confusable characters unfolded. The module normalises before looking up. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `alias` | Alias |  |

### AliasOf

`GET /yamale/blockchain/alias/v1/alias_of/{address}`

AliasOf returns the identifier held by an address, for interfaces that need to show a person their own handle.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `alias` | Alias |  |

### Auditors

`GET /yamale/blockchain/alias/v1/auditors`

Auditors lists the grants that have not expired, with their current keys.

A list endpoint in a module that avoids them, and it is the right call here: who may read across accounts is a fact the people being read about are entitled to see, and a sender cannot build a correct envelope without the whole set.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `auditors` | repeated AuditorEntitlement |  |

### ChainWideGrants

`GET /yamale/blockchain/alias/v1/chain_wide_grants`

ChainWideGrants lists every grant whose scope is "*".

Its own endpoint, taking no argument, precisely because the chain-wide scope is the exception. An exception that can only be found by knowing to ask for the wildcard is an exception nobody audits, so a governance console can put this on a page and show the whole set of accounts that are not bounded by any border.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `grants` | repeated RoleGrant |  |

### Jurisdiction

`GET /yamale/blockchain/alias/v1/jurisdiction/{address}`

Jurisdiction reports the country recorded against one account.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jurisdiction` | Jurisdiction |  |

### Params

`GET /yamale/blockchain/alias/v1/params`

Params returns the module parameters.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

### PayloadReaders

`GET /yamale/blockchain/alias/v1/payload_readers/{country}`

PayloadReaders lists every account entitled to be sealed into the encrypted payload of a payment that settles in one country, with the key each of them reads through.

This is what makes ROLE_SUPERVISOR a role rather than a name in a registry. A grant of it covering a country is an entitlement to read that country's payment detail, and the entitlement is realised by a sender wrapping the content key to the holder — so the set has to be resolvable in one call, at the moment of sending, from the chain rather than from a list somebody maintains beside it.

Two sources, one answer. The appointed regulator of the country is here because that is the authority with standing to act on the payment; every holder of ROLE_SUPERVISOR covering the country is here because oversight is what the role is. A chain-wide supervisor appears for every country, which is what a chain-wide grant means and why it is listed on its own endpoint for anyone auditing the exceptions.

Deliberately NOT here: the auditors. They are time-boxed, chain-wide and unrelated to any settlement jurisdiction, and folding them in would put an expiry rule in two responses that could come to disagree — see Auditors, which a sender calls as well. A complete envelope is this set plus that one plus the payer and the payee.

An empty response is a real answer: a country with no appointed regulator and no supervisor has nobody entitled to read, and the payment is readable by its two parties and any live auditor. It is not an error and a sender must not retry it.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `country` | string | country is an ISO 3166-1 alpha-2 code from the assigned list, in any case. The chain-wide marker and the foundation's reserved code are refused, for the same reason AssertScopeIn refuses them: no payment settles chain-wide, and a payment declaring the absence of a national perimeter would be one no authority is accountable for. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `readers` | repeated PayloadReader |  |

### Perimeter

`GET /yamale/blockchain/alias/v1/perimeter/{country}`

Perimeter lists the accounts recorded in one country, so an authority can see the accounts it may act on and no others.

This is a list endpoint in a module that deliberately has none, so note what it returns and what it does not: jurisdiction records, never identifiers. Walking it tells you which addresses are recorded in a country — a supervisory fact, and the reason the endpoint exists — and gives no shortcut to the alias directory that is withheld above.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `country` | string | country is an ISO 3166-1 alpha-2 code, in any case. |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `jurisdictions` | repeated Jurisdiction |  |
| `pagination` | PageResponse |  |

### Regulator

`GET /yamale/blockchain/alias/v1/regulator/{country}`

Regulator returns the authority appointed over one country, with its current viewing key.

Both in one response because the sender needs both and asking separately invites the half-answer: an appointment resolved, a key not fetched, and an envelope built without the regulator on it that looks complete.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `country` | string | country is an ISO 3166-1 alpha-2 code, in any case. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `appointment` | RegulatorAppointment |  |
| `key` | ViewingKey | key is the appointee's current viewing key. Its public_key is empty when the regulator has been appointed but has not published one — a state a sender has to be able to see, because wrapping to a key of thirty-two zero bytes would produce an envelope that looks addressed to the regulator and opens for nobody. |

### Retired

`GET /yamale/blockchain/alias/v1/retired/{id}`

Retired reports whether an identifier has been tombstoned. A client that resolves nothing should be able to tell "never existed" from "existed and was retired", because those mean very different things to somebody about to send money.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `retired` | bool |  |

### RoleGrants

`GET /yamale/blockchain/alias/v1/role_grants/{holder}`

RoleGrants lists every role one account holds, and where.

The question an operator actually asks about a key before trusting it, and the question a holder asks to find out what the chain thinks they may do. Empty is a real answer: an account with no grants may act nowhere.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `grants` | repeated RoleGrant |  |

### RoleHolders

`GET /yamale/blockchain/alias/v1/role_holders/{jurisdiction}`

RoleHolders lists who holds roles inside one jurisdiction.

The supervisory view: "who may act on my country's accounts". Chain-wide grants are deliberately *not* folded in — they are listed on their own endpoint — because a country's own list should show what that country granted, and silently mixing in the chain-wide exceptions would hide them among the ordinary entries of every country at once.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `jurisdiction` | string | jurisdiction is an ISO 3166-1 alpha-2 code, in any case. |
| `role` | Role | role narrows the answer to one role. Left unspecified it means every role, which is the only sense in which the zero value is accepted anywhere: here it is a filter that has not been applied, never a grant. |
| `pagination` | PageRequest |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `grants` | repeated RoleGrant |  |
| `pagination` | PageResponse |  |

### ViewingKeys

`GET /yamale/blockchain/alias/v1/viewing_keys/{address}`

ViewingKeys returns every version of one account's viewing key, newest first.

Every version, not just the live one, because a payload encrypted last year is wrapped to the key that was live last year. A client that could only fetch the current key would report an old but perfectly readable payment as undecryptable, which is the failure the version field exists to prevent.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `address` | string |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `keys` | repeated ViewingKey |  |

## State

### Alias

Alias binds one identifier to one address, permanently.

One name, one address. One identifier per address. Neither is ever repointed — an alias can be retired, and a retired identifier is never issued again, but it cannot be made to resolve somewhere else. That is the property a payment handle needs: an identifier somebody memorised last year either still means what it meant, or it means nothing.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string | id is the normalised identifier: uppercase, no separators, ISO 3166-1 alpha-2 country prefix first, check character last. The display form (NG-K3M9-7QRT-B) is a client concern; the chain stores and compares only this. The country is not stored as its own field, because two copies of one fact can disagree and the one in the identifier is the one people read. It is the first two characters, and it is true by construction: the chain refuses to issue an identifier whose prefix differs from the jurisdiction recorded against the address, and a correction to that jurisdiction retires the identifier rather than leaving it to age into a lie. The prefix is not Crockford Base32 and is not folded like the payload. A fold that turned I into 1 and O into 0 would map CI and CL onto the same prefix, and SI and SL onto another — Côte d'Ivoire indistinguishable from Chile, Slovenia from Sierra Leone. The prefix uses all 26 letters; only the payload after it is Crockford. |
| `address` | string | address is the account it resolves to, and never changes. |
| `registered_at_height` | int64 | registered_at_height records when it was issued, so a client can show how long an identifier has been in use — a handle registered minutes ago deserves more caution than one that has been answering for a year. |

### AuditorEntitlement

AuditorEntitlement pairs a grant with the key it reads through.

| Field | Type | Description |
| --- | --- | --- |
| `grant` | AuditorGrant |  |
| `key` | ViewingKey | key carries an empty public_key when the auditor has been granted the role but has published no key, for the same reason as on the regulator above. |

### AuditorGrant

AuditorGrant is the time-boxed role that may read payment payloads across accounts, for aggregate checks no single party's own keys can perform.

It expires by itself. A grant that has to be revoked to end is a grant that stays live when the person who would have revoked it moves on, and this one reads the detail of payments belonging to people who never dealt with the holder — so the failure mode of forgetting is the whole population's remittance text, indefinitely.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string | address is the granted account. |
| `granted_by` | string | granted_by is the authority that granted it. |
| `granted_at_height` | int64 | granted_at_height is when the grant was made. |
| `expires_at_height` | int64 | expires_at_height is the first height at which the grant no longer holds. Required and strictly in the future, never zero. Zero would read as "no expiry" to anyone who did not check, which is the one thing a time-boxed role must not be able to become by omission. |

### Jurisdiction

Jurisdiction is the national perimeter an account belongs to.

It lives here rather than in x/paymsg because every module that has to refuse an out-of-perimeter action — the land registry, enforcement, stablecoin issuance — would otherwise have to ask the payments module where an account is. A perimeter that only exists for accounts that happen to be somebody's payment customer is not a perimeter: validators, registry offices and treasury signers are none of those, and each one would sit in the unclaimed state the whole design exists to remove.

It is recorded by the participant that onboarded the account, never self-declared, and changed only by a foundation administrator. That is what makes it evidence rather than a preference.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string | address is the account this perimeter belongs to. |
| `country` | string | country is an ISO 3166-1 alpha-2 code, uppercase, checked against the assigned list. A code that is merely two letters would let a typo — NX for NG — become a perimeter no authority holds and no authority can act on. |
| `recorded_by` | string | recorded_by is the participant or administrator that put it there. Kept because "who says this account is Nigerian" is the question an authority asks when the answer turns out to be wrong, and an unattributed record cannot answer it. |
| `recorded_at_height` | int64 | recorded_at_height is when it was last written, so a change is visible as a change rather than as a fact that was always there. |

### OfficeShape

OfficeShape is an M-of-N, written down before the ceremony and held to afterwards.

Both numbers are FLOORS, not equalities, and the asymmetry is the design. Adding a member to an office is fine: a three-of-five that becomes a three-of-six is more people, not fewer, and refusing it would mean an office could never grow without the foundation re-granting its roles. Dropping below either number is not fine, and it is the same rule the constitution applies to the foundation's own group, for the same reason: three-of-five becoming three-of-four moves sixty per cent to seventy-five and walks towards unanimity, where one unreachable member freezes the office; and a threshold falling to one is the single key the whole arrangement exists to abolish.

What it does NOT constrain is an office tightening itself. An office that votes three-of-five up to five-of-five satisfies both floors and can then be frozen by one absent member — which is self-harm rather than capture, and a rule that refused it would be a rule stopping an office from being more careful than it was asked to be.

| Field | Type | Description |
| --- | --- | --- |
| `signatures` | uint32 | signatures is the fewest members that must sign for the office to act. Checked against how many members it would actually TAKE, which is not the same as the policy's threshold number. x/group votes are weighted, so a group with a threshold of 3 whose members weigh 3, 1, 1, 1 and 1 is a one-of-five wearing a three-of-five's clothes: one member reaches the threshold alone. The check therefore takes the members in descending weight and counts how few of them can reach the threshold. For the equal weights every ceremony produces that is exactly the threshold, and for anything else it is the honest answer to "how many people does it take". Zero is refused rather than read as "no requirement". A requirement that requires nothing reads on a record as though it covered something, and the way to say "no requirement" is to omit required_shape entirely. |
| `members` | uint32 | members is the fewest members the office must have. Counted over members holding a positive weight, because a member who cannot vote is a name on a list rather than a share of an office — and padding a group with weightless members is the obvious way to satisfy a count while shrinking the number of people who actually decide. Must be at least signatures: a requirement of three signatures from two members describes an office that could never act, and a requirement nothing can satisfy is a requirement that will be waived by whoever hits it. |

### PayloadReader

PayloadReader is one account entitled to open a country's payment payloads, and the reason it is entitled.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string | address is the account whose viewing key the payload must be wrapped to. |
| `basis` | PayloadReaderBasis | basis says why this account is in the list, because the two reasons carry different consequences and a client showing the set to a person has to be able to say which is which. A regulator can also act on the payment; a supervisor can only read it. |
| `scope` | string | scope is the jurisdiction of the grant that entitles a supervisor: the country asked about, or "*" for a chain-wide grant. Empty for the appointed regulator, whose entitlement comes from the appointment rather than from a scope. It is here so that an operator reading the list can tell a supervisor this country granted from one that is chain-wide and appears in every country's list — which is exactly the distinction ChainWideGrants exists to keep visible, and it would be lost if the two rendered identically here. |
| `key` | ViewingKey | key is the reader's current viewing key. Its public_key is empty when the account is entitled but has published none — a state a sender has to be able to see, because wrapping to a key of thirty-two zero bytes produces an envelope that looks addressed to them and opens for nobody. |

### RegulatorAppointment

RegulatorAppointment names the authority that holds the third viewing key over every payment declaring one country as its settlement jurisdiction.

One per country, deliberately. The settlement jurisdiction is the field that settles which authority may act on a cross-border payment, and a country with two appointed regulators would reintroduce exactly the contest over standing that the single declaration exists to end.

| Field | Type | Description |
| --- | --- | --- |
| `country` | string | country is an ISO 3166-1 alpha-2 code from the assigned list. |
| `address` | string | address is the appointed authority. Its viewing key is looked up separately, so a regulator can rotate keys without being reappointed. |
| `appointed_by` | string | appointed_by is the authority that made the appointment. Kept because "who says this account regulates Nigeria" is the question asked when the answer turns out to be wrong, and an unattributed appointment cannot answer it. |
| `appointed_at_height` | int64 | appointed_at_height is when it was last written, so a change to who regulates a country is visible as a change rather than as a fact that was always there. |

### RoleGrant

RoleGrant is the triple the whole perimeter is built from: who, what role, where.

It is created and removed by governance and by nobody else. A role its own holder could grant is not a perimeter, and neither is one another holder of the same role could hand out — buying one office would buy the power to manufacture the rest.

| Field | Type | Description |
| --- | --- | --- |
| `holder` | string | holder is the granted account. Expected to be an x/group account wherever the decision the role carries should be M-of-N rather than one official's key; that expectation is checked at grant time rather than trusted, for the same reason x/land refuses a registry office that is a plain key. |
| `role` | Role | role is what the holder may do. Never ROLE_UNSPECIFIED. |
| `jurisdiction` | string | jurisdiction is where the holder may do it: an ISO 3166-1 alpha-2 country code from the assigned list, or "*" for chain-wide. The chain-wide form is the exception and is meant to stay rare. It has its own query endpoint so a governance console can list every one of them without having to know how to spell the wildcard — an exception nobody can enumerate is an exception nobody audits. Note what cannot appear here: the foundation's reserved code. That code marks the *absence* of a national perimeter, so a grant naming it would be a grant over nowhere, and it would read to a human as chain-wide authority while conferring none. Chain-wide is spelled "*" and only "*". |
| `granted_by` | string | granted_by is the authority that made the grant. Kept because "who says this account may freeze Nigerian accounts" is the question asked when the answer turns out to be wrong, and an unattributed grant cannot answer it. |
| `granted_at_height` | int64 | granted_at_height is when it was written, so a widening of somebody's powers is visible as a change rather than as a fact that was always there. |
| `required_shape` | OfficeShape | required_shape is the M-of-N the holder's group must keep in order to keep this authority. Absent means no requirement was recorded. Every check reads it: the perimeter functions resolve the holder's group policy and refuse when the office has fallen below what is written here. That is the whole reason the field exists rather than being a rule applied once at grant time. An office is a group that administers itself, so its members can vote to change their own threshold — a country can hold a proper ceremony, stand up a three-of-five enforcement authority, and that office can later vote itself to one-of-one while keeping the power to freeze accounts. Nothing was notified and nothing refused it, because the only thing the chain ever checked was that the holder *was* a group. A jurisdiction stamped once at account creation and never re-examined is an event; a perimeter is a state, and so is an office's shape. It is a message rather than two integers for one reason, and the reason is presence. A message field is either there or it is not, on the wire and in Go, so "no requirement was recorded" and "a requirement of zero" are different states that no reader can confuse. Two bare uint32 fields could not express that: proto3 cannot tell a zero from a field nobody filled in, which is the trap this repository has been caught by four times, and the permissive reading of an ambiguous zero is exactly the reading an attacker wants. Grants written before this field existed decode with it absent and are unchanged in effect — see OfficeShape for what that costs. |

### ViewingKey

ViewingKey is the X25519 public key a payment payload is encrypted to.

It lives in this module for the same reason the jurisdiction does. The payload is encrypted at the moment the payment is sent, so the sender has to be able to look up every recipient's key *then* — the payer's, the payee's, the key of the regulator of the declared settlement jurisdiction, and every live auditor's. A registry that only covered accounts which happen to be somebody's payment customer would leave the regulator and the auditor unresolvable, which are two of the three parties the design exists to serve.

Only the public half is ever recorded. That is not a convention that could be relaxed later: a private key on an append-only ledger is a private key published to everyone forever, and there is no erasure path that takes it back.

| Field | Type | Description |
| --- | --- | --- |
| `address` | string | address is the account that holds the matching private key. |
| `version` | uint64 | version climbs by one per registration and is never reused. Every envelope names the version it wrapped a content key to, so a payload encrypted years ago still says which key opens it. Reusing a number would make that reference ambiguous, and the party holding the wrong half would see an authentication failure rather than a resolvable "you need the older key" — indistinguishable from a corrupted payload. |
| `public_key` | bytes | public_key is 32 bytes of X25519. Checked for length rather than assumed: a shorter value is not a key, and a sender that wrapped a content key to it would produce an envelope nobody can ever open, discovered only by the party who needed to read it. |
| `registered_at_height` | int64 | registered_at_height is when this version was published. Senders must not encrypt to a key that was not yet published at the height they are sending, and a reader auditing an old envelope needs to know which keys were available when it was written. |
| `revoked` | bool | revoked marks a key whose private half is believed compromised. Revocation is not rotation, and conflating them loses the record. A revoked key must not be wrapped to again — but the envelopes already wrapped to it stay wrapped to it, because ciphertext that exists cannot be recalled. The flag is therefore a warning to senders and an exposure marker for readers, never a claim that old payloads became unreadable. A boolean beside the height rather than "height != 0", which is what this field was first. Proto3 cannot tell a height of zero from an unset field, so a key revoked at height zero — a genesis-seeded revocation, or any revocation on a chain that has not produced a block — read back as live. That failure is silent and points the wrong way: senders go on sealing payment detail to a key its holder has already declared compromised. |
| `revoked_at_height` | int64 | revoked_at_height is when it happened, and is meaningful only when revoked is set. Kept because "when did this key stop being trustworthy" decides which stored payloads are exposed, and a bare boolean cannot answer it. |

## Value types

### PayloadReaderBasis

PayloadReaderBasis is why an account may read a country's payment payloads.

| Value | Meaning |
| --- | --- |
| `PAYLOAD_READER_UNSPECIFIED` | PAYLOAD_READER_UNSPECIFIED is never returned. The zero value is reserved for the same reason Role's is: proto3 cannot tell a zero from a field nobody filled in, and a reader whose basis defaulted to the first of the list would be indistinguishable from one whose basis nobody set. |
| `PAYLOAD_READER_REGULATOR` | PAYLOAD_READER_REGULATOR is the authority appointed over the country by MsgAppointRegulator. One per country, and the one with standing to act. |
| `PAYLOAD_READER_SUPERVISOR` | PAYLOAD_READER_SUPERVISOR holds ROLE_SUPERVISOR covering the country — either granted in it or chain-wide. Oversight without the power to act. |

### Role

Role is a power an account may be granted inside a perimeter.

The list is short and closed on purpose. A role is only worth having if some module refuses an action without it, so a role nothing consults is a name in a registry pretending to be a control — and the way that happens is by letting the set grow faster than the checks.

| Value | Meaning |
| --- | --- |
| `ROLE_UNSPECIFIED` | ROLE_UNSPECIFIED is the unset default and is never valid. The zero value is reserved rather than given to a real role, and every path refuses it. Proto3 cannot tell a zero from an absent field, so a grant whose role happened to be the first of the list would be indistinguishable from a grant whose role nobody filled in — and the second must never be honoured. |
| `ROLE_REGISTRY_AUTHORITY` | ROLE_REGISTRY_AUTHORITY is a lands commission or cadastral office: registering a parcel, validating a transfer, freezing land. |
| `ROLE_MONETARY_AUTHORITY` | ROLE_MONETARY_AUTHORITY is a central bank: admitting the issuer of a currency inside its jurisdiction. |
| `ROLE_PAYMENTS_AUTHORITY` | ROLE_PAYMENTS_AUTHORITY admits the institutions that may appear on a payment instruction. Separate from the monetary authority because licensing a payment service provider and issuing money are different offices in most of the deployments this chain is built for, and collapsing them here would force one key to hold both. |
| `ROLE_ENFORCEMENT_AUTHORITY` | ROLE_ENFORCEMENT_AUTHORITY may stop an account: opening a case against it, or freezing it outright. |
| `ROLE_SUPERVISOR` | ROLE_SUPERVISOR is oversight without the power to act — the role held by an auditor or a regulator that watches a perimeter it does not administer. It is granted through the same registry as the rest so that "who is watching this country" has one answer in one place. What it confers is a READING entitlement, and the shape of it is decided by a fact about gRPC rather than by preference: a query carries no signer, so the chain cannot gate a read by role and any design that pretended to would be worse than an empty role. Ordinary read access on a deployment is controlled at the proxy. So the entitlement is over the one body of data the chain does NOT serve to whoever asks: the encrypted payload of a payment. A holder of this role in country X is entitled to be a viewing-key recipient of every payload whose declared settlement jurisdiction is X — the same declaration that decides which authority may act on a cross-border payment, which is what makes it the right field. The set is published by Query/PayloadReaders, so a sender resolves "who must this be sealed to" from the grant registry rather than from a list somebody maintains. Two consequences the chain can actually enforce, and one it cannot: - a country's appointed regulator must hold this role covering that country. AppointRegulator refuses otherwise, so the most powerful grant the confidentiality design makes cannot be handed to an account that holds nothing in the perimeter registry, and "who is watching this country" really does have one answer in one place. - a grant of it is made by governance or by the foundation like any other, and revoking it removes the holder from every future payload's recipient set. - it cannot force a sender to seal to the holder. The envelope is built off-chain and the chain only ever sees a hash of the plaintext, so a sender that ignores the published set produces a payload the supervisor can never open, and nothing on chain detects it. That is a limit of where the ciphertext lives, not a gap in the registry. And it confers nothing else. A supervisor may not open an enforcement case, freeze, seize, register or validate land, admit an issuer or a participant, grant a role, correct a jurisdiction, or appoint a regulator. Oversight without the power to act, which is what the name says. |
| `ROLE_FOUNDATION_ADMINISTRATOR` | ROLE_FOUNDATION_ADMINISTRATOR is the chain-wide authority that places accounts: it may correct an account's recorded country, appoint a country's regulator, grant the time-boxed auditor role, and hold an identifier with no country behind it. It replaces the alias module's foundation_administrators parameter, which was a list of up to eight addresses appointed by MsgUpdateParams. Collapsing the two mechanisms is the point. Enrolling a country needed BOTH the foundation group (to grant roles) and a foundation administrator (to place the offices' own accounts); they were unrelated lists that happened to share a name, and holding one without the other produced a proposal that passed and an office that could not work. It is CHAIN-WIDE OR NOTHING. A grant of it naming a country is refused, and that refusal is what preserves today's authority exactly: by the rule in GrantRole, a chain-wide scope may be granted by governance and by nobody else — which is the same body that could edit the parameter list. The foundation's power to admit a country is untouched and is not widened into the power to appoint the accounts that stand outside every country. A country scope would also be meaningless rather than merely wrong. What the role exempts is the ABSENCE of a national perimeter, so an administrator of one country is an account claiming an exemption from a rule it is already inside. The cap the parameter carried survives the move — see MaxFoundationAdministrators — because the reason for it survives: this is the single exception to "every account has a jurisdiction", and it must not be possible to widen it without somebody seeing. |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 10 | `ErrInvalidCountry` | that is not an assigned ISO 3166-1 alpha-2 country code |
| 11 | `ErrNotTheRecorder` | only the account's approved participant or a foundation administrator may record its jurisdiction |
| 12 | `ErrJurisdictionSet` | this account's jurisdiction is already recorded; only a foundation administrator may correct it |
| 13 | `ErrInvalidViewingKey` | that is not a 32-byte X25519 public key |
| 14 | `ErrViewingKeyNotFound` | this account has published no viewing key at that version |
| 15 | `ErrNoRegulator` | no regulator is appointed for that country |
| 16 | `ErrInvalidAuditorGrant` | an auditor grant must expire at a future height, and only so many may be live at once |
| 17 | `ErrInvalidRole` | that is not a role that can be held, and an unset role is never a default |
| 18 | `ErrInvalidScope` | a role's jurisdiction must be an assigned country code or the chain-wide marker |
| 19 | `ErrOutOfScope` | this account holds no grant of that role covering that jurisdiction |
| 2 | `ErrAlreadyRegistered` | this account already holds an identifier |
| 20 | `ErrHolderNotGroup` | a role holder must be an x/group account, so that acting on it is M-of-N |
| 21 | `ErrGrantNotFound` | no such grant: this account does not hold that role in that jurisdiction |
| 22 | `ErrNoScopeKeeper` | the jurisdictional perimeter cannot be checked because the registry is not wired in |
| 23 | `ErrOfficeShape` | an office's M-of-N and the shape its authority requires do not agree |
| 3 | `ErrNotRegistered` | this account holds no identifier |
| 4 | `ErrNotFound` | no account holds that identifier |
| 5 | `ErrMalformedID` | that is not a well-formed identifier |
| 6 | `ErrInvalidParams` | invalid parameters |
| 7 | `ErrExhausted` | could not derive an unused identifier |
| 8 | `ErrInvalidSigner` | invalid authority for this message |
| 9 | `ErrNoJurisdiction` | this account has no recorded jurisdiction |
