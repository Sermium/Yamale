#!/bin/bash
# Put something on the secondary market, so the screen that renders it has been
# seen with data in it.
#
# `x/market` applied at height 378,800 and had never been exercised: no listing,
# no auction, no order had ever been placed on any chain. An empty book renders
# correctly no matter what is broken behind it, which is the same reason the
# explorer had to be verified against a populated chain rather than an empty one.
#
# # The money problem this solves first
#
# Every one of the timber holders had a balance of shares and NOT ONE DOLLAR.
# The whole 15,000,000 USD of the distribution sat in the tokenisation module
# account, which is exactly where it should sit — it is owed to holders and has
# not been claimed. So nobody could buy anything, and seeding a market meant
# first making somebody able to pay.
#
# The answer is not to mint: it is to have a holder CLAIM what they are already
# owed. That is the real path a buyer's money takes on this chain, it exercises
# the entitlement the pro-rata migration preserved, and it leaves the vault's
# accounting right. A seeder that minted dollars into a test account would have
# proved the market works with money that could not have got there.
#
# Idempotent in the sense that matters: re-running it adds more offers rather
# than corrupting the ones already there. Nothing here cancels or overwrites.
#
# Run it ON the node. It reports the chain's own words on any failure and stops.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657
CHAIN=yamale-devnet-2
TX="--keyring-backend test --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 --fees 6000uyml -y -o json"
Q="--home $HOME_DIR --node $NODE -o json"

ASSET=4
DENOM=tok/4/SRMT
SELLER=srm-holding
BUYER=tim-familyoffice
UNDERBIDDER=tim-diaspora

addr() { "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null; }

# Broadcast, then find out what the chain actually did with it. The CLI prints
# `gas estimate: N` before the JSON under --gas auto, so the output as a whole
# is not a JSON document; the last line that is one carries the hash.
tx() {
  local what=$1; shift
  local out hash code log res i
  out=$("$BIN" tx "$@" $TX 2>&1)
  hash=$(printf '%s' "$out" | python3 -c "
import sys, json
h = ''
for line in sys.stdin.read().splitlines():
    line = line.strip()
    if not line.startswith('{'): continue
    try: h = json.loads(line).get('txhash', '') or h
    except Exception: pass
print(h)
" 2>/dev/null)
  if [ -z "$hash" ]; then
    printf '  FAIL  %-36s not broadcast\n' "$what"
    printf '%s\n' "$out" | tail -c 400 | sed 's/^/        /'
    return 1
  fi
  for i in $(seq 1 20); do
    sleep 3
    res=$("$BIN" query tx "$hash" $Q 2>/dev/null) || continue
    [ -n "$res" ] || continue
    code=$(printf '%s' "$res" | python3 -c "
import sys, json
try: print(json.loads(sys.stdin.read()).get('code', 1))
except Exception: print(1)")
    if [ "$code" = "0" ]; then
      printf '  ok    %-36s %s\n' "$what" "${hash:0:12}"
      return 0
    fi
    log=$(printf '%s' "$res" | python3 -c "
import sys, json
try: print(json.loads(sys.stdin.read()).get('raw_log',''))
except Exception: print('')")
    printf '  FAIL  %-36s code %s\n' "$what" "$code"
    printf '%s\n' "$log" | cut -c1-300 | sed 's/^/        /'
    return 1
  done
  printf '  FAIL  %-36s never included\n' "$what"
  return 1
}

balance() {
  "$BIN" query bank balance "$(addr "$1")" "$2" $Q 2>/dev/null \
    | python3 -c "import sys,json;print(json.load(sys.stdin)['balance']['amount'])" 2>/dev/null || echo 0
}

echo "=== 0. gas, which is not the point but is a precondition ==="
# The timber holders were seeded with shares and nothing else, so they cannot
# pay a fee to claim the money they are owed. This is a devnet detail and not a
# statement about the mechanism: on a chain with a faucet or a fee grant it does
# not arise. Sent from alice because she is the account this devnet funds from.
for who in "$BUYER" "$UNDERBIDDER"; do
  have=$(balance "$who" uyml)
  if [ "${have:-0}" -ge 100000 ]; then
    printf '  --    %-36s has %s uyml\n' "$who" "$have"
  else
    tx "fund $who for fees" bank send alice "$(addr "$who")" 5000000uyml || exit 1
  fi
done

echo
echo "=== 1. money, in two steps, because a dividend is not a dollar ==="
# Not a mint. These two are owed this out of the vault, and claiming is how a
# shareholder's money reaches their account on this chain.
#
# But a claim pays DIVIDEND TOKENS -- div/4/SRMT -- and not dollars. That is the
# design rather than an inconvenience: the dividend is its own denomination,
# backed one-for-one by a reserve the vehicle sets aside, and it is exchanged
# for the currency behind it at the treasury. A seeder that stopped after the
# claim would have proved a market no buyer could actually reach, because the
# book settles in uusd and a claim gives you none of it.
DIVDENOM="div/$ASSET/SRMT"
for who in "$BUYER" "$UNDERBIDDER"; do
  if [ "$(balance "$who" "$DIVDENOM")" = "0" ] && [ "$(balance "$who" uusd)" = "0" ]; then
    tx "$who claims their entitlement" tokenisation claim "$ASSET" --from "$who" || exit 1
  fi
  div=$(balance "$who" "$DIVDENOM")
  if [ "${div:-0}" -gt 0 ]; then
    # A bare count, not a coin string. The dividend denomination is implied by
    # the asset id, so naming it again is a parse error rather than a redundancy.
    tx "$who exchanges dividends for dollars" \
      tokenisation exchange-dividend "$ASSET" "$div" --from "$who" || exit 1
  fi
done
printf '  %s holds %s uusd\n' "$BUYER" "$(balance "$BUYER" uusd)"
printf '  %s holds %s uusd\n' "$UNDERBIDDER" "$(balance "$UNDERBIDDER" uusd)"

echo "=== 2. a fixed-price listing, and somebody taking it ==="
# 100,000 shares for $60,000, which is $0.60 a share — the price the vehicle was
# issued at. The unit price the chain stores is 600000, because uusd carries six
# decimals and a share carries none. Getting that factor wrong in either
# direction is the whole risk on this surface.
tx "list 100,000 shares for \$60,000" market list "$DENOM" 100000 60000000000uusd \
  --from "$SELLER" || exit 1

LISTING=$("$BIN" query market listings "$DENOM" $Q 2>/dev/null | python3 -c "
import sys, json
ls = json.load(sys.stdin).get('listings', [])
live = [l for l in ls if l.get('state') == 'LISTING_STATE_LIVE']
print(live[-1]['id'] if live else '')" 2>/dev/null)
[ -n "$LISTING" ] || { echo "  FAIL  no live listing to buy"; exit 1; }

tx "$BUYER takes listing $LISTING" market buy "$LISTING" --from "$BUYER" || exit 1

echo
echo "=== 3. an auction, with a bid under way ==="
# Two hours: comfortably inside the chain's own bounds (1h to 30d) and short
# enough that somebody looking at the screen sees the countdown move.
CLOSES=$(( $(date -u +%s) + 7200 ))
tx "auction 50,000 shares, reserve \$25,000" market open-auction "$DENOM" 50000 \
  25000000000uusd "$CLOSES" --from "$SELLER" || exit 1

AUCTION=$("$BIN" query market auctions "$DENOM" $Q 2>/dev/null | python3 -c "
import sys, json
a = [x for x in json.load(sys.stdin).get('auctions', []) if x.get('state') == 'AUCTION_STATE_OPEN']
print(a[-1]['id'] if a else '')" 2>/dev/null)
[ -n "$AUCTION" ] || { echo "  FAIL  no open auction to bid on"; exit 1; }

tx "$UNDERBIDDER bids \$26,000" market bid "$AUCTION" 26000000000uusd \
  --from "$UNDERBIDDER" || exit 1

echo
echo "=== 4. a two-sided book ==="
# An ask above and a bid below, deliberately not crossing. A book that crossed
# would fill instantly and leave nothing resting to look at — and the screen
# that matters most here is the one showing depth on both sides.
tx "ask 200,000 shares at \$0.65" market place-order "$DENOM" ask 200000 650000 \
  --from "$SELLER" || exit 1
tx "bid 100,000 shares at \$0.55" market place-order "$DENOM" bid 100000 550000 \
  --from "$BUYER" || exit 1

echo
echo "=== what the chain holds now ==="
"$BIN" query market book "$DENOM" $Q 2>/dev/null | python3 -c "
import sys, json
b = json.load(sys.stdin)
print('  book settles in', b.get('settlement_denom') or '(unknown)')
print('  bids:', len(b.get('bids', [])), ' asks:', len(b.get('asks', [])))
"
"$BIN" query market listings "$DENOM" $Q 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin).get('listings', []):
    print('  listing', l['id'], l['state'], l['quantity'], 'shares for', l['price']['amount'], l['price']['denom'])
"
"$BIN" query market auctions "$DENOM" $Q 2>/dev/null | python3 -c "
import sys, json
for a in json.load(sys.stdin).get('auctions', []):
    bid = a.get('best_bid', {}).get('amount', '0')
    print('  auction', a['id'], a['state'], a['quantity'], 'shares, best bid', bid)
"

echo
echo "=== the cost basis the buyer now has, which is the point of all this ==="
# Before the market existed the register knew what everybody held and nothing
# about what any of it cost. This is the first row on this chain where it knows.
"$BIN" query tokenisation position "$ASSET" "$(addr "$BUYER")" $Q 2>/dev/null | python3 -c "
import sys, json
p = json.load(sys.stdin).get('position', {})
print('  holder     ', p.get('holder'))
print('  cost_basis ', p.get('cost_basis'), '(uusd, base units)')
print('  cost_qty   ', p.get('cost_qty'), '(shares that cost covers)')
"
