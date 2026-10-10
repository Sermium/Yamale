#!/usr/bin/env bash
# deploy-gate.sh - install the chain's nginx gates on yamale-us (127.0.0.1:8081).
#
# The snippets are deploy/nginx/*.conf, the same files the VM ran. Only the
# account routes change, because the services moved: the custodian is local to
# yamale-us now, and recovery stays on the Pi (never on the same host as the
# custodian) and is reached over the tailnet. Each rewrite is checked, because
# a sed that matched nothing leaves a route pointing at the old host and every
# request through it fails with a 502 that looks like an outage.
#
# The supervisors' password file is copied from the VM by hand, never from git:
#   ssh vm 'sudo cat /etc/nginx/yamale-supervisors' | ssh yamale-us \
#     'sudo install -D -m 640 -g 101 /dev/stdin /etc/yamale/gate/yamale-supervisors'
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
HOST="${GATE_HOST:-yamale-us}"
SRC=../../nginx
T=$(mktemp -d)
trap 'rm -r "$T"' EXIT
mkdir "$T/snippets"

for f in yamale-headers.conf yamale-api.conf yamale-visibility.conf yamale-rpc.conf; do
  tr -d '\r' < "$SRC/$f" > "$T/snippets/$f"
done
tr -d '\r' < "$SRC/yamale-account.conf" \
  | sed -e 's#http://100\.117\.244\.96:8099/#http://127.0.0.1:8099/#g' \
        -e 's#http://127\.0\.0\.1:8100/#http://100.68.207.17:8100/#g' \
  > "$T/snippets/yamale-account.conf"
grep -q '100\.117\.244\.96' "$T/snippets/yamale-account.conf" && { echo "custodian route still points at the VM"; exit 1; }
[ "$(grep -c 'proxy_pass http://127.0.0.1:8099/' "$T/snippets/yamale-account.conf")" -ge 2 ] || { echo "custodian routes not rewritten"; exit 1; }
grep -q 'proxy_pass http://100.68.207.17:8100/' "$T/snippets/yamale-account.conf" || { echo "recovery route not rewritten"; exit 1; }
tr -d '\r' < nginx.conf > "$T/nginx.conf"
tr -d '\r' < docker-compose.yml > "$T/docker-compose.yml"

tar czf - -C "$T" . | ssh "$HOST" 'set -eo pipefail
sudo test -f /etc/yamale/gate/yamale-supervisors || { echo "missing /etc/yamale/gate/yamale-supervisors (see the header of this script)"; exit 1; }
S=$(mktemp -d); trap "sudo rm -r \"\$S\"" EXIT
tar xzf - -C "$S"
# Validate the new configuration in a throwaway container before it replaces
# the running one, so a bad snippet never takes the gate down.
sudo install -d -m 755 /var/log/yamale-gate
# Read-only, as the real container is: nginx -t opens every log file, so a
# path the container cannot write fails here rather than on start-up.
sudo docker run --rm --network none --read-only --tmpfs /var/cache/nginx --tmpfs /var/run \
  -v /var/log/yamale-gate:/var/log/nginx \
  -v "$S/nginx.conf:/etc/nginx/nginx.conf:ro" -v "$S/snippets:/etc/nginx/snippets:ro" \
  -v /etc/yamale/gate/yamale-supervisors:/etc/nginx/yamale-supervisors:ro \
  nginx:stable-alpine nginx -t 2>&1 | tail -2
sudo install -d -m 755 /opt/yamale/gate
sudo rm -rf /opt/yamale/gate/snippets.new
sudo cp -r "$S/snippets" /opt/yamale/gate/snippets.new
sudo install -m 644 "$S/nginx.conf" /opt/yamale/gate/nginx.conf
sudo install -m 644 "$S/docker-compose.yml" /opt/yamale/gate/docker-compose.yml
sudo rm -rf /opt/yamale/gate/snippets && sudo mv /opt/yamale/gate/snippets.new /opt/yamale/gate/snippets
cd /opt/yamale/gate && sudo docker compose up -d --force-recreate 2>&1 | tail -1
sleep 2
curl -s -o /dev/null -w "rpc   /api/rpc/status          %{http_code}\n" http://127.0.0.1:8081/api/rpc/status
curl -s -o /dev/null -w "rest  /api/rest/cosmos/base/... %{http_code}\n" http://127.0.0.1:8081/api/rest/cosmos/base/tendermint/v1beta1/node_info
curl -s -o /dev/null -w "rest  unlisted path (deny)     %{http_code}\n" http://127.0.0.1:8081/api/rest/cosmos/upgrade/v1beta1/current_plan
'
