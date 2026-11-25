import assert from 'node:assert/strict';
import { test } from 'node:test';

import { MAX_CSV_ROWS, parseCsv, toCsv } from './csv.ts';

const ok = (text: string) => { const r = parseCsv(text); assert.ok(r.ok, JSON.stringify(r)); return r.csv; };

test('commas, CRLF and a trailing newline', () => {
  const c = ok('serial,sku\r\nJOA-071-0001,JOA-071\r\nJOA-071-0002,JOA-071\r\n');
  assert.deepEqual(c.header, ['serial', 'sku']);
  assert.deepEqual(c.rows, [['JOA-071-0001', 'JOA-071'], ['JOA-071-0002', 'JOA-071']]);
  assert.equal(c.separator, ',');
});

test('a semicolon header makes a semicolon file, and a decimal comma stays in its cell', () => {
  const c = ok('serial;sku;weight_g\nA0001;JOA-071;4,2\n');
  assert.equal(c.separator, ';');
  assert.deepEqual(c.rows[0], ['A0001', 'JOA-071', '4,2']);
});

test('a BOM is dropped', () => {
  assert.deepEqual(ok('﻿serial,sku\nA0001,X\n').header, ['serial', 'sku']);
});

test('quoted fields hold separators, newlines and doubled quotes', () => {
  const c = ok('serial,sku,notes\nA0001,X,"Re-sized, then ""polished""\nby hand"\n');
  assert.deepEqual(c.rows[0], ['A0001', 'X', 'Re-sized, then "polished"\nby hand']);
});

test('blank lines are skipped and short rows are kept as they are', () => {
  assert.deepEqual(ok('serial,sku\n\nA0001\n').rows, [['A0001']]);
});

test('what cannot be read says why', () => {
  assert.deepEqual(parseCsv(''), { ok: false, problem: 'empty' });
  assert.deepEqual(parseCsv('serial,sku\nA,"open'), { ok: false, problem: 'unterminatedQuote' });
  assert.deepEqual(parseCsv('serial,sku\n' + 'A0001,X\n'.repeat(MAX_CSV_ROWS + 1)), { ok: false, problem: 'tooManyRows' });
  assert.deepEqual(parseCsv('x'.repeat(1_000_001)), { ok: false, problem: 'tooBig' });
});

test('a written CSV reads back the same', () => {
  const rows = [['A0001', 'Re-sized, "twice"\nok'], ['A0002', '']];
  const c = ok(toCsv(['serial', 'notes'], rows, ';'));
  assert.deepEqual(c.rows, rows);
});
