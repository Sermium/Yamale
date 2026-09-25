#!/bin/bash
# GENERATED from african-currencies.json by generate.py -- do not hand-edit.
set -uo pipefail
B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node
K="--keyring-backend test --home $H"
TX="--chain-id yamale-devnet-2 $K --fees 2000uyml --yes -o json"
F=$($B keys show foundation -a $K)
GOV=$($B query auth module-account gov --home $H -o json 2>/dev/null | grep -oE '"address":"[a-z0-9]+"' | head -1 | cut -d'"' -f4)
PROPOSAL=/tmp/approve-issuers.json

# Did it work, in words. Every tx this script sends goes through here,
# because a failure it cannot see is a failure it will repeat.
say() {
  if [ -z "$2" ]; then echo "  $1: NOT BROADCAST"; return 1; fi
  local c
  c=$($B query tx "$2" --home $H -o json 2>/dev/null | grep -oE '"code":[0-9]+' | head -1 | cut -d: -f2)
  if [ "$c" = "0" ]; then echo "  $1: ok"; else echo "  $1: FAILED (code ${c:-no-such-tx})"; return 1; fi
}

# State, not status codes. A re-run must ask the chain what is true rather
# than trust its own earlier output, and "already exists" is a success here.
approved() { $B query stablecoin get-approved-issuer "$1" --home $H -o json >/dev/null 2>&1; }
applied()  { $B query stablecoin get-issuer-application "$1" --home $H -o json >/dev/null 2>&1; }
supply()   { $B query bank total-supply-of "$1" --home $H -o json 2>/dev/null | grep -oE '"amount": *"[0-9]+"' | tail -1 | grep -oE '[0-9]+'; }

# denom|display|exponent|name|symbol
CURRENCIES="uegp|egp|6|Egyptian Pound|EGP
umad|mad|6|Moroccan Dirham|MAD
udzd|dzd|6|Algerian Dinar|DZD
utnd|tnd|6|Tunisian Dinar|TND
uetb|etb|6|Ethiopian Birr|ETB
uugx|ugx|6|Ugandan Shilling|UGX
utzs|tzs|6|Tanzanian Shilling|TZS
urwf|rwf|6|Rwandan Franc|RWF
uzmw|zmw|6|Zambian Kwacha|ZMW
umzn|mzn|6|Mozambican Metical|MZN
uaoa|aoa|6|Angolan Kwanza|AOA
ubwp|bwp|6|Botswana Pula|BWP
unad|nad|6|Namibian Dollar|NAD
umur|mur|6|Mauritian Rupee|MUR
ugmd|gmd|6|Gambian Dalasi|GMD
ugnf|gnf|6|Guinean Franc|GNF
ulrd|lrd|6|Liberian Dollar|LRD
usle|sle|6|Sierra Leonean Leone|SLE
umwk|mwk|6|Malawian Kwacha|MWK
umga|mga|6|Malagasy Ariary|MGA
ucdf|cdf|6|Congolese Franc|CDF
usdg|sdg|6|Sudanese Pound|SDG
ussp|ssp|6|South Sudanese Pound|SSP
ulyd|lyd|6|Libyan Dinar|LYD
usos|sos|6|Somali Shilling|SOS
udjf|djf|6|Djiboutian Franc|DJF
uern|ern|6|Eritrean Nakfa|ERN
ubif|bif|6|Burundian Franc|BIF
uscr|scr|6|Seychellois Rupee|SCR
ucve|cve|6|Cape Verdean Escudo|CVE
ustn|stn|6|São Tomé and Príncipe Dobra|STN
ukmf|kmf|6|Comorian Franc|KMF
ulsl|lsl|6|Lesotho Loti|LSL
uszl|szl|6|Eswatini Lilangeni|SZL
umru|mru|6|Mauritanian Ouguiya|MRU
uzwg|zwg|6|Zimbabwe Gold|ZWG
uusdc|usdc|6|USD Coin|USDC
ueurc|eurc|6|Euro Coin|EURC"

echo "=== 1/4  filing currency applications ==="
UNAPPROVED=""
while IFS="|" read -r d disp exp name sym; do
  [ -z "$d" ] && continue
  if approved "$d"; then echo "  $d: issuer already approved"; continue; fi
  if applied "$d"; then
    echo "  $d: application already on file, awaiting approval"
  else
    h=$($B tx stablecoin register-currency "$d" "$disp" "$exp" "$name" "$sym" "$name stablecoin on Yamale" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1)
    sleep 6
    say "$d register" "$h" || true
    applied "$d" || echo "  $d: NO APPLICATION ON FILE -- the register above did not take"
  fi
  UNAPPROVED="$UNAPPROVED $d"
done <<EOF
$CURRENCIES
EOF

if [ -n "$UNAPPROVED" ]; then
  echo
  echo "=== 2/4  governance approval required -- stopping here ==="
  echo "An application is not a currency. Minting needs an approved issuer,"
  echo "and only governance or a scoped monetary authority can admit one."
  echo "Not approved yet:$UNAPPROVED"
  {
    echo "{"
    echo '  "messages": ['
    first=1
    for d in $UNAPPROVED; do
      [ $first -eq 1 ] || printf ",\\n"
      first=0
      printf "    {\\n"
      echo '      "@type": "/blockchain.stablecoin.v1.MsgApproveIssuer",'
      echo "      \"authority\": \"$GOV\","
      echo "      \"denom\": \"$d\","
      echo '      "approve": true'
      printf "    }"
    done
    printf "\\n  ],\\n"
    echo '  "metadata": "",'
    echo '  "deposit": "10000000uyml",'
    echo '  "title": "Admit the foundation as issuer of new currencies",'
    echo "  \"summary\": \"Admits the foundation, already the issuer of the currencies this chain carries, as the issuer of:$UNAPPROVED. These are devnet test currencies with no reserve behind them.\""
    echo "}"
  } > $PROPOSAL
  echo
  echo "Proposal written to $PROPOSAL. Submit it, let it pass, then re-run this script:"
  echo "  $B tx gov submit-proposal $PROPOSAL --from foundation $TX --gas 400000"
  exit 1
fi

echo "=== 3/4  minting to the foundation ==="
while IFS="|" read -r d disp exp name sym; do
  [ -z "$d" ] && continue
  have=$(supply "$d")
  # Minting is not idempotent, so a second run must not silently double the
  # supply of every currency on the chain.
  if [ -n "$have" ] && [ "$have" != "0" ]; then echo "  $d: supply $have, skip"; continue; fi
  h=$($B tx stablecoin mint-coin "$d" 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1)
  sleep 6
  say "$d mint" "$h" || true
done <<EOF
$CURRENCIES
EOF

echo "=== 4/4  opening YML-hub pools (skip any denom already pooled) ==="
POOLS=$($B query amm pool --home $H -o json 2>/dev/null)
if echo "$POOLS" | grep -q '"uegp"'; then echo "  uyml/uegp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uegp 1000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uegp (1 YML = 50 EGP)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umad"'; then echo "  uyml/umad exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umad 200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umad (1 YML = 10 MAD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"udzd"'; then echo "  uyml/udzd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 udzd 2700000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/udzd (1 YML = 135 DZD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"utnd"'; then echo "  uyml/utnd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 utnd 60000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/utnd (1 YML = 3 TND)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uetb"'; then echo "  uyml/uetb exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uetb 2400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uetb (1 YML = 120 ETB)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uugx"'; then echo "  uyml/uugx exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uugx 74000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uugx (1 YML = 3700 UGX)" "$h" || true; fi
if echo "$POOLS" | grep -q '"utzs"'; then echo "  uyml/utzs exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 utzs 52000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/utzs (1 YML = 2600 TZS)" "$h" || true; fi
if echo "$POOLS" | grep -q '"urwf"'; then echo "  uyml/urwf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 urwf 27000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/urwf (1 YML = 1350 RWF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uzmw"'; then echo "  uyml/uzmw exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uzmw 520000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uzmw (1 YML = 26 ZMW)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umzn"'; then echo "  uyml/umzn exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umzn 1280000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umzn (1 YML = 64 MZN)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uaoa"'; then echo "  uyml/uaoa exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uaoa 18200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uaoa (1 YML = 910 AOA)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ubwp"'; then echo "  uyml/ubwp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ubwp 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ubwp (1 YML = 13 BWP)" "$h" || true; fi
if echo "$POOLS" | grep -q '"unad"'; then echo "  uyml/unad exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 unad 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/unad (1 YML = 18 NAD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umur"'; then echo "  uyml/umur exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umur 920000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umur (1 YML = 46 MUR)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ugmd"'; then echo "  uyml/ugmd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ugmd 1400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ugmd (1 YML = 70 GMD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ugnf"'; then echo "  uyml/ugnf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ugnf 172000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ugnf (1 YML = 8600 GNF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ulrd"'; then echo "  uyml/ulrd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulrd 3860000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulrd (1 YML = 193 LRD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"usle"'; then echo "  uyml/usle exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usle 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usle (1 YML = 22 SLE)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umwk"'; then echo "  uyml/umwk exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umwk 34600000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umwk (1 YML = 1730 MWK)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umga"'; then echo "  uyml/umga exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umga 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umga (1 YML = 4500 MGA)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ucdf"'; then echo "  uyml/ucdf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ucdf 56000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ucdf (1 YML = 2800 CDF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"usdg"'; then echo "  uyml/usdg exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usdg 12000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usdg (1 YML = 600 SDG)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ussp"'; then echo "  uyml/ussp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ussp 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ussp (1 YML = 4500 SSP)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ulyd"'; then echo "  uyml/ulyd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulyd 100000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulyd (1 YML = 5 LYD)" "$h" || true; fi
if echo "$POOLS" | grep -q '"usos"'; then echo "  uyml/usos exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usos 11420000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usos (1 YML = 571 SOS)" "$h" || true; fi
if echo "$POOLS" | grep -q '"udjf"'; then echo "  uyml/udjf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 udjf 3560000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/udjf (1 YML = 178 DJF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uern"'; then echo "  uyml/uern exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uern 300000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uern (1 YML = 15 ERN)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ubif"'; then echo "  uyml/ubif exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ubif 58000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ubif (1 YML = 2900 BIF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uscr"'; then echo "  uyml/uscr exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uscr 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uscr (1 YML = 13 SCR)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ucve"'; then echo "  uyml/ucve exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ucve 2020000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ucve (1 YML = 101 CVE)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ustn"'; then echo "  uyml/ustn exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ustn 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ustn (1 YML = 22 STN)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ukmf"'; then echo "  uyml/ukmf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ukmf 9000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ukmf (1 YML = 450 KMF)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ulsl"'; then echo "  uyml/ulsl exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulsl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulsl (1 YML = 18 LSL)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uszl"'; then echo "  uyml/uszl exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uszl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uszl (1 YML = 18 SZL)" "$h" || true; fi
if echo "$POOLS" | grep -q '"umru"'; then echo "  uyml/umru exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umru 800000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umru (1 YML = 40 MRU)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uzwg"'; then echo "  uyml/uzwg exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uzwg 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uzwg (1 YML = 13 ZWG)" "$h" || true; fi
if echo "$POOLS" | grep -q '"uusdc"'; then echo "  uyml/uusdc exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uusdc 20000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uusdc (1 YML = 1 USDC)" "$h" || true; fi
if echo "$POOLS" | grep -q '"ueurc"'; then echo "  uyml/ueurc exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ueurc 18400000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ueurc (1 YML = 0.92 EURC)" "$h" || true; fi
echo "=== done ==="
