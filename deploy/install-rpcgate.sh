#!/usr/bin/env bash
#
# Puts the RPC method gate in front of a host's CometBFT, and takes the path
# deny-list out of the way.
#
# deploy.sh copies yamale-rpc.conf to both hosts and then probes the POST form
# of every method that is meant to be closed, but it deliberately does not
# install this: switching /api/rpc/ on the only public hostname is not something
# a routine deploy should do unattended. This is the attended half, run once per
# host, and it is idempotent.
#
#   deploy/install-rpcgate.sh pi
#   deploy/install-rpcgate.sh vm
#
# # Why a process and not an nginx rule
#
# The rule this replaces was a location regex listing dangerous methods. A
# location regex matches the URL PATH, and CometBFT takes the method in the POST
# body as well — which is the form every CosmJS client uses. So `GET
# /api/rpc/net_info` returned 403 while `POST /api/rpc/ {"method":"net_info"}`
# returned the validator's full peer map. The list never blocked anything.
#
# nginx cannot read a request body without njs or Lua, so no variation on that
# rule fixes it. The filter has to be a process that parses JSON-RPC.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)

case "${1:-}" in
  pi) SSH="-i ${YAMALE_PI_KEY:-$HOME/.ssh/id_ed25519} -p ${YAMALE_PI_PORT:-2222} ${YAMALE_PI:-zubulmuk92@100.68.207.17}"; ARCH=arm64 ;;
  vm) SSH="-i ${YAMALE_VM_KEY:-$HOME/.ssh/yamale_oracle} ${YAMALE_VM:-ubuntu@92.4.151.72}"; ARCH=amd64 ;;
  *)  echo "usage: $0 pi|vm" >&2; exit 2 ;;
esac
SSH="$SSH -o ConnectTimeout=30"
host() { ssh $SSH "$@"; }

# The service account is read off the node's own unit rather than assumed. The
# committed unit used to say User=yamale, and there is no yamale user on either
# host — the Pi runs these under its own login and the VM under ubuntu — so it
# could not have started on either machine.
user=$(host 'systemctl show yamale-devnet.service -p User --value')
[ -n "$user" ] || { echo "could not read the user yamale-devnet runs as" >&2; exit 1; }
echo "==> $1: $ARCH, service account $user"

echo "==> building rpcgate for linux/$ARCH"
bin=$(mktemp); trap 'rm -f "$bin"' EXIT
( cd "$ROOT" && GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags="-s -w" -o "$bin" ./tools/rpcgate )

echo "==> installing the binary"
# Written beside the target and moved into place: overwriting a running
# executable in place fails with ETXTBSY, and a partial copy of a gate is worse
# than no gate.
host 'sudo tee /opt/yamale/bin/rpcgate.new >/dev/null && sudo chmod 0755 /opt/yamale/bin/rpcgate.new' < "$bin"
host 'sudo mv /opt/yamale/bin/rpcgate.new /opt/yamale/bin/rpcgate'

echo "==> the unit, and the drop-in that names the account"
host 'sudo tee /etc/systemd/system/yamale-rpcgate.service >/dev/null' < "$ROOT/deploy/systemd/yamale-rpcgate.service"
host "sudo mkdir -p /etc/systemd/system/yamale-rpcgate.service.d && printf '[Service]\nUser=%s\n' '$user' | sudo tee /etc/systemd/system/yamale-rpcgate.service.d/user.conf >/dev/null"
host 'sudo systemctl daemon-reload && sudo systemctl enable --now yamale-rpcgate.service && sudo systemctl restart yamale-rpcgate.service'
sleep 2
host 'systemctl is-active yamale-rpcgate.service'

echo "==> checking the gate on 26659 before anything is pointed at it"
host '
for m in status block; do
  curl -s --max-time 10 -X POST http://127.0.0.1:26659/ -H "content-type: application/json" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$m\",\"params\":{}}" | grep -q "\"result\"" \
    || { echo "    !! $m does not answer through the gate; not switching nginx"; exit 1; }
  echo "    ok   $m answers"
done
for m in net_info dump_consensus_state broadcast_tx_commit unconfirmed_txs; do
  curl -s --max-time 10 -X POST http://127.0.0.1:26659/ -H "content-type: application/json" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$m\",\"params\":{}}" | grep -q "\"result\"" \
    && { echo "    !! $m ANSWERED through the gate; not switching nginx"; exit 1; }
  echo "    ok   $m refused"
done'

echo "==> pointing /api/rpc/ at it"
host 'sudo tee /etc/nginx/snippets/yamale-rpc.conf >/dev/null' < "$ROOT/deploy/nginx/yamale-rpc.conf"
host 'bash -s' <<'REMOTE'
set -euo pipefail
f=/etc/nginx/snippets/yamale-api.conf
# Backups go to sites-retired, which is where this host already keeps them.
# nginx includes sites-enabled/* with no extension filter, so a copy left beside
# a site file is a second live config — "duplicate default server", from a file
# whose whole purpose was to be unused. That had already happened once here with
# .bak files; .before-headers repeated it.
sudo mkdir -p /etc/nginx/sites-retired
sudo cp -n "$f" /etc/nginx/sites-retired/yamale-api.conf.before-rpcgate 2>/dev/null || true

sudo python3 - "$f" <<'PY'
import re, sys
path = sys.argv[1]
src = open(path).read()


def block_end(s, start):
    depth, i = 0, start
    while i < len(s):
        if s[i] == '{':
            depth += 1
        elif s[i] == '}':
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    raise SystemExit('unbalanced braces from offset %d' % start)


m = re.search(r'^location\s+~\s+\^/api/rpc/\(', src, re.M)
if m:
    end = block_end(src, m.start())
    lines = src[:m.start()].split('\n')
    while len(lines) > 1 and (lines[-1].startswith('#') or lines[-1].strip() == ''):
        if lines[-1].strip() == '' and not lines[-2].startswith('#'):
            break
        lines.pop()
    src = '\n'.join(lines) + src[end:]
    print('    removed the path deny-list and the comment arguing for it')
else:
    print('    no path deny-list (already removed)')

m = re.search(r'^location\s+/api/rpc/\s*\{', src, re.M)
if m:
    end = block_end(src, m.start())
    src = src[:m.start()] + (
        '# /api/rpc/ and its websocket both live in yamale-rpc.conf, which points\n'
        '# them at tools/rpcgate on 26659 rather than at the node on 26657. The\n'
        '# deny-list regex that used to stand above this went with it: it matched a\n'
        '# URL path, and CometBFT takes the method in the POST body, so it refused\n'
        '# the GET form of five methods that answered in full over POST.\n'
        'include /etc/nginx/snippets/yamale-rpc.conf;'
    ) + src[end:]
    print('    /api/rpc/ now includes yamale-rpc.conf')
elif 'yamale-rpc.conf' in src:
    print('    already switched')
else:
    raise SystemExit('    !! no /api/rpc/ location and no include — refusing to guess')

open(path, 'w').write(re.sub(r'\n{4,}', '\n\n\n', src))
PY

sudo nginx -t
sudo systemctl reload nginx
REMOTE

echo "==> done. Now run deploy/deploy.sh --verify: it probes the POST form on the"
echo "    public hostname, which is the only check that proves this."
