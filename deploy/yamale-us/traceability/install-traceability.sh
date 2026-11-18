#!/usr/bin/env bash
# install-traceability.sh - build the traceability app and traceability-api here, install them on yamale-us.
#
# Installs:
#   --api  the yamale-traceability user, /opt/yamale/bin/yamale-traceabilityapi, a keyring
#          holding the fee sponsor key and nothing else (/opt/yamale/traceability),
#          the operator token, yamale-traceability.service (127.0.0.1:8095), a
#          nightly backup timer, and the jewel-admit wrapper with its scripts.
#   --app  the static app, into /srv/traceability (served by the gate on :8082).
# With neither flag it does both, the API first.
#
# It never prints a seed phrase, a private key, a keyring file or the operator
# token: key creation goes to /dev/null, the token is written straight to its
# file. The sponsor ADDRESS is printed; an address is not a secret.
#
# The app is copied INTO /srv/traceability, never swapped by rename: the gate
# container bind-mounts that directory, and a renamed directory would leave it
# serving the old one. The previous tree is copied aside first.
set -euo pipefail
HOST="${TRACEABILITY_HOST:-yamale-us}"
cd "$(dirname "$0")/../../.."

DO_API=0; DO_APP=0
for a in "$@"; do
  case "$a" in
    --api) DO_API=1 ;;
    --app) DO_APP=1 ;;
    *) echo "usage: install-traceability.sh [--api] [--app]" >&2; exit 2 ;;
  esac
done
if [ $DO_API = 0 ] && [ $DO_APP = 0 ]; then DO_API=1; DO_APP=1; fi

if compgen -G "clients/traceability/.env*" >/dev/null || compgen -G "clients/.env*" >/dev/null; then
  echo "refusing: an .env file under clients/ would be inlined into the bundle by Vite:" >&2
  compgen -G "clients/traceability/.env*" >&2 || true
  compgen -G "clients/.env*" >&2 || true
  exit 1
fi
if ! command -v go >/dev/null 2>&1 && [ -d "/c/Program Files/Go/bin" ]; then
  export PATH="/c/Program Files/Go/bin:$PATH"
fi

T=$(mktemp -d)
trap 'rm -r "$T"' EXIT

if [ $DO_APP = 1 ]; then
  (cd clients && VITE_CHAIN_ID=yamale-devnet-2 VITE_PUBLIC_ORIGIN=https://traceability.yamaletech.com VITE_GAS_PRICE=0 npm run build --workspace @yamale/traceability)
  grep -l yamale-devnet-2 clients/traceability/dist/assets/*.js >/dev/null \
    || { echo "the bundle does not carry yamale-devnet-2" >&2; exit 1; }
  if grep -l yamale-local-1 clients/traceability/dist/assets/*.js >/dev/null 2>&1; then
    echo "the bundle carries yamale-local-1: an env file leaked into the build" >&2
    exit 1
  fi
  cp -r clients/traceability/dist "$T/dist"
fi
if [ $DO_API = 1 ]; then
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$T/yamale-traceabilityapi" ./tools/traceabilityapi
  for sc in scripts/devnet/jewel-lib.sh scripts/devnet/traceability-admit.sh deploy/yamale-us/traceability/jewel-admit; do
    if [ "$(LC_ALL=C tr -dc '\r' < "$sc" | wc -c)" -ne 0 ]; then
      echo "refusing: $sc contains a CR byte (CRLF line endings); a script with CRLF cannot run on the host" >&2
      exit 1
    fi
  done
  cp scripts/devnet/jewel-lib.sh scripts/devnet/traceability-admit.sh deploy/yamale-us/traceability/jewel-admit "$T/"
fi

tar czf - -C "$T" . | ssh "$HOST" "API=$DO_API;APP=$DO_APP;"'set -euo pipefail
S=$(mktemp -d); trap "rm -r \"\$S\"" EXIT
tar xzf - -C "$S"
BIN=/opt/yamale/bin/blockchaind
H=/opt/yamale/traceability
NODE=tcp://127.0.0.1:26657
TOKEN=/etc/yamale/traceability-operator-token
MIN=100000000

if [ "$API" = 1 ]; then
  id yamale-traceability >/dev/null 2>&1 || sudo useradd --system --home-dir $H --no-create-home --shell /usr/sbin/nologin yamale-traceability
  sudo install -o root -g root -m 755 "$S/yamale-traceabilityapi" /opt/yamale/bin/yamale-traceabilityapi
  sudo install -o root -g root -m 755 "$S/jewel-admit" "$S/jewel-lib.sh" "$S/traceability-admit.sh" /opt/yamale/bin/
  sudo install -d -o yamale-traceability -g yamale-traceability -m 700 $H $H/keyring-test
  sudo install -d -o yamale-traceability -g yamale-traceability -m 750 /var/lib/yamale-traceability
  sudo install -d -o root -g root -m 700 /var/backups/yamale-traceability

  # The sponsor key: created only when absent, its seed phrase goes nowhere.
  sudo -u yamale-traceability $BIN keys show traceability-sponsor -a --keyring-backend test --home $H >/dev/null 2>&1 \
    || sudo -u yamale-traceability $BIN keys add traceability-sponsor --keyring-backend test --home $H >/dev/null 2>&1 \
    || { echo "could not create the sponsor key"; exit 1; }
  ADDR=$(sudo -u yamale-traceability $BIN keys show traceability-sponsor -a --keyring-backend test --home $H)
  [ -n "$ADDR" ] || { echo "no sponsor address"; exit 1; }
  echo "sponsor address: $ADDR"
  NKEYS=$(sudo -u yamale-traceability $BIN keys list --keyring-backend test --home $H --list-names | grep -c . || true)
  [ "$NKEYS" = 1 ] || { echo "the sponsor keyring must hold exactly one key, it holds $NKEYS"; exit 1; }

  bal() {
    sudo -u yamale-traceability $BIN query bank balance "$ADDR" uyml --home $H --node $NODE -o json 2>/dev/null \
      | python3 -c "import sys,json;print(json.load(sys.stdin)[\"balance\"][\"amount\"])" 2>/dev/null || echo 0
  }
  B=$(bal)
  if [ "$B" -lt $MIN ]; then
    echo "sponsor balance $B uyml is below 100 YML, asking the faucet"
    curl -fsS -X POST http://127.0.0.1:8085/ -H "Content-Type: application/json" -d "{\"address\":\"$ADDR\"}" >/dev/null
    for i in $(seq 1 30); do
      B=$(bal)
      [ "$B" -ge $MIN ] && break
      sleep 1
    done
    [ "$B" -ge $MIN ] || { echo "the sponsor is still below 100 YML ($B uyml) after 30 s"; exit 1; }
  fi
  echo "sponsor balance: $B uyml"

  # The operator token: created only when absent, never printed.
  sudo install -d -o root -g root -m 755 /etc/yamale
  sudo test -s $TOKEN || (umask 077; head -c 32 /dev/urandom | od -An -tx1 | tr -d " \n" | sudo tee $TOKEN >/dev/null)
  sudo chown root:yamale-traceability $TOKEN && sudo chmod 640 $TOKEN

  sudo tee /etc/systemd/system/yamale-traceability.service >/dev/null <<"UNIT"
[Unit]
Description=Yamale traceability-api (traceability.yamaletech.com): files, profiles, applications, fee sponsorship
After=network-online.target yamale-devnet.service
Wants=network-online.target

[Service]
User=yamale-traceability
Group=yamale-traceability
ExecStart=/opt/yamale/bin/yamale-traceabilityapi --listen 127.0.0.1:8095 --dir /var/lib/yamale-traceability --rest http://127.0.0.1:1317 --chain-id yamale-devnet-2 --bin /opt/yamale/bin/blockchaind --from traceability-sponsor --home /opt/yamale/traceability --keyring-backend test --node tcp://127.0.0.1:26657 --sponsor 2000000uyml --sponsor-for 8760h --fees 0uyml --sponsor-daily 200 --sponsor-floor 10000000uyml --operator-token-file /etc/yamale/traceability-operator-token
Restart=always
RestartSec=5
MemoryMax=256M
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/yamale-traceability /opt/yamale/traceability

[Install]
WantedBy=multi-user.target
UNIT
  # The backup copy lives on the same machine: it protects against mistakes,
  # not against losing the server. An off-server copy is open (gaps.md).
  # It also covers the office keyring once it exists: its seed phrases are never
  # kept, so that keyring is the only copy. In the ExecStart, %% is a systemd
  # escape for a literal percent sign.
  sudo tee /etc/systemd/system/yamale-traceability-backup.service >/dev/null <<"UNIT"
[Unit]
Description=Yamale traceability nightly backup (data, sponsor keyring, office keyring)

[Service]
Type=oneshot
User=root
ExecStart=/bin/sh -c "set -e; f=/var/backups/yamale-traceability/traceability-$(date -u +%%Y%%m%%d).tgz; o=; [ -d /opt/yamale/jewel-office ] && o=opt/yamale/jewel-office; tar -czf \"$f.partial\" -C / var/lib/yamale-traceability opt/yamale/traceability $o; gzip -t \"$f.partial\"; mv \"$f.partial\" \"$f\""
ExecStartPost=/usr/bin/find /var/backups/yamale-traceability -name "traceability-*.tgz" -mtime +14 -delete
UNIT
  sudo tee /etc/systemd/system/yamale-traceability-backup.timer >/dev/null <<"UNIT"
[Unit]
Description=Yamale traceability nightly backup

[Timer]
OnCalendar=*-*-* 03:40:00 UTC
Persistent=true

[Install]
WantedBy=timers.target
UNIT
  sudo systemctl daemon-reload
  sudo systemctl enable --now yamale-traceability yamale-traceability-backup.timer
  sudo systemctl restart yamale-traceability
  OK=0
  for i in $(seq 1 15); do
    if curl -fsS http://127.0.0.1:8095/v1/health >/dev/null 2>&1; then OK=1; break; fi
    sleep 1
  done
  [ $OK = 1 ] || { echo "traceability-api did not answer /v1/health within 15 s"; exit 1; }
  echo "health ok; checking sponsor health"
  SPONSOR=$(curl -s -o /dev/null -w "%{http_code}" 127.0.0.1:8095/v1/health/sponsor || true)
  [ "$SPONSOR" = 200 ] || { echo "sponsor health answered $SPONSOR, not 200: the sponsor is low or unreachable"; exit 1; }
  echo "sponsor health 200"
fi

if [ "$APP" = 1 ]; then
  sudo install -d -o root -g root -m 755 /srv/traceability
  if [ -n "$(ls -A /srv/traceability)" ]; then
    PREV=/srv/traceability.prev-$(date -u +%Y%m%d%H%M)
    sudo cp -a /srv/traceability $PREV
    echo "previous app kept in $PREV"
  fi
  # Copy in place: the gate bind-mounts /srv/traceability, so a rename would leave it on the old directory.
  sudo find /srv/traceability -mindepth 1 -delete
  sudo cp -a "$S/dist/." /srv/traceability/
  sudo chown -R root:root /srv/traceability
  sudo find /srv/traceability -type d -exec chmod 755 {} +
  sudo find /srv/traceability -type f -exec chmod 644 {} +
  CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8082/j/1/1 || true)
  echo "gate probe /j/1/1: $CODE"
  [ "$CODE" = 200 ] || { echo "expected 200 from the gate"; exit 1; }
fi
echo ready
'
