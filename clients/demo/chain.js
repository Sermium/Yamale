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

async function json(url, what) {
  let res;
  try {
    res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  } catch (e) {
    // A halted node, a dropped tunnel, a browser offline. All the same to the
    // reader: the chain is not answering right now.
    throw new Unreachable(what, 'unreachable', e.name === 'TimeoutError' ? 'timed out' : e.message);
  }
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
 * Held to three at a time: this page asks for a dozen block times at once and
 * firing them all is fine on a desk and miserable on the connection it is
 * actually opened on, where the browser's own limit turns a burst into a queue
 * with nothing rendering behind it.
 */
const blockTimes = new Map();
let inFlight = 0;
const waiting = [];
const slot = () => (inFlight < 3
  ? (inFlight += 1, Promise.resolve())
  : new Promise((r) => waiting.push(r)));
const release = () => { const next = waiting.shift(); if (next) next(); else inFlight -= 1; };

export function blockTime(height) {
  const key = String(height);
  if (!height) return Promise.resolve(null);
  if (blockTimes.has(key)) return blockTimes.get(key);
  const p = (async () => {
    await slot();
    try {
      const body = await json(`${RPC}/block?height=${encodeURIComponent(key)}`, `block ${key}`);
      return body.result?.block?.header?.time ?? null;
    } catch {
      return null;   // pruned, or the node will not serve it. Show the height.
    } finally {
      release();
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
