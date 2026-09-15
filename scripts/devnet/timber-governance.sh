#!/bin/bash
# Timber, stage A: the two things only governance can do, in one proposal.
#
# A tokenisation of the SRM holding needs two facts that do not exist on this
# chain:
#
#   1. A UNITED STATES DOLLAR. There is none. The deck is denominated in
#      dollars throughout - "dividends in dollars from a UAE bank" - and the
#      nearest thing on the chain is `usdg`, which is the SUDANESE POUND. Paying
#      a dollar dividend in Sudanese pounds because the ticker looked close is
#      exactly the class of error this platform exists to make impossible, so a
#      real uusd is registered and its issuer approved.
#
#   2. A COLLECTION THAT IS NOT LAND. The only collection on the chain is
#      `cd-terres`, a Congolese land registry. The asset here is an Emirati
#      holding company that collects the proceeds of a concession; it is not a
#      parcel, and minting it into a land registry would assert a relationship
#      to registered land that does not exist and that x/tokenisation would then
#      enforce ceilings against.
#
# Both are governance messages, so they ride in ONE proposal. Two proposals
# would be two votes and two thirty-minute windows for one decision.
#
# Registering the currency is permissionless and is done first, because
# ApproveIssuer refuses an application that is not pending.
set -uo pipefail

BIN=/opt/yamale/bin/blockchaind
HOME_DIR=/opt/yamale/node
NODE=http://127.0.0.1:26657
TX="--keyring-backend test --home $HOME_DIR --chain-id yamale-devnet-2 --node $NODE --gas auto --gas-adjustment 1.5 --fees 6000uyml -y"
Q="--home $HOME_DIR --node $NODE -o json"

# The sponsor: the UAE holding company's account, and the issuer of the dollar.
SPONSOR_KEY=srm-holding
ISSUER_KEY=srm-holding

addr() { "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null; }
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend test --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend test --home "$HOME_DIR" >/dev/null 2>&1
}

echo "== the sponsor account =="
ensure_key "$SPONSOR_KEY"
SPONSOR=$(addr "$SPONSOR_KEY")
echo "  $SPONSOR_KEY  $SPONSOR"

have=$("$BIN" query bank balances "$SPONSOR" $Q 2>/dev/null | python3 -c "
import sys,json
b=json.load(sys.stdin).get('balances',[])
print(next((c['amount'] for c in b if c['denom']=='uyml'),'0'))")
if [ "${have:-0}" = "0" ]; then
  echo "  funding it from alice"
  "$BIN" tx bank send "$(addr alice)" "$SPONSOR" 500000000uyml --from alice $TX >/dev/null 2>&1
  sleep 7
fi

echo
echo "== registering the dollar (permissionless) =="
# Six decimals, matching every other currency on this chain. A dollar with a
# different exponent from its neighbours is the kind of difference nobody
# notices until an amount is out by a thousand.
if "$BIN" query stablecoin get-approved-issuer uusd $Q >/dev/null 2>&1; then
  echo "  uusd already has an approved issuer; nothing to register"
else
  "$BIN" tx stablecoin register-currency uusd USD 6 "United States Dollar" USD \
    "Dollar settlement for tokenised holdings" --from "$ISSUER_KEY" $TX >/dev/null 2>&1 \
    && echo "  ok    application filed for uusd" \
    || echo "  note  register-currency refused; it may already be registered"
  sleep 7
fi

GOV=$("$BIN" query auth module-account gov $Q 2>/dev/null | python3 -c "
import sys,json;print(json.load(sys.stdin)['account']['value']['address'])")
[ -n "$GOV" ] || { echo 'no gov module account'; exit 1; }

echo
echo "== one proposal, two governance acts =="
cat > /tmp/timber-stage-a.json <<JSON
{
  "messages": [
    {
      "@type": "/blockchain.stablecoin.v1.MsgApproveIssuer",
      "authority": "$GOV",
      "denom": "uusd",
      "approve": true
    },
    {
      "@type": "/blockchain.tokenisation.v1.MsgCreateCollection",
      "authority": "$GOV",
      "collection": {
        "id": "ae-holdings",
        "authority": "$SPONSOR",
        "verification": "VERIFY_GOVERNANCE",
        "challenge_window_seconds": "604800",
        "dispute_bond_bps": 500
      }
    }
  ],
  "metadata": "timber-stage-a",
  "deposit": "2000000uyml",
  "title": "A dollar, and a registry for holding companies",
  "summary": "Two things a tokenised operating business needs that this chain does not have. First, a United States dollar: the nearest denomination on the chain is usdg, which is the Sudanese Pound, and a dollar-denominated vehicle settled in Sudanese pounds would be wrong in a way no interface could correct. Second, a collection for holding companies. The only collection is cd-terres, a Congolese land registry, and an Emirati company that collects the proceeds of a concession is not a parcel - minting it there would assert a relationship to registered land that does not exist and that the module would then enforce ceilings against. The collection verifies a reported sale through governance, with a seven-day challenge window and a 5% dispute bond. Attestors would be the better long-run answer - a sale price is a matter for people who can read the accounts - but VERIFY_ATTESTORS requires a register of at least two named attestors at creation, and no such parties exist yet. Appointing them is a later governance act that does not disturb anything already issued."
}
JSON

python3 -c "import json;json.load(open('/tmp/timber-stage-a.json'));print('  proposal json valid')"

BEFORE=$("$BIN" query gov proposals $Q 2>/dev/null | python3 -c "
import sys,json
try: print(max(int(p['id']) for p in json.load(sys.stdin)['proposals']))
except Exception: print(0)")

"$BIN" tx gov submit-proposal /tmp/timber-stage-a.json --from alice $TX >/dev/null 2>&1
sleep 8

"$BIN" query gov proposals $Q 2>/dev/null | python3 -c "
import sys, json
before = int('$BEFORE')
for p in json.load(sys.stdin)['proposals']:
    if int(p['id']) > before:
        print()
        print('  proposal', p['id'], p['status'])
        print('  voting ends', p.get('voting_end_time'))
        print()
        print('  The vote is yours:')
        print('    blockchaind tx gov vote %s yes --from alice --keyring-backend test \\\\' % p['id'])
        print('      --home /opt/yamale/node --chain-id yamale-devnet-2 \\\\')
        print('      --node http://127.0.0.1:26657 --gas auto --gas-adjustment 1.5 --fees 6000uyml -y')
"
