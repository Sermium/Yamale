/**
 * The record a jeweler is drafting, and the one rule for whether it may be
 * registered: the reader's own. A draft is complete only if the record built
 * from it survives encodeRecord and parseRecord, so nothing can reach the
 * chain that every viewer would then see as unreadable.
 */

import { encodeRecord, parseRecord, type JewelRecord, type Stone } from '../verify/record.ts';

export type Draft = Pick<JewelRecord, 'title' | 'metal' | 'weightGrams' | 'stones' | 'origin'>;

export const EMPTY_DRAFT: Draft = {
  title: '', metal: '', weightGrams: '', stones: [],
  origin: { workshop: '', stoneSource: '', labCertificate: '' },
};

export const MAX_STONES = 50;

/** A stone count is a whole number from 1 to 9999, whatever was typed. */
export function clampCount(raw: number): number {
  const n = Math.trunc(Number(raw));
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.min(n, 9999);
}

const DECIMAL = /^[0-9]+(\.[0-9]+)?$/;

/** What must be filled, and within the limits the reader enforces, before a record is worth registering. */
export function draftComplete(d: Draft, version: 1 | 2 = 1): boolean {
  if (d.title.trim() === '' || d.metal.trim() === '' || !DECIMAL.test(d.weightGrams) || d.origin.workshop.trim() === '') return false;
  if (!d.stones.every((s) => s.kind.trim() !== '' && DECIMAL.test(s.carat))) return false;
  const base = { jewelerId: '1', inscription: 'CANDIDATE', ...d, photos: ['0'.repeat(64)], createdAt: '2000-01-01T00:00:00.000Z' };
  const candidate: JewelRecord = version === 1 ? { version: 1, ...base }
    : { version: 2, ...base, description: '', productUrl: '', sku: '', listPrice: null, pieceNotes: '' };
  return parseRecord(encodeRecord(candidate)) !== null;
}

export interface DesignFields { metal: string; weightGrams: string; stones: Stone[] }

/**
 * A catalogue design is complete when a piece of it could be registered: the
 * same rules as a draft, except that the weight may be empty because each CSV
 * row can carry its own. Mirrors fieldsProblem in
 * tools/traceabilityapi/designs.go; change both together.
 */
export function designComplete(title: string, f: DesignFields): boolean {
  if (f.weightGrams !== '' && !DECIMAL.test(f.weightGrams)) return false;
  return draftComplete({
    title, metal: f.metal, weightGrams: f.weightGrams === '' ? '1' : f.weightGrams, stones: f.stones,
    origin: { workshop: 'W', stoneSource: '', labCertificate: '' },
  });
}
