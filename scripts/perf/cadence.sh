#!/bin/bash
# What this VM actually delivers, measured rather than quoted.
#
# Three separate questions, because they have three different answers:
#   1. How fast does it produce blocks when nothing is happening?
#   2. What does consensus allow into a block?
#   3. What happens when transactions arrive faster than blocks?
#
# The third is the only one that is a throughput number. The first two bound it.
set -uo pipefail

B=/opt/yamale/bin/blockchaind
H=/opt/yamale/node
K="--keyring-backend test --home $H"
NODE=http://127.0.0.1:26657
TX="--chain-id yamale-devnet-2 $K --fees 500uyml --yes --broadcast-mode sync -o json"

echo "=== machine ==="
echo "  cores : $(nproc)"
echo "  memory: $(free -m | awk '/Mem:/{print $2" MB total, "$7" MB available"}')"
echo "  disk  : $(df -h /opt | awk 'NR==2{print $2" total, "$4" free"}')"

echo
echo "=== 1. block cadence, idle ==="
h0=$($B status --home $H 2>/dev/null | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')
t0=$(date +%s.%N)
sleep 60
h1=$($B status --home $H 2>/dev/null | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')
t1=$(date +%s.%N)
blocks=$((h1 - h0))
elapsed=$(echo "$t1 - $t0" | bc)
echo "  $blocks blocks in ${elapsed}s"
echo "  block time: $(echo "scale=2; $elapsed / $blocks" | bc)s"
echo "  blocks/day: $(echo "scale=0; 86400 * $blocks / $elapsed" | bc)"

echo
echo "=== 2. what consensus allows ==="
curl -s "$NODE/consensus_params" | python3 -c "
import json,sys
d=json.load(sys.stdin)['result']['consensus_params']['block']
print('  max_gas  :', d['max_gas'])
print('  max_bytes:', d['max_bytes'])
g=int(d['max_gas'])
if g > 0:
    print('  a 100k-gas transfer fits', g//100000, 'times per block')
else:
    print('  max_gas is -1: unlimited by consensus, bounded by max_bytes and CPU')
"

echo
echo "=== 3. throughput under load ==="
# Several accounts, because one account is capped by its own sequence: two
# transactions from the same signer in one block is a sequence mismatch, so a
# single-account test measures the sequence rule, not the machine.
ACCOUNTS=8
PER=6
echo "  preparing $ACCOUNTS senders..."
F=$($B keys show foundation -a $K)
addrs=()
for i in $(seq 1 $ACCOUNTS); do
  $B keys delete "perf$i" -y $K >/dev/null 2>&1
  $B keys add "perf$i" $K >/dev/null 2>&1
  addrs+=("$($B keys show "perf$i" -a $K)")
done

# One transaction funds them all, so preparation costs one block rather than eight.
multi=""
for a in "${addrs[@]}"; do multi="$multi $a 2000000uyml"; done
$B tx bank multi-send foundation ${multi} --from foundation $TX >/dev/null 2>&1
sleep 12

funded=0
for a in "${addrs[@]}"; do
  bal=$($B query bank balances "$a" --home $H -o json 2>/dev/null | grep -oE '"amount":"[0-9]+"' | head -1 | grep -oE '[0-9]+')
  [ -n "$bal" ] && [ "$bal" != "0" ] && funded=$((funded+1))
done
echo "  funded: $funded/$ACCOUNTS"
[ "$funded" -lt 2 ] && { echo "  ABORT: senders unfunded, no throughput number is meaningful"; exit 1; }

start_h=$($B status --home $H 2>/dev/null | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')
started=$(date +%s.%N)

# Fire everything at once: each account sends sequentially in its own
# background shell, so the accounts race each other and nothing races itself.
for i in $(seq 1 $ACCOUNTS); do
  (
    for j in $(seq 1 $PER); do
      $B tx bank send "perf$i" "$F" 1uyml --from "perf$i" $TX >/dev/null 2>&1
    done
  ) &
done
wait
finished=$(date +%s.%N)
sleep 14
end_h=$($B status --home $H 2>/dev/null | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')

sent=$((ACCOUNTS * PER))
echo "  submitted: $sent transactions in $(echo "scale=1; $finished - $started" | bc)s"

# Count what actually landed, block by block. Broadcast is not inclusion.
included=0
for h in $(seq "$start_h" "$end_h"); do
  n=$(curl -s "$NODE/block?height=$h" | python3 -c "
import json,sys
try: print(len(json.load(sys.stdin)['result']['block']['data']['txs'] or []))
except Exception: print(0)
")
  included=$((included + n))
  [ "$n" -gt 0 ] && echo "    height $h: $n txs"
done

span=$((end_h - start_h))
echo
echo "  included : $included txs across $span blocks"
if [ "$included" -gt 0 ] && [ "$span" -gt 0 ]; then
  echo "  peak/block: $(for h in $(seq $start_h $end_h); do curl -s "$NODE/block?height=$h" | python3 -c "
import json,sys
try: print(len(json.load(sys.stdin)['result']['block']['data']['txs'] or []))
except Exception: print(0)
"; done | sort -rn | head -1) txs"
  wall=$(echo "$finished - $started" | bc)
  echo "  sustained: $(echo "scale=1; $included / $wall" | bc) tx/s over the submission window"
fi

for i in $(seq 1 $ACCOUNTS); do $B keys delete "perf$i" -y $K >/dev/null 2>&1; done
echo
echo "=== done ==="
