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
# Re-runnable: register tolerates "already exists" (code 1101), mint just adds
# supply, and a pool is opened only if the denom is not already in a pool — a
# duplicate pool would split liquidity and break routing.
sh = []
sh.append("#!/bin/bash")
sh.append("# GENERATED from african-currencies.json by generate.py -- do not hand-edit.")
sh.append("set -uo pipefail")
sh.append("B=/opt/yamale/bin/blockchaind; H=/opt/yamale/node")
sh.append('K="--keyring-backend test --home $H"')
sh.append('TX="--chain-id yamale-devnet-1 $K --fees 2000uyml --yes -o json"')
sh.append('F=$($B keys show foundation -a $K)')
sh.append("code() { $B query tx \"$1\" --home $H -o json 2>/dev/null | grep -oE '\"code\":[0-9]+' | head -1; }")
sh.append('POOLS=$($B query amm pool --home $H -o json 2>/dev/null)')
sh.append('echo "=== registering + minting new currencies ==="')
for c in new:
    d, name, code, exp = c["denom"], c["name"], c["code"], c["exponent"]
    disp = code.lower()
    sh.append('$B tx stablecoin register-currency %s %s %d "%s" "%s" "%s stablecoin on Yamale" --from foundation $TX >/dev/null 2>&1; sleep 6'
              % (d, disp, exp, name, code, name))
    sh.append('h=$($B tx stablecoin mint-coin %s %d "$F" --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); sleep 6; echo "  %s: mint $(code $h)"'
              % (d, MINT, d))
sh.append('echo "=== opening YML-hub pools (skip any denom already pooled) ==="')
for c in new:
    d, rate, code = c["denom"], c["rate_per_yml"], c["code"]
    ra = DEPTH * 10**6
    # rate may be fractional (EURC ~0.92), so the currency reserve is rounded to
    # an integer base amount rather than assumed whole.
    rb = int(round(DEPTH * rate * 10**6))
    # A new currency only ever appears in its own uyml pool, so a plain grep of
    # the pool list for the denom is a sufficient "already pooled" test.
    sh.append('if echo "$POOLS" | grep -q \'"%s"\'; then echo "  uyml/%s exists, skip"; else '
              'h=$($B tx amm create-pool uyml %d %s %d 30 --from foundation $TX 2>&1 | grep -oE "[A-F0-9]{64}" | head -1); '
              'sleep 6; echo "  pool uyml/%s (1 YML = %s %s): $(code $h)"; fi'
              % (d, d, ra, d, rb, d, rate, code))
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
