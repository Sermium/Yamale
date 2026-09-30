#!/usr/bin/env python3
"""Generate everything downstream of the canonical currency table.

One table, four outputs, so nothing drifts:
  - onchain.sh       : register + mint the new stablecoins, build the YML-hub pools
  - oracle-rates.txt : denom -> indicative rate, for the oracle feeder
  - currencies.ts    : the CURRENCIES array for the app (clients/app/src/money.ts)
  - faucet.txt       : the comma-separated --currencies list for the faucet unit

The chain carries every currency against YML (the hub); cross-currency swaps
double-hop through YML, so N currencies need N pools, not N^2.
"""
import io, json, os

HERE = os.path.dirname(os.path.abspath(__file__))
NL = chr(10)
data = json.load(io.open(os.path.join(HERE, "african-currencies.json"), encoding="utf-8"))
native = data["native"]
curs = data["currencies"]
new = [c for c in curs if not c.get("existing")]

# Pool depth in whole YML-equivalents — comparable liquidity in every market
# regardless of how many units of the currency that represents.
DEPTH = 20000
# What the foundation keeps of every currency to feed the faucet, in base units:
# ten million coins, about ten thousand claims. The script tops the foundation
# up to this plus the currency side of its pool, if the pool is not open yet --
# it no longer skips a currency merely because some of it exists, which left
# 24 currencies unable to fund a pool on 2026-09-30.
FAUCET_STOCK = 10**13

# The supply ceiling governance is asked for, in dollars' worth: the figure uusd
# was given, one billion. Converted at each currency's indicative rate, so a
# ceiling means the same thing for the franc as for the dollar. A ceiling is a
# cap on what may EXIST, not an allowance, and the default is zero -- a currency
# without one cannot be minted at all.
CEILING_USD = 10**9

# --- onchain.sh -----------------------------------------------------------
# Re-runnable, and honest about what it did. Every currency in the table, in
# four phases: file the issuer application, stop at the governance gate if any
# is unapproved, top the foundation up, open the pools that do not exist.
#
# History, because each line below is there for a reason that cost something:
#  - register-currency files an APPLICATION; the old script minted straight
#    after it and sent the register output to /dev/null, so USDC and EURC were
#    never filed and the faucet offered them for weeks.
#  - `query amm pool` is not a command. The old script searched its usage text
#    for pools, found none, and "skipped" pools that had never been opened.
#  - `echo "$POOLS" | grep -q` under pipefail is a race that reads an early
#    match as a failure. grep reads a here-string.
#  - The default mint ceiling is zero. A mint past a currency's ceiling fails,
#    and the reason is printed now rather than a bare code 1113.
#
# DRY_RUN=1 prints every transaction instead of sending it, and still reads the
# chain for everything it decides on. Run it first; it is what caught the race.
ONCHAIN = r"""#!/bin/bash
# GENERATED from african-currencies.json by generate.py -- do not hand-edit.
# DRY_RUN=1 bash onchain.sh   prints the transactions it would send.
set -uo pipefail
B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node
K="--keyring-backend test --home $H"
TX="--chain-id yamale-devnet-2 $K --fees 2000uyml --yes -o json"
F=$($B keys show foundation -a $K)
GOV=$($B query auth module-account gov --home $H -o json 2>/dev/null | grep -oE '"address":"[a-z0-9]+"' | head -1 | cut -d'"' -f4)
PROPOSAL=/tmp/approve-issuers.json
YML_DEPTH=@@YML_DEPTH@@
FAUCET_STOCK=@@FAUCET_STOCK@@

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
CURRENCIES="@@CURRENCIES@@"

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
"""

# One row per currency, the pool's currency side rounded to a whole base unit
# because a rate may be fractional (EURC 0.92).
rows = []
for c in curs:
    rows.append("%s|%s|%d|%s|%s|%d|%s" % (
        c["denom"], c["code"].lower(), c["exponent"], c["name"], c["code"],
        int(round(DEPTH * c["rate_per_yml"] * 10**6)), c["rate_per_yml"]))

onchain = (ONCHAIN
           .replace("@@YML_DEPTH@@", str(DEPTH * 10**6))
           .replace("@@FAUCET_STOCK@@", str(FAUCET_STOCK))
           .replace("@@CURRENCIES@@", NL.join(rows)))
io.open(os.path.join(HERE, "onchain.sh"), "w", encoding="utf-8", newline="\n").write(onchain)

# --- ceilings.json --------------------------------------------------------
# The mint ceilings for the table, for a governance proposal to set. It is a
# FRAGMENT, not a proposal: MsgUpdateParams replaces the whole parameter set,
# so whoever builds the proposal must merge in any live ceiling for a denom not
# in this table (uusd, on devnet-2) or governance erases it by omission.
ceilings = [{"denom": c["denom"],
             "ceiling": str(int(round(CEILING_USD * c["rate_per_yml"] * 10**c["exponent"])))}
            for c in curs]
io.open(os.path.join(HERE, "ceilings.json"), "w", encoding="utf-8", newline="\n").write(
    json.dumps({"mint_ceilings": ceilings}, indent=2) + "\n")

# --- faucet.txt -----------------------------------------------------------
faucet = ",".join(c["denom"] for c in curs)
io.open(os.path.join(HERE, "faucet.txt"), "w", encoding="utf-8", newline="\n").write(faucet + "\n")

# --- currencies.ts (the CURRENCIES array body) ----------------------------
def row(x):
    return ('  { denom: %s, name: %s, code: %s, exponent: %d },'
            % (json.dumps(x["denom"]), json.dumps(x["name"]), json.dumps(x["code"]), x["exponent"]))
lines = [row(native)] + [row(c) for c in curs]
io.open(os.path.join(HERE, "currencies.ts"), "w", encoding="utf-8", newline="\n").write("\n".join(lines) + "\n")

# --- oracle rates ---------------------------------------------------------
io.open(os.path.join(HERE, "oracle-rates.txt"), "w", encoding="utf-8", newline="\n").write(
    "\n".join("%s %s" % (c["denom"], c["rate_per_yml"]) for c in curs) + "\n")

# --- docs/guides/currencies.md --------------------------------------------
# The one place the carried-currency set is written for humans. Generated, so
# it cannot drift from what the chain, the app, and the faucet actually run.
DOCS = os.path.normpath(os.path.join(HERE, "..", "..", "docs", "guides", "currencies.md"))
md = []
md.append("# Currencies Yamale carries")
md.append("")
md.append("Generated from `scripts/currencies/african-currencies.json` — do not hand-edit;")
md.append("run `python scripts/currencies/generate.py` after changing the table.")
md.append("")
md.append("Yamale carries **%d African currencies** as stablecoins, plus the native" % len(curs))
md.append("token **YML**. Every currency is a `u`-prefixed micro-denomination (exponent 6),")
md.append("so 1 %s = 1,000,000 `%s`." % (curs[0]["code"], curs[0]["denom"]))
md.append("")
md.append("## The hub model")
md.append("")
md.append("Every currency has exactly one liquidity pool, paired against **YML**. A swap")
md.append("between two non-YML currencies routes through YML as the hub — a *double hop*")
md.append("(sell A for YML, buy B with YML). This keeps the market at **%d pools** instead" % len(curs))
md.append("of the %d a full mesh would need, and the app performs the two hops as one action." % (len(curs) * (len(curs) - 1) // 2))
md.append("")
md.append("The rate below is indicative — units of the currency per 1 YML (YML stands in")
md.append("for 1 USD on the devnet). It seeds both the pool ratio and the oracle price;")
md.append("live rates come from the oracle feeder in production.")
md.append("")
md.append("| Code | Currency | Denom | Symbol | Per YML |")
md.append("|------|----------|-------|--------|---------|")
md.append("| YML | %s (native) | `%s` | %s | 1 |" % (native["name"], native["denom"], native["symbol"]))
for c in curs:
    md.append("| %s | %s | `%s` | %s | %s |"
              % (c["code"], c["name"], c["denom"], c["symbol"], str(c["rate_per_yml"])))
md.append("")
io.open(DOCS, "w", encoding="utf-8", newline="\n").write("\n".join(md) + "\n")

print("currencies total: %d (incl. YML)  new to add: %d" % (len(curs) + 1, len(new)))
print("wrote: onchain.sh, ceilings.json, faucet.txt, currencies.ts, oracle-rates.txt, docs/guides/currencies.md")

# --- clients/wallet claimable denoms, grouped -----------------------------
# Forty-five currencies in one flat column is a wall, not a choice. The faucet
# page groups them the way somebody actually looks for money: the stablecoins
# and the native token first (what most testers want), then by region, because
# "which of these is my country's" is the real question and the map is how
# people hold that. Generated so the grouping cannot drift from the set.
WALLET = os.path.normpath(os.path.join(HERE, "..", "..", "clients", "wallet", "src", "claimable.ts"))
regions = data.get("regions", {})
order = ["global", "west", "central", "east", "north", "southern", "indian"]

w = []
w.append("// GENERATED from scripts/currencies/african-currencies.json -- do not hand-edit.")
w.append("")
w.append("export interface ClaimGroup {")
w.append("  id: string;")
w.append("  title: string;")
w.append("  denoms: readonly string[];")
w.append("}")
w.append("")
w.append("/** Every denom the faucet is stocked with, in one flat list. */")
w.append("export const CLAIMABLE = [")
w.append("  %s," % json.dumps(native["denom"]))
for c in curs:
    w.append("  %s," % json.dumps(c["denom"]))
w.append("] as const;")
w.append("")
w.append("export type Denom = (typeof CLAIMABLE)[number];")
w.append("")
w.append("/** The same set, grouped for the page: reserves first, then by region. */")
w.append("export const CLAIM_GROUPS: ClaimGroup[] = [")
# group 1: the native token + the two settlement stablecoins
globals_ = [c["denom"] for c in curs if c.get("region") == "global"]
w.append("  { id: 'reserve', title: 'Reserve and stablecoins', denoms: [%s] },"
         % ", ".join(json.dumps(d) for d in [native["denom"]] + globals_))
for r in order:
    if r == "global":
        continue
    members = [c["denom"] for c in curs if c.get("region") == r]
    if not members:
        continue
    w.append("  { id: %s, title: %s, denoms: [%s] },"
             % (json.dumps(r), json.dumps(regions.get(r, r)), ", ".join(json.dumps(d) for d in members)))
w.append("];")
io.open(WALLET, "w", encoding="utf-8", newline=NL).write(NL.join(w) + NL)
print("wrote clients/wallet/src/claimable.ts (%d denoms, %d groups)" % (len(curs) + 1, len(order)))
