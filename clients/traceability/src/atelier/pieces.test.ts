import assert from 'node:assert/strict';
import { test } from 'node:test';

import { parseCsv } from './csv.ts';
import { findRegistered, judgePieces, pieceRecord, readPieces, withColumns, type CatalogueDesign, type PieceRow } from './pieces.ts';

const H = 'a'.repeat(64);
const parrot: CatalogueDesign = {
  key: 'JOA-071', status: 'confirmed', description: 'Twelve emeralds.', productUrl: 'https://www.jewelofafrica.com/products/emerald-parrot-pendant-071',
  listPrice: { amount: '17678.00', currency: 'USD' },
  confirmed: { title: 'Emerald Parrot Pendant', fields: { metal: '18kt Yellow Gold', weightGrams: '', stones: [{ kind: 'Emerald', carat: '32', count: 12 }] }, photos: [H] },
};
const ring: CatalogueDesign = { ...parrot, key: 'handle:emerald-ring-1', confirmed: { ...parrot.confirmed!, fields: { ...parrot.confirmed!.fields, weightGrams: '6.5' } } };
const draft: CatalogueDesign = { ...parrot, key: 'JOA-090', status: 'drafted', confirmed: null };

function rows(text: string): PieceRow[] {
  const r = parseCsv(text);
  assert.ok(r.ok);
  const p = readPieces(r.csv);
  assert.ok(p.ok);
  return p.rows;
}

test('columns are found by name, in any order and case, and the rest are listed', () => {
  const r = parseCsv('SKU;Serial;Weight_g;colour\nJOA-071;A0001;4,2;green\n');
  assert.ok(r.ok);
  const p = readPieces(r.csv);
  assert.ok(p.ok);
  assert.deepEqual(p.ignored, ['colour']);
  assert.equal(p.rows[0]!.serial, 'A0001');
  assert.equal(p.rows[0]!.weight, '4,2');
  const bad = parseCsv('sku,weight_g\nX,1\n');
  assert.ok(bad.ok);
  assert.deepEqual(readPieces(bad.csv), { ok: false, problem: 'noSerial' });
});

test('every row gets the first verdict that applies, in the spec order', () => {
  const judged = judgePieces(rows([
    'serial,sku,weight_g,lab_certificate,notes',
    'A!,JOA-071,,,',                     // invalid serial
    'B0001,JOA-071,,,' + 'n'.repeat(501), // too long
    'C0001,JOA-071,4.2,,',               // ready (weight from the row)
    'c-0001,JOA-071,4.2,,',              // listed twice (normalises to C0001)
    'D0001,NOPE,4.2,,',                  // unknown SKU
    'E0001,JOA-090,4.2,,',               // design not confirmed
    'F0001,JOA-071,,,',                  // no weight: neither the row nor the design states one
    'G0001,joa-071,heavy,,',             // no weight: the row's weight is not a number (SKU matched case-insensitively)
    'H0001,handle:emerald-ring-1,,,',    // ready (weight from the design)
    'I0001,JOA-071,5,,',                 // already registered
  ].join('\n')), [parrot, ring, draft], new Map([['I0001', '41']]));
  assert.deepEqual(judged.map((j) => j.verdict), ['invalidSerial', 'tooLong', 'ready', 'duplicate', 'unknownSku', 'notConfirmed',
    'noWeight', 'noWeight', 'ready', 'alreadyRegistered']);
  assert.equal(judged[2]!.weightGrams, '4.2');
  assert.equal(judged[8]!.weightGrams, '6.5');
  assert.equal(judged[9]!.jewelId, '41');
  assert.equal(judged[2]!.inscription, 'C0001');
});

test('a decimal comma is a weight', () => {
  const [j] = judgePieces(rows('serial;sku;weight_g\nA0001;JOA-071;4,2\n'), [parrot], new Map());
  assert.equal(j!.verdict, 'ready');
  assert.equal(j!.weightGrams, '4.2');
});

test("the register is asked about this jeweler's codes only, in small batches", async () => {
  const asked: string[] = [];
  const out = await findRegistered(['A0001', 'B0001', 'C0001'], '1', async (ins) => {
    asked.push(ins);
    return { ok: true, value: ins === 'B0001' ? [{ jewelerId: '2', id: '7' }, { jewelerId: '1', id: '9' }] : ins === 'C0001' ? [{ jewelerId: '2', id: '8' }] : [] };
  }, 2);
  assert.ok(out.ok);
  assert.deepEqual([...out.registered], [['B0001', '9']]);
  assert.deepEqual(asked.sort(), ['A0001', 'B0001', 'C0001']);
  const failed = await findRegistered(['A0001'], '1', async () => ({ ok: false, reason: 'unreachable', detail: 'down' }));
  assert.deepEqual(failed, { ok: false, detail: 'down' });
});

test("a piece's record is the confirmed design plus the row", () => {
  const [j] = judgePieces(rows('serial,sku,weight_g,lab_certificate,notes\nJOA 071 0001,JOA-071,7.9,GIA 123,Clasp checked\n'), [parrot], new Map());
  const r = pieceRecord(j!, { id: '1', name: 'Jewel Of Africa' }, new Date('2026-11-16T09:30:00Z'));
  assert.deepEqual(r, {
    version: 2, jewelerId: '1', inscription: 'JOA0710001', title: 'Emerald Parrot Pendant', metal: '18kt Yellow Gold',
    weightGrams: '7.9', stones: [{ kind: 'Emerald', carat: '32', count: 12 }],
    origin: { workshop: 'Jewel Of Africa', stoneSource: '', labCertificate: 'GIA 123' }, photos: [H],
    createdAt: '2026-11-16T09:30:00.000Z', description: 'Twelve emeralds.', productUrl: parrot.productUrl, sku: 'JOA-071',
    listPrice: { amount: '17678.00', currency: 'USD', at: '2026-11-16', label: 'list price at registration' },
    pieceNotes: 'Clasp checked',
  });
  const [h] = judgePieces(rows('serial,sku\nH0001,handle:emerald-ring-1\n'), [ring], new Map());
  assert.equal(pieceRecord(h!, { id: '1', name: 'J' }, new Date()).sku, '', 'a handle is not a SKU');
});

test('the rows to fix carry the original cells and a problem column', () => {
  const r = parseCsv('serial,sku,extra\nA!,JOA-071,x\nC0001,JOA-071,y\n');
  assert.ok(r.ok);
  const p = readPieces(r.csv);
  assert.ok(p.ok);
  // parrot states no weight and the rows give none, so C0001 would be noWeight: give the design one so it is ready.
  const weighed: CatalogueDesign = { ...parrot, confirmed: { ...parrot.confirmed!, fields: { ...parrot.confirmed!.fields, weightGrams: '6.5' } } };
  const judged = judgePieces(p.rows, [weighed], new Map());
  assert.deepEqual(judged.map((j) => j.verdict), ['invalidSerial', 'ready']);
  const out = withColumns(r.csv, judged.filter((j) => j.verdict !== 'ready'), (j) => ({ problem: j.verdict }));
  assert.equal(out, 'serial,sku,extra,problem\r\nA!,JOA-071,x,invalidSerial\r\n');
});
