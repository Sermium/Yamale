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
# Mint enough to foundation to seed the pool and feed the faucet for a long time.
MINT = 10**16

# --- onchain.sh -----------------------------------------------------------
# Re-runnable, and honest about what it did.
#
# The version this replaced went register -> mint and threw the register
# call's output at /dev/null. Both halves of that were wrong, and together
# they cost USDC and EURC: the two register calls at the end of the run never
# landed, so no application existed, the mints failed, and the script printed
# a bare status code nobody read. The faucet went on offering both for weeks.
#
# register-currency does NOT create a currency. It files an APPLICATION. The
# applicant becomes the issuer only when governance -- or a monetary authority
# scoped to the applicant's country -- approves it, and minting before that
# can only fail. So this script files the applications, then STOPS and writes
# the approval proposal, rather than minting into a wall and reporting a
# number. What it cannot do itself, it hands over in a form you can run.
sh = []
sh.append("#!/bin/bash")
sh.append("# GENERATED from african-currencies.json by generate.py -- do not hand-edit.")
sh.append("set -uo pipefail")
sh.append("B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node")
sh.append('K="--keyring-backend test --home $H"')
sh.append('TX="--chain-id yamale-devnet-2 $K --fees 2000uyml --yes -o json"')
sh.append('F=$($B keys show foundation -a $K)')
sh.append('GOV=$($B query auth module-account gov --home $H -o json 2>/dev/null | grep -oE \'"address":"[a-z0-9]+"\' | head -1 | cut -d\'"\' -f4)')
sh.append('PROPOSAL=/tmp/approve-issuers.json')
sh.append('')
sh.append('# Did it work, in words. Every tx this script sends goes through here,')
sh.append('# because a failure it cannot see is a failure it will repeat.')
sh.append('say() {')
sh.append('  if [ -z "$2" ]; then echo "  $1: NOT BROADCAST"; return 1; fi')
sh.append('  local c')
sh.append('  c=$($B query tx "$2" --home $H -o json 2>/dev/null | grep -oE \'"code":[0-9]+\' | head -1 | cut -d: -f2)')
sh.append('  if [ "$c" = "0" ]; then echo "  $1: ok"; else echo "  $1: FAILED (code ${c:-no-such-tx})"; return 1; fi')
sh.append('}')
sh.append('')
sh.append('# State, not status codes. A re-run must ask the chain what is true rather')
sh.append('# than trust its own earlier output, and "already exists" is a success here.')
sh.append('approved() { $B query stablecoin get-approved-issuer "$1" --home $H -o json >/dev/null 2>&1; }')
sh.append('applied()  { $B query stablecoin get-issuer-application "$1" --home $H -o json >/dev/null 2>&1; }')
sh.append('supply()   { $B query bank total-supply-of "$1" --home $H -o json 2>/dev/null | grep -oE \'"amount": *"[0-9]+"\' | tail -1 | grep -oE \'[0-9]+\'; }')
sh.append('')
sh.append('# denom|display|exponent|name|symbol')
# Real newlines, not the two characters backslash and n. Bash reads this
# table through a heredoc, and "\\n" inside a double-quoted bash string is
# literally a backslash followed by an n -- the whole table would arrive as
# one line and the loop would run once, on nothing.
sh.append('CURRENCIES="%s"' % NL.join(
    "%s|%s|%d|%s|%s" % (c["denom"], c["code"].lower(), c["exponent"], c["name"], c["code"])
    for c in new))
sh.append('')
sh.append('echo "=== 1/4  filing currency applications ==="')
sh.append('UNAPPROVED=""')
sh.append('while IFS="|" read -r d disp exp name sym; do')
sh.append('  [ -z "$d" ] && continue')
sh.append('  if approved "$d"; then echo "  $d: issuer already approved"; continue; fi')
sh.append('  if applied "$d"; then')
sh.append('    echo "  $d: application already on file, awaiting approval"')
sh.append('  else')
sh.append('    h=$($B tx stablecoin register-currency "$d" "$disp" "$exp" "$name" "$sym" "$name stablecoin on Yamale" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1)')
sh.append('    sleep 6')
sh.append('    say "$d register" "$h" || true')
sh.append('    applied "$d" || echo "  $d: NO APPLICATION ON FILE -- the register above did not take"')
sh.append('  fi')
sh.append('  UNAPPROVED="$UNAPPROVED $d"')
sh.append('done <<EOF')
sh.append('$CURRENCIES')
sh.append('EOF')
sh.append('')
sh.append('if [ -n "$UNAPPROVED" ]; then')
sh.append('  echo')
sh.append('  echo "=== 2/4  governance approval required -- stopping here ==="')
sh.append('  echo "An application is not a currency. Minting needs an approved issuer,"')
sh.append('  echo "and only governance or a scoped monetary authority can admit one."')
sh.append('  echo "Not approved yet:$UNAPPROVED"')
sh.append('  {')
sh.append('    echo "{"')
sh.append('    echo \'  "messages": [\'')
sh.append('    first=1')
sh.append('    for d in $UNAPPROVED; do')
# printf %s does not interpret escapes in its ARGUMENT, so a separator held
# in a variable as a backslash and an n prints as those two characters and
# the proposal is not JSON. The escape has to live in the FORMAT string.
sh.append('      [ $first -eq 1 ] || printf ",\\\\n"')
sh.append('      first=0')
sh.append('      printf "    {\\\\n"')
sh.append('      echo \'      "@type": "/blockchain.stablecoin.v1.MsgApproveIssuer",\'')
sh.append('      echo "      \\"authority\\": \\"$GOV\\","')
sh.append('      echo "      \\"denom\\": \\"$d\\","')
sh.append('      echo \'      "approve": true\'')
sh.append('      printf "    }"')
sh.append('    done')
sh.append('    printf "\\\\n  ],\\\\n"')
sh.append('    echo \'  "metadata": "",\'')
sh.append('    echo \'  "deposit": "10000000uyml",\'')
sh.append('    echo \'  "title": "Admit the foundation as issuer of new currencies",\'')
sh.append('    echo "  \\"summary\\": \\"Admits the foundation, already the issuer of the currencies this chain carries, as the issuer of:$UNAPPROVED. These are devnet test currencies with no reserve behind them.\\""')
sh.append('    echo "}"')
sh.append('  } > $PROPOSAL')
sh.append('  echo')
sh.append('  echo "Proposal written to $PROPOSAL. Submit it, let it pass, then re-run this script:"')
sh.append('  echo "  $B tx gov submit-proposal $PROPOSAL --from foundation $TX --gas 400000"')
sh.append('  exit 1')
sh.append('fi')
sh.append('')
sh.append('echo "=== 3/4  minting to the foundation ==="')
sh.append('while IFS="|" read -r d disp exp name sym; do')
sh.append('  [ -z "$d" ] && continue')
sh.append('  have=$(supply "$d")')
sh.append('  # Minting is not idempotent, so a second run must not silently double the')
sh.append('  # supply of every currency on the chain.')
sh.append('  if [ -n "$have" ] && [ "$have" != "0" ]; then echo "  $d: supply $have, skip"; continue; fi')
sh.append('  h=$($B tx stablecoin mint-coin "$d" %d "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1)' % MINT)
sh.append('  sleep 6')
sh.append('  say "$d mint" "$h" || true')
sh.append('done <<EOF')
sh.append('$CURRENCIES')
sh.append('EOF')
sh.append('')
sh.append('echo "=== 4/4  opening YML-hub pools (skip any denom already pooled) ==="')
sh.append('POOLS=$($B query amm pool --home $H -o json 2>/dev/null)')
for c in new:
    d, rate, ccode = c["denom"], c["rate_per_yml"], c["code"]
    ra = DEPTH * 10**6
    # rate may be fractional (EURC ~0.92), so the currency reserve is rounded
    # to an integer base amount rather than assumed whole.
    rb = int(round(DEPTH * rate * 10**6))
    # A new currency only ever appears in its own uyml pool, so a plain grep
    # of the pool list for the denom is a sufficient "already pooled" test.
    sh.append('if echo "$POOLS" | grep -q \'"%s"\'; then echo "  uyml/%s exists, skip"; else h=$($B tx amm create-pool uyml %d %s %d 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; say "pool uyml/%s (1 YML = %s %s)" "$h" || true; fi'
              % (d, d, ra, d, rb, d, rate, ccode))
sh.append('echo "=== done ==="')
io.open(os.path.join(HERE, "onchain.sh"), "w", encoding="utf-8", newline="\n").write("\n".join(sh) + "\n")

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
print("wrote: onchain.sh, faucet.txt, currencies.ts, oracle-rates.txt, docs/guides/currencies.md")

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
