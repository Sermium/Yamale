# Upgrading devnet-2 to carry deal documents

The second consensus upgrade in two days, and the shorter one. It adds no store
and migrates no state; the halt exists for one reason only, and that reason is
worth stating before anybody asks why a chain has to stop for a feature that
writes nothing during the stop.

Companion to [upgrade-devnet-2-modules.md](upgrade-devnet-2-modules.md), which
carries the general shape. Only what differs is repeated here.

---

## 1. What changed, and why it is consensus-breaking anyway

`x/syndicate` gains `MsgAttachDocument`, `Query/Documents`, and three
collections under its **existing** store key.

| | |
|---|---|
| Store added | **none** — the records live under `syndicate` |
| Consensus version | **unchanged at 1** |
| State migrated | **none** — every existing deal is valid with no papers |

So nothing about the state tree changes at the halt height, and the upgrade
handler writes nothing. What makes this a coordinated stop is the **message**: a
validator running the old binary cannot decode `MsgAttachDocument`, so the first
transaction carrying one would be accepted by the new nodes and rejected by the
old, and the two sets would compute different app hashes in that block.

Two things follow from the absence of a store upgrade, and both were decisions:

- **No `StoreUpgrades.Added` entry.** Adding a key that is already mounted is
  not harmless housekeeping.
- **No consensus-version bump.** A bump obliges a registered migration, and
  there is nothing to migrate. A no-op migration registered to satisfy a version
  number is a lie about what the upgrade did.

`app/profile_upgrades_on_test.go` asserts both, so a later change that quietly
adds a store or a version bump fails a test rather than a halt height.

## 2. The profile split

Identical in shape to the previous upgrade. `x/syndicate` is not linked into a
settlement build, so `profile_upgrades_off.go` carries an entry under the **same
name** that does nothing but exist.

That entry is the point rather than a formality: an upgrade name a binary does
not know is not a no-op — `x/upgrade` panics the node at the halt height and it
stays down until somebody installs a binary that knows the name.

## 3. What the handler asserts

Nothing ceremonial, and nothing that writes.

- **The new collections answer.** An unregistered collection reads as a zero
  value and fails at its first write, in a block on a live chain, rather than
  here.
- **They answer empty, and the id sequence is at 0.** A document is worth the
  accountability of whoever attested to it, and nobody attested to anything
  during a halt. A handler that seeded a document would be manufacturing
  evidence.

## 4. The run

### Step 1 — build and stage, done

```
amd64  /opt/yamale/bin/blockchaind.next on 92.4.151.72   (VM, majority validator)
arm64  /opt/yamale/bin/blockchaind.next on 10.0.0.198     (pi-2)
```

Both verified before staging: `version` exits 0 — which is not a formality on
this codebase, since a proto field named `fees` once made pflag panic while the
root command was built and killed `version` and `start` alike — and
`tx syndicate attach-document --help` prints, so the message is linked in.

### Step 2 — the proposal

Proposal **17**, `MsgSoftwareUpgrade`, plan `syndicate-documents`, halt height
**333,108** (submitted at 331,908; ~4.9 s blocks put it about 95 minutes out,
against a 30-minute voting period).

### Step 3 — the vote is the operator's

`alice` holds 64,574,300,000 of 174,375,300,000 bonded — 37.0% against a 33.4%
quorum and a 50% threshold, so one delegator carries it alone. Not a formality:
it decides the outcome.

### Step 4 — cross the height

Every node halts with `UPGRADE NEEDED`. On each host:

```bash
sudo systemctl stop yamale-devnet
sudo cp /opt/yamale/bin/blockchaind /opt/yamale/bin/blockchaind.pre-documents
sudo mv /opt/yamale/bin/blockchaind.next /opt/yamale/bin/blockchaind
sudo systemctl start yamale-devnet
```

Still two manual swaps: devnet-2 does not run `cosmovisor`. That remains the
right follow-up — an unattended swap is the difference between an upgrade and an
outage at 3am.

### Step 5 — verify

The query that answers `code 6` today must answer `code 0`:

```bash
curl -s 'https://pay.yamalelegal.com/api/rpc/abci_query?path=%22/blockchain.syndicate.v1.Query/Documents%22&data=0x0801'
```

Then seed real papers with `seed-deal-papers.sh`, which writes actual files,
hashes them with `sha256sum`, and attaches the digests. That matters: a fixture
using 32 arbitrary bytes renders identically on screen and proves nothing, and
the entire claim of this feature is that the digest shown matches `sha256sum`
run against a file somebody was sent.

## 5. Rollback

Same as the previous upgrade, and the same trap. Restoring the pre-upgrade
binary makes the chain resume at the pre-upgrade height **with the plan still
scheduled**, so it halts again at the same height. The plan must then be
cancelled by governance before the height is reached again.

Snapshot `/opt/yamale/node/data` on the VM before the halt height. The cheaper
mitigation is that this upgrade writes nothing: a rollback loses no state
because the handler created none.
