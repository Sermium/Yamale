/**
 * The unit, printed once.
 *
 * `<Amount>` ends in the unit — that is the whole reason it exists rather than
 * a call to formatAmount. Putting a `<Symbol>` after one therefore prints it
 * twice, and the page reads "15,000,000 USD USD".
 *
 * That has now shipped twice: first as "100,000 SRMTSRMT" on the holdings
 * table, then again as "15,000,000 USD USD" in the vault panel, found by
 * looking at the page rendered against devnet-2. Neither was visible in a
 * diff — both lines are individually correct, and it is the pair that is
 * wrong — so this is a test about the source rather than about a value.
 *
 * `<Symbol>` on its own is fine and is used deliberately: as the answer to
 * "paid in what?", and as the unit on a per-token rate whose figure carries no
 * symbol of its own. Only the adjacency is refused.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));

const sources = fs.readdirSync(here)
  .filter((f) => f.endsWith('.tsx'))
  .map((f) => ({ file: f, text: fs.readFileSync(path.join(here, f), 'utf8') }));

test('there are .tsx files to check at all', () => {
  assert.ok(sources.length > 0, 'no components found; this test would pass vacuously');
});

test('no <Symbol> immediately follows an <Amount>', () => {
  // Whitespace, a JSX space expression, or both may sit between them; none of
  // those separate the two units in the rendered output.
  const adjacent = /<Amount\b[^>]*\/>\s*(?:\{' '\}\s*)?<Symbol\b/;
  const offenders = sources
    .filter((s) => adjacent.test(s.text))
    .map((s) => s.file);
  assert.deepEqual(offenders, [],
    'these print the unit twice: <Amount> already ends in it');
});

test('no <Symbol> immediately precedes an <Amount>', () => {
  // The same fault the other way round, which reads "USD 15,000,000 USD".
  const adjacent = /<Symbol\b[^>]*\/>\s*(?:\{' '\}\s*)?<Amount\b/;
  const offenders = sources
    .filter((s) => adjacent.test(s.text))
    .map((s) => s.file);
  assert.deepEqual(offenders, [],
    'these print the unit twice: <Amount> already ends in it');
});

test('the check would actually catch the bug it was written for', () => {
  // A source-level assertion that matches nothing is indistinguishable from a
  // correct codebase, so the pattern is exercised against the two shapes the
  // mistake has actually taken.
  const adjacent = /<Amount\b[^>]*\/>\s*(?:\{' '\}\s*)?<Symbol\b/;
  assert.ok(adjacent.test(
    `<Amount amount={fundedTotal} denom={vault.denom} />{' '}\n<Symbol denom={vault.denom} />`),
    'the SRMTSRMT/USD USD shape must match');
  assert.ok(adjacent.test('<Amount amount={x} denom={d} /> <Symbol denom={d} />'));
  assert.ok(!adjacent.test('<Amount amount={x} denom={d} />\n</Field>\n<Symbol denom={d} />'),
    'a Symbol in a different field is not the fault');
});
