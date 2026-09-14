#!/bin/bash
# The papers, attached to the deals that already exist.
#
# A syndicate's economics are half of what an investor decides on. The other
# half is what they are signing, and until this the platform could state a 3%
# fee to the basis point and name no agreement at all.
#
# Every digest here is a REAL SHA-256 of a real file written beside it. A
# fixture using 32 arbitrary bytes would render identically and prove nothing,
# and the whole claim of this feature is that the digest on screen matches
# sha256sum run against a file somebody was sent. So the files are written,
# hashed and left in /tmp/yamale-papers, and the demonstration can be performed
# rather than described.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657
TX="--keyring-backend test --home $HOME_DIR --chain-id yamale-devnet-2 --node $NODE --gas auto --gas-adjustment 1.5 --fees 4000uyml -y"
Q="--home $HOME_DIR --node $NODE -o json"
M1=adm-ba
M2=adm-diallo
DOCS=/tmp/yamale-papers
mkdir -p "$DOCS"

addr() { "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR"; }

OPERATOR=$("$BIN" query syndicate operators $Q 2>/dev/null | python3 -c "
import sys, json; print(json.load(sys.stdin)['operators'][0]['address'])")
[ -n "$OPERATOR" ] || { echo 'no operator'; exit 1; }

as_operator() {
  local body=$1 note=$2
  cat > /tmp/paper-msg.json <<JSON
{"group_policy_address":"$OPERATOR",
 "messages":[$body],
 "metadata":"$note",
 "proposers":["$(addr $M1)"]}
JSON
  local before id="" now c
  before=$("$BIN" query group proposals-by-group-policy "$OPERATOR" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 | sort -n | tr '\n' ' ')
  "$BIN" tx group submit-proposal /tmp/paper-msg.json --from $M1 $TX >/dev/null 2>&1 || return 1
  for _ in $(seq 1 25); do
    sleep 4
    now=$("$BIN" query group proposals-by-group-policy "$OPERATOR" $Q 2>/dev/null \
      | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 | sort -n)
    for c in $now; do
      case " $before " in *" $c "*) ;; *) id=$c ;; esac
    done
    [ -n "$id" ] && break
  done
  [ -n "$id" ] || return 1
  "$BIN" tx group vote "$id" "$(addr $M1)" VOTE_OPTION_YES "" --from $M1 $TX >/dev/null 2>&1
  sleep 8
  "$BIN" tx group vote "$id" "$(addr $M2)" VOTE_OPTION_YES "" --exec try --from $M2 $TX >/dev/null 2>&1
  local p r
  for _ in $(seq 1 25); do
    sleep 4
    p=$("$BIN" query group proposal "$id" $Q 2>/dev/null)
    [ -n "$p" ] || return 0
    r=$(printf '%s' "$p" | grep -oE '"executor_result":[[:space:]]*"[^"]*"' | cut -d'"' -f4)
    [ "$r" = "PROPOSAL_EXECUTOR_RESULT_SUCCESS" ] && return 0
    [ "$r" = "PROPOSAL_EXECUTOR_RESULT_FAILURE" ] && return 1
  done
  return 1
}

# Write the file, hash it, attach the digest.
#
# Proposal JSON carries bytes as base64 and sha256sum prints hex. The conversion
# is done here rather than described in a comment, because getting it wrong
# produces a digest of exactly the right length that matches nothing at all --
# which is the one failure the module's validation cannot catch.
paper() {
  local deal=$1 kind=$2 title=$3 lang=$4 supersedes=$5 file=$6
  local hex b64
  hex=$(sha256sum -b "$file" | cut -d' ' -f1)
  b64=$(printf '%s' "$hex" | xxd -r -p | base64 -w0)
  if as_operator "{\"@type\":\"/blockchain.syndicate.v1.MsgAttachDocument\",
      \"operator\":\"$OPERATOR\",\"deal_id\":\"$deal\",\"kind\":\"$kind\",
      \"title\":\"$title\",\"sha256\":\"$b64\",
      \"uri\":\"ipfs://bafy$(printf '%s' "$hex" | cut -c1-20)\",
      \"language\":\"$lang\",\"supersedes\":\"$supersedes\"}" "$title"; then
    printf '  ok    deal %-2s %-36s %s\n' "$deal" "$title" "${hex:0:16}"
  else
    printf '  FAIL  deal %-2s %-36s\n' "$deal" "$title"
  fi
}

doc_id() {
  "$BIN" query syndicate documents "$1" $Q 2>/dev/null | python3 -c "
import sys, json
want = sys.argv[1]
for d in json.load(sys.stdin).get('documents', []):
    if d['title'] == want:
        print(d['id']); break" "$2"
}

# ---------------------------------------------------------------- the files
# Written out first, all of them, so that a failure attaching one does not
# leave a directory half full of files whose digests are on the chain and
# whose neighbours are not.
python3 - <<'PY'
import os
d = '/tmp/yamale-papers'
os.makedirs(d, exist_ok=True)
files = {
 'd1-term-sheet.txt': """SAHEL LOGISTICS - SERIES A TERM SHEET
Pre-money 8,000,000 USD. Syndicate allocation 3,000,000 uyml alongside
Partech Africa. Liquidation preference 1x non-participating. Pro-rata rights
for holders above 5%. Not binding except as to exclusivity and confidentiality.
""",
 'd1-subscription.txt': """SUBSCRIPTION AGREEMENT
The Subscriber agrees to subscribe for deal tokens of syn/1 in the amount
committed on chain. Entry fees of 3% are deducted at the close. The Subscriber
acknowledges that the investment is illiquid, that it may be lost in full, and
that transfers are restricted to persons onboarded by the Operator.
""",
 'd1-sha.txt': """SHAREHOLDERS AGREEMENT
Drag-along at 75%. Tag-along for all holders. Pre-emption on new issues.
Quarterly information rights. Reserved matters require the Investor Majority.
""",
 'd1-sha-amended.txt': """SHAREHOLDERS AGREEMENT - AMENDED AND RESTATED
Drag-along lowered to 66%. Tag-along for all holders. Pre-emption on new issues.
MONTHLY information rights. Reserved matters require the Investor Majority.
This agreement amends and restates the agreement of the same name.
""",
 'd1-disclosure.txt': """RISK WARNINGS AND FEE DISCLOSURE
Private company shares are illiquid and may be worth nothing. Past distributions
do not predict future ones. FEES: 2% sourcing and 1% syndication at entry, taken
from the raise; 20% of each distribution as it is paid. The Operator earns the
management fee when, and only when, the investors are paid.
""",
 'd1-diligence.txt': """DILIGENCE SUMMARY - prepared by Partech Africa
Commercial, financial and legal review completed. Management references taken.
The Operator has not independently verified this work and does not warrant it.
""",
 'd1-accounts.txt': """ETATS FINANCIERS 2026 - SAHEL LOGISTICS SA
Chiffre d'affaires 1 240 000 000 FCFA. Resultat net 96 000 000 FCFA.
""",
 'd4-term-sheet.txt': """THIES SOLAR - TERM SHEET
Syndicate allocation 2,000,000 uyml. Minimum 1,000,000.
""",
 'd4-completion.txt': """COMPLETION STATEMENT AND EXIT RESOLUTION
The Company's shares were sold to the Purchaser on completion. Net proceeds
attributable to the syndicate: 1,040,000 uyml, distributed in full and marked
final. The deal token no longer trades.
""",
 'd4-final-report.txt': """FINAL INVESTOR REPORT - THIES SOLAR
Subscribed 2,000,000. Returned 1,040,000, being 52% of capital. This is a loss.
It is reported in the same place and in the same words a gain would be.
""",
 'd5-term-sheet.txt': """JOKKOLABS HEALTH - SERIES A TERM SHEET
Syndicate allocation 12,000,000 uyml alongside Partech Africa.
Minimum 7,000,000. Smallest cheque 400,000.
""",
 'd5-disclosure.txt': """AVERTISSEMENTS SUR LES RISQUES ET FRAIS
Les parts de societes non cotees sont illiquides et peuvent perdre toute valeur.
FRAIS: 2% de sourcing et 1% de syndication a l'entree; 20% de chaque
distribution au moment ou elle est versee.
""",
}
for name, body in files.items():
    with open(os.path.join(d, name), 'w', newline='\n') as fh:
        fh.write(body)
print('  %d files written to %s' % (len(files), d))
PY

echo
echo "== deal 1, Sahel Logistics: a funded deal with its full paper trail =="
paper 1 DOCUMENT_KIND_TERM_SHEET             "Term sheet"                       en 0 "$DOCS/d1-term-sheet.txt"
paper 1 DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT "Subscription agreement"           en 0 "$DOCS/d1-subscription.txt"
paper 1 DOCUMENT_KIND_SHAREHOLDERS_AGREEMENT "Shareholders agreement"           en 0 "$DOCS/d1-sha.txt"
paper 1 DOCUMENT_KIND_DISCLOSURE             "Risk warnings and fee disclosure" en 0 "$DOCS/d1-disclosure.txt"
paper 1 DOCUMENT_KIND_DILIGENCE              "Partech Africa diligence pack"    en 0 "$DOCS/d1-diligence.txt"
paper 1 DOCUMENT_KIND_REPORT                 "Etats financiers 2026"            fr 0 "$DOCS/d1-accounts.txt"

echo
echo "== the amendment, which is the part a paper register cannot do =="
# The old agreement STAYS on the record. A dispute is almost always about what
# the terms were at a date rather than about what they are now.
OLD=$(doc_id 1 "Shareholders agreement")
if [ -n "$OLD" ]; then
  paper 1 DOCUMENT_KIND_SHAREHOLDERS_AGREEMENT "Shareholders agreement (amended)" en "$OLD" \
    "$DOCS/d1-sha-amended.txt"
  echo "  the earlier version stays readable as document $OLD"
else
  echo "  FAIL  could not find the agreement to amend"
fi

echo
echo "== deal 4, Thies Solar: realised, so its papers arrived after the money =="
paper 4 DOCUMENT_KIND_TERM_SHEET "Term sheet"                              en 0 "$DOCS/d4-term-sheet.txt"
paper 4 DOCUMENT_KIND_RESOLUTION "Completion statement and exit resolution" en 0 "$DOCS/d4-completion.txt"
paper 4 DOCUMENT_KIND_REPORT     "Final investor report"                    en 0 "$DOCS/d4-final-report.txt"

echo
echo "== deal 5, Jokkolabs Health: still raising, so only what exists yet =="
paper 5 DOCUMENT_KIND_TERM_SHEET "Term sheet"                              en 0 "$DOCS/d5-term-sheet.txt"
paper 5 DOCUMENT_KIND_DISCLOSURE "Avertissements sur les risques et frais"  fr 0 "$DOCS/d5-disclosure.txt"

echo
echo "== what each deal now carries =="
for d in 1 2 3 4 5; do
  "$BIN" query syndicate documents "$d" $Q 2>/dev/null | python3 -c "
import sys, json
deal = sys.argv[1]
docs = json.load(sys.stdin).get('documents', [])
if not docs:
    print('  deal %s: no papers' % deal)
else:
    print('  deal %s: %d document(s)' % (deal, len(docs)))
    for x in docs:
        flag = ''
        if x.get('superseded_by', '0') != '0':
            flag = '  (superseded by %s)' % x['superseded_by']
        elif x.get('supersedes', '0') != '0':
            flag = '  (amends %s)' % x['supersedes']
        print('    %-40s %s%s' % (x['title'], x['kind'].replace('DOCUMENT_KIND_', ''), flag))" "$d"
done

echo
echo "  The files are in $DOCS. Run sha256sum on one and compare it with what"
echo "  the console shows: that comparison is the whole claim this feature makes."
