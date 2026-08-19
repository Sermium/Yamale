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

sed -i.bak \
  -e "s/^persistent_peers = .*/persistent_peers = \"$PEER1,$PEER2\"/" \
  -e 's/^laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' \
  "$CONFIG_TOML"

echo "=== persistent_peers set in $CONFIG_TOML ==="
grep '^persistent_peers' "$CONFIG_TOML"
echo
echo "Next: install and start the systemd service (see yamaled.service),"
echo "then verify with: blockchaind status | jq .sync_info"
