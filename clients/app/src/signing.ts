/**
 * Signed payment requests.
 *
 * The threat a payment code actually faces is **substitution**, not
 * eavesdropping. Nobody gains much from learning that a stall wants 1 500 XOF
 * for invoice 42. What they gain is from printing their own square and taping
 * it over the trader's, so a day of takings arrives in the wrong account and
 * everybody involved believes the system worked.
 *
 * Encryption with a shared key does nothing against that — the attacker has the
 * app, therefore has the key, therefore can produce a perfectly valid sealed
 * code naming themselves. A signature does stop it, because the payer can check
 * that the code was made by the account it claims to pay.
 *
 * So a code carries three things: the request, a signature over it, and the
 * public key that made the signature. Verification is two questions, and both
 * must be yes:
 *
 *   1. Does the signature check out against that public key?
 *   2. Does that public key belong to the payee named inside the request?
 *
 * Question 2 is the one that matters and the one that is easy to forget. A
 * signature that verifies against a key the attacker generated is a valid
 * signature on a fraudulent request. The chain answers it: the user id resolves
 * to an account, and the account is derived from the public key.
 */
import { Secp256k1, Secp256k1Signature, Slip10, Slip10Curve, sha256, stringToPath } from '@cosmjs/crypto';
import { toBech32, fromBech32, toBase64, fromBase64 } from '@cosmjs/encoding';
import { rawSecp256k1PubkeyToRawAddress } from '@cosmjs/amino';

const PATH = "m/44'/118'/0'/0/0";

export interface Signed {
  /** The plain request URI that was signed. */
  payload: string;
  /** Signature over sha256(payload), 64 bytes, base64. */
  signature: string;
  /** Compressed public key, 33 bytes, base64. */
  pubkey: string;
}

/**
 * Sign a request with the account's own key.
 *
 * The mnemonic is reachable here because the wallet holds it; it never leaves
 * this module and is never rendered. Deriving on each call rather than caching
 * the private key keeps the window in which it exists as short as the call.
 */
export async function signRequest(mnemonic: string, payload: string): Promise<Signed> {
  const { privkey } = await deriveKey(mnemonic);
  const digest = sha256(new TextEncoder().encode(payload));
  const signature = await Secp256k1.createSignature(digest, privkey);
  const { pubkey } = await Secp256k1.makeKeypair(privkey);
  return {
    payload,
    signature: toBase64(signature.toFixedLength()),
    pubkey: toBase64(Secp256k1.compressPubkey(pubkey)),
  };
}

/**
 * Check the signature, and return the account it was made by.
 *
 * Returns null on anything that does not verify. The caller must still confirm
 * that this account is the payee — a signature alone proves somebody signed,
 * not that the right somebody signed.
 */
export async function verifySignature(signed: Signed, prefix = 'yml'): Promise<string | null> {
  try {
    const digest = sha256(new TextEncoder().encode(signed.payload));
    const pubkey = fromBase64(signed.pubkey);
    const ok = await Secp256k1.verifySignature(
      Secp256k1Signature.fromFixedLength(fromBase64(signed.signature)),
      digest,
      pubkey,
    );
    if (!ok) return null;
    return toBech32(prefix, rawSecp256k1PubkeyToRawAddress(pubkey));
  } catch {
    return null;
  }
}

/**
 * The whole check, in the order that matters.
 *
 * `resolve` is the chain lookup from a user id to an account. Passing it in
 * rather than importing it keeps this module free of any opinion about how the
 * network is reached, and makes the two-step nature of the check impossible to
 * skip by accident.
 */
export async function verifyPayee(
  signed: Signed,
  claimedPayeeId: string,
  resolve: (id: string) => Promise<string | null>,
): Promise<boolean> {
  const signerAccount = await verifySignature(signed);
  if (!signerAccount) return false;

  const payeeAccount = await resolve(claimedPayeeId);
  // An id that resolves to nothing fails closed. A code naming an unregistered
  // payee is either a typo or somebody hoping the check is skipped when the
  // lookup is inconvenient.
  if (!payeeAccount) return false;

  return normaliseAddress(signerAccount) === normaliseAddress(payeeAccount);
}

function normaliseAddress(addr: string): string {
  try {
    const { data } = fromBech32(addr);
    return toBase64(data);
  } catch {
    return addr;
  }
}

async function deriveKey(mnemonic: string): Promise<{ privkey: Uint8Array }> {
  const { Bip39, EnglishMnemonic } = await import('@cosmjs/crypto');
  const seed = await Bip39.mnemonicToSeed(new EnglishMnemonic(mnemonic));
  const { privkey } = Slip10.derivePath(Slip10Curve.Secp256k1, seed, stringToPath(PATH));
  return { privkey };
}
