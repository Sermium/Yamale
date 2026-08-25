# Roles, stakeholders and jurisdictional perimeter

It answers one requirement from the revised scope: multiple stakeholders hold
roles that are bounded by country, so a national authority can act on its own
jurisdiction's accounts and records and on nobody else's.

**Status.** All three pieces are built, in `x/alias`. Piece 1 is the jurisdiction
on the account, the country prefix in the identifier, and both of the decided
rules. Piece 2 is the grant registry: a `(holder, role, jurisdiction)` triple,
created and removed by governance — or, for a country, by the foundation — with
the chain-wide scope reserved to governance and listed on its own query endpoint
because it is the exception. Piece 3 is `AssertScope`, which
`x/land`, `x/enforcement`, `x/stablecoin` and `x/paymsg` route their authority
actions through — in their message servers, not in an ante decorator, because an
ante gate does not see the messages that arrive through an interchain account or
an `authz` grant.

What this changed for existing behaviour, stated plainly because it is
consensus-breaking: a bonded validator can no longer open an enforcement case
against an account outside the jurisdiction governance granted it, a registry
office can no longer act without a grant covering the country it administers, and
an account with no recorded jurisdiction cannot be acted on by anybody. A
deployment therefore has to seed its grants — genesis carries them — or its
authority actions are refused from block one.

The registry the roles are scoped against had to come first: a grant naming a
country is meaningless until the chain knows which country an account is in.

---

## The requirement

A deployment carries many kinds of actor — a central bank, a lands commission, a
supervisor, commercial participants, agents, ordinary users. Their powers are not
chain-wide. Ghana's lands commission has no business freezing a Kenyan parcel,
and Nigeria's central bank has no business acting on a Senegalese account. The
chain must be able to *refuse* those actions, not merely discourage them.

So the perimeter has to be known to the state machine, and it has to be attached
to the account.

## The country code goes in the identifier, not the address

**Decided.** The visible country marker is real and required — sovereignty is the
point of the deployment, and an operator must be able to see at a glance which
perimeter an account belongs to. It lives in the `x/alias` identifier:

```
NG-K3M9-7QRT-B
```

Not in the address, for a reason that is concrete rather than aesthetic.

A bech32 address is `yml1` followed by the base32 encoding of a **hash of the
public key**. Nothing in it is chosen; it is derived. And **bech32 has no `b`, no
`i` and no `o`** — they are excluded from its alphabet. So `BEN`, `CIV`, `MOZ`,
`SOM`, `GIN`, `LBR`, `BFA`, `COD` and `TGO` cannot be written in an address at
all. A prefix scheme would have to fall back to an arbitrary token table, at
which point it is no longer readable and the reason for wanting it is gone.

The alias does not have that problem, and it is better on every other axis too:

- It is **chain state**, so registration can refuse an alias whose country does
  not match the account's registered jurisdiction. The marker cannot be a lie,
  because the chain never issues a lying one.
- It uses the **full alphabet**, so every ISO code fits.
- It is **correctable** when somebody moves or a firm redomiciles, without
  reissuing a key.
- It **breaks no tooling** — no forked address codec, no explorer or wallet
  rewrite.
- It is already the thing users read, share and type. In this design the address
  is abstracted away from them entirely.

The underlying principle still holds and is what rules out the address version:
**a string is not enforcement.** The chain refuses things by consulting state.
The alias works precisely because it *is* state, checked against the jurisdiction
registry when it is issued.

## The shape that works

Three pieces, each small.

### 1. Jurisdiction on the account record — built

Record an ISO 3166-1 alpha-2 country against the account. This system already has
the right moment to capture it — accounts do not appear from nowhere here. A user
is onboarded by an approved participant, which has already performed KYC and
already knows the country; `x/alias` already registers an identifier, and
`x/paymsg` already registers customers against participants.

The jurisdiction is **set by the onboarding participant, not self-declared**, and
changed only by an authority, leaving a record. That is what makes it evidence
rather than a preference.

The record lives in **`x/alias`**, not in `x/paymsg`, though the participant is
still the one who writes it. Two reasons. The perimeter has to be readable by
every module that refuses an out-of-perimeter action — the land registry,
enforcement, stablecoin issuance — and none of them should have to ask the
payments module where an account is. And a jurisdiction keyed on the customer
record would be undefined for every account that is not somebody's payment
customer: validators, registry offices, treasury signers. Those are exactly the
unclaimed states the rules below abolish. `x/alias` reads a two-method,
read-only view of `x/paymsg` to check who onboarded an account; the dependency
runs one way and moves no money.

A correction retires the account's identifier and issues a replacement in the
same message. It has to: the prefix would otherwise name a country the chain no
longer records, and a prefix that can go stale is a prefix that can lie. Every
existing property of the module survives — the old identifier is tombstoned
rather than repointed, and is never issued again.

### 2. Roles scoped to a jurisdiction — built

A grant is a triple — **who, what role, where**:

```
(yml1lands…, ROLE_REGISTRY_AUTHORITY,   GH)
(yml1cbn…,   ROLE_MONETARY_AUTHORITY,   NG)
(yml1audit…, ROLE_SUPERVISOR,           *)
```

`*` means chain-wide and should be rare, granted visibly, and listed on the
governance console precisely because it is the exception. It has a query endpoint
of its own — `chain-wide-grants`, which takes no argument — so the whole set of
accounts that no border bounds fits on one page. An exception that can only be
found by knowing to ask for the wildcard is an exception nobody audits.

The five roles are registry authority, monetary authority, payments authority,
enforcement authority and supervisor. The zero value of the enum is reserved as
unspecified and refused everywhere: proto3 cannot tell a zero from an absent
field, so a role numbered zero would make "grant the first role in the list" and
"grant whatever the default is" the same message.

**Two of the five conferred nothing, and both now confer something.** `role.proto`
keeps the list short on the stated grounds that "a role nothing consults is a
name in a registry pretending to be a control", and for a while
`ROLE_SUPERVISOR` was exactly that, with no consumer anywhere, while
`ROLE_ENFORCEMENT_AUTHORITY` was nearly that: its only two consumers,
`MsgOpenCase` and `MsgEmergencyFreeze`, each imposed a second condition an office
could not satisfy — being a bonded validator, or being `x/enforcement`'s
`emergency_authority` parameter — so a country's enforcement office held a grant
no message let it use.

What closed the enforcement half was the second condition going away. `MsgOpenCase`
accepts a bonded validator **or** a holder of the role covering the target's
country, and `MsgEmergencyFreeze` is authorised by the role instead of by a
parameter naming one chain-wide address. Both changes are widenings of *who may
accuse* and neither touches *who decides*: two thirds of bonded voting power
still resolves every case, and an office that is not a validator has no vote at
all. A national authority can stop money for a day; the set decides whether any
is taken.

What closed the supervisor half needed a different shape, because of a fact about
the transport rather than about the design: **a gRPC query carries no signer, so
the chain cannot gate a read by role.** Ordinary read access on this deployment
is controlled at the proxy, and a role that pretended to control it would be
worse than an empty one. What the chain can do is decide who is *entitled* to
read the one body of data it does not serve to whoever asks — the encrypted
payload of a payment — and publish that set authoritatively from the grant
registry. A holder of `ROLE_SUPERVISOR` covering a country is entitled to be a
viewing-key recipient for every payload whose declared settlement jurisdiction is
that country, which is the same declaration that decides who may act on the
payment, and `Query/PayloadReaders` returns the whole set with each holder's
current key so a sender can resolve it in one call at the moment it seals.

Two things follow, and the second is a limit worth stating rather than glossing.
A country's appointed regulator must now hold the role covering that country —
`MsgAppointRegulator` refuses otherwise — so "who is watching this country" stops
having two answers in two places, which is what the role was always documented as
providing. And the chain **cannot force a sender to seal to an entitled reader**:
the envelope is built off-chain and the chain holds only a hash of the plaintext,
so a sender that ignores the published set produces a payload the supervisor can
never open and nothing on chain detects it. That is a limit of where the
ciphertext lives, not a gap in the registry.

A supervisor still cannot open a case, freeze, seize, touch land, admit an issuer
or a participant, grant a role, correct a jurisdiction, or appoint a regulator.
Oversight without the power to act, which is what the name says.

Grants naming a **country** are created and removed by governance **or by the
foundation** — meaning the one account `x/constitution` pins as
`enforcement_recovery_destination`, which is the 3-of-5 from the key ceremony.
Grants naming the **chain-wide scope** are governance and nobody else.

That split replaced an earlier rule of governance-only, and the earlier reasoning
still holds for the half that did not change. There is a difference between using
a power and deciding who holds one: an account that could grant itself the
chain-wide scope could then grant it to anybody, at which point the perimeter is
whatever that account says it is. So `*` stays where it was, and the refusal
happens before the constitution is even read — a store failure resolving the
foundation must not be the thing that decides whether the widest scope was allowed.

What changed the country half was [enrolling one](../guides/country-enrolment.md).
A country is not one grant: it is an M-of-N group per office, two to five grants
across those offices, and a jurisdiction record per office, in an order that
matters. Under governance-only that was five or six proposals for one decision,
each able to pass, fail or time out separately — so the friction was not being
paid once per widening of authority but several times over for a decision a room
had already taken, and the predictable outcome of that is a bundle proposal nobody
reads.

**What was given up is publicity and delay**: the validator set no longer has a
veto over who administers a perimeter, and an appointment no longer sits in public
for a voting period first. What replaces them is three of five custodians from
five organisations, attributed on chain in `granted_by` at a recorded height.

The residual risk is stated rather than closed: the foundation can grant one
office the same role in every country, one grant at a time, and reach the same
place a chain-wide grant would by a longer road. It is bounded by the assigned
country list, and it is enumerable through `role-holders` per country and
`chain-wide-grants` for the exceptions.

Revocation carries the same signers as granting, deliberately rather than by
omission. Keeping it governance-only would put the slow path on the wrong action:
the reason to revoke in a hurry is that an office's keys are compromised, and a
rule under which the foundation can appoint a national enforcement authority in
one vote and needs a governance cycle to remove one makes the emergency the
expensive case.

Note that "the foundation" here is **not** a foundation administrator, and the
difference is the point. That role is granted by `MsgGrantRole`, so reading the
foundation out of it would make "who may appoint a country's authorities" a set
that `MsgGrantRole` itself could append to; an invariant cannot be changed
without a constitutional amendment. The circularity is the same one that ruled
out a parameter list, which is where the administrators used to live: a list a
single ordinary proposal could edit.

**The administrators are a role now, and collapsing the two mechanisms was the
point.** `alias.params.foundation_administrators` is retired and its field number
reserved; the exemption is `ROLE_FOUNDATION_ADMINISTRATOR`, granted chain-wide
and refused at a country scope — which keeps the appointment exactly as
governance-only as the parameter was, since a chain-wide grant is governance's
alone. Enrolling a country used to need both mechanisms at once, because
recording an office's own jurisdiction goes through the administrator (no
participant onboarded a group policy) while granting its roles goes through the
registry; they were unrelated lists that happened to share a name, and holding
one without the other produced a proposal that passed and an office that could
not work. It is one registry now, and an enrolment checks one thing.

Three properties the parameter never had come with the move, and each of them was
a real defect: an administrator must be an `x/group` account rather than a bare
key; the address is decoded, where `Params.Validate()` accepted any non-empty
string so a typo passed a vote and granted the exemption to nobody; and a
proposal about something else cannot silently drop the administrators already
appointed, which `MsgUpdateParams` could do every time, because it replaces the
whole parameter object and a shorter list is a valid list.

The cap of eight survives, now counted over the chain-wide grants of the role in
both `GrantRole` and genesis validation, and for the reason it always had: this
is the single exception to "every account has a jurisdiction", and widening it
must be a visible act.

Roles are held by `x/group` accounts, checked at grant time rather than trusted,
so an authority action is M-of-N rather than one official — which is already how
`x/land` treats registry offices, and the same reasoning applies everywhere else.

**And the M-of-N is recorded on the grant, not just asserted once.** "Is the
holder a group policy" was the whole of the check for a while, and it was the
right question asked once and never asked again. Two things followed:

- A one-of-one group *is* a group policy, so it answered yes. Multisig was
  guaranteed in form and not in substance.
- An office is its own admin — necessarily, because that is what makes its
  membership changeable by its own members and by nobody else — so it can vote to
  change its own threshold at any time after the grant. A country could hold a
  proper ceremony, stand up a three-of-five enforcement authority, and that office
  could later become a one-of-one and go on freezing accounts.

A grant therefore carries an optional `required_shape`, a minimum number of
signatures and a minimum number of members, and both perimeter functions resolve
the holder's group policy on every call and refuse when the office has fallen
below it. Both numbers are floors: an office may grow, and may tighten, and may
not shrink — the same rule `x/constitution` applies to the foundation's own group,
for the same reason. Three-of-five becoming three-of-four moves sixty per cent to
seventy-five and walks towards unanimity.

Three properties worth stating because each was a decision:

- **"Signatures" means people, not the threshold number.** `x/group` counts
  weight, so a threshold of three over members weighing 3, 1, 1, 1 and 1 is a
  one-of-five. The check sorts the weights descending and counts how few can reach
  the threshold, which for equal weights is exactly the threshold.
- **Absent means no requirement**, and absence is a nil message rather than a zero
  integer, so the two can never be confused. Grants made before the field existed
  are unchanged in effect, which is honest rather than ideal: their holders can
  still shrink to a single key until each grant is re-made.
- **The requirement is decided before the ceremony**, in the enrolment config, and
  the ceremony refuses to assemble an office whose signed group file does not meet
  it. A requirement captured from whatever group turned up would ratify a
  one-of-one as readily as a three-of-five, which is not a requirement.

It is a state check on the action rather than a gate on the transaction that
changes the group, and that is not a stylistic choice. `x/constitution` guards the
foundation's shape with an ante decorator because there is one foundation and its
shape is in the constitution; an office's shape is on its grant, there are as many
as there are countries times offices, and an ante gate would not see an office
acting through an interchain account or `x/authz` at all.

### 3. One check, called everywhere — built

A single keeper function — `AssertScope(ctx, actor, role, target)` — resolves the
target's jurisdiction and refuses when the actor's grant does not cover it. Every
authority action routes through it: registering, validating and freezing in
`x/land`; freezing and opening cases in `x/enforcement`; issuer approval in
`x/stablecoin`; participant approval in `x/paymsg`.

One function, because a perimeter enforced in eleven places is a perimeter with
eleven ways to be wrong.

There is a second entry point, `AssertScopeIn(ctx, actor, role, jurisdiction)`,
for the case where the jurisdiction is **named in the record** rather than looked
up from an account. `x/land` uses it today: a registry office administers a
country, and that country is a field on the office's own admission record, so
there is no account to resolve. It is also the shape a payment's declared
settlement jurisdiction will take when a message exists by which an authority
*acts* on a payment — today the declaration decides who may read the payload and
nothing on the chain acts on a settled payment, so there is nothing yet to gate.

The two are not interchangeable and neither is folded into the other: collapsing
the second into the first would mean inventing an account to stand for a country,
and collapsing the first into the second would let an actor tell the check which
perimeter its target is in, which is exactly the claim an actor must not be able
to make. `AssertScopeIn` therefore refuses both the chain-wide marker and the
foundation's reserved code as inputs — a named jurisdiction is an assigned
country or it is nothing.

Three things it refuses, in this order:

1. an unset or unknown role;
2. a target whose jurisdiction the chain does not know — before any grant is
   consulted, so not even the chain-wide scope reaches an account nobody has
   placed;
3. no grant covering that country, which includes and is mostly the actor holding
   no grants at all.

The consumers reach it through a narrow local interface with the one or two
methods they call, never through the keeper. `x/paymsg` is the exception in *how*
it receives that interface rather than in what it does with it: `x/alias` consults
`x/paymsg` to find out who onboarded an account, so an edge back the other way
would be a dependency cycle. It is handed the perimeter after construction, and
the check refuses until it is — so a wiring mistake removes a national authority's
ability to admit anybody rather than letting everybody in.

The two governance-gated admissions — issuer approval and participant approval —
accept **either** governance or the relevant national authority. That is a
widening of who may act and the perimeter is what makes it safe: Nigeria's central
bank can admit an issuer recorded in Nigeria and cannot touch one recorded in
Senegal. Governance is accepted without a grant because it is the body that makes
the grants; requiring it to hold one would be circular.

## Two rules, decided

**Every account has a jurisdiction. There is exactly one exception.** *(Built.)*
Registration refuses an account with no country recorded — not a permissive
default, a refusal. The only accounts without one are the **foundation
administrators**, the highest authority on the rail, who hold
`ROLE_FOUNDATION_ADMINISTRATOR` at the chain-wide `*` scope. There is therefore
no ambiguous unclaimed state for anyone to reason about or exploit, and no
migration limbo in which the perimeter is advisory.

The exemption is read as a fact about placement rather than as an authority, and
the distinction matters at exactly one point. `CountryOf` asks only whether the
grant exists; it does not hold the office to the M-of-N its grant may record.
An office that lost a member would otherwise stop having a country at all — its
own identifier unissuable, every authority action against it refused with "no
recorded jurisdiction" — which is failing closed in the direction that removes
an account from every perimeter rather than the one that refuses it a power.
Acting is a different question, asked elsewhere, and there the shape is checked
like everywhere else.

The identifiers issued before this existed are prefixless, and there was no
source of truth to give them a country from — the registry is what the change
introduces, so any prefix written for them would have been invented. They are
therefore **tombstoned** by the module's v1-to-v2 migration: they stop resolving
at the upgrade height, and the account registers again once its participant has
recorded where it is. Tombstoned rather than deleted, because somebody has an
old handle written down and "this was given up" is a truthful answer to them
where "never existed" is not.

**A cross-border deal names its settlement jurisdiction, and that authority
enforces.** A payment from Nigeria to Ghana touches two perimeters, so both
endpoint authorities may *see* it. Only the authority of the **declared
settlement jurisdiction** may *act* on it. One declaration, no contest over who
has standing, and the record shows which authority acted.

That same declaration does a second job: it names the regulator who holds the
third viewing key over the payment's encrypted payload. See
[confidentiality.md](confidentiality.md). One field, two consistent consequences
— which is the sign that it is the right field.

## Two registries hold "which country", and they are reconciled rather than merged

`x/validatorgov` collects a validator's jurisdiction in its application, with its
legal entity and beneficial owner, and validates it against the same assigned
country list this module owns. It does not write it into `Jurisdictions`, and that
is deliberate.

The two facts have different provenance and it is a real difference:

- a validator **declaring** Senegal is a signed claim by its own operator;
- Senegal's participant **recording** Senegal is a finding by whoever did the KYC.

Merging either direction destroys something. Overwriting the declaration with the
record throws away the signature that makes a false declaration an offence, which
is most of what the declaration is for — it is collected alongside a beneficial
owner precisely so that somebody can be held to it. Treating the declaration as a
record makes this registry a place where accounts place themselves, which is the
single thing the whole design refuses.

So they stay separate and the disagreement is made **visible** rather than
resolved: `x/validatorgov`'s `JurisdictionReconciliation` query lists every
approved validator with its declared country, its recorded country if it has one,
who recorded it, and a state — agree, disagree, or unrecorded — plus counts, so
that "three disagree" is a fact somebody sees without scanning. Unrecorded is the
ordinary state for a validator nobody has placed, and it is an answer rather than a
fault: until somebody records where a validator's operator account is, no
authority's perimeter contains it, which is exactly what the two rules above say.

Placing one is not a special case. A validator's operator account is placed the
way any account is — by the participant that onboarded it, or by the foundation
when it banks nowhere.

## The connection to concentration caps

This is the same structural point as the caps in §4 of the revised scope, in a
different costume. A jurisdiction stamped once at account creation and never
re-examined is an *event*; a perimeter is a *state*. The check must run at the
moment of every action, against current state, or it protects only the moment it
ran.
