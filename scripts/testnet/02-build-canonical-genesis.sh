#!/bin/bash
# Run this ONCE, by the coordinator, after collecting every validator's
# operator address + desired self-bond amount from step 1.
#
# Usage: 02-build-canonical-genesis.sh <accounts-file>
#
# <accounts-file> has one "address amount-with-denom" pair per line, e.g.:
#   yml1abc...   100000000uyml
#   yml1def...   100000000uyml
#   yml1ghi...   100000000uyml
#
# Produces ./canonical-genesis.json. Distribute this SAME file to every
# validator's ~/.blockchain/config/genesis.json before they run gentx
# (step 3) — every node must gentx against an identical genesis.
#
# Safe to re-run: the pristine genesis produced by step 1 is kept aside on the
# first run and restored before every rebuild. Without that, a run that failed
# part way left accounts already added, and the retry aborted with
# "Account ... already exists" against a half-edited file.
set -euo pipefail

ACCOUNTS_FILE="${1:?usage: 02-build-canonical-genesis.sh <accounts-file>}"
CHAIN_ID="yamale-testnet-1"
HOME_DIR="${BLOCKCHAIND_HOME:-$HOME/.blockchain}"
GENESIS="$HOME_DIR/config/genesis.json"
PRISTINE="$HOME_DIR/config/genesis.pristine.json"

# Step 1 leaves an untouched copy beside genesis.json. Requiring it rather than
# snapshotting whatever is present matters: if this script snapshotted an
# already-edited file it would pin the wrong baseline, and every later run would
# rebuild from it.
if [ ! -f "$PRISTINE" ]; then
  echo "error: $PRISTINE not found — run 01-init-node.sh on this machine first" >&2
  exit 1
fi

cp "$PRISTINE" "$GENESIS"

while read -r addr amount; do
  [ -z "$addr" ] && continue
  blockchaind genesis add-genesis-account "$addr" "$amount" --home "$HOME_DIR"
done < "$ACCOUNTS_FILE"

# Replace the parameters whose defaults exist for fast local iteration and must
# not reach a real network.
#
# `python3` first, then `python`: on Windows `python3` resolves to a Microsoft
# Store stub that prints an install prompt and exits non-zero, which is how a
# dry run of this script produced a genesis with the devnet emission schedule
# still in it. That genesis passes `genesis validate` — every parameter in it is
# individually legal — so the mistake is invisible until the chain is live and
# minting a thousand times too fast. The verification block at the end of this
# script exists for the same reason.
PYTHON=""
for candidate in python3 python; do
  if command -v "$candidate" >/dev/null 2>&1 && "$candidate" -c "import json" >/dev/null 2>&1; then
    PYTHON="$candidate"
    break
  fi
done
if [ -z "$PYTHON" ]; then
  echo "error: no working python found. Install python3 and re-run." >&2
  exit 1
fi

"$PYTHON" - "$GENESIS" <<'PYEOF'
import json, os, sys

path = sys.argv[1]
with open(path) as f:
    genesis = json.load(f)

# --- governance ---------------------------------------------------------
# The 10s/5s periods in config.yml are for devnet iteration only.
gov = genesis["app_state"]["gov"]["params"]
gov["voting_period"] = "172800s"           # 48h
gov["expedited_voting_period"] = "86400s"  # 24h
gov["min_deposit"] = [{"denom": "uyml", "amount": "10000000"}]
gov["expedited_min_deposit"] = [{"denom": "uyml", "amount": "50000000"}]

# --- emission -----------------------------------------------------------
# The module's defaults compress the whole issuance curve into roughly an hour
# so that its shape is visible on a devnet. On a network meant to run for
# months that means validators earn essentially nothing after the first
# afternoon, so the same curve is stretched over years here.
#
# The schedule is geometric: each period issues `reduction_factor` times the
# last, so the total ever issued converges to
#
#     provisions_per_block * blocks_per_period / (1 - reduction_factor)
#
# With 5s blocks a year is about 6,307,200 blocks. Keeping the same
# 1,000,000,000 YML (1e15 uyml) asymptote and a 2/3 factor gives:
#
#     1e15 * (1 - 2/3) / 6_307_200 ~= 52,850,000 uyml per block
#
# so roughly 333M YML in the first year, 222M in the second, and so on.
# Adjust `PROVISIONS_PER_BLOCK` and `BLOCKS_PER_PERIOD` together if you want a
# different total or a different block time — they are not independent.
BLOCKS_PER_PERIOD = "6307200"      # ~1 year at 5s blocks
PROVISIONS_PER_BLOCK = "52850000"  # ~52.85 YML per block
REDUCTION_FACTOR = "0.666666666666666667"

#
# Absent entirely on the settlement profile, which is built with `-tags
# settlement` and has no native token to issue. Guarded rather than assumed,
# because indexing a section that is not there aborts the ceremony script
# half-way through, leaving a partly-edited genesis on disk that still
# validates.
emission = genesis["app_state"].get("emission")
if emission:
    emission["params"]["reduction_period_in_blocks"] = BLOCKS_PER_PERIOD
    emission["params"]["genesis_provisions_per_block"] = PROVISIONS_PER_BLOCK
    emission["params"]["reduction_factor"] = REDUCTION_FACTOR
    # The running state must start on the same schedule as the parameters, or
    # the first period would pay out at the old rate.
    emission["emission_state"] = {
        "current_provisions_per_block": PROVISIONS_PER_BLOCK,
        "last_reduction_period": "0",
    }

# --- interchain accounts ------------------------------------------------
# Turned off, deliberately, and this is the most consequential line in the
# script.
#
# The module's default genesis enables the host with allow_messages ["*"], and
# an interchain account executes messages through the message router — never
# through the ante chain. The validator gate is an ante decorator, so with the
# host enabled anybody able to open a channel to this chain could execute
# MsgCreateValidator through it and join a permissioned validator set without a
# vote. The same route reaches every other message the chain has.
#
# Nothing is lost by disabling it at launch: no IBC connection exists on day
# one, so the host can only ever be reached after a relayer is set up. Enabling
# it later is a governance decision that should come with an explicit
# allow_messages list rather than a wildcard.
ica = genesis["app_state"].get("interchainaccounts")
if ica:
    ica["host_genesis_state"]["params"]["host_enabled"] = False
    ica["host_genesis_state"]["params"]["allow_messages"] = []
    ica["controller_genesis_state"]["params"]["controller_enabled"] = False

# --- oracle -------------------------------------------------------------
# The module's defaults are already sized for a real network — a rate agreed
# about once a minute, unusable after fifteen — so they are left alone. What is
# reported here is the consequence for a small validator set, because it is not
# obvious from the numbers: the threshold is a share of *stake*, so on a network
# of three equally-bonded validators at least two must run a price feeder or no
# rate is ever agreed and the module sits silently dead.
oracle = genesis["app_state"]["oracle"]["params"]
threshold_bps = int(oracle["vote_threshold_bps"])

# --- enforcement --------------------------------------------------------
# The recovery destination is the one parameter with no sensible default: it
# names a real institution — the foundation, which holds seized assets so they
# can be restituted to the people they were taken from. No address compiled
# into the binary is that institution, so the ceremony has to say it here.
#
# Required, not optional. It used to be left empty when unset, on the argument
# that a chain which can freeze but not seize is a safe launch. That argument
# was wrong in practice: the devnet ran for weeks with it empty, and what an
# empty destination actually buys is a chain where two thirds of the validator
# set can pass a seizure that then has nowhere to send what it took. The module
# now refuses to start without one, so a genesis built without one would only
# fail later, on every validator at height 1.
enforcement = genesis["app_state"]["enforcement"]["params"]
destination = os.environ.get("RECOVERY_DESTINATION", "").strip()
if not destination:
    sys.exit(
        "RECOVERY_DESTINATION is not set.\n"
        "Seized assets go to the foundation account, and the chain will not start\n"
        "without one. Re-run as:\n"
        "  RECOVERY_DESTINATION=yml1... ./02-build-canonical-genesis.sh accounts.txt"
    )
enforcement["recovery_destination"] = destination

# The founders' group policy address, if it exists yet. Unset means there is no
# emergency path at all — not an implicit one — so leaving it out is safe.
emergency = os.environ.get("EMERGENCY_AUTHORITY", "").strip()
if emergency:
    enforcement["emergency_authority"] = emergency

# The threshold is a share of bonded power, and at three equally-bonded
# validators two thirds rounds up to all three. Reported rather than adjusted:
# it is the same arithmetic that makes this network stop producing blocks when
# one of three nodes is down.
enforcement_threshold_bps = int(enforcement["threshold_bps"])

with open(path, "w") as f:
    json.dump(genesis, f, indent=2)

print(f"  gov voting period:      {gov['voting_period']}")
if emission:
    total = int(PROVISIONS_PER_BLOCK) * int(BLOCKS_PER_PERIOD) * 3
    print(f"  emission per block:     {PROVISIONS_PER_BLOCK} uyml")
    print(f"  emission period:        {BLOCKS_PER_PERIOD} blocks (~1 year at 5s)")
    print(f"  approximate total ever: {total // 1_000_000:,} YML")
else:
    print("  emission:               compiled out — no native issuance on this profile")
print(f"  oracle vote period:     {oracle['vote_period']} blocks")
print(f"  oracle threshold:       {threshold_bps / 100:g}% of stake must report")
print(f"  oracle denoms:          {', '.join(oracle['accepted_denoms'])}")
print(f"  interchain accounts:    disabled (host and controller)")
print(f"  enforcement threshold:  {enforcement_threshold_bps / 100:g}% of bonded power to freeze or seize")
print(f"  recovery destination:   {enforcement['recovery_destination']}")
print(f"  emergency authority:    {enforcement['emergency_authority'] or 'unset — no emergency freeze or release path'}")
PYEOF

blockchaind genesis validate --home "$HOME_DIR"

# `genesis validate` is necessary and not sufficient. Every devnet default in
# this file is individually legal, so a genesis where the edits above silently
# did not apply validates cleanly and then mints a thousand times too fast on a
# live network. This re-reads the file that will actually be distributed and
# refuses to produce it unless the values are the intended ones.
"$PYTHON" - "$GENESIS" <<'PYEOF'
import json, sys

with open(sys.argv[1]) as f:
    app = json.load(f)["app_state"]

problems = []
if app["gov"]["params"]["voting_period"] != "172800s":
    problems.append("gov voting_period was not set to 48h")
# Checked only when the profile has issuance. A settlement genesis carries no
# emission section and must not be failed for the absence of one — but a
# genesis that has the section must still have had it set, or the network mints
# a thousand times too fast and nothing here would have said so.
if "emission" in app:
    if app["emission"]["params"]["reduction_period_in_blocks"] != "6307200":
        problems.append("emission reduction period is still the devnet default")
    if app["emission"]["params"]["genesis_provisions_per_block"] != "52850000":
        problems.append("emission provisions per block is still the devnet default")
    if app["emission"]["emission_state"]["current_provisions_per_block"] != "52850000":
        problems.append("emission state does not start on the same schedule as the params")
if int(app["oracle"]["params"]["vote_period"]) == 0:
    problems.append("oracle vote_period is zero, which stops rates being agreed")

enforcement = app["enforcement"]["params"]
if not enforcement.get("recovery_destination", "").strip():
    problems.append(
        "enforcement recovery_destination is empty, so a seizure passed by two thirds of "
        "the validator set would have nowhere to send what it took — and this genesis "
        "will not start a chain"
    )
if int(enforcement["threshold_bps"]) <= 5000:
    problems.append(
        "enforcement threshold_bps is %s, which would let a minority of the validator "
        "set freeze and seize accounts" % enforcement["threshold_bps"]
    )
if int(enforcement["provisional_freeze_blocks"]) < int(enforcement["voting_period_blocks"]):
    problems.append(
        "enforcement provisional_freeze_blocks is shorter than voting_period_blocks, so "
        "freezes would lapse in the middle of the vote that decides them"
    )

ica = app.get("interchainaccounts")
if ica:
    host = ica["host_genesis_state"]["params"]
    if host.get("host_enabled"):
        problems.append(
            "the interchain accounts host is enabled; it executes messages without the "
            "ante chain, so the validator gate does not apply to them"
        )
    if host.get("allow_messages"):
        problems.append("the interchain accounts host allows messages: %s" % host["allow_messages"])
    if ica["controller_genesis_state"]["params"].get("controller_enabled"):
        problems.append("the interchain accounts controller is enabled")

if problems:
    print("REFUSING to write canonical-genesis.json:", file=sys.stderr)
    for p in problems:
        print(f"  - {p}", file=sys.stderr)
    sys.exit(1)

print("  verified: the distributed genesis carries the intended parameters")
PYEOF

cp "$GENESIS" ./canonical-genesis.json

echo
echo "=== canonical-genesis.json written ==="
echo "Distribute this file to every validator's \$HOME/.blockchain/config/genesis.json"
echo "before they run 03-create-gentx.sh."
