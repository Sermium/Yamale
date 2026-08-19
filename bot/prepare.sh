#!/bin/bash
# Creates and funds the accounts the bot's default config uses.
#
# Run this once, on the machine that will run the bot, before starting the
# service. Step 8 of the deployment runbook used to say "make sure the accounts
# are funded" and leave the reader to work out how; this is that, executable.
#
# Usage: ./prepare.sh <funding-key-name> [amount-per-account]
#
# <funding-key-name> is a key in the local keyring holding enough uyml — on a
# fresh testnet that is usually the validator's own operator key.
set -euo pipefail

FUNDER="${1:?usage: prepare.sh <funding-key-name> [amount-per-account]}"
AMOUNT="${2:-10000000000}"   # 10,000 YML each by default
CHAIN_ID="${CHAIN_ID:-yamale-testnet-1}"
KEYRING="${KEYRING_BACKEND:-test}"
NODE="${NODE:-tcp://localhost:26657}"
HOME_FLAG=""
[ -n "${BLOCKCHAIND_HOME:-}" ] && HOME_FLAG="--home $BLOCKCHAIND_HOME"

ACCOUNTS=(bot-a bot-b)

# shellcheck disable=SC2086
q() { blockchaind "$@" --keyring-backend "$KEYRING" $HOME_FLAG; }

echo "=== creating the bot's accounts ==="
for name in "${ACCOUNTS[@]}"; do
  if q keys show "$name" -a >/dev/null 2>&1; then
    echo "  $name already exists"
  else
    # These are disposable automation keys; the mnemonic is deliberately not
    # echoed, because this script's output tends to end up in a deployment log.
    q keys add "$name" >/dev/null 2>&1
    echo "  $name created"
  fi
done

echo
echo "=== funding them from $FUNDER ==="
FUNDER_ADDR=$(q keys show "$FUNDER" -a)
for name in "${ACCOUNTS[@]}"; do
  ADDR=$(q keys show "$name" -a)

  # shellcheck disable=SC2086
  HASH=$(blockchaind tx bank send "$FUNDER_ADDR" "$ADDR" "${AMOUNT}uyml" \
    --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" --node "$NODE" $HOME_FLAG \
    --fees 500uyml --yes --output json | grep -oE '"txhash": *"[A-F0-9]+"' | grep -oE '[A-F0-9]{64}')

  # The code printed at broadcast is CheckTx only; this is the one that counts.
  sleep 6
  # shellcheck disable=SC2086
  CODE=$(blockchaind query tx "$HASH" --node "$NODE" $HOME_FLAG --output json 2>/dev/null |
    grep -oE '"code": *[0-9]+' | head -1 | grep -oE '[0-9]+$')
  echo "  $name  $ADDR  funded with $((AMOUNT / 1000000)) YML (tx code ${CODE:-unknown})"
done

echo
echo "=== balances ==="
for name in "${ACCOUNTS[@]}"; do
  ADDR=$(q keys show "$name" -a)
  # shellcheck disable=SC2086
  echo "  $name: $(blockchaind query bank balances "$ADDR" --node "$NODE" $HOME_FLAG --output json |
    grep -oE '"amount": *"[0-9]+"' | head -1 | grep -oE '[0-9]+') uyml"
done

echo
echo "The bot's default actions will work now. Swaps and ISO 20022 payments are"
echo "commented out in config.yaml — each needs approvals or a pool first."
