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

AppointRegulator names the authority holding the viewing key for a country.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `country` | string | country is an ISO 3166-1 alpha-2 code. Checked against the assigned list for the same reason a jurisdiction is: a mistyped NX would appoint a regulator of nowhere, and every payment declaring NG would go on being encrypted to nobody while the appointment sat in state looking done. |
| `address` | string | address is the appointed authority, expected to be an x/group account wherever the decision to open a payload should be M-of-N rather than one official. The chain does not require that; it records who was named. |

### MsgGrantAuditor

`/blockchain.alias.v1.MsgGrantAuditor`

Signed by the `authority` field.

GrantAuditor grants the time-boxed cross-account reading role.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `address` | string |  |
| `expires_at_height` | int64 | expires_at_height must be in the future. There is no unbounded form of this grant, and no zero-means-forever: a role that can become permanent by leaving a field unset is not time-boxed, it is time-boxed by convention. |

### MsgGrantRole

`/blockchain.alias.v1.MsgGrantRole`

Signed by the `authority` field.

GrantRole grants a role inside one jurisdiction. Governance, or the foundation for a country; governance alone for the chain-wide scope.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance account, or the foundation — the address x/constitution pins as enforcement_recovery_destination. Any other signer is refused, and so is the foundation when the jurisdiction below is "*". |
| `holder` | string | holder is the account being granted the role. |
| `role` | Role | role is what they may do. ROLE_UNSPECIFIED is refused: a grant whose role was left unset must never be honoured, and proto3 cannot tell that from a role that happens to be numbered zero. |
| `jurisdiction` | string | jurisdiction is where: an assigned ISO 3166-1 alpha-2 code, or "*" for chain-wide. The reserved code the foundation's own identifiers carry is refused — it marks the absence of a perimeter, so a grant over it would confer nothing while reading like everything. This field also decides who may sign the message, so it is validated before the signer is checked. "*" narrows the acceptable authority to governance alone, and that narrowing happens before the constitution is read: a store failure while resolving the foundation must not be the thing that decides whether the chain-wide scope was allowed. |

### MsgRegisterAlias

`/blockchain.alias.v1.MsgRegisterAlias`

Signed by the `account` field.

RegisterAlias issues an identifier to the sending account.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

### MsgRegisterViewingKey

`/blockchain.alias.v1.MsgRegisterViewingKey`

Signed by the `account` field.

RegisterViewingKey publishes the sender's X25519 public key, or rotates it.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `public_key` | bytes | public_key is 32 bytes of X25519. The private half never leaves the holder, and nothing on this chain has any use for it. |

### MsgRevokeRole

`/blockchain.alias.v1.MsgRevokeRole`

Signed by the `authority` field.

RevokeRole removes one such grant. The same signers as GrantRole.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string | authority is the governance account, or the foundation. Same rule as MsgGrantRole, including that "*" is governance alone. |
| `holder` | string | holder is the account losing the role. |
| `role` | Role | role is which of the holder's roles to remove. ROLE_UNSPECIFIED is refused here as well as on the grant: "revoke whatever role was left unset" has no meaning, and a message that resolved it to one would revoke something nobody named. |
| `jurisdiction` | string | jurisdiction is which perimeter to remove it in. Naming one that was never granted is an error rather than a quiet success — "nothing to revoke" is how a proposal that named the wrong country passes while leaving the authority it meant to remove in place. As on the grant, this decides who may sign: "*" is governance alone. |

### MsgRevokeViewingKey

`/blockchain.alias.v1.MsgRevokeViewingKey`

Signed by the `account` field.

RevokeViewingKey marks one of the sender's key versions compromised.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |
| `version` | uint64 | version is which registration to mark. Named explicitly rather than defaulting to the newest, because the key an operator wants to revoke is usually the old one they have just rotated away from. |

### MsgRotateAlias

`/blockchain.alias.v1.MsgRotateAlias`

Signed by the `account` field.

RotateAlias retires the sender's identifier and issues a new one.

| Field | Type | Description |
| --- | --- | --- |
| `account` | string |  |

### MsgSetJurisdiction

`/blockchain.alias.v1.MsgSetJurisdiction`

Signed by the `recorder` field.

SetJurisdiction records or corrects the country an account belongs to.

| Field | Type | Description |
| --- | --- | --- |
| `recorder` | string | recorder is the approved participant that onboarded the account, or a foundation administrator, or governance. |
| `account` | string | account is whose perimeter this is. |
| `country` | string | country is an ISO 3166-1 alpha-2 code. It must be one ISO has actually assigned: the reserved codes, ZZ among them, are refused here so that the marker the foundation administrators carry cannot be handed to an ordinary account and read as chain-wide authority. |

### MsgUpdateParams

`/blockchain.alias.v1.MsgUpdateParams`

Signed by the `authority` field.

UpdateParams sets the module parameters. Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

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
| `ROLE_SUPERVISOR` | ROLE_SUPERVISOR is oversight without the power to act — the role held by an auditor or a regulator that watches a perimeter it does not administer. It is granted through the same registry as the rest so that "who is watching this country" has one answer in one place. |

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
| 3 | `ErrNotRegistered` | this account holds no identifier |
| 4 | `ErrNotFound` | no account holds that identifier |
| 5 | `ErrMalformedID` | that is not a well-formed identifier |
| 6 | `ErrInvalidParams` | invalid parameters |
| 7 | `ErrExhausted` | could not derive an unused identifier |
| 8 | `ErrInvalidSigner` | invalid authority for this message |
| 9 | `ErrNoJurisdiction` | this account has no recorded jurisdiction |
