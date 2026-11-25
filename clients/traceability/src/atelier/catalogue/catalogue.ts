/** The catalogue pages' own logic, kept out of React so it can be tested. */

import { designComplete, type DesignFields } from './draft.ts';

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
  interrupted: 'traceability.importErr.interrupted',
};

export function importErrorKey(code: string): string | null { return IMPORT_ERRORS[code] ?? null; }

/**
 * Confirm is allowed only when nothing is loading and no chosen photo failed: leaving one out is a deliberate untick.
 * `settled`: the design is confirmed and nothing on the page has changed since, so there is nothing to confirm.
 */
export function canConfirmDesign(c: {
  gone: boolean; rights: boolean; complete: boolean; busy: boolean; settled?: boolean;
  images: { state: 'loading' | 'ok' | 'failed'; chosen: boolean }[];
}): boolean {
  if (c.gone || !c.rights || !c.complete || c.busy || c.settled) return false;
  if (c.images.some((i) => i.state === 'loading')) return false;
  if (c.images.some((i) => i.state === 'failed' && i.chosen)) return false;
  return c.images.some((i) => i.state === 'ok' && i.chosen);
}

/** What stops a design being confirmed as it stands: a title, a metal, and a name for every stone. Carats and count may be left not stated. */
export type Missing = 'title' | 'metal' | 'weight' | 'stones' | 'photos';
const DECIMAL = /^\d+(\.\d+)?$/;

export function missingForConfirm(d: { title: string; fields: DesignFields; imageUrls: string[] }): Missing[] {
  const out: Missing[] = [];
  if (d.title.trim() === '') out.push('title');
  if (d.fields.metal.trim() === '') out.push('metal');
  if (d.fields.weightGrams !== '' && !DECIMAL.test(d.fields.weightGrams)) out.push('weight');
  if (d.fields.stones.some((s) => s.kind.trim() === '' || (s.carat !== '' && !DECIMAL.test(s.carat)) || !Number.isInteger(s.count) || s.count < 0 || s.count > 9999)) out.push('stones');
  if (d.imageUrls.length === 0) out.push('photos');
  return out;
}

/**
 * The designs "Confirm all" takes: drafted or imported, and complete as they stand.
 * A changed design is left to a person: the website moved since it was confirmed.
 */
export function readyToConfirm<D extends { status: string; title: string; fields: DesignFields; imageUrls: string[] }>(designs: D[]): D[] {
  return designs.filter((d) => (d.status === 'drafted' || d.status === 'imported')
    && missingForConfirm(d).length === 0 && designComplete(d.title, d.fields));
}

export interface BulkDeps<D> {
  /** One of the website's photos, or null when it could not be read: it is left out, as an untick would. */
  image(d: D, n: number): Promise<Blob | null>;
  upload(photo: Blob): Promise<{ ok: true; hash: string } | { ok: false; raw: string }>;
  confirm(d: D, photos: string[]): Promise<{ ok: true } | { ok: false; raw: string }>;
  progress(done: number, of: number, current: D | null): void;
  stopped(): boolean;
}
export interface BulkResult { confirmed: string[]; failed: { id: string; raw: string }[]; notRun: string[] }

/** One design after another, each exactly as a person confirming it alone would; a failure is recorded and the next one tried. */
export async function confirmAll<D extends { id: string; imageUrls: string[] }>(designs: D[], deps: BulkDeps<D>): Promise<BulkResult> {
  const res: BulkResult = { confirmed: [], failed: [], notRun: [] };
  for (let i = 0; i < designs.length; i++) {
    const d = designs[i]!;
    if (deps.stopped()) { res.notRun.push(...designs.slice(i).map((x) => x.id)); break; }
    deps.progress(i, designs.length, d);
    const photos: string[] = [];
    let why = '';
    for (let n = 0; n < d.imageUrls.length && !why; n++) {
      const blob = await deps.image(d, n);
      if (!blob) continue;
      const up = await deps.upload(blob);
      if (up.ok) photos.push(up.hash); else why = up.raw;
    }
    if (!why && photos.length === 0) why = 'no photo could be read from the website';
    if (!why) {
      const c = await deps.confirm(d, photos);
      if (!c.ok) why = c.raw;
    }
    if (why) res.failed.push({ id: d.id, raw: why }); else res.confirmed.push(d.id);
  }
  deps.progress(res.confirmed.length + res.failed.length, designs.length, null);
  return res;
}
