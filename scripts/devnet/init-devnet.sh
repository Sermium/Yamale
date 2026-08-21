#!/bin/bash
# Build a single-node Yamale devnet on the Pi, with every testnet currency
# seeded so the interfaces have real data to render. An empty chain renders
# perfectly no matter what is broken behind it, which is the opposite of useful
# when the point is to find gaps.
set -euo pipefail

BIN=/opt/yamale/bin/blockchaind
CURRENCIES=/opt/yamale/bin/currencies
HOME_DIR=/opt/yamale/node
CHAIN_ID=${CHAIN_ID:-yamale-devnet-2}
KR="--keyring-backend test --home $HOME_DIR"

# The foundation is a 3-of-5 x/group account produced by `ceremony`, not a key
# on this machine. CEREMONY_DIR is where that ceremony wrote its public output.
#
# Required, with no fallback to a local key. A single key that receives every
# seized asset on the chain is the arrangement this whole thing exists to end,
# and a script that quietly substituted one would restore it on the next reset
# without anybody deciding to.
CEREMONY_DIR=${CEREMONY_DIR:?set CEREMONY_DIR to the directory holding group.json from \`ceremony group\` or \`ceremony serve\`}
if [ ! -f "$CEREMONY_DIR/group.json" ]; then
  echo "no group.json in $CEREMONY_DIR — run the ceremony first" >&2
  exit 1
fi
POLICY=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["policy_address"])' "$CEREMONY_DIR/group.json")
echo "foundation group: $POLICY"

rm -rf "$HOME_DIR"

echo "=== init ==="
$BIN init pi --chain-id "$CHAIN_ID" --default-denom uyml --home "$HOME_DIR" 2>&1 | tail -1

echo "=== keys ==="
for k in alice bob foundation; do
  $BIN keys add "$k" $KR >/dev/null 2>&1
  echo "  $k  $($BIN keys show "$k" -a $KR)"
done

ALICE=$($BIN keys show alice -a $KR)
BOB=$($BIN keys show bob -a $KR)
FOUNDATION=$($BIN keys show foundation -a $KR)

echo "=== genesis accounts ==="
$BIN genesis add-genesis-account "$ALICE"      200000000000uyml --home "$HOME_DIR"
$BIN genesis add-genesis-account "$BOB"        100000000000uyml --home "$HOME_DIR"
$BIN genesis add-genesis-account "$FOUNDATION" 500000000000uyml --home "$HOME_DIR"
# The group policy address needs an auth account. Importing a group from genesis
# does not create one the way the runtime path does, and without it the first
# transfer into the account fails on an account that does not exist. It is
# unspendable except through the group's own policy — the address is a hash of a
# module name, not of any public key — so funding it is safe and giving it
# nothing would only mean the foundation cannot pay a fee.
$BIN genesis add-genesis-account "$POLICY" 100000000000uyml --home "$HOME_DIR"
echo "  three accounts and the foundation group funded"

echo "=== seeding currencies ==="
$CURRENCIES --genesis "$HOME_DIR/config/genesis.json" --issuer "$FOUNDATION"

echo "=== enforcement oversight ==="
# Where seized assets go. The foundation is the trust body administering the
# chain, and it holds what is recovered so it can be restituted to the people it
# was taken from. It is the same account that issues every currency here, so it
# is the one this devnet already treats as the institution.
#
# Set at genesis rather than left to a governance proposal later, because the
# chain now refuses to start without it. That refusal exists because of this
# devnet: it ran for weeks with the parameter empty, which meant two thirds of
# the validator set could have passed a seizure that then had nowhere to send
# what it took. Nobody noticed until a console printed the parameter.
#
# The delay schedule and the rolling cap have no defaults either, and for the
# same kind of reason: both are denominated, and no denomination compiled into
# the binary is anybody's currency. A default priced in uyml would be a live
# schedule that silently matched nothing on a chain issuing shillings, which is
# worse than an absent one because it satisfies the check.
#
# The values here are a devnet's, deliberately shorter than a deployment's:
# twenty minutes' floor rather than twelve hours, so that a seizure can actually
# be watched through its hold in an afternoon rather than being taken on trust.
#
# There is no `sed` fallback here any more, and that is deliberate: the tiers
# and the cap are nested JSON that sed cannot write, and a fallback that half
# applied would produce a genesis the chain refuses to start from — after the
# script had reported success. Failing loudly on a missing python3 is the
# smaller problem.
G=$HOME_DIR/config/genesis.json
python3 - "$G" "$POLICY" "$CEREMONY_DIR/group.json" <<'PY'
import json, sys
path, destination, group_path = sys.argv[1], sys.argv[2], sys.argv[3]
g = json.load(open(path))
asm = json.load(open(group_path))

# The group itself, at height zero.
#
# Not created afterwards by a transaction, and this is the whole reason the
# ceremony has to run before genesis rather than after. An x/group policy
# address derives from the group sequence number alone — not from the members,
# the threshold, the admin, or the chain id — so the address is knowable offline
# but commits to nothing about who controls it. A genesis that named the address
# and left the group to be created later would hand every future seizure to
# whoever created the first group policy on the chain. Address and membership
# are fixed by the same file, so there is no interval to race.
g["app_state"]["group"] = asm["group_genesis"]

# The invariants the chain will refuse to start without, and refuse to let
# governance edit afterwards.
#
# The ceremony supplies only the three it can derive from the group it just
# built — the recovery destination, the custodian count and the threshold.
# The other ten are policy that nobody's key determines, so genesis states
# them. Validate refuses a missing one, which is how their absence was found
# here rather than on a chain.
inv = dict(asm["constitution_invariants"])
inv.update({
    # 6000, not something tighter. With min_active_validators at 2, one of them
    # holds 5000 basis points by arithmetic, and the chain refuses a ceiling no
    # set that small could ever satisfy — rather than running permanently in
    # breach. 6000 still means one entity may hold one of two seats, not both.
    "max_entity_power_bps": "6000",
    "max_beneficial_owner_power_bps": "6000",
    "max_jurisdiction_power_bps": "6000",
    "concentration_epoch_blocks": "120",
    "min_active_validators": 2,
    "enforcement_threshold_bps": "6667",
    "enforcement_voting_period_blocks": "360",
    # At least the voting period, so the vote always ends before the freeze
    # lapses. The expiry queue underneath is the backstop, not the mechanism.
    "enforcement_provisional_freeze_blocks": "720",
    # Seven days is the floor compiled into the binary and cannot be shortened
    # by a genesis, which is the point: a delay a deployment could set to an
    # hour is not a delay.
    "amendment_delay_blocks": "120960",
    # Must exceed the seizure threshold. A chain where amending the constitution
    # is easier than acting under it has the wrong thing hard.
    "amendment_threshold_bps": "8000",
})
g["app_state"].setdefault("constitution", {})["invariants"] = inv

p = g["app_state"]["enforcement"]["params"]
p["recovery_destination"] = destination
# These three duplicate constitutional invariants, and AssertConstitutional
# refuses a genesis where the two disagree — so they are written from the same
# values rather than left to drift.
p["threshold_bps"] = inv["enforcement_threshold_bps"]
p["voting_period_blocks"] = inv["enforcement_voting_period_blocks"]
p["provisional_freeze_blocks"] = inv["enforcement_provisional_freeze_blocks"]

# ~20 minutes at 5s blocks. Every seizure waits at least this long after the
# vote, which is the window the ombudsman's veto lands in.
p["seizure_delay_blocks"] = "240"

# Scaled to this devnet's faucet amounts rather than to a real economy: a
# million uyml is a large balance here, so it is where the wait steps up.
p["seizure_delay_tiers"] = [
    {"threshold": {"denom": "uyml", "amount": "1000000"}, "delay_blocks": "720"},
    {"threshold": {"denom": "uyml", "amount": "100000000"}, "delay_blocks": "2880"},
]

# ~24 hours, and a cap that a devnet cannot casually exceed but a test can.
p["seizure_window_blocks"] = "17280"
p["seizure_window_cap"] = [{"denom": "uyml", "amount": "500000000"}]
p["max_seizures_per_window"] = "5"

# No ombudsman by default. An unappointed office means nobody, never anybody,
# and appointing one on a devnet with a single operator would be theatre.
# Set OMBUDSMAN=yml1... to name one.
import os
ombudsman = os.environ.get("OMBUDSMAN", "").strip()
if ombudsman:
    p["ombudsman"] = ombudsman

json.dump(g, open(path, "w"), indent=2)
PY
echo "  recovery destination: $(grep -o '"recovery_destination": *"[^"]*"' "$G" | head -1)"
echo "  seizure delay floor:  240 blocks (~20m), stepping to 720 over 1 YML and 2880 over 100 YML"
echo "  rolling cap:          500 YML or 5 seizures per 17280 blocks (~24h)"
echo "  ombudsman:            ${OMBUDSMAN:-unset — no veto}"

echo "=== validator ==="
$BIN genesis gentx alice 100000000000uyml --chain-id "$CHAIN_ID" $KR 2>&1 | tail -1
$BIN genesis collect-gentxs --home "$HOME_DIR" 2>&1 | tail -1
$BIN genesis validate-genesis --home "$HOME_DIR"

echo "=== shortening governance for a devnet ==="
# A 48-hour voting period makes every governance path untestable, which is why
# the custody asset registration, the stablecoin issuer approval and the
# validator admission flows have only ever been exercised in unit tests.
#
# Thirty minutes, not three. Three was too short to be useful: a proposal has to
# be submitted, then voted by both validators, and each vote is a separate
# transaction from a separate key on a separate machine — one per account per
# block, because the ante handler rejects a non-current sequence. Any pause to
# read an error, fix a flag and retry ran the clock out, so proposals expired
# while they were being debugged. Half an hour survives a debugging session and
# is still short enough to watch a proposal pass inside one.
#
# Deposits drop to a nominal amount for the same reason: the point is to
# exercise the permission gate, not to model the economics.
python3 - "$G" <<'PY' 2>/dev/null || sed -i \
  -e 's|"voting_period": *"[^"]*"|"voting_period": "1800s"|g' \
  -e 's|"expedited_voting_period": *"[^"]*"|"expedited_voting_period": "900s"|g' \
  -e 's|"max_deposit_period": *"[^"]*"|"max_deposit_period": "600s"|g' "$G"
import json, sys
p = sys.argv[1]
g = json.load(open(p))
params = g["app_state"]["gov"]["params"]
params["voting_period"] = "1800s"
params["expedited_voting_period"] = "900s"
params["max_deposit_period"] = "600s"
params["min_deposit"] = [{"denom": "uyml", "amount": "1000000"}]
params["expedited_min_deposit"] = [{"denom": "uyml", "amount": "2000000"}]
json.dump(g, open(p, "w"), indent=2)
PY
echo "  voting period: $(grep -o '"voting_period": *"[^"]*"' "$G" | head -1)"

echo "=== opening the API ==="
APP=$HOME_DIR/config/app.toml
# The node refuses to start without a minimum gas price, and a devnet wants it
# free. Bound to localhost only -- nginx is what faces the network.
sed -i 's|^minimum-gas-prices = .*|minimum-gas-prices = "0uyml"|' "$APP"
sed -i '/^\[api\]/,/^\[/{s|^enable = .*|enable = true|}' "$APP"
sed -i '/^\[api\]/,/^\[/{s|^address = .*|address = "tcp://127.0.0.1:1317"|}' "$APP"
# Prometheus, so the monitoring config written earlier has something to scrape.
sed -i 's|^prometheus = false|prometheus = true|' "$HOME_DIR/config/config.toml"

echo "  minimum-gas-prices : $(grep '^minimum-gas-prices' "$APP")"
echo "  api enable         : $(sed -n '/^\[api\]/,/^\[grpc\]/p' "$APP" | grep '^enable')"
echo "  api address        : $(sed -n '/^\[api\]/,/^\[grpc\]/p' "$APP" | grep '^address')"
echo "  prometheus         : $(grep '^prometheus =' "$HOME_DIR/config/config.toml")"

echo
echo "DEVNET READY"
echo "alice=$ALICE"
echo "bob=$BOB"
echo "foundation=$FOUNDATION"
