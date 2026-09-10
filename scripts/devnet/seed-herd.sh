#!/usr/bin/env bash
#
# Puts a Ladoum flock into the national livestock register.
#
# # What this demonstrates, and why it needs this much setup
#
# A Ladoum birth certificate issued today carries a line reading
# "Classico / Atomique / Palmarin". Those are names in free text. Nobody can
# walk them and — the part that matters — nobody can CONTRADICT them. A breeder
# can declare a sire that was dead, a dam that gave birth a fortnight earlier,
# or an ancestor that is the animal itself, and the document looks exactly as
# good as a true one.
#
# So the demonstration worth doing is not "look, a record on a chain". It is
# the REFUSAL. Stage 4 below tries to register four impossible pedigrees and is
# rejected four times, by consensus, in front of whoever is watching. That is
# the entire argument of the technical note, in four commands.
#
# # Why there are TWO offices
#
# Because a transfer needs an attestor who does not answer to the office that
# holds the animal's record. x/herd refuses the other arrangement outright:
# an attestor from the recording office collapses the four-party shape back to
# one office deciding, which is the arrangement the quorum exists to replace.
# A seeder with one office can register animals and can never demonstrate a
# lawful sale, which is half the point.
#
# # The four things this has to satisfy, none of them optional
#
#   1. An animal may only be recorded by an ACTIVE REGISTRY OFFICE.
#   2. An office must be an x/group POLICY ACCOUNT. A plain key is refused —
#      "an office that is one key is one bribe" — so each office is a real
#      group and every registration is a group decision.
#   3. Admitting an office is GOVERNANCE. MsgRegisterAuthority asserts the gov
#      module account, so stage 1 ends in a proposal with a voting period.
#      There is no way to shortcut that, and it is the feature.
#   4. The jurisdiction must be an assigned ISO-3166 country. SN is.
#
# # Usage
#
#   scripts/devnet/seed-herd.sh offices     stage 1 — two groups + one proposal
#   scripts/devnet/seed-herd.sh flock       stage 2 — herd, animals, pedigree
#   scripts/devnet/seed-herd.sh sale        stage 3 — a supervised transfer
#   scripts/devnet/seed-herd.sh refusals    stage 4 — the four impossible lines
#   scripts/devnet/seed-herd.sh show        print the search values, change nothing
#
# Against the local devnet from scripts/devnet/init-local-devnet.sh:
#
#   BIN=./blockchaind.exe HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 \
#   NODE=tcp://127.0.0.1:26957 M1=custodian1 M2=custodian2 M3=custodian3 \
#   VOTER=validator scripts/devnet/seed-herd.sh offices
set -euo pipefail

BIN=${BIN:-/opt/yamale/bin/blockchaind}
HOME_DIR=${HOME_DIR:-/opt/yamale/node}
KEYRING=${KEYRING:-test}
CHAIN=${CHAIN:-yamale-devnet-2}
NODE=${NODE:-http://127.0.0.1:26657}

# Three keys that already exist and are funded. Each office is 2-of-3 between
# them: enough that no single one can record an animal or certify a line, which
# is the property the technical note sells and therefore the property the
# demonstration has to actually have.
M1=${M1:-adm-ba}
M2=${M2:-adm-diallo}
M3=${M3:-adm-fall}

# Who holds the animals, and who buys one in stage 3.
KEEPER=${KEEPER:-citizen-mukendi}
BUYER=${BUYER:-buyer-kalala}

# The two accredited veterinarians. Plain keys, and that is the point: an
# attestation is one named person saying they stood in front of the animal, and
# a field vet in a pastoral zone is a person with a phone, not an institution.
# Two of them because the quorum is two — one agent signing twice is one agent.
VET1=${VET1:-vet-sow}
VET2=${VET2:-vet-diouf}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"

say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }
# grep exits 1 when it matches nothing, and under `set -o pipefail` that kills
# the script. Matching nothing is a normal answer here — no group policies
# yet, no transfers yet, no proposals yet — so each of these swallows it and
# returns empty. This cost a run: the very first thing stage 1 does is count
# the policies that already exist, and on a fresh chain that is zero.
# The `[[:space:]]*` after each colon is not defensive padding. `-o json` is
# PRETTY-PRINTED here — `"id": "1"`, with a space — while the same flag on the
# deployment emits it compact. A pattern written for one silently matches
# nothing on the other, and "matches nothing" is indistinguishable from "the
# register has no such thing", which is the failure this whole script exists to
# make impossible to confuse with an answer.
json() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
# The address of the group policy created most recently with a given metadata
# string.
#
# NOT `group-policies-by-admin`. These groups are created with
# --group-policy-as-admin, which makes each policy its own admin — so querying
# by the creator's address returns nothing at all, which is what the first
# attempt at this script did, silently, twice. Matching on the metadata written
# at creation is exact where "the last two policies" was positional guesswork,
# and it survives somebody else creating a group in between.
policy_named() {
  local want=$1 found=""
  local ids; ids=$("$BIN" query group groups $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | cut -d'"' -f4 || true)
  for gid in $ids; do
    local out; out=$("$BIN" query group group-policies-by-group "$gid" $Q 2>/dev/null || true)
    if printf '%s' "$out" | grep -qE "\"metadata\":[[:space:]]*\"$want\""; then
      found=$(printf '%s' "$out" | grep -oE '"address":[[:space:]]*"yml1[a-z0-9]+"' \
        | head -1 | cut -d'"' -f4 || true)
    fi
  done
  printf '%s' "$found"
}

# The veterinarians' keys, created on demand. A key that does not exist yet is
# the ordinary case on a fresh devnet, and a seeder that stopped to complain
# about it would be a seeder nobody runs twice.
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1
}

REGISTRY_FILE="$HOME_DIR/herd-office-sn.addr"
VETS_FILE="$HOME_DIR/herd-vets-sn.addr"

# --------------------------------------------------------------- the flock
#
# Real Ladoum naming, and the national identifier in the format the filière
# already writes: 221 is Senegal's calling code, 682 the herd book, then the
# animal's serial and L for Ladoum. Transponders are ISO 11784/11785, fifteen
# digits, 982 being a manufacturer prefix.
#
# The line is deliberately coherent AND deep enough to be worth walking: four
# generations, with one shared grandsire so the graph is a lattice rather than
# a tree — which is what a real closed stud looks like and what an inbreeding
# coefficient is computed over.
#
# name|national_id|tag|sex|born_on|coat|birth_type|sire|dam
FLOCK=$(cat <<'ROWS'
Palmarin|221/682/000101/L|982000000000101|1|2018-03-12|blanc|single||
Nianing|221/682/000102/L|982000000000102|2|2018-05-20|fauve|single||
Somone|221/682/000103/L|982000000000103|2|2018-09-02|blanc tacheté|single||
Atomique|221/682/000201/L|982000000000201|1|2020-01-10|fauve|single|Palmarin|Nianing
Ndiaga|221/682/000202/L|982000000000202|1|2020-04-18|blanc|twin|Palmarin|Somone
Khadija|221/682/000203/L|982000000000203|2|2020-04-18|blanc|twin|Palmarin|Somone
Ndeye|221/682/000204/L|982000000000204|2|2021-02-11|noir|single|Palmarin|Nianing
Classico|221/682/021029/L|982000000021029|1|2023-06-14|blanc tacheté|single|Atomique|Ndeye
Mame Diarra|221/682/021030/L|982000000021030|2|2023-11-03|fauve|single|Ndiaga|Khadija
Bathie|221/682/031044/L|982000000031044|1|2025-08-25|blanc|single|Classico|Mame Diarra
ROWS
)

# --------------------------------------------------------------- stage 1
stage_offices() {
  say "1/4  two offices, each a 2-of-3 group"
  ensure_key "$VET1"; ensure_key "$VET2"

  # A key with no account on chain cannot sign anything, whatever the gas price
  # is: there is no account number to sign against. So the veterinarians are
  # funded once, here, from a member key that already has coins. On a
  # deployment where they are real people with real accounts this does nothing.
  for v in "$VET1" "$VET2"; do
    va=$(addr "$v")
    have=$("$BIN" query bank balances "$va" $Q 2>/dev/null | json amount)
    [ -n "$have" ] && continue
    "$BIN" tx bank send "$(addr "$M3")" "$va" 10000000uyml --from "$M3" $TX >/dev/null 2>&1       && printf '  funded %s
' "$v" || printf '  could not fund %s
' "$v"
    sleep 3
  done
  a1=$(addr "$M1"); a2=$(addr "$M2"); a3=$(addr "$M3")
  printf '  members: %s\n           %s\n           %s\n' "$a1" "$a2" "$a3"

  members="$HOME_DIR/.herd-members.json"
  cat > "$members" <<JSON
{"members":[
  {"address":"$a1","weight":"1","metadata":"member 1"},
  {"address":"$a2","weight":"1","metadata":"member 2"},
  {"address":"$a3","weight":"1","metadata":"member 3"}
]}
JSON

  policy="$HOME_DIR/.herd-policy.json"
  cat > "$policy" <<'JSON'
{"@type":"/cosmos.group.v1.ThresholdDecisionPolicy","threshold":"2",
 "windows":{"voting_period":"120s","min_execution_period":"0s"}}
JSON

  # The metadata is how each policy is found again afterwards, so it is part of
  # the interface rather than a label.
  REGISTRY_META="Registre de l'elevage, juridiction SN"
  VETS_META="Attestateurs accredites, juridiction SN"

  "$BIN" tx group create-group-with-policy "$a1" \
    "Direction de l'Elevage — Senegal" "$REGISTRY_META" \
    "$members" "$policy" --group-policy-as-admin --from "$M1" $TX >/dev/null
  sleep 6
  "$BIN" tx group create-group-with-policy "$a1" \
    "Ordre des Veterinaires — Senegal" "$VETS_META" \
    "$members" "$policy" --group-policy-as-admin --from "$M1" $TX >/dev/null
  sleep 6

  registry=$(policy_named "$REGISTRY_META")
  vets=$(policy_named "$VETS_META")
  [ -n "$registry" ] && [ -n "$vets" ] || {
    echo "could not find both group policies; check the two transactions above" >&2
    exit 1
  }
  echo "$registry" > "$REGISTRY_FILE"
  echo "$vets" > "$VETS_FILE"
  printf '  registry office: %s\n  attestor office: %s\n' "$registry" "$vets"

  say "2/4  one governance proposal admitting both"
  # MsgRegisterAuthority asserts the gov module account. There is no other
  # route, and the voting period is the point rather than an obstacle. Both
  # offices go in one proposal so there is one vote to carry rather than two.
  gov=$("$BIN" query auth module-account gov $Q | json address)
  prop="$HOME_DIR/.herd-proposal.json"
  cat > "$prop" <<JSON
{
  "messages": [
    {
      "@type": "/blockchain.herd.v1.MsgRegisterAuthority",
      "authority": "$gov",
      "office": "$registry",
      "name": "Direction de l'Elevage et des Productions Animales",
      "jurisdiction": "SN",
      "active": true
    },
    {
      "@type": "/blockchain.herd.v1.MsgRegisterAuthority",
      "authority": "$gov",
      "office": "$vets",
      "name": "Ordre National des Docteurs Veterinaires du Senegal",
      "jurisdiction": "SN",
      "active": true
    }
  ],
  "metadata": "",
  "deposit": "1000000uyml",
  "title": "Herd: admit the Senegalese livestock registry and its attestors",
  "summary": "Admits two 2-of-3 group accounts for jurisdiction SN: the registry office that records herds, identifies animals and certifies parentage, and the veterinary order whose members attest that the animal presented at a sale is the animal on the record. They are separate deliberately — x/herd refuses an attestation from the office holding the record, because an attestor who answers to the recording office collapses the four-party transfer back to one office deciding, which is the arrangement the quorum exists to replace. Both are group accounts rather than keys because recording an animal, certifying its parentage and striking it off are each a single bribe otherwise, and no amount of cross-office quorum on transfers repairs a register whose entries were wrong when written."
}
JSON
  "$BIN" tx gov submit-proposal "$prop" --from "$M1" $TX >/dev/null
  sleep 6
  id=$("$BIN" query gov proposals $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)

  cat <<TEXT

  Proposal ${id:-<see 'query gov proposals'>} submitted.

  Vote on it, then run:  $0 flock

  Stage 2 also accredits two veterinarians against the attestor office, which
  is the office's own act and needs no further vote.

    $BIN tx gov vote ${id:-<id>} yes --from \$VOTER $TX

  The voter needs STAKED tokens — a balance is not voting power.
TEXT
}

# --------------------------------------------------------------- helpers
office() {
  local f=$1 what=$2
  local a; a=$(cat "$f" 2>/dev/null || true)
  [ -n "$a" ] || { echo "no $what address; run '$0 offices' first" >&2; exit 1; }
  printf '%s' "$a"
}

# Submit one message AS an office, and carry it over the 2-of-3 threshold.
#
# Three transactions, not one, and every attempt to make it fewer failed in a
# way worth recording — each looked correct and each produced a different lie.
#
#   1. Both members as `proposers` with `--exec try`:
#        wrong number of signers; expected 2, got 1: unauthorized
#      Listing a proposer claims their signature, so a two-proposer proposal is
#      a two-signer TRANSACTION, and the CLI signs with one key.
#
#   2. Omitting `group_policy_address` from the document:
#        request account address of group policy: empty address string
#      The messages name the office as their `creator`, which looks like enough
#      and is not: the document must say which policy is deciding.
#
#   3. Proposing and then having only M2 vote:
#        the proposal simply sat at PROPOSAL_STATUS_SUBMITTED, yes_count 0.
#      Submitting does NOT record the proposer's vote. Nothing errors — the
#      transaction succeeds, the vote succeeds, and the message never executes.
#      That is the dangerous one, because a seeder that reported success off
#      the transaction result would have printed "ok" for every animal and
#      registered none.
#
# Hence: propose, both members vote, and the SECOND vote carries `--exec try`
# so the message runs the instant the threshold is met. The threshold is not an
# obstacle to work around — lowering it to 1 would delete the property this
# demonstration exists to show.
as_office() {
  local body=$1 note=$2 office=$3
  local msg="$HOME_DIR/.herd-msg.json"
  cat > "$msg" <<JSON
{"group_policy_address":"$office",
 "messages":[$body],
 "metadata":"$note",
 "proposers":["$(addr "$M1")"]}
JSON
  "$BIN" tx group submit-proposal "$msg" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 3

  local id
  id=$("$BIN" query group proposals-by-group-policy "$office" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)
  [ -n "$id" ] || return 1

  "$BIN" tx group vote "$id" "$(addr "$M1")" VOTE_OPTION_YES "" \
    --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 2
  "$BIN" tx group vote "$id" "$(addr "$M2")" VOTE_OPTION_YES "" --exec try \
    --from "$M2" $TX >/dev/null 2>&1 || return 1

  # The result is read from the proposal, never from the transaction — and the
  # proposal's ABSENCE is what success looks like.
  #
  # Three things had to be learnt here, and each one flipped the answer.
  #
  # First: a group proposal whose message the chain then REFUSES is still a
  # successful transaction. The vote landed, execution was attempted. Reading
  # the tx result would report every refusal in stage 4 as an acceptance, which
  # is the exact inversion of what that stage exists to show.
  #
  # Second: x/group PRUNES a proposal once it executes successfully. So the
  # obvious follow-up — query the proposal, check executor_result is SUCCESS —
  # finds nothing and concludes failure. This script printed FAIL for a herd
  # and ten animals that were all, in fact, on the register.
  #
  # Third: a fixed sleep before looking is a coin toss. One animal in ten
  # reported FAIL while sitting on the register, because the check ran in the
  # gap between the vote being accepted and the execution being committed. So
  # this waits for an ANSWER rather than for a duration — and a seeder that
  # reports what the register actually holds is the whole point of a seeder
  # whose subject is a register.
  local proposal result
  for _ in $(seq 1 20); do
    proposal=$("$BIN" query group proposal "$id" $Q 2>/dev/null || true)
    [ -n "$proposal" ] || return 0                      # pruned: it executed
    result=$(printf '%s' "$proposal" | json executor_result)
    case "$result" in
      PROPOSAL_EXECUTOR_RESULT_SUCCESS) return 0 ;;
      PROPOSAL_EXECUTOR_RESULT_FAILURE) return 1 ;;     # the chain refused it
    esac
    sleep 1
  done
  return 1                                              # never reached a verdict
}

animal_id() {
  "$BIN" query herd animal-by-id "$1" $Q 2>/dev/null | json id
}

# --------------------------------------------------------------- stage 2
stage_flock() {
  registry=$(office "$REGISTRY_FILE" "registry office")
  keeper=$(addr "$KEEPER")
  say "3/4  the herd, its animals, and a coherent line"
  printf '  office: %s\n  keeper: %s\n' "$registry" "$keeper"

  as_office "{\"@type\":\"/blockchain.herd.v1.MsgEnrolHerd\",
    \"creator\":\"$registry\",\"reference\":\"SN-RF-H-000147\",\"keeper\":\"$keeper\",
    \"species\":\"SPECIES_SHEEP\",\"declared_size\":40,\"locality\":\"Niacoulrab, Rufisque\",
    \"location_hash\":\"\"}" "Bergerie de Niacoulrab" "$registry" \
    && echo "  ok    herd SN-RF-H-000147" || echo "  FAIL  herd"
  sleep 3

  herd=$("$BIN" query herd herd-by-ref SN-RF-H-000147 $Q 2>/dev/null | json id)
  [ -n "$herd" ] || { echo "  the herd was not recorded; is the office admitted yet?" >&2; exit 1; }
  printf '  herd id: %s\n\n' "$herd"

  # Pass one: identify every animal. Parentage cannot be certified until both
  # parents are records, so the two passes are not an optimisation — a single
  # pass would fail on the first animal whose sire is later in the file.
  while IFS='|' read -r name nid tag sex born coat birth sire dam; do
    [ -n "$name" ] || continue
    sexname=$([ "$sex" = "1" ] && echo SEX_MALE || echo SEX_FEMALE)
    if as_office "{\"@type\":\"/blockchain.herd.v1.MsgIdentifyAnimal\",
      \"creator\":\"$registry\",\"herd_id\":\"$herd\",\"national_id\":\"$nid\",
      \"tag\":\"$tag\",\"name\":\"$name\",\"sex\":\"$sexname\",\"coat\":\"$coat\",
      \"born_on\":\"$born\",\"birth_type\":\"$birth\"}" "$name" "$registry"; then
      printf '  ok    %-14s %-20s %s\n' "$name" "$nid" "$born"
    else
      printf '  FAIL  %-14s %s\n' "$name" "$nid"
    fi
    sleep 2
  done <<< "$FLOCK"

  # Pass two: certify the ones whose parents are now on the register.
  printf '\n'
  while IFS='|' read -r name nid tag sex born coat birth sire dam; do
    [ -n "$name" ] || continue
    [ -n "$sire$dam" ] || continue
    id=$(animal_id "$nid")
    sire_nid=$(printf '%s' "$FLOCK" | grep "^$sire|" | cut -d'|' -f2 || true)
    dam_nid=$(printf '%s' "$FLOCK" | grep "^$dam|" | cut -d'|' -f2 || true)
    sire_id=$([ -n "$sire_nid" ] && animal_id "$sire_nid" || echo 0)
    dam_id=$([ -n "$dam_nid" ] && animal_id "$dam_nid" || echo 0)

    # The DNA hash stands in for a laboratory report. Reproducible from this
    # file so the demonstration is repeatable, and clearly not a real profile.
    dna=$(printf 'ladoum-dna-%s' "$nid" | sha256sum | cut -c1-64)
    if as_office "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",
      \"creator\":\"$registry\",\"animal_id\":\"$id\",\"sire_id\":\"$sire_id\",
      \"dam_id\":\"$dam_id\",\"dna_hash\":\"$dna\",
      \"photo_hash\":\"$(printf 'photo-%s' "$nid" | sha256sum | cut -c1-64)\"}" "$name" "$registry"; then
      printf '  ok    %-14s certified: %s x %s\n' "$name" "${sire:-—}" "${dam:-—}"
    else
      printf '  FAIL  %-14s certification refused\n' "$name"
    fi
    sleep 2
  done <<< "$FLOCK"

  # The two veterinarians, accredited by the ATTESTOR office.
  #
  # Not by governance, and not as offices. Attestation required the attestor to
  # be an admitted Authority until this seeder tried to run a sale and could
  # not: an Authority must be an x/group account behind a governance proposal,
  # so two attestations meant two such institutions had to exist before one
  # animal could lawfully change hands. Accreditation is the veterinary order's
  # own act, which is also how it works on paper.
  printf '\n'
  vets=$(office "$VETS_FILE" "attestor office")
  for v in "$VET1" "$VET2"; do
    va=$(addr "$v" 2>/dev/null || true)
    if [ -z "$va" ]; then
      printf '  --    %-14s no such key; create it or set VET1/VET2\n' "$v"
      continue
    fi
    if as_office "{\"@type\":\"/blockchain.herd.v1.MsgAccreditAgent\",
      \"authority\":\"$vets\",\"agent\":\"$va\",
      \"name\":\"Dr $v\",\"active\":true}" "accreditation $v" "$vets"; then
      printf '  ok    %-14s accredited as an attestor\n' "$v"
    else
      printf '  FAIL  %-14s accreditation refused\n' "$v"
    fi
  done

  # A vaccination campaign, which is what turns doses into coverage.
  printf '\n'
  as_office "{\"@type\":\"/blockchain.herd.v1.MsgRecordHealthEvent\",
    \"agent\":\"$registry\",\"herd_id\":\"$herd\",\"disease\":\"PPR\",
    \"batch\":\"PPR-2026-0431\",\"treated\":38}" "campagne PPR" "$registry" \
    && echo "  ok    PPR campaign, 38 of 40 treated" || echo "  FAIL  health event"

  stage_show
}

# --------------------------------------------------------------- stage 3
stage_sale() {
  registry=$(office "$REGISTRY_FILE" "registry office")
  vets=$(office "$VETS_FILE" "attestor office")
  keeper=$(addr "$KEEPER"); buyer=$(addr "$BUYER")

  say "a supervised sale: Bathie, from the keeper to a buyer"
  id=$(animal_id "221/682/031044/L")
  [ -n "$id" ] || { echo "  Bathie is not on the register; run '$0 flock' first" >&2; exit 1; }
  printf '  animal %s, %s -> %s\n' "$id" "$keeper" "$buyer"

  "$BIN" tx herd propose-transfer "$id" "$buyer" 850000 --from "$KEEPER" $TX >/dev/null \
    && echo "  ok    proposed by the keeper" || echo "  FAIL  proposal"
  sleep 3

  tid=$("$BIN" query herd provenance "$id" $Q 2>/dev/null | json id)
  [ -n "$tid" ] || { echo "  no transfer recorded" >&2; exit 1; }

  "$BIN" tx herd accept-transfer "$tid" --from "$BUYER" $TX >/dev/null \
    && echo "  ok    accepted by the buyer" || echo "  FAIL  acceptance"
  sleep 3

  # Two attestations, from two veterinarians accredited by the VETERINARY
  # office — not by the registry that holds the record. The chain refuses the
  # other way round, and it refuses a second attestation from the same person:
  # one agent signing twice is one agent, not a quorum.
  #
  # Each vet signs with their own plain key. No group proposal, no vote: an
  # attestation is one person's statement that they looked at the animal, and
  # making it M-of-N would not make it more true.
  for v in "$VET1" "$VET2"; do
    if "$BIN" tx herd attest-transfer "$tid" \
      "$(printf 'tag-read-%s-%s' "$id" "$v" | sha256sum | cut -c1-64)" \
      --from "$v" $TX >/dev/null 2>&1; then
      printf '  ok    attested by %s\n' "$v"
    else
      printf '  FAIL  %s could not attest\n' "$v"
    fi
    sleep 3
  done

  cat <<TEXT

  The transfer is attested and the challenge window is running. It is seven
  days, deliberately: livestock changes hands at a market on a Tuesday and a
  fortnight's wait would simply be ignored — a window everybody routes around
  protects nobody.

  While it runs, ANYBODY may stop this sale:

    $BIN tx herd object-transfer $tid "ce belier est le mien, vole en juillet" \\
      --from <any-key> $TX

  That openness is the design. The person best placed to recognise a stolen
  animal is the neighbour who has seen it every day, not a party to the sale.
  An objection is then answered by the office — upheld or dismissed, with a
  reason, on the record:

    $BIN tx herd resolve-objection $tid false "aucune preuve produite" ...

  After the window, anybody at all may pay the gas to complete it:

    $BIN tx herd complete-transfer $tid --from <any-key> $TX

TEXT
}

# --------------------------------------------------------------- stage 4
stage_refusals() {
  registry=$(office "$REGISTRY_FILE" "registry office")
  say "4/4  the four impossible declarations, refused by consensus"
  cat <<'TEXT'

  This is the demonstration. Each line below is one a paper certificate would
  carry without complaint, because paper cannot contradict anything written on
  it. Every one is rejected here.

TEXT

  classico=$(animal_id "221/682/021029/L")     # male,   born 2023-06-14
  ndeye=$(animal_id "221/682/000204/L")        # female, born 2021-02-11
  khadija=$(animal_id "221/682/000203/L")      # female, born 2020-04-18
  bathie=$(animal_id "221/682/031044/L")       # male,   born 2025-08-25
  palmarin=$(animal_id "221/682/000101/L")     # male,   born 2018-03-12
  nianing=$(animal_id "221/682/000102/L")      # female, born 2018-05-20

  [ -n "$classico$ndeye$bathie$palmarin" ] || {
    echo "  the flock is not on the register; run '$0 flock' first" >&2; exit 1; }

  dna=$(printf 'refusal-probe' | sha256sum | cut -c1-64)
  try() {
    local what=$1 body=$2 because=$3
    if as_office "$body" "refusal probe" "$registry"; then
      printf '  \033[31mACCEPTED\033[0m  %s — this should not have happened\n' "$what"
    else
      printf '  refused   %-38s %s\n' "$what" "$because"
    fi
    sleep 2
  }

  try "a dam that is male" \
    "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$registry\",
      \"animal_id\":\"$bathie\",\"sire_id\":\"$palmarin\",\"dam_id\":\"$classico\",
      \"dna_hash\":\"$dna\",\"photo_hash\":\"\"}" \
    "Classico is a ram"

  try "a sire born after its own lamb" \
    "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$registry\",
      \"animal_id\":\"$palmarin\",\"sire_id\":\"$bathie\",\"dam_id\":\"0\",
      \"dna_hash\":\"$dna\",\"photo_hash\":\"\"}" \
    "Bathie is seven years younger"

  # Nianing already has Atomique, born 2020-01-10. Khadija was born 99 days
  # later, and 99 is inside the 140-day minimum — which sits below a ewe's
  # roughly 147-day gestation, so a real interval is never refused while a
  # fabricated one is.
  #
  # The first version of this probe named a pair 299 days apart and was
  # ACCEPTED, correctly. A demonstration that shows a refusal which was never
  # due is worse than showing nothing: it teaches the room that the register
  # refuses things arbitrarily, which is the opposite of the argument.
  try "one ewe, two births 99 days apart" \
    "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$registry\",
      \"animal_id\":\"$khadija\",\"sire_id\":\"$palmarin\",\"dam_id\":\"$nianing\",
      \"dna_hash\":\"$dna\",\"photo_hash\":\"\"}" \
    "Nianing had Atomique 99 days before"

  try "a certification with no laboratory" \
    "{\"@type\":\"/blockchain.herd.v1.MsgCertifyAnimal\",\"creator\":\"$registry\",
      \"animal_id\":\"$bathie\",\"sire_id\":\"$classico\",\"dam_id\":\"$ndeye\",
      \"dna_hash\":\"\",\"photo_hash\":\"\"}" \
    "without one the parentage is the breeder's word"

  cat <<'TEXT'

  None of these proves a pedigree true — only a laboratory does that. What each
  one does is refuse a pedigree that CANNOT be true, and a register that refuses
  the impossible is worth more than one that records everything.

TEXT
}

# --------------------------------------------------------------- what to type
stage_show() {
  say "what to search for"
  cat <<'TEXT'

  In the Ladoum console, or against the API. Any of these finds an animal:

    221/682/021029/L     Classico  — the identifier on the reproduced certificate
    982000000021029      the same animal, by transponder
    221/682/031044/L     Bathie    — four generations of line above him
    SN-RF-H-000147       the herd, at Niacoulrab

  The endpoint the paper certificate cannot have — walk the line:

    blockchaind query herd pedigree <animal-id> 4

  What the herd was treated for, which is coverage rather than a dose count:

    blockchaind query herd health <herd-id>

  An animal's provenance, refused sales included:

    blockchaind query herd provenance <animal-id>

  The two offices, and why there are two:

    blockchaind query herd authorities

  The demonstration worth doing in a meeting is `refusals`. Four declarations
  that a paper certificate would carry without complaint, rejected by consensus
  in front of the room.

TEXT
}

case "${1:-}" in
  offices)  stage_offices ;;
  flock)    stage_flock ;;
  sale)     stage_sale ;;
  refusals) stage_refusals ;;
  show)     stage_show ;;
  *) echo "usage: $0 offices|flock|sale|refusals|show" >&2; exit 2 ;;
esac
