# Upgrading a running chain

**Before you start:** a node you can reach over SSH on every host in the
validator set, a build of the new binary for each host's architecture, and an
account holding enough stake to meet quorum on its own or the cooperation of
enough that do.

**At the end:** every node running the new binary, past the upgrade height, with
the same application hash — which is the only evidence that they agree.

A Cosmos chain does not upgrade itself. Governance agrees a *height*; at that
height every node stops with an error and refuses to continue, and an operator
replaces the binary. The chain is down between those two events. The whole
procedure is arranged around making that window short and making it impossible
to walk into it unprepared.

---

## 1. Build the binary for every architecture in the set

Not "for Linux". This chain runs on an amd64 VM and an arm64 Raspberry Pi, and a
node that cannot execute the binary is a node that stays down.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/blockchaind-amd64 ./cmd/blockchaind
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o /tmp/blockchaind-arm64 ./cmd/blockchaind
sha256sum /tmp/blockchaind-amd64 /tmp/blockchaind-arm64
```

`-trimpath` and `CGO_ENABLED=0` are not decoration: they are what make the hash
reproducible, and the hash is what an operator checks at 3am against the one
recorded in the proposal.

Copy each to its host **before proposing anything**, and record both hashes in
the proposal's `info` field along with the commit they were built from. An
upgrade whose binary is staged after the vote passes is an upgrade racing a
deadline it set itself.

## 2. Propose the height

```bash
blockchaind tx gov submit-proposal upgrade.json \
  --from <key> --chain-id <chain-id> --node <rpc> --fees 0uyml --gas 400000 -y
```

Choose the height so that **the voting period ends comfortably before it.** The
arithmetic is the entire risk in this step:

```bash
# seconds per block, measured rather than assumed
blockchaind q block --type=height <h1> -o json | jq -r .header.time
blockchaind q block --type=height <h2> -o json | jq -r .header.time
```

Block time drifts with the validator set and the network. On this chain it has
been between 5 and 6 seconds; a 30-minute voting period is therefore about 330
blocks. Leave several times that. If the chain reaches the height while the
proposal is still in its voting period, the plan is never scheduled, nothing
halts, and the proposal expires having achieved nothing.

## 3. Vote — and confirm the tally, not the transaction

**This is the step that fails.** A proposal sitting at zero votes looks exactly
like a proposal that is progressing: it is `VOTING_PERIOD`, it has a deadline,
nothing is wrong. It simply expires.

```bash
blockchaind tx gov vote <id> yes --from <key> --chain-id <chain-id> --node <rpc> -y
blockchaind q gov tally <id> --node <rpc>
```

Read the tally against the bonded total, not against your expectations:

```bash
blockchaind q staking validators -o json | jq -r '.validators[] | "\(.tokens) \(.description.moniker)"'
blockchaind q gov params --node <rpc> | grep -E 'quorum|threshold'
```

Quorum is a fraction of **all** bonded power, not of the power that voted. On a
small chain one delegator can carry a proposal alone, and on a small chain one
delegator can also be the only participant and still fall short of quorum. Work
it out; do not infer it.

Two traps in the command itself, both of which return a plausible failure that
is not the real one:

* `--chain-id` must be the chain's actual id. A mismatch is reported as
  `signature verification failed`, which reads as a key problem.
* Through `x/group`, `tx group submit-proposal --exec try` returns code 0 on
  broadcast even when the inner message fails. Read
  `PROPOSAL_EXECUTOR_RESULT` from the events, never the top-level code.

Then confirm the plan is actually scheduled — this is the only proof the vote
did what you wanted:

```bash
blockchaind q upgrade plan --node <rpc>
```

An empty `{}` after the voting period has closed means it did not pass.

## 4. Wait for the halt

At the upgrade height every node logs `UPGRADE "<name>" NEEDED at height: N`,
then `CONSENSUS FAILURE!!!` and stops producing. **This is success.** The panic
and the stack trace are how `x/upgrade` refuses to continue on the wrong binary,
and reading them as a crash is the most likely way to make this worse.

```bash
curl -s localhost:26657/status | jq -r .result.sync_info.latest_block_height
journalctl -u <unit> --no-pager | grep -E 'UPGRADE .* NEEDED'
```

## 5. Swap the binary on every host

```bash
sudo systemctl stop <unit>
sudo cp /opt/yamale/bin/blockchaind /opt/yamale/bin/blockchaind.pre-<name>
sudo install -m755 /tmp/blockchaind-next /opt/yamale/bin/blockchaind
sha256sum /opt/yamale/bin/blockchaind        # against the proposal, every time
sudo systemctl start <unit>
```

Keep the old binary. It is the only way back if the new one panics during the
migration, and it costs a few hundred megabytes.

## 6. Verify agreement, which is the only thing that counts

```bash
journalctl -u <unit> --no-pager | grep 'applying upgrade'
journalctl -u <unit> --no-pager | grep 'height=<upgrade-height>' | grep -o 'app_hash=[A-F0-9]*'
```

**Every host must report the same application hash at the upgrade height.** A
node that produces a different one has forked: it will be unable to commit and
will sit there logging vote mismatches. One hash per host, compared by eye, is
the whole verification.

Then confirm the upgrade did the thing it was for — a new parameter is
queryable, a new message appears in `tx <module> --help`. A migration that runs
without error and changes nothing is a migration you have not tested.

## What has actually gone wrong here

* **A halted chain that nobody noticed for nine hours.** The Pi rebooted and the
  node's unit was never `enable`d, so it did not come back. Nothing alarmed,
  because a chain that is not producing blocks is not producing errors either.
  `systemctl is-enabled` on every node is part of this procedure.
* **A proposal at zero votes, 25 minutes from expiry.** Caught by reading the
  tally. Nothing else would have caught it.
* **`{}` from `q upgrade plan`** while the proposal was still open, mistaken for
  a failed submission. It only populates once the proposal passes.

## Related

* [Run a local chain](local-devnet.md) — rehearse an upgrade here first. The
  migration runs the same way against one node as against three.
* [What governance can and cannot change](constitution.md) — some parameters an
  upgrade might want to touch cannot be changed by an ordinary proposal.
