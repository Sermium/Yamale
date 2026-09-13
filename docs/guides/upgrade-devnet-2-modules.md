# Upgrading devnet-2 to carry x/herd, x/syndicate and x/selfcustody

The two new consoles and the key-disclosure pathway are built, tested and
committed. They read nothing on devnet-2, because the binary that chain runs
predates all three modules. This is the plan to change that.

Nothing in this document has been executed. It ends with the one step that is
not mine to take.

---

## 1. Where the chain actually is

Measured against the public RPC on 2026-09-12, not recalled:

```
$ curl -s https://pay.yamalelegal.com/api/rpc/status
  moniker pi · height 307,649 · catching_up false

$ curl -s '…/abci_query?path="/blockchain.syndicate.v1.Query/Params"&data=0x'
  code 6 · unknown query path
  (same for /blockchain.herd.v1.Query/Params and
           /blockchain.selfcustody.v1.Query/Params)

$ curl -s '…/api/rpc/validators'
  57FB83839F903D0E9BDD7B6DB36B2E201929F100  power 122430
```

Three facts follow, and each one shapes the plan:

- **The chain is healthy and producing.** Block time current, not catching up.
- **The three modules are absent.** Not misconfigured — absent. `code 6` is the
  node saying it has no such query path.
- **The active validator set is one.** `pi-2` (52,470, the Raspberry Pi) is not
  in it. It was restored on 2026-09-11 and has since fallen out again; the Pi
  has been off the tailnet for ~10 hours and answers on no port from the LAN.

That last one is a blocker for the *deployment*, not for the upgrade — see §6.

---

## 2. Why this cannot be a rolling restart

Adding a module to a Cosmos chain is consensus-breaking twice over:

- it adds a **store**, which changes the IAVL tree the app hash is computed
  from; and
- it runs **InitGenesis**, which writes state at one specific height.

A node that swaps binaries at a height of its own choosing computes a different
app hash from its peers at the next block and is evicted. Every node must cross
the same height with the same binary. That is what `x/upgrade` is for.

---

## 3. Two paths

### Path A — export and restart

`scripts/devnet/upgrade.sh`, already written and proven on this chain. Exports
state, splices in a default genesis section for each new module, resets blocks,
replays from genesis.

- **Keeps:** accounts, balances, aliases, treasuries, pools, user IDs.
- **Loses:** all 307,649 blocks. Heights restart at 1.
- **Costs:** one coordinated restart, no governance vote.

### Path B — coordinated `x/upgrade` (recommended)

The chain already has the machinery, and a precedent that is exactly this shape:
`netting-and-perimeter` added `x/netting`'s store and let its `InitGenesis`
arrive through the standard migration path. `app/upgrades_test.go` drives a
synthetic upgrade end to end, so the path is known to work rather than assumed
to.

- **Keeps:** everything, block history included.
- **Costs:** a governance proposal, a halt height, and both binaries staged
  before that height arrives.

**Recommendation: Path B.** The block history is the only thing Path A destroys,
and on a chain being shown to people as a working ledger, "the explorer starts
at block 1 again" is a bad answer. Path A stays as the fallback if the Pi cannot
be brought up in time — see §6.

---

## 4. The code change

One entry appended to `upgrades` in `app/upgrades.go`. The file's own comment
says adding an upgrade means appending to that list and nothing else; that is
almost true here, and the exception is worth stating before it is discovered at
a halt height.

### 4.1 The profile problem

The three modules are not in the same set of build profiles:

| module          | default profile | settlement profile |
|-----------------|-----------------|--------------------|
| `x/herd`        | linked          | **not linked**     |
| `x/syndicate`   | linked          | **not linked**     |
| `x/selfcustody` | linked          | linked             |

`app/upgrades.go` carries no build tag and currently imports only modules that
exist in every profile. Declaring `herdtypes.StoreKey` there would compile — the
types package exists either way — but a settlement binary would then add two
stores no module ever reads, and the migration would look for modules that are
not in the manager.

**Resolution:** declare the store list the way `app/app_config.go` already
declares module configs — a profile-varying slice contributed from build-tagged
files, concatenated at the declaration site.

```go
// app/upgrades.go
var upgrades = slices.Concat([]Upgrade{ /* …existing three… */ }, profileUpgrades)
```

```go
// app/profile_upgrades_on.go   //go:build !settlement
var profileUpgrades = []Upgrade{{
    Name: "herd-syndicate-selfcustody",
    StoreUpgrades: storetypes.StoreUpgrades{Added: []string{
        herdtypes.StoreKey, syndicatetypes.StoreKey, selfcustodytypes.StoreKey,
    }},
    Handler: …,
}}
```

```go
// app/profile_upgrades_off.go  //go:build settlement
// A settlement deployment gets the door out and neither commercial module.
var profileUpgrades = []Upgrade{{
    Name: "herd-syndicate-selfcustody",
    StoreUpgrades: storetypes.StoreUpgrades{Added: []string{selfcustodytypes.StoreKey}},
    Handler: …,
}}
```

The name is the same in both because they are different chains, never the same
one; what differs is what each is carrying.

### 4.2 What the handler must assert

Following the `netting-and-perimeter` precedent, the handler exists for its
assertions rather than for state it writes. Three are worth making:

1. **`x/syndicate` arrives with no operator admitted.** Governance admits
   operators, and an upgrade that seeded one would be admitting a platform
   nobody voted for. Assert the operator collection is empty.

2. **`x/herd` arrives with no authority admitted.** Same argument: an office
   that registers animals is admitted by a vote, not by a migration.

3. **`x/selfcustody`'s published acknowledgement hash equals the binary's own.**
   This one is not ceremonial. `Params.AcknowledgementHash` is what every
   `RequestDisclosure` and `ConfirmDisclosure` is checked against; if the chain
   publishes a hash the binary's embedded text does not produce, **every holder
   is refused and nobody can take their key at all** — the exact failure the
   module exists to prevent. Assert
   `params.AcknowledgementHash == selfcustodytypes.AcknowledgementHash()`.

### 4.3 What does *not* go in the handler

The syndicate send restriction and the AMM denom classifier are registered in
`app.go` at construction, not by a migration. They arrive with the new binary
automatically. Putting them in the handler would register them twice.

### 4.4 Tests that must pass before proposing

`app/upgrades_test.go` already enforces most of this and will fail if the entry
is malformed:

- `TestEveryDeclaredUpgradeHasAHandler`
- `TestUpgradeDeclarationsAreWellFormed` (unique name, no empty store keys)

Add one per assertion in §4.2, driven the way `TestNettingArrivesSwitchedOff`
drives its handler with `priorVersions(...)` — the version map a *real* upgrade
receives, not `GetVersionMap()`, or `RunMigrations` sees nothing to do and never
calls the new modules' `InitGenesis`.

Then, on both profiles:

```bash
go test ./app/... && go test -tags settlement ./app/...
```

---

## 5. The run

### Step 1 — build and stage the binary

On this machine the freshly built binary is blocked by Windows app-control at
the repo path; the session scratchpad is not blocked.

```bash
SCRATCH=…/scratchpad
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind
"$SCRATCH/blockchaind.exe" version   # must not panic — see §7
```

Build the Linux binary for the hosts, and put it on **both** the VM and the Pi
at a path that is not yet live (`/opt/yamale/bin/blockchaind.next`). Staging
before the vote is deliberate: a validator that discovers at the halt height
that it has no new binary is a validator that is down until somebody notices.

### Step 2 — rehearse on a local chain first

The local devnet from the last session is still the cheapest rehearsal: it runs
the new binary with all three modules and 646+ blocks of real state. Drive the
upgrade against it end to end before touching devnet-2.

### Step 3 — submit the proposal

A `MsgSoftwareUpgrade` naming `herd-syndicate-selfcustody` and a halt height far
enough out to be comfortable — at ~5s blocks, +2,000 blocks is under three
hours.

I will submit this and print the vote command.

### Step 4 — **the vote is yours**

I will not cast it. On this chain one delegator carries a proposal alone, so a
vote is not a formality — it decides the outcome. You get to see the sequence
work: submit → vote → tally → pass → execute.

### Step 5 — cross the height

Every node halts at the height with `UPGRADE NEEDED`. On each host: stop the
service, move `blockchaind.next` over `blockchaind`, start the service.

`cosmovisor` would do this unattended; devnet-2 does not run it, so it is two
manual swaps. Worth noting as a follow-up — an unattended swap is the difference
between an upgrade and an outage at 3am.

### Step 6 — verify

The same three queries that are `code 6` today must answer `code 0`:

```bash
for q in /blockchain.herd.v1.Query/Params \
         /blockchain.syndicate.v1.Query/Params \
         /blockchain.selfcustody.v1.Query/Params; do
  curl -s "https://pay.yamalelegal.com/api/rpc/abci_query?path=%22$q%22&data=0x"
done
```

Then the consoles, against the funnel rather than against localhost — the public
URL has the last word, for the reason `deploy/deploy.sh` gives at length.

---

## 6. The Pi, which is the real blocker

The consoles cannot be deployed until the Pi is back, because the Pi *is* the
public host — the funnel terminates on it. As of now:

- `tailscale status` → `100.68.207.17 yamale … offline, last seen 10h ago`
- `10.0.0.198` answers on none of 22, 2222, 80, 8093, 26657, 1317
- `10.0.0.188` is no longer the Pi at all; it is a Philips appliance that took
  the address
- `pi-2` has fallen out of the validator set again

**This does not block the upgrade.** The active set is one validator at 100% of
voting power, so the VM alone crosses the halt height and the chain resumes
without the Pi.

**It does block two things:**

1. **Deploying the consoles.** No Pi, no public site.
2. **Letting `pi-2` rejoin.** If it comes back and unjails onto the *old*
   binary, it will halt at the upgrade height and stay down — worse, if it comes
   back *before* the upgrade and starts signing, it is then a second node that
   must cross the height correctly. Either stage the new binary on it first, or
   leave it jailed until the upgrade is done.

The safe ordering is therefore: **upgrade the VM first, with the Pi down and
`pi-2` jailed.** That is the one configuration where a two-validator upgrade has
no coordination problem at all.

---

## 7. Risks, and what each one looks like

| risk | what you would see | mitigation |
|---|---|---|
| Halt height passes with no new binary on the VM | chain stops at the height, `UPGRADE NEEDED` in the log | stage the binary before the vote, not after |
| `pi-2` rejoins mid-upgrade on the old binary | app hash mismatch, node evicted | keep it jailed until the upgrade completes |
| Upgrade name typo | every node halts on a name mismatch | `TestUpgradeDeclarationsAreWellFormed`; the name must never change once proposed |
| `x/enforcement` params rejected on start | node refuses to start after the swap | already handled on the export path; Path B keeps existing params, so this is a Path-A-only risk |
| Acknowledgement hash mismatch | every key-disclosure request refused with `ErrWrongAcknowledgement` | §4.2 assertion 3 |
| Binary panics at startup | `blockchaind version` panics | run `version` on the built binary — this is exactly how the `--fees` flag collision was found on 2026-09-12 |

---

## 8. Rollback

`x/upgrade` has no undo. The rollback is the backup:

- snapshot `/opt/yamale/node/data` on the VM before the halt height;
- keep the old binary in place as `blockchaind.prev`;
- if the new binary will not start, restore the data directory and the old
  binary, and the chain resumes at the pre-upgrade height with the plan still
  scheduled — so the plan must then be cancelled by governance before the height
  is reached again.

---

## 9. What I need from you

1. **Approve Path B** (or say Path A, if losing block history is fine and you
   want this done in one step with no vote).
2. **The Pi.** Whether it is coming back, and whether I should plan the upgrade
   around it being down.
3. **The vote**, at step 4.

Once 1 and 2 are answered I can write the `app/upgrades.go` entry, the two
build-tagged files, the tests, and rehearse the whole thing on the local devnet
— none of which touches devnet-2.
