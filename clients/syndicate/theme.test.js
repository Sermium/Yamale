// The two things about this console's stylesheet that cannot be checked by
// looking at it. Adapted from clients/land/theme.test.js, which is where the
// reasoning was written down first; it is repeated here rather than imported
// because a console whose guard lives in another console's directory is a
// console somebody deletes the guard of by accident.
//
// 1. THE INLINED COPY OF THE SHARED SYSTEM.
//
// This console is deliberately one file with no build step — it is opened by an
// investor deciding whether to commit, from a phone, and a page that needs a
// second request before it can say whether a raise has covered its minimum
// renders as unstyled text at the moment it matters. Linking
// ../shared/yamale.css is also not available in practice: the deployment serves
// this page from /syndicate/ and /shared/yamale.css answers 404 there.
//
// So the shared system is pasted in, and a pasted copy drifts. It had:
// 185 meaningful lines behind canonical when this test was written, including
// the whole INFORMATION DESIGN block — the KPI band, the folders, the data
// tables, the ring — and two palette tokens under names the shared file had
// since stopped using. None of that is visible in the file; all of it is
// visible here.
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

test('the information-design patterns arrived with it, not just the palette', () => {
  // The drift this file was in was not a colour being slightly off. It was 185
  // lines, and every one of them was layout: the KPI band, the folders, the
  // data tables, the ring. A future paste that brought the tokens and stopped
  // would pass the test above only if it also brought these, so this names them
  // — the failure message is then the list of patterns rather than a line of
  // CSS nobody recognises.
  for (const cls of ['.y-kpis', '.y-kpi__value', '.y-kpi__hint', '.y-folders', '.y-folder',
    '.y-folder__count', '.y-data', '.y-scrollx', '.y-ring', '.y-split', '.y-swatch']) {
    assert.ok(style.includes(cls), `${cls} is missing from the inlined copy`);
  }
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

test('body paints its own ground', () => {
  // A transparent body borrows whatever the host paints behind it, which on a
  // preview or artifact host is the wrong theme's colour.
  assert.ok(/body\s*\{[^}]*background:\s*var\(--bg\)/.test(style),
    'body must set an explicit background from a token');
});

/* ---------------------------------------------- what carries state to a reader */

test('no state is signalled by colour alone', () => {
  // One man in twelve cannot separate this page's green from its amber. Every
  // chip class is therefore paired with a word in the markup — `standing()`
  // returns a `label` beside every `tone` — and the chip itself is mono and
  // uppercase, a different shape rather than only a different hue.
  for (const cls of ['.p-ok', '.p-warn', '.p-bad', '.p-mute', '.p-brass']) {
    assert.ok(style.includes(cls), `${cls} is missing`);
  }
  assert.ok(/\.pill[^{]*\{[^}]*text-transform:\s*uppercase/.test(style),
    'chips must be distinguishable by form, not only by colour');
});

test('wide content scrolls inside its own box, never the page', () => {
  // A horizontal scrollbar on the document is how one table takes a whole
  // layout with it. The book of a fully subscribed deal is four columns of
  // money on a phone, which is exactly that table.
  assert.ok(/pre\s*\{[^}]*overflow-x:\s*auto/.test(style), 'pre must scroll itself');
  assert.ok(style.includes('.y-scrollx { overflow-x: auto; }'), 'no scroll utility');
  assert.ok(!/\.scroll\s*\{/.test(style),
    'the console had its own .scroll saying the same thing as .y-scrollx — '
    + 'two rules for one decision is how the next person fixes the wrong one');
});

/* ------------------------------------------------ a figure is one unbroken run */

test('nothing lets a number break across two lines', () => {
  // `overflow-wrap: anywhere` has printed "100,000,0" above "00" in this
  // repository twice. A number split in two is not a smaller number, it is a
  // different one — and on this page the numbers are cheques.
  //
  // The digits themselves are joined by U+202F, a narrow NO-BREAK space, in
  // syndicate.js's group(); this asserts that no rule reaching a figure undoes
  // that. `.digest` is exempt and named: a SHA-256 is 64 hex characters that
  // MUST wrap, and breaking one does not change what it says.
  const rules = style.split('}');
  const offenders = rules
    .filter((r) => /overflow-wrap:\s*anywhere|word-break:\s*break-all/.test(r))
    .map((r) => r.split('{')[0].trim())
    .filter((sel) => !/\.digest|\.y-addr|\.addr/.test(sel));
  assert.deepEqual(offenders, [],
    'these rules can break a run of digits across a line');

  // And the KPI value, which is where the figures a reader decides on are set,
  // says so itself rather than relying on the default.
  const kpi = style.match(/\.y-kpi__value\s*\{([^}]*)\}/)?.[1] ?? '';
  assert.match(kpi, /word-break:\s*keep-all/, 'the KPI value must not break its digits');
  assert.match(kpi, /overflow-wrap:\s*normal/, 'the KPI value must not break its digits');
});

/* ------------------------------------------------------ the folders are places */

test('a folder is a link, so it can be sent to somebody', () => {
  // The deal's areas are anchors carrying `#/deal/<id>/<folder>`, not buttons:
  // somebody sending a colleague to the cap table should be able to send them
  // to the cap table. The shared `.y-folder` is written for a <button>, which
  // is inline-block and has no underline; an anchor is neither, so the console
  // states the difference rather than inheriting a link that looks like prose.
  assert.ok(/a\.y-folder\s*\{[^}]*text-decoration:\s*none/.test(style),
    'folder links must not be underlined like body copy');
  assert.match(page, /href="#\/deal\/\$\{esc\(id\)\}\/\$\{esc\(t\.id\)\}"/,
    'the folder tabs must put the folder in the URL');
});
