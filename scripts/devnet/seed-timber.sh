#!/bin/bash
# The SRM timber holding, tokenised.
#
# The structure, as the presentation states it: the operation stays Congolese,
# a UAE holding company collects the proceeds under a management agreement, and
# THAT COMPANY is what is tokenised. The title minted here is title over the
# holding, never over the forest -- a concession is not transferable, which is
# the whole reason the holding exists.
#
# 100,000,000 tokens carrying ALL of the economics (holder_share_bps = 10000),
# because the deck words the retention promise as tokens rather than as a share
# class: "You retain between 75% and 80% of the tokens." Selling a fifth is then
# a transfer of tokens the sponsor already holds, not a smaller issuance -- and
# that is also what makes the promise checkable by anybody, because the
# sponsor's balance is public and a share class is not.
#
# Money is uusd at six decimals. Every amount below is in base units:
# $1 = 1,000,000.
#
# ===========================================================================
# EVERY TRANSACTION IS CHECKED. THE FIRST VERSION OF THIS SCRIPT DID NOT.
# ===========================================================================
#
# It sent each tx with `>/dev/null 2>&1` and printed a success line regardless,
# so a mint that the chain refused outright -- uusd had a supply ceiling of zero
# -- printed exactly like the six transfers that worked. The failure was found
# only because a later balance read $0.
#
# `tx` below broadcasts, recovers the hash, waits for the transaction, and reads
# its code. A non-zero code prints the chain's own words and stops the script. A
# seeder that reports success it has not verified is worse than one that
# crashes, because the crash is noticed.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657
CHAIN=yamale-devnet-2
TX="--keyring-backend test --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 --fees 6000uyml -y -o json"
Q="--home $HOME_DIR --node $NODE -o json"

SPONSOR_KEY=srm-holding
COLLECTION=ae-holdings
SYMBOL=SRMT
SUPPLY=100000000                 # 100,000,000 tokens
DIST=15000000000000              # $15,000,000 at six decimals
DOCS=/tmp/timber-papers

addr() { "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null; }
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend test --home "$HOME_DIR" >/dev/null 2>&1
}

# Broadcast, then find out what the chain actually did with it.
tx() {
  local what=$1; shift
  local out hash code log
  out=$("$BIN" tx "$@" $TX 2>&1)
  hash=$(printf '%s' "$out" | python3 -c "
import sys, json
try: print(json.loads(sys.stdin.read()).get('txhash',''))
except Exception: print('')
" 2>/dev/null)
  if [ -z "$hash" ]; then
    printf '  FAIL  %-34s not broadcast\n' "$what"
    printf '%s\n' "$out" | tail -c 400 | sed 's/^/        /'
    return 1
  fi
  local i
  for i in $(seq 1 20); do
    sleep 3
    res=$("$BIN" query tx "$hash" $Q 2>/dev/null) || continue
    [ -n "$res" ] || continue
    code=$(printf '%s' "$res" | python3 -c "
import sys, json
try: print(json.loads(sys.stdin.read()).get('code', 1))
except Exception: print(1)")
    if [ "$code" = "0" ]; then
      printf '  ok    %-34s %s\n' "$what" "${hash:0:12}"
      return 0
    fi
    log=$(printf '%s' "$res" | python3 -c "
import sys, json
try: print(json.loads(sys.stdin.read()).get('raw_log',''))
except Exception: print('')")
    printf '  FAIL  %-34s code %s\n' "$what" "$code"
    printf '%s\n' "$log" | cut -c1-300 | sed 's/^/        /'
    return 1
  done
  printf '  FAIL  %-34s never landed in a block\n' "$what"
  return 1
}

balance() {
  "$BIN" query bank balances "$1" $Q 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin).get('balances', [])
print(next((c['amount'] for c in d if c['denom'] == sys.argv[1]), '0'))" "$2"
}

SPONSOR=$(addr $SPONSOR_KEY)
[ -n "$SPONSOR" ] || { ensure_key $SPONSOR_KEY; SPONSOR=$(addr $SPONSOR_KEY); }
echo "sponsor: $SPONSOR"

echo
echo "== preconditions =="
"$BIN" query stablecoin get-approved-issuer uusd $Q >/dev/null 2>&1 \
  && echo "  ok    uusd has an approved issuer" \
  || { echo "  FAIL  uusd has no approved issuer"; exit 1; }
"$BIN" query tokenisation collections $Q 2>/dev/null | grep -q "$COLLECTION" \
  && echo "  ok    collection $COLLECTION exists" \
  || { echo "  FAIL  collection $COLLECTION does not exist"; exit 1; }
CEIL=$("$BIN" query stablecoin params $Q 2>/dev/null | python3 -c "
import sys, json
p = json.load(sys.stdin)['params']
for c in p.get('mint_ceilings', []) or []:
    if c.get('denom') == 'uusd':
        print(c.get('ceiling', '0')); break
else:
    print(p.get('default_mint_ceiling', '0'))")
if [ "${CEIL:-0}" = "0" ]; then
  echo "  FAIL  uusd has a mint ceiling of 0, so no dollars can exist."
  echo "        default_mint_ceiling is what an upgraded chain sees until"
  echo "        governance states a figure. Grant uusd a ceiling and re-run."
  exit 1
fi
echo "  ok    uusd ceiling $CEIL"

echo
echo "== dollars =="
have=$(balance "$SPONSOR" uusd)
if [ "$have" -lt "$DIST" ] 2>/dev/null; then
  tx "mint \$15,000,000" stablecoin mint-coin uusd $DIST "$SPONSOR" --from $SPONSOR_KEY || exit 1
else
  echo "  --    sponsor already holds enough uusd"
fi
printf '  sponsor holds $%s\n' "$(python3 -c "print(f'{int('$(balance "$SPONSOR" uusd)')/1e6:,.0f}')")"

echo
echo "== specimen papers =="
python3 - <<'PY'
import os
d = '/tmp/timber-papers'
os.makedirs(d, exist_ok=True)
files = {
 'incorporation.txt': """SPECIMEN - NOT A FILING
CERTIFICATE OF INCORPORATION
SRM Timber Holding Ltd, Abu Dhabi Global Market, United Arab Emirates.
Incorporated as a private company limited by shares. Sole activity: holding and
collecting the proceeds of a forestry concession in the Republic of the Congo
under a management agreement.
This document is a specimen produced for a demonstration of the Yamale
tokenisation module. It records no real incorporation and has no legal effect.
""",
 'share-creation.txt': """SPECIMEN - NOT A FILING
RESOLUTION CREATING THE SHARE CLASS
The company resolves to create 100,000,000 ordinary participating shares,
carrying in aggregate the entire economic interest in the company's receipts
after the management fee and the reserve for reinvestment on site.
The class is closed. No further shares may be created, and the holders'
proportion is therefore fixed at issuance - a property enforced by the
tokenisation module rather than by this resolution.
SPECIMEN. No shares were created by this document.
""",
 'spv-agreement.txt': """SPECIMEN - NOT A FILING
MANAGEMENT AND PROCEEDS AGREEMENT
Between the Congolese operating company and SRM Timber Holding Ltd.
The operating company retains its permits, its workforce and its obligations
under Congolese law. The holding provides management services against a fee
capped at 20% of the operating result, and receives the dividend on what remains
after Congolese corporate tax and a 25% reserve reinvested on site.
Nothing here transfers the concession, which is not transferable.
SPECIMEN. Produced for demonstration; not executed by any party.
""",
 'risk-disclosure.txt': """SPECIMEN - NOT A FILING
RISK DISCLOSURE
The holders' interest is in an Emirati holding company whose sole receipts come
from one forestry operation in one jurisdiction. Concentration is total. The
year-one distribution is a projection from the operating result, not a
commitment, and depends on a tax treaty whose entry into force was unconfirmed
at the date of the presentation this file accompanies.
The token confers no management rights. The sponsor retains a majority of the
board and of the tokens.
SPECIMEN.
""",
}
for name, body in files.items():
    with open(os.path.join(d, name), 'w', newline='\n') as fh:
        fh.write(body)
print('  %d specimens written to %s' % (len(files), d))
PY

MANIFEST=$DOCS/manifest.txt
{
  echo "SRM TIMBER HOLDING - DOCUMENT MANIFEST (SPECIMEN)"
  echo
  for f in incorporation share-creation spv-agreement risk-disclosure; do
    printf '%s  %s\n' "$(sha256sum -b "$DOCS/$f.txt" | cut -d' ' -f1)" "$f.txt"
  done
} > "$MANIFEST"
MHASH=$(sha256sum -b "$MANIFEST" | cut -d' ' -f1)
echo "  manifest ${MHASH:0:16}"
# One manifest rather than a record per document, because x/tokenisation has no
# document field. That is weaker than the per-document attestation x/syndicate
# carries -- a reader cannot tell WHICH paper changed -- and it is why covenants
# are the next phase rather than an optional extra.

echo
echo "== title and shareholding =="
ASSET=$("$BIN" query tokenisation assets $Q 2>/dev/null | python3 -c "
import sys, json
for a in json.load(sys.stdin).get('assets', []):
    if a.get('collection_id') == '$COLLECTION': print(a['id']); break" 2>/dev/null)

if [ -z "$ASSET" ]; then
  tx "mint title" tokenisation mint-asset "$COLLECTION" "$SPONSOR" \
    "yamale:srm-timber-holding;docs=sha256:$MHASH" --from $SPONSOR_KEY || exit 1
  ASSET=$("$BIN" query tokenisation assets $Q 2>/dev/null | python3 -c "
import sys, json
for a in json.load(sys.stdin).get('assets', []):
    if a.get('collection_id') == '$COLLECTION': print(a['id']); break")
fi
echo "  asset $ASSET"

DENOM=$("$BIN" query tokenisation asset "$ASSET" $Q 2>/dev/null | python3 -c "
import sys, json; print(json.load(sys.stdin)['asset'].get('fraction_denom',''))")
if [ -z "$DENOM" ]; then
  tx "fractionalise $SUPPLY" tokenisation fractionalise "$ASSET" "$SYMBOL" "$SUPPLY" \
    --holder-share-bps 10000 --income-denom uusd --from $SPONSOR_KEY || exit 1
  DENOM=$("$BIN" query tokenisation asset "$ASSET" $Q 2>/dev/null | python3 -c "
import sys, json; print(json.load(sys.stdin)['asset'].get('fraction_denom',''))")
fi
echo "  denom $DENOM"

echo
echo "== the book =="
place() {
  local key=$1 tokens=$2
  ensure_key "$key"
  local a; a=$(addr "$key")
  local held; held=$(balance "$a" "$DENOM")
  if [ "${held:-0}" -ge "$tokens" ] 2>/dev/null; then
    printf '  --    %-18s already holds %s\n' "$key" "$held"
    return 0
  fi
  tx "$key $tokens" bank send "$SPONSOR" "$a" "${tokens}${DENOM}" --from $SPONSOR_KEY
}
place tim-familyoffice  9000000
place tim-tradinghouse  5000000
place tim-esgfund       3500000
place tim-diaspora      2200000
place tim-angel         1500000
place tim-regional       800000

echo
echo "== year one =="
FUNDED=$("$BIN" query tokenisation asset "$ASSET" $Q 2>/dev/null | python3 -c "
import sys, json
v = json.load(sys.stdin).get('vault') or {}
print(sum(int(c['amount']) for c in (v.get('funded') or [])))" 2>/dev/null)
if [ "${FUNDED:-0}" -ge "$DIST" ] 2>/dev/null; then
  echo "  --    the vault has already been funded"
else
  tx "fund vault \$15,000,000" tokenisation fund-vault "$ASSET" "${DIST}uusd" --from $SPONSOR_KEY || exit 1
fi

echo
echo "== the tokenisation now =="
"$BIN" query tokenisation asset "$ASSET" $Q 2>/dev/null | python3 -c "
import sys, json
a = json.load(sys.stdin)['asset']
print('  asset %s | %s | %s' % (a.get('id'), a.get('collection_id'), a.get('status','').replace('STATUS_','')))
print('  denom %s' % a.get('fraction_denom'))
print('  uri   %s' % a.get('uri'))
v = a.get('vault') or {}
f = sum(int(c['amount']) for c in (v.get('funded') or []))
print('  funded \$%s' % format(f/1e6, ',.0f'))"

echo
for k in srm-holding tim-familyoffice tim-tradinghouse tim-esgfund tim-diaspora tim-angel tim-regional; do
  a=$(addr $k) || continue
  "$BIN" query tokenisation entitlement "$ASSET" "$a" $Q 2>/dev/null | python3 -c "
import sys, json
try: e = json.load(sys.stdin)
except Exception: sys.exit(0)
bal = int(e.get('balance', '0') or 0)
if not bal: sys.exit(0)
c = e.get('claimable') or {}
print('  %-18s %11d tokens  %5.1f%%   owed \$%s' % (
    sys.argv[1], bal, 100.0 * bal / $SUPPLY, format(int(c.get('amount','0') or 0)/1e6, ',.2f')))" "$k" 2>/dev/null
done
