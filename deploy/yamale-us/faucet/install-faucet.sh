#!/usr/bin/env bash
# install-faucet.sh - run the devnet faucet on yamale-us, and only there.
#
# The faucet signs with the foundation key, and two faucets on one key collide
# on the account sequence, so there is exactly one: this. The VM's copy is
# stopped by this script, and its /api/faucet/ forwards here; the Pi's funnel
# already forwards to the VM, so every entry point reaches the one faucet.
#
# Its keyring holds the foundation key and nothing else - not the node's home,
# which holds the master validator's signing key, and not the VM's whole test
# keyring, which holds every devnet persona.
set -euo pipefail
VM="ubuntu@92.4.151.72"
VMKEY="$HOME/.ssh/yamale_oracle"
US="${FAUCET_HOST:-yamale-us}"
HEX=239d24b7ee9def4351f2e549d64693f695d35428   # yml1ywwjfdlwnhh5x50ju4yav35n762ax4pg9fz7j0, the foundation

T=$(mktemp -d)
trap 'rm -r "$T"' EXIT
ssh -i "$VMKEY" "$VM" "cat /opt/yamale/node/keyring-test/foundation.info" > "$T/foundation.info"
ssh -i "$VMKEY" "$VM" "cat /opt/yamale/node/keyring-test/$HEX.address" > "$T/$HEX.address"
ssh -i "$VMKEY" "$VM" "cat /opt/yamale/bin/yamale-faucet" > "$T/yamale-faucet"
CUR=$(ssh -i "$VMKEY" "$VM" "sudo cat /etc/systemd/system/yamale-faucet.service.d/currencies.conf" | grep -oE -- '--currencies [^ ]+' | head -1)
[ -n "$CUR" ] || { echo "could not read the currency list"; exit 1; }
echo "$CUR" > "$T/currencies"

tar czf - -C "$T" . | ssh "$US" "HEX=$HEX;"'set -eo pipefail
S=$(mktemp -d); trap "sudo rm -r \"\$S\"" EXIT
tar xzf - -C "$S"
H=/opt/yamale/faucet
id yamale-faucet >/dev/null 2>&1 || sudo useradd --system --home-dir $H --no-create-home --shell /usr/sbin/nologin yamale-faucet
sudo install -o root -g root -m 755 "$S/yamale-faucet" /opt/yamale/bin/yamale-faucet
sudo install -d -o yamale-faucet -g yamale-faucet -m 700 $H $H/keyring-test $H/config
sudo install -o yamale-faucet -g yamale-faucet -m 600 "$S/foundation.info" $H/keyring-test/foundation.info
sudo install -o yamale-faucet -g yamale-faucet -m 600 "$S/$HEX.address" $H/keyring-test/$HEX.address
ADDR=$(sudo -u yamale-faucet /opt/yamale/bin/blockchaind keys show foundation -a --keyring-backend test --home $H)
[ "$ADDR" = yml1ywwjfdlwnhh5x50ju4yav35n762ax4pg9fz7j0 ] || { echo "the faucet keyring does not hold the foundation: $ADDR"; exit 1; }
echo "faucet keyring holds: $(sudo -u yamale-faucet /opt/yamale/bin/blockchaind keys list --keyring-backend test --home $H --list-names | tr "\n" " ")"
CUR=$(cat "$S/currencies")
sudo tee /etc/systemd/system/yamale-faucet.service >/dev/null <<UNIT
[Unit]
Description=Yamale devnet faucet (the only one: it signs with the foundation key)
After=network-online.target yamale-devnet.service
Wants=network-online.target

[Service]
User=yamale-faucet
Group=yamale-faucet
ExecStart=/opt/yamale/bin/yamale-faucet --listen 127.0.0.1:8085 --binary /opt/yamale/bin/blockchaind --home $H --node http://127.0.0.1:26657 --chain-id yamale-devnet-2 --from foundation --amount 1000000000uyml $CUR --sponsor 1000000uyml --cooldown 6h --daily-cap 200
Restart=always
RestartSec=5
MemoryMax=256M
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$H

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload
echo ready
'
