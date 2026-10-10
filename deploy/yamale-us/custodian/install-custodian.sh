#!/usr/bin/env bash
# install-custodian.sh - stage the account custodian on yamale-us. Does NOT start it.
#
# The custodian holds one part of every account and the recovery service holds
# another; the two must never share a host, or that host can attempt a recovery
# alone. Recovery stays on the Pi. This moves the custodian off the VM.
#
# The sealing key and pepper are streamed host to host and never written to
# this machine. They are the VM's raw 32-byte files, read untouched by
# readSecret (d40b6a2). The account store is copied at the cutover, after the
# VM's custodian has stopped, so no enrolment lands on the old copy.
set -euo pipefail
VM="ubuntu@92.4.151.72"
VMKEY="$HOME/.ssh/yamale_oracle"
US="${CUSTODIAN_HOST:-yamale-us}"
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

ssh -i "$VMKEY" "$VM" 'cat /opt/yamale/bin/yamale-custodian' | ssh "$US" 'cat > /tmp/yamale-custodian'
ssh -i "$VMKEY" "$VM" 'cat /opt/yamale/bin/yamale-notify' | ssh "$US" 'cat > /tmp/yamale-notify'
for s in key pepper; do
  ssh -i "$VMKEY" "$VM" "sudo cat /etc/yamale/custodian.$s" \
    | ssh "$US" "T=\$(mktemp); cat > \$T; sudo install -d -m 755 /etc/yamale; id yamale-custodian >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/yamale/custodian --no-create-home --shell /usr/sbin/nologin yamale-custodian; sudo install -o yamale-custodian -g yamale-custodian -m 400 \$T /etc/yamale/custodian.$s; rm \$T"
done
tr -d '\r' < "$SRC/deploy/systemd/yamale-custodian.service" | ssh "$US" 'cat > /tmp/yamale-custodian.service'

ssh "$US" 'set -eo pipefail
sudo install -o root -g root -m 755 /tmp/yamale-custodian /opt/yamale/bin/yamale-custodian
sudo install -o root -g root -m 755 /tmp/yamale-notify /opt/yamale/bin/yamale-notify
rm /tmp/yamale-custodian /tmp/yamale-notify
for s in key pepper; do
  n=$(sudo stat -c %s /etc/yamale/custodian.$s)
  [ "$n" = 32 ] || { echo "custodian.$s is $n bytes, expected 32"; exit 1; }
done
sudo install -d -o yamale-custodian -g yamale-custodian -m 700 /var/lib/yamale /var/lib/yamale/custodian
sudo install -d -o yamale-custodian -g yamale-custodian -m 750 /var/spool/yamale-notices
printf "/var/spool/yamale-notices\n" | sudo tee /etc/yamale/notify.conf >/dev/null
sudo tee /etc/yamale/custodian.env >/dev/null <<ENV
# yamale-custodian on yamale-us. Recovery is on the Pi: never both on one host.
CUSTODIAN_ROLE=custodian
CUSTODIAN_DIR=/var/lib/yamale/custodian
CUSTODIAN_SEALING_KEY=/etc/yamale/custodian.key
CUSTODIAN_PEPPER=/etc/yamale/custodian.pepper
# Loopback: the chain gate on 127.0.0.1:8081 is the only thing that talks to it.
CUSTODIAN_LISTEN=127.0.0.1:8099
CUSTODIAN_NOTIFY_COMMAND=/opt/yamale/bin/yamale-notify
YAMALE_NOTIFY_DIR=/var/spool/yamale-notices
ENV
sudo chmod 640 /etc/yamale/custodian.env
sudo install -m 644 /tmp/yamale-custodian.service /etc/systemd/system/yamale-custodian.service
rm /tmp/yamale-custodian.service
sudo install -d /etc/systemd/system/yamale-custodian.service.d
printf "[Service]\nUser=yamale-custodian\nGroup=yamale-custodian\nReadWritePaths=/var/spool/yamale-notices\nMemoryMax=512M\n" | sudo tee /etc/systemd/system/yamale-custodian.service.d/host.conf >/dev/null
sudo systemctl daemon-reload
echo "staged; not started"
'
