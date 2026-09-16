// The things about this console's stylesheet that cannot be checked by looking
// at it. Adapted from clients/land/theme.test.js, which is the reference copy;
// the reasoning below is that file's, kept here because this console is read on
// its own.
//
// 1. THE INLINED COPY OF THE SHARED SYSTEM.
//
// This console is deliberately one file with no build step — it is opened by an
// onboarding officer with a request in front of them, often from a phone, and a
// page that needs a second request before it can say "this account is already
// recorded in SN" renders as unstyled text at the moment it matters. Linking
// ../shared/yamale.css is also not available in practice: the deployment serves
// this page from /institution/ and /shared/yamale.css answers 404 there.
//
// So the shared system is pasted in, and a pasted copy drifts. This copy drifted
// 183 meaningful lines — the whole INFORMATION DESIGN block was missing, which
// is why this console had no KPIs, no folders and no data tables while every
// other one did. This test is what makes "up to date with
// clients/shared/yamale.css" a fact rather than a claim in a comment.
//
// 2. THE THREE-STATE THEME RULE.
//
// The theme has three states, not two: an explicit `data-theme`, and the
// default un-stamped state where only `prefers-color-scheme` separates light
// from dark. A colour whose ONLY definition sits inside a media query or a
// `[data-theme]` block therefore never applies in one of them, and the symptom
// is one theme's text on the other theme's ground. That bug has shipped in this
// repository once already, which is why it is a test and not a note.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const page = fs.readFileSync(path.join(here, 'index.html'), 'utf8');
const shared = fs.readFileSync(path.join(here, '..', 'shared', 'yamale.css'), 'utf8');

const style = page.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

/** Comments and blank lines out; everything that renders, in order. */
const meaningful = (css) => css
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

test('the typefaces are actually requested', () => {
  // The stylesheet names Archivo and IBM Plex; naming them does nothing unless
  // the page asks for them. A console that forgot this link rendered in
  // whatever face the reader's operating system happened to install, and looked
  // identical to one whose font host was blocked — which is exactly the bug
  // that was live in clients/keys.
  assert.ok(/<link rel="preconnect" href="https:\/\/fonts\.googleapis\.com">/.test(page),
    'no preconnect to the font host');
  assert.ok(/<link rel="preconnect" href="https:\/\/fonts\.gstatic\.com" crossorigin>/.test(page),
    'no preconnect to the font CDN — the stylesheet resolves, the font files then wait');
  const sheet = page.match(/href="(https:\/\/fonts\.googleapis\.com\/css2\?[^"]+)"/)?.[1] ?? '';
  for (const family of ['family=Archivo', 'family=IBM+Plex+Mono', 'family=IBM+Plex+Sans']) {
    assert.ok(sheet.includes(family), `${family} is not requested`);
  }
  assert.ok(sheet.includes('display=swap'),
    'without display=swap the page is blank while the fonts load');
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

  // Depth 0 blocks opened by a bare `:root {` are the base palette; anything
  // inside an @media or a `[data-theme=…]` selector is conditional.
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
  // that defined only the second would ignore the toggle.
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
    '--ok', '--warn', '--bad', '--accent', '--accent-ink']) {
    assert.ok(tokens(style).base.has(token), `${token} is not defined at bare :root`);
  }
});

test('every token this console spends is a token the palette defines', () => {
  // The rule this catches had shipped: the paste box asked for
  // `var(--radius-1)`, which no palette has ever defined, so the field rendered
  // with square corners and nothing in the file said why. A typo in a token
  // name fails silently — that is the whole reason it needs a test.
  const { base } = tokens(style);
  const spent = new Set([...style.matchAll(/var\((--[a-z0-9-]+)/gi)].map((m) => m[1]));
  const undefinedTokens = [...spent].filter((t) => !base.has(t));
  assert.deepEqual(undefinedTokens, [],
    'these tokens are used but never defined at :root — the declaration is dropped');
});

test('body paints its own ground', () => {
  // A transparent body borrows whatever the host paints behind it, which on a
  // preview or artifact host is the wrong theme's colour.
  assert.ok(/body\s*\{[^}]*background:\s*var\(--bg\)/.test(style),
    'body must set an explicit background from a token');
});

/* ---------------------------------------------- what carries state to a reader */

test('no state is signalled by colour alone', () => {
  // One man in twelve cannot separate this page's green from its amber. Every
  // chip class is therefore paired with a word in the markup, and the chip
  // itself is mono and uppercase — a different shape, not only a different
  // hue. This asserts the classes exist as a set; the words beside them are
  // asserted by the console's own copy, which is prose.
  for (const cls of ['.p-ok', '.p-warn', '.p-bad', '.p-mute', '.p-brass']) {
    assert.ok(style.includes(cls), `${cls} is missing`);
  }
  assert.ok(/\.pill[^{]*\{[^}]*text-transform:\s*uppercase/.test(style),
    'chips must be distinguishable by form, not only by colour');
});

test('wide content scrolls inside its own box, never the page', () => {
  // A horizontal scrollbar on the document is how one table takes a whole
  // layout with it.
  assert.ok(/pre\s*\{[^}]*overflow-x:\s*auto/.test(style), 'pre must scroll itself');
  assert.ok(style.includes('.y-scrollx { overflow-x: auto; }'), 'no scroll utility');
  assert.ok(/class="y-scrollx"/.test(page), 'the tables are not wrapped in it');
});

test('no figure is allowed to break across two lines', () => {
  // `overflow-wrap: anywhere` has printed "100,000,0" above "00" in this
  // codebase twice. A number split in two is not a smaller number, it is a
  // different one, so the property is banned from this file outright — the
  // places that genuinely need to break a long string (an address, a hash) say
  // `word-break: break-all` on `.y-addr`, which is not a figure.
  assert.ok(!/overflow-wrap:\s*anywhere/.test(style),
    'overflow-wrap: anywhere is how a number gets cut in half');
  assert.ok(/\.y-kpi__value\s*\{[^}]*overflow-wrap:\s*normal/.test(style),
    'the KPI figure must refuse to wrap');
});

/* ----------------------------------- the imported system is actually used */

test('the information design that was imported is on the page', () => {
  // Pasting the canonical file and then not using it is the state this console
  // was already in for everything else: it had the chrome and none of the
  // shape. These assert the markup, not the stylesheet.
  assert.ok(/class="y-kpis"/.test(page), 'no KPI band');
  assert.ok(/y-kpi__value/.test(page), 'the KPI band has no figures');
  assert.ok(/class="y-folders"/.test(page), 'no folders — this is one long scroll again');
  assert.ok((page.match(/class="y-folder"/g) ?? []).length >= 2,
    'a single folder is not a folder');
  assert.ok(/class="y-data"/.test(page), 'no data table');
  assert.ok(/class="num y-num"/.test(page), 'no numeric cells are marked, so nothing aligns');
});

test('each folder button drives a panel that exists', () => {
  // A tab whose `aria-controls` names nothing is a button that does nothing,
  // and it looks exactly like one that works until it is pressed.
  const controls = [...page.matchAll(/aria-controls="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(controls.length >= 2, 'no tabs found');
  for (const id of controls) {
    assert.ok(page.includes(`id="${id}"`), `aria-controls="${id}" names no element`);
    assert.ok(new RegExp(`id="${id}"[^>]*role="tabpanel"`).test(page),
      `${id} is not a tabpanel`);
  }
});

test('exactly one folder is open when the page loads', () => {
  const selected = (page.match(/aria-selected="true"/g) ?? []).length;
  assert.equal(selected, 1, 'the page must open on one folder, not none and not several');
});
