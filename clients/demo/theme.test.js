// What this page's stylesheets cannot be checked for by reading them.
//
// 1. THE COPY OF THE SHARED SYSTEM.
//
// The tour ships `yamale.css` as its own file next to `tour.css` rather than
// linking ../shared/yamale.css, because the deployment serves this page from
// /demo/ and /shared/yamale.css answers 404 there — the same measured reason
// the land console inlines it.
//
// A copy drifts. This one had drifted 130 meaningful lines: no --brass-ink or
// --warn-ink, the pre-contrast --ok/--bad/--text-muted, no right-alignment for
// numeric table cells, and none of the INFORMATION DESIGN block. Because it is
// a separate file and not an inlined <style> block, the copy has no reason to
// differ from the original at all, so this test asks for more than the land
// console's ordered-subset check: byte-for-byte identical, line endings aside.
//
// Anything the tour needs that the shared system does not have belongs in
// tour.css, which is the demo's own layer. Two rules for one decision is how
// the next person fixes the wrong one.
//
// 2. THE THREE-STATE THEME RULE, FOR THE DEMO'S OWN LAYER.
//
// The theme has three states: an explicit `data-theme="dark"`, an explicit
// `data-theme="light"`, and the un-stamped default where only
// `prefers-color-scheme` separates them. yamale.css gets this right and test 1
// pins it, but what the browser renders is yamale.css PLUS tour.css — so a
// colour whose sole definition sat inside a media query or a [data-theme] block
// in tour.css would produce exactly the same bug, one theme's text on the other
// theme's ground. That bug has shipped in this repository once already.
//
// tour.css used to carry three such tokens legitimately: local darkened copies
// of brass, amber and muted, defined in all three states, because the shared
// file was used by eleven surfaces and could not be darkened from inside one
// page. It since was — --brass-ink and --warn-ink exist centrally. The local
// copies are gone, and this test keeps them gone.
//
// 3. A NUMBER MUST NEVER BREAK ACROSS TWO LINES.
//
// `overflow-wrap: anywhere` rendered 100,000,000 as "100,000,0" above "00" in
// two places on this page. A number split in two is not a smaller number, it is
// a different one.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (...p) => fs.readFileSync(path.join(here, ...p), 'utf8');

const page = read('index.html');
const copy = read('yamale.css');
const tour = read('tour.css');
const shared = read('..', 'shared', 'yamale.css');

/** Line endings are a checkout setting, not content: clients/demo is not pinned
 *  in .gitattributes, so this file is CRLF on Windows and LF on the host that
 *  serves it. Comparing those bytes would fail on one platform and pass on the
 *  other, which is worse than not comparing them. */
const normalise = (css) => css.replace(/\r\n/g, '\n');

/** Comments and blank lines out; everything that renders, in order. The land
 *  console's definition, reused so the two tests measure the same thing. */
const meaningful = (css) => normalise(css)
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .split('\n').map((l) => l.trim()).filter(Boolean);

/* ------------------------------------------------- 1. the copy is the copy */

test('the page actually loads its copy of the shared system', () => {
  // A stylesheet that matches perfectly and is not linked is worth nothing.
  assert.match(page, /<link rel="stylesheet" href="yamale\.css">/,
    'index.html does not link yamale.css');
  assert.match(page, /<link rel="stylesheet" href="tour\.css">/,
    'index.html does not link tour.css');
  assert.ok(page.indexOf('href="yamale.css"') < page.indexOf('href="tour.css"'),
    'tour.css must come after yamale.css, or the demo layer cannot override it');
});

test('clients/demo/yamale.css is identical to clients/shared/yamale.css', () => {
  // Reported as the first differing line rather than as a whole-file diff,
  // because the whole-file diff of a 472-line stylesheet tells you nothing.
  const want = normalise(shared).split('\n');
  const have = normalise(copy).split('\n');
  for (let i = 0; i < Math.max(want.length, have.length); i += 1) {
    if (want[i] !== have[i]) {
      assert.fail(
        `clients/demo/yamale.css has drifted from the shared system at line ${i + 1}.\n`
        + `  shared: ${want[i] === undefined ? '(end of file)' : want[i]}\n`
        + `  demo:   ${have[i] === undefined ? '(end of file)' : have[i]}\n`
        + '  Copy clients/shared/yamale.css over it. Anything this page needs '
        + 'that the shared system lacks belongs in tour.css.');
    }
  }
  assert.equal(have.length, want.length);
});

test('the copy carries the lines the drift was measured against', () => {
  // Named explicitly so a regression reads as the thing that was missing rather
  // than as a line number: the contrast tokens, the numeric column fix, and the
  // information-design block that was absent entirely.
  for (const line of [
    '--brass-ink: #78582b;',
    '--warn-ink: #795814;',
    'td.y-num, th.y-num { text-align: end; }',
    '.y-chip--warn { background: var(--wash-warn); color: var(--warn-ink); }',
    '.y-kpis {',
    '.y-data { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }',
  ]) {
    assert.ok(meaningful(copy).includes(line.trim()),
      `the copy is missing: ${line}`);
  }
});

/* --------------------------------- 2. the three-state rule, in the demo layer */

test('tour.css defines no colour inside a conditional block', () => {
  // The demo's own layer names tokens and never hex, and declares no custom
  // property of its own. That is what makes the three theme states the shared
  // system's problem alone — which is where it is solved.
  const source = normalise(tour).replace(/\/\*[\s\S]*?\*\//g, '');
  // The lookbehind matters: `.mech__go--building:hover` is a BEM class, not a
  // declaration, and a naive /(--[a-z0-9-]+)\s*:/ reports it as one.
  const declared = [...source.matchAll(/(?<![\w-])(--[a-z0-9-]+)\s*:/gi)].map((m) => m[1]);
  assert.deepEqual(declared, [],
    'tour.css declares custom properties; the palette belongs in yamale.css, and '
    + 'a token defined here in only some theme states is the bug that shipped');
  assert.deepEqual([...source.matchAll(/#[0-9a-f]{3,8}\b/gi)].map((m) => m[0]), [],
    'tour.css names a raw colour; it must name tokens, so the three theme '
    + 'states follow from the shared system rather than from this file');
});

test('the local contrast repair is gone, now that the shared system carries it', () => {
  // --brass-ink and --warn-ink exist centrally and .y-eyebrow / .y-chip--warn
  // already name them. A second, local set would be two rules for one decision.
  for (const dead of ['--ink-eyebrow', '--ink-muted', '--ink-warn']) {
    const live = normalise(tour).replace(/\/\*[\s\S]*?\*\//g, '');
    assert.ok(!live.includes(dead),
      `${dead} is back in tour.css; the shared system darkens this colour now`);
  }
});

test('the three theme states all resolve, across both stylesheets together', () => {
  // Asserted on the concatenation the browser actually sees. The shared file is
  // correct by test 1; this rules out tour.css breaking it from below.
  const all = normalise(copy) + '\n' + normalise(tour);
  assert.match(all, /@media\s*\(prefers-color-scheme:\s*dark\)/,
    'no prefers-color-scheme: dark block — the un-stamped state has no dark');
  assert.match(all, /:root:not\(\[data-theme=['"]light['"]\]\)/,
    'the media block must exclude an explicit light choice, or the toggle cannot win');
  assert.match(all, /:root\[data-theme=['"]dark['"]\]/,
    'no explicit dark block — the toggle would do nothing');
  const firstRoot = all.indexOf(':root {');
  assert.ok(firstRoot !== -1 && firstRoot < all.indexOf('@media'),
    'the base light palette must come before any conditional block');
  for (const token of ['--bg', '--surface', '--text', '--text-muted', '--border',
    '--ok', '--warn', '--bad', '--accent', '--accent-ink', '--brass-ink', '--warn-ink']) {
    const base = all.slice(firstRoot, all.indexOf('@media'));
    assert.ok(base.includes(`${token}:`), `${token} is not defined at bare :root`);
  }
});

test('body paints its own ground', () => {
  assert.match(normalise(copy), /body\s*\{[^}]*background:\s*var\(--bg\)/,
    'body must set an explicit background from a token');
});

/* ------------------------------------------- 3. a figure never breaks in two */

test('no rule lets a number break across two lines', () => {
  // `overflow-wrap: anywhere` and `word-break: break-all` are the two ways to
  // split 100,000,000 into 100,000,0 and 00. They are legitimate on an address
  // or a hash, which is what .y-addr is for, and wrong on anything that carries
  // digits meant to be read as one quantity.
  const source = normalise(tour).replace(/\/\*[\s\S]*?\*\//g, '');
  const rules = [...source.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .map((m) => ({ selector: m[1].trim().split('\n').pop().trim(), body: m[2] }));

  const splitters = rules.filter((r) =>
    /overflow-wrap:\s*anywhere|word-break:\s*break-all/.test(r.body));

  const numeric = splitters.filter((r) =>
    /tabular-nums/.test(r.body) || /hero__stat|__value|y-num|kpi/.test(r.selector));

  assert.deepEqual(numeric.map((r) => r.selector), [],
    'these rules carry figures and also allow a break inside a word, which is '
    + 'how 100,000,000 rendered as "100,000,0" above "00" — use '
    + '`word-break: keep-all; overflow-wrap: normal;` as .y-kpi__value does');
});

test('the two figures that already shipped broken are pinned', () => {
  // Named, because a general rule is easy to satisfy by renaming a selector.
  const hero = normalise(tour).match(/\.hero__stat strong\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.ok(hero.includes('overflow-wrap: normal'),
    '.hero__stat strong holds a height and a block time; it must not break inside');
  const value = normalise(tour).match(/\.proof__value\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.ok(value.includes('overflow-wrap: normal'),
    '.proof__value holds amounts read from the chain; it must not break inside');
  // The values that genuinely cannot fit are marked `full` by the proof builder
  // and get .y-addr, whose break-all is correct for an address.
  assert.match(normalise(copy), /\.y-addr\s*\{[^}]*word-break:\s*break-all/,
    '.y-addr must still break, or a bech32 address overflows its column');
});
