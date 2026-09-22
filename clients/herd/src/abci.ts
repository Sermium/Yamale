/**
 * Reading this chain from a pen, without a password and often without a signal.
 *
 * Same transport as the tokenisation app's `clients/rwa/src/abci.ts`, and for
 * the same reason: the node's REST surface is allowlisted per module path by
 * the proxy, `/api/rest/yamale/blockchain/herd/` is outside that allowlist, and
 * a browser asking for an animal over REST gets a 401 that it renders as a
 * login box nobody has the password for. Verified against devnet-2 while this
 * was written — the REST path answers `401 Authorization Required` from nginx,
 * the same query over `/api/rpc/abci_query` answers in full.
 *
 * `x/herd`'s own service comment says the read side is public on purpose: "a
 * stranger with a phone can verify what they are being sold". ABCI is how that
 * sentence is true.
 *
 * # The three failures, and the fourth this app adds
 *
 * Everything below returns an `Outcome`, never a value-or-null. On a registry
 * screen the difference between "the chain answered that there is no such
 * animal" and "the chain did not answer" is the difference between a finding
 * and a network, and an interface that renders both as an empty page is exactly
 * how the explorer once shipped a perfectly correct page for a broken query.
 *
 * This surface adds `offline` to that vocabulary. A field device is expected to
 * have no signal — that is not a fault, it is Tuesday — and a screen that
 * reported it with the same red panel as a broken node would train an operator
 * to ignore the red panel. See `Outcome` below.
 */

/* ------------------------------------------------------------ the outcome */

/** What a query produced. */
export type Outcome<T> =
  | { ok: true; value: T; height: number }
  /** The chain answered, and the answer is that there is no such record. */
  | { ok: false; reason: 'not-found'; detail: string }
  /** The chain answered and refused: an unknown path, a pruned height. */
  | { ok: false; reason: 'refused'; detail: string }
  /** No answer: timeout, a proxy in the way, malformed JSON. A fault. */
  | { ok: false; reason: 'unreachable'; detail: string }
  /**
   * The device knows it has no connection, so nothing was asked.
   *
   * Separate from `unreachable` because it is not a fault and must not be
   * dressed as one. An operator in a pastoral zone sees this most of the day,
   * and the sentence that belongs with it — "working from the copy on this
   * device" — is a different sentence from "the registry is down".
   */
  | { ok: false; reason: 'offline'; detail: string };

/** True when the failure is the chain's answer rather than the absence of one. */
export function isAnswer<T>(outcome: Outcome<T>): boolean {
  return outcome.ok || (outcome.reason !== 'unreachable' && outcome.reason !== 'offline');
}

/** True when the read failed because nothing could be asked. */
export function isSilence<T>(outcome: Outcome<T>): boolean {
  return !outcome.ok && (outcome.reason === 'unreachable' || outcome.reason === 'offline');
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
 * Written by hand because the empty string is a legitimate response — a query
 * whose answer is an empty message encodes to zero bytes — and several base64
 * helpers treat that as an error.
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
 * non-zero ABCI code is the chain *answering* — `x/herd`'s own not-found comes
 * back as `rpc error: code = NotFound desc = no herd 3: key not found`, which
 * was read off devnet-2 and is what the regular expression below is matched
 * against.
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
    const missing = /not ?found|no such|key not found/i.test(log);
    return { ok: false, reason: missing ? 'not-found' : 'refused', detail: log };
  }

  return { ok: true, value: fromBase64(response.value ?? ''), height };
}

/* -------------------------------------------------------------- the call */

const RPC = '/api/rpc';

/**
 * Nine seconds on a desk, and the same nine here.
 *
 * Longer was considered and rejected. A device on a 2G edge either answers in
 * a few seconds or does not answer, and an operator standing in front of a
 * moving animal will have moved on long before a thirty-second timeout expires
 * — so a long timeout does not buy an answer, it buys a screen that appears
 * frozen at the exact moment the local replica could have answered instantly.
 */
const TIMEOUT_MS = 9000;

/**
 * Whether this device believes it has a connection.
 *
 * `navigator.onLine` is famously optimistic: it reports the interface, not the
 * internet, so it says true on a Wi-Fi network with no route out. That is
 * exactly why it is used only in the negative here — a false is trustworthy and
 * saves nine seconds of waiting, a true is merely a reason to try.
 */
export function online(): boolean {
  if (typeof navigator === 'undefined') return true;
  return navigator.onLine !== false;
}

/**
 * One ABCI query.
 *
 * The path is wrapped in literal double quotes before percent-encoding, because
 * CometBFT parses the `path` parameter as a JSON string and an unquoted path is
 * rejected as an unknown query path — which reads exactly like a module that is
 * not installed.
 */
export async function query(
  path: string,
  request: Uint8Array = new Uint8Array(0),
): Promise<Outcome<Uint8Array>> {
  if (!online()) {
    return { ok: false, reason: 'offline', detail: path };
  }

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
    // shape. A real fault, and never an empty answer.
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
 * Read here for two reasons. The obvious one is dating a claim: every screen
 * ends with the block it was read at. The less obvious one is that `x/herd`
 * writes `Animal.registered_at` and `HealthEvent.recorded_at` as BLOCK HEIGHTS
 * rather than as times — see `herd.ts` — so the head is the only thing that
 * makes those numbers mean anything at all.
 */
export async function head(): Promise<Head> {
  if (!online()) return { known: false };
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
