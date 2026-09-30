#!/bin/bash
# GENERATED from african-currencies.json by generate.py -- do not hand-edit.
# DRY_RUN=1 bash onchain.sh   prints the transactions it would send.
set -uo pipefail
B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node
K="--keyring-backend test --home $H"
TX="--chain-id yamale-devnet-2 $K --fees 2000uyml --yes -o json"
F=$($B keys show foundation -a $K)
GOV=$($B query auth module-account gov --home $H -o json 2>/dev/null | grep -oE '"address":"[a-z0-9]+"' | head -1 | cut -d'"' -f4)
PROPOSAL=/tmp/approve-issuers.json
YML_DEPTH=20000000000
FAUCET_STOCK=10000000000000

# Every transaction goes through here, so a dry run is the real run minus the
# broadcast rather than a copy of it.
tx() {
  if [ -n "${DRY_RUN:-}" ]; then echo "  DRY: tx $*" >&2; return 0; fi
  $B tx "$@" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1
}

# Did it work, in words -- the chain's own reason on a failure, because a code
# number is something nobody reads.
say() {
  if [ -z "$2" ]; then
    if [ -n "${DRY_RUN:-}" ]; then echo "  $1: would send"; else echo "  $1: NOT BROADCAST"; fi
    return 1
  fi
  local out c
  out=$($B query tx "$2" --home $H -o json 2>/dev/null)
  c=$(grep -oE '"code": *[0-9]+' <<<"$out" | head -1 | grep -oE '[0-9]+$')
  if [ "$c" = "0" ]; then echo "  $1: ok"; return 0; fi
  echo "  $1: FAILED (code ${c:-no-such-tx}) $(grep -oE '"raw_log": *"[^"]{0,220}' <<<"$out" | head -1 | cut -d'"' -f4)"
  return 1
}

# State, not status codes: a re-run asks the chain what is true.
approved() { $B query stablecoin get-approved-issuer "$1" --home $H -o json >/dev/null 2>&1; }
applied()  { $B query stablecoin get-issuer-application "$1" --home $H -o json >/dev/null 2>&1; }
have()     { local v; v=$($B query bank balance "$F" "$1" --home $H -o json 2>/dev/null | grep -oE '"amount": *"[0-9]+"' | grep -oE '[0-9]+'); echo "${v:-0}"; }

# denom|display|exponent|name|symbol|pool (base units of the currency against YML_DEPTH uyml)|rate
CURRENCIES="uxof|xof|6|West African CFA franc|XOF|12000000000000|600
uxaf|xaf|6|Central African CFA franc|XAF|12000000000000|600
ungn|ngn|6|Nigerian Naira|NGN|31000000000000|1550
ukes|kes|6|Kenyan Shilling|KES|2580000000000|129
uzar|zar|6|South African Rand|ZAR|360000000000|18
ughs|ghs|6|Ghanaian Cedi|GHS|300000000000|15
uegp|egp|6|Egyptian Pound|EGP|1000000000000|50
umad|mad|6|Moroccan Dirham|MAD|200000000000|10
udzd|dzd|6|Algerian Dinar|DZD|2700000000000|135
utnd|tnd|6|Tunisian Dinar|TND|60000000000|3
uetb|etb|6|Ethiopian Birr|ETB|2400000000000|120
uugx|ugx|6|Ugandan Shilling|UGX|74000000000000|3700
utzs|tzs|6|Tanzanian Shilling|TZS|52000000000000|2600
urwf|rwf|6|Rwandan Franc|RWF|27000000000000|1350
uzmw|zmw|6|Zambian Kwacha|ZMW|520000000000|26
umzn|mzn|6|Mozambican Metical|MZN|1280000000000|64
uaoa|aoa|6|Angolan Kwanza|AOA|18200000000000|910
ubwp|bwp|6|Botswana Pula|BWP|260000000000|13
unad|nad|6|Namibian Dollar|NAD|360000000000|18
umur|mur|6|Mauritian Rupee|MUR|920000000000|46
ugmd|gmd|6|Gambian Dalasi|GMD|1400000000000|70
ugnf|gnf|6|Guinean Franc|GNF|172000000000000|8600
ulrd|lrd|6|Liberian Dollar|LRD|3860000000000|193
usle|sle|6|Sierra Leonean Leone|SLE|440000000000|22
umwk|mwk|6|Malawian Kwacha|MWK|34600000000000|1730
umga|mga|6|Malagasy Ariary|MGA|90000000000000|4500
ucdf|cdf|6|Congolese Franc|CDF|56000000000000|2800
usdg|sdg|6|Sudanese Pound|SDG|12000000000000|600
ussp|ssp|6|South Sudanese Pound|SSP|90000000000000|4500
ulyd|lyd|6|Libyan Dinar|LYD|100000000000|5
usos|sos|6|Somali Shilling|SOS|11420000000000|571
udjf|djf|6|Djiboutian Franc|DJF|3560000000000|178
uern|ern|6|Eritrean Nakfa|ERN|300000000000|15
ubif|bif|6|Burundian Franc|BIF|58000000000000|2900
uscr|scr|6|Seychellois Rupee|SCR|260000000000|13
ucve|cve|6|Cape Verdean Escudo|CVE|2020000000000|101
ustn|stn|6|São Tomé and Príncipe Dobra|STN|440000000000|22
ukmf|kmf|6|Comorian Franc|KMF|9000000000000|450
ulsl|lsl|6|Lesotho Loti|LSL|360000000000|18
uszl|szl|6|Eswatini Lilangeni|SZL|360000000000|18
umru|mru|6|Mauritanian Ouguiya|MRU|800000000000|40
uzwg|zwg|6|Zimbabwe Gold|ZWG|260000000000|13
uusdc|usdc|6|USD Coin|USDC|20000000000|1
ueurc|eurc|6|Euro Coin|EURC|18400000000|0.92"

echo "=== 1/4  issuer applications ==="
UNAPPROVED=""
while IFS="|" read -r d disp exp name sym pool rate; do
  [ -z "$d" ] && continue
  if approved "$d"; then echo "  $d: issuer approved"; continue; fi
  if applied "$d"; then
    echo "  $d: application on file, awaiting approval"
  else
    h=$(tx stablecoin register-currency "$d" "$disp" "$exp" "$name" "$sym" "$name stablecoin on Yamale")
    [ -n "$h" ] && sleep 6
    say "$d register" "$h" || true
  fi
  UNAPPROVED="$UNAPPROVED $d"
done <<EOF
$CURRENCIES
EOF

if [ -n "$UNAPPROVED" ]; then
  echo
  echo "=== 2/4  governance approval required -- stopping here ==="
  echo "An application is not a currency. Not approved yet:$UNAPPROVED"
  {
    echo "{"
    echo '  "messages": ['
    first=1
    for d in $UNAPPROVED; do
      [ $first -eq 1 ] || printf ",\n"
      first=0
      printf "    {\n"
      echo '      "@type": "/blockchain.stablecoin.v1.MsgApproveIssuer",'
      echo "      \"authority\": \"$GOV\","
      echo "      \"denom\": \"$d\","
      echo '      "approve": true'
      printf "    }"
    done
    printf "\n  ],\n"
    echo '  "metadata": "",'
    echo '  "deposit": "10000000uyml",'
    echo '  "title": "Admit the foundation as issuer of new currencies",'
    echo "  \"summary\": \"Admits the foundation as the issuer of:$UNAPPROVED. Devnet test currencies with no reserve behind them.\""
    echo "}"
  } > $PROPOSAL
  echo "Proposal written to $PROPOSAL. Submit it, let it pass, re-run this script:"
  echo "  $B tx gov submit-proposal $PROPOSAL --from foundation $TX --gas 400000"
  exit 1
fi

# The pool list is read before minting, because what to mint depends on it.
# `list-pool`, and a list that cannot be read stops the run: an empty one would
# read as "no pools" and open every one of them again.
POOLS=$($B query amm list-pool --page-limit 1000 --home $H -o json 2>/dev/null)
grep -q '"pool"' <<<"$POOLS" || { echo "cannot read the pool list -- refusing to guess which pools exist"; exit 1; }
pooled() { grep -q "\"$1\"" <<<"$POOLS"; }

echo "=== 3/4  topping the foundation up (faucet stock, plus the pool side if unpooled) ==="
while IFS="|" read -r d disp exp name sym pool rate; do
  [ -z "$d" ] && continue
  need=$FAUCET_STOCK
  pooled "$d" || need=$((need + pool))
  held=$(have "$d")
  if [ "$held" -ge "$need" ]; then echo "  $d: holds $held of $need, skip"; continue; fi
  h=$(tx stablecoin mint-coin "$d" $((need - held)) "$F")
  [ -n "$h" ] && sleep 6
  say "$d mint $((need - held))" "$h" || true
done <<EOF
$CURRENCIES
EOF

echo "=== 4/4  opening the YML pools that do not exist ==="
while IFS="|" read -r d disp exp name sym pool rate; do
  [ -z "$d" ] && continue
  if pooled "$d"; then echo "  uyml/$d exists, skip"; continue; fi
  y=$(have uyml); c=$(have "$d")
  # In a dry run nothing was minted, so the currency side is what the mint
  # above would have left.
  [ -n "${DRY_RUN:-}" ] && [ "$c" -lt "$pool" ] && c=$pool
  if [ "$y" -lt "$YML_DEPTH" ] || [ "$c" -lt "$pool" ]; then
    echo "  uyml/$d NOT OPENED: the foundation holds $y uyml and $c $d; the pool needs $YML_DEPTH and $pool"
    continue
  fi
  h=$(tx amm create-pool uyml "$YML_DEPTH" "$d" "$pool" 30)
  [ -n "$h" ] && sleep 6
  say "pool uyml/$d (1 YML = $rate $sym)" "$h" || true
done <<EOF
$CURRENCIES
EOF
echo "=== done ==="
