// The two things about this console's stylesheet that cannot be checked by
// looking at it. Adapted from clients/land/theme.test.js, which carries the
// same two guarantees for the same two reasons.
//
// 1. THE INLINED COPY OF THE SHARED SYSTEM.
//
// This console is one file with no build step. It is opened when something has
// gone wrong — an account is frozen, a settlement window is stalled — often
// from a phone, and a page that needs a second request before it can say which
// is which renders as unstyled text at the moment it matters. Linking
// ../shared/yamale.css is also not available in practice: the deployment serves
// this page from /oversight/ and /shared/yamale.css answers 404 there.
//
// So the shared system is pasted in, and a pasted copy drifts. It had drifted
// twelve meaningful lines — the whole of the contrast repair: --brass-ink and
// --warn-ink absent in all three theme states, --text-muted, --ok and --bad
// still at their pre-repair values, and .y-eyebrow, .y-chip--warn and
// .y-chip--brass still naming the colours that do not clear 4.5:1. This test is
// what makes "current with clients/shared/yamale.css" a fact rather than a
// claim in a comment.
//
// 2. THE THREE-STATE THEME RULE.
//
// The theme has three states, not two: an explicit `data-theme="dark"`, an
// explicit `data-theme="light"`, and the default un-stamped state where only
// `prefers-color-scheme` separates them. A colour whose ONLY definition sits
// inside a media query or a `[data-theme]` block therefore never applies in one
// of them, and the symptom is one theme's text on the other theme's ground.
// That bug has shipped in this repository once already, which is why it is a
// test and not a note.
//
// It nearly shipped here a second time by a different route. This console used
// to define four ink tokens of its own, and a stray comment terminator closed
// the paragraph explaining them sixteen lines early, so the parser discarded
// everything up to the next block — which was the `:root` defining those very
// tokens. In the light theme they were undefined and the documented contrast
// fix was the one thing on the page not applied. The tokens are gone now (the
// shared system carries the repair), and the last test below is what would have
// caught that: it asks the palette to resolve, not merely to be written down.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const page = fs.readFileSync(path.join(here, 'index.html'), 'utf8');
const shared = fs.readFileSync(path.join(here, '..', 'shared', 'yamale.css'), 'utf8');

const style = page.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

/** Comments and blank lines out; everything that renders, in order. Line
 *  endings normalised first: clients/oversight is pinned eol=lf in
 *  .gitattributes and clients/shared is not, so on a Windows checkout the two
 *  files differ by a carriage return and by nothing else. */
const meaningful = (css) => css
  .replace(/\r\n/g, '\n')
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .split('\n').map((l) => l.trim()).filter(Boolean);

test('the page carries a stylesheet at all', () => {
  assert.ok(style.length > 2000, 'no <style> block found in index.html');
});

test('the inlined shared system still matches clients/shared/yamale.css', () => {
  // In order, and allowing the console's own rules to follow: this asks whether
  // the canonical file's lines are all present, in sequence, not whether the
  // console adds nothing of its own.
  const want = meaningful(shared);
  const have = meaningful(style);
  const missing = [];
  let i = 0;
  for (const line of want) {
    const at = have.indexOf(line, i);
    if (at === -1) missing.push(line);
    else i = at + 1;
  }
  assert.deepEqual(missing, [],
    'the inlined copy has drifted from clients/shared/yamale.css — paste it again');
});

/* --------------------------------------------------- the three-state rule */

/**
 * Every custom property, split by whether it is defined at bare `:root` or only
 * inside a conditional block.
 *
 * Crude on purpose: a real CSS parser would be another dependency in a file
 * whose whole argument is that it has none, and the failure being guarded is
 * coarse — a token that exists in one theme and not the other.
 */
function tokens(css) {
  const source = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const base = new Set();
  const conditional = new Set();

  let depth = 0;
  let inBase = false;
  let inConditional = 0;
  let buffer = '';

  for (let i = 0; i < source.length; i += 1) {
    const c = source[i];
    if (c === '{') {
      const selector = buffer.trim().split('\n').pop().trim();
      depth += 1;
      if (/^@media/.test(selector) || /\[data-theme/.test(selector)) inConditional += 1;
      else if (selector === ':root' && inConditional === 0) inBase = true;
      buffer = '';
      continue;
    }
    if (c === '}') {
      depth -= 1;
      if (inConditional > 0 && depth < inConditional) inConditional = depth;
      if (depth === 0) { inBase = false; inConditional = 0; }
      buffer = '';
      continue;
    }
    buffer += c;
    if (c === ';') {
      const m = buffer.match(/(--[a-z0-9-]+)\s*:/i);
      if (m) (inConditional > 0 ? conditional : inBase ? base : base).add(m[1]);
      buffer = '';
    }
  }
  return { base, conditional };
}

test('every token redefined for dark is also defined for light', () => {
  // The failure this rules out is the one that shipped: a colour whose sole
  // definition lives inside `@media (prefers-color-scheme: dark)` resolves to
  // nothing in the un-stamped light state, so the rule using it falls back to
  // an inherited value — one theme's text on the other theme's ground.
  const { base, conditional } = tokens(style);
  const orphans = [...conditional].filter((t) => !base.has(t));
  assert.deepEqual(orphans, [],
    'these colours are defined only inside a conditional block, so they do not '
    + 'exist in the un-stamped state');
});

test('the dark palette is defined twice — once per state that reaches it', () => {
  // Two states reach dark: the explicit `[data-theme="dark"]`, and the default
  // where `prefers-color-scheme` decides. Each needs its own block; a page that
  // defined only the first would ignore the reader's system setting, and one
  // that defined only the second would ignore the toggle. This console has a
  // toggle in its header, so both matter here.
  assert.ok(/@media\s*\(prefers-color-scheme:\s*dark\)/.test(style),
    'no prefers-color-scheme: dark block');
  assert.ok(/:root:not\(\[data-theme=['"]light['"]\]\)/.test(style),
    'the media block must exclude an explicit light choice, or the toggle cannot win');
  assert.ok(/:root\[data-theme=['"]dark['"]\]/.test(style),
    'no explicit dark block');
});

test('the light palette is at bare :root, so the un-stamped state resolves', () => {
  const firstRoot = style.indexOf(':root {');
  const firstMedia = style.indexOf('@media');
  assert.ok(firstRoot !== -1, 'no bare :root block');
  assert.ok(firstRoot < firstMedia, 'the base palette must come before any conditional block');
  for (const token of ['--bg', '--surface', '--text', '--text-muted', '--border',
    '--ok', '--warn', '--bad', '--accent', '--accent-ink', '--brass-ink', '--warn-ink']) {
    assert.ok(tokens(style).base.has(token), `${token} is not defined at bare :root`);
  }
});

test('every token this page names is a token this page defines', () => {
  // The guard against the stray comment terminator. That failure did not delete
  // a `var()` call — it deleted the `:root` block those calls referred to, so
  // every rule still read correctly and none of them resolved. Comparing what
  // is USED against what is DEFINED is the only check that sees it.
  const { base, conditional } = tokens(style);
  const defined = new Set([...base, ...conditional]);
  const used = new Set([...style.replace(/\/\*[\s\S]*?\*\//g, '')
    .matchAll(/var\((--[a-z0-9-]+)/gi)].map((m) => m[1]));
  const dangling = [...used].filter((t) => !defined.has(t));
  assert.deepEqual(dangling, [],
    'these tokens are used but never defined — a var() that resolves to nothing '
    + 'falls back to an inherited value, which is how an unapplied fix looks '
    + 'exactly like an applied one in the source');
});

test('body paints its own ground', () => {
  // A transparent body borrows whatever the host paints behind it, which on a
  // preview or artifact host is the wrong theme's colour.
  assert.ok(/body\s*\{[^}]*background:\s*var\(--bg\)/.test(style),
    'body must set an explicit background from a token');
});

/* ---------------------------------- the contrast repair lives in one place */

test('the local contrast repair is gone, now that the shared system carries it', () => {
  // This console carried --brass-ink, --warn-ink, --mute-ink and --ok-ink of its
  // own, with different values, because the shared palette could not be darkened
  // from inside one page. It since was: --brass-ink and --warn-ink are shared
  // tokens, and --text-muted and --ok were darkened at source, which is what
  // --mute-ink and --ok-ink existed for. Re-measured on --surface-3, the darkest
  // light ground this page uses, all four clear 4.5:1 without them.
  //
  // Two rules for one decision is how the next person fixes the wrong one.
  const source = style.replace(/\/\*[\s\S]*?\*\//g, '');
  for (const dead of ['--mute-ink', '--ok-ink']) {
    assert.ok(!source.includes(dead),
      `${dead} is back; --text-muted and --ok are darkened in the shared system now`);
  }
  // .y-chip--mute and .y-chip--ok take the shared semantic colour directly.
  assert.ok(source.includes('.y-chip--mute { background: var(--wash-mute); color: var(--text-muted); }'),
    '.y-chip--mute must take its colour from the shared token');
  assert.ok(source.includes('.y-chip--ok { background: var(--wash-ok); color: var(--ok); }'),
    '.y-chip--ok must take its colour from the shared token');
});

/* ------------------------------------------ what carries state to a reader */

test('no state is signalled by colour alone', () => {
  // One man in twelve cannot separate this page's green from its amber. Every
  // chip class is therefore paired with a word in the markup, and the chip
  // itself is mono and uppercase — a different shape, not only a different hue.
  for (const cls of ['.y-chip--ok', '.y-chip--warn', '.y-chip--bad',
    '.y-chip--mute', '.y-chip--brass']) {
    assert.ok(style.includes(cls), `${cls} is missing`);
  }
  assert.ok(/\.y-chip\s*\{[^}]*text-transform:\s*uppercase/.test(style),
    'chips must be distinguishable by form, not only by colour');
});

/* ----------------------------------------- a figure never breaks in two */

test('no rule lets a number break across two lines', () => {
  // `overflow-wrap: anywhere` rendered 100,000,000 as "100,000,0" above "00" on
  // a sibling surface. A number split in two is not a smaller number, it is a
  // different one. Breaking IS correct for the two things this page does it to
  // — a 45-character keeper path and an error string, neither of which is
  // compared by eye — so the rule is scoped to anything carrying digits.
  const source = style.replace(/\/\*[\s\S]*?\*\//g, '');
  const offenders = [...source.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .map((m) => ({ selector: m[1].trim().split('\n').pop().trim(), body: m[2] }))
    .filter((r) => /overflow-wrap:\s*anywhere|word-break:\s*break-all/.test(r.body))
    .filter((r) => /tabular-nums/.test(r.body)
      || /__n\b|y-num|kpi__value|amount|__value/.test(r.selector));
  assert.deepEqual(offenders.map((r) => r.selector), [],
    'these rules carry figures and also allow a break inside a word — use '
    + '`word-break: keep-all; overflow-wrap: normal;` as .y-kpi__value does');
});

test('the shared figure rule survived the paste', () => {
  // .y-kpi__value is where the shared system states the rule. If a resync ever
  // drops it the number-splitting comes back silently.
  const rule = style.match(/\.y-kpi__value\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.ok(rule.includes('word-break: keep-all'), 'no keep-all on .y-kpi__value');
  assert.ok(rule.includes('overflow-wrap: normal'), 'no overflow-wrap: normal on .y-kpi__value');
  // The big asymmetry figures are this console's own, and set no wrapping rule
  // at all, which is the correct default.
  const asym = style.match(/\.asym__n\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.ok(asym.includes('tabular-nums'), '.asym__n must use tabular figures');
  assert.ok(!/anywhere|break-all/.test(asym), '.asym__n must not be breakable');
});

/* ------------------------------------------------------ wide content */

test('wide content scrolls inside its own box, never the page', () => {
  // A horizontal scrollbar on the document is how one table takes a whole
  // layout with it — found on this console at phone width, by comparing
  // documentElement.scrollWidth against clientWidth.
  assert.ok(style.includes('.y-scroll { overflow-x: auto; }'), 'no scroll utility');
  assert.ok(style.includes('.y-scrollx { overflow-x: auto; }'), 'no shared scroll utility');
  assert.ok(page.includes('class="y-scroll"'),
    'nothing on the page uses it, which is how a table escapes its box');
});
