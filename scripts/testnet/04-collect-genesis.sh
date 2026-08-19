#!/bin/bash
# Run this ONCE, by the coordinator, after receiving all 3 gentx-*.json
# files from step 3.
#
# Usage: 04-collect-genesis.sh <dir-containing-all-gentx-files>
#
# Produces the FINAL $HOME/.blockchain/config/genesis.json. Distribute this
# file to all 3 validators, overwriting their local genesis.json again
# before they start blockchaind for the first time.
set -euo pipefail

GENTX_DIR="${1:?usage: 04-collect-genesis.sh <dir-containing-all-gentx-files>}"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"

mkdir -p "$HOME_DIR/config/gentx"
cp "$GENTX_DIR"/gentx-*.json "$HOME_DIR/config/gentx/"

blockchaind genesis collect-gentxs --home "$HOME_DIR"
blockchaind genesis validate --home "$HOME_DIR"

cp "$HOME_DIR/config/genesis.json" ./final-genesis.json

echo
echo "=== final-genesis.json written ==="
echo "Distribute this file to every validator's \$HOME/.blockchain/config/genesis.json,"
echo "then configure peers (05-configure-peers.sh) before starting blockchaind."
