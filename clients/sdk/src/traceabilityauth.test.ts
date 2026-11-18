import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { fromHex, toHex } from './jewel.ts';
import { traceabilitySignBytes, sha256Hex, signedHeaders } from './traceabilityauth.ts';

const vectors = JSON.parse(readFileSync(
  fileURLToPath(new URL('../../../tools/traceabilityapi/testdata/vectors.json', import.meta.url)), 'utf8'));

test('signed requests match traceability-api byte for byte', () => {
  for (const v of vectors) {
    const bytes = traceabilitySignBytes(v.purpose, v.chain_id, v.subject, v.issued);
    assert.equal(new TextDecoder().decode(bytes), v.sign_bytes);
    const h = signedHeaders(fromHex(v.priv_key), v.purpose, v.chain_id, v.subject, v.issued);
    assert.equal(toHex(Uint8Array.from(atob(h['X-Yamale-Key']!), (c) => c.charCodeAt(0))), v.pub_key);
    assert.equal(toHex(Uint8Array.from(atob(h['X-Yamale-Signature']!), (c) => c.charCodeAt(0))), v.signature);
    assert.equal(h['X-Yamale-Issued'], String(v.issued));
  }
});

test('the fingerprint is lowercase hex SHA-256', () => {
  assert.equal(sha256Hex(new TextEncoder().encode('abc')),
    'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
});
