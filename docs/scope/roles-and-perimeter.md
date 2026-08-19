# Roles, stakeholders and jurisdictional perimeter

It answers one requirement from the revised scope: multiple stakeholders hold
roles that are bounded by country, so a national authority can act on its own
jurisdiction's accounts and records and on nobody else's.

**Status.** Piece 1 below — the jurisdiction on the account, the country prefix
in the identifier, and both of the decided rules — is built, in `x/alias`.
Pieces 2 and 3 — role grants and the single `AssertScope` every authority action
routes through — are still a design. The registry the roles will be scoped
against exists and is queryable, which is the part that had to come first: a
grant naming a country is meaningless until the chain knows which country an
account is in.

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

### 2. Roles scoped to a jurisdiction — not yet built

A grant is a triple — **who, what role, where**:

```
(yml1lands…, REGISTRY_AUTHORITY,   GH)
(yml1cbn…,   MONETARY_AUTHORITY,   NG)
(yml1audit…, SUPERVISOR,           *)
```

`*` means chain-wide and should be rare, granted visibly, and listed on the
governance console precisely because it is the exception.

Roles are held by `x/group` accounts wherever a decision matters, so an authority
action is M-of-N rather than one official — which is already how `x/land` treats
registry offices, and the same reasoning applies everywhere else.

### 3. One check, called everywhere — not yet built

A single keeper function — `AssertScope(ctx, actor, role, target)` — resolves the
target's jurisdiction and refuses when the actor's grant does not cover it. Every
authority action routes through it: registering, validating and freezing in
`x/land`; freezing and opening cases in `x/enforcement`; issuer approval in
`x/stablecoin`; participant approval in `x/paymsg`.

One function, because a perimeter enforced in eleven places is a perimeter with
eleven ways to be wrong.

## Two rules, decided

**Every account has a jurisdiction. There is exactly one exception.** *(Built.)*
Registration refuses an account with no country recorded — not a permissive
default, a refusal. The only accounts without one are the **foundation
administrators**, the highest authority on the rail, who hold the chain-wide `*`
scope. There is therefore no ambiguous unclaimed state for anyone to reason about
or exploit, and no migration limbo in which the perimeter is advisory.

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

## The connection to concentration caps

This is the same structural point as the caps in §4 of the revised scope, in a
different costume. A jurisdiction stamped once at account creation and never
re-examined is an *event*; a perimeter is a *state*. The check must run at the
moment of every action, against current state, or it protects only the moment it
ran.
