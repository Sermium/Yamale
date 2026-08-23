#!/bin/bash
# Prove on a running chain that an office cannot keep an authority it has stopped
# being able to exercise M-of-N.
#
# Tests prove the keeper refuses. They do not prove that a real x/group policy,
# voting on itself the way an office actually would, loses a real authority in a
# real block and gets it back — and that is a claim about a chain, so the only way
# to make it is to run one.
#
# What this stands up, on top of scripts/devnet/init-local-devnet.sh:
#
#   * a payments office that is a genuine 3-of-5 x/group policy, self-administered
#   * a role grant to it recording required_shape 3-of-5, made by the foundation
#     in one proposal three custodians vote for
#   * a one-of-one group, refused the same grant
#
# and then the sequence that matters, five times over:
#
#   1. the office admits an institution                        -> SUCCESS
#   2. the office votes its own threshold down to one           -> SUCCESS
#   3. the office admits another institution                    -> FAILURE, ErrOfficeShape
#   4. the office puts its threshold back                       -> SUCCESS
#   5. the same admission, unchanged                            -> SUCCESS
#   6. the office drops a member instead, 3-of-5 to 3-of-4      -> the same refusal
#
# Read step 6 as the separate claim it is. The threshold is not the only number on
# the grant, and an office that keeps its threshold while losing a member has
# still moved sixty per cent to seventy-five and walked towards unanimity.
#
# Where the error text lives, because it is not where anybody looks first: an
# x/group proposal that fails in execution produces a transaction with code 0. The
# refusal is inside cosmos.group.v1.EventExec — `result` is
# PROPOSAL_EXECUTOR_RESULT_FAILURE and `logs` carries the message. A script that
# read only the transaction code would report every one of these as a success.
#
# Usage:
#
#   bash scripts/devnet/office-shape-demo.sh
#   SKIP_INIT=1 bash scripts/devnet/office-shape-demo.sh   # reuse a running devnet
set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$REPO"

ROOT=${ROOT:-$REPO/.devnet}
LOCAL=${LOCAL:-$ROOT/local}
HOME_DIR=${HOME_DIR:-$LOCAL/node}
CEREMONY_DIR=${CEREMONY_DIR:-$LOCAL/ceremony}
CHAIN_ID=${CHAIN_ID:-yamale-local-1}
DENOM=${DENOM:-uyml}
RPC_PORT=${RPC_PORT:-26957}
NODE_RPC="tcp://127.0.0.1:$RPC_PORT"
COUNTRY=${COUNTRY:-GH}
WORK=$LOCAL/office-shape
BIN=${BIN:-$REPO/blockchaind.exe}

KR="--keyring-backend test --home $HOME_DIR"
Q="--home $HOME_DIR --node $NODE_RPC -o json"
TX="--home $HOME_DIR --node $NODE_RPC --keyring-backend test --chain-id $CHAIN_ID --gas 1200000 --fees 0$DENOM -y -o json"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

if [ -z "${SKIP_INIT:-}" ]; then
  bash scripts/devnet/init-local-devnet.sh
fi
[ -x "$BIN" ] || BIN=$REPO/blockchaind
[ -x "$BIN" ] || die "no node binary; run without SKIP_INIT"

mkdir -p "$WORK"
POLICY=$(tr -d '\r\n' < "$CEREMONY_DIR/policy-address")
[ -n "$POLICY" ] || die "no foundation policy address"
CUSTODIAN_KEYS=(custodian1 custodian2 custodian3 custodian4 custodian5)
declare -a CUSTODIANS
for i in 0 1 2 3 4; do CUSTODIANS[$i]=$($BIN keys show "${CUSTODIAN_KEYS[$i]}" -a $KR); done

# ---------------------------------------------------------------------------
# Helpers
#
# jq_ evaluates a JavaScript path against a JSON document on stdin, because
# node is the one JSON tool this machine is guaranteed to have and jq is not.
# ---------------------------------------------------------------------------
jq_() { node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const o=JSON.parse(s);process.stdout.write(String(eval("o"+process.argv[1])))})' "$1"; }

broadcast() {
  "$@" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).txhash))'
}

# tx_field pulls one thing out of a queried transaction, retrying until the
# transaction is indexed.
#
# `exec` and `logs` come out of the EventExec typed event rather than out of the
# top-level result, because that is the only place an x/group execution failure
# is recorded. `code` is the transaction's own code and is 0 for a proposal that
# executed and for one that failed in execution alike.
tx_field() {
  local hash=$1 expr=$2
  for _ in $(seq 1 25); do
    if out=$($BIN query tx "$hash" $Q 2>/dev/null); then
      printf '%s' "$out" | node -e '
        let s = "";
        process.stdin.on("data", (d) => (s += d)).on("end", () => {
          const tx = JSON.parse(s);
          const attr = (key) => {
            for (const e of tx.events || []) {
              if (e.type !== "cosmos.group.v1.EventExec") continue;
              for (const a of e.attributes || []) {
                if (a.key === key) { try { return JSON.parse(a.value); } catch { return a.value; } }
              }
            }
            return "NO_EVENT_EXEC";
          };
          const out = {
            code: tx.code,
            raw_log: String(tx.raw_log).slice(0, 400),
            exec: attr("result"),
            logs: attr("logs"),
          };
          process.stdout.write(String(out[process.argv[1]]));
        });
      ' "$expr"
      return 0
    fi
    sleep 1
  done
  printf 'UNINDEXED'
}

# step is a transaction whose only job is to land: a send, a submission, a vote.
# Fatal on a non-zero code, because a vote that did not land makes every number
# after it a lie.
step() {
  local label=$1; shift
  local hash code
  hash=$(broadcast "$@")
  sleep 3
  code=$(tx_field "$hash" code)
  printf '  %-40s tx %s  code=%s\n' "$label" "$hash" "$code"
  [ "$code" = "0" ] || die "$label failed: $(tx_field "$hash" raw_log)"
}

latest_proposal() {
  $BIN query group proposals-by-group-policy "$1" $Q \
    | jq_ '.proposals[o.proposals.length-1].id'
}

# submit_and_vote submits a proposal to a policy and votes it to the given
# number of signers, then executes it and reports what the execution did.
#
# It deliberately does NOT assert. Both outcomes are results this script needs to
# show, and a helper that died on failure could not demonstrate a refusal.
submit_and_vote() {
  local label=$1 policy=$2 file=$3 votes=$4; shift 4
  local voters=("$@")
  step "submit: $label" $BIN tx group submit-proposal "$file" --from "${voters[0]}" $TX
  local id
  id=$(latest_proposal "$policy")
  local i
  for ((i = 0; i < votes; i++)); do
    local addr
    addr=$($BIN keys show "${voters[$i]}" -a $KR)
    step "vote yes: ${voters[$i]}" $BIN tx group vote "$id" "$addr" VOTE_OPTION_YES "" --from "${voters[$i]}" $TX
  done
  local hash
  hash=$(broadcast $BIN tx group exec "$id" --from operator $TX)
  sleep 3
  EXEC_RESULT=$(tx_field "$hash" exec)
  EXEC_LOGS=$(tx_field "$hash" logs)
  EXEC_HASH=$hash
  EXEC_CODE=$(tx_field "$hash" code)
  printf '  %-40s tx %s  code=%s  exec=%s\n' "exec: $label" "$hash" "$EXEC_CODE" "$EXEC_RESULT"
  [ -z "$EXEC_LOGS" ] || [ "$EXEC_LOGS" = "NO_EVENT_EXEC" ] || printf '    logs: %s\n' "$EXEC_LOGS"
}

expect_success() {
  [ "$EXEC_RESULT" = "PROPOSAL_EXECUTOR_RESULT_SUCCESS" ] || \
    die "$1: expected the proposal to execute, got $EXEC_RESULT ($EXEC_LOGS)"
}
expect_failure() {
  [ "$EXEC_RESULT" = "PROPOSAL_EXECUTOR_RESULT_FAILURE" ] || \
    die "$1: expected the proposal to FAIL in execution, got $EXEC_RESULT"
  case "$EXEC_LOGS" in
    *"$2"*) : ;;
    *) die "$1: the failure did not mention '$2'; logs were: $EXEC_LOGS" ;;
  esac
}

# ---------------------------------------------------------------------------
# The office
# ---------------------------------------------------------------------------
say ""
say "=== the office: five super users, three-of-five, self-administered ==="
OFFICE_KEYS=(office1 office2 office3 office4 office5)
declare -a OFFICE_ADDRS
for k in "${OFFICE_KEYS[@]}" solo1; do
  $BIN keys add "$k" $KR >/dev/null 2>&1 || true
done
for i in 0 1 2 3 4; do OFFICE_ADDRS[$i]=$($BIN keys show "${OFFICE_KEYS[$i]}" -a $KR); done
SOLO_KEY=solo1
SOLO_MEMBER=$($BIN keys show $SOLO_KEY -a $KR)

# An account that has never received anything does not exist in x/auth, and a
# transaction from it fails on the sequence lookup rather than on anything to do
# with this demonstration.
for addr in "${OFFICE_ADDRS[@]}" "$SOLO_MEMBER"; do
  step "fund $addr" $BIN tx bank send operator "$addr" 1000000$DENOM $TX
done

cat > "$WORK/office-members.json" <<JSON
{"members": [
  {"address": "${OFFICE_ADDRS[0]}", "weight": "1", "metadata": "A. Diallo"},
  {"address": "${OFFICE_ADDRS[1]}", "weight": "1", "metadata": "B. Sow"},
  {"address": "${OFFICE_ADDRS[2]}", "weight": "1", "metadata": "C. Fall"},
  {"address": "${OFFICE_ADDRS[3]}", "weight": "1", "metadata": "D. Ba"},
  {"address": "${OFFICE_ADDRS[4]}", "weight": "1", "metadata": "E. Ndiaye"}
]}
JSON
cat > "$WORK/office-policy.json" <<'JSON'
{
  "@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
  "threshold": "3",
  "windows": {"voting_period": "120s", "min_execution_period": "0s"}
}
JSON
cat > "$WORK/solo-members.json" <<JSON
{"members": [{"address": "$SOLO_MEMBER", "weight": "1", "metadata": "one person"}]}
JSON
cat > "$WORK/solo-policy.json" <<'JSON'
{
  "@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
  "threshold": "1",
  "windows": {"voting_period": "120s", "min_execution_period": "0s"}
}
JSON

step "create the office group" $BIN tx group create-group-with-policy \
  "${OFFICE_KEYS[0]}" "Ghana payments authority" "Ghana payments authority" \
  "$WORK/office-members.json" "$WORK/office-policy.json" --group-policy-as-admin \
  --from "${OFFICE_KEYS[0]}" $TX
step "create the one-of-one group" $BIN tx group create-group-with-policy \
  "$SOLO_KEY" "One person pretending to be an office" "One person pretending to be an office" \
  "$WORK/solo-members.json" "$WORK/solo-policy.json" --group-policy-as-admin \
  --from "$SOLO_KEY" $TX

# The addresses are READ BACK rather than predicted. A policy address derives
# from the policy sequence number alone, so predicting one proves nothing about
# who controls it — see the note at the top of tools/ceremony/country.go, and the
# live run where a predicted office address turned out to be the foundation's.
OFFICE=$($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].address')
SOLO=$($BIN query group group-policies-by-group 3 $Q | jq_ '.group_policies[0].address')
[ -n "$OFFICE" ] && [ "$OFFICE" != "undefined" ] || die "could not read the office policy address"
[ "$OFFICE" != "$POLICY" ] || die "the office address is the FOUNDATION's; stop"
say "  office policy   $OFFICE  (group 2, $($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].decision_policy.value.threshold')-of-$($BIN query group group-members 2 $Q | jq_ '.members.length'))"
say "  one-of-one      $SOLO  (group 3)"

# The office pays for its own proposals.
step "fund the office" $BIN tx bank send operator "$OFFICE" 10000000$DENOM $TX

# ---------------------------------------------------------------------------
# Two applicants
# ---------------------------------------------------------------------------
say ""
say "=== two institutions apply ==="
step "bankco applies" $BIN tx paymsg apply-participant GHBANK001 "Bank Co Ghana" --from bankco $TX
step "customer applies" $BIN tx paymsg apply-participant GHBANK002 "Second Bank Ghana" --from customer $TX
BANKCO=$($BIN keys show bankco -a $KR)
SECOND=$($BIN keys show customer -a $KR)

# ---------------------------------------------------------------------------
# The foundation grants the role, recording the shape
# ---------------------------------------------------------------------------
say ""
say "=== the foundation admits $COUNTRY: places the accounts, grants 3-of-5 ==="
cat > "$WORK/enrol.json" <<JSON
{
  "group_policy_address": "$POLICY",
  "messages": [
    {"@type": "/blockchain.alias.v1.MsgSetJurisdiction", "recorder": "$POLICY", "account": "$OFFICE", "country": "$COUNTRY"},
    {"@type": "/blockchain.alias.v1.MsgSetJurisdiction", "recorder": "$POLICY", "account": "$SOLO", "country": "$COUNTRY"},
    {"@type": "/blockchain.alias.v1.MsgSetJurisdiction", "recorder": "$POLICY", "account": "$BANKCO", "country": "$COUNTRY"},
    {"@type": "/blockchain.alias.v1.MsgSetJurisdiction", "recorder": "$POLICY", "account": "$SECOND", "country": "$COUNTRY"},
    {
      "@type": "/blockchain.alias.v1.MsgGrantRole",
      "authority": "$POLICY",
      "holder": "$OFFICE",
      "role": "ROLE_PAYMENTS_AUTHORITY",
      "jurisdiction": "$COUNTRY",
      "required_shape": {"signatures": 3, "members": 5}
    }
  ],
  "metadata": "",
  "title": "Admit $COUNTRY",
  "summary": "Places four accounts in $COUNTRY and grants ROLE_PAYMENTS_AUTHORITY to the payments office, required 3-of-5.",
  "proposers": ["${CUSTODIANS[0]}"]
}
JSON
submit_and_vote "enrol $COUNTRY" "$POLICY" "$WORK/enrol.json" 3 \
  "${CUSTODIAN_KEYS[0]}" "${CUSTODIAN_KEYS[1]}" "${CUSTODIAN_KEYS[2]}"
expect_success "the enrolment"

say ""
say "  the grant, as the chain records it:"
$BIN query alias role-grants "$OFFICE" $Q | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{for(const g of (JSON.parse(s).grants||[])){console.log(`    ${g.role} in ${g.jurisdiction}, required ${g.required_shape?`${g.required_shape.signatures}-of-${g.required_shape.members}`:"NOTHING"}, by ${g.granted_by} at height ${g.granted_at_height}`)}})'

# ---------------------------------------------------------------------------
# A one-of-one is refused the same grant
# ---------------------------------------------------------------------------
say ""
say "=== the same grant to a one-of-one office ==="
cat > "$WORK/solo-grant.json" <<JSON
{
  "group_policy_address": "$POLICY",
  "messages": [
    {
      "@type": "/blockchain.alias.v1.MsgGrantRole",
      "authority": "$POLICY",
      "holder": "$SOLO",
      "role": "ROLE_PAYMENTS_AUTHORITY",
      "jurisdiction": "$COUNTRY",
      "required_shape": {"signatures": 3, "members": 5}
    }
  ],
  "metadata": "",
  "title": "Grant a one-of-one a three-of-five",
  "summary": "This must be refused: the holder is a group policy, which is all the chain used to check, and it is one key.",
  "proposers": ["${CUSTODIANS[0]}"]
}
JSON
submit_and_vote "grant to the one-of-one" "$POLICY" "$WORK/solo-grant.json" 3 \
  "${CUSTODIAN_KEYS[0]}" "${CUSTODIAN_KEYS[1]}" "${CUSTODIAN_KEYS[2]}"
expect_failure "the one-of-one grant" "1-of-1"
# (o.grants||[]) because protobuf JSON omits an empty repeated field entirely
# rather than rendering it as []. Reading .grants.length on the response for an
# account that holds nothing is a TypeError, not a zero.
GRANTS=$($BIN query alias role-grants "$SOLO" $Q | jq_ '["grants"]?.length ?? 0')
[ "$GRANTS" = "0" ] || die "a refused grant left $GRANTS rows behind"
say "  and nothing was written: role-grants $SOLO is empty"

# ---------------------------------------------------------------------------
# 1. The office admits an institution
# ---------------------------------------------------------------------------
approval_proposal() {
  cat > "$WORK/approve-$2.json" <<JSON
{
  "group_policy_address": "$OFFICE",
  "messages": [
    {"@type": "/blockchain.paymsg.v1.MsgApproveParticipant", "authority": "$OFFICE", "participant": "$1", "approve": true}
  ],
  "metadata": "",
  "title": "Admit $2",
  "summary": "The payments office of $COUNTRY admits an institution recorded in $COUNTRY.",
  "proposers": ["${OFFICE_ADDRS[0]}"]
}
JSON
}
# approved reports the participant code the rail assigned, or says nobody did.
#
# The query errors for an address that was never approved, which is the answer
# rather than a problem — so the error is swallowed and turned into a word a
# reader can act on.
approved() {
  local out
  if out=$($BIN query paymsg get-approved-participant "$1" $Q 2>/dev/null); then
    printf '%s' "$out" | jq_ '.approved_participant.code'
  else
    printf 'NOT ADMITTED'
  fi
}

say ""
say "=== 1. the office, at three-of-five, admits Bank Co ==="
approval_proposal "$BANKCO" bankco
submit_and_vote "admit Bank Co" "$OFFICE" "$WORK/approve-bankco.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "the first admission"
say "  paymsg participant $BANKCO -> $(approved "$BANKCO")"

# ---------------------------------------------------------------------------
# 2. The office votes itself down
# ---------------------------------------------------------------------------
say ""
say "=== 2. the office votes its own threshold from three to one ==="
cat > "$WORK/reduce.json" <<JSON
{
  "group_policy_address": "$OFFICE",
  "messages": [
    {
      "@type": "/cosmos.group.v1.MsgUpdateGroupPolicyDecisionPolicy",
      "admin": "$OFFICE",
      "group_policy_address": "$OFFICE",
      "decision_policy": {
        "@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
        "threshold": "1",
        "windows": {"voting_period": "120s", "min_execution_period": "0s"}
      }
    }
  ],
  "metadata": "",
  "title": "One signature is enough",
  "summary": "The office reduces its own threshold. x/group permits this: the office is its own admin, which is what makes its arrangement its own to change.",
  "proposers": ["${OFFICE_ADDRS[0]}"]
}
JSON
submit_and_vote "reduce to one-of-five" "$OFFICE" "$WORK/reduce.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "the office reducing itself"
say "  x/group now says threshold $($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].decision_policy.value.threshold') over $($BIN query group group-members 2 $Q | jq_ '.members.length') members"

# ---------------------------------------------------------------------------
# 3. The same action, refused
# ---------------------------------------------------------------------------
say ""
say "=== 3. the same office, now one-of-five, admits the second bank ==="
approval_proposal "$SECOND" second
submit_and_vote "admit Second Bank" "$OFFICE" "$WORK/approve-second.json" 1 "${OFFICE_KEYS[0]}"
expect_failure "the admission by a reduced office" "no longer keeps the M-of-N"
say "  paymsg participant $SECOND -> $(approved "$SECOND")"

# ---------------------------------------------------------------------------
# 4 and 5. Restored, and working again
# ---------------------------------------------------------------------------
say ""
say "=== 4. the office puts its threshold back, on one signature ==="
cat > "$WORK/restore.json" <<JSON
{
  "group_policy_address": "$OFFICE",
  "messages": [
    {
      "@type": "/cosmos.group.v1.MsgUpdateGroupPolicyDecisionPolicy",
      "admin": "$OFFICE",
      "group_policy_address": "$OFFICE",
      "decision_policy": {
        "@type": "/cosmos.group.v1.ThresholdDecisionPolicy",
        "threshold": "3",
        "windows": {"voting_period": "120s", "min_execution_period": "0s"}
      }
    }
  ],
  "metadata": "",
  "title": "Back to three of five",
  "summary": "Restoring the shape the grant records. No foundation involvement and no re-grant.",
  "proposers": ["${OFFICE_ADDRS[0]}"]
}
JSON
submit_and_vote "restore three-of-five" "$OFFICE" "$WORK/restore.json" 1 "${OFFICE_KEYS[0]}"
expect_success "restoring the office"
say "  x/group now says threshold $($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].decision_policy.value.threshold')"

say ""
say "=== 5. the same admission, unchanged, three-of-five again ==="
approval_proposal "$SECOND" second-again
submit_and_vote "admit Second Bank" "$OFFICE" "$WORK/approve-second-again.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "the admission after restoration"
say "  paymsg participant $SECOND -> $(approved "$SECOND")"

# ---------------------------------------------------------------------------
# 6. The member floor, which the threshold alone does not see
# ---------------------------------------------------------------------------
say ""
say "=== 6. a member leaves and is not replaced: three-of-five becomes three-of-four ==="
step "bankco applies again as a third" $BIN tx paymsg apply-participant GHBANK003 "Third Bank Ghana" --from validator $TX
THIRD=$($BIN keys show validator -a $KR)
cat > "$WORK/place-third.json" <<JSON
{
  "group_policy_address": "$POLICY",
  "messages": [
    {"@type": "/blockchain.alias.v1.MsgSetJurisdiction", "recorder": "$POLICY", "account": "$THIRD", "country": "$COUNTRY"}
  ],
  "metadata": "",
  "title": "Place the third applicant",
  "summary": "An institution nobody has placed is one no authority is accountable for, so the delegated approval path refuses it.",
  "proposers": ["${CUSTODIANS[0]}"]
}
JSON
submit_and_vote "place the third applicant" "$POLICY" "$WORK/place-third.json" 3 \
  "${CUSTODIAN_KEYS[0]}" "${CUSTODIAN_KEYS[1]}" "${CUSTODIAN_KEYS[2]}"
expect_success "placing the third applicant"

cat > "$WORK/drop-member.json" <<JSON
{
  "group_policy_address": "$OFFICE",
  "messages": [
    {
      "@type": "/cosmos.group.v1.MsgUpdateGroupMembers",
      "admin": "$OFFICE",
      "group_id": "2",
      "member_updates": [{"address": "${OFFICE_ADDRS[4]}", "weight": "0", "metadata": "E. Ndiaye, resigning"}]
    }
  ],
  "metadata": "",
  "title": "E. Ndiaye resigns",
  "summary": "A weight of zero is how x/group removes a member. The threshold is untouched: this office is a three-of-four afterwards.",
  "proposers": ["${OFFICE_ADDRS[0]}"]
}
JSON
submit_and_vote "drop one member" "$OFFICE" "$WORK/drop-member.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "dropping a member"
say "  x/group now says threshold $($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].decision_policy.value.threshold') over $($BIN query group group-members 2 $Q | jq_ '.members.length') members"

approval_proposal "$THIRD" third
submit_and_vote "admit Third Bank" "$OFFICE" "$WORK/approve-third.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_failure "the admission by a four-member office" "3-of-4"
say "  paymsg participant $THIRD -> $(approved "$THIRD")"

say ""
say "=== 7. the member is replaced, and the office works again ==="
cat > "$WORK/replace-member.json" <<JSON
{
  "group_policy_address": "$OFFICE",
  "messages": [
    {
      "@type": "/cosmos.group.v1.MsgUpdateGroupMembers",
      "admin": "$OFFICE",
      "group_id": "2",
      "member_updates": [{"address": "$SOLO_MEMBER", "weight": "1", "metadata": "F. Sarr, incoming"}]
    }
  ],
  "metadata": "",
  "title": "F. Sarr joins",
  "summary": "The replacement. Departures and replacements are one decision; doing them as two is what leaves an office below its shape in between.",
  "proposers": ["${OFFICE_ADDRS[0]}"]
}
JSON
submit_and_vote "replace the member" "$OFFICE" "$WORK/replace-member.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "replacing the member"
say "  x/group now says threshold $($BIN query group group-policies-by-group 2 $Q | jq_ '.group_policies[0].decision_policy.value.threshold') over $($BIN query group group-members 2 $Q | jq_ '.members.length') members"

approval_proposal "$THIRD" third-again
submit_and_vote "admit Third Bank" "$OFFICE" "$WORK/approve-third-again.json" 3 \
  "${OFFICE_KEYS[0]}" "${OFFICE_KEYS[1]}" "${OFFICE_KEYS[2]}"
expect_success "the admission after the replacement"
say "  paymsg participant $THIRD -> $(approved "$THIRD")"

# ---------------------------------------------------------------------------
# 8. The requirement ratchets
# ---------------------------------------------------------------------------
say ""
say "=== 8. the foundation re-grants the same triple with no required shape ==="
cat > "$WORK/unpin.json" <<JSON
{
  "group_policy_address": "$POLICY",
  "messages": [
    {
      "@type": "/blockchain.alias.v1.MsgGrantRole",
      "authority": "$POLICY",
      "holder": "$OFFICE",
      "role": "ROLE_PAYMENTS_AUTHORITY",
      "jurisdiction": "$COUNTRY"
    }
  ],
  "metadata": "",
  "title": "Re-grant, as a resubmission would",
  "summary": "The obvious resubmission after a timeout: composed from the summary, which does not mention required_shape. It must not remove the pin.",
  "proposers": ["${CUSTODIANS[0]}"]
}
JSON
submit_and_vote "re-grant with the field omitted" "$POLICY" "$WORK/unpin.json" 3 \
  "${CUSTODIAN_KEYS[0]}" "${CUSTODIAN_KEYS[1]}" "${CUSTODIAN_KEYS[2]}"
expect_failure "the unpinning re-grant" "Omitting required_shape"
say "  the grant is untouched:"
$BIN query alias role-grants "$OFFICE" $Q | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{for(const g of (JSON.parse(s).grants||[])){console.log(`    ${g.role} in ${g.jurisdiction}, required ${g.required_shape?`${g.required_shape.signatures}-of-${g.required_shape.members}`:"NOTHING"}`)}})'

say ""
say "================================================================"
say "  PROVEN on $CHAIN_ID"
say "================================================================"
say "  a 3-of-5 office admitted an institution"
say "  it voted itself to 1-of-5 and the same action was refused"
say "  it restored itself and the action worked again, with no re-grant"
say "  it dropped a member to 3-of-4 and was refused for the other floor"
say "  it replaced the member and the action worked again"
say "  a 1-of-1 group was refused a grant requiring 3-of-5, and nothing was written"
say "  a re-grant omitting required_shape was refused rather than unpinning the office"
