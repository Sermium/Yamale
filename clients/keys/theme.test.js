// This page links the shared stylesheet rather than inlining it, so the drift
// test that guards the inlining consoles does not apply here. What does apply is
// the half of the visual system that a linked stylesheet CANNOT deliver on its
// own, and that was missing for as long as this page has existed:
//
// THE TYPEFACES ARE A SEPARATE REQUEST. clients/shared/yamale.css names Archivo
// and IBM Plex in --font-display/--font-sans/--font-mono and deliberately does
// not @import them, so that each surface's HTML can start the fetch before the
// stylesheet has parsed. A page that inherits the stylesheet and forgets the
// link therefore asks for faces it never fetches and falls through to the
// system stack — which is the exact failure the shared file was written to end:
// "the product's headline was rendered in whatever face the reader's operating
// system happened to install."
//
// It goes unnoticed because a missing link and a blocked font host render
// identically. Nothing on screen says which one you are looking at.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const page = fs.readFileSync(path.join(here, 'index.html'), 'utf8');
const style = page.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

test('the shared stylesheet is linked', () => {
  assert.ok(/<link rel="stylesheet" href="\/yamale\.css">/.test(page),
    'the page must link /yamale.css, which resolves at the deployed root');
});

test('the typefaces the shared stylesheet names are actually requested', () => {
  assert.ok(/<link rel="preconnect" href="https:\/\/fonts\.googleapis\.com">/.test(page),
    'no preconnect to the font host');
  assert.ok(/<link rel="preconnect" href="https:\/\/fonts\.gstatic\.com" crossorigin>/.test(page),
    'no preconnect to the font CDN — the stylesheet resolves, the font files then wait');
  const sheet = page.match(/href="(https:\/\/fonts\.googleapis\.com\/css2\?[^"]+)"/)?.[1] ?? '';
  for (const family of ['family=Archivo', 'family=IBM+Plex+Sans:', 'family=IBM+Plex+Sans+Arabic',
    'family=IBM+Plex+Mono']) {
    assert.ok(sheet.includes(family), `${family} is not requested`);
  }
  assert.ok(sheet.includes('display=swap'),
    'without display=swap the page is blank while the fonts load');
});

test('the fonts are asked for before the stylesheet that uses them', () => {
  // A preconnect issued after the blocking stylesheet has already been
  // requested has bought nothing.
  assert.ok(page.indexOf('fonts.googleapis.com') < page.indexOf('href="/yamale.css"'),
    'the font links must come first in the head');
});

test('this page still defines no palette of its own beyond the three parties', () => {
  // The local stylesheet used to carry --k-ground, --k-panel, --k-ink and nine
  // more, which is how one page becomes a slightly different product from the
  // eleven beside it. Only the categorical scale for the three shares survives,
  // and it survives because the shared system has no categorical scale — only a
  // semantic one, where the hue would say the custodian is the good party.
  const declared = [...style.matchAll(/^\s*(--[a-z0-9-]+)\s*:/gim)].map((m) => m[1]);
  const allowed = new Set(['--party-device', '--party-custodian', '--party-recovery']);
  const strays = [...new Set(declared)].filter((t) => !allowed.has(t));
  assert.deepEqual(strays, [],
    'these are a local palette; the shared tokens already have a name for them');
});

test('each party colour exists in all three theme states', () => {
  // The un-stamped default, where only prefers-color-scheme decides, is the
  // state most readers are in. A colour defined only inside the media query is
  // undefined there.
  for (const token of ['--party-device', '--party-custodian', '--party-recovery']) {
    const at = [...style.matchAll(new RegExp(`${token}\\s*:`, 'g'))].length;
    assert.equal(at, 3, `${token} must be defined at :root, in the media query, and in [data-theme='dark']`);
  }
  assert.ok(/@media\s*\(prefers-color-scheme:\s*dark\)/.test(style), 'no prefers-color-scheme block');
  assert.ok(/:root:not\(\[data-theme=['"]light['"]\]\)/.test(style),
    'the media block must exclude an explicit light choice, or a toggle cannot win');
  assert.ok(/:root\[data-theme=['"]dark['"]\]/.test(style), 'no explicit dark block');
});

test('no figure is allowed to break across two lines', () => {
  // `overflow-wrap: anywhere` has printed "100,000,0" above "00" in this
  // codebase twice.
  assert.ok(!/overflow-wrap:\s*anywhere/.test(style),
    'overflow-wrap: anywhere is how a number gets cut in half');
});

test('the data tables do not re-decide what the shared file decides', () => {
  // This file used to restore the trailing padding on a non-last `.num` column,
  // because the shared file zeroed it unconditionally. The shared file now
  // zeroes it only on a LAST column, so the local rule is a second rule for one
  // decision — and two rules for one decision is how the next person fixes the
  // wrong one.
  assert.ok(!/\.y-data\s+t[dh]\.num:not\(:last-child\)/.test(style),
    'clients/shared/yamale.css already scopes this to the last column');
});
