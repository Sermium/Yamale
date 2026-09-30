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
  h=$($B tx stablecoin mint-coin "$d" 10000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1)
  sleep 6
  say "$d mint" "$h" || true
done <<EOF
$CURRENCIES
EOF

echo "=== 4/4  opening YML-hub pools (skip any denom already pooled) ==="
POOLS=$($B query amm list-pool --page-limit 1000 --home $H -o json 2>/dev/null)
grep -q '"pool"' <<<"$POOLS" || { echo "  cannot read the pool list -- refusing to guess which pools exist"; exit 1; }
# What the foundation can actually put in. A pool it cannot fund is
# reported as such rather than attempted and charged a fee to fail.
have() { $B query bank balance "$F" "$1" --home $H -o json 2>/dev/null | grep -oE '"amount": *"[0-9]+"' | grep -oE '[0-9]+'; }
if grep -q '"uegp"' <<<"$POOLS"; then echo "  uyml/uegp exists, skip"; elif [ "$(have uegp)" -lt 1000000000000 ] 2>/dev/null || [ -z "$(have uegp)" ]; then echo "  uyml/uegp NOT OPENED: the foundation holds $(have uegp) uegp and the pool needs 1000000000000"; else h=$($B tx amm create-pool uyml 20000000000 uegp 1000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uegp (1 YML = 50 EGP)" "$h" || true; fi
if grep -q '"umad"' <<<"$POOLS"; then echo "  uyml/umad exists, skip"; elif [ "$(have umad)" -lt 200000000000 ] 2>/dev/null || [ -z "$(have umad)" ]; then echo "  uyml/umad NOT OPENED: the foundation holds $(have umad) umad and the pool needs 200000000000"; else h=$($B tx amm create-pool uyml 20000000000 umad 200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umad (1 YML = 10 MAD)" "$h" || true; fi
if grep -q '"udzd"' <<<"$POOLS"; then echo "  uyml/udzd exists, skip"; elif [ "$(have udzd)" -lt 2700000000000 ] 2>/dev/null || [ -z "$(have udzd)" ]; then echo "  uyml/udzd NOT OPENED: the foundation holds $(have udzd) udzd and the pool needs 2700000000000"; else h=$($B tx amm create-pool uyml 20000000000 udzd 2700000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/udzd (1 YML = 135 DZD)" "$h" || true; fi
if grep -q '"utnd"' <<<"$POOLS"; then echo "  uyml/utnd exists, skip"; elif [ "$(have utnd)" -lt 60000000000 ] 2>/dev/null || [ -z "$(have utnd)" ]; then echo "  uyml/utnd NOT OPENED: the foundation holds $(have utnd) utnd and the pool needs 60000000000"; else h=$($B tx amm create-pool uyml 20000000000 utnd 60000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/utnd (1 YML = 3 TND)" "$h" || true; fi
if grep -q '"uetb"' <<<"$POOLS"; then echo "  uyml/uetb exists, skip"; elif [ "$(have uetb)" -lt 2400000000000 ] 2>/dev/null || [ -z "$(have uetb)" ]; then echo "  uyml/uetb NOT OPENED: the foundation holds $(have uetb) uetb and the pool needs 2400000000000"; else h=$($B tx amm create-pool uyml 20000000000 uetb 2400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uetb (1 YML = 120 ETB)" "$h" || true; fi
if grep -q '"uugx"' <<<"$POOLS"; then echo "  uyml/uugx exists, skip"; elif [ "$(have uugx)" -lt 74000000000000 ] 2>/dev/null || [ -z "$(have uugx)" ]; then echo "  uyml/uugx NOT OPENED: the foundation holds $(have uugx) uugx and the pool needs 74000000000000"; else h=$($B tx amm create-pool uyml 20000000000 uugx 74000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uugx (1 YML = 3700 UGX)" "$h" || true; fi
if grep -q '"utzs"' <<<"$POOLS"; then echo "  uyml/utzs exists, skip"; elif [ "$(have utzs)" -lt 52000000000000 ] 2>/dev/null || [ -z "$(have utzs)" ]; then echo "  uyml/utzs NOT OPENED: the foundation holds $(have utzs) utzs and the pool needs 52000000000000"; else h=$($B tx amm create-pool uyml 20000000000 utzs 52000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/utzs (1 YML = 2600 TZS)" "$h" || true; fi
if grep -q '"urwf"' <<<"$POOLS"; then echo "  uyml/urwf exists, skip"; elif [ "$(have urwf)" -lt 27000000000000 ] 2>/dev/null || [ -z "$(have urwf)" ]; then echo "  uyml/urwf NOT OPENED: the foundation holds $(have urwf) urwf and the pool needs 27000000000000"; else h=$($B tx amm create-pool uyml 20000000000 urwf 27000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/urwf (1 YML = 1350 RWF)" "$h" || true; fi
if grep -q '"uzmw"' <<<"$POOLS"; then echo "  uyml/uzmw exists, skip"; elif [ "$(have uzmw)" -lt 520000000000 ] 2>/dev/null || [ -z "$(have uzmw)" ]; then echo "  uyml/uzmw NOT OPENED: the foundation holds $(have uzmw) uzmw and the pool needs 520000000000"; else h=$($B tx amm create-pool uyml 20000000000 uzmw 520000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uzmw (1 YML = 26 ZMW)" "$h" || true; fi
if grep -q '"umzn"' <<<"$POOLS"; then echo "  uyml/umzn exists, skip"; elif [ "$(have umzn)" -lt 1280000000000 ] 2>/dev/null || [ -z "$(have umzn)" ]; then echo "  uyml/umzn NOT OPENED: the foundation holds $(have umzn) umzn and the pool needs 1280000000000"; else h=$($B tx amm create-pool uyml 20000000000 umzn 1280000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umzn (1 YML = 64 MZN)" "$h" || true; fi
if grep -q '"uaoa"' <<<"$POOLS"; then echo "  uyml/uaoa exists, skip"; elif [ "$(have uaoa)" -lt 18200000000000 ] 2>/dev/null || [ -z "$(have uaoa)" ]; then echo "  uyml/uaoa NOT OPENED: the foundation holds $(have uaoa) uaoa and the pool needs 18200000000000"; else h=$($B tx amm create-pool uyml 20000000000 uaoa 18200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uaoa (1 YML = 910 AOA)" "$h" || true; fi
if grep -q '"ubwp"' <<<"$POOLS"; then echo "  uyml/ubwp exists, skip"; elif [ "$(have ubwp)" -lt 260000000000 ] 2>/dev/null || [ -z "$(have ubwp)" ]; then echo "  uyml/ubwp NOT OPENED: the foundation holds $(have ubwp) ubwp and the pool needs 260000000000"; else h=$($B tx amm create-pool uyml 20000000000 ubwp 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ubwp (1 YML = 13 BWP)" "$h" || true; fi
if grep -q '"unad"' <<<"$POOLS"; then echo "  uyml/unad exists, skip"; elif [ "$(have unad)" -lt 360000000000 ] 2>/dev/null || [ -z "$(have unad)" ]; then echo "  uyml/unad NOT OPENED: the foundation holds $(have unad) unad and the pool needs 360000000000"; else h=$($B tx amm create-pool uyml 20000000000 unad 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/unad (1 YML = 18 NAD)" "$h" || true; fi
if grep -q '"umur"' <<<"$POOLS"; then echo "  uyml/umur exists, skip"; elif [ "$(have umur)" -lt 920000000000 ] 2>/dev/null || [ -z "$(have umur)" ]; then echo "  uyml/umur NOT OPENED: the foundation holds $(have umur) umur and the pool needs 920000000000"; else h=$($B tx amm create-pool uyml 20000000000 umur 920000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umur (1 YML = 46 MUR)" "$h" || true; fi
if grep -q '"ugmd"' <<<"$POOLS"; then echo "  uyml/ugmd exists, skip"; elif [ "$(have ugmd)" -lt 1400000000000 ] 2>/dev/null || [ -z "$(have ugmd)" ]; then echo "  uyml/ugmd NOT OPENED: the foundation holds $(have ugmd) ugmd and the pool needs 1400000000000"; else h=$($B tx amm create-pool uyml 20000000000 ugmd 1400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ugmd (1 YML = 70 GMD)" "$h" || true; fi
if grep -q '"ugnf"' <<<"$POOLS"; then echo "  uyml/ugnf exists, skip"; elif [ "$(have ugnf)" -lt 172000000000000 ] 2>/dev/null || [ -z "$(have ugnf)" ]; then echo "  uyml/ugnf NOT OPENED: the foundation holds $(have ugnf) ugnf and the pool needs 172000000000000"; else h=$($B tx amm create-pool uyml 20000000000 ugnf 172000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ugnf (1 YML = 8600 GNF)" "$h" || true; fi
if grep -q '"ulrd"' <<<"$POOLS"; then echo "  uyml/ulrd exists, skip"; elif [ "$(have ulrd)" -lt 3860000000000 ] 2>/dev/null || [ -z "$(have ulrd)" ]; then echo "  uyml/ulrd NOT OPENED: the foundation holds $(have ulrd) ulrd and the pool needs 3860000000000"; else h=$($B tx amm create-pool uyml 20000000000 ulrd 3860000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulrd (1 YML = 193 LRD)" "$h" || true; fi
if grep -q '"usle"' <<<"$POOLS"; then echo "  uyml/usle exists, skip"; elif [ "$(have usle)" -lt 440000000000 ] 2>/dev/null || [ -z "$(have usle)" ]; then echo "  uyml/usle NOT OPENED: the foundation holds $(have usle) usle and the pool needs 440000000000"; else h=$($B tx amm create-pool uyml 20000000000 usle 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usle (1 YML = 22 SLE)" "$h" || true; fi
if grep -q '"umwk"' <<<"$POOLS"; then echo "  uyml/umwk exists, skip"; elif [ "$(have umwk)" -lt 34600000000000 ] 2>/dev/null || [ -z "$(have umwk)" ]; then echo "  uyml/umwk NOT OPENED: the foundation holds $(have umwk) umwk and the pool needs 34600000000000"; else h=$($B tx amm create-pool uyml 20000000000 umwk 34600000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umwk (1 YML = 1730 MWK)" "$h" || true; fi
if grep -q '"umga"' <<<"$POOLS"; then echo "  uyml/umga exists, skip"; elif [ "$(have umga)" -lt 90000000000000 ] 2>/dev/null || [ -z "$(have umga)" ]; then echo "  uyml/umga NOT OPENED: the foundation holds $(have umga) umga and the pool needs 90000000000000"; else h=$($B tx amm create-pool uyml 20000000000 umga 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umga (1 YML = 4500 MGA)" "$h" || true; fi
if grep -q '"ucdf"' <<<"$POOLS"; then echo "  uyml/ucdf exists, skip"; elif [ "$(have ucdf)" -lt 56000000000000 ] 2>/dev/null || [ -z "$(have ucdf)" ]; then echo "  uyml/ucdf NOT OPENED: the foundation holds $(have ucdf) ucdf and the pool needs 56000000000000"; else h=$($B tx amm create-pool uyml 20000000000 ucdf 56000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ucdf (1 YML = 2800 CDF)" "$h" || true; fi
if grep -q '"usdg"' <<<"$POOLS"; then echo "  uyml/usdg exists, skip"; elif [ "$(have usdg)" -lt 12000000000000 ] 2>/dev/null || [ -z "$(have usdg)" ]; then echo "  uyml/usdg NOT OPENED: the foundation holds $(have usdg) usdg and the pool needs 12000000000000"; else h=$($B tx amm create-pool uyml 20000000000 usdg 12000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usdg (1 YML = 600 SDG)" "$h" || true; fi
if grep -q '"ussp"' <<<"$POOLS"; then echo "  uyml/ussp exists, skip"; elif [ "$(have ussp)" -lt 90000000000000 ] 2>/dev/null || [ -z "$(have ussp)" ]; then echo "  uyml/ussp NOT OPENED: the foundation holds $(have ussp) ussp and the pool needs 90000000000000"; else h=$($B tx amm create-pool uyml 20000000000 ussp 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ussp (1 YML = 4500 SSP)" "$h" || true; fi
if grep -q '"ulyd"' <<<"$POOLS"; then echo "  uyml/ulyd exists, skip"; elif [ "$(have ulyd)" -lt 100000000000 ] 2>/dev/null || [ -z "$(have ulyd)" ]; then echo "  uyml/ulyd NOT OPENED: the foundation holds $(have ulyd) ulyd and the pool needs 100000000000"; else h=$($B tx amm create-pool uyml 20000000000 ulyd 100000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulyd (1 YML = 5 LYD)" "$h" || true; fi
if grep -q '"usos"' <<<"$POOLS"; then echo "  uyml/usos exists, skip"; elif [ "$(have usos)" -lt 11420000000000 ] 2>/dev/null || [ -z "$(have usos)" ]; then echo "  uyml/usos NOT OPENED: the foundation holds $(have usos) usos and the pool needs 11420000000000"; else h=$($B tx amm create-pool uyml 20000000000 usos 11420000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/usos (1 YML = 571 SOS)" "$h" || true; fi
if grep -q '"udjf"' <<<"$POOLS"; then echo "  uyml/udjf exists, skip"; elif [ "$(have udjf)" -lt 3560000000000 ] 2>/dev/null || [ -z "$(have udjf)" ]; then echo "  uyml/udjf NOT OPENED: the foundation holds $(have udjf) udjf and the pool needs 3560000000000"; else h=$($B tx amm create-pool uyml 20000000000 udjf 3560000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/udjf (1 YML = 178 DJF)" "$h" || true; fi
if grep -q '"uern"' <<<"$POOLS"; then echo "  uyml/uern exists, skip"; elif [ "$(have uern)" -lt 300000000000 ] 2>/dev/null || [ -z "$(have uern)" ]; then echo "  uyml/uern NOT OPENED: the foundation holds $(have uern) uern and the pool needs 300000000000"; else h=$($B tx amm create-pool uyml 20000000000 uern 300000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uern (1 YML = 15 ERN)" "$h" || true; fi
if grep -q '"ubif"' <<<"$POOLS"; then echo "  uyml/ubif exists, skip"; elif [ "$(have ubif)" -lt 58000000000000 ] 2>/dev/null || [ -z "$(have ubif)" ]; then echo "  uyml/ubif NOT OPENED: the foundation holds $(have ubif) ubif and the pool needs 58000000000000"; else h=$($B tx amm create-pool uyml 20000000000 ubif 58000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ubif (1 YML = 2900 BIF)" "$h" || true; fi
if grep -q '"uscr"' <<<"$POOLS"; then echo "  uyml/uscr exists, skip"; elif [ "$(have uscr)" -lt 260000000000 ] 2>/dev/null || [ -z "$(have uscr)" ]; then echo "  uyml/uscr NOT OPENED: the foundation holds $(have uscr) uscr and the pool needs 260000000000"; else h=$($B tx amm create-pool uyml 20000000000 uscr 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uscr (1 YML = 13 SCR)" "$h" || true; fi
if grep -q '"ucve"' <<<"$POOLS"; then echo "  uyml/ucve exists, skip"; elif [ "$(have ucve)" -lt 2020000000000 ] 2>/dev/null || [ -z "$(have ucve)" ]; then echo "  uyml/ucve NOT OPENED: the foundation holds $(have ucve) ucve and the pool needs 2020000000000"; else h=$($B tx amm create-pool uyml 20000000000 ucve 2020000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ucve (1 YML = 101 CVE)" "$h" || true; fi
if grep -q '"ustn"' <<<"$POOLS"; then echo "  uyml/ustn exists, skip"; elif [ "$(have ustn)" -lt 440000000000 ] 2>/dev/null || [ -z "$(have ustn)" ]; then echo "  uyml/ustn NOT OPENED: the foundation holds $(have ustn) ustn and the pool needs 440000000000"; else h=$($B tx amm create-pool uyml 20000000000 ustn 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ustn (1 YML = 22 STN)" "$h" || true; fi
if grep -q '"ukmf"' <<<"$POOLS"; then echo "  uyml/ukmf exists, skip"; elif [ "$(have ukmf)" -lt 9000000000000 ] 2>/dev/null || [ -z "$(have ukmf)" ]; then echo "  uyml/ukmf NOT OPENED: the foundation holds $(have ukmf) ukmf and the pool needs 9000000000000"; else h=$($B tx amm create-pool uyml 20000000000 ukmf 9000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ukmf (1 YML = 450 KMF)" "$h" || true; fi
if grep -q '"ulsl"' <<<"$POOLS"; then echo "  uyml/ulsl exists, skip"; elif [ "$(have ulsl)" -lt 360000000000 ] 2>/dev/null || [ -z "$(have ulsl)" ]; then echo "  uyml/ulsl NOT OPENED: the foundation holds $(have ulsl) ulsl and the pool needs 360000000000"; else h=$($B tx amm create-pool uyml 20000000000 ulsl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ulsl (1 YML = 18 LSL)" "$h" || true; fi
if grep -q '"uszl"' <<<"$POOLS"; then echo "  uyml/uszl exists, skip"; elif [ "$(have uszl)" -lt 360000000000 ] 2>/dev/null || [ -z "$(have uszl)" ]; then echo "  uyml/uszl NOT OPENED: the foundation holds $(have uszl) uszl and the pool needs 360000000000"; else h=$($B tx amm create-pool uyml 20000000000 uszl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uszl (1 YML = 18 SZL)" "$h" || true; fi
if grep -q '"umru"' <<<"$POOLS"; then echo "  uyml/umru exists, skip"; elif [ "$(have umru)" -lt 800000000000 ] 2>/dev/null || [ -z "$(have umru)" ]; then echo "  uyml/umru NOT OPENED: the foundation holds $(have umru) umru and the pool needs 800000000000"; else h=$($B tx amm create-pool uyml 20000000000 umru 800000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/umru (1 YML = 40 MRU)" "$h" || true; fi
if grep -q '"uzwg"' <<<"$POOLS"; then echo "  uyml/uzwg exists, skip"; elif [ "$(have uzwg)" -lt 260000000000 ] 2>/dev/null || [ -z "$(have uzwg)" ]; then echo "  uyml/uzwg NOT OPENED: the foundation holds $(have uzwg) uzwg and the pool needs 260000000000"; else h=$($B tx amm create-pool uyml 20000000000 uzwg 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uzwg (1 YML = 13 ZWG)" "$h" || true; fi
if grep -q '"uusdc"' <<<"$POOLS"; then echo "  uyml/uusdc exists, skip"; elif [ "$(have uusdc)" -lt 20000000000 ] 2>/dev/null || [ -z "$(have uusdc)" ]; then echo "  uyml/uusdc NOT OPENED: the foundation holds $(have uusdc) uusdc and the pool needs 20000000000"; else h=$($B tx amm create-pool uyml 20000000000 uusdc 20000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/uusdc (1 YML = 1 USDC)" "$h" || true; fi
if grep -q '"ueurc"' <<<"$POOLS"; then echo "  uyml/ueurc exists, skip"; elif [ "$(have ueurc)" -lt 18400000000 ] 2>/dev/null || [ -z "$(have ueurc)" ]; then echo "  uyml/ueurc NOT OPENED: the foundation holds $(have ueurc) ueurc and the pool needs 18400000000"; else h=$($B tx amm create-pool uyml 20000000000 ueurc 18400000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/ueurc (1 YML = 0.92 EURC)" "$h" || true; fi
echo "=== done ==="
