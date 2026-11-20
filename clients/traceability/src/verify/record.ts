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

export interface ListPrice { amount: string; currency: string; at: string; label: 'list price at registration' }

interface RecordBase {
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

/** Written by hand registration and by corrections of hand-registered jewels. */
export interface JewelRecordV1 extends RecordBase { version: 1 }

/**
 * Written by the catalogue import: the design's own description, its page on
 * the jeweler's site, its SKU, the list price on the day of registration (a
 * dated label, never a valuation), and the piece's own notes.
 */
export interface JewelRecordV2 extends RecordBase {
  version: 2;
  description: string;
  productUrl: string;
  sku: string;
  listPrice: ListPrice | null;
  pieceNotes: string;
}

export type JewelRecord = JewelRecordV1 | JewelRecordV2;

const HASH = /^[0-9a-f]{64}$/;
const ID = /^[1-9][0-9]*$/;
const str = (v: unknown, max: number): v is string => typeof v === 'string' && v.length <= max;

const LIST_PRICE_LABEL = 'list price at registration';
const DAY = /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/;
const AMOUNT = /^[0-9]+(\.[0-9]+)?$/;

/** Keys in one fixed order, so one record always has one fingerprint. v1 bytes are what they always were. */
export function encodeRecord(r: JewelRecord): Uint8Array {
  const base = {
    version: r.version,
    jewelerId: r.jewelerId,
    inscription: r.inscription,
    title: r.title,
    metal: r.metal,
    weightGrams: r.weightGrams,
    stones: r.stones.map((s) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: r.origin.workshop, stoneSource: r.origin.stoneSource, labCertificate: r.origin.labCertificate },
    photos: [...r.photos],
    createdAt: r.createdAt,
  };
  if (r.version === 1) return new TextEncoder().encode(JSON.stringify(base));
  return new TextEncoder().encode(JSON.stringify({
    ...base,
    description: r.description,
    productUrl: r.productUrl,
    sku: r.sku,
    listPrice: r.listPrice ? { amount: r.listPrice.amount, currency: r.listPrice.currency, at: r.listPrice.at, label: r.listPrice.label } : null,
    pieceNotes: r.pieceNotes,
  }));
}

/** 2026-02-30 matches the shape of a day and is not one; a formatter would throw on it. */
function isRealDay(at: string): boolean {
  const d = new Date(`${at}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === at;
}

function parseListPrice(v: any): ListPrice | null | undefined {
  if (v === null) return null;
  if (!v || !str(v.amount, 20) || !AMOUNT.test(v.amount) || !str(v.currency, 3) || !/^[A-Z]{3}$/.test(v.currency)
    || !str(v.at, 10) || !DAY.test(v.at) || !isRealDay(v.at) || v.label !== LIST_PRICE_LABEL) return undefined;
  return { amount: v.amount, currency: v.currency, at: v.at, label: LIST_PRICE_LABEL };
}

export function parseRecord(bytes: Uint8Array): JewelRecord | null {
  let v: any;
  try { v = JSON.parse(new TextDecoder().decode(bytes)); } catch { return null; }
  if (!v || (v.version !== 1 && v.version !== 2) || !str(v.jewelerId, 20) || !ID.test(v.jewelerId)) return null;
  if (!str(v.inscription, 40) || !str(v.title, 120) || !str(v.metal, 80) || !str(v.weightGrams, 12) || !str(v.createdAt, 40)) return null;
  if (!Array.isArray(v.stones) || v.stones.length > 50) return null;
  for (const s of v.stones) {
    if (!s || !str(s.kind, 60) || !str(s.carat, 12) || !Number.isInteger(s.count) || s.count < 1 || s.count > 9999) return null;
  }
  const o = v.origin;
  const labMax = v.version === 1 ? 80 : 200;
  if (!o || !str(o.workshop, 120) || !str(o.stoneSource, 120) || !str(o.labCertificate, labMax)) return null;
  if (!Array.isArray(v.photos) || v.photos.length < 1 || v.photos.length > 12) return null;
  if (!v.photos.every((p: unknown) => typeof p === 'string' && HASH.test(p))) return null;
  const base: RecordBase = {
    jewelerId: v.jewelerId, inscription: v.inscription, title: v.title, metal: v.metal, weightGrams: v.weightGrams,
    stones: v.stones.map((s: Stone) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: o.workshop, stoneSource: o.stoneSource, labCertificate: o.labCertificate },
    photos: [...v.photos], createdAt: v.createdAt,
  };
  if (v.version === 1) return { version: 1, ...base };
  if (!str(v.description, 5000) || !str(v.productUrl, 500) || !str(v.sku, 80) || !str(v.pieceNotes, 500)) return null;
  if (v.productUrl !== '' && !v.productUrl.startsWith('https://')) return null;
  const listPrice = parseListPrice(v.listPrice);
  if (listPrice === undefined) return null;
  return { version: 2, ...base, description: v.description, productUrl: v.productUrl, sku: v.sku, listPrice, pieceNotes: v.pieceNotes };
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
