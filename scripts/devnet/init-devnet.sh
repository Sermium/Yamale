#!/bin/bash
# Build a single-node Yamale devnet on the Pi, with every testnet currency
# seeded so the interfaces have real data to render. An empty chain renders
# perfectly no matter what is broken behind it, which is the opposite of useful
# when the point is to find gaps.
set -euo pipefail

BIN=/opt/yamale/bin/blockchaind
CURRENCIES=/opt/yamale/bin/currencies
HOME_DIR=/opt/yamale/node
CHAIN_ID=yamale-devnet-1
KR="--keyring-backend test --home $HOME_DIR"

rm -rf "$HOME_DIR"

echo "=== init ==="
$BIN init pi --chain-id "$CHAIN_ID" --default-denom uyml --home "$HOME_DIR" 2>&1 | tail -1

echo "=== keys ==="
for k in alice bob foundation; do
  $BIN keys add "$k" $KR >/dev/null 2>&1
  echo "  $k  $($BIN keys show "$k" -a $KR)"
done

ALICE=$($BIN keys show alice -a $KR)
BOB=$($BIN keys show bob -a $KR)
FOUNDATION=$($BIN keys show foundation -a $KR)

echo "=== genesis accounts ==="
$BIN genesis add-genesis-account "$ALICE"      200000000000uyml --home "$HOME_DIR"
$BIN genesis add-genesis-account "$BOB"        100000000000uyml --home "$HOME_DIR"
$BIN genesis add-genesis-account "$FOUNDATION" 500000000000uyml --home "$HOME_DIR"
echo "  three accounts funded"

echo "=== seeding currencies ==="
$CURRENCIES --genesis "$HOME_DIR/config/genesis.json" --issuer "$FOUNDATION"

echo "=== enforcement recovery destination ==="
# Where seized assets go. The foundation is the trust body administering the
# chain, and it holds what is recovered so it can be restituted to the people it
# was taken from. It is the same account that issues every currency here, so it
# is the one this devnet already treats as the institution.
#
# Set at genesis rather than left to a governance proposal later, because the
# chain now refuses to start without it. That refusal exists because of this
# devnet: it ran for weeks with the parameter empty, which meant two thirds of
# the validator set could have passed a seizure that then had nowhere to send
# what it took. Nobody noticed until a console printed the parameter.
G=$HOME_DIR/config/genesis.json
python3 - "$G" "$FOUNDATION" <<'PY' 2>/dev/null || sed -i \
  -e "s|\"recovery_destination\": *\"[^\"]*\"|\"recovery_destination\": \"$FOUNDATION\"|" "$G"
import json, sys
path, destination = sys.argv[1], sys.argv[2]
g = json.load(open(path))
g["app_state"]["enforcement"]["params"]["recovery_destination"] = destination
json.dump(g, open(path, "w"), indent=2)
PY
echo "  recovery destination: $(grep -o '"recovery_destination": *"[^"]*"' "$G" | head -1)"

echo "=== validator ==="
$BIN genesis gentx alice 100000000000uyml --chain-id "$CHAIN_ID" $KR 2>&1 | tail -1
$BIN genesis collect-gentxs --home "$HOME_DIR" 2>&1 | tail -1
$BIN genesis validate-genesis --home "$HOME_DIR"

echo "=== shortening governance for a devnet ==="
# A 48-hour voting period makes every governance path untestable, which is why
# the custody asset registration, the stablecoin issuer approval and the
# validator admission flows have only ever been exercised in unit tests.
#
# Thirty minutes, not three. Three was too short to be useful: a proposal has to
# be submitted, then voted by both validators, and each vote is a separate
# transaction from a separate key on a separate machine — one per account per
# block, because the ante handler rejects a non-current sequence. Any pause to
# read an error, fix a flag and retry ran the clock out, so proposals expired
# while they were being debugged. Half an hour survives a debugging session and
# is still short enough to watch a proposal pass inside one.
#
# Deposits drop to a nominal amount for the same reason: the point is to
# exercise the permission gate, not to model the economics.
python3 - "$G" <<'PY' 2>/dev/null || sed -i \
  -e 's|"voting_period": *"[^"]*"|"voting_period": "1800s"|g' \
  -e 's|"expedited_voting_period": *"[^"]*"|"expedited_voting_period": "900s"|g' \
  -e 's|"max_deposit_period": *"[^"]*"|"max_deposit_period": "600s"|g' "$G"
import json, sys
p = sys.argv[1]
g = json.load(open(p))
params = g["app_state"]["gov"]["params"]
params["voting_period"] = "1800s"
params["expedited_voting_period"] = "900s"
params["max_deposit_period"] = "600s"
params["min_deposit"] = [{"denom": "uyml", "amount": "1000000"}]
params["expedited_min_deposit"] = [{"denom": "uyml", "amount": "2000000"}]
json.dump(g, open(p, "w"), indent=2)
PY
echo "  voting period: $(grep -o '"voting_period": *"[^"]*"' "$G" | head -1)"

echo "=== opening the API ==="
APP=$HOME_DIR/config/app.toml
# The node refuses to start without a minimum gas price, and a devnet wants it
# free. Bound to localhost only -- nginx is what faces the network.
sed -i 's|^minimum-gas-prices = .*|minimum-gas-prices = "0uyml"|' "$APP"
sed -i '/^\[api\]/,/^\[/{s|^enable = .*|enable = true|}' "$APP"
sed -i '/^\[api\]/,/^\[/{s|^address = .*|address = "tcp://127.0.0.1:1317"|}' "$APP"
# Prometheus, so the monitoring config written earlier has something to scrape.
sed -i 's|^prometheus = false|prometheus = true|' "$HOME_DIR/config/config.toml"

echo "  minimum-gas-prices : $(grep '^minimum-gas-prices' "$APP")"
echo "  api enable         : $(sed -n '/^\[api\]/,/^\[grpc\]/p' "$APP" | grep '^enable')"
echo "  api address        : $(sed -n '/^\[api\]/,/^\[grpc\]/p' "$APP" | grep '^address')"
echo "  prometheus         : $(grep '^prometheus =' "$HOME_DIR/config/config.toml")"

echo
echo "DEVNET READY"
echo "alice=$ALICE"
echo "bob=$BOB"
echo "foundation=$FOUNDATION"
