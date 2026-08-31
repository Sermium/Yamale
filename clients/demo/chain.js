// Reading the running chain, three ways, because one way is not enough.
//
// THE TRANSPORT MAP, measured on 2026-08-31 against pay.yamalelegal.com:
//
//   /api/rpc/status              200   the head
//   /api/rpc/abci_query          200   every module's state, unauthenticated
//   /api/rpc/tx_search           200   the transaction index
//   /api/rpc/block               200   block headers, for real times
//   /api/rest/cosmos/...         200   the standard Cosmos modules
//   /api/rest/yamale/.../land    401   denied
//   /api/rest/yamale/.../paymsg  401   denied
//   /api/rest/yamale/.../netting 401   denied
//   /api/rest/.../enforcement    200   allowed
//
// The REST gate is an allowlist per module prefix with deny by default. Four
// modules are off it, and three of those carry mechanisms this page has to
// prove. ABCI answers all of them without a credential, so ABCI is the default
// here and REST is used only where it is both allowed and more convenient.
//
// THE RULE THIS FILE EXISTS TO ENFORCE: a proof that could not be read must
// say so. Not zero, not blank, not a dash. "Nought approved participants" and
// "the chain did not answer" are opposite facts about whether an institution
// can move money, and a tour page that renders them the same way is lying in
// the direction that flatters it.

import { fromBase64, toHex, write } from './proto.js';

const RPC = '/api/rpc';
const REST = '/api/rest';

/** Long enough for a bad connection, short enough to fail rather than hang. */
const TIMEOUT_MS = 12000;

/** A call that did not answer, named, with the reason kept. */
export class Unreachable extends Error {
  constructor(what, status, detail) {
    super(detail ? `${what} → ${status}: ${detail}` : `${what} → ${status}`);
    this.what = what;
    this.status = status;
    this.detail = detail ?? '';
  }
}

/** A call the chain answered by saying the thing does not exist. Not a fault. */
export class NotFound extends Error {
  constructor(what) { super(`${what}: not found`); this.what = what; }
}

/* ========================================================================= */
/*  Not all at once, and RPC not at the same rate as REST                    */
/* ========================================================================= */

/*
 * THE GATEWAY RATE-LIMITS /api/rpc/ AND DOES NOT RATE-LIMIT /api/rest/.
 *
 * Measured on 2026-08-31, firing N identical requests at once and tallying the
 * statuses:
 *
 *     concurrency        /api/rpc/abci_query        /api/rest/…/params
 *          1             200 ×1                     200 ×1
 *          4             200 ×4                     200 ×4
 *          6             200 ×6                     —
 *          8             200 ×4, 503 ×4             —
 *         12             503 ×12                    200 ×12
 *
 * And, immediately after those bursts, twelve RPC requests issued strictly one
 * after another: ok=1, bad=11. That last line is the important one. A pure
 * concurrency limit would have passed all twelve, because only one was ever in
 * flight. It is a token bucket, it was empty, and it stayed empty.
 *
 * Pacing, measured the same way once the bucket had refilled:
 *
 *     one request every 1000ms   9 of 10
 *     one request every  500ms  10 of 10
 *     one request every  300ms   9 of 10
 *     one request every  200ms   4 of 10
 *
 * So RPC sustains roughly two to three requests a second and collapses above
 * it. 500ms is the gap used here rather than 300, because those figures were
 * measured after eight idle seconds and the bucket in practice is already part
 * drained by whatever loaded the page — at 350ms, with the browser warm, about
 * half the calls still took a 503 and a retry, and one mechanism ran out of
 * attempts. At 500ms with four attempts they all land.
 *
 * WHY THIS PAGE CARES MORE THAN MOST. Four modules — x/land, x/paymsg,
 * x/netting, x/builderfee — are not on the gateway's REST allowlist, so the
 * mechanisms that depend on them can only be read over ABCI, which is RPC. The
 * first version of this page opened by firing every query at once and, on a
 * chain that was answering perfectly, showed the six land-and-payments
 * mechanisms as "cannot reach the chain". Honest, and wrong: the page had done
 * it to itself, and "we overwhelmed the node" is not a thing to explain to a
 * finance ministry mid-sentence.
 *
 * Two lanes, therefore. RPC is serialised at one request every 350ms — about
 * fourteen calls, so roughly five seconds for the whole page, with panels
 * filling in as they land. REST keeps a plain concurrency gate of six.
 */

/**
 * The two numbers the measurement above produced, exported so the tests can
 * turn the waiting off.
 *
 * Not because the pacing is optional — it is the difference between the tour
 * working and the tour blaming the chain for something the page did — but
 * because a suite that asserts on failure behaviour would otherwise spend two
 * minutes asleep proving it, and a slow suite is a suite that stops being run.
 */
export const TUNING = { rpcGapMs: 500, backoffMs: [0, 900, 2200, 4000] };

/** REST: a concurrency gate. It is not rate-limited, only finite. */
const MAX_REST_IN_FLIGHT = 6;
let restActive = 0;
const restQueue = [];
const restAcquire = () => (restActive < MAX_REST_IN_FLIGHT
  ? (restActive += 1, Promise.resolve())
  : new Promise((resolve) => restQueue.push(resolve)));
const restRelease = () => { const next = restQueue.shift(); if (next) next(); else restActive -= 1; };

/**
 * RPC: a paced queue, one at a time, never closer together than the gap.
 *
 * A chain of promises rather than a timer, so the pacing survives a slow
 * request — the gap is measured from when the previous call *finished*, which
 * is what the bucket actually refills against.
 */
let rpcTail = Promise.resolve();
function rpcTicket() {
  const wait = rpcTail.then(() => new Promise((r) => setTimeout(r, TUNING.rpcGapMs)));
  // The tail must not reject, or one failure poisons every queued request
  // behind it and the rest of the page silently never asks.
  rpcTail = wait.catch(() => {});
  return wait;
}

const isRpc = (url) => url.startsWith(RPC);

/** A transient gateway failure, distinguished from the chain being down. */
const TRANSIENT = new Set([429, 502, 503, 504]);

async function attempt(url) {
  if (isRpc(url)) {
    await rpcTicket();
    return fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  }
  await restAcquire();
  try {
    return await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  } finally {
    restRelease();
  }
}

async function json(url, what) {
  let res;
  let lastError;
  // Three attempts, backing off, and only for the failures that are genuinely
  // transient. Not a general retry loop: a chain halted for an upgrade must be
  // reported as unreachable within a few seconds, not hidden behind half a
  // minute of hopeful re-asking while somebody stands in front of a room.
  const BACKOFF = TUNING.backoffMs;
  for (let go = 0; go < BACKOFF.length; go += 1) {
    if (BACKOFF[go]) await new Promise((r) => setTimeout(r, BACKOFF[go]));
    try {
      res = await attempt(url);
    } catch (e) {
      lastError = new Unreachable(what, 'unreachable',
        e.name === 'TimeoutError' ? 'timed out' : e.message);
      // A refused or aborted connection is not retried. Only a timeout is:
      // a request that queued and ran out of patience is the one case where
      // asking again is likely to work.
      if (e.name !== 'TimeoutError') break;
      continue;
    }
    if (TRANSIENT.has(res.status)) {
      lastError = new Unreachable(what, res.status, '');
      res = undefined;
      continue;
    }
    break;
  }
  if (!res) throw lastError;

  const text = await res.text();
  if (!res.ok) {
    // 401 here is the proxy's allowlist, not a missing password. Named as such
    // so the page can say the true thing instead of asking for credentials.
    throw new Unreachable(what, res.status, res.status === 401 ? 'the gateway denies this path' : '');
  }
  try {
    return JSON.parse(text);
  } catch {
    throw new Unreachable(what, 'malformed', 'the gateway returned something that is not JSON');
  }
}

/* ========================================================================= */
/*  ABCI                                                                     */
/* ========================================================================= */

/**
 * One ABCI query. `data` is the hex-encoded protobuf request, because
 * CometBFT's URI layer takes a `0x…` string for a byte parameter and avoids the
 * quoting the base64 form needs.
 *
 * Every answer carries the height it was true at. A claim about a chain without
 * the height it was read at is a claim asking to be believed about a moment it
 * will not name.
 */
export async function abci(path, requestBytes = new Uint8Array()) {
  const url = `${RPC}/abci_query?path=${encodeURIComponent(`"${path}"`)}`
    + `&data=0x${toHex(requestBytes)}`;
  const body = await json(url, path);
  if (body.error) throw new Unreachable(path, 'rpc error', body.error.message ?? '');

  const response = body.result?.response;
  if (!response) throw new Unreachable(path, 'malformed', 'no response body');

  if (response.code) {
    const log = String(response.log ?? '').toLowerCase();
    // The keeper's collections.ErrNotFound arrives as a non-zero code with the
    // reason in the log. Matched on the log rather than on the number: the
    // number is the SDK's and moves between versions.
    if (log.includes('not found') || log.includes('no such')) throw new NotFound(path);
    throw new Unreachable(path, `abci ${response.code}`, response.log || '');
  }
  return {
    bytes: fromBase64(response.value ?? ''),
    height: Number(response.height ?? '0'),
  };
}

/** The same, but a genuine "there is no such thing" comes back as null. */
export async function abciOrNull(path, requestBytes) {
  try { return await abci(path, requestBytes); }
  catch (e) { if (e instanceof NotFound) return null; throw e; }
}

/** A request message with a single uint64 in field 1. */
export const byId = (id) => write((w) => w.num(1, id));
/** A request message with a single string in field 1. */
export const byString = (s) => write((w) => w.string(1, s));

/* ========================================================================= */
/*  The head                                                                 */
/* ========================================================================= */

/**
 * Where the chain is.
 *
 * `catchingUp` is surfaced rather than hidden. A node replaying history answers
 * every query correctly for a height that is hours old, and a tour that shows
 * "no objection against this title" from four hours ago is worse than one that
 * shows nothing.
 */
export async function head() {
  const body = await json(`${RPC}/status`, 'the chain head');
  const info = body.result?.sync_info;
  const node = body.result?.node_info;
  if (!info) throw new Unreachable('the chain head', 'malformed', 'no sync_info');
  return {
    height: Number(info.latest_block_height ?? '0'),
    at: info.latest_block_time ?? '',
    catchingUp: Boolean(info.catching_up),
    network: node?.network ?? '',
    moniker: node?.moniker ?? '',
  };
}

/**
 * The time a block was written, from the node rather than from an average.
 *
 * No limiter of its own: every call goes through `json`, which holds the whole
 * page to four requests at a time. A second limiter here would only nest inside
 * that one, and two limiters that both think they are in charge is how a page
 * ends up deadlocked on its own queue.
 *
 * Memoised, because several proofs date the same block, and cached as the
 * promise rather than the result so a second caller arriving mid-flight waits
 * on the first request instead of starting another.
 *
 * A block the node will not serve — pruned, or beyond its retention — comes
 * back null and the caller shows the height instead of a guessed date.
 */
const blockTimes = new Map();

export function blockTime(height) {
  const key = String(height);
  if (!height) return Promise.resolve(null);
  if (blockTimes.has(key)) return blockTimes.get(key);
  const p = (async () => {
    try {
      const body = await json(`${RPC}/block?height=${encodeURIComponent(key)}`, `block ${key}`);
      return body.result?.block?.header?.time ?? null;
    } catch {
      return null;
    }
  })();
  blockTimes.set(key, p);
  return p;
}

/* ========================================================================= */
/*  The transaction index                                                    */
/* ========================================================================= */

const QUOTE = String.fromCharCode(39);

/**
 * Search the transaction index over RPC rather than REST.
 *
 * REST's /cosmos/tx/v1beta1/txs sits behind the operator's gate on this
 * deployment and answers 401; /api/rpc/tx_search answers the same question
 * unauthenticated. Measured, not assumed.
 *
 * A caveat this page states out loud wherever it uses the result: the index is
 * a record of what was **sent**, not of what is **held**. Where an office acts
 * through an x/group account the outer message is a group proposal, so
 * searching for the inner message type returns nothing even though the act
 * happened. That is why every state claim here is read from state, and the
 * index is used only to date things.
 */
export async function txSearch(query, { perPage = 5, order = 'desc' } = {}) {
  const url = `${RPC}/tx_search`
    + `?query=${encodeURIComponent(`"${query}"`)}`
    + `&per_page=${perPage}`
    + `&order_by=${encodeURIComponent(`"${order}"`)}`;
  const body = await json(url, 'the transaction index');
  if (body.error) throw new Unreachable('the transaction index', 'rpc error', body.error.message ?? '');
  const result = body.result;
  if (!result) throw new Unreachable('the transaction index', 'malformed', 'no result');
  return {
    total: Number(result.total_count ?? '0'),
    txs: (result.txs ?? []).map((t) => ({
      hash: t.hash,
      height: Number(t.height ?? '0'),
      code: Number(t.tx_result?.code ?? 0),
      gasUsed: Number(t.tx_result?.gas_used ?? 0),
      actions: (t.tx_result?.events ?? [])
        .filter((e) => e.type === 'message')
        .flatMap((e) => (e.attributes ?? [])
          .filter((a) => a.key === 'action')
          .map((a) => a.value)),
    })),
  };
}

export const bySender = (address) => `message.sender=${QUOTE}${address}${QUOTE}`;
export const byAction = (typeUrl) => `message.action=${QUOTE}${typeUrl}${QUOTE}`;

/* ========================================================================= */
/*  REST, where it is allowed                                                */
/* ========================================================================= */

/**
 * A REST read.
 *
 * Used for the standard Cosmos modules and for the yamale modules the gateway
 * allows, because their JSON is already decoded and writing a protobuf decoder
 * for a value that arrives as a string is work with no reader.
 *
 * A 401 raises like any other failure. The page must never render a gateway
 * denial as an empty result — that is the specific bug this whole file is
 * arranged against.
 */
export async function rest(path, what) {
  return json(`${REST}${path}`, what ?? path);
}
