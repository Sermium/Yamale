#!/usr/bin/env bash
# migrate-from-loyalty.sh - one-off: rename the loyalty service on yamale-us to traceability.
#
#   deploy/yamale-us/traceability/migrate-from-loyalty.sh --dry-run   # says what it would do
#   deploy/yamale-us/traceability/migrate-from-loyalty.sh             # does it
#
# Run it BEFORE install-traceability.sh, which then writes the new units over
# the renamed directories. It moves and renames; it deletes nothing. The old
# unit files are moved aside into /root/loyalty-units.prev-<date>. The sponsor
# key is renamed, not replaced, so its address and every fee allowance it
# granted stay as they are. On a host already migrated it says so and exits 0.
set -euo pipefail
HOST="${TRACEABILITY_HOST:-yamale-us}"
DRY=0
[ "${1:-}" = --dry-run ] && DRY=1
ssh "$HOST" "DRY=$DRY;"'set -euo pipefail
run() { if [ "$DRY" = 1 ]; then echo "would: $*"; else echo "+ $*"; sudo "$@"; fi; }
BIN=/opt/yamale/bin/blockchaind
NEWP=0; OLDP=0; LIST=""
chk() {
  c=$1; l=$2; shift 2
  if "$@" >/dev/null 2>&1; then
    LIST="$LIST  $l: present
"
    if [ "$c" = n ]; then NEWP=$((NEWP+1)); else OLDP=$((OLDP+1)); fi
  else
    LIST="$LIST  $l: absent
"
  fi
}
chk n "user yamale-traceability" id yamale-traceability
chk n "group yamale-traceability" getent group yamale-traceability
chk n "/opt/yamale/traceability" sudo test -e /opt/yamale/traceability
chk n "/var/lib/yamale-traceability" sudo test -e /var/lib/yamale-traceability
chk n "/etc/yamale/traceability-operator-token" sudo test -e /etc/yamale/traceability-operator-token
chk n "key traceability-sponsor" sudo -u yamale-traceability $BIN keys show traceability-sponsor -a --keyring-backend test --home /opt/yamale/traceability
chk o "user yamale-loyalty" id yamale-loyalty
chk o "group yamale-loyalty" getent group yamale-loyalty
chk o "/opt/yamale/loyalty" sudo test -e /opt/yamale/loyalty
chk o "/var/lib/yamale-loyalty" sudo test -e /var/lib/yamale-loyalty
chk o "/etc/yamale/loyalty-operator-token" sudo test -e /etc/yamale/loyalty-operator-token
if [ "$NEWP" = 6 ] && [ "$OLDP" = 0 ]; then echo "already migrated"; exit 0; fi
if [ "$NEWP" != 0 ] || [ "$OLDP" != 5 ]; then
  echo "half-migrated host: "
  printf "%s" "$LIST"
  echo "fix by hand; nothing was changed"
  exit 1
fi
for p in /opt/yamale/traceability /var/lib/yamale-traceability /var/backups/yamale-traceability /etc/yamale/traceability-operator-token; do
  if sudo test -e "$p"; then echo "refusing: $p already exists"; exit 1; fi
done
D=$(date -u +%Y%m%d%H%M)
run systemctl stop yamale-loyalty yamale-loyalty-backup.timer
run systemctl disable yamale-loyalty yamale-loyalty-backup.timer
run usermod -l yamale-traceability -d /opt/yamale/traceability yamale-loyalty
run groupmod -n yamale-traceability yamale-loyalty
run mv /opt/yamale/loyalty /opt/yamale/traceability
run mv /var/lib/yamale-loyalty /var/lib/yamale-traceability
run mv /var/backups/yamale-loyalty /var/backups/yamale-traceability
run mv /etc/yamale/loyalty-operator-token /etc/yamale/traceability-operator-token
if [ "$DRY" = 1 ]; then
  echo "would: keys rename loyalty-sponsor traceability-sponsor (in /opt/yamale/traceability)"
else
  sudo -u yamale-traceability $BIN keys rename loyalty-sponsor traceability-sponsor --keyring-backend test --home /opt/yamale/traceability -y >/dev/null
  echo "sponsor key renamed: $(sudo -u yamale-traceability $BIN keys show traceability-sponsor -a --keyring-backend test --home /opt/yamale/traceability)"
fi
run install -d -o root -g root -m 700 /root/loyalty-units.prev-$D
for u in yamale-loyalty.service yamale-loyalty-backup.service yamale-loyalty-backup.timer; do
  run mv /etc/systemd/system/$u /root/loyalty-units.prev-$D/
done
run systemctl daemon-reload
if [ -d /srv/loyalty ]; then run mv /srv/loyalty /srv/loyalty.prev-$D; fi
echo "migrated; next: install-traceability.sh --api, then deploy-gate.sh, then install-traceability.sh --app, then the platform deploy"
'
