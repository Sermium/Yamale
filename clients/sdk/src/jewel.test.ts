import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { chainRegistry } from './registry.ts';
import {
  addressOfPublicKey, claimJewelOffer, claimLink, claimSignBytes, compressedPublicKey,
  decodeVoucherCode, encodeVoucherCode, fromHex, makeOwnershipProof, makeVoucher,
  normaliseInscription, parseClaimFragment, signArbitrary, toHex, verifyOwnershipProof,
  voucherSignBytes, voucherSigner,
} from './jewel.ts';

const vectors = JSON.parse(readFileSync(
  fileURLToPath(new URL('../../../x/jewel/types/testdata/vectors.json', import.meta.url)), 'utf8'));
const text = (b: Uint8Array) => new TextDecoder().decode(b);

test('inscriptions normalise exactly as the chain does', () => {
  for (const v of vectors.inscriptions) {
    assert.equal(normaliseInscription(v.in) ?? '', v.out, `input ${JSON.stringify(v.in)}`);
  }
});

test('claim keys, sign bytes and signatures match the chain byte for byte', () => {
  for (const v of vectors.claims) {
    const priv = fromHex(v.priv_key);
    const pub = compressedPublicKey(priv);
    assert.equal(toHex(pub), v.pub_key);
    assert.equal(addressOfPublicKey(pub), v.address);
    const bytes = claimSignBytes(v.chain_id, v.jewel_id, v.claimer);
    assert.equal(text(bytes), v.sign_bytes);
    // Both sides sign deterministically (RFC 6979), so the bytes must be
    // identical, not merely both valid.
    assert.equal(toHex(signArbitrary(priv, bytes)), v.signature);
  }
});

test('voucher sign bytes and signatures match the chain byte for byte', () => {
  for (const v of vectors.vouchers) {
    const priv = fromHex(v.priv_key);
    const bytes = voucherSignBytes(v.chain_id,
      { benefitId: v.benefit_id, jewelId: v.jewel_id, nonce: v.nonce, expiresAt: v.expires_at });
    assert.equal(text(bytes), v.sign_bytes);
    assert.equal(toHex(signArbitrary(priv, bytes)), v.signature);
  }
});

test('a claim link carries the jewel and the key, and nothing else parses', () => {
  const priv = fromHex(vectors.claims[0].priv_key);
  const link = claimLink('https://loyalty.example', '12', priv);
  assert.equal(link, `https://loyalty.example/claim#12.${vectors.claims[0].priv_key}`);
  const parsed = parseClaimFragment(new URL(link).hash);
  assert.ok(parsed);
  assert.equal(parsed.jewelId, '12');
  assert.equal(toHex(parsed.privateKey), vectors.claims[0].priv_key);
  for (const bad of ['', '#12', '#0.' + 'a'.repeat(64), '#12.' + 'a'.repeat(63), '#x.' + 'a'.repeat(64)]) {
    assert.equal(parseClaimFragment(bad), null, bad);
  }
  // Well-formed hex that is not a valid secp256k1 scalar: zero, and the curve order itself and above.
  const order = 'fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141';
  for (const bad of ['0'.repeat(64), order, 'f'.repeat(64)]) {
    assert.equal(parseClaimFragment('#12.' + bad), null, bad);
  }
});

test('a claim message signs for the claimer named in it', () => {
  const v = vectors.claims[0];
  const msg = claimJewelOffer(v.claimer, v.jewel_id, fromHex(v.priv_key), v.chain_id);
  const decoded = chainRegistry().decode({
    typeUrl: msg.typeUrl,
    value: chainRegistry().encode(msg as never),
  }) as { claimSignature: Uint8Array; claimPubKey: Uint8Array };
  assert.equal(toHex(decoded.claimSignature), v.signature);
  assert.equal(toHex(decoded.claimPubKey), v.pub_key);
});

test('a voucher code survives the trip through text and names its signer', () => {
  const priv = fromHex(vectors.vouchers[0].priv_key);
  const code = makeVoucher(priv, 'yamale-local-1', '3', '7', 1_790_000_000, 240,
    (n) => new Uint8Array(n).fill(1));
  assert.equal(code.voucher.expiresAt, '1790000240');
  assert.equal(code.voucher.nonce, '01'.repeat(16));
  const back = decodeVoucherCode(encodeVoucherCode(code));
  assert.ok(back);
  assert.deepEqual(back.voucher, code.voucher);
  assert.equal(voucherSigner(back, 'yamale-local-1'), vectors.vouchers[0].address);
  assert.equal(voucherSigner(back, 'another-chain'), null);
  const tampered = { ...back, voucher: { ...back.voucher, expiresAt: '1790000999' } };
  assert.equal(voucherSigner(tampered, 'yamale-local-1'), null);
  assert.equal(decodeVoucherCode('YJV2.' + encodeVoucherCode(code).slice(5)), null);
  assert.equal(decodeVoucherCode('not a voucher'), null);
});

test('an ownership proof verifies for its jewel only, and not once altered', () => {
  const priv = fromHex(vectors.claims[1].priv_key);
  const query = makeOwnershipProof(priv, 'yamale-local-1', '5', 1_790_000_000);
  assert.deepEqual(verifyOwnershipProof('yamale-local-1', '5', query),
    { address: vectors.claims[1].address, issuedAt: 1_790_000_000 });
  assert.equal(verifyOwnershipProof('yamale-local-1', '6', query), null);
  assert.equal(verifyOwnershipProof('yamale-local-1', '5', query.replace('t=1790000000', 't=1790000001')), null);
});
