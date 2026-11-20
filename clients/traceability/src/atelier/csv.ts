/**
 * CSV as RFC 4180 writes it: quoted fields may hold separators, newlines and
 * doubled quotes. The separator is a comma or a semicolon, whichever the
 * header line uses more (a spreadsheet in French writes semicolons). Parsed
 * in the browser, before anything is signed.
 */

export const MAX_CSV_BYTES = 1_000_000;
export const MAX_CSV_ROWS = 2000;

export interface Csv { header: string[]; rows: string[][]; separator: ',' | ';' }
export type CsvResult = { ok: true; csv: Csv } | { ok: false; problem: 'empty' | 'tooBig' | 'tooManyRows' | 'unterminatedQuote' };

function detectSeparator(text: string): ',' | ';' {
  let commas = 0, semis = 0, quoted = false;
  for (const ch of text) {
    if (ch === '"') quoted = !quoted;
    else if (!quoted && (ch === '\n' || ch === '\r')) break;
    else if (!quoted && ch === ',') commas++;
    else if (!quoted && ch === ';') semis++;
  }
  return semis > commas ? ';' : ',';
}

export function parseCsv(input: string): CsvResult {
  if (new TextEncoder().encode(input).length > MAX_CSV_BYTES) return { ok: false, problem: 'tooBig' };
  const text = input.startsWith('﻿') ? input.slice(1) : input;
  const separator = detectSeparator(text);
  const records: string[][] = [];
  let record: string[] = [];
  let field = '';
  let quoted = false;
  let i = 0;
  while (i < text.length) {
    const ch = text[i]!;
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') { field += '"'; i += 2; continue; }
        quoted = false; i++; continue;
      }
      field += ch; i++; continue;
    }
    if (ch === '"' && field === '') { quoted = true; i++; continue; }
    if (ch === separator) { record.push(field); field = ''; i++; continue; }
    if (ch === '\r' || ch === '\n') {
      record.push(field); records.push(record);
      record = []; field = '';
      i += ch === '\r' && text[i + 1] === '\n' ? 2 : 1;
      continue;
    }
    field += ch; i++;
  }
  if (quoted) return { ok: false, problem: 'unterminatedQuote' };
  if (field !== '' || record.length > 0) { record.push(field); records.push(record); }
  const kept = records.filter((r) => !(r.length === 1 && r[0]!.trim() === ''));
  if (kept.length === 0) return { ok: false, problem: 'empty' };
  const [header, ...rows] = kept;
  if (rows.length > MAX_CSV_ROWS) return { ok: false, problem: 'tooManyRows' };
  return { ok: true, csv: { header: header!.map((h) => h.trim()), rows, separator } };
}

function cell(v: string, separator: string): string {
  return /["\r\n]/.test(v) || v.includes(separator) ? `"${v.replaceAll('"', '""')}"` : v;
}

export function toCsv(header: string[], rows: string[][], separator: ',' | ';'): string {
  return [header, ...rows].map((r) => r.map((v) => cell(v, separator)).join(separator)).join('\r\n') + '\r\n';
}
