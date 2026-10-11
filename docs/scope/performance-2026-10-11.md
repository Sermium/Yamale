# What yamale-us can do, measured 2026-10-11

The first measurements on yamale-us (OVH, 6 cores, 11 GB), the chain's master
validator since 2026-10-10. All numbers are from the live devnet-2, with pi-2 (a
Raspberry Pi) and oracle (a 1-core, 1 GB Oracle VM) as the other two validators.
Rerun with `scripts/perf/chain-load.mjs`; the harness and its funded test
accounts are kept on yamale-us at `/opt/yamale/perf` (root only).

## The chain

| Run | Sent | Mempool | Result |
|---|---|---|---|
| 1 | 500 transfers, 100 accounts | 500 (as carried over from the VM) | all 500 in one block, closed in 5.46 s: **92 tx/s**, and that is the mempool setting, not the chain |
| 2 | 5,000 transfers, 100 accounts | 10,000 | **4,483 in one block, closed in a normal 5.02 s**; all 5,000 committed in 10.3 s, **484 tx/s**, bounded by the one load client (~550 tx/s into the node) |

The server stayed at least 71% idle during run 2.

**The ceiling is not yamale-us. It is the two small validators.** After the
4,483-transaction block, pi-2 and oracle stopped signing while they executed it;
the chain carried on because the master holds 68.6%. The Pi was back within
seconds; the VM took about a minute. Neither came near being jailed (that needs
about four minutes of missed blocks). Sustained at run 2's rate, both would fall
behind for good and be jailed, and the auto-unjail would bring them back only
once they caught up.

**The mempool is set to 2,000** on yamale-us (it was 500): four times the old
ceiling, roughly 400 tx/s in bursts, with blocks small enough that the other two
validators keep up. Raise it further only with bigger machines beside it.

## The web

10 s each, 50 concurrent connections, from yamale-us itself.

| What | req/s | median | p99 |
|---|---|---|---|
| Company site, server-rendered per request (Node, direct) | 292 | 160 ms | 294 ms |
| Company site through Caddy and TLS | 324 | 133 ms | 497 ms |
| Wallet app, static (Caddy, then the gate) | 4,192 | 9 ms | 40 ms |
| Chain RPC `/status` (node, direct) | 9,441 | 3 ms | 23 ms |
| Chain REST, total supply (node, direct) | 1,352 | 34 ms | 85 ms |
| Public RPC through the gate, one client | 3,562 | 10 ms | 58 ms |

The last row is the per-visitor limit working: one address gets about 116
answers, then 429. The gate itself answered more than 3,500 requests a second.

**The company site is the slow one, and needlessly:** it renders every page on
every request in one Node process, though its pages are the same for everyone.
Caching them (or prerendering at build) would put it near the static figure.
