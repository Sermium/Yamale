# Upgrading devnet-2 to pay by time held, and to let a holder sell

Two consensus upgrades run back to back on 2026-09-16/17, and the first one did
not go cleanly. The failure is written up here at the same length as the
success, because it cost twenty minutes of avoidable downtime on an already
halted chain and the rule it teaches is not stated anywhere else.

Companion to [upgrade-devnet-2-modules.md](upgrade-devnet-2-modules.md), which
carries the general shape, and to [upgrades.md](upgrades.md), which carries the
rules. Only what differs is repeated here.

---

## 1. What changed

### `tokenisation-prorata`, plan height 376,100

| | |
|---|---|
| Store added | **none** |
| State migrated | **every vault and every position** |
| Applied | height 376,100, 2026-09-16 21:18 UTC |

Entitlement stops being read from an index delta and starts being apportioned by
time held. `cost_basis` and `cost_qty` arrive on every position, which is what
the market writes into.

### `market`, plan height 378,800

| | |
|---|---|
| Store added | **`market`** |
| State migrated | none — the module arrives empty |
| Applied | height 378,800, 2026-09-17 |

A holder may sell three ways: a fixed-price listing, an auction by deadline, or
a resting limit order in a book.

### Why the order is not a preference

Every fill writes the buyer's cost basis into `x/tokenisation`. `cost_basis` is
the field `tokenisation-prorata` creates. A market that traded before that
upgrade would record what every holding cost into a field that does not exist.

## 2. The migration that would otherwise have lost people money

Under the old rule a holder's unsettled income was `balance * (cumulative -
last_index)`, computed at read time. Pro-rata temporis does not read that delta
at all — entitlement now comes from per-period records, and there are none for
any period before the upgrade.

So a position carrying an unsettled delta at the halt height would simply stop
being owed it. Not an error, not a halt: money quietly ceasing to exist, for
every holder who had not happened to transfer since the last distribution. The
handler settles that delta at the OLD rule before switching, which is the whole
reason it exists rather than a few field initialisations.

Read back off the chain afterwards, on the timber holding's vault of 15,000,000
USD:

| holder | accrued | cost_basis | cost_qty | token_seconds |
|---|---|---|---|---|
| 78% | 11,700,000 USD | 0 | 0 | 0 |
| 9% | 1,350,000 USD | 0 | 0 | 0 |
| 2.2% | 330,000 USD | 0 | 0 | 0 |

Every entitlement carried across intact. The three zeroes are set **explicitly**
rather than left unset: `math.Int`'s zero value is nil and panics on arithmetic,
so the first transfer after the upgrade would otherwise have taken down every
validator at once.

`cost_qty: 0` means NOT RECORDED, and the console says so rather than showing a
cost of zero. Nobody ever told the chain what these holders paid — they were
seeded by mints and sends. The field populates on future acquisitions, through a
subscription or a market fill.

## 3. The failure: a binary that was ahead of its upgrade

The binary staged for `tokenisation-prorata` was built from `main`, which by
then had merged `x/market`. A linked module mounts its store key
unconditionally, and the store loader at a halt height only adds the keys named
by the **pending** plan's `StoreUpgrades`. So every start panicked:

```
panic: failed to load latest version: version of store market mismatch
root store's version; expected 376099 got 0; new stores should be added
using StoreUpgrades
```

`version` exited 0, so every pre-flight check reported success, and
`swap-upgrade-binary.sh` declared the swap done. systemd restarted the node
seventeen times before anybody read a log.

**The rule:** the binary you cross an upgrade with must not link a module whose
store a later upgrade adds. You cannot stage ahead of the plan you are crossing.

The fix was to rebuild from the commit before the market merge — one command,
and the chain came up on the first attempt with that binary. Two things changed
afterwards so the next person does not pay for it again:

- `swap-upgrade-binary.sh` no longer accepts `version` as proof. It waits up to
  60 s for the node to **leave** the halt height, prints what it panicked about
  if it does not, and rolls back automatically. A node that cannot start is
  strictly worse than one that was never installed, and the chain simply
  returns to the halt it was already in.
- The rule is written in [upgrades.md](upgrades.md). There is no test for it,
  because the invariant is about which commit you build rather than about code.

## 4. The second failure, which had nothing to do with the upgrade

The Pi's `yamale-rpcgate` died at 08:28:14 — the second `systemctl stop
yamale-devnet` ran for the swap — and did not come back with the node. Its unit
carried `PartOf=yamale-devnet.service`, which propagates stop and restart but
never start. `/api/rpc/` answered 502 for two and a half hours and every console
that reads the chain over RPC was down with it. `/api/rest/` kept answering 200,
which is why nothing looked obviously wrong.

This had been diagnosed and corrected in the repository on **2026-09-10**, with
a comment in the unit spelling out this exact failure. The corrected unit was
never installed on either host.

`deploy/deploy.sh` now compares the installed unit against the repository's on
both hosts and names `install-rpcgate.sh` when they differ. Compared rather than
copied: installing a unit implies a daemon-reload and a restart of the service
in front of the public RPC, and a deploy should not do that as a side effect of
shipping a stylesheet.

## 5. What the market handler refuses

Nothing ceremonial, and nothing that writes.

- **`max_fills_per_order` is not zero.** At zero no incoming order could ever
  cross another: every order would rest, the book would fill with orders
  overlapping in price that never trade, and the market would LOOK like it
  worked. A market that refuses orders is visibly broken; one that matches none
  is not.
- **Every id sequence starts at 0.** A counter arriving part-used would mean the
  handler wrote an offer nobody made, escrowed with property nobody committed.
- **The store answers.** An unregistered collection reads as a zero value and
  fails at its first write, in a block on a live chain, rather than here.

## 6. Rollback

Each node keeps the binary it was running as
`blockchaind.pre-<upgrade-name>` — named from the chain's own plan rather than
hardcoded, which it used to be, so the second upgrade's backup no longer
overwrites the first's. `swap-upgrade-binary.sh` refuses to start if a backup
under that name already exists: an operator running the swap twice is already
having a bad night, and silently destroying the rollback is not what that moment
needs.

Rolling back after a plan has been applied is not a binary swap — the state has
migrated. It is the export-and-restart path in
[`scripts/devnet/upgrade.sh`](../../scripts/devnet/upgrade.sh), and it costs the
block history.
