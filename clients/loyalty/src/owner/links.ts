/**
 * Two small things the owner screens need: recognising an account code, and
 * keeping a claim link on the seller's phone while its offer is open.
 *
 * The link is kept so the seller can show the QR again at the counter, or
 * resend it. It holds the claim key, and the phone holding it is the seller's
 * own, which could cancel the offer anyway; it is removed when the offer goes.
 */

import { fromBech32 } from '@cosmjs/encoding';

export function isAccountCode(text: string): boolean {
  try {
    const { prefix, data } = fromBech32(text.trim());
    return prefix === 'yml' && data.length === 20;
  } catch {
    return false;
  }
}

const PREFIX = 'yamale.loyalty.link.';

export function rememberLink(jewelId: string, claimKey: string, link: string, store: Storage = localStorage): void {
  try { store.setItem(`${PREFIX}${jewelId}.${claimKey}`, link); } catch { /* storage refused: the link shows once */ }
}

export function storedLink(jewelId: string, claimKey: string, store: Storage = localStorage): string | null {
  try { return store.getItem(`${PREFIX}${jewelId}.${claimKey}`); } catch { return null; }
}

export function forgetLinks(jewelId: string, store: Storage = localStorage): void {
  try {
    const doomed: string[] = [];
    for (let i = 0; i < store.length; i++) {
      const k = store.key(i);
      if (k?.startsWith(`${PREFIX}${jewelId}.`)) doomed.push(k);
    }
    for (const k of doomed) store.removeItem(k);
  } catch { /* nothing to forget */ }
}
