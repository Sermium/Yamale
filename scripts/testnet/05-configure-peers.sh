#!/bin/bash
# Run this on EACH of the 3 validator VMs, after receiving final-genesis.json
# (step 4), listing the OTHER two nodes as peers.
#
# Usage: 05-configure-peers.sh <peer1> <peer2>
#   where each <peerN> is "<node-id>@<ip>:26656", e.g.:
#   05-configure-peers.sh \
#     abc123...@203.0.113.10:26656 \
#     def456...@203.0.113.11:26656
#
# Get a node's own ID with: blockchaind tendermint show-node-id
set -euo pipefail

PEER1="${1:?usage: 05-configure-peers.sh <peer1> <peer2>}"
PEER2="${2:?usage: 05-configure-peers.sh <peer1> <peer2>}"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"
CONFIG_TOML="$HOME_DIR/config/config.toml"

# peer_gossip_sleep_duration 100ms -> 10ms. Measured on devnet-2, 2026-10-12:
# a validator able to commit on its own vote proposes, prevotes and precommits
# within milliseconds, and at the default 100ms its precommit reached the others
# before the block did. A validator holding a precommit for a block it has not
# got precommits NIL, so the two followers confirmed 4 and 2 blocks in 30. At
# 10ms on the proposer: 27 and 28 in 30, block times unchanged (median 5.02s).
sed -i.bak \
  -e "s/^persistent_peers = .*/persistent_peers = \"$PEER1,$PEER2\"/" \
  -e 's/^laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' \
  -e 's/^peer_gossip_sleep_duration = .*/peer_gossip_sleep_duration = "10ms"/' \
  "$CONFIG_TOML"

echo "=== persistent_peers set in $CONFIG_TOML ==="
grep -E '^(persistent_peers|peer_gossip_sleep_duration)' "$CONFIG_TOML"
echo
echo "Next: install and start the systemd service (see yamaled.service),"
echo "then verify with: blockchaind status | jq .sync_info"
