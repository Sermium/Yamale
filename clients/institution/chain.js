/**
 * Reading what the institution needs, over REST.
 *
 * REST and not abci_query, which is what the other consoles use. Measured
 * 2026-09-15 against the deployment's proxy: both prefixes below answer 200,
 * while several other modules' are denied. REST returns JSON, so this console
 * transcribes no protobuf at all. Re-measure before changing this.
 *
 * READ ONLY. This console holds no key and composes no transaction.
 */

const REST = '/api/rest';
const TIMEOUT_MS = 12000;

async function get(path) {
  const res = await fetch(`${REST}${path}`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  const body = await res.json().catch(() => null);
  return { status: res.status, body };
}

/** The institutions that may record where an account belongs. */
export async function participants() {
  const { status, body } = await get('/yamale/blockchain/paymsg/v1/approved_participant');
  if (status !== 200) throw new Error(`the register did not answer (${status})`);
  return (body?.approved_participant ?? []).map((r) => ({
    participant: r.participant ?? '',
    code: r.code ?? '',
    name: r.name ?? '',
  }));
}

/**
 * The jurisdiction recorded against an account, or null.
 *
 * Null is an ANSWER and not a failure: the chain replies
 * `{"code":5,"message":"this account has no recorded jurisdiction"}` for an
 * account nobody has placed, which is the normal state of every account this
 * console exists to serve. Reporting it as an error would tell an institution
 * the chain was broken at the exact moment it was working.
 */
export async function jurisdiction(address) {
  const { status, body } = await get(`/yamale/blockchain/alias/v1/jurisdiction/${encodeURIComponent(address)}`);
  if (status === 200 && body?.jurisdiction) return body.jurisdiction;
  // 404 carries this one: the gateway maps gRPC NotFound onto it, so the status
  // alone cannot separate the answer from a failure. The code can.
  if (body?.code === 5) return null;
  throw new Error(body?.message ?? `the chain did not answer (${status})`);
}

/**
 * Whether the chain has ever seen this account.
 *
 * Three outcomes and not two, because the gateway spells them differently and
 * conflating them puts a false sentence on the screen. Measured 2026-09-15:
 * an address nobody has used answers 404 with code 5 — an answer. An address
 * whose bech32 checksum does not hold answers 500 with code 2, and reporting
 * that as 'the chain has never seen this account' tells somebody their customer
 * is unknown when in fact the paste is corrupt. So anything that is not a clean
 * found/not-found is raised, and the page prints what the chain actually said.
 */
export async function accountExists(address) {
  const { status, body } = await get(`/cosmos/auth/v1beta1/accounts/${encodeURIComponent(address)}`);
  if (status === 200) return true;
  if (status === 404 && body?.code === 5) return false;
  throw new Error(body?.message ?? `the chain did not answer (${status})`);
}

/** The identifier issued to an account, or null. */
export async function userId(address) {
  const { status, body } = await get(`/yamale/blockchain/alias/v1/alias_of/${encodeURIComponent(address)}`);
  if (status === 200 && body?.alias?.id) return body.alias.id;
  return null;
}
