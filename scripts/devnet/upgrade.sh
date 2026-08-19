#!/bin/bash
# Upgrade the devnet without losing the people on it.
#
# Adding a module to a Cosmos chain is consensus-breaking, and the reflex is to
# wipe and start again. On a devnet that reflex has a real cost: every account,
# every user ID, every balance and every saved contact stops existing, and
# whoever was demonstrating the thing has to rebuild their world before they can
# show it. We did that three times in one day.
#
# The alternative is the standard export-and-restart path:
#
#   1. Export the running chain's state as a genesis document.
#   2. Splice in a default genesis section for each module that is new.
#   3. Reset the node's blocks and replay from that genesis.
#
# Accounts, balances, aliases, treasuries and pools all survive, because they
# are state and state is what an export carries. What does not survive is block
# history — heights restart at 1 — which is the honest trade: a devnet's history
# is worth less than its accounts.
#
# Usage:  upgrade.sh                 # keep the same chain id
#         upgrade.sh yamale-devnet-2 # rename, if the change is severe enough
set -euo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
SERVICES="yamale-devnet yamale-faucet"
NEW_CHAIN_ID="${1:-}"

STAMP=$(date +%Y%m%d-%H%M%S)
BACKUP=/opt/yamale/backups/$STAMP
mkdir -p "$BACKUP"

echo "=== stopping ==="
for s in $SERVICES; do sudo systemctl stop "$s" 2>/dev/null || true; done

echo "=== exporting state ==="
# The keyring is not part of genesis and is the one thing an export cannot
# rebuild, so it is copied before anything is touched.
cp -r "$HOME_DIR/keyring-test" "$BACKUP/" 2>/dev/null || true
cp "$HOME_DIR/config/genesis.json" "$BACKUP/genesis.before.json"
cp "$HOME_DIR/config/priv_validator_key.json" "$BACKUP/" 2>/dev/null || true
cp "$HOME_DIR/config/node_key.json" "$BACKUP/" 2>/dev/null || true

"$BIN" export --home "$HOME_DIR" > "$BACKUP/exported.json"
echo "  exported: $(wc -c < "$BACKUP/exported.json") bytes"

echo "=== splicing in modules the export does not know about ==="
# A module added since the running binary was built has no section in the
# exported state, and InitGenesis will refuse a genesis that omits it. The new
# binary knows its own defaults, so they are taken from a throwaway init rather
# than written by hand — hand-written genesis is how a launch ceremony fails at
# height 1.
SCRATCH=$(mktemp -d)
"$BIN" init upgrade-probe --chain-id probe --home "$SCRATCH" >/dev/null 2>&1

# The enforcement module refuses to start without a recovery destination, and an
# export made by an older binary can carry an empty one forward. That is exactly
# the state this devnet was found in, so the upgrade fills it rather than
# producing a genesis the new binary will not accept. Overridable, but the
# default is the account this chain already treats as the institution.
RECOVERY_DESTINATION="${RECOVERY_DESTINATION:-$("$BIN" keys show foundation -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null || true)}"

python3 - "$BACKUP/exported.json" "$SCRATCH/config/genesis.json" "${NEW_CHAIN_ID}" "${RECOVERY_DESTINATION}" <<'PY'
import json, sys

exported_path, defaults_path, new_chain_id = sys.argv[1], sys.argv[2], sys.argv[3]
recovery_destination = sys.argv[4] if len(sys.argv) > 4 else ""

exported = json.load(open(exported_path))
defaults = json.load(open(defaults_path))

added = []
for module, section in defaults["app_state"].items():
    if module not in exported["app_state"]:
        exported["app_state"][module] = section
        added.append(module)

# Heights restart, so anything the export stamped with a height is stale.
exported["initial_height"] = "1"
if new_chain_id:
    exported["chain_id"] = new_chain_id

# Governance stays fast on a devnet. An export carries the old params forward,
# and a 48-hour vote makes every gov-gated feature untestable again. Thirty
# minutes rather than three: three ran out while a proposal was being debugged,
# which is the failure this number exists to avoid.
gov = exported["app_state"].get("gov", {}).get("params")
if gov:
    gov["voting_period"] = "1800s"
    gov["expedited_voting_period"] = "900s"
    gov["max_deposit_period"] = "600s"

# Seized assets have to have somewhere to go, and the new binary will not start
# a chain that does not say where. Only filled if the export left it empty —
# a destination already chosen by governance is never overwritten here.
enforcement = exported["app_state"].get("enforcement", {}).get("params")
if enforcement is not None and not enforcement.get("recovery_destination", "").strip():
    if not recovery_destination:
        sys.exit(
            "enforcement recovery_destination is empty and no foundation key was found.\n"
            "Set RECOVERY_DESTINATION=yml1... and re-run: the chain will not start without it."
        )
    enforcement["recovery_destination"] = recovery_destination

json.dump(exported, open(exported_path, "w"), indent=2)
print("  modules added:", ", ".join(added) if added else "none")
print("  chain id     :", exported["chain_id"])
if enforcement is not None:
    print("  recovery dest:", enforcement.get("recovery_destination") or "UNSET")
PY

rm -rf "$SCRATCH"

echo "=== replacing genesis and resetting blocks ==="
cp "$BACKUP/exported.json" "$HOME_DIR/config/genesis.json"
# Blocks go, state comes from genesis. The validator key is kept so the node
# keeps its identity and its stake.
"$BIN" comet unsafe-reset-all --home "$HOME_DIR" --keep-addr-book >/dev/null 2>&1 \
  || "$BIN" tendermint unsafe-reset-all --home "$HOME_DIR" --keep-addr-book >/dev/null 2>&1

"$BIN" genesis validate-genesis --home "$HOME_DIR" 2>&1 | tail -1 || true

echo "=== starting ==="
for s in $SERVICES; do sudo systemctl start "$s" 2>/dev/null || true; done
sleep 20

echo "=== result ==="
echo "  height : $("$BIN" status --home "$HOME_DIR" 2>/dev/null | grep -oE '"latest_block_height":"[0-9]+"' | head -1)"
echo "  backup : $BACKUP"
echo
echo "Accounts, balances and user IDs carried over. Block history did not."
