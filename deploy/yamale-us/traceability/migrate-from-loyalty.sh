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
if ! id yamale-loyalty >/dev/null 2>&1 && id yamale-traceability >/dev/null 2>&1; then echo "already migrated"; exit 0; fi
id yamale-loyalty >/dev/null 2>&1 || { echo "no yamale-loyalty user: nothing to migrate"; exit 1; }
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
echo "migrated; now run install-traceability.sh, deploy-gate.sh, then the platform deploy"
'
