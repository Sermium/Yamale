#!/usr/bin/env bash
#
# Builds the consoles and puts them, and the nginx rules that serve them, on
# both hosts — then checks the result on the PUBLIC hostname rather than on the
# host it was just written to.
#
# # Why this script exists
#
# There are two web hosts and only one of them is public. Deploying to the wrong
# one succeeds, reports success, changes nothing a visitor can see, and leaves no
# trace that anything is wrong. That has now happened twice:
#
#   * Consoles were deployed to the VM for a day while the funnel served the
#     Pi's month-old copy. Every check passed, because every check asked the VM.
#   * The Pi's REST allow-list drifted a revision behind the VM's and was
#     missing /cosmos/auth/.../accounts/, which every client must read before it
#     can sign. Signing was broken for every public visitor, and it failed as a
#     browser credential dialog — a 401 with a WWW-Authenticate challenge — which
#     looks like a login prompt rather than like a misconfigured gateway.
#
# Both were invisible to any check run against the host being changed. So the
# rule this script enforces is: the last word belongs to the public URL.
#
# # The topology, which is the thing worth remembering
#
#   Pi   (tailnet 100.68.207.17) — nginx :8093 — Tailscale Funnel — PUBLIC
#   VM   (92.4.151.72)           — nginx :443  — yamale.ddns.net
#
# The Pi is behind a mobile network that accepts no inbound connection, which is
# why the funnel exists; the funnel terminates on the Pi, which is why the Pi is
# the public host despite being the one that cannot accept inbound. That
# inversion is the whole trap.
#
# # What is shared and what is not
#
# The visibility gate is shared BY VALUE: it decides what an anonymous reader can
# see, and two hosts answering differently is a security difference nobody
# declared. It is copied byte-for-byte to both.
#
# The server blocks are not shared: the VM terminates TLS through certbot and
# runs the faucet locally; the Pi listens plain on 8093 and proxies the faucet
# over the tailnet. Those are real differences and templating them would produce
# a file neither host wants.
#
# Usage:
#   deploy/deploy.sh              build, deploy, verify
#   deploy/deploy.sh --verify     verify only, change nothing
#   deploy/deploy.sh --no-build   deploy what is already in clients/*/dist

set -euo pipefail

PUBLIC=${YAMALE_PUBLIC:-https://yamale.tail4355e8.ts.net}
PI=${YAMALE_PI:-zubulmuk92@100.68.207.17}
PI_PORT=${YAMALE_PI_PORT:-2222}
PI_KEY=${YAMALE_PI_KEY:-$HOME/.ssh/id_ed25519}
VM=${YAMALE_VM:-ubuntu@92.4.151.72}
VM_KEY=${YAMALE_VM_KEY:-$HOME/.ssh/yamale_oracle}

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SITE=/srv/yamale/site

pi() { ssh -i "$PI_KEY" -p "$PI_PORT" -o ConnectTimeout=30 "$PI" "$@"; }
vm() { ssh -i "$VM_KEY" -o ConnectTimeout=30 "$VM" "$@"; }

build=1
verify_only=0
for arg in "$@"; do
  case "$arg" in
    --no-build) build=0 ;;
    --verify)   verify_only=1 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

# ---------------------------------------------------------------- build

if [ "$verify_only" = 0 ] && [ "$build" = 1 ]; then
  echo "==> building the consoles"
  ( cd "$ROOT/clients" && npm run build --workspaces --if-present )
fi

# ---------------------------------------------------------------- deploy

if [ "$verify_only" = 0 ]; then
  echo "==> nginx rules, to both hosts, from the repo"
  # Copied to both because a gate that differs between hosts is a security
  # difference nobody wrote down. nginx -t before the reload: a bad snippet that
  # reaches a reload takes the site down, and a bad snippet that fails the test
  # takes nothing down.
  for conf in yamale-visibility.conf yamale-apps.conf yamale-cache.conf; do
    for host in pi vm; do
      $host "sudo cp -n /etc/nginx/snippets/$conf /etc/nginx/snippets/$conf.before-deploy 2>/dev/null || true" </dev/null
      $host "sudo tee /etc/nginx/snippets/$conf >/dev/null" < "$ROOT/deploy/nginx/$conf"
    done
  done
  for host in pi vm; do
    $host 'sudo nginx -t 2>&1 | tail -1 && sudo systemctl reload nginx' </dev/null
  done

  echo "==> the site, to both hosts"
  # Through this machine rather than host-to-host: neither host holds a key for
  # the other, and giving one that key to save a hop widens the blast radius of
  # either being compromised for no gain a deploy notices.
  tarball=$(mktemp); trap 'rm -f "$tarball"' EXIT
  ( cd "$ROOT/clients" && bash "$ROOT/deploy/stage-site.sh" "$tarball" )

  # Unpacked beside the live tree and moved into place, rather than extracted
  # over it. tar never deletes, so extracting in place accumulates every bundle
  # ever shipped — the VM was carrying four generations of some of them — and,
  # worse, keeps serving a file the build no longer produces if an index.html
  # ever points back at one. The swap is two renames, so the window in which the
  # site is neither the old tree nor the new one is a fraction of a second, and
  # the previous tree stays on disk as .previous to roll back to.
  for host in pi vm; do
    $host "sudo rm -rf $SITE.incoming && sudo mkdir -p $SITE.incoming && sudo tar -xzf - -C $SITE.incoming" < "$tarball"
    $host "sudo rm -rf $SITE.previous         && sudo mv $SITE $SITE.previous         && sudo mv $SITE.incoming $SITE" </dev/null
  done
fi

# ---------------------------------------------------------------- verify

echo "==> verifying on the public hostname: $PUBLIC"
fail=0
check() { # path, expected code, what it proves
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 "$PUBLIC$1" || echo 000)
  if [ "$code" = "$2" ]; then
    printf '    ok   %-3s %-52s %s\n' "$code" "$1" "$3"
  else
    printf '    FAIL %-3s (wanted %s) %-38s %s\n' "$code" "$2" "$1" "$3"
    fail=1
  fi
}

# Every console answers at all.
for p in / /app/ /wallet/ /explorer/ /safe/ /rwa/ /land/ /markets/ /oversight/ \
         /keys/ /demo/ /governance/ /foundation/ /validator/ /docs/; do
  check "$p" 200 ""
done

# A path each router knows and the filesystem does not. This is the check that
# would have caught /rwa/ answering 404 on every route but its own root.
check /rwa/holdings        200 "rwa deep link"
check /explorer/blocks     200 "explorer deep link"
check /wallet/faucet       200 "wallet deep link"
check /safe/proposals      200 "safe deep link"

# The node, and the endpoints a client must read before it can sign anything.
# These are the ones whose absence produces a credential dialog rather than an
# error, so they are checked by code and not by eye.
check /api/rpc/status                                            200 "node reachable"
check /api/rest/cosmos/auth/v1beta1/accounts/yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg 200 "signing possible"
check /api/rest/cosmos/feegrant/v1beta1/allowances/yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg 200 "fee sponsorship visible"
check /api/rest/yamale/blockchain/tokenisation/v1/params         200 "offerings readable"
check /api/rest/yamale/blockchain/oracle/v1/params               200 "rates readable"

# And the gate still closed where it is meant to be. A deploy that quietly
# opens everything passes every check above.
check /api/rest/yamale/blockchain/land/v1/params  401 "land still supervised"
check /api/rpc/net_info                           403 "peer map still refused"

# The bundle the public actually gets is the bundle just built. Everything above
# can pass against a month-old copy of the site.
echo "==> bundle identity"
for app in app explorer wallet safe rwa; do
  local_js=$(ls "$ROOT/clients/$app/dist/assets/"index-*.js 2>/dev/null | head -1 | sed 's|.*/||')
  live_js=$(curl -s --max-time 30 "$PUBLIC/$app/" | grep -oE 'index-[A-Za-z0-9_-]+\.js' | head -1)
  if [ -z "$local_js" ]; then
    printf '    --   %-10s no local build to compare\n' "$app"
  elif [ "$local_js" = "$live_js" ]; then
    printf '    ok   %-10s %s\n' "$app" "$live_js"
  else
    printf '    FAIL %-10s live=%s built=%s\n' "$app" "${live_js:-none}" "$local_js"
    fail=1
  fi
done

if [ "$fail" != 0 ]; then
  echo
  echo "the public hostname does not serve what this repo builds." >&2
  exit 1
fi
echo "==> public hostname serves what this repo builds"
