# Helpers shared by the jewel scripts on the local devnet.
#
# Sourced, never run. The caller sets BIN, HOME_DIR, KEYRING, TX, Q and
# M1..M3 first, exactly as seed-jewel.sh does; FAILED is set to 0 here.
FAILED=0
# How many of M1 M2 M3 vote as the office: 3 on the local 3-of-5, 2 on devnet-2's 2-of-3.
OFFICE_VOTES=${OFFICE_VOTES:-3}
case "$OFFICE_VOTES" in 1|2|3) ;; *) echo "OFFICE_VOTES must be 1, 2 or 3 (got '$OFFICE_VOTES'): the office votes with M1, M2, M3" >&2; exit 1 ;; esac

say()  { LAST_TX=""; printf '\n\033[1m==> %s\033[0m\n' "$*"; }
# LAST_TX holds the chain's raw answer (CLI tx/query/proposal JSON, never a key)
# to the transaction that last failed. bad prints it, bounded, then clears it;
# ok and say clear it too, so a later unrelated FAIL cannot print a stale answer.
LAST_TX=""
ok()   { LAST_TX=""; printf '  \033[32mok\033[0m   %s\n' "$*"; }
bad()  {
  printf '  \033[31mFAIL\033[0m %s\n' "$*"; FAILED=1
  if [ -n "$LAST_TX" ]; then
    printf '       the chain said:\n'
    printf '%s\n' "$LAST_TX" | head -n 20 | cut -c1-400 | sed 's/^/         /'
  fi
  LAST_TX=""
}
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }
json() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1
}
privhex() { yes | "$BIN" keys export "$1" --unarmored-hex --unsafe --keyring-backend "$KEYRING" --home "$HOME_DIR" 2>/dev/null | tail -1 || true; }  # `yes` dies of SIGPIPE; pipefail must not count it

# A transaction's code 0 is only CheckTx. Read the block's verdict.
committed() {
  local out=$1 hash code res
  LAST_TX=$out
  hash=$(printf '%s' "$out" | json txhash)
  [ -n "$hash" ] || return 1
  for _ in $(seq 1 20); do
    res=$("$BIN" query tx "$hash" $Q 2>/dev/null || true)
    code=$(printf '%s' "$res" | grep -oE '"code":[[:space:]]*[0-9]+' | head -1 | grep -oE '[0-9]+$' || true)
    if [ -n "$code" ]; then
      [ "$code" = 0 ] && { LAST_TX=""; return 0; }
      LAST_TX=$res; return 1
    fi
    sleep 1
  done
  LAST_TX="not in a block after 20 s: $out"
  return 1
}
run() { local out; out=$("$BIN" tx "$@" $TX -o json 2>&1) || { LAST_TX=$out; return 1; }; committed "$out"; }

# Everything a transaction said about itself: the CLI's own output (a CheckTx or
# simulation rejection has no hash, and its reason is only there) plus, when a
# hash exists, the block's verdict with its raw_log.
outcome() {
  local out=$1 hash res=""
  hash=$(printf '%s' "$out" | json txhash)
  if [ -n "$hash" ]; then
    for _ in $(seq 1 20); do
      res=$("$BIN" query tx "$hash" $Q 2>/dev/null || true)
      [ -n "$res" ] && break
      sleep 1
    done
  fi
  printf '%s\n%s' "$out" "$res"
}
# A refusal passes only for the stated reason. Any other failure (a keyring
# error, a wrong flag, no gas, an unfunded account) is a FAIL, not an ok.
expect_refused() {
  local label=$1 want=$2; shift 2
  local out; out=$("$BIN" tx "$@" $TX -o json 2>&1 || true)
  local res; res=$(outcome "$out")
  if printf '%s' "$res" | grep -qF -- "$want"; then ok "$label"; else LAST_TX=$res; bad "$label: refused, but not for the expected reason ($want)"; fi
}

# Broadcast one message built by jewelsign, signed by a keyring key.
broadcast_json() {
  local msg=$1 from=$2 body="$HOME_DIR/.jewel-tx.json" signed="$HOME_DIR/.jewel-signed.json"
  cat > "$body" <<JSON
{"body":{"messages":[$msg],"memo":"","timeout_height":"0","extension_options":[],"non_critical_extension_options":[]},
 "auth_info":{"signer_infos":[],"fee":{"amount":[],"gas_limit":"400000","payer":"","granter":""}},"signatures":[]}
JSON
  "$BIN" tx sign "$body" --from "$from" --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN" --node "$NODE" > "$signed" 2>&1 || { echo "sign failed"; return 1; }
  "$BIN" tx broadcast "$signed" --node "$NODE" -o json 2>&1 || true
}
run_json() { local out; out=$(broadcast_json "$@") || return 1; committed "$out"; }
expect_refused_json() {
  local label=$1 want=$2; shift 2
  local out; out=$(broadcast_json "$@" || true)
  local res; res=$(outcome "$out")
  if printf '%s' "$res" | grep -qF -- "$want"; then ok "$label"; else LAST_TX=$res; bad "$label: refused, but not for the expected reason ($want)"; fi
}

# Act as the office (a policy of M1..M3): propose, OFFICE_VOTES of them vote,
# the last with --exec try.
# Success is the proposal being pruned. See seed-herd.sh for why each part.
OFFICE=$("$BIN" query jewel params $Q | json office)
[ -n "$OFFICE" ] || { echo "no office in x/jewel params; rebuild the devnet with init-local-devnet.sh" >&2; exit 1; }
as_office() {
  local body=$1 msg="$HOME_DIR/.jewel-office.json" before id out proposal result
  cat > "$msg" <<JSON
{"group_policy_address":"$OFFICE","messages":[$body],"metadata":"seed-jewel","proposers":["$(addr "$M1")"]}
JSON
  # Each transaction is waited for in its block: a broadcast can exit 0 and
  # still be refused, and a fixed sleep races the commit.
  last_proposal() {
    "$BIN" query group proposals-by-group-policy "$OFFICE" $Q 2>/dev/null \
      | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true
  }
  before=$(last_proposal)
  out=$("$BIN" tx group submit-proposal "$msg" --from "$M1" $TX -o json 2>&1) || { LAST_TX=$out; return 1; }
  committed "$out" || return 1
  id=""
  for _ in $(seq 1 10); do
    id=$(last_proposal)
    [ -n "$id" ] && [ "$id" != "$before" ] && break
    id=""
    sleep 1
  done
  [ -n "$id" ] || return 1
  local n=0 voter execflag
  for voter in "$M1" "$M2" "$M3"; do
    n=$((n + 1)); [ "$n" -le "$OFFICE_VOTES" ] || break
    execflag=""; if [ "$n" -eq "$OFFICE_VOTES" ]; then execflag="--exec try"; fi
    out=$("$BIN" tx group vote "$id" "$(addr "$voter")" VOTE_OPTION_YES "" $execflag --from "$voter" $TX -o json 2>&1) || { LAST_TX=$out; return 1; }
    committed "$out" || return 1
  done
  for _ in $(seq 1 20); do
    proposal=$("$BIN" query group proposal "$id" $Q 2>/dev/null || true)
    [ -n "$proposal" ] || return 0
    result=$(printf '%s' "$proposal" | json executor_result)
    case "$result" in
      PROPOSAL_EXECUTOR_RESULT_SUCCESS) return 0 ;;
      PROPOSAL_EXECUTOR_RESULT_FAILURE) LAST_TX=$proposal; return 1 ;;
    esac
    sleep 1
  done
  return 1
}
