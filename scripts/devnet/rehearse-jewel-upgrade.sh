#!/usr/bin/env bash
# The jewel-register upgrade, end to end, on a local chain, before devnet-2.
#
#   OLD=$SCRATCH/blockchaind-old.exe NEW=$SCRATCH/blockchaind.exe SIGN=$SCRATCH/jewelsign.exe \
#     scripts/devnet/rehearse-jewel-upgrade.sh
#
# OLD is built from the commit before x/jewel was wired (56feacc^), which is
# what devnet-2 runs in substance. The chain starts with x/nft's own send OPEN,
# so the rehearsal proves the upgrade closes it rather than inheriting it.
#
# Run from the repository root. It builds a FRESH local devnet (init-local-devnet.sh
# wipes .devnet/local) and leaves the upgraded node running, so the app smoke
# test can use it; stop it with: kill "$(cat .devnet/local/node.pid)".
set -euo pipefail
OLD=${OLD:?the binary before x/jewel}; NEW=${NEW:?the binary with jewel-register}; SIGN=${SIGN:?jewelsign}
HOME_DIR=.devnet/local/node; CHAIN=yamale-local-1; NODE=tcp://127.0.0.1:26957; KEYRING=test
NAME=jewel-register
HERE=$(cd "$(dirname "$0")" && pwd)
LOG=.devnet/local/node.log; PIDFILE=.devnet/local/node.pid

# Until the office exists jewel-lib.sh cannot be sourced (it reads the office at
# load), so the few helpers the early steps need are defined here.
say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mok\033[0m   %s\n' "$*"; }
bad()  {
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
  out=$("$bin" tx "$@" $GASARGS -o json 2>&1) || bad "tx $1 $2 refused" "$out"
  hash=$(printf '%s' "$out" | jget txhash)
  [ -n "$hash" ] || bad "tx $1 $2 returned no hash" "$out"
  for _ in $(seq 1 20); do
    res=$("$bin" query tx "$hash" --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null || true)
    code=$(printf '%s' "$res" | grep -oE '"code":[[:space:]]*[0-9]+' | head -1 | grep -oE '[0-9]+$' || true)
    if [ -n "$code" ]; then
      [ "$code" = 0 ] && return 0
      bad "tx $1 $2 failed in its block" "$res"
    fi
    sleep 1
  done
  bad "tx $1 $2 not in a block after 20 s" "$out"
}
# Wait for a gov proposal to leave the voting period; prints its status.
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
# Submit a one-message gov proposal from `validator`, vote yes; its id is left in PROP.
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
  [ -n "$PROP" ] || bad "the proposal did not appear"
  tx_ok "$bin" gov vote "$PROP" yes --from validator
  st=$(await_proposal "$bin" "$PROP")
  [ "$st" = PROPOSAL_STATUS_PASSED ] || bad "proposal $PROP ended $st"
}

if (echo > /dev/tcp/127.0.0.1/26957) 2>/dev/null; then
  echo "port 26957 is already in use; stop that node first" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
say "1  a fresh devnet on the OLD binary, x/nft's send open"
INIT_OUT=$(SKIP_NFT_BREAKER=1 BIN="$OLD" "$HERE/init-local-devnet.sh" 2>&1) || { printf '%s\n' "$INIT_OUT" | tail -n 30; bad "init-local-devnet.sh failed"; }
printf '%s\n' "$INIT_OUT" | tail -n 12
DEPOSIT=2000uyml
GOV=$("$OLD" query auth module-account gov --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null | grep -oE 'yml1[0-9a-z]{38,}' | head -1 || true)
[ -n "$GOV" ] || bad "could not read the gov module account"
ok "gov module account $GOV"
DL=$("$OLD" query circuit disabled-list --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
if printf '%s' "$DL" | grep -qi nft; then bad "the old chain already disables an nft message" "$DL"; fi
ok "disabled-list has no nft entry before the upgrade"

# ---------------------------------------------------------------------------
say "2  schedule $NAME by governance"
H=$(height "$OLD"); PLAN=$((H + 40))
ok "height $H, plan at $PLAN"
gov_pass "$OLD" "Upgrade $NAME" "{\"@type\":\"/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade\",\"authority\":\"$GOV\",\"plan\":{\"name\":\"$NAME\",\"height\":\"$PLAN\",\"info\":\"\"}}"
ok "proposal $PROP passed"
PLANQ=$("$OLD" query upgrade plan --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
printf '%s' "$PLANQ" | grep -q "$NAME" || bad "the plan is not scheduled" "$PLANQ"
ok "plan scheduled"

# ---------------------------------------------------------------------------
say "3  wait for the halt at $PLAN"
halted=""
for _ in $(seq 1 120); do
  if grep -q "UPGRADE \"$NAME\" NEEDED at height: $PLAN" "$LOG" 2>/dev/null; then halted=1; break; fi
  sleep 2
done
[ -n "$halted" ] || { tail -n 20 "$LOG" | cut -c1-300; bad "the node did not halt for $NAME at $PLAN"; }
# The halt is a panic, so the process should already be gone; stop it if it is not.
sleep 3
kill "$(cat "$PIDFILE")" 2>/dev/null || true
sleep 3
ok "halted at $PLAN; old node stopped"

# ---------------------------------------------------------------------------
say "4  restart on the NEW binary"
nohup "$NEW" start --home "$HOME_DIR" --minimum-gas-prices 0uyml > "$LOG" 2>&1 &
echo $! > "$PIDFILE"
h=0
for _ in $(seq 1 90); do
  h=$(height "$NEW"); case "$h" in ''|*[!0-9]*) h=0 ;; esac
  [ "$h" -gt "$PLAN" ] && break
  sleep 1
done
[ "$h" -gt "$PLAN" ] || { grep -iE "panic|error|upgrade" "$LOG" | tail -n 15 | cut -c1-400; bad "the new binary did not pass $PLAN (height $h)"; }
ok "height $h, past the plan at $PLAN"

# ---------------------------------------------------------------------------
say "5  what the upgrade did"
AP=$("$NEW" query upgrade applied "$NAME" --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
printf '%s' "$AP" | grep -qE "\"height\":[[:space:]]*\"?$PLAN\"?" || bad "the upgrade was not applied at $PLAN" "$AP"
ok "applied at $PLAN"
DL=$("$NEW" query circuit disabled-list --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
printf '%s' "$DL" | grep -q "/cosmos.nft.v1beta1.MsgSend" || bad "x/nft's send is not disabled" "$DL"
ok "x/nft MsgSend is disabled"
PAR=$("$NEW" query jewel params --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
# An empty office is omitted from the JSON, so "empty" means no office field at all.
if printf '%s' "$PAR" | grep -qE '"office":[[:space:]]*"[^"]'; then bad "the office is not empty" "$PAR"; fi
printf '%s' "$PAR" | grep -qE '"offer_ttl":[[:space:]]*"?1209600' || bad "offer_ttl is not 1209600" "$PAR"
ok "jewel params: empty office, offer_ttl 1209600"

# ---------------------------------------------------------------------------
say "6  governance appoints the foundation as the office"
POLICY=$("$NEW" query group group-policies-by-group 1 --home "$HOME_DIR" --node "$NODE" -o json 2>&1 | jget address || true)
[ -n "$POLICY" ] || bad "could not read the foundation's group policy"
NEWPARAMS=$(printf '%s' "$PAR" | POLICY="$POLICY" node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const p=JSON.parse(s).params;p.office=process.env.POLICY;process.stdout.write(JSON.stringify(p))})')
gov_pass "$NEW" "Appoint the jewel office" "{\"@type\":\"/blockchain.jewel.v1.MsgUpdateParams\",\"authority\":\"$GOV\",\"params\":$NEWPARAMS}"
ok "proposal $PROP passed"
PAR=$("$NEW" query jewel params --home "$HOME_DIR" --node "$NODE" -o json 2>&1 || true)
printf '%s' "$PAR" | grep -q "\"office\":[[:space:]]*\"$POLICY\"" || bad "the office is not the foundation policy" "$PAR"
ok "office is $POLICY"

# ---------------------------------------------------------------------------
say "7  the seeder"
BIN="$NEW" SIGN="$SIGN" HOME_DIR="$HOME_DIR" CHAIN="$CHAIN" NODE="$NODE" "$HERE/seed-jewel.sh" > .devnet/local/seed.out 2>&1 || true
tail -n 80 .devnet/local/seed.out
if grep -q "FAIL" .devnet/local/seed.out; then bad "the seeder reported a FAIL (full output in .devnet/local/seed.out)"; fi
grep -q "all stages behaved" .devnet/local/seed.out || bad "the seeder did not reach its closing line"
grep -q "admitted" .devnet/local/seed.out || bad "the seeder's stage 1 (admission) did not report"
ok "every seeder stage reported ok"

# ---------------------------------------------------------------------------
say "8  the heaviest message"
BIN="$NEW" HOME_DIR="$HOME_DIR" CHAIN="$CHAIN" NODE="$NODE" "$HERE/measure-amend-gas.sh" > .devnet/local/gas.out 2>&1 || { tail -n 20 .devnet/local/gas.out; bad "measure-amend-gas.sh failed"; }
tail -n 8 .devnet/local/gas.out
grep -q "^amendment 50 of 50: [0-9]* gas" .devnet/local/gas.out || bad "no gas line"

echo
echo "rehearsal passed"
