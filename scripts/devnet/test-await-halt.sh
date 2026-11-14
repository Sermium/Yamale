#!/usr/bin/env bash
# Exercises await-halt-then-swap.sh against a stub node and a stub swap script.
#
#   scripts/devnet/test-await-halt.sh
#
# The stub "blockchaind" answers `status` from a sequence file (one height per
# line, or FAIL for an RPC that does not answer; the last line repeats) and
# `query upgrade plan` from a fixed plan.
set -uo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d); trap 'find "$T" -delete' EXIT
FAILED=0
pass() { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; FAILED=1; }

# python3 may be a Windows store stub; shim it with python when it does not run.
mkdir -p "$T/shim"
if ! python3 -c 1 >/dev/null 2>&1; then
  printf '#!/bin/sh\nexec python "$@"\n' > "$T/shim/python3"; chmod +x "$T/shim/python3"
  export PATH="$T/shim:$PATH"
fi

cat > "$T/blockchaind" <<'STUB'
#!/usr/bin/env bash
D=$(dirname "$0")
case "$1 $2 $3" in
  "status --node "*)
    sfx=""; case "$3" in *after*) sfx="-after" ;; esac
    n=$(cat "$D/count$sfx" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" > "$D/count$sfx"
    total=$(wc -l < "$D/heights$sfx"); [ "$n" -le "$total" ] || n=$total
    v=$(sed -n "${n}p" "$D/heights$sfx")
    [ "$v" != FAIL ] || exit 1
    echo "{\"sync_info\":{\"latest_block_height\":\"$v\"}}" ;;
  "query upgrade plan")
    if [ -f "$D/plans" ]; then
      n=$(cat "$D/planc" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" > "$D/planc"
      total=$(wc -l < "$D/plans"); [ "$n" -le "$total" ] || n=$total
      line=$(sed -n "${n}p" "$D/plans")
    else line=$(cat "$D/plan"); fi
    if [ "$line" = none ]; then echo '{"plan":null}'; exit 0; fi
    read -r name height <<<"$line"
    echo "{\"plan\":{\"name\":\"$name\",\"height\":\"$height\"}}" ;;
esac
STUB
printf '#!/bin/sh\necho "$@" >> "$(dirname "$0")/swapped"\n' > "$T/swap"
chmod +x "$T/blockchaind" "$T/swap"
printf '#!/bin/sh\n' > "$T/blockchaind.next"; chmod +x "$T/blockchaind.next"

# setup <plan> <heights...>: resets the stub. The watcher polls every 0.2 s.
setup() { rm -f "$T/count" "$T/count-after" "$T/heights-after" "$T/plans" "$T/planc" "$T/swapped"; echo "$1" > "$T/plan"; shift; printf '%s\n' "$@" > "$T/heights"; }
watch() {  # prints the exit status; the watcher's log goes to $T/out
  env BIN="$T/blockchaind" AFTER_NODE="${AFTER_NODE:-}" SWAP="$T/swap" INTERVAL=0.2 LIMIT="${LIMIT:-40}" \
    "$HERE/await-halt-then-swap.sh" jewel-register > "$T/out" 2>&1
  echo $?
}

echo "(a) height advancing, then stopped at the plan height"
setup "jewel-register 8" 4 5 6 7 8 8 8
rc=$(watch)
if [ "$rc" = 0 ] && [ "$(cat "$T/swapped" 2>/dev/null)" = "jewel-register" ]; then pass "swapped once, called with the name"; else fail "expected a swap (rc=$rc)"; fi
if grep -q "poll: height 5" "$T/out" && grep -q "halted at 8; crossing" "$T/out"; then pass "every poll is logged"; else fail "poll log incomplete"; fi
sed 's/^/       /' "$T/out"

echo "(b) stopped at a height other than the plan's"
setup "jewel-register 8" 7 7 7
rc=$(LIMIT=2 watch)
if [ "$rc" != 0 ] && [ ! -e "$T/swapped" ] && grep -q "no halt at 8" "$T/out"; then pass "no swap, non-zero exit at LIMIT"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(c) the plan names another upgrade"
setup "other-upgrade 8" 8 8 8
rc=$(watch)
if [ "$rc" != 0 ] && [ ! -e "$T/swapped" ] && grep -q "not 'jewel-register'" "$T/out"; then pass "refused at once"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(d1) the RPC fails on every other poll at the plan height: waits, never assumes halted"
setup "jewel-register 8" $(for _ in $(seq 1 60); do echo "8 FAIL"; done)
rc=$(LIMIT=2 watch)
if [ "$rc" != 0 ] && [ ! -e "$T/swapped" ] && grep -q "rpc did not answer" "$T/out"; then pass "no swap, 'rpc did not answer' logged"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(d2) one failed poll, then two consecutive agreeing polls"
setup "jewel-register 8" 6 7 8 FAIL 8 8
rc=$(watch)
if [ "$rc" = 0 ] && [ "$(cat "$T/swapped" 2>/dev/null)" = "jewel-register" ] && grep -q "rpc did not answer" "$T/out"; then pass "swapped after two consecutive agreeing polls"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(f) no plan at first, then the plan appears, then the node halts at it"
setup "x 0" 5 6 7 8 8 8
printf '%s
' none none none "jewel-register 8" > "$T/plans"
rc=$(watch)
if [ "$rc" = 0 ] && [ "$(cat "$T/swapped" 2>/dev/null)" = "jewel-register" ] && [ "$(wc -l < "$T/swapped")" = 1 ]    && grep -q "no plan scheduled yet" "$T/out" && grep -q "plan jewel-register at 8" "$T/out"; then pass "waited for the plan, swapped once"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(g) a different plan name appears while waiting"
setup "x 0" 5 6 7
printf '%s
' none none "other-upgrade 8" > "$T/plans"
rc=$(watch)
if [ "$rc" != 0 ] && [ ! -e "$T/swapped" ] && grep -q "not 'jewel-register'" "$T/out"; then pass "refused at once"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(h) AFTER_NODE: own node halted at 8, the after-node at 8 for a few polls, then past it"
setup "jewel-register 8" 8 8 8 8 8 8 8 8
printf '%s
' 8 8 8 8 9 > "$T/heights-after"
rc=$(AFTER_NODE=https://after.example:443 watch)
polls=$(grep -c "after.example:443 at height" "$T/out")
if [ "$rc" = 0 ] && [ "$(cat "$T/swapped" 2>/dev/null)" = "jewel-register" ] && [ "$polls" -ge 5 ] && grep -q "is past 8" "$T/out"; then pass "swapped only after the after-node passed 8 ($polls polls)"; else fail "rc=$rc polls=$polls"; cat "$T/out"; fi
sed 's/^/       /' "$T/out"

echo "(i) AFTER_NODE never passes the halt height"
setup "jewel-register 8" 8 8 8
printf '%s
' 8 > "$T/heights-after"
rc=$(AFTER_NODE=https://after.example:443 LIMIT=2 watch)
if [ "$rc" != 0 ] && [ ! -e "$T/swapped" ] && grep -q "did not pass 8" "$T/out"; then pass "no swap, non-zero exit at LIMIT"; else fail "rc=$rc"; cat "$T/out"; fi

echo "(e) preconditions, checked at startup"
setup "jewel-register 8" 8 8
rm "$T/blockchaind.next"
rc=$(watch)
if [ "$rc" != 0 ] && grep -q "no executable staged binary" "$T/out" && [ ! -e "$T/count" ]; then pass "missing .next refused before any poll"; else fail "rc=$rc"; cat "$T/out"; fi
printf '#!/bin/sh\n' > "$T/blockchaind.next"   # present but not executable
chmod -x "$T/blockchaind.next"
if [ -x "$T/blockchaind.next" ]; then
  echo "  skip non-executable .next (this filesystem cannot clear the executable bit)"
else
  rc=$(watch)
  if [ "$rc" != 0 ] && grep -q "no executable staged binary" "$T/out"; then pass "non-executable .next refused"; else fail "rc=$rc"; cat "$T/out"; fi
fi
chmod +x "$T/blockchaind.next"; touch "$T/blockchaind.pre-jewel-register"
rc=$(watch)
if [ "$rc" != 0 ] && grep -q "already exists" "$T/out" && [ ! -e "$T/swapped" ]; then pass "existing .pre-<name> refused"; else fail "rc=$rc"; cat "$T/out"; fi

if [ "$FAILED" = 0 ]; then echo "all await-halt checks passed"; else echo "FAILED"; exit 1; fi
