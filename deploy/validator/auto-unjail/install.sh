#!/usr/bin/env bash
# install.sh - install yamale-auto-unjail on a validator's host.
#
#   VALIDATOR_HOST="-p 2222 zubulmuk92@100.68.207.17" \
#   AU_VALOPER=ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j \
#   bash deploy/validator/auto-unjail/install.sh
#
# Creates the host's "unjailer" key (test keyring, its own user, nothing else in
# it), installs the script and a two-minute timer, and prints the two things it
# cannot do itself: fund the unjailer, and the grant the OPERATOR must sign -
# authz for MsgUnjail only. Until the grant exists every unjail attempt fails
# harmlessly with "authorization not found".
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
: "${VALIDATOR_HOST:?ssh target, e.g. -p 2222 user@host}"
: "${AU_VALOPER:?the validator operator address (ymlvaloper1...)}"
CHAIN_ID="${AU_CHAIN_ID:-yamale-devnet-2}"

tr -d '\r' < yamale-auto-unjail | ssh $VALIDATOR_HOST 'cat > /tmp/yamale-auto-unjail'
ssh $VALIDATOR_HOST "AU_VALOPER=$AU_VALOPER CHAIN_ID=$CHAIN_ID"' bash -s' <<'REMOTE'
set -euo pipefail
B=/opt/yamale/bin/blockchaind
H=/opt/yamale/auto-unjail
sudo install -o root -g root -m 755 /tmp/yamale-auto-unjail /usr/local/sbin/yamale-auto-unjail
rm /tmp/yamale-auto-unjail
id yamale-unjail >/dev/null 2>&1 || sudo useradd --system --home-dir $H --no-create-home --shell /usr/sbin/nologin yamale-unjail
sudo install -d -o yamale-unjail -g yamale-unjail -m 700 $H
if ! sudo -u yamale-unjail $B keys show unjailer --keyring-backend test --home $H >/dev/null 2>&1; then
  sudo -u yamale-unjail $B keys add unjailer --keyring-backend test --home $H >/dev/null 2>&1
fi
GRANTEE=$(sudo -u yamale-unjail $B keys show unjailer -a --keyring-backend test --home $H)
GRANTER=$($B debug addr "$AU_VALOPER" 2>&1 | awk '/Bech32 Acc/ {print $NF}')
sudo tee /etc/yamale/auto-unjail.env >/dev/null <<ENV
AU_BINARY=$B
AU_NODE=http://127.0.0.1:26657
AU_CHAIN_ID=$CHAIN_ID
AU_VALOPER=$AU_VALOPER
AU_GRANTER=$GRANTER
AU_KEYRING_HOME=$H
AU_KEY=unjailer
AU_FEES=2000uyml
AU_MAX_PER_DAY=3
ENV
sudo chmod 644 /etc/yamale/auto-unjail.env
sudo tee /etc/systemd/system/yamale-auto-unjail.service >/dev/null <<UNIT
[Unit]
Description=Unjail this host's validator when it is safe (see /usr/local/sbin/yamale-auto-unjail)
After=network-online.target yamale-devnet.service

[Service]
Type=oneshot
User=yamale-unjail
Group=yamale-unjail
StateDirectory=yamale-auto-unjail
EnvironmentFile=/etc/yamale/auto-unjail.env
ExecStart=/usr/local/sbin/yamale-auto-unjail
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$H
UNIT
sudo tee /etc/systemd/system/yamale-auto-unjail.timer >/dev/null <<'UNIT'
[Unit]
Description=Check every two minutes whether this host's validator needs unjailing

[Timer]
OnBootSec=3min
OnUnitActiveSec=2min

[Install]
WantedBy=timers.target
UNIT
sudo systemctl daemon-reload
sudo systemctl enable --now yamale-auto-unjail.timer >/dev/null 2>&1
echo "unjailer (grantee): $GRANTEE"
echo "operator (granter): $GRANTER"
echo "dry run: $(sudo env STATE_DIRECTORY=/tmp/au-dry /usr/local/sbin/yamale-auto-unjail --dry-run 2>&1 || true)"
REMOTE
