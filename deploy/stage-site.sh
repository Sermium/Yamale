#!/usr/bin/env bash
#
# Assembles the public site into one tarball, written to the path given as $1.
#
# The layout is one origin with every console under a path, because that is what
# a Tailscale Funnel can serve — it terminates a single hostname — and because a
# console reading the chain from the same origin needs no CORS and no second
# certificate. See deploy/nginx/yamale-apps.conf for the routing half of it.
#
# Two kinds of console live here and they are staged differently:
#
#   * Vite workspaces build to dist/ and are copied from there. Their asset
#     filenames carry a content hash, which is what lets the cache rules hold
#     them for a year.
#   * The single-file consoles are plain HTML and ES modules with no build step,
#     copied as-is. Their package.json exists to run their tests and has no
#     business being published, so it is left behind along with the tests
#     themselves — a deployed *.test.js is a file that describes how to break the
#     thing next to it.
#
# Run from clients/.

set -euo pipefail

out=${1:?usage: stage-site.sh <output.tgz>}
out=$(cd "$(dirname "$out")" && pwd)/$(basename "$out")
clients=$(pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT

# The root: the presentation site and the shared stylesheets every console
# links. README.md and build-docs.mjs are the sources of this directory, not
# part of it.
for f in index.html favicon.ico logo.svg mark.svg site.css yamale.css doc.css; do
  [ -f "$clients/site/$f" ] && cp "$clients/site/$f" "$staging/"
done
[ -d "$clients/site/docs" ] && cp -r "$clients/site/docs" "$staging/docs"

# Built apps.
for app in app explorer wallet safe rwa; do
  if [ -d "$clients/$app/dist" ]; then
    mkdir -p "$staging/$app"
    cp -r "$clients/$app/dist/." "$staging/$app/"
  else
    echo "warning: $app has no dist/, deploying without it" >&2
  fi
done

# Single-file consoles.
#
# `serve.mjs` is excluded alongside `serve.js`, below. Every console that has
# one is a local proxy that forwards /api to a chain of the operator's choosing,
# written for driving the page against a devnet — shipping it would put a
# configurable forwarder on the public host, which is not what it was written to
# be. The exclusion named the .js spelling only, and three consoles have since
# been written with the .mjs one.
for console in keys demo markets oversight land livestock syndicate governance foundation validator; do
  [ -d "$clients/$console" ] || continue
  mkdir -p "$staging/$console"
  ( cd "$clients/$console"
    find . -maxdepth 2 -type f \
      ! -path './node_modules/*' \
      ! -name 'package.json' ! -name 'package-lock.json' \
      ! -name '*.test.js' ! -name '*.test.mjs' \
      ! -name 'serve.js' ! -name 'serve.mjs' ! -name 'README.md' \
      -print0 | while IFS= read -r -d '' f; do
        mkdir -p "$staging/$console/$(dirname "$f")"
        cp "$f" "$staging/$console/$f"
      done )
done

# Every destination the site's own menu offers must exist in the bundle.
#
# The list above is hand-written and it drifted: the Ladoum console was added to
# the menu on 2026-09-11 and never added here, so the menu linked to a path the
# bundle did not contain and the console could not have been reached by anybody
# who did not know its URL. deploy.sh reads the same menu and would have caught
# it — but only after uploading, against the public hostname, which is a slow
# and public way to find out a directory is missing. Checked here, the bundle
# refuses to be built at all.
#
# Read out of the menu rather than listed, for the reason deploy.sh gives for
# doing the same: a second hand-written list goes stale in the same direction as
# the first.
missing=
for link in $(grep -oE 'class="app-link[^"]*" href="[^"]+"' "$clients/site/index.html" \
              | grep -oE 'href="[^"]+"' | sed 's/href=//;s/"//g' | sort -u); do
  # Only the first segment. /wallet/faucet and /safe/operations are routes
  # inside an app, not directories of their own.
  top=${link#/}; top=${top%%/*}
  [ -n "$top" ] || continue
  [ -d "$staging/$top" ] || missing="$missing $top"
done
if [ -n "$missing" ]; then
  echo "error: the site menu links to$missing, which this bundle does not contain." >&2
  echo "       add it to the console list above, or build its dist/." >&2
  exit 1
fi

tar -czf "$out" -C "$staging" .
echo "staged $(tar -tzf "$out" | wc -l) files into $out" >&2
