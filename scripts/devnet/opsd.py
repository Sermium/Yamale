#!/usr/bin/env python3
"""Devnet operations service — the signing half of the governance console.

The console at /governance is a static page. It can read the chain over the
REST API, but voting, freezing a wallet and opening a recovery case all need a
signature from a validator's key, and that key is on the validator's own host.
This is the small service that holds it: one fixed argv per action, signed with
the node's keyring, the same way the faucet signs its transfers.

THIS FILE IS THE ONLY COPY. Both hosts are deployed from it by deploy-opsd.sh,
because they drifted once already — the VM's map sent pi-2 to /opt/yamale/node
while pival's key has always lived on the Pi at /opt/yamale/join-node, so every
pi-2 request signed with whatever key that other home happened to hold. Edit
here, deploy, never edit in place on a host.

WHAT PROTECTS IT
----------------
Three things, and it is worth being precise about which does what, because the
first version of this file relied on only one of them and the other two were
assumed.

1. The bind address. OPSD_BIND is required and 0.0.0.0 is refused outright.
   The Pi's copy was bound to 0.0.0.0:8086, which meant the whole house LAN
   could reach the signer directly and skip nginx entirely — an unauthenticated
   path to a validator's governance vote, freeze and seizure powers. The VM
   binds loopback because nginx is on the same host. The Pi binds its tailnet
   address because nginx is on the VM and has to cross the tailnet to reach it.

2. OPSD_AUTH, an HTTP Basic credential checked here, in this process. It is
   mandatory whenever the bind address is not loopback: if the socket is
   reachable from another machine, something on this side of it has to ask who
   is calling. On the pi-2 path nginx checks the operator's own console
   password and then replaces the header with this service credential, so the
   two are independent — rotating an operator's password does not touch the
   wire, and the credential on the wire is not one a human types.

3. OPSD_VALIDATOR, the one validator this host may sign for. The map below is
   the same on both hosts, but a host holds one key, so the Pi refuses
   validator=pi and the VM refuses validator=pi-2. Without it a caller holding
   the pi-2 console password could ask the Pi to sign as pi, and the Pi would
   go looking for alice in a stale keyring it still has lying around.

WHAT IT IS STILL NOT: production-safe. On a real network a validator's key does
not sit behind an HTTP service at all; the equivalent of this file there is the
operator's terminal. It is here so the devnet can demonstrate the mechanism.

The allow-list below is the only thing standing between this and a shell: every
request is mapped to a fixed argv, never to a string the caller composed.

REPORTING WHAT HAPPENED
-----------------------
run() reports execution, not acceptance. A code of 0 on broadcast means CheckTx
passed — the transaction is well-formed and the fee is payable — and says
nothing about whether it succeeded when the block ran it. This service used to
report that code as success, so a freeze that failed in the block came back to
the console as "sent". tools/faucet/main.go had the identical bug and the same
fix: broadcast, keep the hash, then poll `query tx <hash>` until the block has
it and report the code the block produced.

The faucet keeps a deliberate 6s sleep after its fee-allowance transaction.
That is not copied here, and the reason it is not is the reason it exists
there: the allowance is fire-and-forget, so it is still in flight when the
handler returns, and the sleep is what stops the next request signing against a
sequence the chain has not committed. Every transaction this service sends is
waited on to completion, and a transaction that has been included has had its
sequence committed — so the wait already provides what that sleep buys, and
adding it would only make every request six seconds slower. The single-threaded
HTTPServer below is load-bearing for the same reason: requests are handled one
at a time, so this service never has two transactions from one key in flight.
"""
import base64
import hmac
import ipaddress
import json
import os
import subprocess
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

BIN = os.environ.get("OPSD_BIN", "/opt/yamale/bin/blockchaind")
CHAIN = os.environ.get("OPSD_CHAIN", "yamale-devnet-1")
NODE = os.environ.get("OPSD_NODE", "tcp://localhost:26657")

# Which validator signs from which key and node home. Requests name a
# validator; they never name a key or a path, so a caller cannot reach a key we
# did not intend to expose. Both hosts carry the whole map and each is held to
# one entry by OPSD_VALIDATOR, so the map is a single fact rather than two that
# have to be kept in agreement.
VALIDATORS = {
    "pi":   {"key": "alice", "home": "/opt/yamale/node"},       # signs on the VM
    "pi-2": {"key": "pival", "home": "/opt/yamale/join-node"},  # signs on the Pi
}

VOTE_OPTIONS = {"yes", "no", "abstain", "no_with_veto"}

# Budgeted against nginx's proxy_read_timeout of 70s: a broadcast that hangs
# for the full 20, then polling to 35, still answers before the proxy gives up
# and the operator is left not knowing whether they froze somebody.
BROADCAST_TIMEOUT = 20
QUERY_TIMEOUT = 15
CONFIRM_DEADLINE = 35
POLL_INTERVAL = 2

# A sequence mismatch. It means another transaction from this key was still in
# the mempool — an operator at a terminal, most likely, since this service
# serialises itself — and it is worth exactly one retry after a block.
SEQUENCE_MISMATCH = 32
BLOCK = 6

_addresses = {}


def address_of(who):
    """The signer's own address, which open-case and withdraw-case take as an argument.

    The chain wants the opener spelled out rather than inferred from the
    signature, so the key name the console sends has to be resolved to an
    address before the argv can be built. Cached: it is a keyring read that
    cannot change while the process is up.
    """
    cache_key = (who["key"], who["home"])
    if cache_key not in _addresses:
        out = subprocess.run(
            [BIN, "keys", "show", who["key"], "-a",
             "--keyring-backend", "test", "--home", who["home"]],
            capture_output=True, text=True, timeout=QUERY_TIMEOUT)
        addr = (out.stdout or "").strip()
        if not addr.startswith("yml1"):
            raise RuntimeError(
                "cannot resolve %s in %s: %s"
                % (who["key"], who["home"], ((out.stderr or "") or addr)[:200]))
        _addresses[cache_key] = addr
    return _addresses[cache_key]


def broadcast(args, home):
    """Send the transaction and return its hash, or raise with what was refused.

    The code checked here is CheckTx's. It is the mempool's opinion, and the
    only thing it is trusted for is deciding whether there is a hash worth
    waiting on.
    """
    argv = [
        BIN, *args,
        "--chain-id", CHAIN,
        "--keyring-backend", "test",
        "--home", home,
        "--node", NODE,
        "--fees", "20000uyml",
        "--gas", "600000",
        "--yes", "-o", "json",
    ]

    for attempt in range(2):
        out = subprocess.run(argv, capture_output=True, text=True,
                             timeout=BROADCAST_TIMEOUT)
        try:
            parsed = json.loads(out.stdout)
        except Exception:
            # No JSON at all: the binary refused before it built a transaction.
            # Its stderr is the only account of why.
            raise RuntimeError(((out.stderr or "") + (out.stdout or "")).strip()[:400]
                               or "the node returned nothing")

        code = parsed.get("code", 1)
        if code == 0:
            txhash = parsed.get("txhash", "")
            if not txhash:
                raise RuntimeError("accepted without a transaction hash")
            return txhash

        if code == SEQUENCE_MISMATCH and attempt == 0:
            time.sleep(BLOCK)
            continue

        raise RuntimeError("refused on broadcast (code %s): %s"
                           % (code, str(parsed.get("raw_log", ""))[:400]))

    raise RuntimeError("refused on broadcast: the account sequence never settled")


def confirm(txhash, home):
    """Wait for the block and return the code it produced.

    Until this returns, nothing has happened. A freeze reported from the
    broadcast code is a freeze that may not exist.
    """
    argv = [BIN, "query", "tx", txhash, "--node", NODE,
            "--home", home, "-o", "json"]

    deadline = time.time() + CONFIRM_DEADLINE
    while True:
        time.sleep(POLL_INTERVAL)

        out = subprocess.run(argv, capture_output=True, text=True,
                             timeout=QUERY_TIMEOUT)
        if out.returncode == 0:
            try:
                parsed = json.loads(out.stdout)
            except Exception:
                parsed = None
            if parsed is not None and "code" in parsed:
                return int(parsed.get("code", 1)), str(parsed.get("raw_log", ""))[:400]

        if time.time() > deadline:
            # Not proven either way, and reported as a failure on purpose. An
            # operator told a freeze failed will look, and find it if it landed;
            # one told it succeeded when it did not will walk away from a wallet
            # they believe is stopped.
            raise RuntimeError(
                "broadcast as %s but not included within %ds — query it before retrying"
                % (txhash, CONFIRM_DEADLINE))


def run(args, home):
    """Sign, broadcast, wait for the block, and report what the block did."""
    try:
        txhash = broadcast(args, home)
    except subprocess.TimeoutExpired:
        return {"ok": False, "stage": "broadcast", "log": "the node did not answer in time"}
    except Exception as exc:
        return {"ok": False, "stage": "broadcast", "log": str(exc)[:400]}

    try:
        code, log = confirm(txhash, home)
    except subprocess.TimeoutExpired:
        return {"ok": False, "stage": "confirm", "txhash": txhash,
                "log": "the node stopped answering while waiting for the block"}
    except Exception as exc:
        return {"ok": False, "stage": "confirm", "txhash": txhash, "log": str(exc)[:400]}

    return {"ok": code == 0, "stage": "executed", "code": code, "txhash": txhash,
            "log": log if code != 0 else ""}


class Handler(BaseHTTPRequestHandler):
    def _reply(self, status, body):
        raw = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        if status == 401:
            self.send_header("WWW-Authenticate", 'Basic realm="yamale-ops"')
        self.end_headers()
        self.wfile.write(raw)

    def _authorised(self):
        """Constant-time check of the service credential.

        Skipped only when the socket is on loopback, where the kernel has
        already established that the caller is on this host and nginx is the
        thing in front of it.
        """
        if not AUTH:
            return True
        header = self.headers.get("Authorization", "")
        if not header.startswith("Basic "):
            return False
        try:
            supplied = base64.b64decode(header[6:].strip()).decode()
        except Exception:
            return False
        return hmac.compare_digest(supplied, AUTH)

    def do_POST(self):
        if not self._authorised():
            return self._reply(401, {"error": "unauthorised"})

        try:
            length = int(self.headers.get("Content-Length", 0))
            req = json.loads(self.rfile.read(length) or b"{}")
        except Exception:
            return self._reply(400, {"error": "bad request"})

        name = req.get("validator", "")
        # This host holds one key. Naming the other validator is not a typo to
        # be tolerated — it is a request to sign as somebody whose key is
        # somewhere else, and the honest answer is no.
        if name != VALIDATOR:
            return self._reply(403, {"error": "this host signs only for %s" % VALIDATOR})
        who = VALIDATORS[name]

        try:
            opener = address_of(who)
        except Exception as exc:
            return self._reply(500, {"error": str(exc)[:200]})

        path = self.path.rstrip("/")

        if path.endswith("/vote"):
            option = str(req.get("option", "")).lower()
            proposal = str(req.get("proposal", ""))
            if option not in VOTE_OPTIONS or not proposal.isdigit():
                return self._reply(400, {"error": "bad vote"})
            return self._reply(200, run(
                ["tx", "gov", "vote", proposal, option, "--from", who["key"]],
                who["home"]))

        # Freeze and seize are the same message with a different action. The
        # endpoint stays named /freeze because the console calls it that, and
        # because to an operator they are two different acts: one stops the
        # money for a day on one signature, the other starts a vote to take it.
        if path.endswith("/freeze"):
            addr = str(req.get("address", ""))
            reason = str(req.get("reason", ""))[:512]
            if not addr.startswith("yml1") or not reason:
                return self._reply(400, {"error": "address and reason required"})
            return self._reply(200, run(
                ["tx", "enforcement", "open-case", opener, addr, "freeze",
                 "--reason", reason, "--from", who["key"]], who["home"]))

        if path.endswith("/case"):
            addr = str(req.get("address", ""))
            reason = str(req.get("reason", ""))[:512]
            evidence = str(req.get("evidence", ""))[:256]
            evidence_hash = str(req.get("evidence_hash", "")).strip()
            if not addr.startswith("yml1") or not reason:
                return self._reply(400, {"error": "address and reason required"})
            # The chain refuses a seizure without both halves when
            # seize_requires_evidence is set, and refusing here says so in
            # words rather than spending a fee to be told the same thing.
            # The hash is of the evidence document, not of its URI: the point
            # is that a document quietly edited later no longer matches.
            if not evidence or not evidence_hash:
                return self._reply(400, {
                    "error": "a recovery case needs an evidence URI and the "
                             "SHA-256 of the evidence it points at"})
            return self._reply(200, run(
                ["tx", "enforcement", "open-case", opener, addr, "seize",
                 "--reason", reason,
                 "--evidence-uri", evidence,
                 "--evidence-hash", evidence_hash,
                 "--from", who["key"]], who["home"]))

        self._reply(404, {"error": "no such action"})

    def log_message(self, *_):
        pass  # journald already timestamps; the default log is noise


def configure():
    """Read the per-host configuration, and refuse to start if it is unsafe.

    These are exits rather than warnings on purpose. The failure being guarded
    is a signer silently listening on every interface, which looks exactly like
    a working service until somebody finds it.
    """
    bind = os.environ.get("OPSD_BIND", "").strip()
    if not bind:
        sys.exit("OPSD_BIND is required: the address to listen on, never a wildcard")
    if bind in ("0.0.0.0", "::", "*"):
        sys.exit("OPSD_BIND=%s would expose a validator's signing key on every "
                 "interface; bind loopback, or the tailnet address with OPSD_AUTH set" % bind)

    try:
        loopback = ipaddress.ip_address(bind).is_loopback
    except ValueError:
        sys.exit("OPSD_BIND=%s is not an IP address" % bind)

    auth = os.environ.get("OPSD_AUTH", "").strip()
    if not loopback and not auth:
        sys.exit("OPSD_BIND=%s is reachable from other hosts, so OPSD_AUTH "
                 "(user:password) is required" % bind)
    if auth and ":" not in auth:
        sys.exit("OPSD_AUTH must be user:password")

    validator = os.environ.get("OPSD_VALIDATOR", "").strip()
    if validator not in VALIDATORS:
        sys.exit("OPSD_VALIDATOR must be one of: %s" % ", ".join(sorted(VALIDATORS)))

    port = int(os.environ.get("OPSD_PORT", "8086"))
    return bind, port, auth, validator


if __name__ == "__main__":
    BIND, PORT, AUTH, VALIDATOR = configure()
    print("opsd: signing for %s on %s:%d (auth %s)"
          % (VALIDATOR, BIND, PORT, "on" if AUTH else "off, loopback only"),
          flush=True)
    # Single-threaded on purpose: see the note on sequences at the top of the
    # file. One transaction at a time from one key is the whole point.
    HTTPServer((BIND, PORT), Handler).serve_forever()
