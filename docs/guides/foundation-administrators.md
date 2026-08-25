# Appointing a foundation administrator

How the one account that can move a customer out from under the authority
investigating them comes into existence, and who gets to decide.

**You need:** three to eight people who do not report to each other, a device
each, about ninety minutes for the ceremony, and a governance voting period —
which on this chain is thirty minutes and on a real one is days.

**You will end with:** an M-of-N `x/group` account holding
`ROLE_FOUNDATION_ADMINISTRATOR` at the chain-wide scope, a signed record, and a
governance proposal in the chain's permanent history saying who agreed to it.

---

## What the power actually is

A foundation administrator is the account that may **correct** the country
recorded against any account on the chain.

That sounds administrative. It is not. Read
[`MsgSetJurisdiction`](../reference/alias.md) and note who may write a country:

| Situation | Who may record it |
| --- | --- |
| First recording | the approved participant that onboarded the account — it did the KYC, so it is the only party that knows |
| A **correction** | a foundation administrator, or governance |
| An account declaring its own | nobody, ever |

The correction is the one that matters. Every jurisdictional rule on this chain
— who may freeze, who may seize, who may supervise, who may see a payment —
resolves through the country recorded against an account. An administrator who
changes Amara's country from NG to KE has moved her out of the perimeter of the
Nigerian authority investigating her and into one that has never heard of her.
Nothing else on the chain can do that.

It also **retires her identifier and issues a replacement**, in the same
message. It has to: the identifier carries the country as a prefix, and a prefix
that can go stale is a prefix that can lie. So a correction is visible — the old
identifier is tombstoned and never reissued — but the visibility is after the
fact.

And administrators are the only accounts permitted to hold an identifier with
**no country at all**, carrying `ZZ`, which ISO 3166-1 reserves permanently and
will never assign. That is the second half of what the role is for: an account
that belongs to no national perimeter, because it is the chain-wide authority and
there is no country that would be true of it.

Two more powers come with it, and they are the same class of act — deciding who
may see inside a perimeter rather than who may act in one. A foundation
administrator may **appoint a country's regulator**, the account entitled to be
sealed into the encrypted payload of every payment settling there, and may grant
the time-boxed **auditor** role that reads across accounts. `x/alias` accepts
governance or a foundation administrator for both, and nobody else.

### Placement and authority are two questions

The chain asks about the role in two different ways, and the split is worth
knowing before you read a refusal.

**Where is this account** is a fact. It is asked by the identifier issuer when it
decides whether `ZZ` may be used, and by every perimeter check about a *target*.
That question reads only whether the grant is present.

**May this account act** is not a fact but an authority, and an office's shape
bears on an authority where it does not bear on a fact. If the
grant records a `required_shape`, an administrator that has fallen below its
M-of-N is refused with `ErrOfficeShape` — naming the shape and the fix — rather
than with "you are not an administrator".

The two are deliberately not one function. If placement consulted the shape, an
administrator that lost a custodian would stop having a country at all: its own
identifier would become unissuable and every authority action against it would
fail with "no recorded jurisdiction", which is a sentence about the wrong thing
entirely.

## Who appoints them, and why it is not the foundation

An administrator is a **role grant**: `ROLE_FOUNDATION_ADMINISTRATOR`, held at
the chain-wide scope `*`, written by `MsgGrantRole` on `x/alias`.

The rule that decides the signer is the one that governs every grant on this
chain. A grant naming a **country** may be made by governance or by the
foundation; a grant naming the **chain-wide** scope is governance and nobody
else. `GrantRole` refuses `*` from every other signer before it even reads the
constitution to find out who the foundation is, because an acceptance must never
depend on a store read that could fail.

So the foundation's own M-of-N **cannot appoint an administrator**. Not "should
not" — cannot. It is worth being clear about why that is right rather than an
oversight:

- The foundation is already the account that admits countries and receives every
  seized asset. An account that could also decide who may rewrite a jurisdiction
  would be able to grant itself the ability to move any customer anywhere, and
  the perimeter would be advisory.
- The role is the single exception to "every account carries a country". Widening
  it is the kind of act that should cost a public vote and a waiting period, not
  three signatures on a call.

### Chain-wide or nothing, and why that is the load-bearing rule

A grant of this role that names a country is **refused** — `ChainWideOnly` in
`x/alias`, checked in `GrantRole` and again in genesis validation.

That refusal is what keeps the appointment exactly as governance-only as the
parameter list it replaced. A country-scoped grant is one the *foundation* may
make; refusing the country form leaves the chain-wide form, and the chain-wide
form is governance's alone. Without it, moving the administrators out of the
parameters would have quietly handed the foundation the power to appoint the
accounts that stand outside every country — a widening nobody voted for, arriving
as a side effect of a refactor.

It would also be meaningless rather than merely wrong. What the role exempts is
the *absence* of a national perimeter, so an administrator of one country is an
account claiming an exemption from a rule it is already inside.

**No grant is the default and no grant is safe.** With nobody holding the role,
the exemption grants nothing at all. What it costs is that no recorded country
can ever be corrected, that no country's regulator can be appointed, and that a
new country cannot be enrolled — the first institutions in one have no
participant to record their jurisdiction, so somebody with the exemption has to
do it. That is the trade, and it is a real one in both directions.

## The trap that used to be here, and why it is gone

This section used to be the longest in the guide, under the heading "the one
thing that will bite you", and it is worth recording what it said rather than
deleting it — because "the failure mode this used to have" is exactly what a
reader coming back to this page will be looking for.

The appointment used to be `alias.params.foundation_administrators`, a repeated
field of up to eight addresses, set by `MsgUpdateParams`. `MsgUpdateParams`
carries a `Params` **message**, not a field mask, so setting it replaced the
whole object. "Appoint one administrator" was really *read the current
parameters, add one address, and resubmit every parameter*. Composed by hand, the
proposal that appointed one administrator silently dropped the administrators
already appointed, or reset `payload_length` to its default on a chain that had
raised it. Nothing on the chain caught either: `Params.Validate()` bounded the
list and refused duplicates, and a list shorter than the one before it is a
perfectly valid list. The proposal passed, executed, and read as correct. The
only evidence was a count that went down, in a field nobody was watching.

**That trap is gone, and it is the single best thing about the change.**
`MsgGrantRole` names one holder and is additive. It cannot drop an administrator
it does not mention, and it changes no parameter of `x/alias` at all, so it
cannot re-parameterise the chain while reading as an appointment. A proposal
composed from a view of the chain that went stale during the voting period is
now merely out of date, where before it was destructive.

What is left of the old machinery is one read, for one reason: the cap. See
[the sequence](#5-the-governance-proposal), where `--chain-wide-grants` is
required and the tool says in as many words that it is required for the cap and
for nothing else.

The related trap is gone with it. `payload_length` reading back as `0` used to be
a refusal in both interfaces, because proto3 cannot tell a zero from a field
nobody filled in and a tool that defaulted it to 8 would compose a proposal that
re-parameterised the chain while reading as an appointment. The appointment no
longer carries `payload_length`, so it no longer has an opinion about it.

## What the chain does not check

Both of the things this section used to warn about are now checked, and one thing
it never mentioned is not. It is worth being precise about which is which.

**An entry that is not an address.** `Params.Validate()` never checked this: a
mistyped address passed a governance vote, occupied one of the eight capped
places and granted the exemption to nobody. `GrantRole` decodes the holder, so
that failure is now a refused message rather than a passed proposal.

**A holder that is not a group account.** The parameter matched by exact address
equality and did not care what kind of account it was, so the interfaces warned
and proceeded and said plainly that the chain would accept a single key.
`GrantRole` refuses a holder that is not an `x/group` account. An office that is
one key is one bribe, and this particular office can move any customer on the
chain — the chain now says so itself.

**What is still not checked is the shape the office keeps afterwards.** A grant
may record a `required_shape`, the M-of-N its holder must keep, and the chain
then resolves the group policy on every authority action and refuses an office
that has fallen below it. A grant that records **none** constrains nothing: its
holder is a group account on the day it is appointed and can vote itself down to
a one-of-one the next morning, with nothing notified and nothing refused. An
office administers itself — that is what makes its membership changeable by its
own members and nobody else — so this is not a hypothetical.

The appointment ceremony deliberately records no shape, and
[the section on the migration](#what-the-migration-did-to-the-administrator-you-already-have)
has what to do about it. The short version: the only numbers to hand are read out
of the group file the custodians signed, and a requirement captured from the
group that turned up ratifies a one-of-one as readily as a three-of-five. A
requirement has to be decided before the day or it is not a requirement.

## Before the day

### Who the custodians are

Three at minimum in this group, at most eight accounts holding the role at once
across the whole chain, and **not people who report to each other**. The whole value of M-of-N is that some
of them would have to agree against the interests of the others, and colleagues
with a shared manager cannot do that. If separate organisations are not
available, separate line managers and separate offices is the floor.

### The threshold

Never `1` — that is the single key this ceremony exists to abolish, and the tool
refuses it. Never equal to the membership either: a 4-of-4 means losing one key
freezes the account forever, and the tool refuses that too. 3-of-5 or 3-of-4 are
the shapes that work.

### The cap

Eight administrators, chain-wide, from `MaxFoundationAdministrators`. It kept its
number and its reason when the administrators stopped being a parameter list, and
the argument for it changed shape rather than going away.

What it used to defend against was a proposal that appended a hundred addresses
to a repeated field and passed because nobody scrolled. That exact failure is
gone: a grant is one message naming one holder, with the holder decoded, the role
named, an event emitted and a height recorded, so a hundred administrators is a
hundred visible acts.

What it still defends against is the two places where a set can grow without
being read one entry at a time — a governance proposal carrying many messages,
and a genesis file. Both are counted: `GrantRole` counts the chain-wide grants of
the role before writing another, and `GenesisState.Validate` counts them in the
file.

The count **excludes the holder being granted**, so re-granting an existing
administrator is never refused by a place it already occupies. That is what makes
a proposal resubmitted after a timeout safe, and it is what makes it possible to
add a `required_shape` to the eighth administrator's grant at all.

At the cap, the tools refuse an appointment and tell you to remove somebody
**first, in its own proposal**, so both decisions are voted on separately.

---

## The sequence

### 1. Generate the keys

Same machinery as [the key ceremony](key-ceremony.md) — read that first, and in
particular the section on the [hosted path versus the air-gapped
one](key-ceremony.md#two-ways-to-run-it-and-which-is-stronger). The air-gapped
path is stronger and the difference is not a formality.

```bash
ceremony host --out ./out --public-url https://ceremony.example/
```

On the coordinator's screen, tick **"These keys are for a foundation-administrator
group"** and leave the country blank. Both matter:

- The **country blank** because an administrator has no perimeter — that is what
  it *is*. A country and the administrator exemption are opposites, and the tool
  refuses them together rather than choosing between them.
- The **box ticked** because the marker is inside the parameters fingerprint every
  custodian reads aloud before they generate. Without it, keys generated "for the
  Yamale foundation" could be stood up as an administrator group and nothing
  anybody saw would have said so — and the group would be recorded on chain as
  `Yamale foundation`, indistinguishable in the one field a human reads from the
  account that holds every seized asset.

Each custodian's page says, in as many words, that the key becomes one share of a
group that may correct any account's country. They read the params fingerprint
aloud, generate, write twenty-four words on paper, send only the public half,
compare the **group** fingerprint aloud, and sign an attestation.

You end with `out/group.json`. There is **no** `group-genesis.json` and **no**
`constitution-invariants.json`, and both omissions are deliberate: this group is
created by a transaction on a running chain, and a constitutional fragment
naming it the destination of every seized asset would be the most dangerous file
this tool could produce — its name already contains the word "foundation".

### 2. Write the config

```json
{
  "ceremony": "Yamale foundation administrators",
  "chain_id": "yamale-devnet-2",
  "group": "./out/group.json",
  "reason": "Appointed at the ceremony of 2026-08-23; four fingerprints read aloud and attested."
}
```

Note what is **not** in it: the threshold, the members, and the address. The
first two come from `group.json`, so the config cannot disagree with what the
custodians signed for. The third comes from the chain.

The `reason` is required. It becomes the proposal's summary, which is the only
explanation most of the voting set will read.

```bash
ceremony administrators init --config administrators.json
```

This refuses a `group.json` from the wrong kind of ceremony — the foundation's,
or a country office's — in both directions, and says which.

### 3. Create the group

```bash
ceremony administrators group --dossier appointment-yamale-foundation-administrators.json
```

It prints the transaction and **nothing else**. No proposal, no address.

```bash
blockchaind tx group create-group-with-policy <your-key> \
  "<metadata>" "<metadata>" \
  administrators-...-members.json administrators-...-policy.json \
  --group-policy-as-admin --chain-id yamale-devnet-2
```

Any funded key can broadcast it. `--group-policy-as-admin` makes the policy its
own admin, so the key that signs keeps nothing — it cannot change the membership,
the threshold, or who administers the group afterwards.

### 4. Read the address back

**This is the step that cannot be skipped, and here is what it costs to skip it.**

An `x/group` policy address derives from the group policy sequence number alone —
not from the members, not from the threshold, not from the admin. So an address
computed offline commits to nothing whatever about who controls it.

This is not hypothetical, and it happened again while this guide was being
written. A rehearsal appointment on `yamale-devnet-2` printed a predicted address
of:

```
yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj
```

which is byte-for-byte the value of `enforcement_recovery_destination` in that
chain's constitution — **the foundation's own account** — because both were policy
sequence 1. The chain gave the group sequence 2, and a completely different
address. Carried into an appointment proposal, the prediction would have been a
governance vote appointing the foundation a foundation administrator: passing,
executing, and reading as correct at every step, with the group the ceremony was
actually held for holding nothing.

So `confirm` takes a `--foundation` flag and refuses that address outright. Pass
it. Without it the step says so rather than staying quiet, because a check
somebody skipped silently is not a check.

```bash
blockchaind query tx <hash> -o json > tx.json
# the address is in the EventCreateGroupPolicy in that file
blockchaind query group group-policy-info <address> -o json > policy.json
blockchaind query group group-members <group id> -o json > members.json
blockchaind query constitution invariants -o json   # for --foundation

ceremony administrators confirm --dossier appointment-*.json \
  --tx tx.json --policy policy.json --members members.json \
  --foundation <the foundation's policy address>
```

A broadcast reporting `code: 0` has been **accepted** and has not executed. The
code that matters is in the queried result.

`confirm` checks that the policy at that address really is this group: the same
members both ways as a set, equal weight, the same threshold, administering
itself. Set equality both ways, not "the ceremony's members are among the
group's" — a group with an extra member has an extra vote, and a 3-of-4 with five
members is a 3-of-5.

### 5. The governance proposal

```bash
blockchaind query alias chain-wide-grants -o json > chain-wide-grants.json
blockchaind query auth module-account gov -o json > gov-account.json
blockchaind query gov params deposit -o json      # for --deposit

ceremony administrators propose --dossier appointment-*.json \
  --chain-wide-grants chain-wide-grants.json --gov-account gov-account.json \
  --deposit 1000000uyml
```

Both files are required and neither has a default, and the two reasons are not
the same one.

`--chain-wide-grants` is required **for the cap and for nothing else**. At most
eight accounts may hold the role at once, and `GrantRole` counts them when the
proposal *executes* — so a ninth appointment composed without reading them costs a
full voting period and then fails in a transaction log nobody is watching. It has
to be that query and not `role-grants <holder>`, which renders the same shape for
one account: a file from the second would report a chain with no administrators
at all, and the count would read as seven places free on a chain that has none.

`--gov-account` is required because the authority is read off the chain rather
than compiled in. A chain-wide grant is refused from every signer but the
governance module account, and an address that had gone stale would produce a
proposal that passed its vote and was then refused when it executed.

The tool prints the accounts that already hold the role, marked **untouched**,
and the one this proposal adds. Read that list — not because anything in it is at
risk, but because it is the only screen on which an operator sees the whole of
the capped set before adding to it. A list that is out of date makes the count
wrong and the proposal still correct, which is the difference this change bought.

```bash
blockchaind tx gov submit-proposal appoint-...-proposal.json \
  --from <your-key> --chain-id yamale-devnet-2 \
  --gas 600000 --fees 20000uyml
```

The gas is explicit because the 200,000 default runs out part-way through a
proposal carrying a message and fails with `code: 11` — which reads like a
rejected proposal rather than an unfunded transaction.

The proposal is **not** expedited, deliberately. Shortening the vote on the
appointment of the account that can move any customer out from under their
regulator buys nothing except less time for somebody to notice.

### 6. Vote

The proposal id is in the queried transaction's events and nowhere in the
broadcast's own output.

```bash
blockchaind query tx <hash> -o json          # find the proposal id
blockchaind tx gov vote <id> yes --from <key> --chain-id yamale-devnet-2
```

Each validator votes from its own node, where its key is. The governance console
composes the command for each one.

### 7. Verify that it actually happened

```bash
blockchaind query alias chain-wide-grants -o json > chain-wide-grants.json
ceremony administrators verify --dossier appointment-*.json \
  --chain-wide-grants chain-wide-grants.json
```

**A proposal that PASSED and a proposal that took effect are two different
states.** A proposal can pass its vote and still fail when it executes, which
leaves the registry exactly as it was and reports it in a transaction log nobody
is watching. `PROPOSAL_STATUS_PASSED` is not evidence. The two failures this step
actually catches are a grant refused at execution by the cap, and a grant refused
because the holder is not a group account.

`verify` records the **whole** set of accounts holding the role, not just this
group. That is deliberate, and the reason has changed with the mechanism: the set
is no longer something a careless proposal can destroy, but it is still the
single exception to every account on the chain having a jurisdiction, and how
many accounts hold it belongs on a record somebody reads years later without a
chain to query.

### 8. The record

```bash
ceremony administrators record --dossier appointment-*.json --config record.json
```

Print it, read it in the room, sign it on paper. It states what the power is, that
this ceremony did not confer it, and that the address was read off the chain
rather than predicted.

---

## Using it

The administrator is the **group**, so a correction is a group proposal that its
custodians vote on. Write the message:

```json
{
  "group_policy_address": "<the administrator group>",
  "messages": [
    {
      "@type": "/blockchain.alias.v1.MsgSetJurisdiction",
      "recorder": "<the administrator group>",
      "account": "<the account being corrected>",
      "country": "KE"
    }
  ],
  "metadata": "",
  "proposers": ["<one custodian>"],
  "title": "Correct <account> to KE",
  "summary": "Why, and on whose authority. A case number if there is one."
}
```

```bash
blockchaind tx group submit-proposal correction.json --from <custodian> \
  --chain-id yamale-devnet-2 --gas 600000 --fees 20000uyml
# then M custodians vote yes, and one of them executes
```

Two things to know:

- `country` must be an **assigned** ISO 3166-1 alpha-2 code. `ZZ` and `*` are
  refused here: `ZZ` marks the absence of a perimeter, and recorded as a
  jurisdiction it would let an ordinary account be issued an identifier that reads
  as chain-wide authority.
- Correcting to the **same** country is a deliberate no-op that retires nothing.
  Rotating an identifier for a no-op would destroy a live handle for free, and a
  message resubmitted after a timeout is the ordinary way that happens.

Check that it landed:

```bash
blockchaind query alias jurisdiction <account> -o json    # the new country
blockchaind query alias id-of <account> -o json           # the new identifier
blockchaind query alias retired <the old identifier> -o json   # true, forever
```

`tx group exec` below the threshold returns **code 0 and does nothing**. The only
signal is the `EventExec` attribute in the transaction, which reads
`PROPOSAL_EXECUTOR_RESULT_NOT_RUN` rather than `SUCCESS`.

## Removing one

`MsgRevokeRole`, naming the holder, the role and the chain-wide scope exactly —
and governance again, because only governance may touch a grant at that scope in
either direction.

Revoking a grant that was never made is an **error** rather than a quiet success,
on purpose: "nothing to revoke" is how a proposal that named the wrong scope
passes while leaving the authority it meant to remove in place.

Removing the **last** administrator leaves nobody holding the role, which is a
real state and the documented default. Nothing blocks it. What it means: no
recorded country can be corrected until governance appoints somebody again, no
account can hold a `ZZ` identifier, no country's regulator can be appointed, and
a new country cannot be enrolled. Governance can appoint again by another
proposal exactly like this one, so it is recoverable — it is just not reversible
by the people who did it.

## What the migration did to the administrator you already have

A chain that ran under the parameter has an administrator in it, and the
`x/alias` v2-to-v3 migration — which runs in the `roles-that-do-something`
upgrade — is what carries that account across. Read this before deciding it did
nothing.

`Migrate2to3` reads the retired `foundation_administrators` field out of the
**raw stored parameter bytes** — a reserved field is one the generated Go type no
longer has, and unmarshalling with the current type would drop the addresses
silently, with no error, because dropping an unknown field is what a protobuf
decoder is supposed to do. Every address it finds becomes a chain-wide grant of
`ROLE_FOUNDATION_ADMINISTRATOR`, attributed in `granted_by` to **governance** —
because governance is what appointed them, the parameter having been set by
`MsgUpdateParams` and nothing else. Attributing it to the upgrade would have lost
the one fact `granted_by` exists to record. The parameters are then rewritten
through the current type, which drops the dead field from the store rather than
leaving it there until the next `MsgUpdateParams`.

**What the migration deliberately does not do is apply today's rules.** The
carried grants record no `required_shape`, and their holders are not required to
be `x/group` accounts. Both are true of a grant made today and neither was true
of the administrators a parameter list held, which were bare addresses. A
migration that applied the new rule would not be carrying an authority across; it
would be deleting one and calling the deletion a standard — and the account it
deleted is the one that can correct a country. The cap is not enforced during the
migration either, for the same reason: the parameter's own `Validate()` refused a
ninth entry so a chain cannot be carrying more than eight, and halting every node
on an upgrade is a worse answer than carrying the state that exists and letting
the next grant be refused.

### What to do afterwards

Two things, both deliberate acts rather than cleanup.

**Check that the holder is actually a group.** The parameter never required it and
the migration did not impose it, so a chain that appointed a single key still has
one.

```bash
blockchaind query alias chain-wide-grants -o json
blockchaind query group group-policy-info <holder> -o json
# a plain-account answer here means it is a single key
```

There is no in-place fix for that one: the authority has to be revoked and
granted again to a group the chain will accept, which is two governance
proposals, and in between the chain has no administrator.

**Consider re-granting with a `required_shape`, and decide the numbers first.**
Adding a requirement to a grant that had none is an ordinary re-grant and needs no
revoke: `GrantRole` counts the cap excluding the holder precisely so that a grant
can be amended. `assertShapeNotReduced` then stops any later re-grant lowering the
bar or dropping it by omitting the field, so the pin is one-way once it is on.

The numbers have to be agreed before anybody looks at the group, and this is why
the appointment ceremony records none: the only numbers to hand at ceremony time
are read out of the group file the custodians signed, and a requirement captured
from the group that turned up ratifies a one-of-one as readily as a three-of-five.
Writing 3-of-5 onto a grant because the group was a 3-of-5 puts a requirement on
the chain that nobody decided, and it reads on the signed record as though
somebody had.

A grant requiring more than the office currently is will be refused before it is
written, which is the other half of the same rule — so the office and the
requirement have to agree on the day the proposal executes, not on the day it was
composed.

## What a run of this actually looked like

A rehearsal on `yamale-devnet-2`, 2026-08-23, so the numbers below are checkable
rather than illustrative. It predates this change and was composed as a
`MsgUpdateParams`; the ceremony steps, the address hazard and everything the
administrator then did are unchanged, and the row that named the parameter is
marked. Nobody on the chain could correct a recorded country before it.

| Step | What happened |
| --- | --- |
| Ceremony | 3-of-4, params fingerprint `E1TH-KP2X-GWM6-X594`, group fingerprint `RVA4-6W1S-RX0V-GNDK` |
| Predicted address | `yml1afk9zr2…3xm8uj` — **the foundation's own**, sequence 1 |
| Real address | `yml1dlszg2s…rmuayr`, group 2, created at height 28480 |
| Proposal | gov #2, deposit 1 YML, 30-minute vote, passed 65,000,000,000 yes / 0 no |
| After | the parameter had one entry; `payload_length` still 8. The v2-to-v3 migration has since carried that entry into a chain-wide grant of the role, attributed to governance |

Then, as the appointed group, three of the four custodians voting:

| Act | Result |
| --- | --- |
| Record alice in `NG` | `jurisdiction_recorded`, `recorded_by` the group, height 28854 |
| alice registers | identifier `NGH8QTHP2B4` |
| **bob** tries to correct her to `KE` | refused, `code 12` in codespace `alias`: *"is recorded in NG: this account's jurisdiction is already recorded; only a foundation administrator may correct it"* |
| **alice** tries to correct herself | refused, identically |
| The **group** corrects her to `KE` | `retired: NGH8QTHP2B4`, `id: KEM1BMZ66YP`, height 28873 |
| `query alias retired NGH8QTHP2B4` | `true` — and it resolves to no account, permanently |

That last row is the point of the whole feature. The prefix followed the country,
the old identifier was tombstoned rather than repointed, and the correction is on
the record with the group that made it named in it.

## Checking somebody else's appointment

```bash
# who holds the exemption, how many places are left, and who granted each one
blockchaind query alias chain-wide-grants -o json

# one account's grants, from the other end
blockchaind query alias role-grants <address> -o json

# is it a group, and who is in it?
blockchaind query group group-policy-info <address> -o json
blockchaind query group group-members <group id> -o json

# a plain-account answer here means it is a single key
```

`chain-wide-grants` is the query to start from and `role-grants <holder>` is the
one to finish with. The first enumerates the exception; the second answers a
question about one account and, asked first, would report a chain with no
administrators at all to somebody who had guessed the wrong address.

Two things the registry now tells you that the parameter could not. Every grant
carries `granted_by` and `granted_at_height`, so "who says this account may
rewrite a jurisdiction, and since when" has an answer on chain rather than in
somebody's proposal archive. And it carries `required_shape`, which is the field
worth reading: absent means the grant predates the rule or was made without one,
and its holder can shrink to a single key without losing anything.

An address holding the role that `group-policy-info` does not recognise is a
single key holding the power to move any customer on the chain. Since the change
that cannot arrive through `GrantRole` any more — it can only have been carried
across by the migration, or seeded in genesis.

## When something goes wrong

**The proposal passed and nobody holds the role.** It failed at execution.
`blockchaind query gov proposal <id> -o json` and read the `failed_reason`. The
three causes worth checking first are an authority that is not the gov module
account, a holder that is not an `x/group` account, and the cap — eight accounts
already holding the role means the ninth grant is refused after the vote rather
than before it.

**The grant is there and the group cannot act.** Read `required_shape` on the
grant against `group-members`. An office that has fallen below a recorded shape is
refused with `ErrOfficeShape` — which is a statement about the group, not about
the grant — and the office repairs it by its own vote, with no governance
involved. Check `group-members` against the ceremony record while you are there:
a membership that does not match means the group at that address is not the group
the record describes, and the appointment went to whoever created that policy.

**A grant names an address nobody holds a key for.** It grants nothing and
occupies one of the eight places. Remove it with `MsgRevokeRole` naming the
holder, the role and `*` exactly. The chain accepted it only if it arrived through
genesis or the migration; `GrantRole` decodes the holder and would have refused
it — see [what the chain does not check](#what-the-chain-does-not-check) for which
of the old omissions closed and which one did not.
