// Reading the syndication layer.
//
// ===========================================================================
// EVERYTHING HERE SPEAKS ABCI, AND THE REASON IS MEASURED
// ===========================================================================
//
// The deployment's proxy allowlists REST paths per module and denies by
// default. Against x/land, on 2026-08-27:
//
//   $ curl -si https://pay.yamalelegal.com/api/rest/yamale/blockchain/land/v1/params
//     HTTP/1.1 401 Unauthorized
//     WWW-Authenticate: Basic realm="Yamale — supervisor access"
//
//   $ curl -s '…/api/rpc/abci_query?path="/blockchain.land.v1.Query/Params"&data=0x'
//     {"result":{"response":{"code":0,"value":"CgYIAxCA6kk=","height":"94464"}}}
//
// x/syndicate is the newest module on this chain, so its REST prefix is
// certainly not on that allowlist. Reading over ABCI is not a workaround: it is
// the interface the proxy publishes, and it is what clients/ladoum and
// clients/app both settled on for the same reason.
//
// ===========================================================================
// EVERY ANSWER CARRIES THE HEIGHT IT IS TRUE AT
// ===========================================================================
//
// "This deal has raised 340,000 of 500,000" is a claim about a moment. An
// investor sizing a cheque against the remaining room is going to be refused if
// that room was filled two blocks ago, and the height is what lets them see
// that their figure is stale rather than wrong.
//
// READ ONLY. This console composes no transaction and holds no key. Committing
// to a deal moves money, and a page that could do it would need a signer; what
// it does instead is show the exact command, so the reader can see what they
// would be signing before anything asks them to sign it.

import {
  BALANCE_QUERY,
  QUERY,
  balanceRequest,
  decodeBalance,
  fromBase64,
  read,
  toHex,
} from './proto.js';

const RPC = '/api/rpc';

/** Long enough for a one-bar connection, short enough to fail rather than hang. */
const TIMEOUT_MS = 12000;

/**
 * A call that did not answer, named.
 *
 * "Something went wrong" tells an investor nothing. They need to know whether
 * the chain said "this book is empty" or whether the chain did not answer,
 * because those are opposite facts about whether to wire money today.
 */
export class CallFailed extends Error {
  constructor(what, status, detail) {
    super(detail ? `${what} → ${status}: ${detail}` : `${what} → ${status}`);
    this.what = what;
    this.status = status;
  }
}

/* ========================================================================= */
/*  ABCI                                                                     */
/* ========================================================================= */

/**
 * One ABCI query, returning the response bytes and the height they are true at.
 *
 * `data` is the hex-encoded protobuf request. CometBFT's URI layer takes a
 * `0x…` string for a byte parameter, which avoids the quoting the base64 form
 * needs — and quoting inside a query string is where this kind of call usually
 * goes wrong.
 */
export async function abci(path, requestBytes = new Uint8Array()) {
  const url = `${RPC}/abci_query?path=${encodeURIComponent(`"${path}"`)}`
    + `&data=0x${toHex(requestBytes)}`;

  let res;
  try {
    res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  } catch (e) {
    throw new CallFailed(path, 'unreachable', e.message);
  }
  // A 503 here is the edge's rate limiter, not the chain.
  //
  // nginx allows 600r/m on the RPC zone - ten a second. The holding view asks
  // one Entitlement question per funded deal, and the deal list one per deal,
  // so a platform with a few dozen deals clears that in a burst. Measured on
  // 2026-09-13 against the sibling console: forty probes returned 38 x 200 and
  // 2 x 503.
  //
  // Retried once, after a pause longer than the limiter's window needs, because
  // a rate limit is transient by construction. A second 503 is reported rather
  // than hidden: at that point the edge is genuinely refusing, and a page that
  // retried for ever would hang instead of saying so.
  if (res.status === 503) {
    await new Promise((r) => setTimeout(r, 700));
    try {
      res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    } catch (e) {
      throw new CallFailed(path, 'unreachable', e.message);
    }
  }
  if (!res.ok) throw new CallFailed(path, res.status, '');

  const json = await res.json();
  if (json.error) throw new CallFailed(path, 'rpc error', json.error.message ?? '');

  const response = json.result?.response;
  if (!response) throw new CallFailed(path, 'malformed', 'no response body');

  // A non-zero ABCI code is the node refusing the query — an unknown path, a
  // pruned height, a key that does not exist. It is not an empty result, and
  // conflating the two is how "no such deal" comes to look like "this deal has
  // taken no commitments".
  if (response.code) {
    throw new CallFailed(path, `abci ${response.code}`, response.log || '');
  }
  return {
    bytes: fromBase64(response.value ?? ''),
    height: Number(response.height ?? '0'),
  };
}

/** The height the last answer was true at, for the line under every figure. */
export let lastHeight = 0;

/** One of x/syndicate's queries, decoded. */
export async function query(name, arg) {
  const q = QUERY[name];
  if (!q) throw new Error(`no such syndicate query: ${name}`);
  const { bytes, height } = await abci(q.path, q.request(arg));
  if (height > lastHeight) lastHeight = height;
  return { ...q.response(read(bytes)), height };
}

/**
 * Returns null when the chain answered "there is no such thing".
 *
 * Matched on the log rather than on the ABCI code, because the code is the
 * SDK's and moves between versions. The distinction being preserved is the one
 * every screen depends on: "no deal under that reference" and "the chain did
 * not answer" are opposite facts.
 */
export async function queryOrNull(name, arg) {
  try {
    return await query(name, arg);
  } catch (e) {
    const text = String(e.message ?? '').toLowerCase();
    if (text.includes('not found') || text.includes('no such') || text.includes('key not found')) {
      return null;
    }
    throw e;
  }
}

/* ------------------------------------------------------------ the chain head */

/**
 * Where the chain is, read from /status.
 *
 * `catchingUp` is surfaced rather than hidden. A node replaying history answers
 * every query correctly for a height that is hours old, and a book that says
 * "room for 160,000" from four hours ago is worse than one that says nothing.
 */
export async function head() {
  const res = await fetch(`${RPC}/status`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) throw new CallFailed('/status', res.status, '');
  const info = (await res.json())?.result?.sync_info;
  if (!info) throw new CallFailed('/status', 'malformed', 'no sync_info');
  return {
    height: Number(info.latest_block_height ?? '0'),
    at: new Date(String(info.latest_block_time ?? '')),
    // The CHAIN's clock, not the browser's. Every deadline on this page is
    // measured against it, so a laptop running fast cannot declare a live raise
    // closed.
    now: Math.floor(new Date(String(info.latest_block_time ?? '')).getTime() / 1000),
    catchingUp: Boolean(info.catching_up),
  };
}

/* ========================================================================= */
/*  The reads the page actually makes                                        */
/* ========================================================================= */

export const params = async () => (await query('Params')).params;

export const operators = async () => (await query('Operators')).operators ?? [];

export const deals = async () => (await query('Deals')).deals ?? [];

export async function deal(id) {
  const res = await queryOrNull('Deal', id);
  return res?.deal ?? null;
}

export async function dealByRef(reference) {
  const res = await queryOrNull('DealByRef', reference);
  return res?.deal ?? null;
}

export const commitments = async (dealId) =>
  (await query('Commitments', dealId)).commitments ?? [];

export const listings = async (dealId, liveOnly = false) =>
  (await query('Listings', { dealId, liveOnly })).listings ?? [];

/**
 * Whether an address may hold a deal token.
 *
 * `onboarded` comes from the chain's own boolean rather than from the presence
 * of a record, because an investor whose onboarding was WITHDRAWN still has a
 * record. Telling a struck-off investor they may bid is the mistake this
 * function exists not to make.
 */
export async function investor(address) {
  const res = await query('Investor', address);
  return { onboarded: Boolean(res.onboarded), investor: res.investor, height: res.height };
}

export const entitlement = async (dealId, holder) =>
  query('Entitlement', { dealId, holder });

/** A raw bank balance, for a holding in a deal the reader has not named. */
export async function balance(address, denom) {
  const { bytes, height } = await abci(BALANCE_QUERY, balanceRequest(address, denom));
  if (height > lastHeight) lastHeight = height;
  return { ...decodeBalance(bytes), height };
}
