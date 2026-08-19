# Yamale 3-validator testnet deployment runbook

> **Status:** steps 1–5, 7 and 8 have been rehearsed end to end with **three
> validators** on one machine — three homes, three keys, three gentxs collected
> into one genesis, peered, reaching consensus at identical app hashes, verified
> by `07-verify.sh`, with the activity bot running against that chain.
>
> **Not yet exercised:** step 6, because systemd does not exist on the rehearsal
> machine — the unit files have been reviewed but never started. And nothing has
> run across genuinely separate hosts: distinct IPs, a real network between
> them, and firewall rules are all untested.
>
> What the rehearsals changed:
>
> - **Step 2 is now safe to re-run.** It used to edit `genesis.json` in place,
>   so a run that failed part way left accounts already added and every retry
>   aborted with `Account ... already exists`. Step 1 now keeps a pristine copy
>   and step 2 rebuilds from it.
> - **Step 2 now sets the emission schedule**, for the same reason it already
>   reset the governance periods: the module's defaults compress the entire
>   issuance curve into about an hour so it is observable on a devnet. Measured
>   on a real node, supply went from 300k YML to 971M within 900 blocks. The
>   ceremony now stretches the same curve over years.
> - **Step 2 turns off interchain accounts**, which is the change with the
>   sharpest edge. The module's default genesis enables the host with
>   `allow_messages: ["*"]`, and an interchain account executes messages through
>   the message router rather than the ante chain — so the validator gate, which
>   is an ante decorator, does not apply to them. With the host enabled, anyone
>   able to open a channel to this chain could execute `MsgCreateValidator`
>   through it and join a permissioned validator set without a vote, and reach
>   every other message the chain has by the same route. Nothing is lost by
>   disabling it at launch: no IBC connection exists on day one. Enabling it
>   later is a governance decision that needs an explicit allow-list rather than
>   a wildcard.
> - **Step 2 verifies its own output, and this is the important one.** In the
>   three-validator rehearsal its Python block failed — `python3` on that machine
>   is a Microsoft Store stub — and the script produced a genesis that
>   `blockchaind genesis validate` accepted, because every devnet default in it
>   is individually legal. A genesis carrying the devnet emission schedule would
>   have minted roughly a thousand times too fast on a live network, and nothing
>   would have said so. Step 2 now tries `python3` then `python`, and re-reads
>   the finished file to confirm the parameters really changed, refusing to write
>   `canonical-genesis.json` otherwise.

This is the genesis ceremony and deployment process for running the
`yamale-testnet-1` chain on 3 separate Linux VMs, one validator each. It
uses the standard Cosmos SDK multi-validator genesis flow (`init` →
distribute a canonical genesis → `gentx` on each node → `collect-gentxs` →
redistribute the final genesis → configure peers → start).

This runbook is meant for you (or whoever operates the VMs) to execute: steps 6
onwards need root on the target servers, and the whole flow needs SSH access to
all three. Steps 1–5 have been rehearsed against three local homes on separate
ports, which exercises everything except the hosts being genuinely separate —
distinct IPs, a real network between them, and firewall rules.

## 0. Prerequisites, per VM

- Linux x86_64 (Ubuntu 22.04+ or similar), a non-root user (e.g. `yamale`)
  to run the daemon.
- Open inbound ports: `26656` (p2p, must be reachable between all 3 nodes),
  `26657` (RPC), `1317` (REST), `9090` (gRPC). RPC/REST/gRPC only need to be
  reachable by you/your tooling, not the public internet, unless you want
  public block explorers/faucets to reach them.
- The `blockchaind` binary, built for `linux/amd64`. Build it on the VMs
  themselves — `go build -o blockchaind ./cmd/blockchaind` from the repo root,
  Go 1.25.10+ (the floor in go.mod) — or cross-compile from the dev machine with
  `GOOS=linux GOARCH=amd64 go build -o blockchaind ./cmd/blockchaind`.

  > An earlier version of this runbook said to copy the binary out of the
  > project's WSL2 environment. There is no longer a WSL distribution on the
  > dev machine; the toolchain is native Windows Go, so that path does not
  > exist and cross-compiling is the equivalent.

- `python3` (or `python`) on whichever machine runs step 2. It edits the
  genesis parameters, and step 2 refuses to produce a genesis without it.

## 1. Initialize each node (run on all 3 VMs)

```bash
./01-init-node.sh validator-1   # use validator-2 / validator-3 on the other VMs
```

Each operator then creates their own operator key (do this once, keep the
mnemonic offline):

```bash
blockchaind keys add validator --keyring-backend file
```

Send your operator address (`blockchaind keys show validator -a --keyring-backend file`)
and your desired self-bond amount to whoever is acting as coordinator for
step 2. `file` keyring is used here (not `test`) since these are real
validator keys — it encrypts the keyring on disk behind a passphrase.

## 2. Build the canonical genesis (coordinator only, once)

Create `accounts.txt` with one `<address> <amount>uyml` line per validator,
then:

```bash
./02-build-canonical-genesis.sh accounts.txt
```

This also resets governance voting periods to realistic values (48h voting,
24h expedited) — the 10s/5s periods in the repo's `config.yml` are for local
devnet iteration only and must not reach a real genesis.

The foundation account that receives recovered assets is **required**, and the
script refuses to build a genesis without it:

```bash
RECOVERY_DESTINATION=yml1... ./02-build-canonical-genesis.sh accounts.txt
```

This used to be optional, on the argument that a chain which can freeze but not
seize is a safe launch. That argument did not survive contact with the devnet,
which ran for weeks with the parameter empty: what an empty destination actually
buys is a chain where two thirds of the validator set can pass a seizure that
then has nowhere to send what it took, and nobody notices until something prints
the parameter. The module now refuses to start without one, so a genesis built
without one would fail on every validator at height 1 instead. See
[the enforcement guide](../../docs/guides/enforcement.md).

Create the foundation account before this step if it does not exist yet — it is
also the issuer named in step 2b, so the same address is used twice.

Send the resulting `canonical-genesis.json` to all 3 validators, who each
overwrite their `$HOME/.blockchain/config/genesis.json` with it.

> Produce this file fresh for the launch. It is not kept in the repository,
> because a stored copy carries whichever accounts and module set existed when
> it was made — and one predating a module fails `genesis validate` on every
> validator with `section is missing in the app_state`, which is a confusing way
> to find out you distributed the wrong file. (That check is the SDK's, and it
> is reliable: a genesis missing a module section is refused rather than
> starting a chain with that module uninitialised.)

### 2b. Seed the currencies (coordinator only, optional)

To launch with the African currency set — 42 ISO 4217 codes covering all 54
countries, since XOF and XAF are monetary unions — run this against the
canonical genesis before distributing it:

```bash
go run ./tools/currencies --genesis canonical-genesis.json --issuer <foundation address>
```

That writes three things at once: the stablecoin module's approved issuers, the
bank module's denom metadata so wallets render `₦1,359.84` instead of
`1359844414 ungn`, and the oracle's accepted denom list so a rate can be agreed
for each one.

Genesis rather than governance on purpose. Each of these approvals is a
decision a real network should vote on one at a time — which is exactly why
there is no message that registers forty-two currencies at once. On a testnet
the vote teaches nobody anything and delays the point, which is having real
currencies to move around.

The foundation is the single approved issuer for all of them on testnet. Nobody
else can mint them, and the approval is visible in genesis rather than being a
setting somebody remembers.

### 2c. Price feeder (each validator, after launch)

The oracle only agrees a rate when enough stake reports one, so the currencies
seeded in 2b stay unpriced until validators run a feeder:

```bash
go build -o /usr/local/bin/feeder ./tools/feeder
systemctl enable --now yamale-feeder
```

`yamale-feeder.service` is in this directory. Set `--validator` to your operator
address and give the feeder its own hot key (`tx oracle delegate-feeder`) rather
than the validator's.

**Use different `--source` values across the set.** The oracle takes a
stake-weighted median, and four validators reporting from the same endpoint is
one source counted four times — the threshold looks like agreement and measures
nothing. `yahoo` gives intraday market quotes, `erapi` a daily published table.

Currencies the source cannot price are named in the log every round and simply
not reported, rather than filled in with a stale number. A rate nobody submits
ages out and the chain refuses it; a rate quietly invented does not.

### 2d. Faucet and monitoring (main server, after launch)

```bash
go build -o /usr/local/bin/faucet ./tools/faucet
systemctl enable --now yamale-faucet
```

`yamale-faucet.service` is in this directory. It listens on localhost and
belongs behind the same reverse proxy as the explorer — its per-address
cooldown stops one address asking twice and does nothing about one script
asking from ten thousand addresses, so the rate limiting that matters is at the
proxy.

Give the faucet key only what a testnet needs and refill it deliberately. A
drained faucet should be an empty faucet, not an empty treasury.

Metrics are off by default. On each validator:

```toml
# config.toml
prometheus = true
prometheus_listen_addr = ":26660"
```

Reachable from the monitoring host only — it is an unauthenticated endpoint
that describes your node's health to anyone who asks.

`monitoring/prometheus.yml` and `monitoring/alerts.yml` in this directory are
the scrape config and seven alert rules. Every metric name in them was scraped
from a running node rather than taken from documentation: the names changed
between Tendermint and CometBFT, and a rule naming a metric that does not exist
never fires and never says so.

The one that matters most on a small set is `ValidatorMissing`. At four equal
validators consensus needs three; at three it needs all three. A single absent
validator is not a warning here — it is the last one before the chain stops.

**Application alerting is not configured**, and `alerts.yml` says why rather
than shipping a rule that quietly never fires: CometBFT exports consensus and
mempool metrics only, nothing from this chain's own modules, so there is no
metric for a stale price feed to alert on. Closing that needs a small exporter
polling the oracle — the file describes it.

## 3. Create each validator's gentx (run on all 3 VMs)

```bash
./03-create-gentx.sh validator-1 100000000uyml
```

Send the resulting `gentx-<node-id>.json` back to the coordinator.

## 4. Collect gentxs into the final genesis (coordinator only, once)

```bash
./04-collect-genesis.sh /path/to/dir/with/all/three/gentx/files
```

Send the resulting `final-genesis.json` to all 3 validators, who each
overwrite their `config/genesis.json` with it (again).

## 5. Configure peers (run on all 3 VMs)

Get each node's ID with `blockchaind tendermint show-node-id`, then on each
VM point at the *other two*:

```bash
./05-configure-peers.sh <peer1-id>@<peer1-ip>:26656 <peer2-id>@<peer2-ip>:26656
```

## 6. Install and start the systemd service (run on all 3 VMs)

```bash
sudo cp yamaled.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now yamaled
```

## 7. Verify

```bash
./07-verify.sh http://<peer1-ip>:26657 http://<peer2-ip>:26657
```

Give it the *other two* validators' RPC URLs. It checks that the local node is
responding and its height is actually advancing, that the expected number of
validators is bonded, that every node reports the same block hash at the same
height, and whether any price feeds are running. It exits non-zero with a count
of problems, so it works as a gate in a larger script.

The cross-node hash check is the one that matters most: a mismatched genesis
produces two chains that each look perfectly healthy on their own.

Sample output from a healthy network, and from the same network with one
validator stopped — both taken from the deployment rehearsal:

```
=== local node ===                        === local node ===
  chain yamale-testnet-1 at height 261       chain yamale-testnet-1 at height 271
  ✓ producing blocks (261 → 264)            ✗ no new block in 12s (still 271)
  ✓ caught up with the network              ✓ caught up with the network
=== validator set ===                     === validator set ===
  ✓ 3 of 3 validators bonded                ✓ 3 of 3 validators bonded
=== agreement across nodes ===            === agreement across nodes ===
  ✓ …:26667 agrees at height 264            ✓ …:26667 agrees at height 271
  ✓ …:26677 agrees at height 264            ✗ …:26677 unreachable
All checks passed.                        2 problem(s) found.
```

Note that the validator set still reads 3 of 3 in the failed case: a stopped
validator is still bonded. Liveness has to be checked by watching the height
move, which is why the script waits and looks twice.

**Losing any one validator stops the chain.** With 3 equal-power validators the
network tolerates 0 failures under the classic BFT bound (`n ≥ 3f+1`): two of
three is exactly ⅔ of the voting power, and committing a block needs *more* than
⅔. This was a known, accepted tradeoff for this testnet size, and the rehearsal
confirmed it is not theoretical — stopping one node produced **0 blocks in 30
seconds**, and restarting it resumed production immediately with no intervention
and no state loss.

So: an unattended reboot on one VM halts the testnet until it comes back. That
is what step 6's systemd unit is for — `Restart=always` matters more here than
it would on a larger set. Adding a fourth validator would let the network
survive one failure.

## 7b. Run a price feeder (all 3 VMs)

`x/oracle` is in this genesis, and it does nothing until validators report
prices. The threshold is a share of **stake**, so on a network of three
equally-bonded validators **at least two must report or no rate is ever
agreed** — the module simply sits idle and every validator's miss counter
climbs, with nothing to indicate why.

Verified on the three-validator rehearsal:

| Reporting | Voting power | Result |
| --- | --- | --- |
| 1 of 3 | 3,333 bps | no rate agreed — `no rate has ever been agreed for uyml` |
| 2 of 3 | 6,666 bps | rate agreed, `voting_power_bps: 6666` |

Each validator votes with its own account by default, or delegates to a hot key
so the operator key can stay offline:

```bash
blockchaind tx oracle delegate-feeder <validator-operator-address> <feeder-address> \
  --from validator --chain-id yamale-testnet-1 --keyring-backend file
```

Submitting is one transaction per voting period (12 blocks, about a minute):

```bash
blockchaind tx oracle submit-rates <validator-operator-address> \
  --rates '{"denom":"uyml","rate":"0.4213"}' \
  --rates '{"denom":"uusd","rate":"1.00"}' \
  --from feeder --chain-id yamale-testnet-1 --keyring-backend file
```

The chain does not ship a feeder daemon; wrap that command in whatever reads
your price source. Check it is working with `blockchaind query oracle rates` and
`blockchaind query oracle misses`. See
[the oracle guide](../../docs/guides/oracle.md) for the whole picture.

## 8. (Optional) Run the activity bot

The bot drives visible traffic so a testnet is not an empty chain. Build and
deploy it to one node or a separate small VM that has the `blockchaind` CLI:

```bash
cd bot && go build -o yamale-bot .
```

Create and fund its accounts — this is a script rather than an instruction,
because "make sure the accounts are funded" is the kind of step that gets
skipped:

```bash
./prepare.sh validator          # `validator` = any local key holding uyml
```

Then install the service:

```bash
sudo cp yamale-bot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now yamale-bot
```

`config.yaml`'s default actions — transfers, opening treasuries, valuer
applications — need nothing but funded accounts, so they work on a chain that
has just launched. Swaps and ISO 20022 payments are included but commented out:
each needs something that does not exist on a fresh genesis (a second denom and
a pool; a governance vote approving the participants). Uncomment them once
those exist.

Watch it with `journalctl -u yamale-bot -f`. Every line reports what the
transaction did *in the block*, not what the broadcast returned:

```
[bank-send] from=bot-a to=bot-b ok: 85D3C763…
[paymsg-…] from=bot-a to=bot-b FAILED in block: AF471FF9… code=1104: … is not an approved participant
```

That distinction is the point of the log. A transaction the node accepts into
its mempool can still fail when the block runs it, so a bot reporting the
broadcast result would print `ok` indefinitely while the chain recorded nothing
but failures.

## 9. Upgrading the chain later

A coordinated upgrade is the one procedure that cannot be retried casually:
every validator halts at the same height, and a node that gets it wrong either
diverges or stays down. The mechanism is registered in `app/upgrades.go` and
tested in `app/upgrades_test.go`, which drives the whole sequence below.

**Add the upgrade to the binary.** Append an entry to `upgrades` in
`app/upgrades.go` — a name, any store changes, and optionally a handler. A nil
handler runs the standard module migrations, which is the common case. The name
must never change once proposed: plans are matched by name.

**Propose it.** A `MsgSoftwareUpgrade` naming that upgrade and a height far
enough ahead for every operator to prepare.

**Do not deploy the new binary yet.** This is the mistake worth stating
outright: a node running the new binary before the height stops with

```
BINARY UPDATED BEFORE TRIGGER! UPGRADE "..." - in binary but not executed on
chain. Downgrade your binary
```

That refusal is deliberate — new logic before the coordinated height would
diverge from the rest of the network — and it is verified in the test suite.

**At the height, every node stops.** The running binary does not know the
upgrade, so it halts with `UPGRADE "..." NEEDED at height ...`. This is the
expected, correct outcome, not a failure. The chain is now waiting.

**Swap the binary and restart.** With the new binary in place the upgrade
applies, the module versions are migrated, the plan is cleared so a later
restart does not repeat it, and block production resumes.

Running [cosmovisor](https://docs.cosmos.network/main/build/tooling/cosmovisor)
automates the swap, which is worth setting up before the first upgrade rather
than during it.
