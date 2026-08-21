// The page stores nothing, asserted rather than intended.
//
// The custodian is told to open their link in a private window. A web page cannot
// make that happen — it is a browser security boundary — so the only honest
// response is to leave nothing behind that private mode would have to hide. That
// means no localStorage, no sessionStorage, no IndexedDB, no cookies, and not
// even a remembered theme or language.
//
// Checked against the BUILT bundle, not against the source. A dependency reaching
// for localStorage is exactly the version of this that a review of our own files
// would miss, and the built file is what a custodian's browser actually runs.
// tools/ceremony/host_test.go asserts the same thing over the embedded copy, so
// the guard survives a CI job with no npm.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const BUILT = ['ceremony.js', 'index.html'].map((name) => ({
  name,
  text: readFileSync(fileURLToPath(new URL(`../../../tools/ceremony/hosted/${name}`, import.meta.url)), 'utf8'),
}));

const FORBIDDEN = [
  'localStorage',
  'sessionStorage',
  'indexedDB',
  'document.cookie',
  'caches.open',
  'openDatabase',
  'navigator.storage',
];

test('the built bundle is present, so this suite is not passing vacuously', () => {
  for (const file of BUILT) {
    assert.ok(file.text.length > 100, `${file.name} is empty or missing — run npm run build`);
  }
  // The bundle has to actually be the ceremony rather than some other build that
  // happened to land in the directory.
  assert.ok(
    BUILT.some((file) => file.text.includes('yamale-ceremony-possession-v1')),
    'the built bundle does not contain the ceremony domain separators',
  );
});

test('nothing in the built bundle touches browser storage', () => {
  for (const file of BUILT) {
    for (const api of FORBIDDEN) {
      assert.ok(
        !file.text.includes(api),
        `${file.name} references ${api}. The page must store nothing: a custodian who forgot to open a private ` +
          'window has to be left with nothing worth finding.',
      );
    }
  }
});

// The sources as well as the bundle, because the bundle is tree-shaken.
//
// A mutation run made this necessary rather than tidy: a localStorage call added
// to a source file and never called was dropped by the bundler, so the check
// above stayed green while the source said otherwise. That particular code was
// harmless — dead code stores nothing — but a reviewer reading these files has to
// be able to trust that what they see is what runs, and the next such addition
// would be one line from being live.
test('no source file touches browser storage, comments aside', () => {
  const directory = fileURLToPath(new URL('.', import.meta.url));
  const sources = readdirSync(directory).filter((name) => name.endsWith('.ts') && !name.endsWith('.test.ts'));
  assert.ok(sources.length >= 8, `only ${sources.length} source files found, so this would pass vacuously`);
  for (const name of sources) {
    // Comments are stripped first, because several of these files name these
    // APIs in order to explain that the page does not use them. A check that
    // could not tell a promise from its breach would force the promise to go
    // undocumented, which is the wrong way round.
    const code = withoutComments(readFileSync(join(directory, name), 'utf8'));
    for (const api of FORBIDDEN) {
      assert.ok(!code.includes(api), `${name} uses ${api}`);
    }
  }
});

function withoutComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|\s)\/\/.*$/gm, '');
}

// The bundle is meant to be readable by the custodian who takes the trust note up
// on its offer. A minified build would make comparing the hash a ritual rather
// than a check, since nobody can read what they are comparing.
test('the built bundle is not minified', () => {
  const script = BUILT.find((file) => file.name === 'ceremony.js');
  assert.ok(script);
  assert.ok(script.text.includes('\n// '), 'the bundle carries no comments, so it has been minified');
  assert.ok(
    script.text.includes('possessionMessage'),
    'identifiers have been mangled, so the served code cannot be read against the source',
  );
});

// A page that could POST anywhere would make the whole client-side argument
// worthless. So every source file is enumerated — not a list written here, which
// would silently stop covering a file somebody added — and only two of them are
// allowed to make a request at all.
//
// api.ts is the ceremony's requests. trust.ts is the one exception, and it earns
// it: it fetches its own script in order to hash the code the browser is running,
// which is the check the trust note offers.
const REQUESTS_ALLOWED = new Set(['api.ts', 'trust.ts']);

test('only api.ts and trust.ts can make a request, and neither names another origin', () => {
  const directory = fileURLToPath(new URL('.', import.meta.url));
  const sources = readdirSync(directory).filter((name) => name.endsWith('.ts') && !name.endsWith('.test.ts'));
  assert.ok(sources.length >= 8, `only ${sources.length} source files found, so this would pass vacuously`);

  for (const name of sources) {
    const text = readFileSync(join(directory, name), 'utf8');
    if (!REQUESTS_ALLOWED.has(name)) {
      assert.ok(!text.includes('fetch('), `${name} makes its own request; every call belongs in api.ts`);
    }
    // Absolute origins, sockets and beacons are refused everywhere, including in
    // the two files that may fetch. A beacon in particular is a request that
    // outlives the page, which is the one shape of exfiltration that would still
    // land after the custodian closed the tab.
    assert.ok(!/(fetch|open)\(\s*['"`]https?:/.test(text), `${name} names an absolute origin`);
    assert.ok(!text.includes('XMLHttpRequest'), `${name} uses XMLHttpRequest`);
    assert.ok(!text.includes('WebSocket'), `${name} opens a socket`);
    assert.ok(!text.includes('sendBeacon'), `${name} uses sendBeacon`);
    assert.ok(!text.includes('new Image('), `${name} could exfiltrate through an image URL`);
  }
});
