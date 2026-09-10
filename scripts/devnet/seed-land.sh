#!/usr/bin/env bash
#
# Puts parcels in the land register, because there are none.
#
# # Why this exists
#
# Checked 2026-09-10: the land module is deployed and completely empty. No
# genesis state, no seeding anywhere under scripts/. Every parcel query answers
# nothing, and the console renders that correctly — which is the trap, because
# an empty page looks identical to a working one. Nobody could tell, and nobody
# did.
#
# So there was nothing to demonstrate and no search term to hand anybody. This
# produces both.
#
# # Why Senegalese parcels and not lorem
#
# The PNDIES expression of interest goes to a Senegalese ministry, and the
# argument it makes about land — a registry that refuses a second title over
# ground somebody already holds — is the same argument it makes about livestock.
# A demonstrator showing Congolese test names while the letter talks about
# Senegal invites the obvious question. These are real places, in real
# jurisdictions, with references in a plausible cadastral form.
#
# Niacoulrab is deliberate: it is the address on the Ladoum birth certificate
# reproduced in the technical note. The two demonstrations meet there.
#
# # The three things this has to satisfy, none of them optional
#
#   1. A parcel may only be registered by an ACTIVE REGISTRY OFFICE.
#      RegisterParcel calls activeAuthority(creator) first.
#   2. An office must be an x/group POLICY ACCOUNT. A plain key is refused —
#      "an office that is one key is one bribe" — so the office is created as a
#      real group and every registration is a group decision.
#   3. Admitting an office is GOVERNANCE. RegisterAuthority asserts the gov
#      module account, so stage 1 is a proposal with a voting period, not a
#      transaction. There is no way to shortcut that, and it is the feature.
#
# The jurisdiction must also be an assigned ISO-3166 country. SN is.
#
# Usage:
#   scripts/devnet/seed-land.sh office     stage 1 — group + governance proposal
#   scripts/devnet/seed-land.sh parcels    stage 2 — once the office is admitted
#   scripts/devnet/seed-land.sh show       print the search values, change nothing
set -euo pipefail

BIN=${BIN:-/opt/yamale/bin/blockchaind}
HOME_DIR=${HOME_DIR:-/opt/yamale/node}
KEYRING=${KEYRING:-test}
CHAIN=${CHAIN:-yamale-devnet-2}
NODE=${NODE:-http://127.0.0.1:26657}

# Three keys that already exist and are funded on this chain. The office is
# 2-of-3 between them: enough that no single one of them can register a title,
# which is the property the technical note sells and therefore the property the
# demonstration has to actually have.
M1=${M1:-adm-ba}
M2=${M2:-adm-diallo}
M3=${M3:-adm-fall}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.4 -y"
Q="--home $HOME_DIR --node $NODE -o json"

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }

# ---------------------------------------------------------------- the parcels
#
# geometry_hash stands in for the hash of a surveyed boundary. Here it is the
# sha256 of a WKT point, which is stable, reproducible from this file, and
# unique per parcel — that last part matters, because a duplicate is exactly
# what the registry is built to refuse and a seeder that trips it looks like a
# bug rather than a demonstration.
#
# ref|locality|region|wkt|holder-key
PARCELS=$(cat <<'ROWS'
DK-ALM-2026-0147|Almadies|Dakar|POINT(-17.5069 14.7447)|citizen-mukendi
DK-NGO-2026-0148|Ngor|Dakar|POINT(-17.5147 14.7539)|citizen-mukendi
DK-OUA-2026-0151|Ouakam|Dakar|POINT(-17.4903 14.7264)|buyer-kalala
DK-YOF-2026-0163|Yoff|Dakar|POINT(-17.4739 14.7581)|investor-nsimba
DK-PTE-2026-0170|Point E|Dakar|POINT(-17.4614 14.6937)|sponsor-mwamba
RF-NIA-2026-0204|Niacoulrab|Rufisque|POINT(-17.2381 14.7644)|investor-nsimba
TH-THI-2026-0311|Thies Nord|Thies|POINT(-16.9246 14.7886)|buyer-kalala
SL-SLO-2026-0402|Saint-Louis Sud|Saint-Louis|POINT(-16.4889 16.0179)|citizen-mukendi
ZG-ZIG-2026-0518|Ziguinchor Centre|Ziguinchor|POINT(-16.2719 12.5681)|sponsor-mwamba
KL-KAO-2026-0627|Kaolack Ouest|Kaolack|POINT(-16.0726 14.1652)|investor-nsimba
ROWS
)

geom() { printf '%s' "$1" | sha256sum | cut -c1-64; }

# ---------------------------------------------------------------- stage 1
office_addr_file="$HOME_DIR/land-office-sn.addr"

stage_office() {
  say "1/3  the office as a 2-of-3 group"
  a1=$(addr "$M1"); a2=$(addr "$M2"); a3=$(addr "$M3")
  printf '  members: %s\n           %s\n           %s\n' "$a1" "$a2" "$a3"

  members=$(mktemp); trap 'rm -f "$members"' RETURN
  cat > "$members" <<JSON
{"members":[
  {"address":"$a1","weight":"1","metadata":"Cadastre SN — member 1"},
  {"address":"$a2","weight":"1","metadata":"Cadastre SN — member 2"},
  {"address":"$a3","weight":"1","metadata":"Cadastre SN — member 3"}
]}
JSON

  policy=$(mktemp); trap 'rm -f "$members" "$policy"' RETURN
  cat > "$policy" <<'JSON'
{"@type":"/cosmos.group.v1.ThresholdDecisionPolicy","threshold":"2",
 "windows":{"voting_period":"120s","min_execution_period":"0s"}}
JSON

  "$BIN" tx group create-group-with-policy "$a1" \
    "Direction du Cadastre — Senegal" "Registry office, jurisdiction SN" \
    "$members" "$policy" --group-policy-as-admin --from "$M1" $TX

  say "waiting for the policy address"
  sleep 8
  office=$("$BIN" query group group-policies-by-admin "$a1" $Q 2>/dev/null \
    | grep -oE '"address":"yml1[a-z0-9]+"' | head -1 | cut -d'"' -f4)
  [ -n "$office" ] || { echo "could not read the policy address; check the tx"; exit 1; }
  echo "$office" > "$office_addr_file"
  printf '  office: %s\n  saved to %s\n' "$office" "$office_addr_file"

  say "2/3  governance proposal to admit it"
  # RegisterAuthority asserts the gov module account. There is no other route,
  # and the voting period is the point rather than an obstacle.
  gov=$("$BIN" query auth module-account gov $Q | grep -oE '"address":"yml1[a-z0-9]+"' | head -1 | cut -d'"' -f4)
  prop=$(mktemp); trap 'rm -f "$members" "$policy" "$prop"' RETURN
  cat > "$prop" <<JSON
{
  "messages": [{
    "@type": "/blockchain.land.v1.MsgRegisterAuthority",
    "authority": "$gov",
    "office": "$office",
    "name": "Direction du Cadastre et de la Conservation Fonciere",
    "jurisdiction": "SN",
    "active": true
  }],
  "metadata": "",
  "deposit": "1000000uyml",
  "title": "Land: admit the Senegalese cadastral office",
  "summary": "Admits a 2-of-3 group account as the registry office for jurisdiction SN, so that parcels can be registered. The office is a group rather than a key because registering a title, validating a transfer and freezing land are each a single signature otherwise, and no amount of cross-office quorum on transfers repairs that."
}
JSON
  "$BIN" tx gov submit-proposal "$prop" --from "$M1" $TX

  cat <<TEXT

  Vote on it, then run:  $0 parcels

  The proposal needs a voter with STAKED tokens — a balance is not voting
  power. On this chain that is alice, or whichever account holds a delegation.
TEXT
}

# ---------------------------------------------------------------- stage 2
stage_parcels() {
  office=$(cat "$office_addr_file" 2>/dev/null || true)
  [ -n "$office" ] || { echo "no office address; run '$0 office' first"; exit 1; }

  say "3/3  registering parcels as the office"
  printf '  office: %s\n' "$office"

  n=0
  while IFS='|' read -r ref locality region wkt holder; do
    [ -n "$ref" ] || continue
    h=$(geom "$wkt")
    holder_addr=$(addr "$holder")

    # Each registration is a group proposal. --exec try executes it immediately
    # when the proposers already meet the threshold, so two members submitting
    # together is one command rather than three — without lowering the
    # threshold, which would defeat the point of the office being a group.
    msg=$(mktemp)
    cat > "$msg" <<JSON
{"messages":[{
  "@type":"/blockchain.land.v1.MsgRegisterParcel",
  "creator":"$office","geometry_hash":"$h","cadastral_ref":"$ref","holder":"$holder_addr"
}],
 "metadata":"$locality, $region",
 "proposers":["$(addr "$M1")","$(addr "$M2")"]}
JSON
    if "$BIN" tx group submit-proposal "$msg" --exec try --from "$M1" $TX >/dev/null 2>&1; then
      printf '  ok    %-18s %-18s %s\n' "$ref" "$locality" "$holder"
      n=$((n+1))
    else
      printf '  FAIL  %-18s %s\n' "$ref" "$locality"
    fi
    rm -f "$msg"
    sleep 2
  done <<< "$PARCELS"

  printf '\n  %d parcels registered\n' "$n"
  stage_show
}

# ---------------------------------------------------------------- what to type
stage_show() {
  say "what to search for"
  cat <<'TEXT'

  In the land console, or against the API. Any of these finds a parcel:

    DK-ALM-2026-0147     Almadies, Dakar
    RF-NIA-2026-0204     Niacoulrab, Rufisque  — the Ladoum certificate address
    SL-SLO-2026-0402     Saint-Louis Sud
    ZG-ZIG-2026-0518     Ziguinchor Centre

  By reference:
    curl -s "$PUBLIC/api/rest/yamale/blockchain/land/v1/parcel_by_ref?cadastral_ref=DK-ALM-2026-0147"

  By id — parcel 0 is never issued, so they start at 1:
    curl -s "$PUBLIC/api/rest/yamale/blockchain/land/v1/parcel/1"

  Everything one holder owns:
    curl -s "$PUBLIC/api/rest/yamale/blockchain/land/v1/parcels_by_holder/<address>"

  The office itself:
    curl -s "$PUBLIC/api/rest/yamale/blockchain/land/v1/authorities"

  PUBLIC=https://yamale.tail4355e8.ts.net  from off the tailnet;
  from either host use http://127.0.0.1:26657 or the Pi over the tailnet —
  MagicDNS resolves the funnel name to ingress the hosts cannot route to.

  The demonstration worth doing in a meeting is the REFUSAL: register a second
  parcel with a geometry hash that already exists and watch it be rejected.
  That is the whole argument of the technical note, in one command.

TEXT
}

case "${1:-}" in
  office)  stage_office ;;
  parcels) stage_parcels ;;
  show)    stage_show ;;
  *) echo "usage: $0 office|parcels|show" >&2; exit 2 ;;
esac
