/** The catalogue pages' own logic, kept out of React so it can be tested. */

/**
 * The shop's origin, as traceability-api will accept it: https and a real
 * host. On the local devnet alone, the fixture shop on http://127.0.0.1 too
 * (traceability-api's --catalogue-dev-origin refuses any other chain).
 */
export function shopOrigin(input: string, chainId: string): string | null {
  const raw = input.trim();
  if (!raw) return null;
  let u: URL;
  try { u = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `https://${raw}`); } catch { return null; }
  if (u.username || u.password) return null;
  if (chainId === 'yamale-local-1' && u.protocol === 'http:' && u.hostname === '127.0.0.1' && u.port !== '') return u.origin;
  if (u.protocol !== 'https:' || !u.hostname.includes('.')) return null;
  return u.origin;
}

export type Filter = 'review' | 'confirmed' | 'gone';
const REVIEW_ORDER = ['changed', 'drafted', 'imported'];

export function filterDesigns<T extends { status: string }>(designs: T[], filter: Filter): T[] {
  if (filter === 'confirmed') return designs.filter((d) => d.status === 'confirmed');
  if (filter === 'gone') return designs.filter((d) => d.status === 'gone');
  return designs.filter((d) => REVIEW_ORDER.includes(d.status))
    .sort((a, b) => REVIEW_ORDER.indexOf(a.status) - REVIEW_ORDER.indexOf(b.status));
}

const IMPORT_ERRORS: Record<string, string> = {
  unreadable: 'traceability.importErr.unreadable', robots: 'traceability.importErr.robots',
  refused: 'traceability.importErr.refused', rateLimited: 'traceability.importErr.rateLimited',
  unreachable: 'traceability.importErr.unreachable', busy: 'traceability.importErr.busy',
};

export function importErrorKey(code: string): string | null { return IMPORT_ERRORS[code] ?? null; }

/** Confirm is allowed only when nothing is loading and no chosen photo failed: leaving one out is a deliberate untick. */
export function canConfirmDesign(c: {
  gone: boolean; rights: boolean; complete: boolean; busy: boolean;
  images: { state: 'loading' | 'ok' | 'failed'; chosen: boolean }[];
}): boolean {
  if (c.gone || !c.rights || !c.complete || c.busy) return false;
  if (c.images.some((i) => i.state === 'loading')) return false;
  if (c.images.some((i) => i.state === 'failed' && i.chosen)) return false;
  return c.images.some((i) => i.state === 'ok' && i.chosen);
}
