#!/bin/bash
# Cross the upgrade height: stop, keep the old binary, swap, start.
#
# Run on a node that has ALREADY HALTED. A node that is still producing has not
# reached the height, and swapping under it is the one way to turn a clean
# coordinated upgrade into the uncoordinated one x/upgrade exists to prevent.
#
# THE HALT CHECK IS A HEIGHT, NOT A LOG LINE. The first version of this script
# grepped the last 200 journal lines for UPGRADE NEEDED and refused a node that
# had genuinely halted 13 minutes earlier: pex logs every 30 seconds, so the one
# line that mattered had scrolled out of the window. A log is a scrolling buffer
# and the thing being tested is a state, so the state is what gets tested — the
# node has stopped advancing, and it stopped at the scheduled height.
set -euo pipefail

# The name of the upgrade being crossed, which is where the OLD binary is kept.
#
# This was hardcoded to `.pre-documents` from the run it was written for. Left
# that way, the second upgrade's backup overwrites the first's — so the rollback
# path for the run you are on destroys the rollback path for the run before it,
# silently, at the moment a rollback is most likely to be wanted. Taken from the
# scheduled plan below when not given, so it cannot be stale again.
UPGRADE=${1:-}

BIN=/opt/yamale/bin/blockchaind
NEXT=$BIN.next
SERVICE=yamale-devnet
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657

[ -x "$NEXT" ] || { echo "no staged binary at $NEXT"; exit 1; }

height() {
  "$BIN" status --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null
}
plan_height() {
  "$BIN" query upgrade plan --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null \
    | python3 -c 'import sys,json;print(json.load(sys.stdin).get("plan",{}).get("height",""))' 2>/dev/null
}

plan_name() {
  "$BIN" query upgrade plan --home "$HOME_DIR" --node "$NODE" -o json 2>/dev/null     | python3 -c 'import sys,json;print(json.load(sys.stdin).get("plan",{}).get("name",""))' 2>/dev/null
}

PLAN=$(plan_height)
[ -n "$PLAN" ] || { echo "no upgrade plan is scheduled — nothing to cross"; exit 1; }

# The chain's own word for which upgrade this is, so the kept binary is named
# after the thing it is the rollback from.
[ -n "$UPGRADE" ] || UPGRADE=$(plan_name)
[ -n "$UPGRADE" ] || { echo "the plan has no name; pass one as the first argument"; exit 1; }
KEEP=$BIN.pre-$UPGRADE

if [ -e "$KEEP" ]; then
  echo "$KEEP already exists — a previous run of this upgrade kept a binary there."
  echo "Move it aside before running again; overwriting it destroys the rollback."
  exit 1
fi

A=$(height); sleep 8; B=$(height)
if [ -z "$A" ] || [ "$A" != "$B" ]; then
  echo "still producing ($A -> $B) — this node has not reached the halt height yet"
  exit 1
fi
if [ "$A" != "$PLAN" ]; then
  echo "stopped at $A but the plan is at $PLAN — that is not this upgrade's halt"
  exit 1
fi

echo "== halted at $A, plan $PLAN ($UPGRADE), swapping =="
sudo systemctl stop "$SERVICE"
sudo cp -f "$BIN" "$KEEP"
sudo mv "$NEXT" "$BIN"
sudo chmod +x "$BIN"
"$BIN" version >/dev/null || { echo "the new binary does not run; restoring"; sudo cp -f "$KEEP" "$BIN"; exit 1; }
sudo systemctl start "$SERVICE"

echo "  old binary kept at $KEEP"
sleep 15
sudo journalctl -u "$SERVICE" --no-pager -n 300 2>/dev/null \
  | grep -E 'applying upgrade|arrived|nothing to do|UPGRADE|panic|ERR' | tail -8 || true
echo "  height now: $(height)"
