#!/usr/bin/env bash
#
# Adds `include /etc/nginx/snippets/yamale-headers.conf;` to every server block
# that does not already have it. Run ON the host, once:
#
#   ssh <host> 'bash -s -- yamale' < deploy/include-headers.sh
#
# deploy.sh copies the snippet to both hosts and deliberately does not include
# it, because a bad header on the only public hostname breaks every console at
# once. This is the attended half, and it is idempotent.
#
# Note that including it at server level is not sufficient on its own:
# add_header REPLACES rather than merges, so any location setting a header of
# its own drops all of these. yamale-cache.conf sets Cache-Control in three
# locations and therefore includes this snippet inside each of them.
#
# The snippet has been copied to both hosts by deploy.sh for two days and has
# done nothing, because a snippet nobody includes is a file on a disk. The Pi is
# the public host and includes it from no block at all.
#
# Inserted after the `server_name` line of each block, which is the one line
# every block here has exactly once. nginx -t decides whether the reload happens.
set -euo pipefail

f=/etc/nginx/sites-enabled/$1
inc='    include /etc/nginx/snippets/yamale-headers.conf;'

# The backup goes to sites-retired, where this host already keeps them. nginx
# includes sites-enabled/* with no extension filter, so a copy left beside the
# original is a second live config — exactly what happened here: "a duplicate
# default server for 0.0.0.0:80", from a file whose whole purpose was to not be
# used. The same trap had already been hit with .bak files.
sudo mkdir -p /etc/nginx/sites-retired
sudo cp -n "$f" "/etc/nginx/sites-retired/$1.before-headers" 2>/dev/null || true

sudo python3 - "$f" "$inc" <<'PY'
import re, sys
path, inc = sys.argv[1], sys.argv[2]
src = open(path).read()

# Walk the file brace by brace so that each top-level server block is found with
# its true extent; a regex over the whole file cannot tell a nested location's
# closing brace from the block's own.
out, i, added, skipped = [], 0, 0, 0
while True:
    m = re.compile(r'^server\s*\{', re.M).search(src, i)
    if not m:
        out.append(src[i:])
        break
    depth, j = 0, m.start()
    while j < len(src):
        if src[j] == '{':
            depth += 1
        elif src[j] == '}':
            depth -= 1
            if depth == 0:
                break
        j += 1
    block = src[m.start():j + 1]
    if 'yamale-headers.conf' in block:
        skipped += 1
    else:
        sn = re.search(r'^([ \t]*)server_name[^;]*;\s*$', block, re.M)
        if sn:
            block = block[:sn.end()] + '\n' + inc + block[sn.end():]
            added += 1
        else:
            print('  !! no server_name line; left alone:', block.split('\n')[1].strip())
    out.append(src[i:m.start()])
    out.append(block)
    i = j + 1

open(path, 'w').write(''.join(out))
print('  server blocks: %d given the include, %d already had it' % (added, skipped))
PY

sudo nginx -t
