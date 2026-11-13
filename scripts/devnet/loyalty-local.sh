#!/usr/bin/env bash
#
# Runs loyalty-api against the local devnet: a sponsor key funded from a
# custodian, an operator token for the application list, and the data under
# .devnet/local/loyalty. Runs in the foreground; stop it with Ctrl-C.
#
#   BIN=$SCRATCH/blockchaind.exe API_BIN=$SCRATCH/loyaltyapi.exe scripts/devnet/loyalty-local.sh
set -euo pipefail
BIN=${BIN:?the chain binary}
API_BIN=${API_BIN:?the loyaltyapi binary}
HOME_DIR=${HOME_DIR:-.devnet/local/node}
CHAIN=${CHAIN:-yamale-local-1}
NODE=${NODE:-tcp://127.0.0.1:26957}
REST=${REST:-http://127.0.0.1:1517}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}
DATA=${DATA:-.devnet/local/loyalty}
TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"
. "$(dirname "$0")/jewel-lib.sh"

mkdir -p "$DATA"
ensure_key loyalty-sponsor
SPONSOR=$(addr loyalty-sponsor)
balance=$("$BIN" query bank balance "$SPONSOR" uyml $Q | json amount)
if [ "${balance:-0}" = "0" ]; then
  run bank send "$(addr "$M3")" "$SPONSOR" 1000000000uyml --from "$M3" >/dev/null && ok "funded the sponsor" || { bad "funding the sponsor"; exit 1; }
fi
[ -s "$DATA/operator-token" ] || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$DATA/operator-token"

say "loyalty-api on 127.0.0.1:8095, sponsor $SPONSOR"
exec "$API_BIN" --listen 127.0.0.1:8095 --dir "$DATA" --rest "$REST" --chain-id "$CHAIN" \
  --bin "$BIN" --from loyalty-sponsor --home "$HOME_DIR" --keyring-backend "$KEYRING" --node "$NODE" \
  --operator-token-file "$DATA/operator-token"
