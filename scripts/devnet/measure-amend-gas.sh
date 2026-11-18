#!/usr/bin/env bash
#
# Measures the gas of the heaviest jewel message: the 50th amendment to one
# record, with a 500-byte note. An amendment re-reads and re-writes the jewel
# and appends to its history, so its cost grows with the history; this prints
# the cost at the cap the app sets its gas limit against (JEWEL_GAS in
# clients/sdk/src/jewel.ts).
#
# Run it on a local devnet that has an office and an admitted jeweler with
# staff, right after seed-jewel.sh (which leaves `jw-staff` in the keyring):
#
#   BIN=$SCRATCH/blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 scripts/devnet/measure-amend-gas.sh
set -euo pipefail

BIN=${BIN:?the chain binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
STAFF_KEY=${STAFF_KEY:-jw-staff}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"

# jewel-lib.sh reads the office at load; it is needed only for run/committed/ok/bad.
. "$(dirname "$0")/jewel-lib.sh"

NOTE=$(head -c 500 /dev/zero | tr '\0' 'n')
HASHCMD() { printf '%s' "$1" | sha256sum | cut -d' ' -f1; }

INSCRIPTION="GAS-$(date +%s)"
say "registering $INSCRIPTION as $STAFF_KEY"
h0=$(HASHCMD "$INSCRIPTION-0")
run jewel register "$INSCRIPTION" "$h0" "https://traceability.yamaletech.com/r/$h0" --from "$STAFF_KEY" \
  && ok "registered" || { bad "register"; exit 1; }
KEY=$(printf '%s' "$INSCRIPTION" | tr -d '-' | tr -d ' ')
ID=$("$BIN" query jewel inscription "$KEY" $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4 || true)
[ -n "$ID" ] || { bad "could not read the new jewel's id from the chain"; exit 1; }

say "49 amendments, 500-byte note each"
for i in $(seq 1 49); do
  h=$(HASHCMD "$INSCRIPTION-$i")
  run jewel amend "$ID" "$h" "https://traceability.yamaletech.com/r/$h" "$NOTE" --from "$STAFF_KEY" \
    || { bad "amendment $i"; exit 1; }
done
ok "49 applied"

# --dry-run does not open the keyring, so --from must be the bech32 address, and
# it refuses --gas auto: the simulation is the point, and it prints "gas estimate: N".
say "simulating the 50th"
h=$(HASHCMD "$INSCRIPTION-50")
STAFF_ADDR=$(addr "$STAFF_KEY" | tr -d '\r')
out=$("$BIN" tx jewel amend "$ID" "$h" "https://traceability.yamaletech.com/r/$h" "$NOTE" --from "$STAFF_ADDR" \
  --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN" --node "$NODE" \
  --gas-adjustment 1 --dry-run 2>&1) || { LAST_TX=$out; bad "simulation of amendment 50"; exit 1; }
gas=$(printf '%s' "$out" | grep -oE 'gas estimate: [0-9]+' | grep -oE '[0-9]+' | tail -1 || true)
[ -n "$gas" ] || { LAST_TX=$out; bad "no gas figure in the simulation's answer"; exit 1; }
echo "amendment 50 of 50: $gas gas"
