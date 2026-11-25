import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  cardClaimSignBytes, cardLink, compressedPublicKey, fileJewelClaim, fromHex, parseCardFragment, STAFF_MESSAGES, toHex,
  verifyArbitrary,
} from './jewel.ts';

const key = new Uint8Array(32).fill(7);
const pubHex = toHex(compressedPublicKey(key));
const text = (b: Uint8Array) => new TextDecoder().decode(b);

test('a card link carries the jewel and the secret in the fragment, and parses back', () => {
  const link = cardLink('https://traceability.yamaletech.com/', '42', key);
  assert.equal(link, `https://traceability.yamaletech.com/card#42.${toHex(key)}`);
  const parsed = parseCardFragment(new URL(link).hash);
  assert.ok(parsed);
  assert.equal(parsed.jewelId, '42');
  assert.deepEqual(parsed.privateKey, key);
});

test('malformed card fragments are refused rather than thrown on', () => {
  for (const bad of ['', '#', '#42', '#42.', `#0.${toHex(key)}`, `#042.${toHex(key)}`, `#42.${toHex(key).slice(2)}`,
    `#42.${'0'.repeat(64)}`, `#42.${'f'.repeat(64)}`, `#4x.${toHex(key)}`, `#42.${toHex(key)}00`]) {
    assert.equal(parseCardFragment(bad), null, bad);
  }
  assert.ok(parseCardFragment(`42.${toHex(key).toUpperCase()}`));
});

test('the card signs the chain, the jewel, its own key and the claimer', () => {
  assert.equal(text(cardClaimSignBytes('yamale-devnet-2', '7', pubHex, 'yml1buyer')),
    `yamale-jewel-card/v1:yamale-devnet-2:7:${pubHex}:yml1buyer`);
  assert.throws(() => cardClaimSignBytes('c', '0', pubHex, 'yml1buyer'));
  assert.throws(() => cardClaimSignBytes('c', '7', pubHex.toUpperCase(), 'yml1buyer'));
});

test('a filed claim verifies for its claimer only', () => {
  const m = fileJewelClaim('yml1buyer', '7', key, 'yamale-devnet-2');
  assert.equal(m.typeUrl, '/blockchain.jewel.v1.MsgFileClaim');
  const v = m.value as { claimer: string; jewelId: string; cardPubKey: Uint8Array; cardSignature: Uint8Array };
  assert.equal(toHex(v.cardPubKey), pubHex);
  assert.ok(verifyArbitrary(v.cardPubKey, cardClaimSignBytes('yamale-devnet-2', '7', pubHex, 'yml1buyer'), v.cardSignature));
  assert.ok(!verifyArbitrary(v.cardPubKey, cardClaimSignBytes('yamale-devnet-2', '7', pubHex, 'yml1thief'), v.cardSignature));
});

test('staff fees cover issuing cards and deciding claims', () => {
  for (const m of ['MsgIssueCard', 'MsgApproveClaim', 'MsgRejectClaim']) {
    assert.ok(STAFF_MESSAGES.includes(`/blockchain.jewel.v1.${m}`), m);
  }
});

const vectors = JSON.parse(readFileSync(
  fileURLToPath(new URL('../../../x/jewel/types/testdata/vectors.json', import.meta.url)), 'utf8'));

test('card sign bytes and signatures match the chain\'s vectors', () => {
  assert.ok(Array.isArray(vectors.cards) && vectors.cards.length > 0);
  for (const v of vectors.cards) {
    const priv = fromHex(v.priv_key);
    assert.equal(toHex(compressedPublicKey(priv)), v.card_key);
    assert.equal(text(cardClaimSignBytes(v.chain_id, v.jewel_id, v.card_key, v.claimer)), v.sign_bytes);
    const m = fileJewelClaim(v.claimer, v.jewel_id, priv, v.chain_id).value as { cardSignature: Uint8Array };
    assert.equal(toHex(m.cardSignature), v.signature);
    // The page parses the same jewel id the chain signs for, up to the largest uint64.
    assert.equal(parseCardFragment(`#${v.jewel_id}.${v.priv_key}`)?.jewelId, v.jewel_id);
  }
});
