# Resetting the devnet

Two scripts, in order. They exist because adding a module is consensus-breaking,
so every new module means a fresh genesis — and that has happened three times
already, each time reassembled by hand from memory, each time missing something.

```bash
bash scripts/devnet/init-devnet.sh   # wipes and rebuilds genesis, then configure
sudo systemctl start yamale-devnet
bash scripts/devnet/populate.sh      # currencies, balances, pools, faucet float
sudo systemctl start yamale-faucet yamale-bot
```

## What each one does, and why it is in there

**init-devnet.sh** — `init`, three keys, funded genesis accounts, the 42 African
currencies seeded, alice as the validator, and the API opened on localhost.

It also **shortens the governance voting period to three minutes**. A 48-hour
period makes every governance path untestable inside a working session, which is
why registering a custody asset, approving a stablecoin issuer and admitting a
validator have only ever been exercised in unit tests. On a devnet the point is
to watch the permission gate work, not to model the economics.

It also sets the **constitution invariants** a one-machine devnet can satisfy,
and it has to, before `gentx`. Two of them are relaxations — `min_active_validators`
down to 1, and the concentration caps up to 10000 basis points, because one
validator holds all the voting power by definition and any lower cap makes a
single-node set arithmetically impossible. The third is not a relaxation but a
duplicate: x/constitution keeps its own copy of the enforcement recovery
destination and checks that it is populated, so setting only the x/enforcement
parameter leaves `gentx` refusing genesis with a message naming the parameter
that *is* set. That is a confusing half-hour, once per person.

It also seeds **payments**: the foundation as an approved participant, alice and
bob as its registered customers, and a country recorded against each of the
three accounts.

That last part is what made x/paymsg reachable at all. Approval is gov-gated, so
before this the devnet had no approved participant — and every `MsgSendPayment`
names an instructing participant the module refuses unless it has been admitted.
The visible symptom was not a failure: it was that no interface ever sent a
payment. The Pay app and the wallet both sent plain bank transfers, so the
participant gating, the end-to-end id, the metadata hash and the settlement
jurisdiction went untested outside unit tests.

**What this still does not cover.** Accounts created *in the Pay app* have their
keys generated in the browser, so their addresses do not exist when genesis is
written and they are nobody's customer. The Pay screen refuses their payments,
correctly and in those words — "this account is not registered with a bank".
Making them payable needs something holding the participant's key at sign-up,
which is the same moment the faucet already funds them; there is no such step
today, and a demonstration of the payment path has to use alice or bob.

**populate.sh** — bot accounts, minted currencies, a treasury, **the two AMM
pools**, and **the faucet's own float**.

The pools and the float were separate manual steps for three resets running. The
activity bot broke twice with `pool 1 not found`, which reads as a chain fault
and is actually a missing setup step. Anything the running system needs belongs
in the script that builds it.

## The ops signing service

`opsd.py` is what makes the governance console more than a read-only page: it
holds a validator's keyring and signs votes, freezes and recovery cases on
request. `deploy-opsd.sh` puts it on a host.

```bash
bash scripts/devnet/deploy-opsd.sh pi    # do the Pi first — the VM reads its credential
bash scripts/devnet/deploy-opsd.sh vm    # service, then the nginx that fronts both
```

Order matters exactly once: the `vm` target rewrites
`/etc/nginx/snippets/yamale-ops.conf`, and to do that it needs the service
credential the Pi generated.

**There is one copy of `opsd.py`, and it is the one in this directory.** Each
host used to have its own, edited in place over ssh, and they drifted: the VM's
map sent `pi-2` to `/opt/yamale/node` while pival's key has only ever lived on
the Pi at `/opt/yamale/join-node`. The Pi still has a `/opt/yamale/node` from an
earlier devnet with a *different* `alice` in it, so the mismatch did not fail
loudly — it signed with the wrong key. Everything that differs between the two
hosts is now systemd environment, set by the deploy script:

| | validator | binds | signs with |
|---|---|---|---|
| VM  | `pi`   | `127.0.0.1` | `alice` in `/opt/yamale/node` |
| Pi  | `pi-2` | `100.68.207.17` (tailnet) | `pival` in `/opt/yamale/join-node` |

`OPSD_VALIDATOR` is what stops a host signing as the other one. Both hosts carry
the same map, but a host holds one key, so the Pi refuses `validator=pi`
outright rather than going looking for an `alice` it should not be using.

### Why the bind address is a security control, not a detail

The Pi's copy was bound to `0.0.0.0:8086`. nginx is on the VM, so on the Pi
there was nothing in front of that port at all — anything on the house LAN could
reach the signer directly and cast a governance vote, freeze a wallet or open a
seizure case with the validator's key, with no credential of any kind. The
service now refuses to start on a wildcard address, and refuses to start on any
non-loopback address unless `OPSD_AUTH` is set. Both nginx locations replace the
operator's console password with that service credential, so the question nginx
answers (*which operator is this, and log it*) stays separate from the question
opsd answers (*is this nginx at all*).

### Reporting execution, not acceptance

`run()` broadcasts, keeps the hash, then polls `query tx <hash>` until the block
has it, and reports the code the block produced. A `code: 0` from broadcast only
means the mempool accepted the transaction. Reporting that as success is the
same bug `tools/faucet/main.go` had — a freeze that failed in the block came
back to the console as "sent". Requests therefore take a block to answer, which
is what `proxy_read_timeout 70s` in the nginx snippet is for.

## What is deliberately not here

No `unsafe-reset-all` shortcut. `init-devnet.sh` removes the home directory
outright, because a partial reset that keeps old state alongside a new module
set is exactly the mismatch that crash-looped the node with
`status=2/INVALIDARGUMENT` — a failure worth having loudly rather than
subtly.
