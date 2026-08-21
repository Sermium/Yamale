#!/bin/bash
# A one-validator devnet that clears a real multilateral cycle and measures the
# compression it achieved.
#
# The tests prove the arithmetic against a keeper. They do not prove that an
# institution's back office can post a reserve over the wire, that an obligation
# submitted as a transaction lands in the open window, that an end blocker on a
# chain producing blocks closes that window on the height it said it would, or
# that the figure the chain reports is the one the design was bought with. Those
# are claims about a running chain, and the only way to make them is to run one.
#
# What the run is for is the last of those. Tiering was chosen over confidential
# amounts because net settlement between institutions is low-volume and carries
# no customer detail — and the whole argument rests on netting actually removing
# most of the value that would otherwise have to be funded. This measures how
# much, on a cycle of criss-crossing bilateral obligations rather than on a
# hand-picked ring of three.
#
# Non-default ports throughout. Two chains on 26657 produced transactions that
# succeeded and then reported "account not found", because the CLI was talking
# to one node about a transaction the other had accepted.
set -euo pipefail

BIN=${BIN:-./blockchaind.exe}
ROOT=${ROOT:-./.devnet-netting}
CHAIN_ID=${CHAIN_ID:-yamale-netting-1}
H="$ROOT/node0"

# 26787 rather than 26757, which is the number agreed for a devnet on this
# machine: 26757 was already held by another local chain, this script bound
# nothing, and every query went to that chain instead — it reported a height of
# 2046 on a chain that had produced two blocks. Hence the port check below,
# which is the part that matters more than the numbers.
RPC=${RPC:-26787}
API=${API:-1447}
P2P=$((RPC - 1))
GRPC=9190
PPROF=6160

# The settlement currency. ungn rather than the bond denomination, so that
# posting a reserve cannot be confused with bonding and the compression figure
# is measured on money that only ever moves through this module.
DENOM=ungn

# Sixty blocks at roughly a second each: long enough to submit a day's worth of
# obligations into one window, short enough to watch it close. On a real chain
# this is a settlement session.
CYCLE_BLOCKS=60

# Everything below this nets; at or above it settles gross in its own block.
# 5,000,000 base units against obligations drawn from a 1,000,000-4,000,000
# spread, so the whole cycle nets and the threshold is exercised separately.
GROSS_THRESHOLD=5000000

KR="--keyring-backend test --home $H"
TX="--chain-id $CHAIN_ID --keyring-backend test --home $H --node tcp://127.0.0.1:$RPC --fees 500uyml --yes --output json"
Q="--home $H --node tcp://127.0.0.1:$RPC --output json"

BANKS=(bank-a bank-b bank-c bank-d)

# Refused before anything is built. A port already in use means `start` fails to
# bind and every query afterwards is answered by whatever is already there —
# which does not look like an error, it looks like a chain that is further along
# than it should be, and every figure this script prints would be somebody
# else's.
for port in $RPC $API $P2P; do
  if netstat -ano 2>/dev/null | grep -qE "[:.]$port +[0-9.]+:[0-9*]+ +LISTENING"; then
    echo "port $port is already in use; set RPC= and API= to something free" >&2
    exit 1
  fi
done

echo "=== clearing $ROOT ==="
rm -rf "$ROOT"; mkdir -p "$ROOT"
$BIN init node0 --chain-id "$CHAIN_ID" --default-denom uyml --home "$H" >/dev/null 2>&1

echo "=== keys ==="
declare -A ADDR
for k in val0 foundation "${BANKS[@]}"; do
  $BIN keys add "$k" $KR >/dev/null 2>&1
  ADDR[$k]=$($BIN keys show "$k" -a $KR)
  echo "  $k  ${ADDR[$k]}"
done

echo "=== genesis accounts ==="
# Each bank holds fees in uyml and 200,000,000 of the settlement currency. The
# reserve posted below is a fraction of the gross flow, which is the point being
# demonstrated: what a netted system has to prefund is the net exposure.
$BIN genesis add-genesis-account "${ADDR[val0]}" 100000000000uyml --home "$H"
$BIN genesis add-genesis-account "${ADDR[foundation]}" 100000000000uyml --home "$H"
for b in "${BANKS[@]}"; do
  $BIN genesis add-genesis-account "${ADDR[$b]}" "10000000000uyml,200000000$DENOM" --home "$H"
done

echo "=== seeding the register and the netting policy ==="
# Both written into genesis rather than proposed afterwards. Admission to the
# rail is gov-gated and a governance proposal does not pass on a one-validator
# devnet inside the life of a demonstration, and netting parameters are
# authority-gated for the same reason — so a run that needed proposals to pass
# would be a run that measured nothing.
python - "$H/config/genesis.json" "$DENOM" "$CYCLE_BLOCKS" "$GROSS_THRESHOLD" \
  "${ADDR[bank-a]}" "${ADDR[bank-b]}" "${ADDR[bank-c]}" "${ADDR[bank-d]}" "${ADDR[foundation]}" <<'PY'
import json, sys

path, denom, cycle_blocks, threshold = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
banks = sys.argv[5:9]
foundation = sys.argv[9]
g = json.load(open(path))
app = g["app_state"]

# The founding settlement, complete. The chain refuses to start on a partial
# one, which is the behaviour that matters and not something to route around —
# so it is written out in full here rather than borrowed from a ceremony this
# demonstration has no need of.
#
# Every concentration ceiling is 10000, which is no ceiling at all, and that is
# the honest value for a one-validator chain: it holds every basis point, and a
# tighter number would be a permanent breach dressed up as a rule. The caps are
# demonstrated by scripts/devnet/concentration-demo.sh, which is what four
# validators are for.
app.setdefault("constitution", {})["invariants"] = {
    "max_entity_power_bps": "10000",
    "max_beneficial_owner_power_bps": "10000",
    "max_jurisdiction_power_bps": "10000",
    "concentration_epoch_blocks": "120",
    "min_active_validators": 1,
    "enforcement_threshold_bps": "6667",
    "enforcement_recovery_destination": foundation,
    "enforcement_voting_period_blocks": "360",
    "enforcement_provisional_freeze_blocks": "720",
    "amendment_delay_blocks": "120960",
    "amendment_threshold_bps": "8000",
    "foundation_custodian_count": 5,
    "foundation_signature_threshold": 3,
}
app["constitution"]["amendments"] = []
app["constitution"]["ratifications"] = []
app["constitution"]["amendment_count"] = "1"

# x/enforcement keeps its own copy of four of those values and the chain refuses
# a genesis where the two disagree, so they are written from the same numbers.
enf = app["enforcement"]["params"]
enf["recovery_destination"] = foundation
enf["threshold_bps"] = "6667"
enf["voting_period_blocks"] = "360"
enf["provisional_freeze_blocks"] = "720"
# The rest of the seizure schedule, which the chain also refuses to start
# without. None of it is what this run is about; the values mirror
# scripts/devnet/init-devnet.sh so the two devnets behave the same when
# somebody looks at enforcement on either of them.
enf["seizure_delay_blocks"] = "240"
enf["seizure_delay_tiers"] = [
    {"threshold": {"denom": "uyml", "amount": "1000000"}, "delay_blocks": "720"},
    {"threshold": {"denom": "uyml", "amount": "100000000"}, "delay_blocks": "2880"},
]
enf["seizure_window_blocks"] = "17280"
enf["seizure_window_cap"] = [{"denom": "uyml", "amount": "500000000"}]
enf["max_seizures_per_window"] = "5"

app["paymsg"]["approved_participant_map"] = [
    {
        "participant": address,
        "code": "DEMOBANK%s" % chr(ord("A") + i),
        "name": "Demonstration Bank %s" % chr(ord("A") + i),
        "payload_store_url": "",
    }
    for i, address in enumerate(banks)
]

# One currency, one threshold. A currency with no policy nets nothing, which is
# the safe direction and is also what every other denomination on this chain
# keeps doing while the demonstration runs.
app["netting"]["params"] = {
    "cycle_blocks": str(cycle_blocks),
    "denom_policies": [{"denom": denom, "gross_threshold": threshold}],
}

gov = app["gov"]["params"]
gov["voting_period"] = "300s"
gov["expedited_voting_period"] = "150s"
gov["max_deposit_period"] = "300s"

json.dump(g, open(path, "w"), indent=2)
print("  four participants approved, %s netting every %s blocks" % (denom, cycle_blocks))
PY

echo "=== gentx ==="
$BIN genesis gentx val0 1000000uyml --chain-id "$CHAIN_ID" $KR >/dev/null 2>&1
$BIN genesis collect-gentxs --home "$H" >/dev/null 2>&1
$BIN genesis validate-genesis --home "$H"

echo "=== ports ==="
CFG="$H/config/config.toml"; APP="$H/config/app.toml"
sed -i "s|^laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://127.0.0.1:$RPC\"|" "$CFG"
sed -i "s|^laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:$P2P\"|" "$CFG"
sed -i "s|^pprof_laddr = \".*\"|pprof_laddr = \"localhost:$PPROF\"|" "$CFG"
sed -i "s|^timeout_commit = \".*\"|timeout_commit = \"1s\"|" "$CFG"
sed -i "s|^minimum-gas-prices = .*|minimum-gas-prices = \"0uyml\"|" "$APP"
sed -i "s|^address = \"tcp://localhost:1317\"|address = \"tcp://localhost:$API\"|" "$APP"
sed -i "s|^address = \"localhost:9090\"|address = \"localhost:$GRPC\"|" "$APP"
sed -i "0,/^enable = false/s//enable = true/" "$APP"

echo "=== starting ==="
$BIN start --home "$H" >"$ROOT/node0.log" 2>&1 &
NODE_PID=$!
# Matched on the full path rather than the bare name: a pkill for "blockchaind"
# on this box matches the grep that is looking for it and kills the script.
trap 'kill $NODE_PID 2>/dev/null || true' EXIT

height() { $BIN status --node "tcp://127.0.0.1:$RPC" 2>/dev/null | python -c 'import json,sys; print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null || echo 0; }

echo -n "  waiting for the first block"
for _ in $(seq 1 60); do
  [ "$(height)" -ge 1 ] 2>/dev/null && break
  echo -n "."; sleep 1
done
echo " height $(height)"

# One transaction per block per key. Two from the same account in one block
# collide on the sequence and the second is dropped without an error anybody
# sees, which on this script would silently drop an obligation out of the cycle
# and quietly change the compression figure it reports.
send() {
  local out code
  out=$($BIN tx "$@" $TX 2>&1)
  code=$(echo "$out" | python -c 'import json,sys
try: print(json.load(sys.stdin)["code"])
except Exception: print("?")' 2>/dev/null || echo "?")
  if [ "$code" != "0" ]; then
    echo "  FAILED code=$code"
    echo "$out" | head -5
    exit 1
  fi
  sleep 2
}

# A stand-in for SHA-256 over the salted retail batch an obligation summarises.
# Real ones are computed by the participant over its own book; what matters here
# is that the chain refuses anything that is not 32 bytes.
bh() { printf '%s' "$1" | python -c 'import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())'; }

echo "=== posting reserves: 20,000,000 $DENOM each ==="
# A tenth of the gross flow submitted below. That the cycle clears at all on
# reserves this thin is half the result.
for b in "${BANKS[@]}"; do
  echo "  $b"
  send netting post-reserve "${ADDR[$b]}" "20000000$DENOM" --from "$b"
done

echo "=== submitting the day's obligations ==="
# Sixteen obligations between four institutions, including the three-way ring
# that bilateral netting cannot touch at all: A owes B, B owes C, C owes A.
# Amounts are all below the gross threshold, so every one of them nets.
OBLIGATIONS=(
  "bank-a bank-b 4000000"
  "bank-b bank-c 3800000"
  "bank-c bank-a 3600000"
  "bank-a bank-c 2500000"
  "bank-b bank-d 3100000"
  "bank-c bank-d 1900000"
  "bank-d bank-a 2800000"
  "bank-a bank-d 1200000"
  "bank-b bank-a 2200000"
  "bank-c bank-b 1700000"
  "bank-d bank-b 2600000"
  "bank-a bank-b 1500000"
  "bank-d bank-c 3300000"
  "bank-b bank-d 1100000"
  "bank-c bank-a 2400000"
  "bank-d bank-a 1600000"
)
GROSS=0
i=0
for row in "${OBLIGATIONS[@]}"; do
  set -- $row
  from=$1; to=$2; amount=$3
  i=$((i + 1))
  echo "  $i/16  $from -> $to  $amount"
  send netting submit-obligation "${ADDR[$from]}" "${ADDR[$to]}" "$DENOM" "$amount" \
    --batch-hash "$(bh "batch-$i")" --from "$from"
  GROSS=$((GROSS + amount))
done
echo "  gross submitted: $GROSS $DENOM"

echo "=== positions before the window closes ==="
for b in "${BANKS[@]}"; do
  echo "  $b: $($BIN query netting position "${ADDR[$b]}" $Q | python -c '
import json,sys
for e in json.load(sys.stdin).get("entries", []):
    print("reserve %s locked %s available %s net %s" % (e["reserve"], e["locked"], e["available"], e["net_position"]))' )"
done

echo "=== an obligation at the gross threshold settles in its own block ==="
BEFORE=$($BIN query bank balance "${ADDR[bank-b]}" "$DENOM" $Q | python -c 'import json,sys; print(json.load(sys.stdin)["balance"]["amount"])')
send netting submit-obligation "${ADDR[bank-a]}" "${ADDR[bank-b]}" "$DENOM" "$GROSS_THRESHOLD" \
  --batch-hash "$(bh high-value)" --from bank-a
AFTER=$($BIN query bank balance "${ADDR[bank-b]}" "$DENOM" $Q | python -c 'import json,sys; print(json.load(sys.stdin)["balance"]["amount"])')
echo "  bank-b balance $BEFORE -> $AFTER (moved $((AFTER - BEFORE)), and it is not in the window)"

echo "=== waiting for the window to close at height $CYCLE_BLOCKS ==="
while [ "$(height)" -lt $((CYCLE_BLOCKS + 1)) ]; do
  echo -n "."; sleep 2
done
echo " height $(height)"

echo "=== the cycle, as the chain reports it ==="
# Written to a file rather than piped: a heredoc is itself stdin, so a pipe into
# `python - <<PY` is silently discarded and the script reads the heredoc twice.
$BIN query netting cycle 1 $Q > "$ROOT/cycle1.json"
python - "$GROSS" "$ROOT/cycle1.json" <<'PY'
import json, sys
expected_gross = int(sys.argv[1])
res = json.load(open(sys.argv[2]))
cycle = res["cycle"]
print("  status: %s   closed at height %s" % (cycle["status"], cycle["closed_at_height"]))
bps = {c["denom"]: int(c["compression_bps"]) for c in res.get("compression", [])}
for outcome in cycle.get("outcomes", []):
    gross, net = int(outcome["gross_amount"]), int(outcome["net_amount"])
    b = bps.get(outcome["denom"], 0)
    print("  %s: %d obligations, gross %d, net funded %d" %
          (outcome["denom"], int(outcome["obligation_count"]), gross, net))
    print("  compression: %d.%02d%% -- %d of every 100 units submitted never had to be funded"
          % (b // 100, b % 100, b // 100))
    assert gross == expected_gross, "the chain's gross (%d) is not what was submitted (%d)" % (gross, expected_gross)
    assert outcome["status"] == "DENOM_STATUS_SETTLED", outcome["status"]
PY

echo "=== nothing is held ==="
$BIN query netting held $Q

echo "=== reserves after settlement ==="
TOTAL=0
for b in "${BANKS[@]}"; do
  R=$($BIN query netting position "${ADDR[$b]}" $Q | python -c '
import json,sys
e = json.load(sys.stdin)["entries"]
print(e[0]["reserve"] if e else 0)')
  echo "  $b reserve $R  locked $($BIN query netting position "${ADDR[$b]}" $Q | python -c '
import json,sys
e = json.load(sys.stdin)["entries"]
print(e[0]["locked"] if e else 0)')"
  TOTAL=$((TOTAL + R))
done
echo "  recorded reserves total: $TOTAL"
echo "  module account holds:    $($BIN query bank balances "$($BIN query auth module-account netting $Q | python -c 'import json,sys; print(json.load(sys.stdin)["account"]["value"]["address"])')" $Q | python -c '
import json,sys
for c in json.load(sys.stdin)["balances"]:
    print(c["amount"], c["denom"])')"

echo
echo "=== done. node log: $ROOT/node0.log ==="
