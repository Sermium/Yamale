/**
 * Reading the chain without a password: ABCI queries, the chain head and
 * tx_search, over the node's CometBFT RPC.
 *
 * Promoted from clients/herd/src/abci.ts, which explains why reads go over
 * ABCI rather than REST: the public proxy allowlists REST paths per module and
 * answers the rest with a 401, which a browser renders as a login box. The one
 * change is that the RPC base can be set, so a Node script can point at a node
 * while a browser keeps the same-origin proxy. herd and rwa keep their own
 * copies for now.
 *
 * Every read returns an Outcome, never a value-or-null: "the chain says there
 * is no such jewel" and "the chain did not answer" must not render alike.
 */

export type Outcome<T> =
  | { ok: true; value: T; height: number }
  | { ok: false; reason: 'not-found' | 'refused' | 'unreachable' | 'offline'; detail: string };

let rpcBase = '/api/rpc';
const TIMEOUT_MS = 9000;

export function setRpcBase(base: string): void {
  rpcBase = base.replace(/\/+$/, '');
}

function offline(): boolean {
  return typeof navigator !== 'undefined' && navigator.onLine === false;
}

function hex(bytes: Uint8Array): string {
  let out = '0x';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}

function fromBase64(value: string): Uint8Array {
  if (value === '') return new Uint8Array(0);
  const bin = atob(value);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function rpc(path: string): Promise<{ ok: true; json: any } | { ok: false; detail: string }> {
  try {
    const res = await fetch(`${rpcBase}${path}`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) return { ok: false, detail: `http ${res.status}` };
    return { ok: true, json: await res.json() };
  } catch (err) {
    return { ok: false, detail: err instanceof Error ? err.message : String(err) };
  }
}

/**
 * The path is wrapped in literal double quotes before encoding: CometBFT reads
 * the parameter as a JSON string, and an unquoted path is refused as an
 * unknown query, which reads exactly like a module that is not installed.
 */
export async function abciQuery(path: string, request: Uint8Array = new Uint8Array(0)): Promise<Outcome<Uint8Array>> {
  if (offline()) return { ok: false, reason: 'offline', detail: path };
  const r = await rpc(`/abci_query?path=${encodeURIComponent(`"${path}"`)}&data=${hex(request)}`);
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  if (r.json?.error) return { ok: false, reason: 'unreachable', detail: r.json.error.message ?? 'rpc error' };
  const response = r.json?.result?.response;
  if (!response) return { ok: false, reason: 'unreachable', detail: 'no response' };
  const height = Number(response.height ?? '0');
  if (response.code) {
    const log = String(response.log ?? `abci code ${response.code}`);
    return { ok: false, reason: /not ?found|no such|key not found/i.test(log) ? 'not-found' : 'refused', detail: log };
  }
  return { ok: true, value: fromBase64(response.value ?? ''), height };
}

export function decoded<T>(outcome: Outcome<Uint8Array>, decode: (bytes: Uint8Array) => T): Outcome<T> {
  if (!outcome.ok) return outcome;
  try {
    return { ok: true, value: decode(outcome.value), height: outcome.height };
  } catch (err) {
    return { ok: false, reason: 'refused', detail: `could not decode: ${err instanceof Error ? err.message : String(err)}` };
  }
}

export interface ChainHead { chainId: string; height: number; time: number }

/** The latest block. Its time, not the device clock, dates vouchers. */
export async function chainHead(): Promise<Outcome<ChainHead>> {
  if (offline()) return { ok: false, reason: 'offline', detail: 'status' };
  const r = await rpc('/status');
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  const sync = r.json?.result?.sync_info;
  const node = r.json?.result?.node_info;
  if (!sync || !node) return { ok: false, reason: 'unreachable', detail: 'malformed status' };
  const height = Number(sync.latest_block_height ?? '0');
  const time = Math.floor(Date.parse(String(sync.latest_block_time)) / 1000);
  if (!Number.isFinite(time)) return { ok: false, reason: 'unreachable', detail: 'no block time' };
  return { ok: true, height, value: { chainId: String(node.network ?? ''), height, time } };
}

export interface TxEvent { type: string; attributes: { key: string; value: string }[] }
export interface TxHit { hash: string; height: number; code: number; events: TxEvent[] }

/**
 * Transactions whose events match a CometBFT query, newest first. Typed
 * events carry JSON-encoded values, so an address is matched as '"yml1…"',
 * quotes included.
 */
export async function txSearch(query: string, perPage = 50): Promise<Outcome<TxHit[]>> {
  if (offline()) return { ok: false, reason: 'offline', detail: 'tx_search' };
  const r = await rpc(`/tx_search?query=${encodeURIComponent(JSON.stringify(query))}`
    + `&prove=false&page=1&per_page=${perPage}&order_by=${encodeURIComponent('"desc"')}`);
  if (!r.ok) return { ok: false, reason: 'unreachable', detail: r.detail };
  if (r.json?.error) return { ok: false, reason: 'refused', detail: r.json.error.data ?? r.json.error.message ?? 'rpc error' };
  const txs: any[] = r.json?.result?.txs ?? [];
  const height = Number(txs[0]?.height ?? '0');
  return {
    ok: true, height,
    value: txs.map((tx) => ({
      hash: String(tx.hash), height: Number(tx.height), code: Number(tx.tx_result?.code ?? 0),
      events: (tx.tx_result?.events ?? []).map((e: any) => ({
        type: String(e.type),
        attributes: (e.attributes ?? []).map((a: any) => ({ key: String(a.key), value: String(a.value) })),
      })),
    })),
  };
}
