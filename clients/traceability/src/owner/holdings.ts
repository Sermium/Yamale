/**
 * "My jewels" as a list of designs: one row per design, how many pieces of it
 * are held, and their serial numbers. Kept out of React so it can be tested.
 */

/** One held jewel, as the list needs it. `title` is null when its record could not be read or checked. */
export interface Held {
  id: string; jewelerId: string; inscription: string;
  title: string | null; sku: string; photo?: string;
  stolen: boolean; onOffer: boolean;
  /** Whether an ownership card is issued for it, and whether a claim on that card waits for the shop. */
  hasCard?: boolean; claimed?: boolean;
}

export interface HeldGroup {
  key: string; title: string | null; photo?: string; items: Held[]; stolen: number; onOffer: number;
  withoutCard: number; claimed: number;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

/**
 * Pieces of one design share a row: the design code when the record carries one
 * (two designs may share a name), the title otherwise, and never across jewelers.
 * A jewel whose record is unreadable stands alone, after the named rows.
 */
export function groupHoldings(items: Held[]): HeldGroup[] {
  const groups = new Map<string, HeldGroup>();
  for (const it of items) {
    const key = it.title === null ? `?${it.id}` : `${it.jewelerId}\u0000${it.sku || `title:${it.title}`}`;
    let g = groups.get(key);
    if (!g) { g = { key, title: it.title, photo: it.photo, items: [], stolen: 0, onOffer: 0, withoutCard: 0, claimed: 0 }; groups.set(key, g); }
    g.items.push(it);
    if (!g.photo && it.photo) g.photo = it.photo;
    if (it.stolen) g.stolen++;
    if (it.onOffer) g.onOffer++;
    if (!it.hasCard) g.withoutCard++;
    if (it.claimed) g.claimed++;
  }
  const out = [...groups.values()];
  for (const g of out) g.items.sort((a, b) => collator.compare(a.inscription, b.inscription));
  return out.sort((a, b) => {
    if ((a.title === null) !== (b.title === null)) return a.title === null ? 1 : -1;
    if (a.title === null || b.title === null) return collator.compare(a.items[0]!.id, b.items[0]!.id);
    return collator.compare(a.title, b.title) || collator.compare(a.key, b.key);
  });
}

/** `fn` over every item, at most `limit` at a time, results in the items' order. */
export async function mapLimit<T, R>(items: T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length);
  let next = 0;
  async function worker() {
    while (next < items.length) {
      const i = next++;
      out[i] = await fn(items[i]!);
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
  return out;
}
