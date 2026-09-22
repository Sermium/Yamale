// What this surface's stylesheet cannot be checked for by looking at it.
//
// Three defects, each of which renders perfectly on the machine it was written
// on and is wrong somewhere else.
//
// 1. A COLOUR DEFINED ONLY INSIDE A MEDIA QUERY.
//
//    The theme has three states: an explicit `data-theme="dark"`, an explicit
//    `data-theme="light"`, and the un-stamped default where only
//    `prefers-color-scheme` separates them. A token whose sole definition sits
//    under `@media (prefers-color-scheme: dark)` is UNDEFINED in the un-stamped
//    light state, which is the state most readers are in — and the symptom is
//    one theme's text on the other theme's ground. That has shipped in this
//    repository once already.
//
//    Measured in the browser rather than trusted from the source: every token
//    this file adds resolves in all three states, verified at 390px.
//
// 2. A NUMBER BREAKING ACROSS TWO LINES.
//
//    `overflow-wrap: anywhere` printed `100,000,0` above `00` twice in this
//    repository. On this surface the same defect is worse, because
//    `221/682/021029/L` gives the browser four break opportunities and both
//    halves of a broken identifier look like perfectly good identifiers.
//
// 3. AN IDENTIFIER REORDERED BY THE BIDI ALGORITHM.
//
//    Found by looking, in Arabic, at 390px: `221/682/000102/L` rendered as
//    `L/221/682/000102`, and `0 / 9` rendered as `9 / 0`. The solidi and the
//    trailing breed letter are bidi-neutral, so in an RTL paragraph the
//    algorithm reorders the run and prints a DIFFERENT identifier that looks
//    entirely plausible. Nothing about the layout looks wrong.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (...p) => fs.readFileSync(path.join(here, ...p), 'utf8');

const css = read('src', 'styles.css');
const html = read('index.html');

const withoutComments = css.replace(/\/\*[\s\S]*?\*\//g, '');

/* ------------------------------------------------- 1. the three-state rule */

test('every custom property is declared outside a conditional block', () => {
  // Split on the blocks that only some readers are inside. Anything that
  // remains is unconditional, and a token declared there is defined for every
  // reader whatever their device and whatever they have chosen.
  const conditional = [];
  const re = /(@media[^{]*\{|:root\[data-theme=[^\]]*\]\s*\{)/g;
  let m;
  while ((m = re.exec(withoutComments)) !== null) {
    // Take the rest of the file from here and let the unconditional scan below
    // decide; a precise brace-matcher is not needed to answer the question,
    // which is only "is this token also declared at the top level".
    conditional.push(m.index);
  }
  const firstConditional = conditional.length ? Math.min(...conditional) : withoutComments.length;
  const unconditional = withoutComments.slice(0, firstConditional);

  // Anchored to the start of a declaration, so `.btn--primary:hover` is not
  // read as a custom property called `--primary`.
  const DECL = /(?:^|[;{])\s*(--[a-z0-9-]+)\s*:/gm;
  const names = (s) => new Set([...s.matchAll(DECL)].map((m) => m[1]));
  const declared = names(withoutComments);
  const atRoot = names(unconditional);

  for (const token of declared) {
    assert.ok(atRoot.has(token),
      `${token} is only ever declared inside a media query or a [data-theme] block. `
      + 'In the un-stamped state it resolves to nothing. Declare it at :root as well.');
  }
});

test('the dark palette is redeclared in both places the shared sheet uses', () => {
  // The shared system redefines its tokens under BOTH
  // `@media (prefers-color-scheme: dark) :root:not([data-theme='light'])` and
  // `:root[data-theme='dark']`. A local layer that carried only one of the two
  // would break exactly one of the three states, which is the hardest kind of
  // theme bug to notice.
  assert.match(withoutComments, /@media \(prefers-color-scheme: dark\)\s*\{\s*:root:not\(\[data-theme='light'\]\)/,
    'no system-preference block guarded against an explicit light choice');
  assert.match(withoutComments, /:root\[data-theme='dark'\]\s*\{/,
    'no explicit dark block');

  const sysBlock = withoutComments.match(
    /@media \(prefers-color-scheme: dark\)\s*\{\s*:root:not\(\[data-theme='light'\]\)\s*\{([\s\S]*?)\}/,
  )[1];
  const explicit = withoutComments.match(/:root\[data-theme='dark'\]\s*\{([\s\S]*?)\}/)[1];

  const names = (s) => [...s.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((x) => x[1]).sort();
  assert.deepEqual(names(sysBlock), names(explicit),
    'the two dark blocks define different sets of tokens, so one theme state is wrong');
});

/* --------------------------------------------- 2. and 3. the identifiers */

test('nothing lets a number or an identifier break across two lines', () => {
  assert.ok(!/overflow-wrap:\s*anywhere/.test(withoutComments),
    'overflow-wrap: anywhere has printed 100,000,0 above 00 in this repository twice');

  // The classes that carry a figure or an identifier, and the pair of
  // declarations that holds each one together.
  const rule = withoutComments.match(/\.natid[^{]*\{([\s\S]*?)\}/);
  assert.ok(rule, 'no rule covers .natid');
  assert.match(rule[1], /word-break:\s*keep-all/);
  assert.match(rule[1], /overflow-wrap:\s*normal/);
});

test('an identifier is pinned to left-to-right order', () => {
  const rule = withoutComments.match(/\.natid[^{]*\{([\s\S]*?)\}/)[1];
  assert.match(rule, /direction:\s*ltr/,
    'without this, 221/682/000102/L renders as L/221/682/000102 in Arabic');
  assert.match(rule, /unicode-bidi:\s*isolate/,
    'embed would let the run reorder the Arabic text around it');
});

/* ------------------------------------------------- logical properties only */

test('the stylesheet uses no physical direction', () => {
  // `direction: ltr` and `[dir=rtl]` are the two deliberate exceptions and are
  // statements about CONTENT rather than about layout: an identifier and a
  // number line read left to right in every locale. Everything else mirrors.
  const offenders = [];
  const physical = /(?:^|[\s;{])((?:margin|padding|border|inset)?-?(?:left|right)\s*:)/g;
  for (const line of withoutComments.split('\n')) {
    physical.lastIndex = 0;
    if (physical.test(line)) offenders.push(line.trim());
  }
  assert.deepEqual(offenders, [],
    'physical properties do not mirror when setLocale writes dir="rtl"');
});

test('a chevron is the only thing mirrored by hand', () => {
  // An SVG path cannot be written in logical coordinates, so the two glyphs
  // that point somewhere are flipped explicitly. Anything else appearing here
  // means a layout is being corrected instead of written logically.
  const flips = [...withoutComments.matchAll(/\[dir='rtl'\][^{]*\{[^}]*\}/g)].map((m) => m[0]);
  assert.ok(flips.length >= 1, 'nothing is mirrored, which for an SVG chevron is wrong');
  for (const flip of flips) {
    assert.ok(/transform:\s*scaleX\(-1\)|direction:\s*ltr/.test(flip),
      `an [dir='rtl'] block is doing something other than flipping a glyph: ${flip}`);
  }
});

/* -------------------------------------------------------- the field device */

test('the primary target is sized for a gloved finger', () => {
  const root = withoutComments.match(/:root\s*\{([\s\S]*?)\}/)[1];
  const hit = root.match(/--hit:\s*(\d+)px/);
  assert.ok(hit, '--hit is not declared');
  assert.ok(Number(hit[1]) >= 56,
    '44px is the floor for a finger; a gloved finger on a moving animal is not that finger');

  const primary = withoutComments.match(/\.btn--primary\s*\{([\s\S]*?)\}/)[1];
  assert.match(primary, /min-block-size:\s*var\(--hit\)/);
});

test('the text field does not make a phone zoom the page on focus', () => {
  // Below 16px, mobile Safari zooms in when an input takes focus — which on a
  // surface shaped like a device is a layout that jumps under the thumb.
  const input = withoutComments.match(/\.bigfield__input\s*\{([\s\S]*?)\}/)[1];
  const size = input.match(/font-size:\s*([\d.]+)rem/);
  assert.ok(size && Number(size[1]) >= 1,
    'the identifier field must be at least 16px');
});

test('the placeholder cannot be mistaken for a value', () => {
  // An example identifier sitting in an empty field, in the text colour, on the
  // one screen whose entire purpose is not to show an identity it does not have.
  assert.match(withoutComments, /\.bigfield__input::placeholder\s*\{[^}]*color:\s*var\(--text-faint\)/);
});

/* ------------------------------------------------------------- the document */

test('the page carries the viewport and the typefaces', () => {
  assert.match(html, /viewport-fit=cover/,
    'without this the action bar sits above the home indicator on a notched device');
  assert.match(html, /fonts\.googleapis\.com\/css2\?family=Archivo/);
  assert.match(html, /IBM\+Plex\+Sans\+Arabic/,
    'Arabic is one of the five languages and Plex Sans Arabic is its designed companion');
});
