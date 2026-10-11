#!/usr/bin/env bash
# standalone-bench.sh - what one host can do as a validator on its own.
#
# The live chain cannot answer that: its throughput is held to what the two
# small validators (a Raspberry Pi and a 1-core VM) can keep up with. So this
# forks a COPY of the live state into a private single-validator chain on the
# same host, floods it, and reads the blocks back. The live chain is never
# touched beyond being peered with once, to copy its state.
#
# Run ON the host, from the directory holding chain-load.mjs and its
# node_modules (yamale-us: /opt/yamale/perf):
#
#   sudo BIN=/path/to/blockchaind-bench bash standalone-bench.sh setup
#   sudo bash standalone-bench.sh load <clients> <accountsPerClient> <txsPerAccount>
#   sudo bash standalone-bench.sh teardown
#
# BIN must run the SAME state machine as the live chain, plus a working
# in-place-testnet (fixed in 011ba04; before that it panicked on every run).
#
# Isolation: its own user, its own home (/opt/yamale/bench), loopback-only ports
# 36656-36658, 31317, 39090, and chain-id yamale-bench-1, which no live node
# will talk to. It runs at CPUWeight 20 against the live validator's 1000, with
# a 4 GB memory cap, so under contention the live chain wins; the load clients
# run at nice 19. The live chain's block times over the run are reported, so
# that claim is checked rather than asserted.
set -euo pipefail
H=/opt/yamale/bench
U=yamale-bench
LIVE_RPC=http://127.0.0.1:26657
RPC=http://127.0.0.1:36657
REST=http://127.0.0.1:31317
CHAIN=yamale-bench-1
UNIT=yamale-bench
here=$(pwd)

height() { curl -s -m 5 "$1/status" | python3 -c 'import sys,json; print(json.load(sys.stdin)["result"]["sync_info"]["latest_block_height"])'; }
running() { systemctl is-active --quiet "$UNIT"; }
start_node() {  # start_node <args...>
  systemd-run --unit="$UNIT" --uid="$U" --gid="$U" -p CPUWeight=20 -p MemoryMax=4G \
    -p LimitNOFILE=65535 --collect /opt/yamale/bench/blockchaind "$@" --home "$H" >/dev/null
}
toml() {  # toml <file> <section or -> <key> <value>
  python3 - "$@" <<'PY'
import re, sys
path, section, key, value = sys.argv[1:5]
lines = open(path).read().split("\n")
cur = None; hits = 0
for i, ln in enumerate(lines):
    m = re.match(r"^\[([^\]]+)\]", ln)
    if m: cur = m.group(1); continue
    if re.match(r"^%s\s*=" % re.escape(key), ln) and (section == "-" or cur == section):
        lines[i] = "%s = %s" % (key, value); hits += 1
assert hits == 1, "%s [%s]: %d matches" % (key, section, hits)
open(path, "w").write("\n".join(lines))
PY
}

case "${1:-}" in
setup)
  : "${BIN:?BIN=path to the bench binary}"
  running && { echo "a bench node is already running; teardown first"; exit 1; }
  [ -e "$H" ] && { echo "$H exists; teardown first"; exit 1; }
  id "$U" >/dev/null 2>&1 || useradd --system --home-dir "$H" --no-create-home --shell /usr/sbin/nologin "$U"
  install -d -o "$U" -g "$U" -m 700 "$H"
  install -o root -g root -m 755 "$BIN" "$H/blockchaind"
  sudo -u "$U" "$H/blockchaind" init bench --chain-id yamale-devnet-2 --home "$H" >/dev/null 2>&1
  install -o "$U" -g "$U" -m 644 /opt/yamale/node/config/genesis.json "$H/config/genesis.json"

  C=$H/config/config.toml; A=$H/config/app.toml
  toml "$C" rpc laddr '"tcp://127.0.0.1:36657"'
  toml "$C" p2p laddr '"tcp://127.0.0.1:36656"'
  toml "$C" - proxy_app '"tcp://127.0.0.1:36658"'
  toml "$C" rpc pprof_laddr '""'
  toml "$C" instrumentation prometheus false
  toml "$C" p2p addr_book_strict false
  toml "$C" p2p allow_duplicate_ip true
  toml "$C" p2p pex false
  # The live master, to state-sync from. It is the only peer this node ever has.
  MASTER=$(sudo -u yamale-chain /opt/yamale/bin/blockchaind comet show-node-id --home /opt/yamale/node)
  toml "$C" p2p persistent_peers "\"$MASTER@127.0.0.1:26656\""
  # The same 5 s block cadence as the live chain, so the numbers compare.
  toml "$C" consensus timeout_commit '"5s"'
  toml "$C" mempool size 100000
  toml "$C" mempool max_txs_bytes 1073741824
  SNAP=$(ls /opt/yamale/node/data/snapshots | grep -E '^[0-9]+$' | sort -n | tail -1)
  HASH=$(curl -s "$LIVE_RPC/commit?height=$SNAP" | python3 -c 'import sys,json; print(json.load(sys.stdin)["result"]["signed_header"]["commit"]["block_id"]["hash"])')
  toml "$C" statesync enable true
  toml "$C" statesync rpc_servers "\"$LIVE_RPC,$LIVE_RPC\""
  toml "$C" statesync trust_height "$SNAP"
  toml "$C" statesync trust_hash "\"$HASH\""
  # The live chain's: a fresh init leaves it empty and the node will not start.
  toml "$A" - minimum-gas-prices '"0uyml"'
  toml "$A" api enable true
  toml "$A" api address '"tcp://127.0.0.1:31317"'
  toml "$A" grpc address '"127.0.0.1:39090"'
  toml "$A" state-sync snapshot-interval 0
  toml "$A" - iavl-cache-size 200000

  echo "state-syncing a copy of the live chain from its snapshot at $SNAP"
  start_node start
  for i in $(seq 1 60); do
    sleep 3
    s=$(curl -s -m 3 "$RPC/status" || true)
    echo "$s" | grep -q '"catching_up":false' && [ "$(echo "$s" | grep -oE '"latest_block_height":"[0-9]+"' | grep -oE '[0-9]+')" -gt "$SNAP" ] && break
  done
  echo "copy at height $(height $RPC)"
  systemctl stop "$UNIT"; sleep 2

  # Fork it: a new validator, its own chain-id, and NO peer connections at all.
  #
  # The chain-id alone does not isolate it. in-place-testnet rewrites the chain
  # id in the app state and the block headers, but CometBFT keeps the genesis it
  # first started with in its own database, so the node still ANNOUNCES the live
  # chain's name in the p2p handshake and a live node would pair with it.
  # Transactions are safe either way (they are signed for yamale-bench-1); the
  # connection is what is refused, by allowing none: a single validator needs
  # no peers.
  toml "$C" p2p persistent_peers '""'
  toml "$C" p2p max_num_inbound_peers 0
  toml "$C" p2p max_num_outbound_peers 0
  toml "$C" statesync enable false
  sudo -u "$U" "$H/blockchaind" keys add benchval --keyring-backend test --home "$H" >/dev/null 2>&1
  ACC=$(sudo -u "$U" "$H/blockchaind" keys show benchval -a --keyring-backend test --home "$H")
  VALOPER=$("$H/blockchaind" debug addr "$ACC" 2>&1 | awk '/Bech32 Val/ {print $NF}')
  # The foundation, to fund the load accounts in the copy (it has its live balance there).
  install -d -o "$U" -g "$U" -m 700 "$H/keyring-test"
  for f in /opt/yamale/faucet/keyring-test/*; do install -o "$U" -g "$U" -m 600 "$f" "$H/keyring-test/"; done
  echo "forking as $CHAIN with validator $VALOPER"
  # The new validator's consensus key is this node's own priv_validator_key: in
  # this SDK the command reads it from the home (--validator-privkey, in the
  # command's example text, is not a flag it has).
  start_node in-place-testnet "$CHAIN" "$VALOPER" --skip-confirmation
  for i in $(seq 1 40); do
    sleep 3
    h=$(height $RPC 2>/dev/null || true)
    [ -n "$h" ] && curl -s -m 3 "$RPC/block" | grep -q "\"chain_id\":\"$CHAIN\"" && break
  done
  echo "bench chain $CHAIN producing at height $(height $RPC)"
  ;;

load)
  clients=${2:?clients}; per_client=${3:?accounts per client}; per_account=${4:?txs per account}
  running || { echo "no bench node; run setup"; exit 1; }
  cd "$here"
  export RPC REST CHAIN_ID=$CHAIN
  for c in $(seq 1 "$clients"); do
    ACCOUNTS=bench-$c.json node chain-load.mjs keys "$per_client" > "bench-$c.addrs"
  done
  # Fund every load account from the foundation, 100 recipients a transaction.
  fee_each=$(( (per_account + 5) * 2000 ))
  cat bench-*.addrs | tr ' ' '\n' | grep . | xargs -n 100 | while read -r batch; do
    h=$(sudo -u "$U" "$H/blockchaind" tx bank multi-send foundation $batch "${fee_each}uyml" \
        --keyring-backend test --home "$H" --chain-id "$CHAIN" --node "$RPC" \
        --fees 0uyml --gas 4000000 --yes -o json 2>&1 | grep -oE '[A-F0-9]{64}' | head -1)
    sleep 7
    code=$("$H/blockchaind" q tx "$h" --node "$RPC" -o json 2>/dev/null | grep -oE '"code": *[0-9]+' | head -1)
    echo "  funded 100 accounts: $code"
  done
  live0=$(height $LIVE_RPC)
  start_at=$(( $(date +%s%3N) + 600000 ))  # generous: every client must finish signing first
  pids=()
  for c in $(seq 1 "$clients"); do
    ACCOUNTS=bench-$c.json NO_MEASURE=1 START_AT=$start_at nice -n 19 \
      node chain-load.mjs run "$per_account" 50 > "bench-$c.log" 2>&1 &
    pids+=($!)
  done
  # Release the flood as soon as every client has signed, not after the cap.
  until [ "$(grep -l '^signed ' bench-*.log 2>/dev/null | wc -l)" -ge "$clients" ]; do sleep 2; done
  echo "all clients signed; flooding"
  wait "${pids[@]}"
  cat bench-*.log | grep -E '^(signed|broadcast|flood)'
  h0=$(grep -h 'flood started at height' bench-*.log | grep -oE '[0-9]+' | sort -n | head -1)
  node chain-load.mjs measure "$h0" "$(height $RPC)" | tail -25
  live1=$(height $LIVE_RPC)
  echo
  echo "live chain while this ran (it must not have slowed):"
  RPC=$LIVE_RPC node chain-load.mjs measure "$live0" "$live1" | grep -E '^\s+[0-9]+' | awk '{print $4}' | sort -n | awk '{a[NR]=$1} END {print "  " NR " blocks, interval min " a[1] " s, median " a[int((NR+1)/2)] " s, max " a[NR] " s"}'
  ;;

teardown)
  running && systemctl stop "$UNIT"
  systemctl reset-failed "$UNIT" 2>/dev/null || true
  rm -rf "$H"
  rm -f "$here"/bench-*.json "$here"/bench-*.addrs "$here"/bench-*.log
  echo "bench removed"
  ;;

*)
  echo "usage: standalone-bench.sh setup | load <clients> <accountsPerClient> <txsPerAccount> | teardown"; exit 2 ;;
esac
