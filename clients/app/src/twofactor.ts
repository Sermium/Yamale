/**
 * Two-step sign-in.
 *
 * Three methods are offered and they are **not** equally good, so the interface
 * says which is which rather than presenting a menu of equals:
 *
 * **Authenticator app (TOTP).** Real, implemented here, and works with no
 * network at all. A shared secret plus the clock produces a six-digit code, so
 * an attacker needs the password *and* the phone. This is the one to
 * recommend, and the only one that costs nothing to run.
 *
 * **Email.** A code sent to the address on the account. Weaker than it looks:
 * whoever controls the mailbox controls the account, and a mailbox is usually
 * protected by a password somebody reused.
 *
 * **SMS.** Weakest of the three. SIM swapping is not exotic — it is the
 * standard attack on anyone worth attacking, and in markets where a SIM is
 * replaced at a kiosk it is close to trivial. It is offered because a great
 * many people have no smartphone and no email they check, and a weak second
 * factor genuinely beats none. It should never be the default.
 *
 * Email and SMS need something that can send, which a static app is not. Their
 * flows are built and clearly marked as unavailable until a service exists,
 * rather than faked with a code the page prints to itself — a demo that shows a
 * pretend code teaches the audience the wrong thing about what is real.
 */

export type Method = 'totp' | 'email' | 'sms';

const STORE = 'yamale.app.2fa';
const DIGITS = 6;
const PERIOD = 30;

export interface Enrolment {
  method: Method;
  /** Base32 shared secret. TOTP only. */
  secret?: string;
  /** Where a code would be sent. Email and SMS only. */
  destination?: string;
  confirmedAt: number;
}

export function enrolment(): Enrolment | null {
  try {
    const raw = localStorage.getItem(STORE);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

export function save(e: Enrolment): void {
  localStorage.setItem(STORE, JSON.stringify(e));
}

export function disable(): void {
  localStorage.removeItem(STORE);
}

// --- TOTP, RFC 6238 --------------------------------------------------------

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** A fresh secret. 20 random bytes, which is what RFC 4226 asks for. */
export function newSecret(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(20));
  let bits = 0, value = 0, out = '';
  for (const b of bytes) {
    value = (value << 8) | b;
    bits += 8;
    while (bits >= 5) {
      out += B32[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += B32[(value << (5 - bits)) & 31];
  return out;
}

/** The URI an authenticator app scans. */
export function otpauthUri(secret: string, account: string): string {
  const label = encodeURIComponent(`Yamale:${account}`);
  return `otpauth://totp/${label}?secret=${secret}&issuer=Yamale&digits=${DIGITS}&period=${PERIOD}`;
}

function fromBase32(text: string): Uint8Array {
  let bits = 0, value = 0;
  const out: number[] = [];
  for (const ch of text.toUpperCase().replace(/=+$/, '')) {
    const index = B32.indexOf(ch);
    if (index < 0) continue;
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return new Uint8Array(out);
}

/** A plain ArrayBuffer, which is what crypto.subtle accepts unambiguously. */
function toBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(copy).set(bytes);
  return copy;
}

async function code(secret: string, counter: number): Promise<string> {
  const key = await crypto.subtle.importKey(
    'raw', toBuffer(fromBase32(secret)), { name: 'HMAC', hash: 'SHA-1' }, false, ['sign'],
  );

  const buf = new ArrayBuffer(8);
  const view = new DataView(buf);
  // Counters are 64-bit and JavaScript numbers are not, so the halves are
  // written separately. Ignoring the high word works until 2038 and then
  // silently authenticates nobody.
  view.setUint32(0, Math.floor(counter / 2 ** 32));
  view.setUint32(4, counter >>> 0);

  const mac = new Uint8Array(await crypto.subtle.sign('HMAC', key, buf));
  const offset = mac[mac.length - 1] & 0x0f;
  const binary =
    ((mac[offset] & 0x7f) << 24) |
    ((mac[offset + 1] & 0xff) << 16) |
    ((mac[offset + 2] & 0xff) << 8) |
    (mac[offset + 3] & 0xff);
  return (binary % 10 ** DIGITS).toString().padStart(DIGITS, '0');
}

/**
 * Check a code against the current window and one step either side.
 *
 * The tolerance is not laziness: phone clocks drift, and a code typed in the
 * last second of a window arrives in the next one. Rejecting those produces a
 * system people believe is broken, and a person who believes 2FA is broken
 * turns it off.
 */
export async function verify(secret: string, entered: string): Promise<boolean> {
  const clean = entered.replace(/\D/g, '');
  if (clean.length !== DIGITS) return false;
  const counter = Math.floor(Date.now() / 1000 / PERIOD);
  for (const step of [-1, 0, 1]) {
    if (await code(secret, counter + step) === clean) return true;
  }
  return false;
}

/** Seconds left in the current window, for the countdown ring. */
export function secondsRemaining(): number {
  return PERIOD - Math.floor(Date.now() / 1000) % PERIOD;
}
