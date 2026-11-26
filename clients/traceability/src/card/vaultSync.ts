/**
 * One round with the shop's card vault: fetch it, keep in this browser every
 * card this member's boxes open, and seal the cards held here for the members
 * who lack them. Run when the cards page opens and right after cards are
 * issued, so a card made on one device is on the others by the time they look.
 */

import { putVault, readVault } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { keepCard, keptCard } from './cards.ts';
import { cardKeyOf, openCard, plan, reconcile, sealCard } from './vault.ts';

type WithKey = <T>(use: (key: Uint8Array) => T) => T;

export type VaultSync = { ok: true; received: number; shared: number } | { ok: false; detail: string };

export async function syncVault(withKey: WithKey, me: string, jewelerId: string, jewelIds: string[],
  fresh: ReadonlySet<string> = new Set()): Promise<VaultSync> {
  try {
    const read = await withKey((k) => readVault(k, jewelerId, CHAIN_ID));
    if (!read.ok) return { ok: false, detail: read.error || `HTTP ${read.status}` };
    const cards = reconcile(read.value, jewelIds, (id) => keptCard(id),
      (box, id) => withKey((k) => openCard(box, k, id, me)), fresh);
    let received = 0;
    for (const [id, secret] of cards) {
      const local = keptCard(id);
      if (!local || cardKeyOf(local) !== cardKeyOf(secret)) {
        keepCard(id, secret);
        received++;
      }
    }
    let shared = 0;
    for (const u of plan(read.value, cards, sealCard)) {
      const r = await withKey((k) => putVault(k, jewelerId, u.jewelId, u.body, CHAIN_ID));
      if (r.ok) shared += Object.keys(u.body.boxes).length;
    }
    return { ok: true, received, shared };
  } catch (e) {
    return { ok: false, detail: e instanceof Error ? e.message : String(e) };
  }
}
