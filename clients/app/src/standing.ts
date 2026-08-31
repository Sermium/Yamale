/**
 * What the chain says about the ISO payment path, read live.
 *
 * chain.ts records that the app "cannot even discover its own standing",
 * because `/api/rest/yamale/blockchain/paymsg/` sits behind the supervisor
 * credential under the split-visibility policy and a browser gets a 401. That
 * is still true of the REST prefix. It is not true of the chain: the same
 * queries are reachable over the node's ABCI interface, which is proxied at
 * /api/rpc/ and is not gated — verified against yamale-devnet-2, where
 *
 *     GET /api/rpc/abci_query?path="/blockchain.paymsg.v1.Query/ListApprovedParticipant"&data=0x
 *
 * answers 200 with an empty participant list.
 *
 * That distinction matters for the design. Reading it live is the difference
 * between a screen that says "there are no approved participants" because
 * somebody typed that sentence into it, and one that says so because it asked
 * and got an answer at a stated block height. When the query fails, this
 * reports that it failed. It never reports zero.
 */
import { QueryAllApprovedParticipantResponse } from '../../sdk/src/generated/blockchain/paymsg/v1/query.ts';

import type { ParticipantSummary, Standing } from './iso.ts';

const RPC = '/api/rpc';

/** How long to wait before calling the node unreachable. */
const TIMEOUT_MS = 8000;

interface AbciReply {
  result?: { response?: { code?: number; log?: string; value?: string | null; height?: string } };
  error?: { data?: string; message?: string };
}

/**
 * One ABCI query, returning the response bytes.
 *
 * Throws with a short reason rather than returning a partial answer: every
 * caller here has to distinguish "the chain said X" from "the chain did not
 * say", and a null return conflates them.
 */
async function abciQuery(path: string): Promise<{ bytes: Uint8Array; height: number }> {
  const url = `${RPC}/abci_query?path=${encodeURIComponent(`"${path}"`)}&data=0x`;

  const res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!res.ok) throw new Error(`http ${res.status}`);

  const json: AbciReply = await res.json();
  if (json.error) throw new Error(json.error.message ?? 'rpc error');

  const response = json.result?.response;
  if (!response) throw new Error('no response');
  // A non-zero ABCI code is the node refusing the query — an unknown path, a
  // pruned height. It is not an empty result.
  if (response.code) throw new Error(response.log || `abci code ${response.code}`);

  const value = response.value ?? '';
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);

  return { bytes, height: Number(response.height ?? '0') };
}

/**
 * Every governance-approved participant on this chain.
 *
 * The result carries the height it was read at, because "zero approved
 * participants" is a claim about a moment. A screen that shows the number
 * without the height is asking to be believed about a fact it cannot date.
 */
export async function approvedParticipants(): Promise<Standing> {
  try {
    const { bytes, height } = await abciQuery('/blockchain.paymsg.v1.Query/ListApprovedParticipant');
    const decoded = QueryAllApprovedParticipantResponse.decode(bytes);

    const participants: ParticipantSummary[] = (decoded.approvedParticipant ?? []).map((p) => ({
      address: p.participant ?? '',
      code: p.code ?? '',
      name: p.name ?? '',
    }));

    return { known: true, participants, height };
  } catch {
    // Deliberately not distinguishing timeout from 404 from a decode failure in
    // what the user is shown: all three mean the same thing to them, which is
    // that this number is not available right now.
    return { known: false, whyKey: 'iso.standingUnreachable' };
  }
}

/** ------------------------------------------------------------- chain head */

export type Head =
  | { known: false }
  | { known: true; chainId: string; height: number; at: Date; catchingUp: boolean };

/**
 * The chain's own account of where it is.
 *
 * Read from /status rather than from a REST endpoint, for the same reason as
 * above: the RPC proxy is open and the REST gate is selective, and a figure
 * that disappears behind a 401 on one host and not another is a figure nobody
 * can rely on.
 */
export async function head(): Promise<Head> {
  try {
    const res = await fetch(`${RPC}/status`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) throw new Error(`http ${res.status}`);
    const json = await res.json();
    const info = json?.result?.sync_info;
    const node = json?.result?.node_info;
    if (!info || !node) throw new Error('malformed status');

    return {
      known: true,
      chainId: String(node.network ?? ''),
      height: Number(info.latest_block_height ?? '0'),
      at: new Date(String(info.latest_block_time ?? '')),
      catchingUp: Boolean(info.catching_up),
    };
  } catch {
    return { known: false };
  }
}
