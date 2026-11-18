/**
 * The record file: what a jeweler registered about a jewel, stored by
 * traceability-api under its SHA-256, which the chain holds as record_hash.
 *
 * It never names an owner. Owners change on the chain; the record describes
 * the jewel.
 */

import { useEffect, useState } from 'react';

import type { Jewel } from '../../../sdk/src/generated/blockchain/jewel/v1/jewel.ts';
import { fetchVerified } from '../api.ts';

export interface Stone { kind: string; carat: string; count: number }

export interface JewelRecord {
  version: 1;
  jewelerId: string;
  inscription: string;
  title: string;
  metal: string;
  weightGrams: string;
  stones: Stone[];
  origin: { workshop: string; stoneSource: string; labCertificate: string };
  photos: string[];
  createdAt: string;
}

const HASH = /^[0-9a-f]{64}$/;
const ID = /^[1-9][0-9]*$/;
const str = (v: unknown, max: number): v is string => typeof v === 'string' && v.length <= max;

/** Keys in one fixed order, so one record always has one fingerprint. */
export function encodeRecord(r: JewelRecord): Uint8Array {
  return new TextEncoder().encode(JSON.stringify({
    version: 1,
    jewelerId: r.jewelerId,
    inscription: r.inscription,
    title: r.title,
    metal: r.metal,
    weightGrams: r.weightGrams,
    stones: r.stones.map((s) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: r.origin.workshop, stoneSource: r.origin.stoneSource, labCertificate: r.origin.labCertificate },
    photos: [...r.photos],
    createdAt: r.createdAt,
  }));
}

export function parseRecord(bytes: Uint8Array): JewelRecord | null {
  let v: any;
  try { v = JSON.parse(new TextDecoder().decode(bytes)); } catch { return null; }
  if (!v || v.version !== 1 || !str(v.jewelerId, 20) || !ID.test(v.jewelerId)) return null;
  if (!str(v.inscription, 40) || !str(v.title, 120) || !str(v.metal, 80) || !str(v.weightGrams, 12) || !str(v.createdAt, 40)) return null;
  if (!Array.isArray(v.stones) || v.stones.length > 50) return null;
  for (const s of v.stones) {
    if (!s || !str(s.kind, 60) || !str(s.carat, 12) || !Number.isInteger(s.count) || s.count < 1 || s.count > 9999) return null;
  }
  const o = v.origin;
  if (!o || !str(o.workshop, 120) || !str(o.stoneSource, 120) || !str(o.labCertificate, 80)) return null;
  if (!Array.isArray(v.photos) || v.photos.length < 1 || v.photos.length > 12) return null;
  if (!v.photos.every((p: unknown) => typeof p === 'string' && HASH.test(p))) return null;
  return {
    version: 1, jewelerId: v.jewelerId, inscription: v.inscription, title: v.title, metal: v.metal,
    weightGrams: v.weightGrams,
    stones: v.stones.map((s: Stone) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: o.workshop, stoneSource: o.stoneSource, labCertificate: o.labCertificate },
    photos: [...v.photos], createdAt: v.createdAt,
  };
}

/** A correction replaces the record shown first; the original stays readable. */
export function currentRecordHash(jewel: Pick<Jewel, 'recordHash' | 'amendments'>): string {
  const last = jewel.amendments[jewel.amendments.length - 1];
  return last?.recordHash || jewel.recordHash;
}

export type RecordState =
  | { state: 'loading' }
  | { state: 'ok'; record: JewelRecord }
  | { state: 'missing' | 'mismatch' | 'unreachable' | 'unreadable' };

/**
 * A state belongs to the hash it was fetched for. Whenever the hash asked about
 * now differs (including none), the answer is "loading", never the previous
 * jewel's record: the render before the effect runs must not show it.
 */
export function stateForHash(held: { hash: string | null; state: RecordState }, hash: string | null): RecordState {
  return hash !== null && held.hash === hash ? held.state : { state: 'loading' };
}

/** What the chain holds about a jewel that a record must agree with. */
export type RecordSubject = Pick<Jewel, 'recordHash' | 'amendments' | 'inscription' | 'jewelerId'>;

/**
 * A file can match its fingerprint and still describe another jewel: a jeweler
 * could register a cheap piece under the record of a fine one. A record counts
 * as this jewel's only if it names the inscription and the jeweler the chain
 * holds for it.
 */
export function recordMatches(record: JewelRecord, jewel: Pick<Jewel, 'inscription' | 'jewelerId'>): boolean {
  return record.inscription === jewel.inscription && record.jewelerId === jewel.jewelerId;
}

/** A record that describes another jewel is reported as a mismatch, never shown. */
export function checkRecord(state: RecordState, jewel: Pick<Jewel, 'inscription' | 'jewelerId'>): RecordState {
  return state.state === 'ok' && !recordMatches(state.record, jewel) ? { state: 'mismatch' } : state;
}

/**
 * The record of a jewel as read from the chain, checked against it. `which`
 * picks the current record (the last correction) or the original one.
 * Every page reads records through here, so none can show an unchecked one.
 */
export function useRecord(jewel: RecordSubject | null, which: 'current' | 'original' = 'current'): RecordState {
  const hash = jewel ? (which === 'current' ? currentRecordHash(jewel) : jewel.recordHash) : null;
  const [held, setHeld] = useState<{ hash: string | null; state: RecordState }>({ hash: null, state: { state: 'loading' } });
  useEffect(() => {
    if (!hash) return;
    let live = true;
    fetchVerified(hash).then((f) => {
      if (!live) return;
      if (!f.ok) { setHeld({ hash, state: { state: f.reason } }); return; }
      const record = parseRecord(f.bytes);
      setHeld({ hash, state: record ? { state: 'ok', record } : { state: 'unreadable' } });
    });
    return () => { live = false; };
  }, [hash]);
  const state = stateForHash(held, hash);
  return jewel ? checkRecord(state, jewel) : state;
}
