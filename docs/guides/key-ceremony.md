# The key ceremony

How the account that holds every seized asset on this chain comes into
existence, in a room, on paper, with five people watching each other.

**You need:** two laptops that will never touch a network again in their current
form, printed sheets, pens, tamper-evident envelopes, and eight people with half
a day.

**You will end with:** a 3-of-5 group policy address to put in genesis, five
sealed envelopes leaving in five different directions, and a signed record.

---

## Why this exists

The `foundation` account on the devnet — `x/enforcement`'s
`recovery_destination`, the address every seized asset is sent to — was a single
secp256k1 key in a `keyring-test` backend on a cloud VM. Unencrypted. Created by
a setup script, printed once to a terminal nobody was reading, and written down
nowhere.

Anyone who could read that disk owned every asset the chain had ever recovered,
and losing the VM would have lost them permanently. Nothing about it was
malicious or even careless in the moment; it is simply what happens when the
account that matters most is created by the same line of shell as the account
that funds a test.

This runbook is the version of that which cannot happen.

## What is being built

Not a key. A **3-of-5 `x/group` policy account**: five named custodians, each
holding their own key, any three of whom can act together.

That is different from a key with five backup copies in three ways that matter:

- **The authority is distributed, not the backup.** No single person ever holds
  a key that can move anything, so there is no moment at which somebody could.
- **Every signature is attributable.** The chain records which three custodians
  agreed. A key with five copies records only that "the foundation" acted.
- **A custodian can be lost.** Two can, in fact. Nothing has to be migrated,
  re-issued or announced.

The chain enforces the shape as well as the address.
[`x/constitution`](constitution.md) holds three values about this account —
where seizures go, that there are exactly five custodians, and that three must
sign — and none of them can be changed by an ordinary vote.

## What this is not

**It is not an HSM ceremony.** [Scope §6](../scope/revised-scope.md) calls for
hardware custody, and this is the version that works today with nothing to
procure. Everything below is arranged so a hardware or HSM path substitutes for
one step — "the custodian's key is generated and stays on device X" — without
changing the roles, the sequence, the verification, the record or the group. If
you buy hardware later, you replace step 5 and keep the rest.

**It is not a way of hiding the address.** The group policy address is public,
printed in the record, and written into a genesis file everybody has. It has to
be: it is where seizures go, and an address nobody can check is an address
nobody can hold you to.

## Two ways to run it, and which is stronger

There are two, and they are not equivalent. Everything from **Roles** onwards
describes the air-gapped one, which is the ceremony. The hosted one is
[at the end](#the-hosted-ceremony) and it exists for a reason that is about
logistics rather than security.

|                          | Air-gapped (`ceremony serve`, or the terminal) | Hosted (`ceremony host`)                          |
| ------------------------ | ---------------------------------------------- | ------------------------------------------------- |
| Where the key is made    | On a wiped machine with no network             | In the custodian's own browser                    |
| Who could see a phrase   | Nobody outside the room                        | Nobody — but you are trusting the served code      |
| What you must trust      | The binary, whose hash the observer checks     | The binary **and** the bundle it serves            |
| Memory after the fact    | Machine powered off and wiped                  | A browser tab, on a device you do not control      |
| Who has to be present    | Everybody, in one room                         | Nobody, anywhere                                  |

**The air-gapped path is stronger, and the difference is not a formality.** It
is the only one where the code generating the key cannot have been substituted
between the review and the ceremony: the binary is carried across on removable
media and the observer hashes it on the machine it will run on. In the hosted
path the custodian receives the code over the network at the moment they use it,
and a coordinator who served a different page could take every phrase. The page
shows the digest of the bundle it served and the browser's own measurement of
the script it loaded, so an alteration that did not also alter the manifest is
caught — but a coordinator who changed both would not be, and no arrangement of
a web page can fix that.

**Use the hosted path for a rehearsal**, and for the case where the alternative
is not "a room" but "no ceremony at all". Use the air-gapped path for the keys
that will hold value.

What the hosted path does **not** concede is the thing that would make it
pointless. The seed is generated in the custodian's browser and never
transmitted; the coordinator relays public keys and signatures. If the
coordinator generated the phrases and sent them down the links, one machine would
again have held every key to the account that receives every seized asset —
which is the single key this whole document exists to abolish, with extra steps.

---

## Roles

Eight people. The roles are not ceremonial — each exists because a ceremony run
without it produces exactly the same files as one run properly, and nothing
afterwards can tell the difference.

| Role | Who | What they are responsible for |
| --- | --- | --- |
| **Lead** | one person, named in advance | Runs the sequence. Does not hold a key. Does not touch a keyboard while a phrase is on screen. |
| **Custodians** | five, from five different organisations or at minimum five different reporting lines | Each generates one key, transcribes it, verifies it, seals it, and leaves with it. |
| **Scribe** | one person, not a custodian | Writes the record as it happens — addresses, fingerprints, times, and anything that goes wrong. |
| **Observer** | one person from outside the organisation running the chain | Watches. Checks the binary hash independently. Signs that what the record says is what they saw. |

**Five custodians who all work for the same person is not a 3-of-5.** The
arrangement's whole value is that three of them would have to agree against the
interests of the other two, and colleagues with a shared manager cannot do that.
If five organisations are not available, five people with different line
managers and different offices is the floor.

**The observer's independence is the thing being bought.** An observer from the
same team as the lead is a witness. Record their organisation in the record so a
reader can see which one they were.

---

## Before the day

### The machines

Two laptops. The second one is for the restore drill and is not optional.

1. **Wipe and reinstall** from known media. Not "a clean user account" — a fresh
   install.
2. **Disable swap and hibernation.** This is the one item most often skipped and
   it defeats everything the tool does about memory: a phrase this program
   carefully zeroes is a phrase the kernel may already have written to disk.
   On Linux, `swapoff -a` and remove the swap entry from `/etc/fstab`. Boot from
   a read-only or RAM-backed image if you can.
3. **Disable the radios in firmware**, not in software. A Wi-Fi card switched
   off in the operating system is a Wi-Fi card one command away from being on.
4. **Bare metal.** Not a virtual machine: a hypervisor can write guest memory to
   the host's disk, and the guest has no way to know.
5. **No dock, no cable, no phone plugged in**, and none for the duration.

Both machines are destroyed or wiped afterwards. Plan for that before you use
them for anything else.

### The tool

Build `ceremony` on a normal machine and carry the binary across on removable
media:

```bash
go build -o ceremony ./tools/ceremony
sha256sum ceremony
```

**The observer computes that hash independently** — on their own machine, from
their own checkout, with their own `sha256sum` — and again on the ceremony
machine before anything is generated. The tool prints its own hash when it
starts, and that number proves nothing on its own: a substituted binary would
print whatever hash it liked. It is there so the observer has something to
compare against, and the comparison is the check.

Carry `blockchaind` too. It is not needed to generate keys, and it is what the
restore drill's independent check uses.

### The paper

Per custodian: a numbered sheet with twenty-four ruled lines, a line for the
fingerprint, a line for their name and the date, and a tamper-evident envelope.

Print them. Handwriting twenty-four lines onto blank paper is how words end up
in the wrong order.

---

## The sequence

### 1. Open

The lead states the date, the chain id, who is present and in what role. The
scribe writes it down. Phones go in a box outside the room, including the
lead's.

Read this out:

> Nothing that appears on that screen may be photographed, copied to another
> medium, read aloud, or written anywhere except on the sheet belonging to the
> custodian it belongs to. If any of that happens, we destroy the key and
> generate a new one. Nobody has to argue for that and nobody has to feel
> awkward about calling it.

### 2. Prepare the machine

Boot it. Confirm it is offline as far as anybody can. Show the observer the
firmware settings for the radios.

```bash
./ceremony preflight
```

The tool prints its own hash, reports what it found about the network, and then
prints what it could **not** check. Read that second list aloud. It is there so
nobody hears "no network detected" as a guarantee — the check cannot see a radio
that is present but unassociated, a cable plugged in one second later, whether
this machine has ever been online, or whether anything in the room is recording
the screen.

Then it asks ten questions and you answer `yes` to each, out loud, with the
observer listening. Any answer that is not `yes` stops the ceremony. There is no
`--force`; if the machine has a network, proceeding needs
`--network-acknowledged "<reason>"` and the reason goes into the record where
somebody will read it later.

### 3. Generate the first custodian's key

Everyone except that custodian, the lead and the observer turns away from the
screen or leaves. The custodian sits down with their sheet.

```bash
./ceremony custodian --name "Amara Okafor" --ceremony "yamale-testnet-1 foundation"
```

The pre-flight runs again — this is deliberate, not a leftover — and then
twenty-four numbered words appear.

The custodian writes them down. In order. Numbered. There is no hurry and the
room waits.

### 4. Verify the transcription — with the screen cleared

They press return. **The screen clears, scrollback included.** The tool then
asks for five words at random positions, and one of them is always the last.

The clearing is what makes this a check rather than a formality. With the phrase
still visible the custodian reads it off the monitor, the sheet in their hand is
never consulted, and the tool has confirmed only that the screen agrees with
itself.

A wrong answer names every position that disagreed, redisplays the phrase, and
tries again. Three failed passes and the tool stops and tells you to destroy the
sheet and start over. **Do that.** A sheet that cannot be read back three times
has something wrong with it, and a fresh key costs five minutes.

**Why five words and not twenty-four.** Sampling five of twenty-four catches a
single transcription error about one time in five, which sounds poor until you
notice what the check is really for: it makes the custodian read their own sheet
back, in order, under someone else's eye, before the room moves on. Asking for
all twenty-four would be better and nobody would do it a second time.

### 5. Record the public half

The tool prints the address, the public key, and a **fingerprint** — eight
characters in two groups, like `08DZ-S1VA`.

The custodian writes the fingerprint on their own sheet, next to the phrase, and
reads it aloud. The scribe writes it on the record.

That is what makes a swapped or mis-filed envelope detectable. Five years from
now an envelope labelled "custodian 3" either recovers to a key whose
fingerprint matches the record or it does not, and that check needs no network,
no node, and nobody's word. The alphabet has no `I`, `L`, `O` or `U` in it, so
there is nothing to misread when it is read across a room.

Then: seal the sheet in the envelope, sign across the seal, and the custodian
keeps it.

**Do not pass `--armor` for a custodian.** It writes an encrypted copy of the
key to a file, which is a second copy of something that should have exactly one,
on a medium that leaves the room. It exists for validator operator keys, which
have to get onto a node somehow.

### 6. Repeat, four more times

One custodian per invocation. The tool refuses to overwrite an existing public
record, which is the check against two people being entered under the same name.

### 7. The restore drill

**Do this during the ceremony, not afterwards.** It is the only step that tests
the thing the whole ceremony produces — the paper — and a ceremony that skips it
finds out the paper was wrong on the day it matters.

Pick one custodian. Any one; let the observer choose.

On **the second machine**, which has never seen the phrase:

```bash
./ceremony restore --expect yml1et6twxgwvfx7pkh9kt57sttz0qtjujmtx402rn
```

The custodian opens their envelope and types the phrase from the sheet. Not from
memory — from the sheet, because the sheet is what is being tested.

The tool derives the address and compares.

- **It matches.** The paper works. Say so, write it in the record, then destroy
  this instance: wipe the second machine before it leaves the room. The phrase
  you just typed is on it until you do.
- **It does not match.** The sheet is wrong. Destroy the key, destroy the sheet,
  and generate a new one from step 3. **Do not** try to work out which word is
  wrong and correct it: a sheet edited after the fact is a sheet nobody can
  trust, and the key it protects is a key nobody has proven.

The custodian re-seals their sheet in a **fresh** envelope. The old one has been
opened and its seal means nothing now.

Independent check, if you want one — and you should:

```bash
blockchaind keys add drill --recover --keyring-backend test --home /tmp/drill
blockchaind keys show drill -a --keyring-backend test --home /tmp/drill
rm -rf /tmp/drill
```

Same phrase, same address, a completely different implementation. If
`ceremony` and `blockchaind` disagree, stop the ceremony: one of them is wrong
and you do not yet know which.

### 8. Assemble the group

```bash
./ceremony group --threshold 3 custodian-*.json
```

This prints the **group policy address** and writes:

| File | What it is for |
| --- | --- |
| `group-genesis.json` | Spliced into `app_state.group`. This is the one a launch uses. |
| `constitution-invariants.json` | The three values `app_state.constitution.invariants` needs, so they cannot disagree with the group. |
| `group-members.json`, `group-policy.json` | For `blockchaind tx group create-group-with-policy`, on a chain that is already running. |
| `group-create-msg.json` | The same message as JSON, for review. |

Read the policy address aloud. The scribe writes it on the record. Anyone can
check it independently, without the custodian files:

```bash
./ceremony address --seq 1
```

**Why the group goes into genesis rather than being created afterwards.** The
address `x/group` gives a policy account is derived from the policy sequence
number alone — not from the members, not from the threshold, not from the admin,
not from the chain id. So it is perfectly knowable before genesis, and it
commits to nothing whatsoever about who controls it. A genesis that named the
address and left the group to be created by a transaction after launch would be
pointing every future seizure at whichever group policy somebody created first.
Putting the group in the same file closes that window rather than shortening it.

### 9. The record

The scribe fills in a config file and renders it:

```json
{
  "ceremony": "yamale-testnet-1 foundation",
  "chain_id": "yamale-testnet-1",
  "location": "Geneva",
  "started_at": "2026-09-01T09:00:00Z",
  "completed_at": "2026-09-01T14:30:00Z",
  "threshold": 3,
  "policy_address": "yml1afk9...",
  "binary_hash": "…",
  "custodian_files": ["custodian-amara-okafor.json", "…"],
  "participants": [
    {"name": "R. Lead", "role": "ceremony lead", "organisation": "Yamale Foundation"},
    {"name": "S. Scribe", "role": "scribe", "organisation": "Yamale Foundation"},
    {"name": "O. Observer", "role": "independent observer", "organisation": "External Auditors LLP"}
  ],
  "notes": []
}
```

```bash
./ceremony record --config record.json
```

The addresses and fingerprints are read from the custodian files rather than
typed, so the scribe cannot mistype one into the document that exists to catch
mistyped addresses.

**Print it. Everybody signs the paper copy before leaving** — all five
custodians, the lead, the scribe and the observer. A record everybody signs in a
shared document afterwards is a record signed by whoever had the link.

`notes` is where anything unusual goes: an interruption, a key destroyed and
regenerated, a phrase exposed. An empty list is itself a claim — that nothing
happened — and signing the record is signing that claim.

### 10. Close the machines

Wipe both, in the room, with everybody watching. Not "later". The phrases are
recoverable from a disk that leaves the building with a filesystem on it.

If the machines are being kept as evidence rather than wiped, they go into a
sealed container and the record says who holds it.

---

## When a phrase is exposed

Somebody photographs the screen. A stranger opens the door. A custodian reads a
word out loud to check it. A laptop's screen is visible from the corridor and
somebody notices halfway through.

**Destroy the key and generate a new one. Always. Every time.**

There is no version of this where the answer is "it was only for a second" or
"it was only one word" or "that was a colleague". Not because those are
necessarily dangerous, but because a rule with an exception is a rule somebody
has to argue for in a room where everybody wants to go home — and the person who
would have to make that argument is usually the one who made the mistake.

It is written here in plain words so that nobody has to make that call under
social pressure. The rule already made it. Say "we're regenerating", start again
from step 3, and put both the exposure and the regeneration in the record.

Regenerating one custodian's key costs about ten minutes. Nothing else has to
change: the other four keys are untouched, and the group is assembled at the end
from whatever the five current public records say.

**If the exposure is noticed after the group has been assembled and genesis
built**, it is not a ceremony problem any more — it is a custodian replacement,
below.

---

## Validators are different, and the difference matters

A validator has two keys, and they are handled in opposite ways.

| | Operator key | Consensus key |
| --- | --- | --- |
| **What it does** | Moves the stake, changes commission, signs governance | Signs blocks, thousands of times a day |
| **Where it is made** | Here, on the air-gapped machine | On the node, by `blockchaind init` |
| **Where it lives** | Off the node entirely | `config/priv_validator_key.json`, and nowhere else, ever |
| **Backups** | On paper, like a custodian's | **None** |

```bash
./ceremony validator --name "Banque Nationale" --armor operator.asc
```

`--armor` is appropriate here: the key has to reach a node somehow, and retyping
twenty-four words into a production server is worse than an encrypted file.
Import it with `blockchaind keys import <name> operator.asc` and delete the file
afterwards.

**The consensus key is never generated here.** `ceremony consensus` exists only
to refuse and explain why:

```
blockchaind init <moniker> --chain-id <chain> --default-denom uyml
```

Two copies of a consensus key signing at once is double-signing, which the chain
slashes and jails automatically, with no appeal and no way to distinguish malice
from a restored backup. A copy taken "for safety" is the usual cause. Carry the
**public** half — `blockchaind comet show-validator` — to wherever the
`create-validator` transaction is built, and leave the private half where it was
made for the whole life of the validator.

---

## Replacing a custodian

Custodians leave. Somebody retires, changes employer, or is no longer somebody
the other four would want holding a share of this.

**A departure and a replacement are one decision, in one message.** Never a
removal now and an appointment later.

The reason is that the drift is quiet and it ratchets:

| Custodians | Rule | Share needed |
| --- | --- | --- |
| 5 | 3 of 5 | 60% |
| 4 | 3 of 4 | 75% |
| 3 | 3 of 3 | unanimity — every custodian holds a veto |

Dropping to four is not "one short". It concentrates authority in whoever
remains, above what the ceremony gave them. Drop to three and one custodian who
cannot be reached freezes the account the chain is still sending seized property
into — permanently. Nobody ever votes for that outcome; it is arrived at by two
individually reasonable decisions taken months apart.

So:

1. **The incoming custodian generates their key through this ceremony**, from
   step 2. Same machine preparation, same transcription check, same restore
   drill, same envelope. They are not handed a key somebody else made, and they
   do not use a key they already had.
2. **Any current custodian proposes the swap.** Build it with the tool, which
   has no way to express only half of it:

   ```bash
   ./ceremony replace-custodian \
     --outgoing custodian-bernard-kouassi.json \
     --incoming custodian-fatou-diallo.json \
     custodian-*.json
   ```

   Fill in the proposer's address and submit:

   ```bash
   blockchaind tx group submit-proposal replace-custodian-proposal.json \
     --chain-id yamale-testnet-1 --keyring-backend file
   ```
3. **Three custodians vote yes**, and one of them executes it. The same
   three-of-five that moves money moves membership.
4. **The outgoing custodian's envelope is destroyed**, witnessed, and the
   destruction is recorded. Their key no longer controls anything, but an
   envelope in a drawer that nobody can account for is a question somebody will
   eventually have to answer.
5. **The record is amended** with the swap, both fingerprints, and the date. The
   tool prints the line to use.

**The chain enforces this; it is not a convention.** `x/constitution`'s ante
gate refuses any `MsgUpdateGroupMembers` that would leave the foundation group
at a size other than `foundation_custodian_count` — whether it arrives directly,
inside a group proposal, inside an `x/authz` `MsgExec`, inside a governance
proposal, or as the execution of a proposal submitted earlier. It also refuses
`MsgLeaveGroup` for the foundation group outright: leaving changes how much
authority everybody else holds, so it is not a decision one custodian takes
alone. And it refuses moving the group's admin, which would put a single key
back in charge of the membership.

Changing the numbers themselves — three of five to something else — is a
**constitutional amendment**: a governance proposal, three weeks in public, and
a four-fifths ratification by the validator set. See
[what governance can and cannot change](constitution.md). That is deliberately
much harder than a group vote, because the custodians should not be able to
rewrite the rule that constrains them.

---

## The hosted ceremony

Five custodians on five devices, in five places, each reaching the ceremony by a
link. Read [Two ways to run it](#two-ways-to-run-it-and-which-is-stronger) first:
this is the weaker of the two and it is the right choice for a rehearsal.

### Start it

```bash
go build -o ceremony ./tools/ceremony
./ceremony host --public-url https://pay.yamalelegal.com/ceremony/
```

That is the only command anybody types. Everything else is a page.

It binds `127.0.0.1:8787` and refuses anything else — there is no flag that
allows a wildcard bind and no fallback that would do it quietly, because a
signing service in this project once bound `0.0.0.0` while its credential gate
sat on another host and accepted unauthenticated requests for days. Put nginx in
front of it, on the certificate that already exists:

```nginx
location /ceremony/ {
    proxy_pass http://127.0.0.1:8787/ceremony/;
    proxy_set_header Host $host;
}
```

No new DNS and no new certificate. `--public-url` must be `https` for anything
but loopback: the browser generates the key from the code that URL serves, so
anything able to rewrite the response owns every key in the ceremony.

On startup it prints two things. The **bundle SHA-256**, which is the digest a
custodian sees in the page and can compare against the value published below.
And the **coordinator link**, which is the ceremony: anyone holding it can issue
invitations, so it is not a link to paste into a group chat.

### What the coordinator does

Four screens, in the order the hour happens in.

1. **Set it up.** The roster of names, the threshold, the chain id, the voting
   window. Not flags. These are covered by the parameters fingerprint every
   custodian sees before they generate anything, and they cannot change once
   somebody has submitted.
2. **Hand out the links.** One per custodian, each with a copy button and a **QR
   code** — custodians will be on phones, and nobody types a URL with a
   256-bit token in it correctly.
3. **Watch.** A board naming who has reached which step: invited, opened,
   generated, submitted, attested. The first three are the page's own word for
   it and the board says so; the last two are backed by a signature this process
   verified. It names who you are waiting for rather than showing a spinner,
   because a relay withholding one submission is only visible if the interface
   says which one is missing.
4. **Read the group out**, and render the record. The fingerprint, the policy
   address and the genesis fragment, each with a copy action.

### What a custodian does

One continuous flow, with no step where they leave the page: open the link →
what this is → generate → the twenty-four words, once → confirm written down,
words removed from the page → read four back including the last → their address
and fingerprint → wait, with live progress naming who is outstanding → the group
fingerprint their own device computed → check their own address is in it → sign.

Three things about it are worth knowing before you run one.

**The words appear once and cannot be reissued.** Nothing on the coordinator's
side ever held them, so there is nothing to show again. A custodian who closes
the tab before the words appear has lost nothing. One who closes it after has
lost that key: the coordinator presses **Reissue this link**, which revokes the
old link and says on screen what it costs, the custodian destroys the sheet they
started, and they generate again. If they still have the words on paper they do
not need any of that — the page asks for them and carries on, which is a second
check on the sheet as it now stands.

**A link speaks for one custodian.** The name on a submission comes from the
token, never from the request, so a link that leaked cannot put a stranger's
address into the group under a custodian's name. Once a key is recorded for a
name, a different key for that name is refused outright.

**The page will not let a custodian attest to a group their own key is not in.**
That check exists because the room used to do it: five people watching one screen
saw the same five fingerprints. Alone on a phone they cannot, so it is a refusal
rather than a warning.

### What the coordinator cannot do

It cannot see a phrase, and this is arranged rather than intended:

- Every request body is decoded with `DisallowUnknownFields`, so a body carrying
  a `phrase` field is a 400 rather than a field the server ignores. A modified
  page could not post one here even deliberately.
- No request type in the server has a field that could hold seed material, and a
  test drives **every** route with a phrase in the body to prove it, iterating
  the route table so a route added later is covered without anybody remembering.
- The page's `Content-Security-Policy` pins `connect-src 'self'`, so even an
  altered page could not POST anywhere else, and `script-src 'self'` with no
  `'unsafe-inline'`.
- The page stores nothing — no `localStorage`, `sessionStorage`, IndexedDB or
  cookies, not even a theme — asserted against the built bundle rather than
  assumed, so a dependency that reached for storage would fail the build's tests.
  Custodians are told to open the link in a private window; a web page cannot
  force that, so the response is to leave nothing behind worth hiding.

### The bundle digest

The page a custodian runs is committed at `tools/ceremony/hosted/`, unminified so
it can be read, and embedded in the binary so one file is everything a
coordinator needs. Rebuild it with:

```bash
cd clients && npm install && npm run build --workspace @yamale/ceremony
```

`ceremony host` prints the digest of what it will serve, and the page shows both
that value and the browser's own measurement of the script it loaded. Comparing
the printed digest against a published one is what proves the code generating the
keys is the code that was reviewed. Publish it with the ceremony announcement —
the digest of the reviewed build belongs somewhere the coordinator does not
control, which is the only place it means anything.

### What the two paths share

Everything that matters is the same code. The addresses, the possession
signatures, the group, the genesis fragment and the fingerprint five people read
aloud are produced by `tools/ceremony` on one side and `clients/ceremony` on the
other, and the two are held to byte-for-byte agreement by
`testdata/vectors/ceremony.json`, which both test suites read. Editing that file
turns both red. A browser that derived even slightly differently would hand five
custodians addresses that look right and control nothing, and nobody would find
out until a seizure arrived at an account no three people could open.

---

## Into genesis

The ceremony happens **before** the genesis is built, not after.
`x/constitution`'s `InitGenesis` refuses to start a chain whose recovery
destination is unset, so a genesis built without a foundation is a genesis that
fails on every validator at height one.

```bash
CEREMONY_DIR=/path/to/ceremony ./02-build-canonical-genesis.sh accounts.txt
```

Step 2 of the [deployment runbook](../../scripts/testnet/README.md) reads
`group-genesis.json` and `constitution-invariants.json` from that directory,
splices the group into `app_state.group`, sets the recovery destination in both
the places that must agree, and then re-reads the file it is about to distribute
to check that all of it took. It refuses to write `canonical-genesis.json`
otherwise — because every one of these values is individually legal and
collectively wrong in ways `genesis validate` cannot see. A constitution saying
three-of-five over a group that is a two-of-four starts a chain perfectly
happily.

Verify it on the running chain:

```bash
blockchaind query constitution invariants
blockchaind query enforcement params
blockchaind query group group-policies-by-group 1
```

The address in all three has to be the one on the signed record.

---

**Full reference:** [x/constitution](../reference/constitution.md) and
[x/enforcement](../reference/enforcement.md) — every message, query, parameter
and error code, generated from the source.
