#!/usr/bin/env bash
# Wait for this node to halt at the scheduled upgrade, then cross it.
#
# Run as root on each validator host, as a systemd-run job, so the chain never
# depends on a laptop's ssh session staying up: on 2026-10-12 the watcher ran
# over ssh, the connection reset, and devnet-2 sat halted for seven hours.
#
#   sudo systemd-run --unit yamale-upgrade-<name> --collect \
#     /opt/yamale/bin/await-halt-then-swap.sh <name>
#
# Halted means: the height has not moved for 20 s AND it equals the plan's
# height. "Stopped" alone can mean crashed; the plan height alone can mean the
# node is about to reach it.
set -euo pipefail
NAME=${1:?the upgrade name}
BIN=/opt/yamale/bin/blockchaind
NODE=http://127.0.0.1:26657
LIMIT=${LIMIT:-21600}   # give up after 6 h: a plan that never comes is not this job's to wait for

h() { "$BIN" status --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null || true; }
plan() { "$BIN" query upgrade plan --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;p=json.load(sys.stdin).get("plan",{});print(p.get("name",""),p.get("height",""))' 2>/dev/null || true; }

read -r PNAME PHEIGHT <<<"$(plan)"
[ "$PNAME" = "$NAME" ] || { echo "the scheduled plan is '${PNAME:-none}', not '$NAME'"; exit 1; }
echo "waiting for $NAME at $PHEIGHT (now $(h))"
start=$(date +%s)
while :; do
  a=$(h); sleep 20; b=$(h)
  if [ -n "$a" ] && [ "$a" = "$b" ] && [ "$a" = "$PHEIGHT" ]; then break; fi
  [ $(( $(date +%s) - start )) -lt "$LIMIT" ] || { echo "no halt at $PHEIGHT after ${LIMIT}s"; exit 1; }
done
echo "halted at $PHEIGHT; crossing"
exec /opt/yamale/bin/swap-upgrade-binary.sh "$NAME"
