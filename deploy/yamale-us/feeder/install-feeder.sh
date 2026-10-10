#!/usr/bin/env bash
# install-feeder.sh - move the master validator's oracle feeder to yamale-us.
#
# The feeder reports exchange rates for validator "pi" (the master, on yamale-us
# since 2026-10-10) and signs as alice, the delegate the validator authorised.
# Two feeders on one key collide on the account sequence, so this is a cutover:
# the VM's feeder is stopped in the same run that starts this one.
#
# Its keyring holds alice and nothing else, and it may write only its own home:
# the repo unit allows all of /opt/yamale, which here includes the node's home
# and the master validator's signing key.
set -euo pipefail
VM="ubuntu@92.4.151.72"
VMKEY="$HOME/.ssh/yamale_oracle"
US="${FEEDER_HOST:-yamale-us}"
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
HEX=1997d0e2d3dde8764d13a1b76dd46df0935b4c3c   # yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg, alice
VALIDATOR=ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh

T=$(mktemp -d)
trap 'rm -r "$T"' EXIT
ssh -i "$VMKEY" "$VM" 'cat /opt/yamale/node/keyring-test/alice.info' > "$T/alice.info"
ssh -i "$VMKEY" "$VM" "cat /opt/yamale/node/keyring-test/$HEX.address" > "$T/$HEX.address"
ssh -i "$VMKEY" "$VM" 'cat /opt/yamale/bin/yamale-feeder' > "$T/yamale-feeder"
tr -d '\r' < "$SRC/deploy/systemd/yamale-feeder.service" > "$T/yamale-feeder.service"

tar czf - -C "$T" . | ssh "$US" "HEX=$HEX VALIDATOR=$VALIDATOR;"'set -eo pipefail
S=$(mktemp -d); trap "sudo rm -r \"\$S\"" EXIT
tar xzf - -C "$S"
H=/opt/yamale/feeder
id yamale-feeder >/dev/null 2>&1 || sudo useradd --system --home-dir $H --no-create-home --shell /usr/sbin/nologin yamale-feeder
sudo install -o root -g root -m 755 "$S/yamale-feeder" /opt/yamale/bin/yamale-feeder
sudo install -d -o yamale-feeder -g yamale-feeder -m 700 $H $H/keyring-test
sudo install -o yamale-feeder -g yamale-feeder -m 600 "$S/alice.info" $H/keyring-test/alice.info
sudo install -o yamale-feeder -g yamale-feeder -m 600 "$S/$HEX.address" $H/keyring-test/$HEX.address
A=$(sudo -u yamale-feeder /opt/yamale/bin/blockchaind keys show alice -a --keyring-backend test --home $H)
[ "$A" = yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg ] || { echo "feeder keyring does not hold alice: $A"; exit 1; }
sudo tee /etc/yamale/feeder.env >/dev/null <<ENV
FEEDER_BINARY=/opt/yamale/bin/blockchaind
FEEDER_HOME=$H
FEEDER_NODE=http://127.0.0.1:26657
FEEDER_CHAIN_ID=yamale-devnet-2
FEEDER_KEYRING=test
FEEDER_KEY=alice
FEEDER_VALIDATOR=$VALIDATOR
FEEDER_SOURCE=erapi
FEEDER_INTERVAL=60s
ENV
sudo chmod 640 /etc/yamale/feeder.env
sudo install -m 644 "$S/yamale-feeder.service" /etc/systemd/system/yamale-feeder.service
sudo install -d /etc/systemd/system/yamale-feeder.service.d
printf "[Service]\nUser=yamale-feeder\nGroup=yamale-feeder\nReadWritePaths=\nReadWritePaths=$H\nMemoryMax=128M\n" | sudo tee /etc/systemd/system/yamale-feeder.service.d/host.conf >/dev/null
sudo systemctl daemon-reload
echo staged
'

# The cutover: the VM stops reporting before this host starts.
ssh -i "$VMKEY" "$VM" 'sudo systemctl disable --now yamale-feeder 2>&1 | tail -1; echo "VM feeder: $(systemctl is-active yamale-feeder)"'
ssh "$US" 'sudo systemctl enable --now yamale-feeder 2>&1 | tail -1; echo "yamale-us feeder: $(systemctl is-active yamale-feeder)"'
