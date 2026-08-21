#!/bin/bash
# Install a node and sync it against an existing chain.
#
# Usage: 01-install-node.sh <moniker> <peer-rpc-url> [peer-id@host:port ...]
#   e.g. 01-install-node.sh banque-nationale https://rpc.example.com \
#          abc123...@100.64.0.2:26656
#
# This does NOT make you a validator. It makes you a full node that has
# demonstrably kept up, which is the argument you bring to the vote in step 2.
# Admission on this chain is a governance decision, not an auction, so the
# sequence is: run, be seen running, then ask.
set -euo pipefail

MONIKER="${1:?usage: 01-install-node.sh <moniker> <peer-rpc-url> [peer-id@host:port ...]}"
PEER_RPC="${2:?usage: 01-install-node.sh <moniker> <peer-rpc-url> [peer-id@host:port ...]}"
shift 2
PEERS="$*"

BIN=${BIN:-/opt/yamale/bin/blockchaind}
HOME_DIR=${BLOCKCHAIND_HOME:-$HOME/.blockchain}

# The chain id comes from the peer rather than from a flag. A mistyped chain id
# produces a node that syncs nothing and reports no error worth reading, and
# every signature it later makes would be for a chain that does not exist.
CHAIN_ID=$(curl -sf "$PEER_RPC/status" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["node_info"]["network"])')
echo "chain: $CHAIN_ID (read from $PEER_RPC)"

if [ -d "$HOME_DIR/config" ]; then
  echo "error: $HOME_DIR already initialised. Move it aside first — this script will not" >&2
  echo "       overwrite a home directory that may hold a consensus key." >&2
  exit 1
fi

echo "=== init ==="
$BIN init "$MONIKER" --chain-id "$CHAIN_ID" --default-denom uyml --home "$HOME_DIR" >/dev/null
echo "  $HOME_DIR"

# The consensus key was just generated, and it is the one key on this host that
# must never leave it. Said here because it is the moment it comes into
# existence, and because it is easily confused with the operator key from the
# ceremony — the correct handling is opposite for each.
echo
echo "=== consensus key (generated just now, never leaves this host) ==="
$BIN comet show-validator --home "$HOME_DIR"
echo "  Back up $HOME_DIR/config/priv_validator_key.json somewhere only this host"
echo "  can restore from. A copy that has been anywhere else is a copy that might"
echo "  sign two blocks at one height, which is the one offence slashed for certain."
echo

echo "=== genesis, from the peer ==="
# Taken from the network rather than from a file somebody sent, and then hashed
# so it can be compared out of band. Two nodes on different genesis files fork
# at the first block, and the symptom is an app-hash mismatch that names
# neither cause.
curl -sf "$PEER_RPC/genesis" |
  python3 -c 'import json,sys; json.dump(json.load(sys.stdin)["result"]["genesis"], open(sys.argv[1],"w"), indent=2)' \
  "$HOME_DIR/config/genesis.json"
$BIN genesis validate "$HOME_DIR/config/genesis.json"
echo "  sha256: $(sha256sum "$HOME_DIR/config/genesis.json" | cut -d' ' -f1)"
echo "  Compare that with the operators you are joining. If it differs, stop."

if [ -n "$PEERS" ]; then
  echo "=== peers ==="
  JOINED=$(echo "$PEERS" | tr ' ' ',')
  sed -i "s|^persistent_peers = .*|persistent_peers = \"$JOINED\"|" "$HOME_DIR/config/config.toml"
  echo "  $JOINED"
else
  echo "=== peers ==="
  echo "  none given. Set persistent_peers in $HOME_DIR/config/config.toml before starting,"
  echo "  or this node will sit alone and report nothing wrong."
fi

# No public P2P port is needed. This chain peers its nodes over a private
# network, which is what lets a validator behind a home connection join at all —
# that connection accepts no inbound, and does not need to.
cat <<'NOTE'

=== next ===

  1. Start the node and let it catch up:

       blockchaind start --minimum-gas-prices 0.0001uyml

     Confirm `catching_up` has gone false:

       blockchaind status | python3 -c 'import json,sys; print(json.load(sys.stdin)["sync_info"]["catching_up"])'

  2. Generate your OPERATOR key in a ceremony — not with `keys add`:

       ceremony validator --name "<moniker>" --armor operator.asc
       blockchaind keys import validator operator.asc --keyring-backend file

  3. Then apply: ./02-apply-to-join.sh

Do not skip step 1. You are asking to be trusted with block production, and
having demonstrably kept up is the argument.
NOTE
