/**
 * Payment requests as a single scannable number.
 *
 * A request carries everything needed to pay it — who is being paid, how much,
 * in what, whether it repeats — packed into one string that can be read aloud
 * down a phone line, written on a receipt, or scanned. That is deliberate: a
 * market trader with a printed code and a customer with a feature phone both
 * need to complete the same payment, and a QR-only design serves only one of
 * them.
 *
 * The code is a URI under the hood. Scanners hand back a string either way, and
 * a URI means a camera app that does not know Yamale still does something
 * sensible with it instead of showing gibberish.
 *
 *   yamale:pay?to=K3M9-7QRT-B&amt=1500&cur=XOF&every=month&ref=INV-42
 *
 * The short numeric form is the same fields, base32-packed, for reading aloud.
 */
import { CURRENCIES, currencyOf } from './money.ts';
import { seal, open as unseal, toBase32, fromBase32 } from './sealed.ts';
import { verifyPayee, type Signed } from './signing.ts';

export type Recurrence = 'once' | 'week' | 'month' | 'year';

export interface PaymentRequest {
  /** The payee's chain user id or claimed name. Never an address. */
  to: string;
  /** Decimal, as a person would write it. Empty means "payer decides". */
  amount?: string;
  /** ISO-ish currency code, e.g. XOF. */
  currency: string;
  recurrence: Recurrence;
  /** Free-text reference — an invoice number, a stall number, a name. */
  reference?: string;
  /** Who is asking, for display before the payer commits. */
  payeeName?: string;
}

const SCHEME = 'yamale:pay';

export function encode(req: PaymentRequest): string {
  const p = new URLSearchParams();
  p.set('to', req.to);
  if (req.amount) p.set('amt', req.amount);
  p.set('cur', req.currency);
  if (req.recurrence !== 'once') p.set('every', req.recurrence);
  if (req.reference) p.set('ref', req.reference);
  if (req.payeeName) p.set('name', req.payeeName);
  return `${SCHEME}?${p.toString()}`;
}

export function decode(raw: string): PaymentRequest | null {
  const text = raw.trim();
  if (!text.toLowerCase().startsWith(SCHEME)) return null;
  const p = new URLSearchParams(text.slice(SCHEME.length + 1));
  const to = p.get('to');
  const currency = p.get('cur');
  if (!to || !currency) return null;
  if (!CURRENCIES.some((c) => c.code === currency)) return null;

  const every = (p.get('every') ?? 'once') as Recurrence;
  return {
    to,
    amount: p.get('amt') ?? undefined,
    currency,
    recurrence: (['once', 'week', 'month', 'year'] as const).includes(every) ? every : 'once',
    reference: p.get('ref') ?? undefined,
    payeeName: p.get('name') ?? undefined,
  };
}

// --- the spoken form -------------------------------------------------------

// Crockford base32, the same alphabet x/alias uses for user ids: no I, L, O or
// U, so nothing in a code can be misheard as something else or misread in
// handwriting. A payment code exists to be read aloud badly and still work.
const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

/**
 * A short numeric code for the same request, for people without a camera.
 *
 * The full request is packed to bytes, base32-encoded, and given a check
 * character. The check character is the point: a transposed digit produces an
 * invalid code rather than a valid code that pays the wrong person, which is
 * the failure that actually loses money.
 */
export function shortCode(req: PaymentRequest): string {
  const bytes = new TextEncoder().encode(encode(req));
  let bits = 0;
  let value = 0;
  let out = '';
  for (const b of bytes) {
    value = (value << 8) | b;
    bits += 8;
    while (bits >= 5) {
      out += ALPHABET[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += ALPHABET[(value << (5 - bits)) & 31];
  return group(out + checkCharacter(out));
}

/** Luhn mod N over the same alphabet, so a single wrong or swapped character
 *  fails rather than silently naming somebody else. */
function checkCharacter(payload: string): string {
  let factor = 2;
  let sum = 0;
  const n = ALPHABET.length;
  for (let i = payload.length - 1; i >= 0; i--) {
    const code = ALPHABET.indexOf(payload[i]);
    let addend = factor * code;
    factor = factor === 2 ? 1 : 2;
    addend = Math.floor(addend / n) + (addend % n);
    sum += addend;
  }
  return ALPHABET[(n - (sum % n)) % n];
}

export function verifyShortCode(code: string): boolean {
  const clean = normalise(code);
  if (clean.length < 2) return false;
  const payload = clean.slice(0, -1);
  return checkCharacter(payload) === clean.slice(-1);
}

/** Fold the characters a reader confuses, then group for legibility. */
export function normalise(code: string): string {
  return code.replace(/-/g, '').toUpperCase().replace(/[IL]/g, '1').replace(/O/g, '0');
}

function group(s: string): string {
  return (s.match(/.{1,4}/g) ?? []).join('-');
}



// --- the sealed forms ------------------------------------------------------
//
// Same request, wrapped so it is opaque to anything that is not this app. See
// sealed.ts for exactly how much that is worth: the key ships in every bundle,
// so this stops casual parsing and stops nothing determined.

/** What goes in the QR square. */
export async function encodeSealed(req: PaymentRequest): Promise<string> {
  return SCHEME + '?s=' + toBase32(await seal(encode(req)));
}

/** What gets read aloud, grouped and check-summed. */
export async function shortCodeSealed(req: PaymentRequest): Promise<string> {
  const body = toBase32(await seal(encode(req)));
  return group(body + checkCharacter(body));
}

/**
 * Read either form, sealed or plain.
 *
 * Plain codes are still accepted because a printed square outlives a release,
 * and refusing last month's poster to enforce this month's format is a cost
 * paid by the trader rather than by us.
 */
export async function decodeAny(raw: string): Promise<PaymentRequest | null> {
  const text = raw.trim();

  // Spoken form: letters and hyphens only, ending in a check character.
  if (!text.toLowerCase().startsWith(SCHEME)) {
    const clean = normalise(text);
    if (!verifyShortCode(clean)) return null;
    const bytes = fromBase32(clean.slice(0, -1));
    if (!bytes) return null;
    const plain = await unseal(bytes);
    return plain ? decode(plain) : null;
  }

  const params = new URLSearchParams(text.slice(SCHEME.length + 1));
  const sealedPart = params.get('s');
  if (sealedPart) {
    const bytes = fromBase32(normalise(sealedPart));
    if (!bytes) return null;
    const plain = await unseal(bytes);
    return plain ? decode(plain) : null;
  }
  return decode(text);
}



// --- the signed form, which is the one that travels ------------------------
//
// A payment code moves over channels nobody controls: a printed poster, a
// WhatsApp forward, a number read down a phone line. None of them can be made
// private, and none of them need to be — what they need is to be *tamper
// evident*, so that a code arriving by any route can be shown to have come from
// the account it claims to pay.
//
// The signature does that. Sealing on top of it hides the contents from casual
// readers, which is a smaller and separate benefit.

export interface SignedRequest {
  request: PaymentRequest;
  signed: Signed;
}

/** Produce a code that can safely cross an untrusted channel. */
export async function encodeSigned(
  req: PaymentRequest,
  sign: (payload: string) => Promise<Signed>,
): Promise<string> {
  const signed = await sign(encode(req));
  const packed = JSON.stringify(signed);
  return SCHEME + '?g=' + toBase32(await seal(packed));
}

/** The spoken form of a signed code. Longer, because a signature is 64 bytes
 *  and there is no honest way to make that short. */
export async function shortCodeSigned(
  req: PaymentRequest,
  sign: (payload: string) => Promise<Signed>,
): Promise<string> {
  const signed = await sign(encode(req));
  const body = toBase32(await seal(JSON.stringify(signed)));
  return group(body + checkCharacter(body));
}

/**
 * Read a code and decide whether to trust it.
 *
 * Returns the request only when the signature verifies *and* the signer is the
 * payee named inside it. Either check alone is worthless: an unverified request
 * is whatever the attacker wrote, and a verified signature from an unrelated
 * key is a genuine signature on a fraudulent request.
 */
export async function readSigned(
  raw: string,
  resolve: (id: string) => Promise<string | null>,
): Promise<{ request: PaymentRequest; trusted: boolean } | null> {
  const text = raw.trim();
  let body: string | null = null;

  if (text.toLowerCase().startsWith(SCHEME)) {
    const params = new URLSearchParams(text.slice(SCHEME.length + 1));
    body = params.get('g');
  } else {
    const clean = normalise(text);
    if (!verifyShortCode(clean)) return null;
    body = clean.slice(0, -1);
  }
  if (!body) return null;

  const bytes = fromBase32(normalise(body));
  if (!bytes) return null;
  const packed = await unseal(bytes);
  if (!packed) return null;

  let signed: Signed;
  try {
    signed = JSON.parse(packed);
  } catch {
    return null;
  }

  const request = decode(signed.payload);
  if (!request) return null;

  const trusted = await verifyPayee(signed, request.to, resolve);
  // The request is returned either way so the screen can say *why* it refused,
  // rather than showing a blank failure the payer cannot act on.
  return { request, trusted };
}

/** Human summary, used before anyone commits money. */
export function describe(req: PaymentRequest): string {
  const c = CURRENCIES.find((x) => x.code === req.currency);
  const amount = req.amount ? `${req.amount} ${c?.code ?? req.currency}` : '';
  return [amount, req.reference].filter(Boolean).join(' · ');
}

export { currencyOf };
