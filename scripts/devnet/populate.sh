#!/bin/bash
# Put real data on the devnet so the interfaces have something to be wrong
# about. Every one of these is a transaction the chain actually accepted --
# nothing here is seeded into genesis after the fact.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
H=/opt/yamale/node
CHAIN=yamale-devnet-1
TX="--chain-id $CHAIN --keyring-backend test --home $H --fees 500uyml --yes --output json"
Q="--home $H --output json"

addr() { $BIN keys show "$1" -a --keyring-backend test --home "$H"; }

ALICE=$(addr alice); BOB=$(addr bob); FOUNDATION=$(addr foundation)

# One transaction per block, because two from the same key in one block collide
# on the account sequence and the second is silently dropped.
send() {
  local out; out=$($BIN tx "$@" $TX 2>&1)
  local code; code=$(echo "$out" | grep -o '"code":[0-9]*' | head -1 | cut -d: -f2)
  echo "  code=${code:-?}  $(echo "$out" | grep -o '"txhash":"[A-F0-9]*"' | head -1 | cut -d'"' -f4 | cut -c1-16)"
  sleep 6
}

echo "=== bot accounts ==="
for k in bot-a bot-b; do
  $BIN keys add "$k" --keyring-backend test --home "$H" >/dev/null 2>&1 || true
  echo "  $k  $(addr $k)"
done
BOT_A=$(addr bot-a); BOT_B=$(addr bot-b)

echo "=== funding the bots from alice ==="
send bank send alice "$BOT_A" 5000000000uyml
send bank send alice "$BOT_B" 5000000000uyml

echo "=== minting currencies (foundation is the approved issuer for all 42) ==="
# A spread across regions so the wallet shows more than one row and the
# explorer's supply page has something other than YML in it.
for pair in ungn:250000000000 uzar:40000000000 ukes:180000000000 ughs:12000000000 uxof:900000000000; do
  denom=${pair%%:*}; amount=${pair##*:}
  echo "  mint $denom to alice"
  send stablecoin mint-coin "$denom" "$amount" "$ALICE" --from foundation
done

echo "=== spreading some to bob so transfers have two sides ==="
send bank send alice "$BOB" 50000000000ungn
send bank send alice "$BOB" 8000000000uzar

echo "=== a treasury, so the Safe app has a subject ==="
send treasury create-treasury "Yamale Foundation Operations" --from foundation

# Pools belong here, not in a separate step somebody has to remember.
#
# They were a manual follow-up for three resets running, and the activity bot
# broke with "pool 1 not found" twice because of it — a failure that looks like
# a chain fault and is actually a missing setup step.
echo "=== AMM pools, so swaps have somewhere to go ==="
send amm create-pool uyml 20000000000 ungn 30000000000 30 --from alice
send amm create-pool uyml 10000000000 uzar 15000000000 30 --from alice

# And the faucet's own float. The faucet hands out currencies it holds, so a
# fresh chain leaves it able to send YML and nothing else.
echo "=== stocking the faucet ==="
for pair in ungn:500000000000 ukes:400000000000 uzar:100000000000 uxof:800000000000 uxaf:800000000000; do
  denom=${pair%%:*}; amount=${pair##*:}
  echo "  mint $denom to the faucet key"
  send stablecoin mint-coin "$denom" "$amount" "$FOUNDATION" --from foundation
done

echo
echo "=== RESULT ==="
echo "alice balances:"; $BIN query bank balances "$ALICE" $Q 2>/dev/null | grep -o '"denom":"[a-z]*","amount":"[0-9]*"' | head -10
echo "bob balances:";   $BIN query bank balances "$BOB" $Q 2>/dev/null | grep -o '"denom":"[a-z]*","amount":"[0-9]*"' | head -10
echo "treasuries:";     $BIN query treasury list-treasury $Q 2>/dev/null | head -c 300
echo
echo "addresses:"
echo "  alice      $ALICE"
echo "  bob        $BOB"
echo "  foundation $FOUNDATION"
echo "  bot-a      $BOT_A"
echo "  bot-b      $BOT_B"
