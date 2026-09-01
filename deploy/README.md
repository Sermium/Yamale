# Deployment

Two web hosts, and the public one is not the one you would guess.

```
Pi   tailnet 100.68.207.17   nginx :8093   <- Tailscale Funnel   <- THE PUBLIC
VM   92.4.151.72             nginx :443       yamale.ddns.net
```

The Pi sits behind a mobile network that accepts no inbound connection. That is
*why* the funnel exists — and the funnel terminates on the Pi, which makes the
Pi the public host despite being the one that cannot be dialled. The VM, which
can be dialled and holds the certificate, is the private one.

That inversion has cost real time twice:

* Consoles were deployed to the VM for a day while the funnel served the Pi's
  month-old copy. Every check passed, because every check asked the VM.
* The Pi's REST allow-list drifted a revision behind the VM's, missing
  `/api/rest/cosmos/auth/v1beta1/accounts/`. Every client reads that endpoint
  before it can sign, so signing was broken for every public visitor — and it
  failed as a `401` carrying a `WWW-Authenticate` challenge, which a browser
  renders as a username-and-password dialog. It looks like a login page on an
  app that has no login.

Neither was visible to a check run against the host being changed. Hence the one
rule this directory enforces: **the last word belongs to the public URL.**

## Deploying

```bash
deploy/deploy.sh
```

Builds every console, installs the nginx rules on both hosts, swaps the site
into place, then verifies — against `https://yamale.tail4355e8.ts.net`, never
against a host it just wrote to. It exits non-zero if the public hostname does
not serve what the repo builds.

`--verify` checks without changing anything. `--no-build` deploys whatever is
already in `clients/*/dist`.

Hosts and keys come from the environment (`YAMALE_PI`, `YAMALE_VM`,
`YAMALE_PUBLIC`, …) so this is not welded to one operator's machine.

## What is shared between the hosts, and what is not

`nginx/yamale-visibility.conf` is copied **byte-for-byte to both**. It decides
what an anonymous reader may see, and two hosts answering that question
differently is a security difference nobody declared. It is the file that
drifted.

`nginx/yamale-apps.conf` and `nginx/yamale-cache.conf` are shared for the same
reason in weaker form: an app that routes on one host and 404s on the other is
the same class of bug with a smaller blast radius.

The **server blocks are deliberately not shared.** The VM terminates TLS through
certbot and runs the faucet locally; the Pi listens plain on 8093 and proxies
the faucet to the VM over the tailnet. Those are real differences, and
templating them would produce a file neither host wants. They stay on the hosts.

## The checks, and why each one is there

The listing checks are dull and cheap. Three groups are not:

**Deep links.** For each routed app, a path its router knows and the filesystem
does not. Client-side routing never asks the server, so a missing `try_files`
fallback is invisible until somebody refreshes or opens a shared link — and then
the app 404s on a URL it put in the address bar itself. `/rwa/` was in exactly
that state until 2026-09-01.

**Signing.** `cosmos/auth/.../accounts/` and `feegrant`. Their absence does not
look like a gateway fault; it looks like a login prompt, which sends the
diagnosis in the wrong direction for hours.

**Still-closed.** `land` must still answer 401 and `net_info` must still answer
403. Every other check in the file passes on a gateway that has been opened up
completely, so without these the suite would applaud the worst possible outcome.

Then bundle identity: the hash the public is served, against the hash just
built. Everything above passes against a month-old copy of the site.

## Rolling back

The previous tree is left on each host as `/srv/yamale/site.previous`, and the
nginx snippets as `*.before-deploy` (written once, on the first deploy through
this script, so they are the last known-good hand-edited state).

```bash
ssh <host> 'sudo rm -rf /srv/yamale/site.rollback \
  && sudo mv /srv/yamale/site /srv/yamale/site.rollback \
  && sudo mv /srv/yamale/site.previous /srv/yamale/site'
```

## Not in here

The chain binary and its upgrades. A software upgrade is agreed by governance at
a height and applied by swapping the binary while the node is halted; that is a
different operation with a different failure mode, and putting it behind the
same command as a CSS change would be a mistake. See `docs/guides/upgrades.md`.
