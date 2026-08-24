import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { classifySearch, matchDenom, matchValidator } from './search.ts';
import { KNOWN_DENOMS, type DenomInfo } from '../../sdk/src/denom.ts';

const ADDRESS = 'yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg';
const POLICY = 'yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr';
const HASH = 'D16E0689BB330AA35CF1AD7CA8BAEA10A46B19CFDF02FB6A6F93212FE001E45F';

/** The two validators yamale-devnet-2 is actually running. */
const MONIKERS = {
  ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh: 'pi',
  ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j: 'pi-2',
};

test('the four kinds decidable from the string alone need no round trip', () => {
  assert.deepEqual(classifySearch('34928'), { kind: 'height', value: '34928', label: 'block' });
  assert.equal(classifySearch(HASH).kind, 'tx');
  assert.equal(classifySearch(ADDRESS).kind, 'address');
  assert.equal(
    classifySearch('ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh').kind,
    'validator',
  );
});

test('a hash is normalised so a pasted lowercase one still resolves', () => {
  // The REST endpoint is case-sensitive on the hash. A hash copied out of a log
  // is often lowercase, and the old box sent it through unchanged.
  assert.equal(classifySearch(HASH.toLowerCase()).value, HASH);
});

test('a module address is an account, not something unrecognised', () => {
  // 63-character bech32: a group policy account, which is what holds the money
  // on every shared-account row in the feed.
  assert.equal(classifySearch(POLICY).kind, 'address');
});

test('a user ID is recognised, in any of the forms people write it', () => {
  // The identifier a citizen has actually seen. The old search rejected all of
  // these and told them it did not look like an account.
  for (const written of ['NG-K3M9-7QRT-5', 'ngk3m97qrt5', 'NG K3M9 7QRT 5']) {
    const guess = classifySearch(written);
    assert.equal(guess.kind, 'userId', `${written} should be a user ID`);
    assert.equal(guess.value, 'NGK3M97QRT5', 'normalised for the lookup');
  }
});

test('a mistyped user ID is caught here, not by the node', () => {
  // Its own check character says so. Sending it to the chain would come back
  // "not found", which a person reads as "that account does not exist" rather
  // than "you typed it wrong".
  assert.equal(classifySearch('NG-K3M9-7QRT-X').kind, 'unknown');
});

test('a foundation ID is a user ID even though ZZ is not a country', () => {
  // ISO 3166-1 reserves ZZ permanently, and the foundation administrators carry
  // it. A country check that used the assigned list alone would reject the one
  // set of accounts most likely to be looked up by an auditor.
  assert.equal(classifySearch('ZZ-K3M9-7QRT-J').kind, 'userId');
});

test('an unassigned country prefix is not a user ID', () => {
  // NX is not a country, so the chain would never issue this. Accepting it
  // would let a search present a fabricated perimeter as a real one.
  assert.equal(classifySearch('NX-K3M9-7QRT-5').kind, 'unknown');
});

test('a currency is found by the code people write, not only the one stored', () => {
  const guess = classifySearch('NGN', { registry: KNOWN_DENOMS });
  assert.equal(guess.kind, 'denom');
  assert.equal(guess.value, 'ungn', 'resolved to the base unit the chain stores');

  assert.equal(classifySearch('ungn', { registry: KNOWN_DENOMS }).value, 'ungn');
  assert.equal(classifySearch('yml', { registry: KNOWN_DENOMS }).value, 'uyml');
});

test('a currency registered after launch is findable without a client release', () => {
  // x/stablecoin publishes denom metadata when governance approves an issuer.
  const registry: Record<string, DenomInfo> = {
    ...KNOWN_DENOMS,
    'factory/new': { base: 'factory/new', symbol: 'NEWC', exponent: 6, name: 'New Coin' },
  };
  assert.equal(classifySearch('newc', { registry }).value, 'factory/new');
});

test('a validator is found by its moniker', () => {
  const guess = classifySearch('pi-2', { monikers: MONIKERS });
  assert.equal(guess.kind, 'validator');
  assert.equal(guess.value, 'ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j');
});

test('an exact moniker wins over one it is a prefix of', () => {
  // "pi" and "pi-2" both exist on this chain. Prefix matching alone would make
  // the shorter name unreachable, or ambiguous, depending on the order.
  assert.equal(
    classifySearch('pi', { monikers: MONIKERS }).value,
    'ymlvaloper1m9xhc6zy7fxfax9t5fnykh9k2e29faj7p4h3kh',
  );
});

test('an ambiguous name resolves to nothing rather than to a guess', () => {
  // Sending somebody to the wrong validator's page is worse than telling them
  // the name was not specific enough.
  const many = { a: 'Bank One', b: 'Bank Two' };
  assert.equal(matchValidator('Bank', many), null);
  assert.equal(classifySearch('Bank', { monikers: many }).kind, 'unknown');
});

test('classification is ordered so a short currency code cannot shadow anything', () => {
  // "1000" is a height, not a denom, whatever a registry says.
  const registry: Record<string, DenomInfo> = {
    '1000': { base: '1000', symbol: '1000', exponent: 0, name: 'Silly' },
  };
  assert.equal(classifySearch('1000', { registry }).kind, 'height');
});

test('an empty box is empty, not unknown', () => {
  // The distinction drives whether the page says anything at all.
  assert.equal(classifySearch('').kind, 'empty');
  assert.equal(classifySearch('   ').kind, 'empty');
});

test('without the chain lists, the string-only kinds still work', () => {
  assert.equal(classifySearch(ADDRESS, {}).kind, 'address');
  assert.equal(matchDenom('NGN', undefined), null);
  assert.equal(matchValidator('pi', undefined), null);
});

test('surrounding whitespace from a copy-paste is trimmed', () => {
  assert.equal(classifySearch(`  ${ADDRESS}  `).kind, 'address');
  assert.equal(classifySearch(' 34928 ').value, '34928');
});
