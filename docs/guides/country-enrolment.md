# Enrolling a country

How a country's authorities come to exist on a chain that is already running:
their keys made in their own offices, their groups created by transaction, and
their powers granted by the foundation in one decision three custodians sign.

**You need:** the country's offices named and staffed, two to five people per
office who can each open a link on their own device, three of the foundation's
five custodians reachable, and somebody with a funded key who can broadcast.

**You will end with:** one `x/group` account per office, a role grant per office
scoped to that country and to nothing else, a jurisdiction record for each
office's own account, the first institutions placed so they can be admitted, and
a signed record naming every address and who granted what.

**The steps, in the order they happen:**

| | Command | Who signs |
| --- | --- | --- |
| 1 | `ceremony host` — once per office | each super user, in their own browser |
| 2 | `ceremony country init` | nobody; it reads and refuses |
| 3 | `ceremony country groups` | any funded key |
| 4 | `ceremony country confirm` | nobody; it reads the chain back |
| 5 | `ceremony country grants` | the foundation, M-of-N |
| 6 | `ceremony country verify` | nobody; it reads the chain back |
| 7 | `ceremony country seed` | the foundation, M-of-N |
| 8 | `ceremony country admit` | the country's payments office, M-of-N |
| 9 | `ceremony country record` | everybody, on paper |

`ceremony country validator` slots in wherever a validator needs placing. Each
step refuses until the previous one's evidence is in the dossier.

---

## Why this is a separate ceremony

[The key ceremony](key-ceremony.md) exists because one account mattered more than
any other and had been created by a line of shell. This one exists for the
opposite reason: because there are going to be a great many of these, in a hurry,
and the thing that goes wrong at volume is not carelessness about a single key.
It is a procedure with a step in it that looks optional.

The two share everything that touches key material. An office's super users
generate their keys in their own browsers, from the same `ceremony host`, over the
same code, with the same possession signatures, the same group assembly and the
same eighty-bit fingerprint read aloud before anybody signs anything. If you have
run the hosted foundation ceremony you have already run three quarters of this
one.

What differs is the far end, and it differs completely. The foundation's ceremony
produces a **genesis fragment**, because the foundation has to exist at height
zero — `x/constitution` refuses to start a chain whose recovery destination is
unset. A country's ceremony produces **transactions and proposals**, because the
chain is up, blocks are being made, and money is already moving.

That one difference is where all the danger in this document lives.

## The one thing that will bite you

An `x/group` policy address is derived from the **group policy sequence number
and nothing else**. Not the members. Not the threshold. Not the admin. Not the
chain id. `ceremony address --seq N` prints it offline, and that is a real
property the launch runbook depends on.

On a live chain it is a trap.

Suppose this tool predicted an office's address, and a grant were composed naming
it. Whoever created a group policy first owns that address. The grant would still
be a real grant — `PAYMENTS_AUTHORITY` over a whole country, or
`ENFORCEMENT_AUTHORITY`, signed by the foundation, every signature valid, sitting
in the registry. And nothing downstream would notice, because every later check
reads the same predicted address and agrees with it.

So the enrolment is **two phases**, and the boundary between them is not
negotiable:

1. **Create the group.** `ceremony country groups` writes the transactions. It
   writes no address and composes no grant.
2. **Read the address back.** `ceremony country confirm` takes the chain's own
   answers and checks that the policy at that address really is this office — the
   same members, the same threshold, administering itself.
3. **Then compose the grant.** `ceremony country grants` refuses outright for any
   office whose address has not been read back. There is no flag that relaxes it
   and no `--seq` to fall back on.

If you remember one thing from this document, remember that an office's address
comes from the chain and is checked against its membership. Everything else here
is bookkeeping by comparison.

## Who admits a country, and what that cost

The **foundation** does — the 3-of-5 from the key ceremony, which is to say the
account `x/constitution` pins as `enforcement_recovery_destination`.

This used to be governance and nobody else, and the argument for that was good:
there is a difference between using a power and deciding who holds one, and every
widening of who may act ought to cost a public vote. What broke it was arithmetic.
Enrolling one country is a group per office, two to five grants across those
offices, and a jurisdiction record per office — a sequence that has to land in
order. Under governance-only that was five or six separate proposals, each able to
pass, fail or time out on its own, for one decision a room had already taken. The
predictable result of that is a bundle proposal nobody reads.

**What was given up is publicity and delay.** The validator set no longer has a
veto over who administers a perimeter, and the appointment no longer sits in
public for a voting period before it takes effect. What replaces those is
narrower: three of five custodians from five organisations, on chain, with the
grant attributed in `granted_by` and the height in `granted_at_height`.

**Chain-wide is still governance and nobody else.** The foundation admitting a
country and the foundation manufacturing authority over every country are
different acts, and only the first was decided. `*` is refused from the foundation
by the chain, before the constitution is even consulted, and refused again by this
tool.

The residual risk, stated plainly because it is not closed: the foundation can
grant one office the same role in every country, one grant at a time, and arrive
where a chain-wide grant would by a longer road. It is bounded by the assigned
country list rather than prevented, and it is enumerable —
`blockchaind query alias role-holders <CC>` for each country,
`blockchaind query alias chain-wide-grants` for the exceptions.

## Before the day

### The offices

Decide them first, on paper, and decide the roles with them. The
[roles and perimeter design](../scope/roles-and-perimeter.md) has the five:
registry authority, monetary authority, payments authority, enforcement authority
and supervisor. A payments country needs **payments authority and enforcement
authority** at a minimum, and the tool refuses without both unless you write down
why.

Why those two. Without a payments authority nothing in the country can be admitted
to the rail, so no account in the country can be onboarded or placed, and the
country is enrolled and inert. Without an enforcement authority money moves in a
perimeter and no office can stop it. If the country genuinely is registry-only or
supervision-only, say so in the config:

```json
"waivers": [{"rule": "payments-minimum", "reason": "land registry pilot; no payments licence yet"}]
```

The rule still refuses. What gets past it is a sentence that ends up on the signed
record, which is the same arrangement as `preflight`'s
`--network-acknowledged`.

### One person, one office

An office of two-of-three sharing a member with the office that supervises it is
not the separation the roles describe. The tool refuses a super user who appears
in two of a country's offices, because once both groups exist the arrangement is
invisible: each policy looks correct on its own.

### The threshold, and the minimum it may never fall below

Per office, and the same rules as the foundation's: never one (that is a key with
extra steps), never unanimity (one person unreachable freezes the office
permanently). Two-of-three and three-of-five are the shapes that work.

Decide **two** numbers per office and write both down, because they are different
claims:

| | What it is | Where it comes from |
| --- | --- | --- |
| The threshold | what the office **is** | the group file its super users signed |
| The minimum | what the office may **never fall below** | `minimum` in the config, decided before the day |

The minimum is the one that is new and the one that does the work. It goes onto
every grant as `required_shape`, and from then on the chain resolves the office's
group policy on **every** authority action and refuses when the office has fallen
below it. That is the difference between multisig as a fact checked once and
multisig as a state:

- An office is its own admin — that is what makes its membership changeable by its
  own members and by nobody else. So a properly-ceremonied three-of-five can vote
  itself to a one-of-one the next morning, and before this it went on freezing
  accounts with nothing notified and nothing refused. `x/constitution`'s ante gate
  pins the **foundation's** shape and has never had anything to say about an
  office's.
- Both numbers are floors, and only floors. Three-of-six is fine — a member
  joined. Four-of-six is fine — the office decided to be more careful.
  Three-of-four is refused: a member left and was not replaced, which moves sixty
  per cent to seventy-five and walks towards unanimity. One-of-five is refused for
  the obvious reason.
- "How many signatures" means **how many people**, not the threshold number.
  `x/group` counts weight, so a group with a threshold of three whose members
  weigh 3, 1, 1, 1 and 1 is a one-of-five: the first member reaches the threshold
  alone. The chain takes the members in descending weight and counts how few of
  them can reach the threshold, which for the equal weights every ceremony
  produces is exactly the threshold.

`ceremony country init` refuses an office whose signed group file does not reach
its minimum, before any group exists on the chain. It also refuses a minimum of
one signature, and a unanimous minimum — the chain holds an office to its
minimum, so a floor of three-of-three is a promise that the office may never
survive a lost key.

**Choose the minimum at or below the shape you are actually standing up.** A
country agreeing three-of-five and generating three-of-five gets a grant it
satisfies exactly, and one lost key takes the office out of service until it is
replaced — which is the intended behaviour and is why departures and replacements
are one decision. A country wanting slack can agree three-of-five and stand up
four-of-seven; the chain holds it to the three-of-five.

### What the foundation needs to be able to do

Two separate things, and they are granted by two separate mechanisms with two
different costs. Check both before the day, because discovering the second one
late means a governance cycle in the middle of a ceremony.

| The foundation must | Mechanism | Cost to change |
| --- | --- | --- |
| grant a role in a country | it is the address `x/constitution` pins as `enforcement_recovery_destination` | a constitutional amendment |
| record a jurisdiction | it is in `alias.params.foundation_administrators` | one governance proposal |

The second is the one people miss. An office's group account was onboarded by no
participant, so nobody but a foundation administrator or governance may record
where it is — and that record is part of what makes the office real. So:

```bash
blockchaind query alias params
```

If the foundation's policy address is not in `foundation_administrators`, put it
there with a governance proposal, **once, before enrolling any country**. It is
not part of an enrolment and it should not be. `ceremony country grants` checks it
and refuses rather than composing a proposal that would be voted through and then
fail.

---

## The sequence

### 1. Generate each office's keys

One `ceremony host` per office, with the country and the roles set:

```bash
go build -o ceremony ./tools/ceremony
./ceremony host --public-url https://pay.yamalelegal.com/ceremony/ --out ./senegal-payments
```

The coordinator's setup screen takes the office name, the roster, the threshold,
the chain id — and now a **country** and a list of **roles**. Fill them in. They
are not decoration:

**The country and the roles are inside the parameters fingerprint every super user
reads before generating a key.** That is the point. Without them the sequence is:
three people are told they are generating keys for Senegal's payments authority,
they compare a fingerprint, they sign — and then a config pairs their group with a
grant of enforcement authority over Nigeria, with every signature still verifying.
With them in the fingerprint, a config that disagrees with what they signed is a
refusal, and `ceremony country init` is where it refuses.

Leave the country blank and it is a foundation ceremony. `ceremony country` refuses
a foundation ceremony used as an office, and says why: those custodians attested to
holding the chain's recovery destination, and nothing they signed mentions
administering a country.

Everything else is the hosted ceremony as documented: the words appear once and
are never transmitted, a link speaks for one person, the page will not let anybody
attest to a group their own key is not in. Read
[the hosted ceremony](key-ceremony.md#the-hosted-ceremony) before running one, and
in particular read [which of the two paths is stronger](key-ceremony.md#two-ways-to-run-it-and-which-is-stronger)
— the same trade applies here, and for a country whose enforcement authority will
be able to freeze accounts, the air-gapped path is still the stronger one.

What each office ends with is `group.json` in its own directory, and a record its
super users sign. **Keep the group fingerprint.** It is written into the enrolment
record later and it is what ties an on-chain office back to the ceremony that made
its keys.

### 2. Write the enrolment config

```json
{
  "ceremony": "Senegal enrolment",
  "chain_id": "yamale-testnet-1",
  "country": "SN",
  "foundation": "yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj",
  "offices": [
    {
      "name": "Senegal payments authority",
      "roles": ["ROLE_PAYMENTS_AUTHORITY", "ROLE_ENFORCEMENT_AUTHORITY"],
      "group": "./senegal-payments/group.json",
      "minimum": {"signatures": 3, "members": 5}
    },
    {
      "name": "Senegal lands commission",
      "roles": ["ROLE_REGISTRY_AUTHORITY"],
      "group": "./senegal-lands/group.json",
      "minimum": {"signatures": 2, "members": 3}
    }
  ]
}
```

Note what is **not** in it: no thresholds, no member addresses, no policy
addresses. The first two come from the group files, which the offices' super users
signed for; the third comes from the chain. A config field for any of them would
be a field somebody could fill in wrongly, and two of the three decide who ends up
holding a country's authority.

`minimum` is the exception, and it is not the same kind of thing. An actual
threshold in a config is an **observation**, and one that could contradict the
keys — a config claiming three-of-five over a group file that says one-of-one
would be a config that lies about who holds an authority. A minimum is a
**constraint**: the decision the country took before anybody generated a key, and
what the ceremony holds the signed group file against. It is required per office,
with no default, because a default of "no minimum" would silently produce exactly
the state it exists to prevent.

```bash
./ceremony country init --config senegal.json
```

This prints every office, its threshold, the minimum it may never fall below, its
members with their fingerprints, its roles — and **no address for anything**,
which is not an omission. It writes `country-SN.json`, the dossier, which every
later step reads and rewrites.

Read the printed fingerprints against the records the offices signed. This is the
last moment at which a wrong group file costs nothing.

It is also where a shape that does not match the decision is refused:

```
Senegal payments authority must be at least 3-of-5 and ./senegal-payments/group.json
was generated as 2-of-3: its threshold of 2 is below the 3 signatures this enrolment
requires.
```

That is the tool doing its job, and the two ways out are both deliberate: run the
office's ceremony again with the agreed threshold, or change the minimum in the
config and have the change agreed. Not silently — the chain will hold the office to
whichever number ends up on the grant, for as long as the grant exists.

### 3. Create the groups

```bash
./ceremony country groups --dossier country-SN.json --from operator
```

Two files per office and the exact command line for each. Broadcast them.

**Any funded key can broadcast these**, and that is safe rather than sloppy.
`--group-policy-as-admin` makes the policy its own admin, so the account that
signs keeps nothing: it cannot change the membership, the threshold, or who
administers the office afterwards. Step 4 checks that it came out that way rather
than assuming it.

### 4. Read the addresses back

For each office:

```bash
blockchaind query tx <hash> -o json > tx.json
# the policy address is in the EventCreateGroupPolicy in that file
blockchaind query group group-policy-info <address> -o json > policy.json
blockchaind query group group-members <group id> -o json > members.json

./ceremony country confirm --dossier country-SN.json \
  --office "Senegal payments authority" \
  --tx tx.json --policy policy.json --members members.json
```

Three files, and yes, that is more copying than fetching would be. `tools/ceremony`
makes no outbound network connections, and the key ceremony guide makes that claim
to people who are about to type a seed phrase in front of it. One `net/http` call
here would cost the claim to save a paste.

What it buys is that the address is **verified rather than trusted**. A fetched
address would be believed because it came over a socket. This one is checked
against what the office is:

| Refusal | Why it matters |
| --- | --- |
| the transaction created no group policy | you queried the wrong transaction |
| the policy is for a different address than the transaction created | the grant would land on whichever one the tool believed |
| the group id disagrees between the three documents | one of them is about something else |
| the policy is not its own admin | somebody outside can rewrite the membership, so the threshold is advisory |
| the threshold is not what the super users attested to | the office on the chain is not the office they agreed to |
| the members are not exactly the office's roster | **this is the whole check** — see below |
| the address is the foundation's own | the foundation would be appointing itself |

The member check is set equality both ways. Not "the office's members are among
the group's" — a group with a fourth member is a two-of-four. Not "the group's are
among the office's" — a group missing one is a two-of-two, which concentrates more
authority in whoever remains than the ceremony gave them. And a member with weight
2 is refused, because equal weight is what makes "two of three" mean what it says.

**Note the trap in the transaction result.** A broadcast reporting `code: 0` has
been **accepted into a mempool**. It has not executed. The tool refuses a document
with `height: 0` for exactly this reason and tells you to query it instead. This
is not hypothetical — see the next section, where the same trap has a much worse
version.

### 5. The enrolment proposal

```bash
blockchaind query constitution invariants -o json > invariants.json
blockchaind query alias params -o json > alias-params.json

./ceremony country grants --dossier country-SN.json \
  --proposer <a foundation custodian> \
  --invariants invariants.json --alias-params alias-params.json
```

One proposal for the whole country, carrying, in this order:

- `MsgSetJurisdiction` per office, placing its own group account in the country,
  signed by the foundation as a foundation administrator;
- `MsgGrantRole` per office per role, scoped to the country, never to `*`.

**One proposal and not one per grant**, deliberately. `x/group` executes a
proposal's messages together or not at all, so this is the only shape in which
"the country is enrolled" is a thing that either happened or did not. Split up,
the state in between is a country with a payments authority and no enforcement
authority — money moving in a perimeter nobody can stop. The cost is a longer
document for three custodians to read, and the summary states every grant in words
so that a custodian who reads only the summary has read the proposal.

Submit it, and three custodians vote. Use the
[foundation console](../../clients/foundation) — it decodes a proposal into words,
counts the votes, and composes the commands to run where the keys are.

**Read the proposal on your own node before voting.** Not this tool's summary of
it, and not the console's:

```bash
blockchaind query group proposals-by-group-policy <foundation> -o json
```

### 6. Verify that it actually happened

This step is not optional and here is why.

`blockchaind tx group exec` on a proposal that has **not** reached its threshold
returns **`code: 0`**. Not an authorization error. Not a failed transaction. A
clean, successful transaction that does nothing at all. The only signal anywhere
is an event attribute:

```
EventExec.result = PROPOSAL_EXECUTOR_RESULT_NOT_RUN     # two of five
EventExec.result = PROPOSAL_EXECUTOR_RESULT_SUCCESS     # three of five
```

So a script that checked the exit status, or even `query tx | .code`, would
conclude the foundation had acted on two signatures. And an executed proposal is
**pruned**, so `query group proposal <id>` afterwards returning "not found" is
success rather than a fault.

Do not infer. Read the registry:

```bash
blockchaind query alias role-grants <office policy address> -o json > grants.json
blockchaind query alias jurisdiction <office policy address> -o json > placed.json

./ceremony country verify --dossier country-SN.json \
  --office "Senegal payments authority" \
  --grants grants.json --jurisdiction placed.json
```

It checks that every role the dossier describes is present, in this country, and
**granted by the foundation** — that last part as much as the first, because a
grant of the right role in the right country made by some other authority is
somebody else's act, and recording it on this enrolment's record would put your
signature on it.

Grants the office holds that this enrolment did not make are **reported, not
refused**. An office may legitimately have been granted something by governance.
An office holding a chain-wide grant is the single most important thing a reader of
the record could want to know, so it appears on the screen rather than being
filtered out.

### 7. Place the first institutions

This is the step nothing announces, and skipping it produces a chain that looks
broken.

The country's payments authority now holds a perfectly good grant. It still cannot
admit the first bank in its own country. `x/paymsg`'s delegated approval path calls
the perimeter check on the **applicant**, and the perimeter check refuses a target
the chain cannot place *before* it consults any grant. The applicant has no
jurisdiction record. It may not declare its own. And there is no approved
participant in the country yet to record one, because that is what is being
attempted.

So the foundation records it, once, for the seed:

```bash
./ceremony country seed --dossier country-SN.json \
  --proposer <a foundation custodian> --account <applicant>
```

Another foundation proposal, three votes, and then:

```bash
blockchaind query alias jurisdiction <applicant> -o json > placed.json
./ceremony country seed --dossier country-SN.json --account <applicant> --verified placed.json
```

**Every account after these is placed by the participant that onboarded it**,
which is where the record belongs: the participant did the KYC and is the only
party that knows the answer. The seed is a bootstrap and not a pattern. A
deployment that kept using it would have the foundation asserting the country of
accounts it has never met, which is exactly the arrangement
[the perimeter design](../scope/roles-and-perimeter.md) rejects.

### 8. The payments authority admits them

```bash
./ceremony country admit --dossier country-SN.json \
  --proposer <an office super user> --applicant <bank>
```

A proposal to **the office's** group, not the foundation's. This is what enrolling
the country bought: licensing a payment service provider in Senegal is decided by
Senegal's payments authority, M-of-N, and not by the foundation and not by a
chain-wide governance vote.

The tool refuses to compose it until two things have been read back off the chain
— the office's grant, and the applicant's placement — because both failures look
identical from the outside. A payments authority whose grant never executed refuses
exactly as one that was never appointed does.

Then verify, as always:

```bash
blockchaind query paymsg get-approved-participant <bank> -o json > admitted.json
./ceremony country admit --dossier country-SN.json --applicant <bank> --verified admitted.json
```

### 9. The record

```bash
./ceremony country record --dossier country-SN.json --config record.json
```

```json
{
  "location": "Dakar",
  "started_at": "2026-09-01T09:00:00Z",
  "completed_at": "2026-09-01T15:30:00Z",
  "proposal_id": "4",
  "custodians": ["Amara Okafor", "Chipo Mwale", "Eshe Njoroge"],
  "participants": [
    {"name": "R. Lead", "role": "enrolment lead", "organisation": "Yamale Foundation"},
    {"name": "S. Scribe", "role": "scribe", "organisation": "Yamale Foundation"},
    {"name": "O. Observer", "role": "independent observer", "organisation": "External Auditors LLP"}
  ],
  "notes": []
}
```

Every address, threshold, grant and fingerprint in the rendered record is read out
of the dossier — which is to say out of the chain's own answers, verified. Nothing
is retyped. An enrolment record carrying a mistyped office address would be a
document whose only purpose is to detect mistyped office addresses.

**It refuses to render** if any office is unconfirmed or any grant unverified. A
record signed at that point would state that a country's authorities hold powers
nobody has checked they hold, and an accepted-but-unexecuted proposal looks exactly
like that.

One claim in it is typed rather than read, and the record says so: **which three
custodians voted**. That is a fact on the chain, in the proposal's votes, and this
tool cannot read it. The record names the proposal so a reader can check.

**Print it. Everybody signs the paper copy before leaving** — every office's super
users, the lead, the scribe and the observer.

---

## Checking somebody else's enrolment

Every claim in the record is a query. This is the whole point of putting the
addresses in it.

```bash
# the foundation is who the record says it is, and cannot be changed by a vote
blockchaind query constitution invariants

# each office is the group the record describes
blockchaind query group group-members <group id>
blockchaind query group group-policies-by-group <group id>

# and holds exactly the roles the record lists, granted by the foundation
blockchaind query alias role-grants <office policy address>

# everybody who may act in the country, from the country's end
blockchaind query alias role-holders SN

# and everybody no border bounds, which should be a very short list
blockchaind query alias chain-wide-grants
```

The last two are the supervisory pair. `role-holders SN` deliberately does **not**
fold in the chain-wide grants: a country's own list should show what that country
granted, and mixing the exceptions in would hide them among the ordinary entries
of every country at once.

## What an authority can and cannot do once enrolled

The perimeter is not advisory and it is not checked once at appointment. It is
`AssertScope`, in every message server that acts on an account, against current
state, every time.

So Senegal's enforcement authority can open a case against an account recorded in
`SN` and is refused — `ErrOutOfScope`, naming the role and the country — against
an account recorded anywhere else. An account the chain cannot place at all is
refused to everybody, including the holder of a chain-wide grant, and that refusal
comes *before* any grant is consulted.

That last one is worth internalising, because it is the shape of most surprises
here: a great many failures that look like "my authority does not work" are
actually "the account you named has no jurisdiction record".

### Two of the five roles have nothing to do yet

This is a gap in the chain rather than in the enrolment, and it is stated here
because an operator will otherwise grant a role and then spend an afternoon
finding out why nobody can use it.

| Role | What consults it today |
| --- | --- |
| `ROLE_PAYMENTS_AUTHORITY` | `x/paymsg` — admitting a participant. Works. |
| `ROLE_MONETARY_AUTHORITY` | `x/stablecoin` — approving an issuer. Works. |
| `ROLE_REGISTRY_AUTHORITY` | `x/land` — parcels, transfers, freezes. Works. |
| `ROLE_ENFORCEMENT_AUTHORITY` | `x/enforcement` — but see below. |
| `ROLE_SUPERVISOR` | **nothing at all.** |

`ROLE_ENFORCEMENT_AUTHORITY` is consulted in exactly two places and both have a
second gate an office cannot pass. `MsgOpenCase` requires the opener to be a
**bonded validator**, and a group policy account cannot be one.
`MsgEmergencyFreeze` requires the signer to be `x/enforcement`'s
`emergency_authority` parameter. So a country's enforcement office holds a grant
that no message currently lets it use. Grant it anyway — the enrolment is the
right moment, and appointing the office later is much harder than appointing it
now — but do not expect it to be able to freeze anything yet.

`ROLE_SUPERVISOR` has no consumer anywhere. It is a name in the registry, which is
precisely what `role.proto`'s own comment warns about: "a role nothing consults is
a name in a registry pretending to be a control". Granting it records who is
watching a country and confers nothing.

## Validators are a different registry, and the difference is real

`x/validatorgov` collects a validator's jurisdiction in its application, alongside
its legal entity and beneficial owner, and validates it against the same assigned
country list. It does **not** write it into `x/alias`.

That is not an oversight and it is not being fixed by merging them. The two facts
have different provenance:

- a validator **declaring** Senegal is a signed claim by its operator;
- Senegal's participant **recording** Senegal is a finding by whoever did the KYC.

Merging either direction destroys something. Overwriting the declaration with the
record throws away the signature that makes a false declaration an offence.
Treating the declaration as a record makes the registry a place where accounts
place themselves, which is the one thing it exists to prevent.

So they are reconciled and the disagreement is made visible:

```bash
blockchaind query validatorgov jurisdiction-reconciliation
```

Every approved validator, its declared country, its recorded country if it has
one, who recorded it, and a state: `AGREE`, `DISAGREE`, or `UNRECORDED` — plus
counts, so "three disagree" is visible without scanning. The zero value of that
enum is reserved as unspecified, like every enum on this chain, so an unfilled row
can never read as agreement.

`UNRECORDED` is the ordinary state for a validator nobody has placed, and it is a
real answer rather than a fault: a validator's operator account can be put in a
jurisdiction the same way any other account is, and until somebody does, no
authority's perimeter contains it.

To place one, the enrolment has a command of its own, and it reads the
reconciliation first:

```bash
blockchaind query validatorgov jurisdiction-reconciliation -o json > reconciliation.json

./ceremony country validator --dossier country-SN.json \
  --proposer <a foundation custodian> \
  --candidate <the validator's operator account> \
  --reconciliation reconciliation.json
```

A foundation proposal, for the same reason the first institutions need one: a
validator's operator account is nobody's payment customer, so no approved
participant acts for it, and `MsgSetJurisdiction` accepts only that participant, a
foundation administrator or governance.

**It refuses to place a validator in a country it did not declare**, and that is
why it wants the reconciliation rather than just an address. Placing one against a
different declaration would *manufacture* the disagreement that query exists to
reveal, with the foundation's signature on it. If the two really do differ, that
is a finding for a person: either the validator declared a country it does not
answer to, or somebody is about to record the wrong one. The declaration is signed
by the operator, so if it is the declaration that is wrong, that is what has to be
corrected — through `x/validatorgov`, by whoever signed it.

It also refuses a candidate that is not an *approved* validator, because an
application is not an approval; and where the two already agree it says there is
nothing to do rather than composing a proposal that changes nothing. Where the
state is `DISAGREE` it will compose one — correcting a recorded country is exactly
a foundation administrator's job — but it labels the proposal a correction and
names on screen whose finding is being overwritten.

What it never does is copy one registry into the other. It writes the `x/alias`
record and leaves the declaration where it is. The two remain two, and the
reconciliation query remains the thing that shows them side by side.

---

## When something goes wrong

**A group was created with the wrong members.** Do not grant anything to it.
`confirm` will refuse it, which is the tool doing its job. Create the group again
from the same `group.json` — the office's keys are untouched — and confirm the new
one. The abandoned group policy sits on the chain holding nothing; note it in the
record.

**The enrolment proposal was accepted and did not execute.** `verify` refuses and
names the missing grant. Look at the proposal rather than the transaction: it may
not have reached three votes, it may have expired, or one message in it may have
failed and taken the rest with it. Re-submitting the same proposal is ordinary and
safe — granting the same triple again rewrites the attribution and the height and
changes nothing else, deliberately, so that a proposal resubmitted after a timeout
does what an operator expects.

**An office's keys are compromised.** Revoke first, ask questions afterwards. The
foundation may revoke what the foundation may grant, so this is one 3-of-5 vote
rather than a governance cycle — that asymmetry is deliberate, because the reason
to revoke in a hurry is that somebody is abusing an authority, and granting is the
act that wants friction rather than removing.

```bash
blockchaind query alias role-grants <office policy address>
# then a foundation proposal carrying MsgRevokeRole for each grant, named exactly
```

Revoking a grant that was never made is an **error** rather than a quiet success,
on purpose: "nothing to revoke" is how a proposal that named the wrong country
passes while leaving the authority it meant to remove in place.

**A super user leaves.** The office's group administers itself, so a
`MsgUpdateGroupMembers` proposal to that office, voted by its own threshold. The
foundation is not involved and does not need to be. Read
[replacing a custodian](key-ceremony.md#replacing-a-custodian) first — the ratchet
described there applies to any M-of-N: dropping three-of-three to three-of-two is
not "one short", it is a veto for everybody left. Departures and replacements are
one decision.

Do it as one message. Removing a member on the understanding that a replacement
will be added next week takes the office below its minimum, and the office stops
being able to act the moment that proposal executes.

---

## When an office falls below its minimum

The office is still an office. Its grants are still in the store, its queries still
answer, and every action it attempts is refused:

```
yml1dlszg2s… was granted its authority as 3-of-5 and is now 3-of-4. An office may
grow; it may not fall below the shape it was granted under. Restore it with a
MsgUpdateGroupMembers or a MsgUpdateGroupPolicyDecisionPolicy voted by the office
itself, or have the grant re-made: this office no longer keeps the M-of-N its
authority was granted under
```

An office acts through its own group, so that text arrives where nobody looks
first. **The transaction's code is 0.** An `x/group` proposal that fails in
execution produces a successful transaction carrying
`cosmos.group.v1.EventExec` with `result: PROPOSAL_EXECUTOR_RESULT_FAILURE` and
the refusal in its `logs` attribute:

```bash
blockchaind query tx <exec hash> -o json   # look at events, not at code
blockchaind query group proposal <id> -o json   # a failed proposal is not pruned
```

That is error code **23**, `ErrOfficeShape`, from `x/alias`. Note what it is not:
it is not `ErrOutOfScope` (code 19), which means this office never held that
authority here. The two send you to different places, which is why they are
different codes — code 19 is a question about the grant, code 23 is a question
about the group.

**Diagnose it.** Two queries, and the second is the one that matters:

```bash
blockchaind query alias role-grants <office policy address> -o json
# required_shape on each grant is what the office is held to

blockchaind query group group-policy-info <office policy address> -o json
blockchaind query group group-members <group id> -o json
# the threshold it has now, and how many members hold a weight
```

If `required_shape` is absent from a grant, that grant predates this rule and
constrains nothing — see below.

**Fix it: the office restores itself.** No foundation involvement, no governance,
no re-grant. The office proposes to itself and votes with its own threshold:

```bash
# a member left and was not replaced — add the replacement
blockchaind tx group submit-proposal <proposal.json>   # MsgUpdateGroupMembers
# or the threshold was lowered — put it back
blockchaind tx group submit-proposal <proposal.json>   # MsgUpdateGroupPolicyDecisionPolicy
```

The authority comes back on the next block, with nothing else to do. That is the
whole point of recording the requirement rather than revoking on discovery: the
chain refuses while the office is short and stops refusing when it is not, and
nobody has to be watching either way.

The one case the office cannot vote its way out of is the one where it has fallen
so far that it cannot reach its own threshold — a three-of-five that lost three
keys cannot pass anything, including its own repair. The refusal says so
(`no set of members can act at all`), and the way out is the foundation revoking
the grants and the country running the office's ceremony again.

**Relax the requirement instead.** A country that has genuinely decided its
enforcement authority is a two-of-three now, not a three-of-five, changes the
grant rather than the group. It is a foundation act and it takes two messages, in
one proposal:

```
MsgRevokeRole  { holder, role, jurisdiction }
MsgGrantRole   { holder, role, jurisdiction, required_shape: 2-of-3 }
```

Two rather than one, because a re-grant may **raise** a recorded requirement and
may never lower or drop one. The reason is the resubmission: the obvious way to
re-make a grant is to compose it from the proposal summary, the summary does not
list `required_shape`, and a single `MsgGrantRole` with the field omitted would
remove the pin while claiming to change nothing. `x/group` executes a proposal's
messages together or not at all, so the revoke-and-grant pair is atomic and the
office is never briefly unpinned. Attempting it as one message is refused:

```
yml1dlszg2s… already holds ROLE_PAYMENTS_AUTHORITY in GH with a required shape of
3-of-5, and this grant records none. Omitting required_shape on a re-grant would
remove the pin; to relax it deliberately, revoke the grant and make a new one in
the same proposal
```

And a grant requiring more than the office is refused before it is written, which
is the other half of the same rule:

```
a grant requiring 3-of-5 cannot be made to yml1c799jdd…, which is 1-of-1. Either
the office is not the one this grant was meant for, or it has to be brought up to
3-of-5 before it can hold the authority
```

**Grants made before this existed.** A grant with no `required_shape` is treated
as recording no requirement, and its holder can shrink to a single key without
losing anything. That is deliberate — the alternative is a chain upgrade that
disables every existing authority at once — and it is a real gap until each one is
re-granted. To close it for an office, one foundation proposal per grant carrying
`required_shape`; adding a requirement to a grant that had none is an ordinary
re-grant and needs no revoke first. `blockchaind query alias role-holders <CC>`
lists who to do it for.

`x/constitution`'s ante gate still protects the **foundation's** group shape by a
different mechanism: it refuses the transaction that would change it, rather than
refusing the actions afterwards. The difference is deliberate. There is one
foundation, its shape is in the constitution, and a gate can read it on every
transaction; there are as many offices as there are countries times offices, their
shapes are on their grants, and an ante gate would not see an office acting through
an interchain account or `x/authz` at all.

---

**Full reference:** [x/alias](../reference/alias.md) for the grants, the
jurisdictions and every error code; [x/validatorgov](../reference/validatorgov.md)
for the reconciliation query; [x/constitution](../reference/constitution.md) for
what governance can and cannot change. All generated from the source.
