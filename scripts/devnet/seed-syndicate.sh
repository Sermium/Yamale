#!/usr/bin/env bash
#
# Runs one syndicated deal end to end on a live chain.
#
# # What this demonstrates
#
# Not "a token exists". The three things the platform is actually sold on:
#
#   1. A deal that MISSES its minimum gives every cheque back, in full,
#      including the fees — and anybody can make that happen, not just the
#      platform. That is the risk an investor takes before there is anything to
#      own, and it is the one a slide deck cannot answer.
#
#   2. A deal that closes issues ownership against the GROSS cheque while the
#      fees come out of the raise, so the three numbers an investor is entitled
#      to see — raised, fees, invested — are on the record together rather than
#      reconciled from a statement.
#
#   3. A distribution reaches whoever holds the token AT THAT MOMENT. Stage 4
#      sells half a position privately BETWEEN two distributions and then shows
#      that both parties were paid for exactly the period they held. Nobody
#      reconciled anything; there is no chain of historical owners to track.
#
# # Usage
#
#   scripts/devnet/seed-syndicate.sh operator    stage 1 — group + governance
#   scripts/devnet/seed-syndicate.sh failed      stage 2 — a deal that refunds
#   scripts/devnet/seed-syndicate.sh deal        stage 3 — a deal that closes
#   scripts/devnet/seed-syndicate.sh secondary   stage 4 — sale between payouts
#   scripts/devnet/seed-syndicate.sh show        print what to query
#
# Against the local devnet from scripts/devnet/init-local-devnet.sh:
#
#   BIN=./blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 M1=custodian1 M2=custodian2 M3=custodian3 \
#   scripts/devnet/seed-syndicate.sh operator
set -euo pipefail

BIN=${BIN:-/opt/yamale/bin/blockchaind}
HOME_DIR=${HOME_DIR:-/opt/yamale/node}
KEYRING=${KEYRING:-test}
CHAIN=${CHAIN:-yamale-devnet-2}
NODE=${NODE:-http://127.0.0.1:26657}

# The operator is 2-of-3 between three existing funded keys. The module refuses
# a plain key outright: an operator opens deals, decides they have closed and
# moves escrowed money to a drawdown account, and each of those on one key is
# one key away from a platform that takes money and does not invest it.
M1=${M1:-adm-ba}
M2=${M2:-adm-diallo}
M3=${M3:-adm-fall}

# The settlement currency. USD or a USD-pegged stablecoin in production — one
# denomination for the whole deal, because accounting and investor
# administration across several African currencies is the burden the platform
# exists to remove. On a devnet, whatever the chain issues.
DENOM=${DENOM:-uyml}

# Three investors and the partner firm.
ALICE=${ALICE:-syn-alice}
BOB=${BOB:-syn-bob}
CAROL=${CAROL:-syn-carol}
PARTNER=${PARTNER:-syn-partner}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"

say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }

# `-o json` is pretty-printed locally and compact on the deployment, so every
# pattern tolerates the space. A pattern written for one silently matches
# nothing on the other, and matching nothing is indistinguishable from an empty
# answer. grep exiting 1 on no match would also kill the script under pipefail.
json() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
jnum() { grep -oE "\"$1\":[[:space:]]*[0-9]+" | head -1 | grep -oE '[0-9]+$' || true; }

OPERATOR_FILE="$HOME_DIR/syndicate-operator.addr"

ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1
}

fund() {
  local who=$1 amount=$2
  local a; a=$(addr "$who")
  local have; have=$("$BIN" query bank balances "$a" $Q 2>/dev/null | json amount)
  [ -n "$have" ] && return 0
  "$BIN" tx bank send "$(addr "$M3")" "$a" "$amount$DENOM" --from "$M3" $TX >/dev/null 2>&1 \
    && printf '  funded %-12s %s\n' "$who" "$amount$DENOM" || printf '  could not fund %s\n' "$who"
  sleep 3
}

# The group policy created most recently with a given metadata string.
#
# NOT group-policies-by-admin: these groups are created with
# --group-policy-as-admin, so each policy is its own admin and querying by the
# creator returns nothing at all.
policy_named() {
  local want=$1 found=""
  local ids; ids=$("$BIN" query group groups $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 || true)
  for gid in $ids; do
    local out; out=$("$BIN" query group group-policies-by-group "$gid" $Q 2>/dev/null || true)
    if printf '%s' "$out" | grep -qE "\"metadata\":[[:space:]]*\"$want\""; then
      found=$(printf '%s' "$out" | grep -oE '"address":[[:space:]]*"yml1[a-z0-9]+"' \
        | head -1 | cut -d'"' -f4 || true)
    fi
  done
  printf '%s' "$found"
}

# Submit one message AS the operator, and carry it over the 2-of-3 threshold.
#
# Three transactions, and each shortcut was tried and failed in x/herd's seeder:
# naming both members as proposers needs two signatures on one transaction;
# omitting group_policy_address is refused; and submitting does NOT record the
# proposer's vote, which fails silently — the transaction succeeds, the vote
# succeeds, and the message never executes.
as_operator() {
  local body=$1 note=$2 office=$3
  local msg="$HOME_DIR/.syn-msg.json"
  cat > "$msg" <<JSON
{"group_policy_address":"$office",
 "messages":[$body],
 "metadata":"$note",
 "proposers":["$(addr "$M1")"]}
JSON
  "$BIN" tx group submit-proposal "$msg" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 3

  local id
  id=$("$BIN" query group proposals-by-group-policy "$office" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)
  [ -n "$id" ] || return 1

  "$BIN" tx group vote "$id" "$(addr "$M1")" VOTE_OPTION_YES "" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 2
  "$BIN" tx group vote "$id" "$(addr "$M2")" VOTE_OPTION_YES "" --exec try \
    --from "$M2" $TX >/dev/null 2>&1 || return 1

  # Success is the proposal being GONE: x/group prunes one that executes. A
  # check for executor_result SUCCESS finds nothing and reports failure, and a
  # fixed sleep before looking is a coin toss — both were learnt the hard way.
  local proposal result
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

operator_addr() {
  local a; a=$(cat "$OPERATOR_FILE" 2>/dev/null || true)
  [ -n "$a" ] || { echo "no operator address; run '$0 operator' first" >&2; exit 1; }
  printf '%s' "$a"
}

deal_id() { "$BIN" query syndicate deal-by-ref "$1" $Q 2>/dev/null | json id; }

# --------------------------------------------------------------- stage 1
stage_operator() {
  say "1/4  the platform operator, as a 2-of-3 group"
  for k in "$ALICE" "$BOB" "$CAROL" "$PARTNER"; do ensure_key "$k"; done
  a1=$(addr "$M1"); a2=$(addr "$M2"); a3=$(addr "$M3")

  members="$HOME_DIR/.syn-members.json"
  cat > "$members" <<JSON
{"members":[
  {"address":"$a1","weight":"1","metadata":"member 1"},
  {"address":"$a2","weight":"1","metadata":"member 2"},
  {"address":"$a3","weight":"1","metadata":"member 3"}
]}
JSON
  policy="$HOME_DIR/.syn-policy.json"
  cat > "$policy" <<'JSON'
{"@type":"/cosmos.group.v1.ThresholdDecisionPolicy","threshold":"2",
 "windows":{"voting_period":"120s","min_execution_period":"0s"}}
JSON

  META="Plateforme de syndication Yamale"
  "$BIN" tx group create-group-with-policy "$a1" \
    "Yamale Syndication" "$META" "$members" "$policy" \
    --group-policy-as-admin --from "$M1" $TX >/dev/null
  sleep 6

  operator=$(policy_named "$META")
  [ -n "$operator" ] || { echo "could not find the operator policy" >&2; exit 1; }
  echo "$operator" > "$OPERATOR_FILE"
  printf '  operator: %s\n' "$operator"

  # The operator needs coins of its own: it pays gas on nothing here, but it
  # receives fees, and an account the chain has never seen cannot be a
  # recipient of a module send until it exists.
  fund "$ALICE" 5000000
  fund "$BOB" 5000000
  fund "$CAROL" 5000000
  fund "$PARTNER" 20000000

  say "2/4  governance proposal admitting it"
  gov=$("$BIN" query auth module-account gov $Q | json address)
  prop="$HOME_DIR/.syn-proposal.json"
  cat > "$prop" <<JSON
{
  "messages": [{
    "@type": "/blockchain.syndicate.v1.MsgRegisterOperator",
    "authority": "$gov",
    "operator": "$operator",
    "name": "Yamale Syndication Platform",
    "jurisdiction": "SN",
    "active": true
  }],
  "metadata": "",
  "deposit": "1000000uyml",
  "title": "Syndicate: admit the platform operator",
  "summary": "Admits a 2-of-3 group account as a syndication platform operator. A group rather than a key because an operator opens deals, decides they have closed, moves escrowed investor capital to a drawdown account and attests that investors completed KYC — each of those on one signature is one signature away from a platform that takes money and does not invest it, and these investors are spread across jurisdictions with no practical recourse. What governance controls is which platforms exist at all; onboarding investors is then the platform's own act, which is where that decision belongs."
}
JSON
  "$BIN" tx gov submit-proposal "$prop" --from "$M1" $TX >/dev/null
  sleep 6
  id=$("$BIN" query gov proposals $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)

  cat <<TEXT

  Proposal ${id:-<see 'query gov proposals'>} submitted. Vote, then run:

    $0 failed     a deal that misses its minimum and refunds everybody
    $0 deal       a deal that closes and pays out

TEXT
}

# onboard runs KYC onboarding for one investor, as the operator.
onboard() {
  local who=$1 operator=$2
  if as_operator "{\"@type\":\"/blockchain.syndicate.v1.MsgOnboardInvestor\",
    \"operator\":\"$operator\",\"investor\":\"$(addr "$who")\",
    \"reference\":\"KYC/2026/$who\",\"jurisdiction\":\"SN\",\"active\":true}" \
    "onboarding $who" "$operator"; then
    printf '  ok    %-12s onboarded\n' "$who"
  else
    printf '  FAIL  %-12s onboarding refused\n' "$who"
  fi
}

open_deal() {
  local operator=$1 ref=$2 company=$3 target=$4 minimum=$5 ticket=$6
  local closes=$(( $(date +%s) + 90000 ))   # just over a day: above the floor
  as_operator "{\"@type\":\"/blockchain.syndicate.v1.MsgOpenDeal\",
    \"operator\":\"$operator\",\"reference\":\"$ref\",\"company\":\"$company\",
    \"partner\":\"Partech Africa\",\"drawdown_account\":\"$(addr "$PARTNER")\",
    \"settlement_denom\":\"$DENOM\",\"target\":\"$target\",\"minimum\":\"$minimum\",
    \"minimum_ticket\":\"$ticket\",
    \"fees\":{\"sourcing_bps\":200,\"syndication_bps\":100,\"management_bps\":2000},
    \"closes_at\":\"$closes\"}" "$ref" "$operator"
}

# --------------------------------------------------------------- stage 2
stage_failed() {
  operator=$(operator_addr)
  say "3/4  a deal that misses its minimum — and gives every cheque back"

  for who in "$ALICE" "$BOB" "$CAROL"; do onboard "$who" "$operator"; done

  if open_deal "$operator" "SYN-2026-FAIL" "Kigali Fintech" 4000000 3000000 500000; then
    echo "  ok    deal opened, target 4000000, minimum 3000000"
  else
    echo "  FAIL  deal not opened"; exit 1
  fi
  id=$(deal_id "SYN-2026-FAIL")

  before=$("$BIN" query bank balances "$(addr "$ALICE")" $Q | json amount)
  "$BIN" tx syndicate commit "$id" "800000$DENOM" --from "$ALICE" $TX >/dev/null 2>&1 \
    && echo "  ok    alice committed 800000" || echo "  FAIL  alice"
  sleep 3
  "$BIN" tx syndicate commit "$id" "600000$DENOM" --from "$BOB" $TX >/dev/null 2>&1 \
    && echo "  ok    bob committed 600000" || echo "  FAIL  bob"
  sleep 3

  during=$("$BIN" query bank balances "$(addr "$ALICE")" $Q | json amount)
  printf '\n  alice before %s, with the cheque escrowed %s\n' "$before" "$during"

  cat <<TEXT

  The deal is 1400000 against a minimum of 3000000. It cannot be closed —
  closing it would fund a company with less than the round needed:

    $BIN tx syndicate close-deal $id --from ... (refused: minimum not met)

  Once the window runs, ANYBODY may abort it and every cheque comes back in
  full, fees included. That asymmetry is the point: an operator may decide not
  to proceed, but may not sit on escrowed money after a deal has demonstrably
  failed, and no investor needs the operator's cooperation to be repaid.

    $BIN tx syndicate abort-deal $id "missed its minimum" --from $ALICE $TX

  Deal $id. Withdrawing early works too, and needs nobody's permission:

    $BIN tx syndicate withdraw $id --from $ALICE $TX

TEXT
}

# --------------------------------------------------------------- stage 3
stage_deal() {
  operator=$(operator_addr)
  say "4/4  a deal that closes, and pays out"

  for who in "$ALICE" "$BOB" "$CAROL"; do onboard "$who" "$operator"; done

  if open_deal "$operator" "SYN-2026-001" "Sahel Logistics" 3000000 2000000 500000; then
    echo "  ok    deal opened, target 3000000, minimum 2000000, ticket 500000"
  else
    echo "  FAIL  deal not opened"; exit 1
  fi
  id=$(deal_id "SYN-2026-001")
  printf '  deal id: %s\n\n' "$id"

  "$BIN" tx syndicate commit "$id" "1800000$DENOM" --from "$ALICE" $TX >/dev/null 2>&1 \
    && echo "  ok    alice committed 1800000 (60%)" || echo "  FAIL  alice"
  sleep 3
  "$BIN" tx syndicate commit "$id" "1200000$DENOM" --from "$BOB" $TX >/dev/null 2>&1 \
    && echo "  ok    bob committed 1200000 (40%)" || echo "  FAIL  bob"
  sleep 3

  # Full, so it can close before the window runs: there is nothing left to wait
  # for and holding a filled allocation open only costs the investors.
  if as_operator "{\"@type\":\"/blockchain.syndicate.v1.MsgCloseDeal\",
    \"operator\":\"$operator\",\"deal_id\":\"$id\"}" "close $id" "$operator"; then
    echo "  ok    closed"
  else
    echo "  FAIL  close refused"; exit 1
  fi
  sleep 3

  printf '\n'
  "$BIN" query syndicate deal "$id" $Q | grep -E '"raised"|"denom"|"supply"|"state"' | sed 's/^/  /'
  printf '\n  3%% of the raise went to the platform, 97%% to the company. Ownership is\n'
  printf '  against the GROSS cheque, so alice holds 60%% and bob 40%%.\n\n'

  "$BIN" query syndicate entitlement "$id" "$(addr "$ALICE")" $Q | sed 's/^/  /'

  cat <<TEXT

  Now pay a dividend into it, from the partner:

    $BIN tx syndicate distribute $id 500000$DENOM false --from $PARTNER $TX

  Then run:  $0 secondary

TEXT
}

# --------------------------------------------------------------- stage 4
stage_secondary() {
  say "the claim the product rests on: a sale between two distributions"
  id=$(deal_id "SYN-2026-001")
  [ -n "$id" ] || { echo "  run '$0 deal' first" >&2; exit 1; }

  "$BIN" tx syndicate distribute "$id" "500000$DENOM" false --from "$PARTNER" $TX >/dev/null 2>&1 \
    && echo "  ok    first distribution, 500000" || echo "  FAIL  first distribution"
  sleep 3

  # Alice sells half her position to carol, privately, at a price the two of
  # them agreed. The platform is not consulted about the price and does not
  # need to be.
  "$BIN" tx syndicate list "$id" "$(addr "$CAROL")" 900000 "1350000$DENOM" \
    --from "$ALICE" $TX >/dev/null 2>&1 \
    && echo "  ok    alice listed 900000 privately to carol" || echo "  FAIL  listing"
  sleep 3
  listing=$("$BIN" query syndicate listings "$id" $Q 2>/dev/null | json id)
  "$BIN" tx syndicate buy "$listing" --from "$CAROL" $TX >/dev/null 2>&1 \
    && echo "  ok    carol bought it — payment and tokens moved together" \
    || echo "  FAIL  purchase"
  sleep 3

  "$BIN" tx syndicate distribute "$id" "500000$DENOM" false --from "$PARTNER" $TX >/dev/null 2>&1 \
    && echo "  ok    second distribution, 500000" || echo "  FAIL  second distribution"
  sleep 3

  printf '\n  What each is owed, with nobody having reconciled anything:\n\n'
  for who in "$ALICE" "$BOB" "$CAROL"; do
    e=$("$BIN" query syndicate entitlement "$id" "$(addr "$who")" $Q 2>/dev/null)
    printf '  %-12s balance %-10s claimable %-10s share %s bps\n' \
      "$who" "$(printf '%s' "$e" | json balance)" \
      "$(printf '%s' "$e" | json amount)" "$(printf '%s' "$e" | jnum share_bps)"
  done

  cat <<'TEXT'

  Alice held 60% for the first distribution and 30% for the second. Carol held
  nothing for the first and 30% for the second. Bob held 40% throughout. Nobody
  told the platform about the sale in any sense other than sending the
  transaction, and no statement was reissued.

  That is what the tokenisation layer buys: without it, paying the second
  distribution correctly means knowing who held what on which date, through
  every private sale, for the life of the investment — and that administrative
  cost is precisely why private positions have historically had no liquidity.

TEXT
}

stage_show() {
  say "what to query"
  cat <<'TEXT'

    blockchaind query syndicate deals
    blockchaind query syndicate deal-by-ref SYN-2026-001
    blockchaind query syndicate entitlement <deal-id> <address>
    blockchaind query syndicate listings <deal-id>
    blockchaind query syndicate investor <address>

  The demonstration worth doing in a meeting is the REFUND. Commit to a deal
  that will not fill, then abort it from an account that is not the platform's,
  and watch every cheque come back in full.

TEXT
}

case "${1:-}" in
  operator)  stage_operator ;;
  failed)    stage_failed ;;
  deal)      stage_deal ;;
  secondary) stage_secondary ;;
  show)      stage_show ;;
  *) echo "usage: $0 operator|failed|deal|secondary|show" >&2; exit 2 ;;
esac
