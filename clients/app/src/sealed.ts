/**
 * Sealing payment codes with a key every installation shares.
 *
 * What this achieves: a code is opaque to a camera app, a passer-by, or a rival
 * wallet that would otherwise parse the plain URI. Amount, reference and payee
 * do not sit in the open on a printed square taped to a market stall.
 *
 * What it does not achieve, stated here so nobody later mistakes it for what it
 * is not: **this is obfuscation, not confidentiality.** The key is in every
 * bundle. Anyone who wants it opens developer tools and reads it in a minute.
 * It raises the cost of casual snooping from zero to slightly above zero, and
 * against anyone determined it raises nothing at all.
 *
 * If a code must genuinely be unreadable except by its intended payer, the key
 * has to belong to that payer — seal to their public key, so only they can
 * open it. That is a different design and a bigger one; this file is not it,
 * and pretending otherwise is how a demo becomes a liability.
 *
 * AES-GCM with a random nonce per code, so two identical requests do not
 * produce the same square. A payee whose codes were byte-identical would be
 * trivially trackable across every stall they ever printed one at.
 */

// The shared secret. Not secret in any meaningful sense — see above.
const SHARED = 'yamale-payment-code-v1-shared-obfuscation-key';

const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
const MAGIC = 0x59; // 'Y', so a sealed payload is recognisable as ours

let cached: CryptoKey | null = null;

async function key(): Promise<CryptoKey> {
  if (cached) return cached;
  const material = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(SHARED));
  cached = await crypto.subtle.importKey('raw', material, 'AES-GCM', false, ['encrypt', 'decrypt']);
  return cached;
}

/** Seal a plaintext request into bytes: magic ‖ nonce ‖ ciphertext. */
export async function seal(plaintext: string): Promise<Uint8Array> {
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ciphertext = new Uint8Array(
    await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, await key(),
      new TextEncoder().encode(plaintext)),
  );
  const out = new Uint8Array(1 + nonce.length + ciphertext.length);
  out[0] = MAGIC;
  out.set(nonce, 1);
  out.set(ciphertext, 1 + nonce.length);
  return out;
}

/**
 * Open a sealed payload.
 *
 * Returns null rather than throwing on anything malformed. A code that fails to
 * open is an ordinary outcome — a smudged print, a mistyped character, a QR
 * from some other system entirely — and the screen should say "that code is not
 * one of ours", not crash.
 *
 * GCM authenticates, so a tampered code fails here rather than opening as a
 * payment to somebody else. That authentication is the one genuine security
 * property in this file, and it holds only against people who do not have the
 * key — which is to say, not against anyone who has the app.
 */
export async function open(bytes: Uint8Array): Promise<string | null> {
  if (bytes.length < 1 + 12 + 16 || bytes[0] !== MAGIC) return null;
  try {
    const nonce = bytes.slice(1, 13);
    const ciphertext = bytes.slice(13);
    const plain = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, await key(), ciphertext);
    return new TextDecoder().decode(plain);
  } catch {
    return null;
  }
}

// --- base32, for the spoken form ------------------------------------------

export function toBase32(bytes: Uint8Array): string {
  let bits = 0, value = 0, out = '';
  for (const b of bytes) {
    value = (value << 8) | b;
    bits += 8;
    while (bits >= 5) {
      out += ALPHABET[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += ALPHABET[(value << (5 - bits)) & 31];
  return out;
}

export function fromBase32(text: string): Uint8Array | null {
  let bits = 0, value = 0;
  const out: number[] = [];
  for (const ch of text) {
    const index = ALPHABET.indexOf(ch);
    if (index < 0) return null;
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return new Uint8Array(out);
}
