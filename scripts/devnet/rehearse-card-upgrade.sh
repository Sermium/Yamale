#!/usr/bin/env bash
# The jewel-cards upgrade, end to end, on a local chain, before devnet-2.
#
#   OLD=$SCRATCH/blockchaind-old.exe NEW=$SCRATCH/blockchaind.exe SIGN=$SCRATCH/jewelsign.exe \
#     scripts/devnet/rehearse-card-upgrade.sh
#
# OLD is built from the commit before the ownership card (93c3cbe), which is
# what devnet-2 runs in substance: x/jewel with offers, no cards. SIGN is
# tools/jewelsign from this commit, which can sign a card claim. The old chain
# registers and sells jewels first, so the upgrade meets a register that holds
# stock and sold pieces, as devnet-2's does; then the new binary prints a card
# for a jewel registered BEFORE the upgrade and the card moves it.
#
# Run from the repository root. It builds a FRESH local devnet (init-local-devnet.sh
# wipes .devnet/local) and leaves the upgraded node running; stop it with:
# kill "$(cat .devnet/local/node.pid)".
set -euo pipefail
OLD=${OLD:?the binary before the ownership card}; NEW=${NEW:?the binary with jewel-cards}; SIGN=${SIGN:?jewelsign}
HOME_DIR=.devnet/local/node; CHAIN=yamale-local-1; NODE=tcp://127.0.0.1:26957; KEYRING=test
NAME=jewel-cards
HERE=$(cd "$(dirname "$0")" && pwd)
LOG=.devnet/local/node.log; PIDFILE=.devnet/local/node.pid

# The helpers the steps before jewel-lib.sh can be sourced; the same as
# rehearse-jewel-upgrade.sh's.
say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mok\033[0m   %s\n' "$*"; }
die()  {
  printf '  \033[31mFAIL\033[0m %s\n' "$1"
  if [ -n "${2:-}" ]; then
    printf '       the chain said:\n'
    printf '%s\n' "$2" | head -n 20 | cut -c1-400 | sed 's/^/         /'
  fi
  exit 1
}
jget() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
height() {  # $1 binary
  "$1" status --node "$NODE" 2>/dev/null \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{process.stdout.write(String(JSON.parse(s).sync_info.latest_block_height))}catch(e){process.stdout.write("0")}})' 2>/dev/null || echo 0
}
GASARGS="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
# Broadcast a transaction and wait for the block's verdict; code 0 is only CheckTx.
tx_ok() {  # $1 binary, rest: tx args
  local bin=$1 out hash res code; shift
  out=$("$bin" tx "$@" $GASARGS -o json 2>&1) || die "tx $1 $2 refused" "$out"
  hash=$(printf '%s' "$out" | jget txhash)
  [ -n "$hash" ] || die "tx $1 $2 returned no hash" "$out"
  for _ in $(seq 1 20); do
    res=$("$bin" query tx "$hash" --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null || true)
    code=$(printf '%s' "$res" | grep -oE '"code":[[:space:]]*[0-9]+' | head -1 | grep -oE '[0-9]+$' || true)
    if [ -n "$code" ]; then
      [ "$code" = 0 ] && { TX_RESULT=$res; return 0; }
      die "tx $1 $2 failed in its block" "$res"
    fi
    sleep 1
  done
  die "tx $1 $2 not in a block after 20 s" "$out"
}
await_proposal() {  # $1 binary, $2 id
  local st
  for _ in $(seq 1 60); do
    st=$("$1" query gov proposal "$2" --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null | jget status || true)
    case "$st" in PROPOSAL_STATUS_PASSED|PROPOSAL_STATUS_REJECTED|PROPOSAL_STATUS_FAILED) echo "$st"; return 0 ;; esac
    sleep 2
  done
  echo "timeout:$st"
}
last_proposal() {
  "$1" query gov proposals --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true
}
gov_pass() {  # $1 binary, $2 title, $3 message JSON
  local bin=$1 file=.devnet/local/.proposal.json before st
  cat > "$file" <<JSON
{"messages":[$3],"metadata":"","deposit":"$DEPOSIT","title":"$2","summary":"$2 (local rehearsal)","expedited":false}
JSON
  before=$(last_proposal "$bin")
  tx_ok "$bin" gov submit-proposal "$file" --from validator
  PROP=""
  for _ in $(seq 1 10); do
    PROP=$(last_proposal "$bin")
    [ -n "$PROP" ] && [ "$PROP" != "$before" ] && break
    PROP=""; sleep 1
  done
  [ -n "$PROP" ] || die "the proposal did not appear"
  tx_ok "$bin" gov vote "$PROP" yes --from validator
  st=$(await_proposal "$bin" "$PROP")
  [ "$st" = PROPOSAL_STATUS_PASSED ] || die "proposal $PROP ended $st"
}
Q="--home $HOME_DIR --node $NODE -o json"
jewel_json() { "$1" query jewel jewel "$2" $Q 2>&1 | tr -d '\n'; }

if (echo > /dev/tcp/127.0.0.1/26957) 2>/dev/null; then
  echo "port 26957 is already in use; stop that node first" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
say "1  a fresh devnet on the OLD binary"
INIT_OUT=$(BIN="$OLD" "$HERE/init-local-devnet.sh" 2>&1) || { printf '%s\n' "$INIT_OUT" | tail -n 30; die "init-local-devnet.sh failed"; }
printf '%s\n' "$INIT_OUT" | tail -n 6
DEPOSIT=2000uyml
GOV=$("$OLD" query auth module-account gov $Q 2>/dev/null | grep -oE 'yml1[0-9a-z]{38,}' | head -1 || true)
[ -n "$GOV" ] || die "could not read the gov module account"
ok "gov module account $GOV"
if "$OLD" tx jewel --help 2>&1 | grep -q issue-card; then die "OLD already knows issue-card; it is not the pre-card binary"; fi
ok "OLD has no issue-card command"

# ---------------------------------------------------------------------------
say "2  the old register: the seeder's sale, plus two pieces left in stock"
BIN="$OLD" SIGN="$SIGN" HOME_DIR="$HOME_DIR" CHAIN="$CHAIN" NODE="$NODE" "$HERE/seed-jewel.sh" > .devnet/local/seed.out 2>&1 || true
if grep -q "FAIL" .devnet/local/seed.out; then tail -n 40 .devnet/local/seed.out; die "the seeder reported a FAIL on the old binary"; fi
grep -q "all stages behaved" .devnet/local/seed.out || { tail -n 40 .devnet/local/seed.out; die "the seeder did not reach its closing line"; }
ok "seeder: admitted, registered, sold, resold, benefit used, theft reported and cleared"
HASH=9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
tx_ok "$OLD" jewel register "MD-2026-0002" "$HASH" "https://traceability.yamaletech.com/r/$HASH" --from jw-staff
tx_ok "$OLD" jewel register "MD-2026-0003" "$HASH" "https://traceability.yamaletech.com/r/$HASH" --from jw-staff
RING=$("$OLD" query jewel inscription MD20260001 $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4 || true)
STOCK=$("$OLD" query jewel inscription MD20260002 $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4 || true)
STOCK2=$("$OLD" query jewel inscription MD20260003 $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4 || true)
JEWELER=$("$OLD" query jewel jewelers $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)
[ -n "$RING" ] && [ -n "$STOCK" ] && [ -n "$STOCK2" ] && [ -n "$JEWELER" ] || die "could not read the jewel ids"
ok "jeweler $JEWELER; sold ring $RING; stock $STOCK and $STOCK2, registered on the OLD binary"

# ---------------------------------------------------------------------------
say "3  schedule $NAME by governance"
H=$(height "$OLD"); PLAN=$((H + 40))
ok "height $H, plan at $PLAN"
gov_pass "$OLD" "Upgrade $NAME" "{\"@type\":\"/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade\",\"authority\":\"$GOV\",\"plan\":{\"name\":\"$NAME\",\"height\":\"$PLAN\",\"info\":\"\"}}"
ok "proposal $PROP passed"
PLANQ=$("$OLD" query upgrade plan $Q 2>&1 || true)
printf '%s' "$PLANQ" | grep -q "$NAME" || die "the plan is not scheduled" "$PLANQ"
ok "plan scheduled"

# ---------------------------------------------------------------------------
say "4  wait for the halt at $PLAN"
halted=""
for _ in $(seq 1 150); do
  if grep -q "UPGRADE \"$NAME\" NEEDED at height: $PLAN" "$LOG" 2>/dev/null; then halted=1; break; fi
  sleep 2
done
[ -n "$halted" ] || { tail -n 20 "$LOG" | cut -c1-300; die "the node did not halt for $NAME at $PLAN"; }
sleep 3
kill "$(cat "$PIDFILE")" 2>/dev/null || true
sleep 3
ok "halted at $PLAN; old node stopped"

# ---------------------------------------------------------------------------
say "5  restart on the NEW binary"
nohup "$NEW" start --home "$HOME_DIR" --minimum-gas-prices 0uyml > "$LOG" 2>&1 &
echo $! > "$PIDFILE"
h=0
for _ in $(seq 1 90); do
  h=$(height "$NEW"); case "$h" in ''|*[!0-9]*) h=0 ;; esac
  [ "$h" -gt "$PLAN" ] && break
  sleep 1
done
[ "$h" -gt "$PLAN" ] || { grep -iE "panic|error|upgrade" "$LOG" | tail -n 15 | cut -c1-400; die "the new binary did not pass $PLAN (height $h)"; }
ok "height $h, past the plan at $PLAN"

# ---------------------------------------------------------------------------
say "6  what the upgrade did"
AP=$("$NEW" query upgrade applied "$NAME" $Q 2>&1 || true)
printf '%s' "$AP" | grep -qE "\"height\":[[:space:]]*\"?$PLAN\"?" || die "the upgrade was not applied at $PLAN" "$AP"
ok "applied at $PLAN"
grep -q "ownership cards arrived" "$LOG" || die "the handler's log line is missing"
ok "the handler logged its arrival"
for id in "$RING" "$STOCK" "$STOCK2"; do
  J=$(jewel_json "$NEW" "$id")
  printf '%s' "$J" | grep -q '"jewel"' || die "jewel $id is not readable after the upgrade" "$J"
  if printf '%s' "$J" | grep -qE '"has_card":[[:space:]]*true|"claim":[[:space:]]*\{'; then die "jewel $id arrived with a card or a claim" "$J"; fi
done
ok "the old jewels read back, with no card and no claim"
CL=$("$NEW" query jewel claims "$JEWELER" $Q 2>&1 || true)
if printf '%s' "$CL" | grep -q '"claimer"'; then die "a claim exists after the upgrade" "$CL"; fi
ok "no claim is listed for jeweler $JEWELER"

# ---------------------------------------------------------------------------
# From here the office exists and the new binary answers, so jewel-lib.sh's
# helpers can be used: run, run_json, expect_refused, expect_refused_json.
BIN=$NEW; M1=custodian1; M2=custodian2; M3=custodian3
TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
. "$HERE/jewel-lib.sh"
bad() {
  printf '  \033[31mFAIL\033[0m %s\n' "$*"
  if [ -n "$LAST_TX" ]; then printf '%s\n' "$LAST_TX" | head -n 20 | cut -c1-400 | sed 's/^/         /'; fi
  exit 1
}
for k in jw-cardbuyer jw-stranger; do ensure_key "$k"; done
CARDBUYER=$(addr jw-cardbuyer); STRANGER=$(addr jw-stranger)
for a in "$CARDBUYER" "$STRANGER"; do
  run bank send "$(addr "$M3")" "$a" 10000000uyml --from "$M3" >/dev/null || bad "could not fund $a"
done

say "7  a card for a jewel registered before the upgrade, claimed and approved"
read -r CARD_PRIV CARD_PUB < <("$SIGN" claimkey)
run jewel issue-card "$STOCK" "$CARD_PUB" --from jw-staff && ok "card issued for jewel $STOCK" || bad "issue-card"
jewel_json "$NEW" "$STOCK" | grep -qE '"has_card":[[:space:]]*true' && ok "the jewel shows has_card" || bad "has_card is not set"
run_json "$("$SIGN" card "$CARD_PRIV" "$CHAIN" "$STOCK" "$CARDBUYER")" jw-cardbuyer && ok "claim filed by $CARDBUYER" || bad "file claim"
CL=$("$NEW" query jewel claims "$JEWELER" $Q 2>&1 | tr -d '\n' || true)
printf '%s' "$CL" | grep -q "\"claimer\":[[:space:]]*\"$CARDBUYER\"" && ok "the claim is listed for the jeweler" || { LAST_TX=$CL; bad "the claim is not listed"; }
J=$(jewel_json "$NEW" "$STOCK")
printf '%s' "$J" | grep -q "\"claimer\":[[:space:]]*\"$CARDBUYER\"" && ok "the verification query shows the claim" || { LAST_TX=$J; bad "Jewel does not show the claim"; }
BUSINESS=$(addr jw-business)
printf '%s' "$J" | grep -q "\"owner\":[[:space:]]*\"$BUSINESS\"" && ok "still the shop's until it approves" || { LAST_TX=$J; bad "the claim moved the jewel"; }
OUT=$("$BIN" tx jewel approve-claim "$STOCK" --from jw-staff $TX -o json 2>&1) || { LAST_TX=$OUT; bad "approve-claim refused"; }
committed "$OUT" || bad "approve-claim failed in its block"
AHASH=$(printf '%s' "$OUT" | json txhash)
ARES=$("$BIN" query tx "$AHASH" $Q 2>/dev/null | tr -d '\n' || true)
printf '%s' "$ARES" | grep -q "blockchain.jewel.v1.EventClaimApproved" && ok "EventClaimApproved emitted" || { LAST_TX=$ARES; bad "no EventClaimApproved"; }
# A typed event's attribute values are JSON, so the kind reads \"card\" in the
# transaction's JSON; no other attribute of these events is that string.
printf '%s' "$ARES" | grep -q "blockchain.jewel.v1.EventJewelTransferred" && printf '%s' "$ARES" | grep -qF '\"card\"' \
  && ok "EventJewelTransferred with kind card" || { LAST_TX=$ARES; bad "no EventJewelTransferred with kind card"; }
J=$(jewel_json "$NEW" "$STOCK")
printf '%s' "$J" | grep -q "\"owner\":[[:space:]]*\"$CARDBUYER\"" && ok "jewel $STOCK belongs to $CARDBUYER" || { LAST_TX=$J; bad "the jewel did not move"; }
if printf '%s' "$J" | grep -qE '"has_card":[[:space:]]*true|"claim":[[:space:]]*\{'; then LAST_TX=$J; bad "the card or the claim survived the transfer"; fi
ok "the card and the claim are spent"
expect_refused_json "the spent card claims nothing" "this jewel has no ownership card" \
  "$("$SIGN" card "$CARD_PRIV" "$CHAIN" "$STOCK" "$STRANGER")" jw-stranger

say "8  a stranger's claim rejected, a withdrawal, and a reprint that cancels the old card"
read -r C2_PRIV C2_PUB < <("$SIGN" claimkey)
run jewel issue-card "$STOCK2" "$C2_PUB" --from jw-staff && ok "card issued for jewel $STOCK2" || bad "issue-card"
run_json "$("$SIGN" card "$C2_PRIV" "$CHAIN" "$STOCK2" "$STRANGER")" jw-stranger && ok "a stranger files first" || bad "stranger's claim"
expect_refused_json "a second claim waits its turn" "a claim on this jewel is already waiting for the shop" \
  "$("$SIGN" card "$C2_PRIV" "$CHAIN" "$STOCK2" "$CARDBUYER")" jw-cardbuyer
run jewel reject-claim "$STOCK2" --from jw-staff && ok "staff reject it" || bad "reject-claim"
run_json "$("$SIGN" card "$C2_PRIV" "$CHAIN" "$STOCK2" "$CARDBUYER")" jw-cardbuyer && ok "the same card still works for the buyer" || bad "claim after reject"
expect_refused "only the claimer withdraws" "this claim was filed by someone else" jewel withdraw-claim "$STOCK2" --from jw-stranger
run jewel withdraw-claim "$STOCK2" --from jw-cardbuyer && ok "the buyer withdraws" || bad "withdraw-claim"
read -r C3_PRIV C3_PUB < <("$SIGN" claimkey)
run jewel issue-card "$STOCK2" "$C3_PUB" --from jw-staff && ok "the card is reprinted" || bad "reprint"
expect_refused_json "the old card is cancelled" "this is not the jewel's current ownership card" \
  "$("$SIGN" card "$C2_PRIV" "$CHAIN" "$STOCK2" "$CARDBUYER")" jw-cardbuyer
expect_refused "no card for a jewel that left the shop" "only the jewel's owner may do this" \
  jewel issue-card "$RING" "$C3_PUB" --from jw-staff
CL=$("$NEW" query jewel claims "$JEWELER" $Q 2>&1 || true)
if printf '%s' "$CL" | grep -q '"claimer"'; then LAST_TX=$CL; bad "a claim is still listed"; fi
ok "no claim is left in the jeweler's list"

[ "$FAILED" = 0 ] || exit 1
echo
echo "rehearsal passed"
