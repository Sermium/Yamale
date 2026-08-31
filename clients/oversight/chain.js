/**
 * Reading x/enforcement and x/netting over ABCI.
 *
 * Every read on this page goes through `abci` below. Nothing here is
 * authenticated, nothing here needs a key, and nothing here can write. That is
 * deliberate and is the point of the console: an oversight surface whose
 * reader has to be trusted with a credential is not oversight, it is an
 * administration tool with a nicer name.
 *
 * See proto.js for why this is ABCI rather than REST.
 */

import * as P from './proto.js';

/** Where the node is. Overridable so this file can be pointed at a local node
 *  or a different network without an edit — a console with the endpoint
 *  compiled in reads one chain and claims to read whichever one you opened it
 *  against. */
export const DEFAULT_RPC = '/api/rpc';

/**
 * A failure that names the call that failed.
 *
 * "Something went wrong" is not an acceptable answer on this page. On an
 * enforcement console the reader has to be able to tell apart three things
 * that all look like an empty screen: nobody is frozen, the node did not
 * answer, and the query is not one this node serves. Each of those gets a
 * different sentence, and they come from here.
 */
export class QueryFailed extends Error {
  constructor(path, kind, detail) {
    super(`${path} — ${kind}${detail ? `: ${detail}` : ''}`);
    this.path = path;
    this.kind = kind;   // 'unreachable' | 'http' | 'rpc' | 'abci' | 'decode'
    this.detail = detail || '';
  }
}

/**
 * One ABCI query.
 *
 * The node returns HTTP 200 with a JSON-RPC envelope even when the query
 * failed, and returns `response.code != 0` with the reason in `response.log`
 * even when the envelope succeeded. Both are checked. Collapsing them would
 * turn "no such case" into "the chain is down", and a page that cries down
 * when it is merely empty is a page nobody keeps open.
 */
export async function abci(path, requestBytes, { rpc = DEFAULT_RPC, height } = {}) {
  const data = `0x${P.toHex(requestBytes || new Uint8Array(0))}`;
  const url = `${rpc}/abci_query?path=${encodeURIComponent(`"${path}"`)}`
    + `&data=${data}${height ? `&height=${height}` : ''}`;

  let res;
  try {
    res = await fetch(url);
  } catch (e) {
    throw new QueryFailed(path, 'unreachable', e && e.message);
  }
  if (!res.ok) throw new QueryFailed(path, 'http', String(res.status));

  let body;
  try {
    body = await res.json();
  } catch (e) {
    throw new QueryFailed(path, 'decode', 'the node did not answer with JSON');
  }
  if (body.error) {
    throw new QueryFailed(path, 'rpc', body.error.data || body.error.message || '');
  }
  const r = body.result && body.result.response;
  if (!r) throw new QueryFailed(path, 'rpc', 'no response in the envelope');
  if (r.code && r.code !== 0) {
    throw new QueryFailed(path, 'abci', r.log || `code ${r.code}`);
  }
  return { bytes: P.fromBase64(r.value), height: Number(r.height || 0) };
}

/** Runs a query and decodes it. `at` is the height the node answered at, which
 *  every panel shows: a number without a height is a claim about now that was
 *  true at some point in the past. */
export async function query(path, requestBytes, schema, opts) {
  const { bytes, height } = await abci(path, requestBytes, opts);
  let value;
  try {
    value = P.decode(bytes, schema);
  } catch (e) {
    throw new QueryFailed(path, 'decode', e && e.message);
  }
  return { value, at: height };
}

// ---------------------------------------------------------------------------
// The node's own clock and validator set.
//
// Not module state, but the console cannot say anything useful about a freeze
// without them: "expires at height 120640" is not an answer to "how long have
// I got", and "two thirds" is not an answer to "who has to agree".
// ---------------------------------------------------------------------------

export async function status(rpc = DEFAULT_RPC) {
  const res = await fetch(`${rpc}/status`).catch((e) => {
    throw new QueryFailed('/status', 'unreachable', e && e.message);
  });
  if (!res.ok) throw new QueryFailed('/status', 'http', String(res.status));
  const b = await res.json();
  const s = b.result.sync_info;
  return {
    chainId: b.result.node_info.network,
    height: Number(s.latest_block_height),
    time: s.latest_block_time,
    catchingUp: !!s.catching_up,
    moniker: b.result.node_info.moniker,
  };
}

/**
 * The consensus validator set and its voting power.
 *
 * This is the set a seizure vote is measured against. It is read rather than
 * assumed because the whole claim this console makes — that taking money needs
 * two thirds — is a claim about *these* numbers, and a page that asserts the
 * asymmetry without showing the arithmetic is asking to be believed rather
 * than showing its work.
 */
export async function validators(rpc = DEFAULT_RPC) {
  const out = [];
  let page = 1;
  for (;;) {
    const res = await fetch(`${rpc}/validators?per_page=100&page=${page}`).catch((e) => {
      throw new QueryFailed('/validators', 'unreachable', e && e.message);
    });
    if (!res.ok) throw new QueryFailed('/validators', 'http', String(res.status));
    const b = await res.json();
    if (b.error) throw new QueryFailed('/validators', 'rpc', b.error.data || b.error.message);
    const r = b.result;
    for (const v of r.validators || []) {
      out.push({ address: v.address, power: Number(v.voting_power) });
    }
    if (out.length >= Number(r.total || 0) || !(r.validators || []).length) break;
    page += 1;
    if (page > 20) break; // a set this large is not this network; stop rather than loop
  }
  out.sort((a, b) => b.power - a.power);
  return out;
}

/** Block time for a height, so a countdown can be stated in hours rather than
 *  in blocks. Cached: a page showing ten freezes must not ask ten times. */
const blockTimes = new Map();
export async function blockTime(height, rpc = DEFAULT_RPC) {
  const key = String(height);
  if (blockTimes.has(key)) return blockTimes.get(key);
  const p = (async () => {
    const res = await fetch(`${rpc}/block?height=${height}`);
    if (!res.ok) return null;
    const b = await res.json();
    return b.result && b.result.block && b.result.block.header
      ? b.result.block.header.time : null;
  })().catch(() => null);
  blockTimes.set(key, p);
  return p;
}

// ---------------------------------------------------------------------------
// x/enforcement.
//
// The type URL prefix is `blockchain.` and not `yamale.blockchain.` — the
// proto package is `blockchain.enforcement.v1` (see the proto files and
// x/enforcement/types/*.pb.go), while the REST gateway paths carry a
// `/yamale/blockchain/...` prefix that belongs to the HTTP annotation and to
// nothing else. Mixing the two produces a query the node routes nowhere and
// answers with an unhelpful error.
// ---------------------------------------------------------------------------

const ENF = '/blockchain.enforcement.v1.Query';

export const enforcement = {
  params: (o) => query(`${ENF}/Params`, null, P.QueryEnforcementParamsResponse, o),
  openCases: (o) => query(`${ENF}/OpenCases`, null, P.QueryOpenCasesResponse, o),
  heldCases: (o) => query(`${ENF}/HeldCases`, null, P.QueryHeldCasesResponse, o),
  recovered: (o) => query(`${ENF}/Recovered`, null, P.QueryRecoveredResponse, o),
  seizureWindow: (o) => query(`${ENF}/SeizureWindow`, null, P.QuerySeizureWindowResponse, o),

  /** Every case, newest first. `reverse` on the page request is what makes the
   *  most recent case the first row without reading the whole history. */
  listCases: (limit = 50, o) => query(
    `${ENF}/ListCase`,
    P.encodeSub(1, P.encode({ 3: limit, 5: true })),
    P.QueryListCaseResponse,
    o,
  ),

  listFreezes: (limit = 100, o) => query(
    `${ENF}/ListFreeze`,
    P.encodeSub(1, P.encode({ 3: limit })),
    P.QueryListFreezeResponse,
    o,
  ),

  /** Case ids start at ONE. x/enforcement's InitGenesis seeds the sequence at
   *  1 and coerces a genesis count of 0 up to 1, for the reason the keeper
   *  states in as many words: a case id of zero is indistinguishable from an
   *  unset field, and "frozen by case 0" is an accusation nobody could look up.
   *
   *  This is not a general rule on this chain — x/land's transfers start at 0
   *  — so it is verified here rather than assumed. The practical consequence:
   *  a Freeze carrying case_id 0 is a freeze with NO case attached, not a
   *  freeze attached to the first case, and oversight.js reads it that way.
   *
   *  Note also that requesting id 0 encodes to an empty buffer, since proto3
   *  omits a zero. That is correct and the server decodes it back to 0 — but
   *  since no case 0 exists, it will answer not-found. */
  getCase: (id, o) => query(`${ENF}/GetCase`, P.encode({ 1: id }), P.QueryGetCaseResponse, o),

  caseVotes: (id, o) => query(`${ENF}/CaseVotes`, P.encode({ 1: id }), P.QueryCaseVotesResponse, o),

  freezeStatus: (address, o) => query(
    `${ENF}/FreezeStatus`, P.encode({ 1: address }), P.QueryFreezeStatusResponse, o,
  ),
};

// ---------------------------------------------------------------------------
// x/netting.
// ---------------------------------------------------------------------------

const NET = '/blockchain.netting.v1.Query';

export const netting = {
  params: (o) => query(`${NET}/Params`, null, P.QueryNettingParamsResponse, o),
  currentCycle: (o) => query(`${NET}/CurrentCycle`, null, P.QueryCurrentCycleResponse, o),
  heldSlices: (o) => query(`${NET}/HeldSlices`, null, P.QueryHeldSlicesResponse, o),
  cycle: (id, o) => query(`${NET}/Cycle`, P.encode({ 1: id }), P.QueryCycleResponse, o),
  position: (participant, o) => query(
    `${NET}/Position`, P.encode({ 1: participant }), P.QueryPositionResponse, o,
  ),
  obligations: (participant, cycleId, limit = 100, o) => query(
    `${NET}/ParticipantObligations`,
    P.concat(
      P.encode({ 1: participant, 2: cycleId }),
      P.encodeSub(3, P.encode({ 3: limit })),
    ),
    P.QueryParticipantObligationsResponse,
    o,
  ),
};

/**
 * Runs a set of reads and keeps the failures alongside the successes.
 *
 * The panels on this page fail independently on purpose. A netting query that
 * errors must not blank the enforcement half, because the enforcement half is
 * the one somebody opened the page to read. `Promise.all` would do the
 * opposite: one refusal and the whole console says nothing.
 */
export async function settleAll(spec) {
  const keys = Object.keys(spec);
  const results = await Promise.allSettled(keys.map((k) => spec[k]));
  const out = {};
  for (let i = 0; i < keys.length; i += 1) {
    const r = results[i];
    out[keys[i]] = r.status === 'fulfilled'
      ? { ok: true, ...r.value }
      : { ok: false, error: r.reason };
  }
  return out;
}
