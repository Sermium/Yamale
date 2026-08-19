#!/bin/bash
# Run this on EACH of the 3 validator VMs, after:
#   1. Placing the coordinator's canonical-genesis.json at
#      $HOME/.blockchain/config/genesis.json (overwriting the local one from
#      01-init-node.sh)
#   2. Creating the operator key referenced in that genesis (`blockchaind
#      keys add validator --keyring-backend file`), if not already created
#      before step 2 of the runbook
#
# Usage: 03-create-gentx.sh <moniker> <self-bond-amount-with-denom>
#   e.g. 03-create-gentx.sh validator-1 100000000uyml
#
# Produces a gentx-<node-id>.json file. Send this file back to the
# coordinator for step 4 (04-collect-genesis.sh).
set -euo pipefail

MONIKER="${1:?usage: 03-create-gentx.sh <moniker> <self-bond-amount>}"
AMOUNT="${2:?usage: 03-create-gentx.sh <moniker> <self-bond-amount>}"
CHAIN_ID="yamale-testnet-1"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"

blockchaind genesis gentx validator "$AMOUNT" \
  --chain-id "$CHAIN_ID" \
  --moniker "$MONIKER" \
  --commission-rate "0.10" \
  --commission-max-rate "0.20" \
  --commission-max-change-rate "0.01" \
  --min-self-delegation "1" \
  --keyring-backend file \
  --home "$HOME_DIR"

GENTX_FILE=$(ls "$HOME_DIR"/config/gentx/gentx-*.json | head -1)
echo
echo "=== gentx created: $GENTX_FILE ==="
echo "Send this file to the coordinator (e.g. scp) for collection."
