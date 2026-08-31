/**
 * Reading this chain without a password.
 *
 * The node's REST surface is allowlisted per module path by the proxy, and
 * everything under `/api/rest/yamale/blockchain/tokenisation/` and
 * `/api/rest/yamale/blockchain/land/` is outside the allowlist: a browser
 * asking for a vehicle gets a 401, which the browser renders as a login box.
 * Nobody has that password, because it is not that kind of gate.
 *
 * The same queries answer unauthenticated over the node's ABCI interface, which
 * is proxied at `/api/rpc/` and is not gated. That is not a workaround — x/land
 * says so in its own service comment, at length, and means it: *"A citizen must
 * be able to check a title before paying anybody: without an account, without
 * an official's permission, and without anybody knowing they looked."* An
 * investor checking whether a vehicle's authorisation is live is doing the same
 * thing, and must be able to do it the same way.
 *
 * This file is the transport. The parts that can be got wrong without a network
 * — the hex the request travels as, and the difference between "the chain said
 * no such record" and "the chain did not answer" — are pure and tested.
 */

/* ------------------------------------------------------------ the outcome */

/**
 * What a query produced.
 *
 * Three failures rather than one, because a screen has to say different things
 * about them and a null return conflates all three into "nothing here". A
 * parcel with no fractionalisation authorisation is a finding an investor needs
 * to act on; a node that timed out is not a finding at all, and showing it as
 * one would tell somebody the registry never permitted a sale it may well have.
 */
export type Outcome<T> =
  | { ok: true; value: T; height: number }
  /** The chain answered, and the answer is that there is no such record. */
  | { ok: false; reason: 'not-found'; detail: string }
  /** The chain answered and refused the query: an unknown path, a pruned height. */
  | { ok: false; reason: 'refused'; detail: string }
  /** No answer: timeout, network, a proxy in the way, malformed JSON. */
  | { ok: false; reason: 'unreachable'; detail: string };

/** True when the failure is the chain's answer rather than the absence of one. */
export function isAnswer<T>(outcome: Outcome<T>): boolean {
  return outcome.ok || outcome.reason !== 'unreachable';
}

/* ------------------------------------------------------------- the wire */

/** Request bytes as the `data` parameter wants them: lower-case hex, 0x-prefixed. */
export function toHex(bytes: Uint8Array): string {
  let out = '0x';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}

/**
 * Response bytes out of the base64 the RPC wraps them in.
 *
 * Written by hand rather than through a helper because the empty string is a
 * legitimate response — a query whose answer is an empty message encodes to
 * zero bytes — and several base64 helpers treat that as an error.
 */
export function fromBase64(value: string): Uint8Array {
  if (value === '') return new Uint8Array(0);
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

/** The shape CometBFT's /abci_query replies in. */
export interface AbciReply {
  result?: {
    response?: {
      code?: number;
      log?: string;
      value?: string | null;
      height?: string;
    };
  };
  error?: { data?: string; message?: string };
}

/**
 * One reply, turned into an outcome.
 *
 * The `code` is the whole of the distinction this function exists to draw. A
 * non-zero ABCI code is the chain *answering*: 22 is codespace `sdk`'s
 * not-found, and a gRPC NotFound surfaces with "not found" in the log. Treating
 * either as an empty result would render a missing record and an unreachable
 * node identically, and those need opposite sentences.
 *
 * Pure, and separate from the fetch, so the mapping can be tested against
 * replies that are awkward to provoke from a live node.
 */
export function interpret(reply: AbciReply): Outcome<Uint8Array> {
  if (reply.error) {
    return {
      ok: false,
      reason: 'unreachable',
      detail: reply.error.message ?? reply.error.data ?? 'rpc error',
    };
  }

  const response = reply.result?.response;
  if (!response) return { ok: false, reason: 'unreachable', detail: 'no response' };

  const height = Number(response.height ?? '0');

  if (response.code) {
    const log = response.log ?? `abci code ${response.code}`;
    const missing = /not ?found|no such|unknown request: .*not found/i.test(log);
    return { ok: false, reason: missing ? 'not-found' : 'refused', detail: log };
  }

  return { ok: true, value: fromBase64(response.value ?? ''), height };
}

/* -------------------------------------------------------------- the call */

const RPC = '/api/rpc';
const TIMEOUT_MS = 9000;

/**
 * One ABCI query.
 *
 * The path is wrapped in literal double quotes before percent-encoding, because
 * CometBFT parses the `path` parameter as a JSON string and an unquoted path is
 * rejected as an unknown query path — which reads exactly like a module that is
 * not installed, and cost an hour to tell apart from one.
 */
export async function query(
  path: string,
  request: Uint8Array = new Uint8Array(0),
): Promise<Outcome<Uint8Array>> {
  const url = `${RPC}/abci_query?path=${encodeURIComponent(`"${path}"`)}&data=${toHex(request)}`;

  let reply: AbciReply;
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) {
      return { ok: false, reason: 'unreachable', detail: `http ${res.status}` };
    }
    reply = await res.json();
  } catch (err) {
    return {
      ok: false,
      reason: 'unreachable',
      detail: err instanceof Error ? err.message : String(err),
    };
  }

  return interpret(reply);
}

/** Decode a successful outcome, keeping the height and the failure shapes. */
export function decoded<T>(
  outcome: Outcome<Uint8Array>,
  decode: (bytes: Uint8Array) => T,
): Outcome<T> {
  if (!outcome.ok) return outcome;
  try {
    return { ok: true, value: decode(outcome.value), height: outcome.height };
  } catch (err) {
    // A decode failure is the chain and this client disagreeing about a message
    // shape, which is a real fault and must not be reported as an empty answer.
    return {
      ok: false,
      reason: 'refused',
      detail: `could not decode: ${err instanceof Error ? err.message : String(err)}`,
    };
  }
}

/* ---------------------------------------------------------- the chain head */

export type Head =
  | { known: false }
  | { known: true; chainId: string; height: number; at: Date; catchingUp: boolean };

/**
 * Where the chain is, and when it thinks it is.
 *
 * The block time matters here more than on most surfaces. Every deadline this
 * app renders — a challenge window closing, an authorisation expiring — is
 * decided by the keeper against `BlockTime()`, not against the reader's laptop
 * clock. A countdown driven by the wrong clock tells somebody they have time
 * they do not have.
 */
export async function head(): Promise<Head> {
  try {
    const res = await fetch(`${RPC}/status`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) throw new Error(`http ${res.status}`);
    const json = await res.json();
    const sync = json?.result?.sync_info;
    const node = json?.result?.node_info;
    if (!sync || !node) throw new Error('malformed status');

    return {
      known: true,
      chainId: String(node.network ?? ''),
      height: Number(sync.latest_block_height ?? '0'),
      at: new Date(String(sync.latest_block_time ?? '')),
      catchingUp: Boolean(sync.catching_up),
    };
  } catch {
    return { known: false };
  }
}
