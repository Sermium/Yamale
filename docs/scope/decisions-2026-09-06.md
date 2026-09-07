# Decisions taken 2026-09-06, and what each one needs

Four decisions, recorded so that they are choices on the record rather than
things that drifted. Every command below is written out and **none of them has
been run** — each either signs with a key or moves stake, and both are yours.

The chain facts quoted here were read from the live chain on 2026-09-06 at
height 218,283. Re-read them before acting; the point of writing them down is
that they can be checked, not that they can be trusted.

---

## The decisions

| | Decision |
|---|---|
| **What this chain is** | A demonstrator. devnet-2 will be relaunched from a fresh genesis rather than carried forward. |
| **The 976,733,334 YML premine** | Move it to the foundation 3-of-5. Not burned. |
| **The operator passphrase in `~/.bash_history`** | Rotate it and shred the history. |
| **The 9.2-day concentration clock** | Start today. |

The second and the first sit together deliberately. Burning is the tidier answer
for a chain being thrown away, but moving it into the 3-of-5 *exercises* the
foundation group — a demonstrator that has never once used its own custody
arrangement has not demonstrated it. It is also the reversible choice: the group
can burn it later, and nothing can un-burn it.

Because this is a demonstrator, the work that only matters for a chain carrying
real value is now explicitly deferred rather than pending: the air-gapped key
ceremony, and the custody hardening in
[`docs/guides/key-ceremony.md`](../guides/key-ceremony.md). What is *not* deferred is anything a
visitor can reach, because the demonstrator is reachable by the public.

---

## 1. Start the concentration clock

**Why you run this and not me.** Because you asked to drive governance
yourself, which is the whole of the reason. An earlier draft of this section
argued it was a security matter — that submitting from
`yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg`, the `alice` key in an unencrypted
keyring that has signed all 20 votes ever cast, would deepen what the audit
flagged. That was overstated, and it is worth correcting rather than deleting:
**the proposer of a governance proposal holds no power.** It posts a deposit
that comes back and grants nothing.

Where `alice` does matter is the **vote**, which is weighted by staked tokens,
and where it holds 37.16% against a 33.4% quorum. So use whichever funded key is
convenient to submit, and read §3 for the key that actually needs attention.

The proposal is written and checked in:
[`proposals/12-concentration-ceilings.json`](proposals/12-concentration-ceilings.json).
It carries all thirteen invariant fields and changes four.

Run it on the VM. The repository is not checked out on that host, so the
proposal has to be copied across first; the path below is where it now sits.

```bash
blockchaind tx gov submit-proposal /home/ubuntu/proposal-12.json --from foundation --keyring-backend test --home /opt/yamale/node --chain-id yamale-devnet-2 --node http://127.0.0.1:26657 --gas auto --gas-adjustment 1.4
```

**`--node` is the local node, not the public URL, and that is not a shortcut.**
The funnel hostname is unreachable from either host. MagicDNS resolves
`yamale.tail4355e8.ts.net` to Tailscale's IPv6 ingress addresses
(`2a00:dd80:3e::…`) for anything on the tailnet, and neither the Pi nor the VM
can route to them — so a command copied from a document and run on the server
fails with `post failed: EOF`, which reads as the chain being down. It is not:
the same URL answers from any machine off the tailnet.

Three endpoints, and which one to use depends on where you are standing:

| From | Use | Goes through the RPC gate |
|---|---|---|
| either host | `http://127.0.0.1:26657` | no — it *is* the node |
| either host | `http://100.68.207.17:8093/api/rpc` | yes |
| anywhere else | `https://yamale.tail4355e8.ts.net/api/rpc` | yes |

The trailing slash no longer matters on any of them. It used to: the no-slash
form answered `301` pointing at `http://…:8093/api/rpc/`, and the Cosmos RPC
client does not follow a redirect on POST, so every command written the way this
document writes them failed with `EOF`. `yamale-rpc.conf` now rewrites it
internally.

**The two keyring flags are not optional.** `--keyring-backend` defaults to `os`
and `--home` to `~/.blockchain`, which on the VM is empty. Every key on that
host is in `/opt/yamale/node`: the `test` backend holds most of them and `file`
holds only `pi-operator`. Omitting the flags fails with `key with address ... not
found`, which reads as "you do not have this key" when it means "I looked
somewhere else".

Any funded account may submit, and `foundation` holds 498,012 YML. The proposer
is **not** a privileged role: it posts a 1 YML deposit that is returned, and
confers no authority. The power is in the vote, which is weighted by staked
tokens.

Then, once it has an id (it will be 12), the vote is yours to cast:

```bash
blockchaind tx gov vote 12 yes --from <your-key> --chain-id yamale-devnet-2 --node https://yamale.tail4355e8.ts.net/api/rpc --gas auto --gas-adjustment 1.4
```

The voting period is 1,800s — 30 minutes. The amendment delay is 120,960 blocks
after that, which at the ~6.6s this chain is producing is **about 9.2 days**.

### Then ratify it, or the nine days are wasted

**Submitted and passed 2026-09-07.** Proposal 12 was submitted from `foundation`
at height 228,440 and voted through by `alice` — 65,000 YML, 37.16%, against a
33.4% quorum. A second vote from `yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas`
carried no power, that key having no stake.

Passing the vote only *opens* the amendment. `EffectiveAtHeight` is stamped when
it opens — `height + AmendmentDelayBlocks` — and at that height the end blocker
does one of two things:

```go
if amendment.RatifiedPower < current.RequiredPower(amendment.SnapshotPower) {
```

Enough ratified power and it is enacted. Not enough and it **lapses**. It does
not wait, and there is no extension. So the nine days are a window to ratify in,
not a delay to sit out.

**80% cannot be reached from one validator on this chain.** `pi` is 57.18% and
`pi-2` is 42.82%, so both must ratify. The signer has to be the validator's own
operator account — `bondedValidatorOf` reinterprets the signer's address bytes
as the operator address — and both of those keys are in passphrase-protected
`file` keyrings, on different hosts:

| Key | Host | Home | Power |
|---|---|---|---|
| `pi-operator` | VM | `/opt/yamale/node` | 57.18% |
| `pi2-operator` | Pi | `/opt/yamale/join-node` | 42.82% |

The command takes **two** positional arguments — `[validator] [amendment-id]` —
where `validator` is the operator's own account address, the same key that
signs. It is not inferred from `--from`.

```bash
# on the VM, once the amendment id is known
blockchaind tx constitution ratify-amendment yml1m9xhc6zy7fxfax9t5fnykh9k2e29faj7htmqms <id> --from pi-operator --keyring-backend file --home /opt/yamale/node --chain-id yamale-devnet-2 --node http://127.0.0.1:26657 --gas auto --gas-adjustment 1.4

# on the Pi — note the different home; the second validator is join-node
blockchaind tx constitution ratify-amendment <pi2-operator-account> <id> --from pi2-operator --keyring-backend file --home /opt/yamale/join-node --chain-id yamale-devnet-2 --node http://127.0.0.1:26657 --gas auto --gas-adjustment 1.4
```

Both account addresses, converted from the operator addresses the staking module
holds:

    pi    yml1m9xhc6zy7fxfax9t5fnykh9k2e29faj7htmqms
    pi-2  yml1cgguvt0hvdg2602flzan9shg0g56rujev5see4

**Do not derive one of these by swapping the prefix on a `ymlvaloper` address.**
Bech32 checksums are computed over the prefix, so the account form of
`ymlvaloper1m9xhc…p4h3kh` is `yml1m9xhc…htmqms` — the data part is identical and
the last six characters are not. An earlier draft of this file did exactly that
and produced an address that would have been rejected as malformed. `blockchaind
keys parse <address>` prints every form.

**There is no way to take a ratification back.** The command's own help says so:
the protection an amendment carries is the delay and the threshold, not the
ability to run the vote backwards. Read the amendment before signing it.

Watch it approach the threshold rather than assuming:

```bash
blockchaind query constitution amendments --node http://127.0.0.1:26657 -o json
```

`EventAmendmentRatified` carries the running total and the required figure on
each ratification, precisely so this can be watched instead of guessed.

### One thing retiring `alice` will break, unless it is done in the right order

`alice` is the only key on either host that holds **staked** tokens and sits in
a keyring with no passphrase. That combination is exactly what the audit
objected to, and it is also what made proposal 12 votable in a thirty-minute
window. `foundation` holds 498,012 YML and none of it bonded, so its vote counts
for nothing; `bob` likewise.

So retiring `alice` by itself does not reduce the risk, it removes the ability to
govern: quorum is 33.4% of bonded stake, and after `alice` the only remaining
voting power is the two operator keys, both behind passphrases, one of which is
the passphrase in §3.

The order that works is: give the intended successor real stake **first**,
confirm it can meet quorum on its own, and only then retire `alice`. Delegating
to a validator is enough — voting power is bonded stake, not balance.

### What it sets, and the one thing that is not obvious

    max_entity_power_bps            10000 -> 3300
    max_beneficial_owner_power_bps  10000 -> 3300
    max_jurisdiction_power_bps      10000 -> 5000
    min_active_validators               1 -> 2

All three ceilings have read 10,000 bps — 100% — since genesis.
`Invariants.Validate()` refuses zero and permits 10,000, so a settlement that
enforces nothing passes validation and reads as configured.

`min_active_validators` moves in the same amendment because of how the end
blocker corrects a breach. It demotes members of an over-ceiling group until the
group is inside its ceiling, and stops when the active set would fall to the
floor, recording an `EventConcentrationUncorrected` instead:

```go
if active <= caps.MinActive {
    break
}
```

This chain has two validators — `pi` at 57.18% and `pi-2` at 42.82% of 174,900
YML bonded — and both are yours. Declared honestly they are **one**
beneficial-owner group holding 100%, so any ceiling below 10,000 is breached the
instant it exists. At a floor of 1 the end blocker would jail one of the two and
leave the chain on a single validator with no fault tolerance: a ceiling doing
more damage than the concentration it was written to limit. At a floor of 2 it
demotes nobody and records the breach every epoch, in public.

That is the honest outcome for a two-validator chain under one owner. The
concentration is real, it becomes visible, and it is fixed by adding validators
under different ownership — not by removing the ones there are.

### It will not bind anybody until the founding set is declared

`activeSeatHolders` needs an `ApprovedValidator` record and the live query
returns an empty set: both validators came through the gentx ceremony and the
founding set was never declared. Until then they are counted in the total,
belong to no group, and cannot be demoted — so no breach is even reported.

I need three values from you before I can prepare that, and inventing them would
defeat the exercise:

- `legal-entity-id` for each of `pi` and `pi-2`
- `beneficial-owner-id` — **if it is the same person for both, say so.** The
  ceiling being violated is information; hiding it in the declaration is the one
  failure mode this system has no defence against.
- `jurisdiction` — the ISO country code each validator actually operates from.
  These are two different countries in fact: the Pi is on a mobile network in
  one place and the VM is in a cloud region in another.

---

## 2. Move the premine to the foundation

**The amounts, read today.**

| Holder | Total YML |
|---|---:|
| `yml1m9xhc…htmqms`, operator of `pi` | 887,940,502.67 |
| `yml1cgguvt0…v5see4`, operator of `pi-2` | 88,792,831.43 |
| | **976,733,334.10** |

That is 5,584× the entire bonded stake of the chain. Quorum is 33.4%, the
enforcement supermajority 66.67%, a constitutional amendment 80% — all measured
against 174,900 YML. Any fraction of this clears all three in one block, which
is why every protection in this document is provisional until it moves.

Emission has stopped — `current_provisions_per_block` reads `0` — so the figure
is fixed rather than growing. That is the only reason this is not an emergency.

The destination is the 3-of-5 group, which is the same address the constitution
already pins as `enforcement_recovery_destination`:

    yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj

Withdraw first, then send. Do it for one operator, confirm the balance arrived,
then do the other — there is no reason to have both in flight at once.

```bash
# 1. withdraw rewards and commission to the operator's own account
blockchaind tx distribution withdraw-rewards ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh --commission --from <pi-operator> --chain-id yamale-devnet-2 --node https://yamale.tail4355e8.ts.net/api/rpc --gas auto --gas-adjustment 1.4

# 2. check what actually landed before sending it anywhere
blockchaind query bank balances yml1m9xhc... --node https://yamale.tail4355e8.ts.net/api/rpc

# 3. send it on, leaving enough behind to pay for future transactions
blockchaind tx bank send yml1m9xhc... yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj <amount>uyml --from <pi-operator> --chain-id yamale-devnet-2 --node https://yamale.tail4355e8.ts.net/api/rpc --gas auto --gas-adjustment 1.4
```

**Leave a working balance behind** — though not for the reason it first appears.
`minimum-gas-prices` is `"0uyml"` on this chain, so transactions cost nothing and
an account with a zero balance can still sign: `pi-operator` holds nothing today
and can still withdraw its own rewards. The reason to keep some anyway is that a
zero gas price is a per-node setting rather than a chain rule, and any validator
may raise its own tomorrow.

**Check the destination accepts a send before you send 887 million to it.** A
group *policy* account is an ordinary account and will; a module account would
not, and a bank send to a blocked address strands the funds permanently. Prove
it with one token first:

```bash
blockchaind tx bank send yml1m9xhc... yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj 1uyml --from <pi-operator> --chain-id yamale-devnet-2 --node https://yamale.tail4355e8.ts.net/api/rpc
blockchaind query bank balances yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj --node https://yamale.tail4355e8.ts.net/api/rpc
```

---

## 3. Rotate the passphrase, shred the history

The passphrase for the majority validator's operator key was found in
`~/.bash_history` on the VM. It was reported and never used. It is still there.

Shell access to that host is currently equivalent to control of the chain, so
the order matters: **rotate first, shred second.** Shredding first leaves the
old passphrase valid and removes your record of what it was.

A keyring passphrase is not changed in place. It is exported under the old one
and imported under the new one:

```bash
# on the VM. Do this in a shell that is not recording:
unset HISTFILE

# 1. export under the old passphrase, to a file on tmpfs, not the home directory
blockchaind keys export <operator-key> --keyring-backend file --home /opt/yamale/node > /dev/shm/op.asc

# 2. import into a fresh keyring under a new passphrase
blockchaind keys import <operator-key> /dev/shm/op.asc --keyring-backend file --home /opt/yamale/node-newkeyring

# 3. confirm the address is identical before you destroy anything
blockchaind keys show <operator-key> --keyring-backend file --home /opt/yamale/node-newkeyring

# 4. then, and only then
shred -u /dev/shm/op.asc
```

Once the address matches and the new keyring is in place, move the old keyring
aside rather than deleting it, restart whatever unit uses it, confirm the
validator is still signing, and only then destroy the old copy.

Then the history itself:

```bash
unset HISTFILE          # so this shell does not rewrite the file on exit
shred -u ~/.bash_history
touch ~/.bash_history && chmod 600 ~/.bash_history
```

`unset HISTFILE` is the part people miss: without it the shell rewrites the file
from memory when it exits and the passphrase comes back.

**Assume it is compromised regardless.** Shredding removes it from anyone who
gains access from now on. It does nothing about anyone who has already read it,
and the file has been there for an unknown period on a host with a public
address.

---

## 4. Still open, not decided here

- **`max_gas` is `-1`** on the live chain, confirmed today. One consensus
  parameter change, and the cheapest single defence available. Not started
  because it is a second governance proposal and one clock at a time is easier
  to follow.
- **`abci_query` reaches modules closed on REST.** The RPC gate filters the
  method and not the query path inside it. `deploy/deploy.sh` reports it on
  every run so it changes deliberately rather than drifting.
- **The `release` workflow** fails at "Delete the latest Release", which looks
  like a token scope.
- **Treasury 2's admin** is still the `pi-2` oracle feeder key. Needs an address
  to move it to — ideally an x/group account, which is what x/alias already
  requires of every role holder.
