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
# Read what it is doing with: journalctl -u yamale-upgrade-<name> -n 50
#
# Halted means: two consecutive polls of the node's height agree AND equal the
# plan's height. "Stopped" alone can mean crashed; the plan height alone can
# mean the node is about to reach it. A poll the RPC does not answer counts as
# no height at all: there is deliberately no "RPC is down, so assume halted"
# path, because a node that is down short of the halt is not at the halt.
#
# Everything below can be overridden from the environment (the tests do):
#   BIN (default /opt/yamale/bin/blockchaind), NODE, SWAP, LIMIT (seconds, 21600),
#   INTERVAL (seconds between polls, 20).
set -euo pipefail
NAME=${1:?the upgrade name}
BIN=${BIN:-/opt/yamale/bin/blockchaind}
NODE=${NODE:-http://127.0.0.1:26657}
SWAP=${SWAP:-/opt/yamale/bin/swap-upgrade-binary.sh}
LIMIT=${LIMIT:-21600}   # give up after 6 h: a plan that never comes is not this job's to wait for
INTERVAL=${INTERVAL:-20}

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*"; }
h() { "$BIN" status --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null || true; }
plan() { "$BIN" query upgrade plan --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;p=json.load(sys.stdin).get("plan",{});print(p.get("name",""),p.get("height",""))' 2>/dev/null || true; }

# What the swap needs, checked now rather than after hours of waiting.
[ -x "$BIN.next" ] || { echo "no executable staged binary at $BIN.next; stage it first"; exit 1; }
[ ! -e "$BIN.pre-$NAME" ] || { echo "$BIN.pre-$NAME already exists; move it aside first (the swap refuses to overwrite a rollback)"; exit 1; }

read -r PNAME PHEIGHT <<<"$(plan)"
[ "$PNAME" = "$NAME" ] || { echo "the scheduled plan is '${PNAME:-none}', not '$NAME'"; exit 1; }
log "waiting for $NAME at $PHEIGHT (now $(h)); polling every ${INTERVAL}s, giving up after ${LIMIT}s"
start=$(date +%s)
prev=""
while :; do
  cur=$(h)
  if [ -z "$cur" ]; then log "poll: rpc did not answer"; else log "poll: height $cur (previous ${prev:-none}), plan $PHEIGHT"; fi
  if [ -n "$prev" ] && [ "$prev" = "$cur" ] && [ "$cur" = "$PHEIGHT" ]; then break; fi
  prev=$cur
  [ $(( $(date +%s) - start )) -lt "$LIMIT" ] || { log "no halt at $PHEIGHT after ${LIMIT}s"; exit 1; }
  sleep "$INTERVAL"
done
log "halted at $PHEIGHT; crossing"
exec "$SWAP" "$NAME"
