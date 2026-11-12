#!/usr/bin/env bash
#
# Puts a jeweler, a ring and a resale through the jewel register on the local
# devnet, and shows the refusals that make it worth having.
#
# What it demonstrates, in order:
#   1. the foundation office admits "Maison Diop" (a 3-of-5 group decision)
#   2. the business adds a staff member; staff register ring MD-2026-0001
#   3. a second registration of the same inscription is refused
#   4. a bare `tx nft send` is refused at the router
#   5. staff sell the ring to a customer by email: a claim key
#   6. the customer resells it to a buyer, who accepts
#   7. a cleaning benefit is attached and used with the buyer's signed code
#   8. the buyer reports it stolen; an offer is refused; the flag is cleared
#
# Usage, against scripts/devnet/init-local-devnet.sh:
#
#   BIN=$SCRATCH/blockchaind.exe SIGN=$SCRATCH/jewelsign.exe \
#   HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 NODE=tcp://127.0.0.1:26957 \
#   M1=custodian1 M2=custodian2 M3=custodian3 scripts/devnet/seed-jewel.sh
set -euo pipefail

BIN=${BIN:?the chain binary}
SIGN=${SIGN:?the jewelsign binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"

say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mok\033[0m   %s\n' "$*"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$*"; FAILED=1; }
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }
json() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1
}
privhex() { yes | "$BIN" keys export "$1" --unarmored-hex --unsafe --keyring-backend "$KEYRING" --home "$HOME_DIR" 2>/dev/null | tail -1; }
FAILED=0

# A transaction's code 0 is only CheckTx. Read the block's verdict.
committed() {
  local out=$1 hash code
  hash=$(printf '%s' "$out" | json txhash)
  [ -n "$hash" ] || return 1
  for _ in $(seq 1 20); do
    code=$("$BIN" query tx "$hash" $Q 2>/dev/null | grep -oE '"code":[[:space:]]*[0-9]+' | head -1 | grep -oE '[0-9]+$' || true)
    [ -n "$code" ] && { [ "$code" = 0 ]; return; }
    sleep 1
  done
  return 1
}
run() { local out; out=$("$BIN" tx "$@" $TX -o json 2>&1) || return 1; committed "$out"; }

# Broadcast one message built by jewelsign, signed by a keyring key.
run_json() {
  local msg=$1 from=$2 body="$HOME_DIR/.jewel-tx.json" signed="$HOME_DIR/.jewel-signed.json" out
  cat > "$body" <<JSON
{"body":{"messages":[$msg],"memo":"","timeout_height":"0","extension_options":[],"non_critical_extension_options":[]},
 "auth_info":{"signer_infos":[],"fee":{"amount":[],"gas_limit":"400000","payer":"","granter":""}},"signatures":[]}
JSON
  "$BIN" tx sign "$body" --from "$from" --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN" --node "$NODE" > "$signed" || return 1
  out=$("$BIN" tx broadcast "$signed" --node "$NODE" -o json 2>&1) || return 1
  committed "$out"
}

# Act as the office (a 3-of-5 policy): propose, three members vote, the third
# with --exec try.
# Success is the proposal being pruned. See seed-herd.sh for why each part.
OFFICE=$("$BIN" query jewel params $Q | json office)
[ -n "$OFFICE" ] || { echo "no office in x/jewel params; rebuild the devnet with init-local-devnet.sh" >&2; exit 1; }
as_office() {
  local body=$1 msg="$HOME_DIR/.jewel-office.json" id proposal result
  cat > "$msg" <<JSON
{"group_policy_address":"$OFFICE","messages":[$body],"metadata":"seed-jewel","proposers":["$(addr "$M1")"]}
JSON
  "$BIN" tx group submit-proposal "$msg" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 3
  id=$("$BIN" query group proposals-by-group-policy "$OFFICE" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)
  [ -n "$id" ] || return 1
  "$BIN" tx group vote "$id" "$(addr "$M1")" VOTE_OPTION_YES "" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 2
  "$BIN" tx group vote "$id" "$(addr "$M2")" VOTE_OPTION_YES "" --from "$M2" $TX >/dev/null 2>&1 || return 1
  sleep 2
  "$BIN" tx group vote "$id" "$(addr "$M3")" VOTE_OPTION_YES "" --exec try --from "$M3" $TX >/dev/null 2>&1 || return 1
  for _ in $(seq 1 20); do
    proposal=$("$BIN" query group proposal "$id" $Q 2>/dev/null || true)
    [ -n "$proposal" ] || return 0
    result=$(printf '%s' "$proposal" | json executor_result)
    case "$result" in
      PROPOSAL_EXECUTOR_RESULT_SUCCESS) return 0 ;;
      PROPOSAL_EXECUTOR_RESULT_FAILURE) return 1 ;;
    esac
    sleep 1
  done
  return 1
}

for k in jw-business jw-staff jw-customer jw-buyer; do ensure_key "$k"; done
BUSINESS=$(addr jw-business); STAFF=$(addr jw-staff); CUSTOMER=$(addr jw-customer); BUYER=$(addr jw-buyer)
for a in "$BUSINESS" "$STAFF" "$CUSTOMER" "$BUYER"; do
  run bank send "$(addr "$M3")" "$a" 10000000uyml --from "$M3" >/dev/null || true
done

say "1  the office admits Maison Diop"
as_office "{\"@type\":\"/blockchain.jewel.v1.MsgAdmitJeweler\",\"office\":\"$OFFICE\",\"name\":\"Maison Diop\",\"business\":\"$BUSINESS\"}" \
  && ok "admitted" || bad "admission"
JEWELER=$("$BIN" query jewel jewelers $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4)

say "2  staff, then a ring"
run jewel set-staff "$JEWELER" --add "$STAFF" --from jw-business && ok "staff added" || bad "staff"
HASH=9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
run jewel register "MD-2026-0001" "$HASH" "https://loyalty.yamaletech.com/r/$HASH" --from jw-staff && ok "ring registered" || bad "register"
RING=$("$BIN" query jewel inscription MD20260001 $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4)

say "3  the same inscription again"
run jewel register "md 2026 0001" "$HASH" "u" --from jw-staff && bad "a duplicate was accepted" || ok "refused"

say "4  a bare x/nft send"
run nft send "jewel-$JEWELER" "jewel-$RING" "$BUYER" --from jw-business && bad "the bare send went through" || ok "refused at the router"

say "5  sold by email: a claim key"
read -r CLAIM_PRIV CLAIM_PUB < <("$SIGN" claimkey)
run jewel offer "$RING" --claim-key "$CLAIM_PUB" --from jw-staff && ok "offered to a claim key" || bad "offer"
run_json "$("$SIGN" claim "$CLAIM_PRIV" "$CHAIN" "$RING" "$CUSTOMER")" jw-customer && ok "claimed by the customer" || bad "claim"

say "6  resold, owner to owner"
run jewel offer "$RING" --to "$BUYER" --from jw-customer && ok "offered" || bad "offer"
run jewel accept "$RING" --from jw-buyer && ok "accepted" || bad "accept"

say "7  a cleaning, used with the buyer's code"
run jewel attach-benefit "$RING" cleaning 2 --period 31536000 --from jw-staff && ok "attached" || bad "attach"
BENEFIT=$("$BIN" query jewel jewel "$RING" $Q | sed -n '/"benefits"/,$p' | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4)
EXPIRES=$(( $(date +%s) + 240 ))
VOUCHER=$("$SIGN" voucher "$(privhex jw-buyer)" "$CHAIN" "$STAFF" "$BENEFIT" "$RING" "seed-1" "$EXPIRES")
run_json "$VOUCHER" jw-staff && ok "used" || bad "redeem"
run_json "$VOUCHER" jw-staff && bad "the same code was used twice" || ok "a second use of the same code is refused"

say "8  stolen"
run jewel report-stolen "$RING" --from jw-buyer && ok "reported" || bad "report"
run jewel offer "$RING" --to "$CUSTOMER" --from jw-buyer && bad "a stolen jewel was offered" || ok "an offer is refused"
run jewel clear-stolen "$RING" --from jw-buyer && ok "cleared" || bad "clear"

say "the record"
"$BIN" query jewel jewel "$RING" $Q
[ "$FAILED" = 0 ] && echo "all stages behaved" || { echo "some stages did not behave; see FAIL above" >&2; exit 1; }
