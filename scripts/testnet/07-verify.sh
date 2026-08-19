#!/bin/bash
# Checks that a launched network is actually healthy. Run on any node once all
# three are started (step 6).
#
# Usage: 07-verify.sh [peer-rpc-url ...]
#
# With no arguments it checks the local node only. Give it the other validators'
# RPC URLs to also confirm every node agrees on the same block — the check that
# catches a mismatched genesis, which is the failure this ceremony is most
# likely to produce and the one that is hardest to spot by eye:
#
#   ./07-verify.sh http://203.0.113.11:26657 http://203.0.113.12:26657
#
# Exits non-zero if anything is wrong, so it can be used as a gate in a script.
set -uo pipefail

LOCAL="${LOCAL_RPC:-http://localhost:26657}"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"
EXPECTED_VALIDATORS="${EXPECTED_VALIDATORS:-3}"

problems=0
note() { echo "  $*"; }
bad() { echo "  ✗ $*"; problems=$((problems + 1)); }
good() { echo "  ✓ $*"; }

# Reads a field out of the RPC status response without needing jq, which is not
# installed by default on a minimal Ubuntu image.
status_field() {
  curl -s --max-time 10 "$1/status" 2>/dev/null |
    grep -o "\"$2\":\"[^\"]*\"" | head -1 | cut -d'"' -f4
}

echo "=== local node ==="
HEIGHT=$(status_field "$LOCAL" latest_block_height)
if [ -z "$HEIGHT" ]; then
  bad "no response from $LOCAL — is the service running? (systemctl status yamaled)"
  echo
  echo "$problems problem(s) found."
  exit 1
fi

CHAIN=$(status_field "$LOCAL" network)
CATCHING=$(curl -s --max-time 10 "$LOCAL/status" | grep -o '"catching_up":[a-z]*' | cut -d: -f2)
note "chain $CHAIN at height $HEIGHT"

# Blocks must be advancing, not merely present. A node stuck at a height still
# answers /status perfectly happily.
sleep 12
HEIGHT2=$(status_field "$LOCAL" latest_block_height)
if [ "${HEIGHT2:-0}" -gt "${HEIGHT:-0}" ]; then
  good "producing blocks ($HEIGHT → $HEIGHT2)"
else
  bad "no new block in 12s (still $HEIGHT) — with 3 validators this usually means one is down"
fi

if [ "$CATCHING" = "false" ]; then
  good "caught up with the network"
else
  note "still catching up — re-run when it has finished"
fi

echo
echo "=== validator set ==="
BONDED=$(blockchaind query staking validators --home "$HOME_DIR" --node "${LOCAL/http:\/\//tcp://}" \
  --output json 2>/dev/null | grep -c 'BOND_STATUS_BONDED')
if [ "${BONDED:-0}" -eq "$EXPECTED_VALIDATORS" ]; then
  good "$BONDED of $EXPECTED_VALIDATORS validators bonded"
else
  bad "$BONDED validators bonded, expected $EXPECTED_VALIDATORS — a gentx may not have been collected"
fi

# Losing any one of three equal validators stops the chain, so absence is worth
# reporting even while everything still looks fine.
echo
echo "=== agreement across nodes ==="
if [ "$#" -eq 0 ]; then
  note "no peer URLs given; pass them to check every node agrees on the same block"
else
  LOCAL_HASH=$(status_field "$LOCAL" latest_block_hash)
  for peer in "$@"; do
    PEER_HEIGHT=$(status_field "$peer" latest_block_height)
    if [ -z "$PEER_HEIGHT" ]; then
      bad "$peer unreachable"
      continue
    fi

    # Compare at a height both have seen rather than the tip, which drifts by a
    # block between two requests and would report a false mismatch.
    TARGET=$((HEIGHT2 < PEER_HEIGHT ? HEIGHT2 : PEER_HEIGHT))
    a=$(curl -s --max-time 10 "$LOCAL/block?height=$TARGET" | grep -o '"hash":"[A-F0-9]*"' | head -1 | cut -d'"' -f4)
    b=$(curl -s --max-time 10 "$peer/block?height=$TARGET" | grep -o '"hash":"[A-F0-9]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$a" ] && [ "$a" = "$b" ]; then
      good "$peer agrees at height $TARGET"
    else
      bad "$peer DISAGREES at height $TARGET ($a vs $b) — the genesis files were not identical"
    fi
  done
fi

echo
echo "=== price feeds ==="
RATES=$(blockchaind query oracle rates --home "$HOME_DIR" --node "${LOCAL/http:\/\//tcp://}" \
  --output json 2>/dev/null | grep -c '"denom"')
if [ "${RATES:-0}" -gt 0 ]; then
  good "$RATES rate(s) agreed"
else
  note "no rates agreed yet — at least two of three validators must run a feeder (step 7b)"
fi

echo
if [ "$problems" -eq 0 ]; then
  echo "All checks passed."
else
  echo "$problems problem(s) found."
fi
exit "$problems"
