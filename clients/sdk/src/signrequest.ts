/**
 * Reading a signing request — the bytes, not the requester's description of
 * them.
 *
 * This exists because the wallet's approval screen — the highest-stakes screen
 * in the product — decoded a transaction only as far as each message's proto
 * type URL. It told somebody they were about to sign
 * `/cosmos.group.v1.MsgSubmitProposal` and then, honestly but uselessly, that
 * "amounts and recipients are inside these messages and are not yet decoded
 * here". A wallet that cannot say what will change is a wallet whose approval
 * button means nothing: the only remaining check is trusting the page that
 * asked, which is precisely the trust a wallet exists to remove.
 *
 * The decode is real, and it is deliberately not a hand-rolled protobuf reader.
 * `registry.ts` already carries generated encoders for every message this chain
 * adds, produced from the same .proto files the chain is built from, and
 * `decode.ts` already turns a message into a sentence. What was missing was the
 * three lines between them: bytes → message object → the snake_case JSON shape
 * the decoders were written against.
 *
 * Two rules govern everything below.
 *
 * **A message the registry does not know is reported as unknown, never
 * guessed.** A plausible-looking summary of bytes nobody decoded is worse than
 * a blank one, because it converts "I could not read this" into "I read this
 * and it is fine".
 *
 * **Nested messages are decoded too.** A group or governance proposal carries
 * its payload as `Any`, so a decoder that stopped at the outer message would
 * say "requested approval for an action on a shared account" — which is the
 * wrapper, not the act. The action inside is the thing being approved.
 */

import { AuthInfo, TxBody, TxRaw } from 'cosmjs-types/cosmos/tx/v1beta1/tx.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { chainRegistry } from './registry.ts';
import { decodeMessage, shortTypeUrl, type DecodeContext, type DecodedMessage } from './decode.ts';
import { formatAmount, type Coin } from './denom.ts';
import { truncateAddress } from './format.ts';

/** A message inside a signing request, decoded as far as it can honestly be. */
export interface RequestMessage {
  typeUrl: string;
  /** Present when the registry could decode it. */
  decoded?: DecodedMessage;
  /**
   * Present instead of `decoded` when it could not. Carries why, so the screen
   * can distinguish "this wallet does not know this message" from "these bytes
   * are corrupt" — the first is a gap, the second is an attack.
   */
  problem?: 'unregistered' | 'malformed';
  /**
   * Messages this one carries inside it: a proposal's payload, an authz exec's
   * inner messages. Empty for the ordinary case.
   */
  contains: RequestMessage[];
}

/** What a transaction will pay to be included, and who pays it. */
export interface RequestFee {
  amount: Coin[];
  gasLimit: string;
  /** Set when another account was named to pay the fee. */
  payer?: string;
  /** Set when a fee allowance is being drawn on. */
  granter?: string;
}

export interface SigningRequestSummary {
  messages: RequestMessage[];
  memo: string;
  fee?: RequestFee;
  /**
   * Set when the transaction body itself could not be read. Nothing else in
   * this object is meaningful when it is, and the only correct interface
   * response is to refuse.
   */
  undecodable?: string;
  /** True when any message, at any depth, could not be decoded. */
  incomplete: boolean;
}

/**
 * Decodes the two byte strings a `signDirect` request carries.
 *
 * `authInfoBytes` is optional only because a caller may not have it; when it is
 * present the fee is read from it rather than from anything the requester said,
 * for the same reason as everything else here.
 */
export function summariseSigningRequest(
  bodyBytes: Uint8Array,
  authInfoBytes?: Uint8Array,
  ctx: DecodeContext = {},
): SigningRequestSummary {
  let body: TxBody;
  try {
    body = TxBody.decode(bodyBytes);
  } catch (err) {
    return {
      messages: [],
      memo: '',
      incomplete: true,
      undecodable: err instanceof Error ? err.message : 'the transaction body is not valid protobuf',
    };
  }

  const messages = body.messages.map((m) => decodeAny(m.typeUrl, m.value, ctx, 0));

  let fee: RequestFee | undefined;
  if (authInfoBytes) {
    try {
      const authInfo = AuthInfo.decode(authInfoBytes);
      if (authInfo.fee) {
        fee = {
          amount: (authInfo.fee.amount ?? []).map((c) => ({ denom: c.denom, amount: c.amount })),
          gasLimit: String(authInfo.fee.gasLimit ?? 0),
          payer: authInfo.fee.payer || undefined,
          granter: authInfo.fee.granter || undefined,
        };
      }
    } catch {
      // A body that decoded and an auth-info that did not is worth reporting as
      // an incomplete read rather than as a hard failure: the messages above
      // are real and readable, and the missing piece is the fee.
      fee = undefined;
    }
  }

  return {
    messages,
    memo: body.memo ?? '',
    fee,
    incomplete: messages.some(anyIncomplete),
  };
}

function anyIncomplete(m: RequestMessage): boolean {
  return m.problem !== undefined || m.contains.some(anyIncomplete);
}

/**
 * How deep to follow nested messages.
 *
 * A proposal containing a proposal containing a proposal is not a thing anybody
 * signs on purpose, and following an unbounded chain of `Any` out of bytes a
 * caller controls is how a decoder becomes a denial of service on the one
 * screen that must never hang.
 */
const MAX_DEPTH = 4;

function decodeAny(
  typeUrl: string,
  value: Uint8Array,
  ctx: DecodeContext,
  depth: number,
): RequestMessage {
  const registry = sharedRegistry();

  if (!registry.lookupType(typeUrl)) {
    return { typeUrl, problem: 'unregistered', contains: [] };
  }

  let json: Record<string, unknown>;
  try {
    json = toJsonShape(registry, typeUrl, value);
  } catch {
    return { typeUrl, problem: 'malformed', contains: [] };
  }

  const contains: RequestMessage[] = [];
  if (depth < MAX_DEPTH) {
    for (const inner of collectAnys(json)) {
      contains.push(decodeAny(inner.typeUrl, inner.value, ctx, depth + 1));
    }
  }

  return {
    typeUrl,
    decoded: decodeMessage({ ...json, '@type': typeUrl }, ctx),
    contains,
  };
}

/**
 * A decoded message in the shape `decode.ts` expects.
 *
 * The generated types' own `toJSON` does the parts that matter and are easy to
 * get wrong: enum numbers become their names (`1` → `LOCK_TYPE_VESTING`), bytes
 * become base64, and 64-bit fields become strings rather than losing their
 * bottom bits in a double. What it does not do is rename fields — ts-proto
 * emits `endToEndId` where the REST gateway emits `end_to_end_id`, and the
 * decoders were written against the gateway — so the keys are converted here.
 *
 * Doing it the other way round, writing a second set of camelCase decoders,
 * would mean the explorer and the wallet describing the same payment through
 * two implementations. They would agree until one of them was edited.
 */
function toJsonShape(
  registry: ReturnType<typeof chainRegistry>,
  typeUrl: string,
  value: Uint8Array,
): Record<string, unknown> {
  const type = registry.lookupType(typeUrl) as
    | { decode: (b: Uint8Array) => unknown; toJSON?: (m: unknown) => unknown }
    | undefined;
  if (!type) throw new Error(`no encoder for ${typeUrl}`);

  const message = type.decode(value);
  const json = typeof type.toJSON === 'function' ? type.toJSON(message) : message;
  return snakeKeys(json) as Record<string, unknown>;
}

/** camelCase → snake_case, recursively, leaving values alone. */
function snakeKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(snakeKeys);
  if (value === null || typeof value !== 'object') return value;
  if (value instanceof Uint8Array) return value;

  const out: Record<string, unknown> = {};
  for (const [key, v] of Object.entries(value as Record<string, unknown>)) {
    // Both spellings. `typeUrl` is read by the Any detection below and by the
    // decoders' own `@type` lookup, so renaming it away would break both; the
    // snake form is kept beside it because that is what the gateway emits and
    // what a raw-JSON disclosure should show.
    const snake = key.replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase();
    out[snake] = snakeKeys(v);
    if (snake !== key && (key === 'typeUrl' || key === 'msgTypeUrl')) out[key] = out[snake];
  }
  return out;
}

/**
 * Every `Any` reachable in a decoded message, at any nesting.
 *
 * Matched on shape — a `type_url` beside a base64 `value` — rather than on the
 * field name, because the field is called `messages` in a gov proposal, `msgs`
 * in an authz exec, `allowance` in a fee grant and `authorization` in a grant.
 */
function collectAnys(node: unknown, found: { typeUrl: string; value: Uint8Array }[] = []) {
  if (Array.isArray(node)) {
    for (const item of node) collectAnys(item, found);
    return found;
  }
  if (node === null || typeof node !== 'object') return found;

  const record = node as Record<string, unknown>;
  const url = record.type_url ?? record.typeUrl;
  const raw = record.value;
  if (typeof url === 'string' && url.startsWith('/') && typeof raw === 'string') {
    const bytes = fromBase64(raw);
    if (bytes) {
      found.push({ typeUrl: url, value: bytes });
      return found;
    }
  }

  for (const item of Object.values(record)) collectAnys(item, found);
  return found;
}

function fromBase64(value: string): Uint8Array | null {
  try {
    if (typeof atob === 'function') {
      const binary = atob(value);
      const out = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
      return out;
    }
    // Node, for the tests. Buffer is not assumed to exist in a browser build.
    const buffer = (globalThis as { Buffer?: { from: (s: string, e: string) => Uint8Array } }).Buffer;
    return buffer ? new Uint8Array(buffer.from(value, 'base64')) : null;
  } catch {
    return null;
  }
}

/**
 * One registry for the process.
 *
 * `chainRegistry()` registers around 90 types on every call, and the approval
 * screen decodes on every keystroke-free render. Sharing it is not an
 * optimisation for its own sake: building it per render is what makes a
 * signing screen feel slow at the moment somebody is trying to read it.
 */
let shared: ReturnType<typeof chainRegistry> | null = null;
function sharedRegistry() {
  if (!shared) shared = chainRegistry();
  return shared;
}

/**
 * The identifier the chain will give a transaction, computed before it is sent.
 *
 * A wallet that only signs never learns what happened next: it hands back a
 * signature, the calling application broadcasts, and the person who approved is
 * left looking at whatever that application chooses to tell them. Which is the
 * one party a wallet exists not to trust.
 *
 * It does not have to be that way. A transaction's hash is the SHA-256 of its
 * serialised `TxRaw`, and every part of that is known at the moment of signing —
 * the body and auth-info bytes are the ones that were signed, and the signature
 * is the one just produced. So the wallet can watch for its own transaction in
 * a block and report pending → confirmed with the height, from the chain rather
 * than from the requester.
 *
 * This holds only while the wallet returns the bytes it signed unaltered, which
 * this one does. A caller that broadcast something else would produce a
 * different hash, and this one would simply never appear — which is itself the
 * correct thing to show.
 */
export function transactionHash(
  bodyBytes: Uint8Array,
  authInfoBytes: Uint8Array,
  signature: Uint8Array,
): string {
  const raw = TxRaw.encode(TxRaw.fromPartial({ bodyBytes, authInfoBytes, signatures: [signature] })).finish();
  return [...sha256(raw)]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
    .toUpperCase();
}

/**
 * The one line a person should read before approving.
 *
 * A transaction with one message reads as that message's own sentence. Where a
 * message carries a nested action, the nested one is what is described — the
 * wrapper is plumbing, and "requested approval for an action on a shared
 * account" is exactly the sentence that made the old screen useless.
 */
export function headline(summary: SigningRequestSummary): string {
  if (summary.undecodable) return 'These bytes could not be read';
  if (summary.messages.length === 0) return 'A transaction that does nothing';

  if (summary.messages.length === 1) {
    return describe(summary.messages[0]!);
  }
  return `${summary.messages.length} actions in one transaction`;
}

/** One message as a sentence, preferring what it contains over its wrapper. */
export function describe(message: RequestMessage): string {
  if (message.problem === 'unregistered') {
    return `An action this wallet cannot read: ${shortTypeUrl(message.typeUrl)}`;
  }
  if (message.problem === 'malformed') {
    return `A message that is not valid ${shortTypeUrl(message.typeUrl)}`;
  }

  const summary = message.decoded?.summary ?? shortTypeUrl(message.typeUrl);
  if (message.contains.length === 1) {
    return `${summary}: ${describe(message.contains[0]!)}`;
  }
  return summary;
}

/**
 * The fee, in words, or nothing at all when it is zero.
 *
 * A zero fee is the normal case on this chain and saying "fee: 0 YML" on every
 * approval trains people to skip the line where a real fee would appear. A fee
 * paid by somebody else is always stated, because who pays is the part that is
 * not obvious.
 */
export function describeFee(fee: RequestFee | undefined, ctx: DecodeContext = {}): string | null {
  if (!fee) return null;

  const paying = fee.amount.filter((c) => c.amount !== '0' && c.amount !== '');
  // Truncated like every other identifier. A full bech32 address in the middle
  // of a sentence is 43 characters nobody reads and a line that wraps three
  // times on a phone, which pushes the amount above it off screen.
  const name = (address: string) => ctx.names?.[address] ?? truncateAddress(address);
  const who = fee.granter
    ? ` — paid by ${name(fee.granter)}'s allowance`
    : fee.payer
      ? ` — paid by ${name(fee.payer)}`
      : '';

  if (paying.length === 0) return who ? `No network fee${who}` : null;

  const parts = paying.map((c) => formatAmount(c.amount, c.denom, { registry: ctx.registry }));
  return `Network fee ${parts.join(' + ')}${who}`;
}
