#!/bin/bash
# Stand up a single-node Yamale devnet on a developer machine, with a real
# 3-of-5 foundation, ready for a country-enrolment end-to-end test.
#
# This is the local sibling of scripts/devnet/init-devnet.sh. That one builds
# the Raspberry Pi demo host: systemd units, an nginx-facing API, a validator
# operator key that arrives on paper from a ceremony room, and python3 for the
# genesis surgery. None of that is available or wanted here. What is the same,
# and is the whole reason this script exists rather than a `blockchaind init`
# in a README, is the shape of the institution: the foundation is a 3-of-5
# x/group policy account seeded at height zero, and the constitution the chain
# refuses to start without agrees with it.
#
# What you get:
#
#   * a chain producing one-second blocks on non-default ports
#   * a foundation group policy address that really needs three signatures
#   * all five custodian keys in the node's `test` keyring, so a test can BE
#     three of the five rather than mocking them
#   * a complete app_state.constitution.invariants, and enforcement params that
#     agree with it
#   * the foundation named in alias.params.foundation_administrators, so it can
#     place its own offices in a country that has no participant yet
#   * gov voting periods short enough to exercise a proposal inside a test
#
# Usage:
#
#   bash scripts/devnet/init-local-devnet.sh
#   PROVE_FOUNDATION=1 bash scripts/devnet/init-local-devnet.sh
#
# Everything below is overridable by environment variable, and the defaults are
# chosen so that two people on one machine, or this and concentration-demo.sh,
# do not collide.
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$REPO"

# ---------------------------------------------------------------------------
# Where everything lives.
#
# Under .devnet/ because that path is gitignored, but in a subdirectory of it
# rather than at its root. concentration-demo.sh clears ./.devnet wholesale on
# startup; netting-demo.sh keeps its chain in .devnet-netting for exactly that
# reason. This script only ever deletes its own subtree, so running it cannot
# take somebody else's chain down with it.
# ---------------------------------------------------------------------------
ROOT=${ROOT:-$REPO/.devnet}
LOCAL=${LOCAL:-$ROOT/local}
HOME_DIR=${HOME_DIR:-$LOCAL/node}
CEREMONY_DIR=${CEREMONY_DIR:-$LOCAL/ceremony}
CHAIN_ID=${CHAIN_ID:-yamale-local-1}
MONIKER=${MONIKER:-local}
DENOM=${DENOM:-uyml}

# Non-default ports, all of them.
#
# Two chains on 26657 produced transactions that succeeded and then reported
# "account not found", because the CLI was talking to one node about a
# transaction the other had accepted — see the note at the top of
# concentration-demo.sh. proxy_app is in that list: it is the ABCI socket, it
# defaults to 26658, and a second node that binds it talks to the first node's
# application.
RPC_PORT=${RPC_PORT:-26957}
P2P_PORT=${P2P_PORT:-26956}
PROXY_PORT=${PROXY_PORT:-26955}
API_PORT=${API_PORT:-1517}
GRPC_PORT=${GRPC_PORT:-9290}
PPROF_PORT=${PPROF_PORT:-6260}

NODE_RPC="tcp://127.0.0.1:$RPC_PORT"
API_URL="http://127.0.0.1:$API_PORT"

# The passphrase the ceremony harness encrypts its armored keys with. It is a
# constant in tools/ceremony/live_test.go, not a secret: these are throwaway
# keys generated seconds ago for a chain that will be deleted.
CEREMONY_PASSPHRASE=${CEREMONY_PASSPHRASE:-ceremony-rehearsal-passphrase}

# The five, in the order live_test.go generates them. custodian-N.asc is the
# Nth name in this list; custodian-<slug>.json is its public record. The group
# members are sorted by address rather than by name, so these two orderings
# differ and the mapping has to be read from the files rather than assumed.
CUSTODIAN_NAMES=("Amara Okafor" "Bernard Kouassi" "Chipo Mwale" "Dalia Haddad" "Eshe Njoroge")
CUSTODIAN_KEYS=(custodian1 custodian2 custodian3 custodian4 custodian5)

KR="--keyring-backend test --home $HOME_DIR"
Q="--home $HOME_DIR --node $NODE_RPC -o json"
TX="--home $HOME_DIR --node $NODE_RPC --keyring-backend test --chain-id $CHAIN_ID --gas 900000 --fees 0$DENOM -y -o json"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
say "=== preflight ==="

command -v go >/dev/null 2>&1 || die "go is not on PATH. On Windows: export PATH=\"/c/Program Files/Go/bin:\$PATH\""
command -v node >/dev/null 2>&1 || die "node is not on PATH. The genesis surgery below is nested JSON; sed cannot write it and python3 is not on this machine."
: "${GOTMPDIR:=$REPO/.gotmp}"
export GOTMPDIR
mkdir -p "$GOTMPDIR"

# Refuse rather than clobber.
#
# `rm -rf` on a running node's home does not stop it: the process keeps its open
# handles, carries on writing, and the new genesis is never read because a
# genesis file is only consulted at height zero. The symptom is a chain that
# reports the new chain id and the old state, and both halves are internally
# consistent so nothing looks wrong. Checked rather than killed automatically —
# stopping somebody's chain because a script wanted a clean directory is not a
# decision a script should take.
port_busy() {
  netstat -an -p tcp 2>/dev/null | grep -i listening | grep -qE "[:.]$1[[:space:]]"
}
for port in "$RPC_PORT" "$P2P_PORT" "$PROXY_PORT" "$API_PORT" "$GRPC_PORT"; do
  if port_busy "$port"; then
    say "" >&2
    say "  Something is already listening on $port." >&2
    say "" >&2
    say "  If it is a previous run of this script, stop it first:" >&2
    say "    cat $LOCAL/node.pid   # then: kill that pid" >&2
    say "" >&2
    say "  Deleting the home under a running node does not reset it — the process" >&2
    say "  keeps its open files and the new genesis is never read." >&2
    die "port $port is in use; refusing to build a genesis a running node will ignore"
  fi
done
say "  ports $RPC_PORT/$P2P_PORT/$PROXY_PORT/$API_PORT/$GRPC_PORT are free"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
say "=== build ==="
if [ -n "${BIN:-}" ]; then
  say "  using BIN=$BIN as given"
else
  BIN=$REPO/blockchaind.exe
  [ -x "$BIN" ] || BIN=$REPO/blockchaind
  go build -o "$BIN" ./cmd/blockchaind
  say "  $BIN"
fi
[ -x "$BIN" ] || die "$BIN is not executable"

# The ceremony as a test binary rather than `go test`.
#
# Windows application control refuses to execute anything Go builds into the
# user temp directory, which is where `go test` puts its binary — so it is built
# to an explicit path here and run from there. The failure it avoids says "Une
# stratégie de contrôle d'application a bloqué ce fichier" and is not a test
# failure, which is a confusing thing to debug at the point it appears.
#
# CEREMONY_TEST_BIN_PREBUILT=1 skips the compile and uses CEREMONY_TEST_BIN as
# it stands. That is for one situation only: somebody else is mid-edit in
# tools/ceremony and it does not compile, but a binary from before their edit is
# lying around and the devnet is not what is being changed. It is a way past
# somebody else's half-finished work, not a way past your own — a stale binary
# builds a genesis that disagrees with the source beside it.
CEREMONY_TEST_BIN=${CEREMONY_TEST_BIN:-$REPO/.gotmp/ceremony.test.exe}
if [ -z "${CEREMONY_TEST_BIN_PREBUILT:-}" ]; then
  go test -c -o "$CEREMONY_TEST_BIN" ./tools/ceremony
  say "  $CEREMONY_TEST_BIN"
else
  say "  $CEREMONY_TEST_BIN (prebuilt, not recompiled)"
fi

# ---------------------------------------------------------------------------
# Wipe
# ---------------------------------------------------------------------------
say "=== clearing $LOCAL ==="
rm -rf "$LOCAL"
mkdir -p "$CEREMONY_DIR"

# ---------------------------------------------------------------------------
# The ceremony
#
# The foundation is a 3-of-5 x/group policy account, not a key on this machine.
# A single key that receives every seized asset on the chain is the arrangement
# the whole constitutional layer exists to end, and a devnet that quietly
# substituted one would let a test pass against a shape production does not
# have.
#
# TestFiveCustodianCeremony is the tool's own harness: five custodians, the
# transcription check on each, the group assembled by the real buildGroup. The
# one thing it does that a paper ceremony never would is export each custodian's
# private key as armor — and that is precisely what makes this devnet useful,
# because three of the five have to be able to sign a vote.
# ---------------------------------------------------------------------------
say "=== ceremony: five custodians, 3-of-5 ==="
( cd "$REPO/tools/ceremony" && CEREMONY_LIVE_DIR="$CEREMONY_DIR" "$CEREMONY_TEST_BIN" \
    -test.run TestFiveCustodianCeremony -test.v ) 2>&1 | sed -n 's/^ *live_test.go:[0-9]*: /  /p'

[ -f "$CEREMONY_DIR/policy-address" ] || die "the ceremony did not write policy-address"
POLICY=$(tr -d '\r\n' < "$CEREMONY_DIR/policy-address")
[ -n "$POLICY" ] || die "policy-address is empty"
say "  foundation group policy: $POLICY"

# ---------------------------------------------------------------------------
# Init
# ---------------------------------------------------------------------------
say "=== init ==="
$BIN init "$MONIKER" --chain-id "$CHAIN_ID" --default-denom "$DENOM" --home "$HOME_DIR" >/dev/null 2>&1
G=$HOME_DIR/config/genesis.json
say "  $CHAIN_ID at $HOME_DIR"

# ---------------------------------------------------------------------------
# Keys
#
# Two kinds, and the difference matters even here.
#
# The five custodians are IMPORTED from the ceremony's armor. Their addresses
# are already fixed in the group inside genesis, so a key generated here would
# not be a member of anything.
#
# validator, operator, bankco and customer are generated, disposable, and live
# unencrypted in the `test` keyring. On the Pi the validator operator key comes
# from a ceremony too and goes into the `file` backend under a passphrase,
# because that key signs every block on a host that is reachable from the
# internet. This one signs blocks on a laptop for a chain that gets deleted, so
# it is `keys add` — the deviation is deliberate and is the only one.
# ---------------------------------------------------------------------------
say "=== keys: importing the five custodians ==="
for i in 0 1 2 3 4; do
  name=${CUSTODIAN_KEYS[$i]}
  armor=$CEREMONY_DIR/custodian-$((i + 1)).asc
  [ -f "$armor" ] || die "no $armor — the ceremony did not export custodian $((i + 1))"
  # One prompt, not three: the `test` backend has no keyring passphrase of its
  # own, so only the armor's decryption passphrase is read. Piping three (which
  # is what the `file` backend needs) leaves import consuming the extra lines.
  echo "$CEREMONY_PASSPHRASE" | $BIN keys import "$name" "$armor" $KR >/dev/null
done

# Verify each import against the ceremony's public record before trusting any of
# it. An armor file that decrypted to the wrong key would still import cleanly
# and would fail much later, as a vote from an address the group has never heard
# of — which reads as a bug in x/group.
declare -a CUSTODIAN_ADDRS
for i in 0 1 2 3 4; do
  name=${CUSTODIAN_KEYS[$i]}
  full=${CUSTODIAN_NAMES[$i]}
  slug=$(printf '%s' "$full" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')
  record=$CEREMONY_DIR/custodian-$slug.json
  [ -f "$record" ] || die "no public record at $record"
  expected=$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).address)' "$record")
  actual=$($BIN keys show "$name" -a $KR)
  [ "$expected" = "$actual" ] || die "$name imported as $actual but $record says $expected"
  CUSTODIAN_ADDRS[$i]=$actual
  printf '  %-12s %-46s %s\n' "$name" "$actual" "$full"
done

say "=== keys: validator and the ordinary accounts ==="
for k in validator operator bankco customer; do
  $BIN keys add "$k" $KR >/dev/null 2>&1
done
VALIDATOR=$($BIN keys show validator -a $KR)
OPERATOR=$($BIN keys show operator -a $KR)
BANKCO=$($BIN keys show bankco -a $KR)
CUSTOMER=$($BIN keys show customer -a $KR)
printf '  %-12s %s\n' validator "$VALIDATOR" operator "$OPERATOR" bankco "$BANKCO" customer "$CUSTOMER"

# ---------------------------------------------------------------------------
# Genesis accounts
# ---------------------------------------------------------------------------
say "=== genesis accounts ==="
$BIN genesis add-genesis-account "$VALIDATOR" 500000000000$DENOM --home "$HOME_DIR"
$BIN genesis add-genesis-account "$OPERATOR"  200000000000$DENOM --home "$HOME_DIR"
$BIN genesis add-genesis-account "$BANKCO"    100000000000$DENOM --home "$HOME_DIR"
$BIN genesis add-genesis-account "$CUSTOMER"  100000000000$DENOM --home "$HOME_DIR"
# The group policy address needs an auth account, and importing a group from
# genesis does not create one the way the runtime path does. Without it the
# first transfer into the account fails on an account that does not exist, and
# the foundation cannot pay a fee for its own proposals. It is unspendable
# except through the group's own policy — the address is a hash of a module name
# and a sequence number, not of any public key — so funding it is safe.
$BIN genesis add-genesis-account "$POLICY"    100000000000$DENOM --home "$HOME_DIR"
# Custodians get a float of their own, because a vote is a transaction and a
# custodian who cannot pay for one is a custodian who cannot vote.
for addr in "${CUSTODIAN_ADDRS[@]}"; do
  $BIN genesis add-genesis-account "$addr" 10000000000$DENOM --home "$HOME_DIR"
done
say "  validator, operator, bankco, customer, the policy and five custodians funded"

# ---------------------------------------------------------------------------
# The settlement
#
# Written by node, not python3, and not by sed. The tiers and the rolling cap
# are nested JSON that sed cannot write, and a fallback that half applied would
# produce a genesis the chain refuses to start from — after the script had
# reported success.
# ---------------------------------------------------------------------------
say "=== the settlement, the group and the parameters ==="
EDIT=$LOCAL/genesis-edit.mjs
cat > "$EDIT" <<'JS'
import { readFileSync, writeFileSync } from "node:fs";

const [genesisPath, ceremonyDir, policy] = process.argv.slice(2);
const read = (p) => JSON.parse(readFileSync(p, "utf8"));
const g = read(genesisPath);

// -- the group, at height zero -------------------------------------------
//
// Not created afterwards by a transaction, and this is the whole reason the
// ceremony runs before genesis rather than after. An x/group policy address
// derives from the group sequence number alone — not from the members, the
// threshold, the admin or the chain id — so the address is knowable offline
// but commits to nothing about who controls it. A genesis that named the
// address and left the group to be created later would hand every future
// seizure to whoever created the first group policy on the chain.
g.app_state.group = read(`${ceremonyDir}/group-genesis.json`);

// Sanity, because the whole devnet is worthless if this is wrong: the group in
// the genesis must be the 3-of-5 at the address everything else points at.
const pol = g.app_state.group.group_policies[0];
if (pol.address !== policy) {
  throw new Error(`group-genesis policy ${pol.address} != policy-address ${policy}`);
}
if (pol.decision_policy.threshold !== "3") {
  throw new Error(`decision policy threshold is ${pol.decision_policy.threshold}, expected 3`);
}
if (g.app_state.group.group_members.length !== 5) {
  throw new Error(`group has ${g.app_state.group.group_members.length} members, expected 5`);
}

// -- the invariants ------------------------------------------------------
//
// Validate() refuses a missing field, so every one is stated. The three the
// ceremony determines come from the group it just built; the other ten are
// policy that nobody's key decides.
const inv = {
  // 10000, which is no ceiling at all, and that is the honest value on a chain
  // with one validator. It holds every basis point, so the chain refuses any
  // tighter ceiling rather than starting and reporting a permanent breach.
  // With N equal validators the floor is 10000/N; the caps are demonstrated
  // properly by scripts/devnet/concentration-demo.sh, which stands up four.
  max_entity_power_bps: "10000",
  max_beneficial_owner_power_bps: "10000",
  max_jurisdiction_power_bps: "10000",
  concentration_epoch_blocks: "120",
  // One, because there is one. Validate() also refuses a cap below one seat's
  // worth of power out of this number, which is why the three above are 10000.
  min_active_validators: 1,
  enforcement_threshold_bps: "6667",
  enforcement_recovery_destination: policy,
  enforcement_voting_period_blocks: "360",
  // At least the voting period, so the vote always ends before the freeze
  // lapses. The expiry queue underneath is the backstop, not the mechanism.
  enforcement_provisional_freeze_blocks: "720",
  // Seven days of five-second blocks is MinAmendmentDelayBlocks, a floor
  // compiled into the binary that a genesis cannot go under. A shorter value
  // here is not a shorter delay, it is a chain that will not start.
  amendment_delay_blocks: "120960",
  // Must exceed the seizure threshold. A chain where amending the constitution
  // is easier than acting under it has the wrong thing hard.
  amendment_threshold_bps: "8000",
  foundation_custodian_count: 5,
  foundation_signature_threshold: 3,
};
g.app_state.constitution.invariants = inv;

// -- enforcement --------------------------------------------------------
//
// Four of these duplicate constitutional invariants and AssertConstitutional
// refuses a genesis where the two disagree, so they are written from the same
// values rather than left to drift.
const p = g.app_state.enforcement.params;
p.recovery_destination = inv.enforcement_recovery_destination;
p.threshold_bps = inv.enforcement_threshold_bps;
p.voting_period_blocks = inv.enforcement_voting_period_blocks;
p.provisional_freeze_blocks = inv.enforcement_provisional_freeze_blocks;
// Blocks are one second here, so these are minutes rather than the Pi's hours:
// a seizure can be watched through its hold inside a test run.
p.seizure_delay_blocks = "60";
p.seizure_delay_tiers = [
  { threshold: { denom: "uyml", amount: "1000000" }, delay_blocks: "180" },
  { threshold: { denom: "uyml", amount: "100000000" }, delay_blocks: "720" },
];
p.seizure_window_blocks = "17280";
p.seizure_window_cap = [{ denom: "uyml", amount: "500000000" }];
p.max_seizures_per_window = "5";
// No ombudsman. An unappointed office means nobody, never anybody, and
// appointing one on a single-operator devnet would be theatre.

// -- alias: the foundation as an administrator --------------------------
//
// Not in the Pi script, and needed here. MsgSetJurisdiction accepts a
// foundation administrator or governance as the recorder for an account no
// participant onboarded, and a country-enrolment ceremony starts with the
// foundation placing its own offices — before any participant exists that
// could have onboarded them. Their aliases carry ZZ, which ISO 3166-1 reserves
// permanently, so the marker cannot be mistaken for a country.
//
// Params.Validate() caps the list at MaxFoundationAdministrators (8) and
// refuses duplicates and empties. One address, non-empty, well under the cap.
g.app_state.alias.params.foundation_administrators = [policy];

// -- gov ----------------------------------------------------------------
//
// A 48-hour voting period makes every governance path untestable, which is why
// the custody asset registration and stablecoin issuer approval flows have only
// ever been exercised in unit tests. Twenty seconds, because on a one-validator
// chain a proposal needs one deposit and one vote and there is nobody to wait
// for. The expedited period must be strictly shorter than the ordinary one or
// gov's own genesis validation refuses the file.
const gov = g.app_state.gov.params;
gov.voting_period = "20s";
gov.expedited_voting_period = "10s";
gov.max_deposit_period = "20s";
gov.min_deposit = [{ denom: "uyml", amount: "1000" }];
gov.expedited_min_deposit = [{ denom: "uyml", amount: "2000" }];

writeFileSync(genesisPath, JSON.stringify(g, null, 2) + "\n");
console.log(`  recovery destination: ${p.recovery_destination}`);
console.log(`  group 1: 3-of-5 at ${pol.address}, ${g.app_state.group.group_members.length} members`);
console.log(`  alias foundation administrators: ${g.app_state.alias.params.foundation_administrators.join(", ")}`);
console.log(`  gov: voting ${gov.voting_period}, expedited ${gov.expedited_voting_period}, deposit ${gov.min_deposit[0].amount}${gov.min_deposit[0].denom}`);
JS
node "$EDIT" "$G" "$CEREMONY_DIR" "$POLICY"

# ---------------------------------------------------------------------------
# The validator
# ---------------------------------------------------------------------------
say "=== validator ==="
$BIN genesis gentx validator 100000000000$DENOM \
  --chain-id "$CHAIN_ID" --moniker "$MONIKER" $KR 2>&1 | tail -1
$BIN genesis collect-gentxs --home "$HOME_DIR" 2>&1 | tail -1
$BIN genesis validate-genesis --home "$HOME_DIR"

# ---------------------------------------------------------------------------
# Ports and the node config
#
# sed is fine here and only here: these are flat TOML keys, one per line, not
# nested JSON. Each substitution is scoped to its own section where the key name
# repeats across sections — `address` appears under [api], [grpc] and
# [grpc-web], and an unscoped substitution would rewrite all three.
# ---------------------------------------------------------------------------
say "=== ports and config ==="
CFG=$HOME_DIR/config/config.toml
APP=$HOME_DIR/config/app.toml

sed -i "s|^proxy_app = .*|proxy_app = \"tcp://127.0.0.1:$PROXY_PORT\"|" "$CFG"
sed -i "/^\[rpc\]/,/^\[/{s|^laddr = .*|laddr = \"tcp://127.0.0.1:$RPC_PORT\"|}" "$CFG"
sed -i "/^\[p2p\]/,/^\[/{s|^laddr = .*|laddr = \"tcp://0.0.0.0:$P2P_PORT\"|}" "$CFG"
sed -i "s|^pprof_laddr = .*|pprof_laddr = \"localhost:$PPROF_PORT\"|" "$CFG"
# One-second blocks. The whole point of a local devnet is that a flow measured
# in blocks — a freeze, a seizure delay, a proposal window — completes while
# somebody is still looking at it.
sed -i "/^\[consensus\]/,/^\[/{s|^timeout_commit = .*|timeout_commit = \"1s\"|}" "$CFG"

# The node refuses to start without a minimum gas price and a devnet wants it
# free.
sed -i "s|^minimum-gas-prices = .*|minimum-gas-prices = \"0$DENOM\"|" "$APP"
sed -i "/^\[api\]/,/^\[/{s|^enable = .*|enable = true|}" "$APP"
sed -i "/^\[api\]/,/^\[/{s|^swagger = .*|swagger = true|}" "$APP"
sed -i "/^\[api\]/,/^\[/{s|^address = .*|address = \"tcp://127.0.0.1:$API_PORT\"|}" "$APP"
sed -i "/^\[grpc\]/,/^\[/{s|^address = .*|address = \"127.0.0.1:$GRPC_PORT\"|}" "$APP"

printf '  rpc %s   p2p %s   proxy %s   api %s   grpc %s   pprof %s\n' \
  "$RPC_PORT" "$P2P_PORT" "$PROXY_PORT" "$API_PORT" "$GRPC_PORT" "$PPROF_PORT"
say "  timeout_commit $(sed -n '/^\[consensus\]/,/^\[/{/^timeout_commit/p}' "$CFG" | head -1 | cut -d'"' -f2)"

# ---------------------------------------------------------------------------
# Start
# ---------------------------------------------------------------------------
say "=== starting ==="
LOG=$LOCAL/node.log
PIDFILE=$LOCAL/node.pid
nohup "$BIN" start --home "$HOME_DIR" --minimum-gas-prices "0$DENOM" >"$LOG" 2>&1 &
NODE_PID=$!
echo "$NODE_PID" > "$PIDFILE"

height=0
for _ in $(seq 1 90); do
  height=$($BIN status --node "$NODE_RPC" 2>/dev/null \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{process.stdout.write(String(JSON.parse(s).sync_info.latest_block_height))}catch(e){process.stdout.write("0")}})' 2>/dev/null || echo 0)
  case "$height" in ''|*[!0-9]*) height=0 ;; esac
  [ "$height" -ge 2 ] && break
  sleep 1
done
if [ "${height:-0}" -lt 2 ]; then
  say "" >&2
  say "the node did not reach block 2. Last 30 lines of $LOG:" >&2
  tail -30 "$LOG" >&2
  die "chain did not start"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
say ""
say "================================================================"
say "  LOCAL DEVNET READY — height $height"
say "================================================================"
say ""
say "  chain id            $CHAIN_ID"
say "  home                $HOME_DIR"
say "  rpc                 $NODE_RPC"
say "  api                 $API_URL   (swagger at $API_URL/swagger/)"
say "  grpc                127.0.0.1:$GRPC_PORT"
say "  log                 $LOG"
say "  pid                 $NODE_PID   (also in $PIDFILE)"
say ""
say "  foundation group    group 1, 3-of-5"
say "  policy address      $POLICY"
for i in 0 1 2 3 4; do
  printf '    %-12s %-46s %s\n' "${CUSTODIAN_KEYS[$i]}" "${CUSTODIAN_ADDRS[$i]}" "${CUSTODIAN_NAMES[$i]}"
done
say ""
printf '  %-19s %s\n' validator "$VALIDATOR" operator "$OPERATOR" bankco "$BANKCO" customer "$CUSTOMER"
say ""
say "  keyring             --keyring-backend test --home $HOME_DIR"
say "  query              $BIN query <module> <...> --node $NODE_RPC"
say "  stop               kill $NODE_PID"
say ""

# ---------------------------------------------------------------------------
# Read-only verification
#
# Run every time, because a devnet that reports READY and has an empty
# constitution is worse than one that failed: an empty page renders correctly no
# matter what is broken behind it.
# ---------------------------------------------------------------------------
say "=== verification ==="
fail=0
check() {
  local label=$1 want=$2 got=$3
  if [ "$got" = "$want" ]; then
    printf '  ok    %-42s %s\n' "$label" "$got"
  else
    printf '  FAIL  %-42s got %s, want %s\n' "$label" "$got" "$want"
    fail=1
  fi
}
jq_() { node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const o=JSON.parse(s);process.stdout.write(String(eval("o"+process.argv[1])))})' "$1"; }

check "constitution recovery destination" "$POLICY" \
  "$($BIN query constitution invariants $Q | jq_ '.invariants.enforcement_recovery_destination')"
check "constitution custodian count" "5" \
  "$($BIN query constitution invariants $Q | jq_ '.invariants.foundation_custodian_count')"
check "constitution signature threshold" "3" \
  "$($BIN query constitution invariants $Q | jq_ '.invariants.foundation_signature_threshold')"
check "enforcement recovery destination" "$POLICY" \
  "$($BIN query enforcement params $Q | jq_ '.params.recovery_destination')"
check "group policy address" "$POLICY" \
  "$($BIN query group group-policies-by-group 1 $Q | jq_ '.group_policies[0].address')"
# .value.threshold, not .threshold: the decision policy is a protobuf Any, and
# the CLI's JSON renders an Any as {type, value} rather than flattening it with
# an @type key the way the genesis file does. Reading the flattened path finds
# undefined, which compares unequal to "3" and reports a 3-of-5 as broken.
check "group policy threshold" "3" \
  "$($BIN query group group-policies-by-group 1 $Q | jq_ '.group_policies[0].decision_policy.value.threshold')"
check "group members" "5" \
  "$($BIN query group group-members 1 $Q | jq_ '.members.length')"
check "alias foundation administrator" "$POLICY" \
  "$($BIN query alias params $Q | jq_ '.params.foundation_administrators[0]')"
check "policy account balance" "100000000000" \
  "$($BIN query bank balances "$POLICY" $Q | jq_ '.balances[0].amount')"
[ "$fail" -eq 0 ] || die "the chain started but does not match what this script asked for"

# ---------------------------------------------------------------------------
# The 3-of-5, proven on the running chain
#
# Off by default because it costs a couple of minutes and mutates the chain.
# Worth running at least once after any change to the ceremony or the genesis
# assembly, because it is the only thing here that proves the foundation needs
# three signatures rather than merely claiming a threshold of 3 in a query.
#
# The proposal is deliberately trivial — one microtoken from the policy to
# operator — so that a failure is a failure of the threshold and not of the
# message.
# ---------------------------------------------------------------------------
if [ "${PROVE_FOUNDATION:-0}" != "1" ]; then
  say ""
  say "  Re-run with PROVE_FOUNDATION=1 to have three custodians move a coin"
  say "  and to watch two of them fail to."
  exit 0
fi

say ""
say "=== proving the 3-of-5 on the running chain ==="

# Read the whole result of a transaction, not its broadcast code.
#
# `code: 0` from a broadcast means ACCEPTED, and for x/group's Exec it does not
# even mean the proposal ran. Exec on a proposal that has not reached its
# threshold is a successful transaction that does nothing: code 0, no error, and
# an EventExec carrying PROPOSAL_EXECUTOR_RESULT_NOT_RUN. That is the single
# most dangerous thing about driving a group from a script — a caller checking
# only the exit status of the broadcast would conclude the foundation had acted
# on two signatures. So the assertion below is on the EventExec result attribute
# and, at the end, on a balance.
tx_field() {
  local hash=$1 expr=$2
  for _ in $(seq 1 25); do
    if out=$($BIN query tx "$hash" $Q 2>/dev/null); then
      printf '%s' "$out" | node -e '
        let s = "";
        process.stdin.on("data", (d) => (s += d)).on("end", () => {
          const tx = JSON.parse(s);
          const execResult = () => {
            for (const e of tx.events || []) {
              if (e.type !== "cosmos.group.v1.EventExec") continue;
              for (const a of e.attributes || []) {
                if (a.key === "result") return JSON.parse(a.value);
              }
            }
            return "NO_EVENT_EXEC";
          };
          const out = { code: tx.code, raw_log: String(tx.raw_log).slice(0, 300), exec: execResult() };
          process.stdout.write(String(out[process.argv[1]]));
        });
      ' "$expr"
      return 0
    fi
    sleep 1
  done
  printf 'NOT_FOUND'
}
broadcast() {
  "$@" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).txhash))'
}
# A step whose only job is to land: submitting, voting. Reported, and fatal if
# the chain rejected it, because a failed vote makes every number after it a lie.
step() {
  local label=$1; shift
  local hash code
  hash=$(broadcast "$@")
  sleep 3
  code=$(tx_field "$hash" code)
  printf '  %-34s tx %s  code=%s\n' "$label" "$hash" "$code"
  [ "$code" = "0" ] || die "$label failed: $(tx_field "$hash" raw_log)"
}
balance_of() { $BIN query bank balances "$1" $Q | jq_ '.balances[0].amount'; }

PROP=$LOCAL/foundation-proposal.json
cat > "$PROP" <<JSON
{
  "group_policy_address": "$POLICY",
  "messages": [
    {
      "@type": "/cosmos.bank.v1beta1.MsgSend",
      "from_address": "$POLICY",
      "to_address": "$OPERATOR",
      "amount": [{"denom": "$DENOM", "amount": "1"}]
    }
  ],
  "metadata": "",
  "title": "One coin, to prove three signatures",
  "summary": "Sends 1$DENOM from the foundation policy to operator. The amount is irrelevant; the point is how many custodians it takes to move it.",
  "proposers": ["${CUSTODIAN_ADDRS[0]}"]
}
JSON

BEFORE=$(balance_of "$OPERATOR")
say "  operator balance before: ${BEFORE}$DENOM"

# Submitting is not voting. x/group casts no vote for a proposer, so all three
# yes votes below are separate transactions — including the proposer's.
step "submit (custodian1 proposes)" $BIN tx group submit-proposal "$PROP" --from "${CUSTODIAN_KEYS[0]}" $TX
PROPOSAL_ID=$($BIN query group proposals-by-group-policy "$POLICY" $Q | jq_ '.proposals[0].id')
say "  proposal id $PROPOSAL_ID"

step "vote yes: ${CUSTODIAN_KEYS[0]}" $BIN tx group vote "$PROPOSAL_ID" "${CUSTODIAN_ADDRS[0]}" VOTE_OPTION_YES "" --from "${CUSTODIAN_KEYS[0]}" $TX
step "vote yes: ${CUSTODIAN_KEYS[1]}" $BIN tx group vote "$PROPOSAL_ID" "${CUSTODIAN_ADDRS[1]}" VOTE_OPTION_YES "" --from "${CUSTODIAN_KEYS[1]}" $TX

say ""
say "  -- two of five --"
say "  tally: $($BIN query group tally-result "$PROPOSAL_ID" $Q | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).tally;process.stdout.write(`yes ${t.yes_count} no ${t.no_count} abstain ${t.abstain_count} veto ${t.no_with_veto_count}`)})')"
HASH2=$(broadcast $BIN tx group exec "$PROPOSAL_ID" --from operator $TX)
sleep 3
CODE2=$(tx_field "$HASH2" code)
EXEC2=$(tx_field "$HASH2" exec)
say "  exec: tx $HASH2  code=$CODE2  EventExec.result=$EXEC2"
say "  proposal status: $($BIN query group proposal "$PROPOSAL_ID" $Q 2>&1 | jq_ '.proposal.status' 2>/dev/null || echo 'pruned')"
MID=$(balance_of "$OPERATOR")
say "  operator balance: ${MID}$DENOM"
[ "$EXEC2" = "PROPOSAL_EXECUTOR_RESULT_NOT_RUN" ] || \
  die "two custodians out of five executed the proposal (EventExec.result=$EXEC2). The 3-of-5 is not being enforced."
[ "$MID" = "$BEFORE" ] || die "the balance moved on two signatures: $BEFORE -> $MID"
say "  correct: code 0, nothing ran, nothing moved"

step "vote yes: ${CUSTODIAN_KEYS[2]}" $BIN tx group vote "$PROPOSAL_ID" "${CUSTODIAN_ADDRS[2]}" VOTE_OPTION_YES "" --from "${CUSTODIAN_KEYS[2]}" $TX

say ""
say "  -- three of five --"
say "  tally: $($BIN query group tally-result "$PROPOSAL_ID" $Q | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).tally;process.stdout.write(`yes ${t.yes_count} no ${t.no_count} abstain ${t.abstain_count} veto ${t.no_with_veto_count}`)})')"
HASH3=$(broadcast $BIN tx group exec "$PROPOSAL_ID" --from operator $TX)
sleep 3
CODE3=$(tx_field "$HASH3" code)
EXEC3=$(tx_field "$HASH3" exec)
say "  exec: tx $HASH3  code=$CODE3  EventExec.result=$EXEC3"
AFTER=$(balance_of "$OPERATOR")
say "  operator balance: ${AFTER}$DENOM"
[ "$EXEC3" = "PROPOSAL_EXECUTOR_RESULT_SUCCESS" ] || \
  die "three custodians out of five did NOT execute the proposal (EventExec.result=$EXEC3)"
[ "$AFTER" = "$((BEFORE + 1))" ] || die "expected $((BEFORE + 1)), got $AFTER"
# The proposal is gone from the store now rather than sitting there as history:
# x/group prunes an executed proposal, which the EventProposalPruned in the exec
# transaction records with the status and tally it was pruned at. `query group
# proposal` on it returns "load proposal: not found" from here on, and that is
# success rather than an error.
# Checked without a pipeline, deliberately. `query ... | grep -o 'not found' ||
# echo present` prints BOTH branches under `set -o pipefail`: grep matches and
# prints, and then pipefail propagates the failed query's own status so the ||
# fires anyway. The exit code of the query on its own is the honest signal.
if $BIN query group proposal "$PROPOSAL_ID" $Q >/dev/null 2>&1; then
  say "  proposal after exec: still in the store, which it should not be"
else
  say "  proposal after exec: gone — pruned, see EventProposalPruned in $HASH3"
fi

say ""
say "  PROVEN: two of five moved nothing, three of five moved the coin."
say "  node still running as pid $NODE_PID — kill $NODE_PID to stop it"
