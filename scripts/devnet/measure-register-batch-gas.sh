#!/usr/bin/env bash
#
# Measures the gas of one transaction carrying N MsgRegisterJewel, the batch
# the catalogue import sends (REGISTER_BATCH in
# clients/traceability/src/atelier/register.ts). The batch must fit JEWEL_GAS
# (1,650,000) with at least 25% headroom: at most 1,237,500 gas.
#
# Run it on a local devnet right after seed-jewel.sh (which leaves jw-staff),
# for N = 4, 5, 6, ... until it prints OVER:
#
#   BIN=$SCRATCH/blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 N=5 scripts/devnet/measure-register-batch-gas.sh
set -euo pipefail

BIN=${BIN:?the chain binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
STAFF_KEY=${STAFF_KEY:-jw-staff}
N=${N:-5}
BUDGET=1237500

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"
. "$(dirname "$0")/jewel-lib.sh"

TMP=$(mktemp -d)
STAFF_ADDR=$(addr "$STAFF_KEY" | tr -d '\r')
PREFIX="B$(date +%s)X"
h0=$(printf '%s' "${PREFIX}0" | sha256sum | cut -d' ' -f1)

"$BIN" tx jewel register "${PREFIX}0" "$h0" "https://traceability.yamaletech.com/r/$h0" --from "$STAFF_ADDR" \
  --chain-id "$CHAIN" --node "$NODE" --home "$HOME_DIR" --keyring-backend "$KEYRING" --gas 1650000 --generate-only > "$TMP/one.json"

node -e '
const fs = require("fs"), crypto = require("crypto");
const [file, n, prefix] = process.argv.slice(1);
const tx = JSON.parse(fs.readFileSync(file, "utf8"));
const m = tx.body.messages[0];
tx.body.messages = [];
for (let i = 0; i < Number(n); i++) {
  const h = crypto.createHash("sha256").update(prefix + i).digest("hex");
  tx.body.messages.push({ ...m, inscription: prefix + i, record_hash: h, record_uri: "https://traceability.yamaletech.com/r/" + h });
}
process.stdout.write(JSON.stringify(tx));
' "$TMP/one.json" "$N" "$PREFIX" > "$TMP/many.json"

"$BIN" tx sign "$TMP/many.json" --from "$STAFF_KEY" --chain-id "$CHAIN" --node "$NODE" --home "$HOME_DIR" \
  --keyring-backend "$KEYRING" > "$TMP/signed.json"
out=$("$BIN" tx broadcast "$TMP/signed.json" --node "$NODE" -o json)
hash=$(printf '%s' "$out" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const j=JSON.parse(s);if(j.code){console.error("refused at CheckTx: "+j.raw_log);process.exit(1)}console.log(j.txhash)})')

for i in $(seq 1 30); do
  res=$("$BIN" query tx "$hash" $Q 2>/dev/null || true)
  [ -n "$res" ] && break
  sleep 1
done
[ -n "$res" ] || { echo "the transaction was not committed in 30 s"; exit 1; }
printf '%s' "$res" | node -e '
let s=""; process.stdin.on("data",d=>s+=d).on("end",()=>{
  const j = JSON.parse(s), n = Number(process.argv[1]), budget = Number(process.argv[2]);
  if (j.code) { console.log(`${n} registrations: refused (${j.raw_log})`); process.exit(1); }
  const g = Number(j.gas_used);
  console.log(`${n} registrations in one transaction: ${g} gas (budget ${budget}) ${g <= budget ? "fits" : "OVER"}`);
});' "$N" "$BUDGET"
