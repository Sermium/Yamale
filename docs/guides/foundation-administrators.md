# Appointing a foundation administrator

How the one account that can move a customer out from under the authority
investigating them comes into existence, and who gets to decide.

**You need:** three to eight people who do not report to each other, a device
each, about ninety minutes for the ceremony, and a governance voting period —
which on this chain is thirty minutes and on a real one is days.

**You will end with:** an M-of-N `x/group` account named in
`alias.params.foundation_administrators`, a signed record, and a governance
proposal in the chain's permanent history saying who agreed to it.

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
will never assign. That is the second half of what the parameter is for: an
account that belongs to no national perimeter, because it is the chain-wide
authority and there is no country that would be true of it.

## Who appoints them, and why it is not the foundation

`alias.params.foundation_administrators` is a **module parameter**. The only
message that can change it is `MsgUpdateParams`, whose authority is the
governance module account and nothing else.

So the foundation's own M-of-N **cannot appoint an administrator**. Not "should
not" — cannot. It is worth being clear about why that is right rather than an
oversight:

- The foundation is already the account that admits countries and receives every
  seized asset. An account that could also decide who may rewrite a jurisdiction
  would be able to grant itself the ability to move any customer anywhere, and
  the perimeter would be advisory.
- The list is the single exception to "every account carries a country". Widening
  it is the kind of act that should cost a public vote and a waiting period, not
  three signatures on a call.

Compare [`GrantRole`](../reference/alias.md), where the foundation *can* act for
a country but chain-wide scope stays governance-only, for the same argument by
the same road.

**Empty is the default and empty is safe.** With nobody named, the exemption
grants nothing at all. What it costs is that no recorded country can ever be
corrected, and that a new country cannot be enrolled — the first institutions in
one have no participant to record their jurisdiction, so somebody with the
exemption has to do it. That is the trade, and it is a real one in both
directions.

## The one thing that will bite you

`MsgUpdateParams` carries a `Params` **message**, not a field mask. Setting it
**replaces the whole object**.

So "appoint one administrator" is really *read the current parameters, add one
address, and resubmit every parameter*. Composed by hand, the proposal that
appoints one administrator silently:

- drops the administrators already appointed, or
- resets `payload_length` to its default on a chain that had raised it.

**Nothing on the chain catches either.** `Params.Validate()` bounds the list and
refuses duplicates; a list shorter than the one before it is a perfectly valid
list. The proposal passes, executes, and reads as correct. The only evidence is
a count that went down, in a field nobody was watching.

Two tools absorb this, and both refuse rather than defaulting:

- **`clients/governance`** reads the current parameters, changes exactly one
  thing, and shows you the **whole object before and after** — including what did
  not move, because "payload_length is unchanged" is information when the message
  replaces everything.
- **`ceremony administrators propose`** requires `--alias-params` and will not
  run without it.

A third trap sits inside that one. If `payload_length` reads back as **0**, both
tools **refuse**. Proto3 cannot tell a zero from a field nobody filled in, so a
zero means the value is *unknown* — and `Validate()` refuses a zero, so the chain
never actually holds one. A tool that defaulted it to 8 would compose a proposal
that re-parameterised the chain while reading as an appointment.

> `blockchaind query alias params -o json` on the live devnet returns
> `{"params":{"payload_length":8}}` — the empty administrator list is **omitted
> entirely**, because protobuf JSON drops an empty repeated field. That is fine:
> for a repeated field, absent and empty are the same value, and there is no
> third state to confuse them with. `payload_length` is the field where a zero is
> ambiguous, which is why only that one is a refusal.

## What the chain does not check

`Params.Validate()` refuses an empty string, a duplicate, and a ninth entry. It
does **not** check that an entry is an address.

A mistyped address therefore passes a governance vote, occupies one of the eight
capped places, and grants the exemption to nobody. The list reads as five names
and grants four, which is exactly the auditability the cap exists to protect.
Both interfaces verify the bech32 checksum themselves before composing, because
the chain will not. Six characters of checksum catch every single-character typo
and every transposition, which is what somebody copying an address off a record
actually does wrong.

The chain also does not require an administrator to be a **group account**. It
matches by exact address equality and does not care what kind of account it is —
unlike `MsgGrantRole`, which refuses a holder that is not an M-of-N group. So the
interfaces **warn** and proceed, and say plainly that the chain will accept a
single key. Appoint a group anyway. An office that is one key is one bribe, and
this particular office can move any customer on the chain.

## Before the day

### Who the custodians are

Three at minimum, eight at most in the list overall, and **not people who report
to each other**. The whole value of M-of-N is that some of them would have to
agree against the interests of the others, and colleagues with a shared manager
cannot do that. If separate organisations are not available, separate line
managers and separate offices is the floor.

### The threshold

Never `1` — that is the single key this ceremony exists to abolish, and the tool
refuses it. Never equal to the membership either: a 4-of-4 means losing one key
freezes the account forever, and the tool refuses that too. 3-of-5 or 3-of-4 are
the shapes that work.

### The cap

Eight administrators, chain-wide, from `MaxFoundationAdministrators`. It is not
about storage. It is there so that widening the one rule the whole perimeter
rests on cannot happen by accident: a proposal that appends a hundred addresses
fails outright rather than passing because nobody scrolled.

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
blockchaind query alias params -o json > alias-params.json
blockchaind query auth module-account gov -o json > gov-account.json
blockchaind query gov params deposit -o json      # for --deposit

ceremony administrators propose --dossier appointment-*.json \
  --alias-params alias-params.json --gov-account gov-account.json \
  --deposit 1000000uyml
```

Both files are required and neither has a default. `--alias-params` because the
message replaces every parameter, and `--gov-account` because the authority is
read off the chain rather than compiled in — one that had gone stale would
produce a proposal that passed its vote and was then refused when it executed.

The tool prints the whole object, before and after, and every address it is
re-submitting. **Read that list.** Every address in it is one this proposal
carries; any that is missing is one it removes, silently, with a valid signature.
If the count is lower than you believe it should be, the parameters were read
before somebody else's proposal landed — re-read them and compose it again.

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
blockchaind query alias params -o json > alias-params.json
ceremony administrators verify --dossier appointment-*.json --alias-params alias-params.json
```

**A proposal that PASSED and a proposal that took effect are two different
states.** A proposal can pass its vote and still fail when it executes, which
leaves the parameters exactly as they were and reports it in a transaction log
nobody is watching. `PROPOSAL_STATUS_PASSED` is not evidence.

`verify` records the **whole** administrator list, not just this group. That is
deliberate: the list is what a carelessly composed `MsgUpdateParams` destroys,
and if it is shorter than it was before this proposal, the evidence is here and
nowhere else.

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

The same message, with the address taken out instead of added — and it is the
same trap, so use the governance console or compose it against a fresh read of
the parameters.

Removing the **last** administrator leaves the list empty, which is a real state
and the documented default. Nothing blocks it. What it means: no recorded country
can be corrected until governance appoints somebody again, no account can hold a
`ZZ` identifier, and a new country cannot be enrolled. Governance can appoint
again by another proposal exactly like this one, so it is recoverable — it is just
not reversible by the people who did it.

## What a run of this actually looked like

A rehearsal on `yamale-devnet-2`, 2026-08-23, so the numbers above are checkable
rather than illustrative. The list was empty before it and nobody on the chain
could correct a recorded country.

| Step | What happened |
| --- | --- |
| Ceremony | 3-of-4, params fingerprint `E1TH-KP2X-GWM6-X594`, group fingerprint `RVA4-6W1S-RX0V-GNDK` |
| Predicted address | `yml1afk9zr2…3xm8uj` — **the foundation's own**, sequence 1 |
| Real address | `yml1dlszg2s…rmuayr`, group 2, created at height 28480 |
| Proposal | gov #2, deposit 1 YML, 30-minute vote, passed 65,000,000,000 yes / 0 no |
| After | `foundation_administrators` had one entry; `payload_length` still 8 |

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
# who holds the exemption, and how many places are left
blockchaind query alias params -o json

# each one: is it a group, and who is in it?
blockchaind query group group-policy-info <address> -o json
blockchaind query group group-members <group id> -o json

# a plain-account answer here means it is a single key
```

An address in that list that `group-policy-info` does not recognise is a single
key holding the power to move any customer on the chain. That is not
misconfiguration the chain will report; it is a thing you have to go and look at.

## When something goes wrong

**The proposal passed and the parameters did not change.** It failed at
execution. `blockchaind query gov proposal <id> -o json` and read the
`failed_reason`. The usual causes are an authority that is not the gov module
account, and a `Params` object the chain refuses.

**The list is shorter than it was.** Somebody composed a proposal from a stale
read of the parameters. The administrators that vanished are in the chain's
history — find the proposal, take the list from *before* it, and propose that plus
whatever legitimate change has happened since. There is no undo.

**A group in the list cannot act.** Check `group-members` against the ceremony
record. A membership that does not match means the group at that address is not
the group the record describes, and the appointment went to whoever created that
policy.

**An entry in the list is not an address.** It grants nothing and occupies one of
the eight places. Remove it with a proposal like any other. The chain accepted it
because `Validate()` never checks — see [what the chain does not
check](#what-the-chain-does-not-check).
