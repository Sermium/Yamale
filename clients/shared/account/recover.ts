/**
 * Getting the key back after an operator-run recovery.
 *
 * The account service never hands a key to anyone. When a recovery has run its
 * course (two approvers, 72 hours' notice, then a 24-hour hold the owner can
 * still cancel), each service releases its holding to the account holder, who
 * proves it with their password and second factor. The two holdings and the
 * password open the key on this device, and nowhere else. See
 * tools/custodian/claim.go.
 *
 * What this cannot do: open a key whose password is forgotten. Nobody can.
 * For that the platform proposes reassigning the jewels, publicly, over 30 days.
 */

import { restore, ROLE_CUSTODIAN, ROLE_RECOVERY } from '../../wallet/src/account/protect.ts';
import { canonicalEmail, type Holding } from '../../wallet/src/account/deposit.ts';
import type { Services } from '../../wallet/src/account/enrol.ts';
import { addressOfPublicKey, compressedPublicKey } from '../../sdk/src/jewel.ts';

export interface RecoveryClaim {
  recoveryId: string;
  email: string;
  password: string;
  factor: 'email' | 'totp' | 'passkey';
  answer: string;
}

export type ClaimOutcome =
  | { ok: true; privateKey: Uint8Array; address: string }
  | { ok: false; reason: 'not-ready' | 'held' | 'refused' | 'unreachable' | 'mismatch' | 'unavailable'; detail: string };

function bytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// Go marshals []byte as base64, which is what both services send.
function holdingFromWire(w: any): Holding {
  return {
    role: String(w.role), salt: bytes(w.salt), nonce: bytes(w.nonce),
    time: Number(w.time), memory: Number(w.memory), threads: Number(w.threads),
    share: { x: Number(w.share.x), y: bytes(w.share.y) },
  };
}

async function claimOne(url: string, role: string, claim: RecoveryClaim, fetchImpl: typeof fetch):
  Promise<{ ok: true; address: string; holding: Holding } | Extract<ClaimOutcome, { ok: false }>> {
  let res: Response;
  try {
    res = await fetchImpl(`${url}/v1/recovery/claim`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: claim.recoveryId, email: canonicalEmail(claim.email), password: claim.password,
        factor: claim.factor, answer: claim.answer }),
    });
  } catch (err) {
    return { ok: false, reason: 'unreachable', detail: err instanceof Error ? err.message : String(err) };
  }
  const body = await res.json().catch(() => ({}));
  if (res.status === 409) {
    return { ok: false, reason: res.headers.get('Retry-After') ? 'held' : 'not-ready', detail: String(body.error ?? '') };
  }
  // 503: the service cannot take a claim at all (no second factor configured,
  // or down). Nothing the person typed was wrong.
  if (res.status === 503) return { ok: false, reason: 'unavailable', detail: String(body.error ?? '') };
  if (!res.ok) return { ok: false, reason: 'refused', detail: String(body.error ?? `http ${res.status}`) };
  if (body.role !== role) {
    return { ok: false, reason: 'mismatch', detail: 'a service answered as the wrong role' };
  }
  try {
    return { ok: true, address: String(body.address), holding: holdingFromWire(body.holding) };
  } catch {
    return { ok: false, reason: 'refused', detail: 'the service sent a holding this app cannot read' };
  }
}

export async function claimRecovered(claim: RecoveryClaim, services: Services,
  fetchImpl: typeof fetch = fetch): Promise<ClaimOutcome> {
  const custodian = await claimOne(services.custodianUrl, ROLE_CUSTODIAN, claim, fetchImpl);
  if (!custodian.ok) return custodian;
  const recovery = await claimOne(services.recoveryUrl, ROLE_RECOVERY, claim, fetchImpl);
  if (!recovery.ok) return recovery;
  if (custodian.address !== recovery.address) {
    return { ok: false, reason: 'mismatch', detail: 'the two services named different accounts' };
  }
  let privateKey: Uint8Array;
  try {
    privateKey = await restore([custodian.holding, recovery.holding], claim.password);
  } catch (err) {
    return { ok: false, reason: 'refused', detail: err instanceof Error ? err.message : String(err) };
  }
  const address = addressOfPublicKey(compressedPublicKey(privateKey));
  if (address !== custodian.address) return { ok: false, reason: 'mismatch', detail: 'the holdings open a different key' };
  return { ok: true, privateKey, address };
}
