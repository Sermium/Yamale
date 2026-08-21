#!/bin/bash
# GENERATED from african-currencies.json by generate.py -- do not hand-edit.
set -uo pipefail
B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node
K="--keyring-backend test --home $H"
TX="--chain-id yamale-devnet-2 $K --fees 2000uyml --yes -o json"
F=$($B keys show foundation -a $K)
code() { $B query tx "$1" --home $H -o json 2>/dev/null | grep -oE '"code":[0-9]+' | head -1; }
POOLS=$($B query amm pool --home $H -o json 2>/dev/null)
echo "=== registering + minting new currencies ==="
$B tx stablecoin register-currency uegp egp 6 "Egyptian Pound" "EGP" "Egyptian Pound stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uegp 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uegp: mint $(code $h)"
$B tx stablecoin register-currency umad mad 6 "Moroccan Dirham" "MAD" "Moroccan Dirham stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umad 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umad: mint $(code $h)"
$B tx stablecoin register-currency udzd dzd 6 "Algerian Dinar" "DZD" "Algerian Dinar stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin udzd 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  udzd: mint $(code $h)"
$B tx stablecoin register-currency utnd tnd 6 "Tunisian Dinar" "TND" "Tunisian Dinar stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin utnd 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  utnd: mint $(code $h)"
$B tx stablecoin register-currency uetb etb 6 "Ethiopian Birr" "ETB" "Ethiopian Birr stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uetb 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uetb: mint $(code $h)"
$B tx stablecoin register-currency uugx ugx 6 "Ugandan Shilling" "UGX" "Ugandan Shilling stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uugx 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uugx: mint $(code $h)"
$B tx stablecoin register-currency utzs tzs 6 "Tanzanian Shilling" "TZS" "Tanzanian Shilling stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin utzs 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  utzs: mint $(code $h)"
$B tx stablecoin register-currency urwf rwf 6 "Rwandan Franc" "RWF" "Rwandan Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin urwf 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  urwf: mint $(code $h)"
$B tx stablecoin register-currency uzmw zmw 6 "Zambian Kwacha" "ZMW" "Zambian Kwacha stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uzmw 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uzmw: mint $(code $h)"
$B tx stablecoin register-currency umzn mzn 6 "Mozambican Metical" "MZN" "Mozambican Metical stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umzn 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umzn: mint $(code $h)"
$B tx stablecoin register-currency uaoa aoa 6 "Angolan Kwanza" "AOA" "Angolan Kwanza stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uaoa 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uaoa: mint $(code $h)"
$B tx stablecoin register-currency ubwp bwp 6 "Botswana Pula" "BWP" "Botswana Pula stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ubwp 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ubwp: mint $(code $h)"
$B tx stablecoin register-currency unad nad 6 "Namibian Dollar" "NAD" "Namibian Dollar stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin unad 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  unad: mint $(code $h)"
$B tx stablecoin register-currency umur mur 6 "Mauritian Rupee" "MUR" "Mauritian Rupee stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umur 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umur: mint $(code $h)"
$B tx stablecoin register-currency ugmd gmd 6 "Gambian Dalasi" "GMD" "Gambian Dalasi stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ugmd 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ugmd: mint $(code $h)"
$B tx stablecoin register-currency ugnf gnf 6 "Guinean Franc" "GNF" "Guinean Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ugnf 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ugnf: mint $(code $h)"
$B tx stablecoin register-currency ulrd lrd 6 "Liberian Dollar" "LRD" "Liberian Dollar stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ulrd 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ulrd: mint $(code $h)"
$B tx stablecoin register-currency usle sle 6 "Sierra Leonean Leone" "SLE" "Sierra Leonean Leone stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin usle 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  usle: mint $(code $h)"
$B tx stablecoin register-currency umwk mwk 6 "Malawian Kwacha" "MWK" "Malawian Kwacha stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umwk 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umwk: mint $(code $h)"
$B tx stablecoin register-currency umga mga 6 "Malagasy Ariary" "MGA" "Malagasy Ariary stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umga 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umga: mint $(code $h)"
$B tx stablecoin register-currency ucdf cdf 6 "Congolese Franc" "CDF" "Congolese Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ucdf 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ucdf: mint $(code $h)"
$B tx stablecoin register-currency usdg sdg 6 "Sudanese Pound" "SDG" "Sudanese Pound stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin usdg 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  usdg: mint $(code $h)"
$B tx stablecoin register-currency ussp ssp 6 "South Sudanese Pound" "SSP" "South Sudanese Pound stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ussp 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ussp: mint $(code $h)"
$B tx stablecoin register-currency ulyd lyd 6 "Libyan Dinar" "LYD" "Libyan Dinar stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ulyd 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ulyd: mint $(code $h)"
$B tx stablecoin register-currency usos sos 6 "Somali Shilling" "SOS" "Somali Shilling stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin usos 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  usos: mint $(code $h)"
$B tx stablecoin register-currency udjf djf 6 "Djiboutian Franc" "DJF" "Djiboutian Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin udjf 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  udjf: mint $(code $h)"
$B tx stablecoin register-currency uern ern 6 "Eritrean Nakfa" "ERN" "Eritrean Nakfa stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uern 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uern: mint $(code $h)"
$B tx stablecoin register-currency ubif bif 6 "Burundian Franc" "BIF" "Burundian Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ubif 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ubif: mint $(code $h)"
$B tx stablecoin register-currency uscr scr 6 "Seychellois Rupee" "SCR" "Seychellois Rupee stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uscr 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uscr: mint $(code $h)"
$B tx stablecoin register-currency ucve cve 6 "Cape Verdean Escudo" "CVE" "Cape Verdean Escudo stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ucve 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ucve: mint $(code $h)"
$B tx stablecoin register-currency ustn stn 6 "São Tomé and Príncipe Dobra" "STN" "São Tomé and Príncipe Dobra stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ustn 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ustn: mint $(code $h)"
$B tx stablecoin register-currency ukmf kmf 6 "Comorian Franc" "KMF" "Comorian Franc stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ukmf 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ukmf: mint $(code $h)"
$B tx stablecoin register-currency ulsl lsl 6 "Lesotho Loti" "LSL" "Lesotho Loti stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ulsl 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ulsl: mint $(code $h)"
$B tx stablecoin register-currency uszl szl 6 "Eswatini Lilangeni" "SZL" "Eswatini Lilangeni stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uszl 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uszl: mint $(code $h)"
$B tx stablecoin register-currency umru mru 6 "Mauritanian Ouguiya" "MRU" "Mauritanian Ouguiya stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin umru 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  umru: mint $(code $h)"
$B tx stablecoin register-currency uzwg zwg 6 "Zimbabwe Gold" "ZWG" "Zimbabwe Gold stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uzwg 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uzwg: mint $(code $h)"
$B tx stablecoin register-currency uusdc usdc 6 "USD Coin" "USDC" "USD Coin stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin uusdc 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  uusdc: mint $(code $h)"
$B tx stablecoin register-currency ueurc eurc 6 "Euro Coin" "EURC" "Euro Coin stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6
h=$($B tx stablecoin mint-coin ueurc 10000000000000000 "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  ueurc: mint $(code $h)"
echo "=== opening YML-hub pools (skip any denom already pooled) ==="
if echo "$POOLS" | grep -q '"uegp"'; then echo "  uyml/uegp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uegp 1000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uegp (1 YML = 50 EGP): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umad"'; then echo "  uyml/umad exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umad 200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umad (1 YML = 10 MAD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"udzd"'; then echo "  uyml/udzd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 udzd 2700000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/udzd (1 YML = 135 DZD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"utnd"'; then echo "  uyml/utnd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 utnd 60000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/utnd (1 YML = 3 TND): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uetb"'; then echo "  uyml/uetb exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uetb 2400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uetb (1 YML = 120 ETB): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uugx"'; then echo "  uyml/uugx exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uugx 74000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uugx (1 YML = 3700 UGX): $(code $h)"; fi
if echo "$POOLS" | grep -q '"utzs"'; then echo "  uyml/utzs exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 utzs 52000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/utzs (1 YML = 2600 TZS): $(code $h)"; fi
if echo "$POOLS" | grep -q '"urwf"'; then echo "  uyml/urwf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 urwf 27000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/urwf (1 YML = 1350 RWF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uzmw"'; then echo "  uyml/uzmw exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uzmw 520000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uzmw (1 YML = 26 ZMW): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umzn"'; then echo "  uyml/umzn exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umzn 1280000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umzn (1 YML = 64 MZN): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uaoa"'; then echo "  uyml/uaoa exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uaoa 18200000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uaoa (1 YML = 910 AOA): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ubwp"'; then echo "  uyml/ubwp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ubwp 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ubwp (1 YML = 13 BWP): $(code $h)"; fi
if echo "$POOLS" | grep -q '"unad"'; then echo "  uyml/unad exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 unad 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/unad (1 YML = 18 NAD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umur"'; then echo "  uyml/umur exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umur 920000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umur (1 YML = 46 MUR): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ugmd"'; then echo "  uyml/ugmd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ugmd 1400000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ugmd (1 YML = 70 GMD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ugnf"'; then echo "  uyml/ugnf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ugnf 172000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ugnf (1 YML = 8600 GNF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ulrd"'; then echo "  uyml/ulrd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulrd 3860000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ulrd (1 YML = 193 LRD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"usle"'; then echo "  uyml/usle exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usle 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/usle (1 YML = 22 SLE): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umwk"'; then echo "  uyml/umwk exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umwk 34600000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umwk (1 YML = 1730 MWK): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umga"'; then echo "  uyml/umga exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umga 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umga (1 YML = 4500 MGA): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ucdf"'; then echo "  uyml/ucdf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ucdf 56000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ucdf (1 YML = 2800 CDF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"usdg"'; then echo "  uyml/usdg exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usdg 12000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/usdg (1 YML = 600 SDG): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ussp"'; then echo "  uyml/ussp exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ussp 90000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ussp (1 YML = 4500 SSP): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ulyd"'; then echo "  uyml/ulyd exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulyd 100000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ulyd (1 YML = 5 LYD): $(code $h)"; fi
if echo "$POOLS" | grep -q '"usos"'; then echo "  uyml/usos exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 usos 11420000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/usos (1 YML = 571 SOS): $(code $h)"; fi
if echo "$POOLS" | grep -q '"udjf"'; then echo "  uyml/udjf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 udjf 3560000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/udjf (1 YML = 178 DJF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uern"'; then echo "  uyml/uern exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uern 300000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uern (1 YML = 15 ERN): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ubif"'; then echo "  uyml/ubif exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ubif 58000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ubif (1 YML = 2900 BIF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uscr"'; then echo "  uyml/uscr exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uscr 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uscr (1 YML = 13 SCR): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ucve"'; then echo "  uyml/ucve exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ucve 2020000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ucve (1 YML = 101 CVE): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ustn"'; then echo "  uyml/ustn exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ustn 440000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ustn (1 YML = 22 STN): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ukmf"'; then echo "  uyml/ukmf exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ukmf 9000000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ukmf (1 YML = 450 KMF): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ulsl"'; then echo "  uyml/ulsl exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ulsl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ulsl (1 YML = 18 LSL): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uszl"'; then echo "  uyml/uszl exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uszl 360000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uszl (1 YML = 18 SZL): $(code $h)"; fi
if echo "$POOLS" | grep -q '"umru"'; then echo "  uyml/umru exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 umru 800000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/umru (1 YML = 40 MRU): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uzwg"'; then echo "  uyml/uzwg exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uzwg 260000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uzwg (1 YML = 13 ZWG): $(code $h)"; fi
if echo "$POOLS" | grep -q '"uusdc"'; then echo "  uyml/uusdc exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 uusdc 20000000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/uusdc (1 YML = 1 USDC): $(code $h)"; fi
if echo "$POOLS" | grep -q '"ueurc"'; then echo "  uyml/ueurc exists, skip"; else h=$($B tx amm create-pool uyml 20000000000 ueurc 18400000000 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  pool uyml/ueurc (1 YML = 0.92 EURC): $(code $h)"; fi
echo "=== done ==="
