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
# The watcher is started BEFORE the governance vote passes, so at first there is
# no plan at all. It polls for the plan every INTERVAL (logging each poll) until
# one names this upgrade, then reads that plan's height. A plan naming a
# DIFFERENT upgrade is refused at once: that is a mistake to look at, not
# something to wait out. The wait for the plan is bounded by LIMIT as well.
#
# Halted means: two consecutive polls of the node's height agree AND equal the
# plan's height. "Stopped" alone can mean crashed; the plan height alone can
# mean the node is about to reach it. A poll the RPC does not answer counts as
# no height at all: there is deliberately no "RPC is down, so assume halted"
# path, because a node that is down short of the halt is not at the halt.
#
# AFTER_NODE (optional) makes a FOLLOWER wait for the master. Once this node has
# confirmed its own halt at H, it polls `blockchaind status --node "$AFTER_NODE"`
# and swaps only when that node reports a height above H, i.e. the master has
# crossed and is producing again. Set it on the followers, never on the master
# (a master waiting for itself would never swap). The value is whatever `--node`
# accepts: tcp://host:26657, http://host:26657 or https://rpc.yamaletech.com:443.
#
# Everything below can be overridden from the environment (the tests do):
#   BIN (default /opt/yamale/bin/blockchaind), NODE, SWAP, LIMIT (seconds, 21600),
#   INTERVAL (seconds between polls, 20), AFTER_NODE (default empty).
set -euo pipefail
# systemd-run starts this with no HOME, and blockchaind panics in its init when
# HOME is unset, before it reads a flag. Every query below discards its errors,
# so on 2026-11-15 all three watchers logged "no plan scheduled yet" for a plan
# that was on the chain, and would have logged "rpc did not answer" at the halt
# and never swapped. Nothing here reads HOME; the binary only needs it defined.
export HOME=${HOME:-/root}
NAME=${1:?the upgrade name}
BIN=${BIN:-/opt/yamale/bin/blockchaind}
NODE=${NODE:-http://127.0.0.1:26657}
SWAP=${SWAP:-/opt/yamale/bin/swap-upgrade-binary.sh}
AFTER_NODE=${AFTER_NODE:-}
LIMIT=${LIMIT:-21600}   # give up after 6 h: a plan that never comes is not this job's to wait for
INTERVAL=${INTERVAL:-20}

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*"; }
h() { "$BIN" status --node "${1:-$NODE}" -o json 2>/dev/null | python3 -c 'import sys,json;print(json.load(sys.stdin)["sync_info"]["latest_block_height"])' 2>/dev/null || true; }
plan() { "$BIN" query upgrade plan --node "$NODE" -o json 2>/dev/null | python3 -c 'import sys,json;p=json.load(sys.stdin).get("plan") or {};print(p.get("name",""),p.get("height",""))' 2>/dev/null || true; }

# What the swap needs, checked now rather than after hours of waiting.
[ -x "$BIN.next" ] || { echo "no executable staged binary at $BIN.next; stage it first"; exit 1; }
[ ! -e "$BIN.pre-$NAME" ] || { echo "$BIN.pre-$NAME already exists; move it aside first (the swap refuses to overwrite a rollback)"; exit 1; }

start=$(date +%s)
expired() { [ $(( $(date +%s) - start )) -ge "$LIMIT" ]; }

# 1. Wait for the plan to name this upgrade.
log "waiting for a plan named $NAME; polling every ${INTERVAL}s, giving up after ${LIMIT}s"
while :; do
  PNAME=""; PHEIGHT=""
  read -r PNAME PHEIGHT <<<"$(plan)" || true
  if [ -z "$PNAME" ]; then
    log "poll: no plan scheduled yet"
  elif [ "$PNAME" = "$NAME" ] && [ -n "$PHEIGHT" ]; then
    log "poll: plan $PNAME at $PHEIGHT"
    break
  else
    log "poll: the scheduled plan is '$PNAME', not '$NAME'"
    echo "the scheduled plan is '$PNAME', not '$NAME'"; exit 1
  fi
  expired && { log "no plan named $NAME after ${LIMIT}s"; exit 1; }
  sleep "$INTERVAL"
done

# 2. Wait for this node to halt at the plan's height.
log "waiting for $NAME at $PHEIGHT (now $(h))"
prev=""
while :; do
  cur=$(h)
  if [ -z "$cur" ]; then log "poll: rpc did not answer"; else log "poll: height $cur (previous ${prev:-none}), plan $PHEIGHT"; fi
  if [ -n "$prev" ] && [ "$prev" = "$cur" ] && [ "$cur" = "$PHEIGHT" ]; then break; fi
  prev=$cur
  expired && { log "no halt at $PHEIGHT after ${LIMIT}s"; exit 1; }
  sleep "$INTERVAL"
done
log "halted at $PHEIGHT"

# 3. A follower waits for the master to have crossed.
if [ -n "$AFTER_NODE" ]; then
  log "waiting for $AFTER_NODE to pass $PHEIGHT before crossing"
  while :; do
    a=$(h "$AFTER_NODE")
    if [ -z "$a" ]; then log "poll: $AFTER_NODE did not answer"; else log "poll: $AFTER_NODE at height $a"; fi
    if [ -n "$a" ] && [ "$a" -gt "$PHEIGHT" ] 2>/dev/null; then break; fi
    expired && { log "$AFTER_NODE did not pass $PHEIGHT after ${LIMIT}s"; exit 1; }
    sleep "$INTERVAL"
  done
  log "$AFTER_NODE is past $PHEIGHT"
fi
log "halted at $PHEIGHT; crossing"
exec "$SWAP" "$NAME"
