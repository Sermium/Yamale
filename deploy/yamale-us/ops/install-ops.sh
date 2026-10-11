#!/usr/bin/env bash
# install-ops.sh - move the ops signer for validator "pi" to yamale-us.
#
# scripts/devnet/opsd.py is the governance console's signing half: votes,
# freezes and recovery cases, signed as alice. It ran on the VM and was reached
# only through yamale.ddns.net, which no longer answers, so the console's
# buttons did nothing. On yamale-us it is reached as governance.yamaletech.com's
# /api/ops/, through the gate's private routes (/etc/yamale/gate/private, not in
# git: they inject the service credential), with the operator's console
# password in front.
#
# Its keyring holds alice alone, under its own user; the node's home holds the
# master validator's consensus key. The VM's copy is stopped in the same run.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
VM="ubuntu@92.4.151.72"
VMKEY="$HOME/.ssh/yamale_oracle"
US="${OPS_HOST:-yamale-us}"
HEX=1997d0e2d3dde8764d13a1b76dd46df0935b4c3c   # alice

T=$(mktemp -d)
trap 'rm -r "$T"' EXIT
ssh -i "$VMKEY" "$VM" 'cat /opt/yamale/node/keyring-test/alice.info' > "$T/alice.info"
ssh -i "$VMKEY" "$VM" "cat /opt/yamale/node/keyring-test/$HEX.address" > "$T/$HEX.address"
ssh -i "$VMKEY" "$VM" 'sudo cat /etc/yamale/opsd.env' > "$T/opsd.env"
tr -d '\r' < ../../../scripts/devnet/opsd.py > "$T/opsd.py"
grep -q '"pi":   {"key": "alice", "home": "/opt/yamale/ops"}' "$T/opsd.py" || { echo "opsd.py does not map pi to /opt/yamale/ops"; exit 1; }

tar czf - -C "$T" . | ssh "$US" "HEX=$HEX;"'set -eo pipefail
S=$(mktemp -d); trap "sudo rm -r \"\$S\"" EXIT
tar xzf - -C "$S"
H=/opt/yamale/ops
id yamale-ops >/dev/null 2>&1 || sudo useradd --system --home-dir $H --no-create-home --shell /usr/sbin/nologin yamale-ops
sudo install -o root -g root -m 755 "$S/opsd.py" /opt/yamale/bin/opsd.py
sudo install -d -o yamale-ops -g yamale-ops -m 700 $H $H/keyring-test
sudo install -o yamale-ops -g yamale-ops -m 600 "$S/alice.info" $H/keyring-test/alice.info
sudo install -o yamale-ops -g yamale-ops -m 600 "$S/$HEX.address" $H/keyring-test/$HEX.address
sudo install -o root -g root -m 640 "$S/opsd.env" /etc/yamale/opsd.env
A=$(sudo -u yamale-ops /opt/yamale/bin/blockchaind keys show alice -a --keyring-backend test --home $H)
[ "$A" = yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg ] || { echo "ops keyring does not hold alice: $A"; exit 1; }
sudo tee /etc/systemd/system/yamale-ops.service >/dev/null <<UNIT
[Unit]
Description=Yamale ops service - signs for validator pi (governance console)
After=network-online.target yamale-devnet.service
Wants=network-online.target

[Service]
User=yamale-ops
Group=yamale-ops
ExecStart=/usr/bin/python3 /opt/yamale/bin/opsd.py
Environment=OPSD_VALIDATOR=pi
Environment=OPSD_BIND=127.0.0.1
EnvironmentFile=/etc/yamale/opsd.env
Restart=always
RestartSec=5
MemoryMax=128M
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$H

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload
echo staged
'
ssh -i "$VMKEY" "$VM" 'sudo systemctl disable --now yamale-ops 2>&1 | tail -1; echo "VM ops: $(systemctl is-active yamale-ops)"'
ssh "$US" 'sudo systemctl enable --now yamale-ops 2>&1 | tail -1; sleep 2; echo "yamale-us ops: $(systemctl is-active yamale-ops)"; sudo ss -ltnp | grep 8086'
