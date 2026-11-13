#!/usr/bin/env bash
#
# Admits a jeweler created in the loyalty app: the office's vote, as in
# seed-jewel.sh stage 1 (3-of-5 locally; 2-of-3 on devnet-2, OFFICE_VOTES=2, with
# FUNDER naming a key that holds funds there), then 50 YML to the business so it can pay its staff's
# fees.
#
#   BIN=$SCRATCH/blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 scripts/devnet/loyalty-admit.sh "Maison Test" yml1…
set -euo pipefail
BIN=${BIN:?the chain binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}
FUNDER=${FUNDER:-$M3}
NAME=${1:?the jeweler name}
BUSINESS=${2:?the business account code, from the Account tab of the app}
# The name is written into JSON below, so it may not carry a double quote, a
# backslash or a control character.
case "$NAME" in
  *'"'* | *'\'*) echo "the jeweler name may not contain a double quote or a backslash" >&2; exit 1 ;;
esac
if [ -n "$(printf '%s' "$NAME" | LC_ALL=C tr -dc '[:cntrl:]')" ]; then
  echo "the jeweler name may not contain a control character" >&2
  exit 1
fi
TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"
. "$(dirname "$0")/jewel-lib.sh"

run bank send "$(addr "$FUNDER")" "$BUSINESS" 50000000uyml --from "$FUNDER" >/dev/null \
  && ok "funded the business" || { bad "funding"; exit 1; }
as_office "{\"@type\":\"/blockchain.jewel.v1.MsgAdmitJeweler\",\"office\":\"$OFFICE\",\"name\":\"$NAME\",\"business\":\"$BUSINESS\"}" \
  && ok "admitted $NAME" || { bad "admission"; exit 1; }
"$BIN" query jewel jewelers $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4
