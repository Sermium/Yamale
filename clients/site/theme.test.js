// The copy of the shared system that the whole site is served from.
//
// clients/site/yamale.css is not a variant. It is the file deploy/stage-site.sh
// publishes AS /yamale.css at the root, which is what every console that LINKS
// the shared system loads — keys, demo, institution, syndicate, and the five
// built apps. clients/shared/yamale.css is the canonical one nobody serves.
//
// So this copy drifting is not a local cosmetic problem: it is the whole
// deployed site rendering against a stylesheet the repository no longer has.
// That has already happened once — the host served 282 lines against a
// repository carrying 472, for weeks, answering 200 throughout, so every
// linking console rendered its .y-kpi markup with no .y-kpi rules at all.
//
// deploy/deploy.sh checks the SERVED file against this one by digest. This test
// is the other half: it checks this one against canonical, so that "the host
// matches clients/site/yamale.css" is also a statement about
// clients/shared/yamale.css rather than about two copies agreeing on a mistake.
//
// Line endings are normalised on both sides, deliberately. .gitattributes pins
// some client directories to eol=lf and not others, so on a Windows checkout
// these two files differ by 472 bytes and not one character — which is a fact
// about the checkout and not about the stylesheet, and a test that failed on it
// would be a test people learn to ignore.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (p) => fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n');

const canonical = read(path.join(here, '..', 'shared', 'yamale.css'));
const served = read(path.join(here, 'yamale.css'));

test('there is something to compare', () => {
  assert.ok(canonical.length > 5000, 'canonical stylesheet is missing or truncated');
  assert.ok(served.length > 5000, 'the served copy is missing or truncated');
});

test('the copy the site serves is the canonical one', () => {
  if (served === canonical) return;

  // A diff of two 470-line files is unreadable in an assertion message, so the
  // failure names the first line that differs and how far apart they are.
  const want = canonical.split('\n');
  const have = served.split('\n');
  let i = 0;
  while (i < want.length && i < have.length && want[i] === have[i]) i++;

  assert.fail(
    `clients/site/yamale.css has drifted from clients/shared/yamale.css.\n` +
    `  first difference at line ${i + 1}\n` +
    `    canonical: ${JSON.stringify(want[i] ?? '(end of file)')}\n` +
    `    served:    ${JSON.stringify(have[i] ?? '(end of file)')}\n` +
    `  canonical is ${want.length} lines, the served copy is ${have.length}.\n` +
    `  This file is what the deployed site loads as /yamale.css. Copy the\n` +
    `  canonical one over it rather than editing either by hand.`,
  );
});

test('the site links the copy beside it, not one it does not ship', () => {
  // A relative href resolves to this directory, which stage-site.sh flattens to
  // the root. An href of ../shared/yamale.css would resolve to /shared/... on
  // the host, where nothing is published — and a 404 stylesheet renders as an
  // unstyled page, not as an error anybody sees in a log.
  const page = fs.readFileSync(path.join(here, 'index.html'), 'utf8');
  const hrefs = [...page.matchAll(/<link[^>]+href="([^"]*yamale\.css[^"]*)"/g)]
    .map((m) => m[1]);

  assert.ok(hrefs.length > 0, 'the site does not link yamale.css at all');
  for (const href of hrefs) {
    assert.ok(!href.includes('..'),
      `index.html links ${href}; the deployed site has no directory above the root`);
  }
});
