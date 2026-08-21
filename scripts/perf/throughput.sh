#!/bin/bash
# The machine's ceiling, measured the only way this chain permits.
#
# Established by the previous run: the ante handler rejects any transaction
# whose sequence is not the account's current one, so a single account can put
# exactly one transaction in a block no matter how fast it signs. Throughput is
# therefore a function of how many distinct accounts are paying at once — which
# is also how a real payments network behaves, so the number means something.
#
# One account per payer, one pre-signed transaction each, all broadcast at once.
set -uo pipefail

B=/opt/yamale/bin/blockchaind
H=/opt/yamale/node
K="--keyring-backend test --home $H"
NODE=http://127.0.0.1:26657
N=${1:-40}

F=$($B keys show foundation -a $K)
addrs=()
echo "  creating $N payer accounts..."
for i in $(seq 1 $N); do
  $B keys delete "ld$i" -y $K >/dev/null 2>&1
  $B keys add "ld$i" $K >/dev/null 2>&1
  addrs+=("$($B keys show "ld$i" -a $K)")
done

echo "  funding them in one transaction..."
h=$($B tx bank multi-send foundation "${addrs[@]}" 3000000uyml --from foundation \
      --chain-id yamale-devnet-2 $K --fees 20000uyml --gas 20000000 --yes -o json 2>&1 \
      | grep -oE '[A-F0-9]{64}' | head -1)
sleep 12
code=$($B query tx "$h" --home $H -o json 2>/dev/null | grep -oE '"code":[0-9]+' | head -1)
echo "  funding tx: ${code:-not found}"

echo "  pre-signing one transaction per payer..."
rm -rf /tmp/ld && mkdir -p /tmp/ld
ready=0
for i in $(seq 1 $N); do
  a="${addrs[$((i-1))]}"
  info=$(curl -s "http://127.0.0.1:1317/cosmos/auth/v1beta1/accounts/$a")
  num=$(echo "$info" | grep -oE '"account_number":"[0-9]+"' | grep -oE '[0-9]+' | head -1)
  [ -z "$num" ] && continue
  $B tx bank send "$a" "$F" 1uyml --from "ld$i" --chain-id yamale-devnet-2 $K \
     --fees 500uyml --gas 100000 --account-number "$num" --sequence 0 \
     --generate-only -o json > /tmp/ld/r$i.json 2>/dev/null
  $B tx sign /tmp/ld/r$i.json --from "ld$i" --chain-id yamale-devnet-2 $K \
     --account-number "$num" --sequence 0 \
     --output-document /tmp/ld/s$i.json >/dev/null 2>&1
  $B tx encode /tmp/ld/s$i.json > /tmp/ld/e$i.txt 2>/dev/null
  [ -s /tmp/ld/e$i.txt ] && ready=$((ready+1))
done
echo "  ready to fire: $ready/$N"
[ "$ready" -lt 5 ] && { echo "  ABORT: too few signed"; exit 1; }

start_h=$(curl -s "$NODE/status" | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')
t0=$(date +%s.%N)
for i in $(seq 1 $N); do
  [ -s /tmp/ld/e$i.txt ] || continue
  tx=$(cat /tmp/ld/e$i.txt)
  curl -s -X POST "$NODE" -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$i,\"method\":\"broadcast_tx_async\",\"params\":{\"tx\":\"$tx\"}}" >/dev/null 2>&1 &
done
wait
t1=$(date +%s.%N)
echo "  all $ready broadcast in $(echo "scale=2; $t1 - $t0" | bc)s"

sleep 22
end_h=$(curl -s "$NODE/status" | grep -oE '"latest_block_height":"[0-9]+"' | head -1 | grep -oE '[0-9]+')

included=0; peak=0
for h in $(seq "$start_h" "$end_h"); do
  n=$(curl -s "$NODE/block?height=$h" | python3 -c "
import json,sys
try: print(len(json.load(sys.stdin)['result']['block']['data']['txs'] or []))
except Exception: print(0)
")
  included=$((included + n))
  [ "$n" -gt "$peak" ] && peak=$n
  [ "$n" -gt 0 ] && echo "    height $h: $n txs"
done

echo
echo "  included  : $included of $ready"
echo "  peak block: $peak txs"
[ "$peak" -gt 0 ] && echo "  peak rate : $(echo "scale=1; $peak / 5.03" | bc) tx/s"

for i in $(seq 1 $N); do $B keys delete "ld$i" -y $K >/dev/null 2>&1; done
rm -rf /tmp/ld
echo "=== done ==="
