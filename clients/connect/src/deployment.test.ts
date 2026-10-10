// Run with: node --test src/deployment.test.ts
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { walletUrlFor, appOriginsFor, APP_HOSTS } from './deployment.ts';

test('on yamaletech.com every app opens the ONE wallet origin', () => {
  for (const host of ['explorer.yamaletech.com', 'rwa.yamaletech.com', 'pay.yamaletech.com']) {
    assert.equal(walletUrlFor(`https://${host}`), 'https://wallet.yamaletech.com/wallet');
  }
});

// The wallet's own subdomain opens itself, not a second wallet somewhere else.
test('the wallet subdomain is its own wallet', () => {
  assert.equal(walletUrlFor('https://wallet.yamaletech.com'), 'https://wallet.yamaletech.com/wallet');
});

// The old hosts serve every app under one origin, and accounts created there
// live in that origin's storage. Pointing them at the new subdomain would show
// their owners an empty wallet.
test('a host that is not yamaletech.com keeps its same-origin wallet', () => {
  assert.equal(walletUrlFor('https://pay.yamalelegal.com'), 'https://pay.yamalelegal.com/wallet');
  assert.equal(walletUrlFor('https://yamale.tail4355e8.ts.net'), 'https://yamale.tail4355e8.ts.net/wallet');
  assert.equal(walletUrlFor('http://localhost:5173'), 'http://localhost:5173/wallet');
});

// A look-alike must not be treated as ours: the suffix check is on a dot
// boundary, or evil-yamaletech.com would be sent to our wallet as an app.
test('a look-alike domain is not yamaletech.com', () => {
  assert.equal(walletUrlFor('https://evilyamaletech.com'), 'https://evilyamaletech.com/wallet');
  assert.equal(walletUrlFor('https://yamaletech.com.attacker.io'), 'https://yamaletech.com.attacker.io/wallet');
});

test('the wallet subdomain accepts exactly the app subdomains', () => {
  const allowed = appOriginsFor('https://wallet.yamaletech.com');
  assert.ok(Array.isArray(allowed));
  assert.ok(allowed.includes('https://explorer.yamaletech.com'));
  assert.ok(allowed.includes('https://rwa.yamaletech.com'));
  assert.ok(!allowed.includes('https://evilyamaletech.com'));
  assert.equal(allowed.length, APP_HOSTS.length);
  for (const origin of allowed) assert.match(origin, /^https:\/\/[a-z]+\.yamaletech\.com$/);
});

// On an old host the apps are on the wallet's own origin, so that origin is
// the whole list. Never '*' outside development.
test('an old host accepts only itself', () => {
  assert.deepEqual(appOriginsFor('https://pay.yamalelegal.com'), ['https://pay.yamalelegal.com']);
});
