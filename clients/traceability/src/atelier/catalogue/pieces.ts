/**
 * The pieces in a jeweler's CSV, one physical piece per row, judged before
 * anything is signed. Each row gets exactly one verdict, the first that
 * applies, in the order the spec fixes (plus "tooLong", second).
 */

import { normaliseInscription } from '../../../../sdk/src/jewel.ts';
import type { JewelRecordV2, Stone } from '../../verify/record.ts';
import { encodeRecord, parseRecord } from '../../verify/record.ts';
import { toCsv, type Csv } from './csv.ts';

export interface PieceRow { index: number; cells: string[]; serial: string; sku: string; weight: string; labCertificate: string; notes: string }
export type PiecesResult = { ok: true; rows: PieceRow[]; ignored: string[] } | { ok: false; problem: 'noSerial' | 'noSku' };

/** What the verdicts and records need of a catalogue design (api.ts Design is assignable to it). */
export interface CatalogueDesign {
  key: string;
  status: string;
  description: string;
  productUrl: string;
  listPrice: { amount: string; currency: string } | null;
  confirmed: { title: string; fields: { metal: string; weightGrams: string; stones: Stone[] }; photos: string[] } | null;
}

export const VERDICTS = ['invalidSerial', 'tooLong', 'duplicate', 'unknownSku', 'notConfirmed', 'noWeight', 'unreadableRecord', 'alreadyRegistered', 'ready'] as const;
export type Verdict = typeof VERDICTS[number];

export interface Judged { row: PieceRow; verdict: Verdict; inscription: string | null; design: CatalogueDesign | null; weightGrams: string; jewelId: string }

const KNOWN = ['serial', 'sku', 'weight_g', 'lab_certificate', 'notes'];
const DECIMAL = /^[0-9]+(\.[0-9]+)?$/;

export function readPieces(csv: Csv): PiecesResult {
  const names = csv.header.map((h) => h.trim().toLowerCase());
  const at = (n: string) => names.indexOf(n);
  if (at('serial') < 0) return { ok: false, problem: 'noSerial' };
  if (at('sku') < 0) return { ok: false, problem: 'noSku' };
  const get = (cells: string[], n: string) => (at(n) >= 0 ? (cells[at(n)] ?? '').trim() : '');
  return {
    ok: true,
    ignored: csv.header.filter((h, i) => h.trim() !== '' && !KNOWN.includes(names[i]!)),
    rows: csv.rows.map((cells, index) => ({
      index, cells, serial: get(cells, 'serial'), sku: get(cells, 'sku'), weight: get(cells, 'weight_g'),
      labCertificate: get(cells, 'lab_certificate'), notes: get(cells, 'notes'),
    })),
  };
}

/** A row's own weight: '' when it gives none, null when what it gives is not a weight. */
function rowWeight(raw: string): string | null {
  const w = raw.trim().replace(',', '.');
  if (w === '') return '';
  return DECIMAL.test(w) && w.length <= 12 ? w : null;
}

export function judgePieces(rows: PieceRow[], designs: CatalogueDesign[], registered: ReadonlyMap<string, string>,
  jeweler?: { id: string; name: string }): Judged[] {
  const exact = new Map(designs.map((d) => [d.key, d]));
  const folded = new Map<string, CatalogueDesign | null>();
  for (const d of designs) {
    const k = d.key.toLowerCase();
    folded.set(k, folded.has(k) ? null : d);
  }
  const seen = new Set<string>();
  return rows.map((row): Judged => {
    const inscription = normaliseInscription(row.serial);
    const base = { row, inscription, design: null, weightGrams: '', jewelId: '' };
    if (!inscription) return { ...base, verdict: 'invalidSerial' };
    if (row.labCertificate.length > 200 || row.notes.length > 500) return { ...base, verdict: 'tooLong' };
    if (seen.has(inscription)) return { ...base, verdict: 'duplicate' };
    seen.add(inscription);
    const design = exact.get(row.sku) ?? folded.get(row.sku.toLowerCase()) ?? null;
    if (!design) return { ...base, verdict: 'unknownSku' };
    if (design.status !== 'confirmed' || !design.confirmed) return { ...base, design, verdict: 'notConfirmed' };
    const own = rowWeight(row.weight);
    const weightGrams = own === '' ? design.confirmed.fields.weightGrams : own;
    if (own === null || !weightGrams || !DECIMAL.test(weightGrams)) return { ...base, design, verdict: 'noWeight' };
    const judged = { ...base, design, weightGrams };
    // The server clips by characters and parseRecord limits by UTF-16 units, so a record can still fail to read back.
    if (jeweler && parseRecord(encodeRecord(pieceRecord(judged as Judged, jeweler, new Date(0)))) === null) return { ...judged, verdict: 'unreadableRecord' };
    const id = registered.get(inscription);
    if (id) return { ...judged, jewelId: id, verdict: 'alreadyRegistered' };
    return { ...judged, verdict: 'ready' };
  });
}

type Found = { ok: true; value: { jewelerId: string; id: string }[] } | { ok: false; reason: string; detail: string };

/** Which of these codes this jeweler already registered, asked `parallel` at a time. */
export async function findRegistered(inscriptions: string[], jewelerId: string, find: (ins: string) => Promise<Found>,
  parallel = 8): Promise<{ ok: true; registered: Map<string, string> } | { ok: false; detail: string }> {
  const registered = new Map<string, string>();
  for (let i = 0; i < inscriptions.length; i += parallel) {
    const slice = inscriptions.slice(i, i + parallel);
    const answers = await Promise.all(slice.map((ins) => find(ins)));
    for (let j = 0; j < slice.length; j++) {
      const a = answers[j]!;
      if (!a.ok) return { ok: false, detail: a.detail };
      const mine = a.value.find((x) => x.jewelerId === jewelerId);
      if (mine) registered.set(slice[j]!, mine.id);
    }
  }
  return { ok: true, registered };
}

/** The record of one ready piece: the confirmed design, plus what the row says of this piece. */
export function pieceRecord(j: Judged, jeweler: { id: string; name: string }, now: Date): JewelRecordV2 {
  const d = j.design!;
  const c = d.confirmed!;
  const iso = now.toISOString();
  return {
    version: 2, jewelerId: jeweler.id, inscription: j.inscription!, title: c.title, metal: c.fields.metal,
    weightGrams: j.weightGrams, stones: c.fields.stones.map((s) => ({ kind: s.kind, carat: s.carat, count: s.count })),
    origin: { workshop: jeweler.name, stoneSource: '', labCertificate: j.row.labCertificate },
    photos: [...c.photos], createdAt: iso,
    description: d.description, productUrl: d.productUrl,
    sku: d.key.startsWith('handle:') || d.key.startsWith('url:') ? '' : d.key,
    listPrice: d.listPrice ? { amount: d.listPrice.amount, currency: d.listPrice.currency, at: iso.slice(0, 10), label: 'list price at registration' } : null,
    pieceNotes: j.row.notes,
  };
}

/** The original rows with extra columns, as a CSV in the file's own separator. */
export function withColumns(csv: Csv, judged: Judged[], extra: (j: Judged) => Record<string, string>): string {
  const names = judged.length ? Object.keys(extra(judged[0]!)) : [];
  const width = csv.header.length;
  const rows = judged.map((j) => {
    const cells = [...j.row.cells];
    while (cells.length < width) cells.push('');
    const add = extra(j);
    return [...cells.slice(0, width), ...names.map((n) => add[n] ?? '')];
  });
  return toCsv([...csv.header, ...names], rows, csv.separator);
}

/** A design code folded as an engraved code is: upper case, letters and digits only. */
function foldCode(s: string): string { return s.toUpperCase().replace(/[^A-Z0-9]/g, ''); }

/**
 * The confirmed design a serial number belongs to, when the serial starts with the
 * design's code (JOA-071-0001 is a piece of JOA-071). The longest code wins, so
 * JOA-0711 is not taken for JOA-071; a serial that is only the code is no piece of it.
 * Designs keyed by their page rather than a code (handle:, url:) never match.
 */
export function designForSerial<D extends CatalogueDesign>(serial: string, designs: D[]): D | null {
  const s = foldCode(serial);
  let best: D | null = null;
  let bestLen = 0;
  for (const d of designs) {
    if (d.status !== 'confirmed' || !d.confirmed || d.key.startsWith('handle:') || d.key.startsWith('url:')) continue;
    const k = foldCode(d.key);
    if (k.length >= 2 && s.length > k.length && s.startsWith(k) && k.length > bestLen) { best = d; bestLen = k.length; }
  }
  return best;
}
