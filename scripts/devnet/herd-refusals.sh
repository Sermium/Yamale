#!/bin/bash
# Four declarations a paper certificate would carry without complaint, refused
# by consensus.
#
# Each one is chosen so that exactly ONE thing is wrong with it, and so that the
# animal it names is ACTIVE. Both matter. The seeder's own probes now target
# animals that are mid-transfer or disputed, so they would be refused for not
# being active rather than for the pedigree being impossible — and, as that
# script's own comment says, a demonstration that shows a refusal which was
# never due is worse than showing nothing: it teaches the room that the register
# refuses arbitrarily, which is the opposite of the argument.
#
# The reason is read out of the execution events, not inferred. A group proposal
# whose message the chain refuses is still a SUCCESSFUL transaction — the vote
# landed and execution was attempted — so reading the tx result would report
# every refusal here as an acceptance.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657
TX="--keyring-backend test --home $HOME_DIR --chain-id yamale-devnet-2 --node $NODE --gas auto --gas-adjustment 1.5 --fees 4000uyml -y"
Q="--home $HOME_DIR --node $NODE -o json"
M1=adm-ba
M2=adm-diallo
DNA=$(printf 'refusal-probe' | sha256sum | cut -c1-64)

addr() { "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR"; }

REGISTRY=$("$BIN" query herd authorities $Q 2>/dev/null | python3 -c "
import sys, json
for a in json.load(sys.stdin)['authorities']:
    if 'Elevage' in a['name']:
        print(a['address']); break")
[ -n "$REGISTRY" ] || { echo 'no registry office'; exit 1; }

# Submit as the office, vote it through, and return the chain's own words.
#
# Prints "ACCEPTED" if the message went through — which for every probe in this
# file would be a failure of the register, not a success of the script.
try() {
  local what=$1 body=$2 because=$3
  cat > /tmp/refuse-msg.json <<JSON
{"group_policy_address":"$REGISTRY",
 "messages":[$body],
 "metadata":"refusal probe",
 "proposers":["$(addr $M1)"]}
JSON
  local before id="" now c
  before=$("$BIN" query group proposals-by-group-policy "$REGISTRY" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 | sort -n | tr '\n' ' ')

  "$BIN" tx group submit-proposal /tmp/refuse-msg.json --from $M1 $TX >/dev/null 2>&1

  for _ in $(seq 1 25); do
    sleep 4
    now=$("$BIN" query group proposals-by-group-policy "$REGISTRY" $Q 2>/dev/null \
      | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 | sort -n)
    for c in $now; do
      case " $before " in *" $c "*) ;; *) id=$c ;; esac
    done
    [ -n "$id" ] && break
  done
  [ -n "$id" ] || { printf '  %-34s  could not submit\n' "$what"; return; }

  "$BIN" tx group vote "$id" "$(addr $M1)" VOTE_OPTION_YES "" --from $M1 $TX >/dev/null 2>&1
  sleep 8

  # The executing vote. Its hash is how the reason is recovered.
  local h
  h=$("$BIN" tx group vote "$id" "$(addr $M2)" VOTE_OPTION_YES "" --exec try \
        --from $M2 $TX 2>/dev/null | grep -oE '^txhash: .*' | awk '{print $2}')
  sleep 10

  local reason=""
  if [ -n "$h" ]; then
    reason=$("$BIN" query tx "$h" $Q 2>/dev/null | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
# x/group records the failure of the message it tried to run in an event
# attribute, because the transaction that carried the vote succeeded.
for ev in d.get('events', []):
    for at in ev.get('attributes', []):
        v = at.get('value') or ''
        if 'logs' in (at.get('key') or '') or 'error' in (at.get('key') or ''):
            if v and v not in ('', '[]'):
                print(v); sys.exit(0)
" 2>/dev/null)
  fi

  # Absence of the proposal means it executed; presence with FAILURE means the
  # chain refused the message inside it.
  local p r verdict
  p=$("$BIN" query group proposal "$id" $Q 2>/dev/null)
  if [ -z "$p" ]; then
    verdict="ACCEPTED — the register should not have allowed this"
  else
    r=$(printf '%s' "$p" | grep -oE '"executor_result":[[:space:]]*"[^"]*"' | cut -d'"' -f4)
    case "$r" in
      *SUCCESS) verdict="ACCEPTED — the register should not have allowed this" ;;
      *FAILURE) verdict="refused" ;;
      *)        verdict="no verdict ($r)" ;;
    esac
  fi

  printf '  %-34s  %s\n' "$what" "$verdict"
  printf '  %-34s  %s\n' "" "why it cannot be true: $because"
  [ -n "$reason" ] && printf '  %-34s  chain: %s\n' "" "$(printf '%s' "$reason" | tr -d '\n' | cut -c1-150)"
  echo
}

echo "== four declarations a paper certificate would carry =="
echo

# 1. Ndiaga is a ram. Chosen as the dam precisely because he is old enough to be
#    a parent (2020-04-18) — so the ONLY thing wrong is his sex.
try "a dam that is male" \
  "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$REGISTRY\",
    \"animal_id\":\"8\",\"sire_id\":\"7\",\"dam_id\":\"4\",
    \"dna_hash\":\"$DNA\",\"photo_hash\":\"\"}" \
  "Ndiaga is a ram, and a ram is nobody's mother"

# 2. Nianing was born in 2018 and Classico in 2023.
try "a sire born after his own offspring" \
  "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$REGISTRY\",
    \"animal_id\":\"1\",\"sire_id\":\"7\",\"dam_id\":\"0\",
    \"dna_hash\":\"$DNA\",\"photo_hash\":\"\"}" \
  "Classico is five years younger than the animal he is offered as sire of"

# 3. A parent the register does not hold.
#
#    NOT the cycle check, and the first version of this probe got that wrong.
#    Offering an animal as its own sire IS refused, but by the birth-date rule —
#    a dated cycle always implies a descendant older than its ancestor, so dates
#    catch it before the cycle check is reached. That is why x/herd's own cycle
#    tests use UNDATED animals. Labelling a date refusal as a cycle refusal is
#    the same error this file exists to avoid.
#
#    What this probe shows instead is the difference between a register and a
#    free-text certificate: a paper pedigree can name an ancestor that never
#    existed, and this one cannot.
try "a parent the register does not hold" \
  "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$REGISTRY\",
    \"animal_id\":\"12\",\"sire_id\":\"9041\",\"dam_id\":\"11\",
    \"dna_hash\":\"$DNA\",\"photo_hash\":\"\"}" \
  "no animal 9041 has ever been registered; paper can name an ancestor that never existed"

# 4. A pedigree that is entirely plausible — Ndiaw and Sira are both older than
#    Fanta and of the right sexes — and carries no laboratory result. This is
#    the one that shows the register asking for evidence rather than for
#    coherence.
try "a certification with no laboratory" \
  "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$REGISTRY\",
    \"animal_id\":\"12\",\"sire_id\":\"10\",\"dam_id\":\"11\",
    \"dna_hash\":\"\",\"photo_hash\":\"\"}" \
  "the parentage is plausible; without a DNA profile it is the breeder's word"

echo "== the register is unchanged =="
"$BIN" query herd animal 8 $Q 2>/dev/null | python3 -c "
import sys, json
a = json.load(sys.stdin)['animal']
print('   Mame Diarra  level %s  sire %s  dam %s' % (
    a['level'].replace('LEVEL_',''), a.get('sire_id','0'), a.get('dam_id','0')))"
"$BIN" query herd animal 12 $Q 2>/dev/null | python3 -c "
import sys, json
a = json.load(sys.stdin)['animal']
print('   Fanta        level %s  sire %s  dam %s' % (
    a['level'].replace('LEVEL_',''), a.get('sire_id','0'), a.get('dam_id','0')))"
echo
echo "  None of these proves a pedigree true — only a laboratory does that. What"
echo "  each refuses is a pedigree that CANNOT be true, and a register that"
echo "  refuses the impossible is worth more than one that records everything."
