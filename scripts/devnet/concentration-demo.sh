#!/bin/bash
# A four-validator devnet that demonstrates a concentration demotion on a
# running chain.
#
# Tests prove the epoch check does the right thing to a keeper. They do not
# prove that a validator with real consensus power stops producing blocks when
# its beneficial owner goes over a ceiling, that the rest of the set keeps
# committing without it, or that it comes back when the breach clears. Those are
# claims about a chain, and the only way to make them is to run one.
#
# Four validators because the arithmetic needs it: two share an owner, the owner
# ceiling is a third, and the floor below which the chain reports instead of
# acting is three. One demotion takes the set from four to three, which is the
# floor exactly — so the chain corrects the breach and then stops, which is both
# halves of the design in one run.
#
# Non-default ports throughout. Two chains on 26657 produced transactions that
# succeeded and then reported "account not found", because the CLI was talking
# to one node about a transaction the other had accepted.
set -euo pipefail

BIN=${BIN:-./blockchaind.exe}
ROOT=${ROOT:-./.devnet}
CHAIN_ID=${CHAIN_ID:-yamale-caps-1}

# One seat is one unit of consensus power, which is DefaultPowerReduction base
# units. Equal seats means every validator bonds exactly this.
SEAT=1000000

# node index -> ports. RPC starts at 26757 and the API at 1417, as agreed, and
# every other port is derived so that four nodes on one machine cannot collide.
rpc_port()  { echo $((26757 + $1 * 10)); }
p2p_port()  { echo $((26756 + $1 * 10)); }
api_port()  { echo $((1417  + $1 * 10)); }
grpc_port() { echo $((9190  + $1 * 10)); }
pprof_port() { echo $((6160 + $1 * 10)); }

home_of() { echo "$ROOT/node$1"; }
kr() { echo "--keyring-backend test --home $(home_of "$1")"; }

echo "=== clearing $ROOT ==="
rm -rf "$ROOT"
mkdir -p "$ROOT"

echo "=== init four nodes ==="
for i in 0 1 2 3; do
  $BIN init "node$i" --chain-id "$CHAIN_ID" --default-denom uyml --home "$(home_of $i)" >/dev/null 2>&1
done

echo "=== keys ==="
declare -a VAL
for i in 0 1 2 3; do
  $BIN keys add "val$i" $(kr $i) >/dev/null 2>&1
  VAL[$i]=$($BIN keys show "val$i" -a $(kr $i))
  echo "  val$i  ${VAL[$i]}"
done
$BIN keys add foundation $(kr 0) >/dev/null 2>&1
FOUNDATION=$($BIN keys show foundation -a $(kr 0))
echo "  foundation  $FOUNDATION"

echo "=== genesis accounts ==="
# Node 0 builds the genesis; the others receive it.
G=$(home_of 0)/config/genesis.json
for i in 0 1 2 3; do
  $BIN genesis add-genesis-account "${VAL[$i]}" $((SEAT * 10))uyml --home "$(home_of 0)"
done
$BIN genesis add-genesis-account "$FOUNDATION" 500000000000uyml --home "$(home_of 0)"

echo "=== the settlement and the register ==="
# Written into genesis rather than proposed afterwards, because that is what
# "genesis-fixed" means: the chain refuses to start without a complete one.
#
# The two ceilings this run is not about are set to the whole set, which is the
# only value that genuinely disables one — at four validators anything less is a
# real ceiling and would demote validators this demonstration is not looking at.
python - "$G" "$FOUNDATION" "${VAL[0]}" "${VAL[1]}" "${VAL[2]}" "${VAL[3]}" <<'PY'
import json, sys

path, foundation = sys.argv[1], sys.argv[2]
validators = sys.argv[3:7]
g = json.load(open(path))
app = g["app_state"]

app["constitution"] = {
    "invariants": {
        "max_entity_power_bps": "10000",
        "max_beneficial_owner_power_bps": "3400",
        "max_jurisdiction_power_bps": "10000",
        # Ten blocks, so an epoch arrives while somebody is watching. On a real
        # chain this is a day.
        "concentration_epoch_blocks": "10",
        # Three: one demotion takes four validators to three, and the chain
        # must refuse to go further rather than enforce a ceiling into a halt.
        "min_active_validators": 3,
        "enforcement_threshold_bps": "6667",
        "enforcement_recovery_destination": foundation,
        "enforcement_voting_period_blocks": "8640",
        "enforcement_provisional_freeze_blocks": "17280",
        "amendment_delay_blocks": "362880",
        "amendment_threshold_bps": "8000",
    },
    "amendments": [],
    "ratifications": [],
    "amendment_count": "1",
}

# x/enforcement keeps its own copy of the four values above, and the chain
# refuses to start if they disagree.
enforcement = app["enforcement"]["params"]
enforcement["recovery_destination"] = foundation
enforcement["threshold_bps"] = "6667"
enforcement["voting_period_blocks"] = "8640"
enforcement["provisional_freeze_blocks"] = "17280"

# The founding set, declared. A validator with no declaration belongs to no
# group and sits under no ceiling, and a genesis is the one place a whole set
# can be admitted that way at once — so genesis refuses one.
#
# val0 and val1 are two legal entities behind one owner. That is the case the
# beneficial-owner ceiling exists for: each is inside the entity ceiling, and an
# owner admitted twice votes twice.
declarations = [
    ("SUBSIDIARY-A", "STATE-BANK"),
    ("SUBSIDIARY-B", "STATE-BANK"),
    ("INDEPENDENT-C", "INDEPENDENT-C"),
    ("INDEPENDENT-D", "INDEPENDENT-D"),
]
vg = app["validatorgov"]
vg["approved_validator_map"] = [
    {
        "candidate": address,
        "approved": "true",
        "declaration": {
            "legal_entity_id": entity,
            "beneficial_owner_id": owner,
            "jurisdiction": "CH",
            "attested_at_height": "0",
        },
    }
    for address, (entity, owner) in zip(validators, declarations)
]

# Governance short enough to watch, for the same reason the devnet script does
# it: a 48-hour voting period makes every governance path untestable.
gov = app["gov"]["params"]
gov["voting_period"] = "300s"
gov["expedited_voting_period"] = "150s"
gov["max_deposit_period"] = "300s"

json.dump(g, open(path, "w"), indent=2)
print("  constitution, enforcement and the register written")
PY

echo "=== gentx: equal seats ==="
# Equal seats, bonded equally. Under this model a seat is a fixed quantity of
# the bond denomination, because Cosmos derives consensus power from bonded
# tokens and permits exactly one module to report validator updates.
for i in 0 1 2 3; do
  # Every node signs its gentx against the same genesis, which is the one node 0
  # built and which already funds all four accounts. A node signing against its
  # own untouched genesis would be signing for a chain where the other three do
  # not exist.
  if [ "$i" != "0" ]; then
    cp "$G" "$(home_of $i)/config/genesis.json"
  fi
  $BIN genesis gentx "val$i" ${SEAT}uyml --chain-id "$CHAIN_ID" $(kr $i) \
    --output-document "$ROOT/gentx-$i.json" >/dev/null 2>&1
done
mkdir -p "$(home_of 0)/config/gentx"
cp "$ROOT"/gentx-*.json "$(home_of 0)/config/gentx/"
$BIN genesis collect-gentxs --home "$(home_of 0)" >/dev/null 2>&1
$BIN genesis validate-genesis --home "$(home_of 0)"

echo "=== distributing genesis ==="
for i in 1 2 3; do
  cp "$(home_of 0)/config/genesis.json" "$(home_of $i)/config/genesis.json"
done

echo "=== ports and peers ==="
NODE0_ID=$($BIN comet show-node-id --home "$(home_of 0)")
PEERS="$NODE0_ID@127.0.0.1:$(p2p_port 0)"
for i in 0 1 2 3; do
  CFG="$(home_of $i)/config/config.toml"
  APP="$(home_of $i)/config/app.toml"
  sed -i "s|^laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://127.0.0.1:$(rpc_port $i)\"|" "$CFG"
  sed -i "s|^laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:$(p2p_port $i)\"|" "$CFG"
  sed -i "s|^pprof_laddr = \".*\"|pprof_laddr = \"localhost:$(pprof_port $i)\"|" "$CFG"
  sed -i "s|^allow_duplicate_ip = false|allow_duplicate_ip = true|" "$CFG"
  sed -i "s|^addr_book_strict = true|addr_book_strict = false|" "$CFG"
  sed -i "s|^timeout_commit = \".*\"|timeout_commit = \"1s\"|" "$CFG"
  sed -i "s|^minimum-gas-prices = .*|minimum-gas-prices = \"0uyml\"|" "$APP"
  sed -i "s|^address = \"tcp://localhost:1317\"|address = \"tcp://localhost:$(api_port $i)\"|" "$APP"
  sed -i "s|^address = \"localhost:9090\"|address = \"localhost:$(grpc_port $i)\"|" "$APP"
  sed -i "0,/^enable = false/s//enable = true/" "$APP"
  if [ "$i" != "0" ]; then
    sed -i "s|^persistent_peers = \".*\"|persistent_peers = \"$PEERS\"|" "$CFG"
  fi
done

echo "=== starting ==="
for i in 0 1 2 3; do
  $BIN start --home "$(home_of $i)" >"$ROOT/node$i.log" 2>&1 &
  echo "  node$i pid $! rpc $(rpc_port $i)"
done

cat <<EOF

Four nodes are starting. Watch the demonstration with:

  export N0="--node tcp://127.0.0.1:$(rpc_port 0)"
  $BIN query validatorgov concentration \$N0
  $BIN query validatorgov list-demotion \$N0
  $BIN query constitution invariants \$N0

The owner ceiling is 3400 basis points and STATE-BANK holds two seats of four,
which is 5000. At the first epoch — height 10 — one of val0 and val1 is jailed
and the owner comes back to one seat of four. At the second, the set is at the
floor of three and nothing further is done.

To watch it come back, move val1 out from under the shared owner:

  $BIN tx validatorgov attest-ownership SUBSIDIARY-B INDEPENDENT-B CH \\
    --from val1 --keyring-backend test --home $(home_of 1) \\
    --chain-id $CHAIN_ID --node tcp://127.0.0.1:$(rpc_port 1) --fees 500uyml --yes

and wait for the next epoch.

Stop everything with:  pkill -f "blockchaind.*$ROOT"
EOF
