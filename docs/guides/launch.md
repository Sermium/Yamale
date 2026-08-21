# Launching a chain

The whole sequence, in order, from an empty server to a chain producing blocks
with a foundation nobody can spend from alone.

**You need:** a host you can reach, and five people who will each hold one key.

**You will end with:** a running chain whose constitution is fixed at genesis,
whose foundation is a 3-of-5 that no single person controls, and a signed record
of who witnessed it.

---

## The order matters, and here is why

The ceremony comes **before** genesis, not after. That is not a preference.

An `x/group` policy address derives from the group's sequence number and nothing
else — not the members, not the threshold, not the admin, not even the chain id.
So the address is knowable before the chain exists, and it commits to nothing
about who controls it. A genesis that named the address and left the group to be
created afterwards would hand every future seizure to whoever created the first
group policy on the chain.

So: hold the ceremony, put the group **and** the address into genesis together,
and there is no interval to race.

---

## 1. Install

One binary each. Both embed everything they need — the ceremony carries its own
web interface, so there is nothing to serve separately and nothing to install on
the machine a custodian uses.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o blockchaind ./cmd/blockchaind
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ceremony    ./tools/ceremony
```

For a Raspberry Pi or an ARM host, `GOARCH=arm64`. A mismatched binary forks at
the first block, so every validator runs the same build.

```bash
sudo install -m755 blockchaind ceremony /opt/yamale/bin/
```

## 2. Choose which ceremony you are running

Two paths, and the difference is not convenience.

| | Air-gapped | Hosted |
|---|---|---|
| Custodians | one room, one machine | their own devices, anywhere |
| Command | `ceremony serve` or the CLI | `ceremony host` |
| Where keys are made | in the tool, on a machine with no network | in each custodian's browser |
| Who could see a phrase | nobody outside the room | nobody — it never leaves the browser |
| What you trust | the binary, whose hash you checked | the binary **and** the page it served |

**The air-gapped path is stronger** and should be used for a chain that will
hold value. It is stronger for one reason: a hosted page is code a browser
downloaded, and a browser is a large thing to trust with twenty-four words. The
hosted path exists because five institutions in five countries cannot always be
in one room, and a ceremony that cannot happen is worth less than one with a
smaller attack surface on paper.

Both generate the key. Nobody arrives with a phrase; custodians arrive with a
blank pre-printed sheet.

## 3. Run the ceremony

### Hosted

```bash
ceremony host --public-url https://pay.example.com/ceremony/ --out .
```

It binds **loopback only** and expects a proxy in front. The snippet it prints is
the one to install:

```nginx
location /ceremony/ {
    proxy_pass http://127.0.0.1:8787/ceremony/;
    proxy_set_header Host $host;
    # A custodian reads twenty-four words off this page and writes them down. A
    # read timeout in seconds would drop them mid-transcription.
    proxy_read_timeout 3600s;
}
```

It prints two things you need:

- **The bundle SHA-256.** Publish it with the invitation, from somewhere other
  than the page — an email, a phone call. A digest a custodian reads off the
  page it is meant to be checking proves nothing.
- **The coordinator link.** That link *is* the ceremony: anyone holding it can
  issue invites. It is not a link to paste into a group chat.

Then everything is in the browser. Open the coordinator link, enter the roster
and the threshold, and it issues one invite per custodian — each as a link and a
QR code, because nobody types a URL with a token in it correctly. Send each
person theirs. The board then shows, per custodian: invited, opened, generated,
submitted, attested, and names whoever you are still waiting on.

**Do not put this behind a systemd unit with `Restart=always`.** A ceremony holds
its state in memory and a restart loses it. That is the correct trade — nothing
is written that could reconstruct a phrase — but it means an automatic restart
would quietly destroy a ceremony in progress. Run it in a terminal you are
watching, or under `screen`, and stop it when the ceremony ends.

### Air-gapped

```bash
ceremony serve --out .            # a page, on this machine only
ceremony custodian --name "..."   # or the terminal, one custodian at a time
```

`serve` launches the browser itself on a temporary profile it deletes — no
extensions, no autofill, no history, no session to restore. That is the one place
private browsing can be enforced rather than requested.

### What each custodian does

Open the link, generate, write down twenty-four words, confirm, read four back
from the sheet — including the last — then write the fingerprint next to the
phrase and seal the envelope. Then they wait, see the group fingerprint, confirm
by voice that everyone sees the same one, verify their own address is in it, and
sign.

**Run the restore drill during the ceremony**, on a second machine, for at least
one custodian: reconstruct from their paper, confirm the address matches, destroy
that instance. A backup nobody has tested is not a backup, and the day you find
out is the day it is the only copy.

**If a phrase is exposed** — a photograph, a stranger walking in — destroy that
key and generate another. Always. It is written here so nobody has to make that
call under pressure with five senior people waiting.

## 4. The validator's operator key

From the ceremony, not from `keys add`.

```bash
ceremony validator --name "<moniker>" --armor operator.asc
blockchaind keys import validator operator.asc --keyring-backend file
```

Never the `test` backend. It writes the key unencrypted, which is what it is for.

**This is a different key from the consensus key.** `blockchaind init` already
generated that one into `priv_validator_key.json` and it must never leave the
host — a consensus key that has been anywhere else is one that might sign two
blocks at one height, which is the offence that gets a validator slashed for
certain. `ceremony consensus` exists only to explain why the tool refuses to
generate one.

The paper is the backup. The encrypted keystore is the working copy, because
unlike a custodian key an operator key gets used again — to edit a commission,
to unjail.

## 5. Build genesis

One validator, all in one pass:

```bash
CEREMONY_DIR=/path/to/ceremony OPERATOR_PASSPHRASE=... ./scripts/devnet/init-devnet.sh
```

More than one, in two phases — because a joining validator signs its own gentx
on its own host, which is what makes it a second validator rather than a second
process the first host happens to control:

```bash
# 1. On the coordinating host. Fund every joining operator here: a
#    self-delegation needs a balance already in the genesis, and a joining
#    operator whose account is absent gets a gentx refused for having nothing to
#    bond — which reads as a problem with their key rather than with the file
#    they were sent.
PHASE=accounts JOINING_ACCOUNTS="yml1... yml1..."   CEREMONY_DIR=/path/to/ceremony OPERATOR_PASSPHRASE=...   ./scripts/devnet/init-devnet.sh
# it stops, and prints the genesis sha256

# 2. On each joining host: put that genesis at <home>/config/genesis.json,
#    COMPARE THE HASH, then
blockchaind genesis gentx <key> <minority-stake> --chain-id <id>   --moniker <moniker> --keyring-backend file
#    and send the gentx-*.json back

# 3. On the coordinating host, with every gentx in config/gentx/
PHASE=finalise CEREMONY_DIR=/path/to/ceremony OPERATOR_PASSPHRASE=...   ./scripts/devnet/init-devnet.sh
```

**Keep joining stakes a minority — and know what that buys.** Two thirds of two
equal validators is both of them, so an equal pair halts whenever either drops.
At 10000 against 100000 the first validator alone stays above the threshold and
keeps producing blocks through the other's outages.

What it does not buy is tolerance for the *first* validator's outage. A chain
survives losing a node only if the ones left hold more than two thirds, so every
validator has to hold less than a third — and no pair can. With this split the
majority node is a single point of failure, which is correct for a rehearsal and
must not be mistaken for redundancy. It has already halted `yamale-devnet-2`
once, for two hours, when the cloud host went away.

**Four equal validators is the minimum that tolerates one loss**: each holds 25%,
any three hold 75%. Seat four before anything depends on uptime.

**`sudo -E` may not carry the variables.** sudo resets the environment and `-E`
only works if the sudoers policy allows it — the symptom is a phase flag being
ignored and the script running straight through to the end. Put them on sudo's
own command line instead: `sudo PHASE=accounts CEREMONY_DIR=... bash ...`.

It refuses without the ceremony's output, and refuses without an operator key.
Both refusals are the point: a key created to get past them is a key with no
paper behind it, and the chain would launch anyway.

What it writes into genesis:

- the **group** itself, so address and membership are fixed by one file;
- the **constitutional invariants** — the concentration ceilings, the seizure
  threshold, the recovery destination, the delays — which the chain then refuses
  to let governance edit;
- `enforcement.recovery_destination`, pointing at the group's policy address.

Five ways it will stop you, all of them earned:

- **A missing invariant.** The ceremony supplies three of the thirteen — the ones
  it can derive from the group it built. The other ten are policy no key
  determines, and genesis has to state them.
- **A ceiling nothing could satisfy.** With a floor of two validators, one holds
  5000 basis points by arithmetic, so a 3400 ceiling is refused outright rather
  than accepted and left permanently in breach.
- **Parameters disagreeing with the constitution.** Three enforcement parameters
  duplicate invariants, and a genesis where the two differ is refused — better
  than a chain whose constitution and whose module hold different numbers.
- **No operator key.** It refuses rather than running `keys add`, because a key
  created to get past that line is a key with no paper behind it and the chain
  would launch anyway.
- **No ceremony output.** `CEREMONY_DIR` has no default and there is no fallback
  to a local foundation key. A single key receiving every seized asset is the
  arrangement this replaces, and a script that quietly substituted one would
  restore it on the next reset without anybody deciding to.

## 6. Start, and check what you actually built

```bash
blockchaind start --minimum-gas-prices 0.0001uyml
blockchaind query constitution invariants
blockchaind query group group-members 1
```

Then the check nothing else covers: **have three custodians move something, and
two fail to.** A foundation that cannot spend is discovered at the worst possible
moment, and every other step in this runbook can pass without exercising it.

---

## Things worth knowing

**The chain id is part of every signature.** Change it and every client that
still names the old one keeps signing for a chain that does not exist, failing as
a signature error rather than a configuration one. It appears in the clients, the
scripts, the service units and the guides — change all of them.

**Demo keys and control keys are not the same thing.** The devnet's alice, bob and
foundation keys live unencrypted in a `test` keyring and are regenerated on every
reset, which is what they are for. What controls the chain — the foundation group,
the validator's operator key — gets a ceremony. Do not let the two meet.

**Stop the chain before rebuilding its genesis.** `rm -rf` on a running node's
home does not stop it: Linux keeps the process's open handles after the
directory is unlinked, so the old chain carries on, `systemctl start` afterwards
is a no-op because the unit is already active, and the new genesis is never read
— a genesis is only consulted at height zero. The symptom is a chain reporting
the new chain id and the old state, both internally consistent. If you find
yourself there, `blockchaind comet unsafe-reset-all` clears the data and keeps
the config and keys. The script now refuses to run while the unit is up.

**A halted node refuses every query, rather than serving stale state.** Every
Cosmos query is answered against the state left by the last block the node
finalised, so a node that is up but has finalised nothing — the chain stopped and
the node restarted — answers *every* REST and gRPC call with
`invalid height: context did not contain latest block height ... (2733)`. The
height in the parentheses is the last committed block, and asking for it
explicitly works perfectly:

```bash
curl -H "x-cosmos-block-height: 2733" localhost:1317/cosmos/group/v1/group_members/1
```

Worth knowing before diagnosing anything during an outage, because a node
refusing all queries reads like a broken node rather than a stopped chain. The
foundation console does this automatically and says which height it is showing.

**The node home is owned by whoever ran the script.** Running the init under
`sudo` leaves it root-owned while the service runs as an unprivileged user, and
the node then dies on `client.toml: permission denied` — which names a file
rather than an ownership problem. `chown` the home to the service user after
building the genesis.

**A one-validator chain cannot bound concentration.** It holds every basis point,
so the ceilings have to be 10000, which is no ceiling. That is honest rather than
decorative; `scripts/devnet/concentration-demo.sh` stands up four validators and
watches one get demoted, which is where the mechanism is actually demonstrated.

**Full reference:** [The key ceremony](key-ceremony.md) for the ceremony itself,
[Run a validator](validator.md) for joining an existing chain.
