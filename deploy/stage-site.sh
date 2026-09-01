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
for console in keys demo markets oversight land governance foundation validator; do
  [ -d "$clients/$console" ] || continue
  mkdir -p "$staging/$console"
  ( cd "$clients/$console"
    find . -maxdepth 2 -type f \
      ! -path './node_modules/*' \
      ! -name 'package.json' ! -name 'package-lock.json' \
      ! -name '*.test.js' ! -name '*.test.mjs' \
      ! -name 'serve.js' ! -name 'README.md' \
      -print0 | while IFS= read -r -d '' f; do
        mkdir -p "$staging/$console/$(dirname "$f")"
        cp "$f" "$staging/$console/$f"
      done )
done

tar -czf "$out" -C "$staging" .
echo "staged $(tar -tzf "$out" | wc -l) files into $out" >&2
