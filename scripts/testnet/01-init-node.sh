#!/bin/bash
# Run this on EACH of the 3 validator VMs.
#
# Usage: 01-init-node.sh <moniker>
#
# Creates this node's local config, node key, and consensus (priv_validator)
# key under ~/.blockchain. It does NOT create genesis.json yet — that is
# assembled centrally by the coordinator in step 2/4 and distributed to
# every node before gentx (step 3).
set -euo pipefail

MONIKER="${1:?usage: 01-init-node.sh <moniker>}"
CHAIN_ID="yamale-testnet-1"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"

blockchaind init "$MONIKER" --chain-id "$CHAIN_ID" --home "$HOME_DIR" --default-denom uyml --overwrite

# A bare `init` leaves minimum-gas-prices empty, which the SDK refuses to
# start with (it exists to stop a validator from accidentally accepting
# zero-fee transactions). 0.001uyml is a reasonable testnet floor.
sed -i 's/^minimum-gas-prices = ""/minimum-gas-prices = "0.001uyml"/' "$HOME_DIR/config/app.toml"

# Keep an untouched copy of the freshly-initialised genesis. Step 2 edits
# genesis.json in place, so without a pristine baseline a second run would be
# editing an already-edited file — which fails on "account already exists" and
# leaves the coordinator with no clean way to retry.
cp "$HOME_DIR/config/genesis.json" "$HOME_DIR/config/genesis.pristine.json"

echo
echo "=== Node initialized ==="
echo "Home directory: $HOME_DIR"
echo "Node ID:        $(blockchaind tendermint show-node-id --home "$HOME_DIR")"
echo "Consensus pubkey:"
blockchaind tendermint show-validator --home "$HOME_DIR"
echo
echo "Next: create this node's operator key if you don't already have one:"
echo "  blockchaind keys add validator --keyring-backend file --home $HOME_DIR"
echo
echo "Send the operator address (blockchaind keys show validator -a --keyring-backend file --home $HOME_DIR)"
echo "and the desired initial self-bond amount to the coordinator so they can"
echo "add it to the canonical genesis.json (see 02-add-genesis-accounts.sh)."
