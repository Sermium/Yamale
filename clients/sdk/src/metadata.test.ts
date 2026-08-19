import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { PaymentMetadata } from './generated/blockchain/paymsg/v1/payment_metadata.ts';
import {
  METADATA_HASH_BYTES,
  METADATA_SALT_BYTES,
  metadataHash,
  newPaymentMetadata,
  verifyMetadata,
} from './metadata.ts';
import { payment } from './signing.ts';

/** The file x/paymsg/types reads too. See the vector test below. */
const vectors = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../testdata/vectors/confidentiality.json', import.meta.url)), 'utf8'),
) as {
  payment_metadata: Array<{
    name: string;
    salt_hex: string;
    purpose_code: string;
    remittance_information: string;
    wire_hex: string;
    hash_hex: string;
  }>;
};

function fromHex(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i += 1) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

test('a payload round-trips and its hash verifies', async () => {
  const payload = newPaymentMetadata('SALA', 'March salary, employee 4417');
  assert.equal(payload.salt.length, METADATA_SALT_BYTES);

  const hash = await metadataHash(payload);
  assert.equal(hash.length, METADATA_HASH_BYTES);
  assert.equal(await verifyMetadata(payload, hash), true);

  // The same payload hashes the same way every time, which is the whole basis
  // of the check.
  assert.deepEqual(await metadataHash(payload), hash);
});

test('an altered payload fails against the recorded hash', async () => {
  const payload = newPaymentMetadata('SALA', 'March salary, employee 4417');
  const hash = await metadataHash(payload);

  assert.equal(
    await verifyMetadata({ ...payload, remittanceInformation: 'March salary, employee 4418' }, hash),
    false,
  );
  assert.equal(await verifyMetadata({ ...payload, purposeCode: 'SUPP' }, hash), false);
  assert.equal(await verifyMetadata({ ...payload, remittanceInformation: '' }, hash), false);

  // A different salt is a different payload even when every readable field
  // matches, which stops one payment's payload being offered as another's.
  const other = newPaymentMetadata('SALA', 'March salary, employee 4417');
  assert.equal(await verifyMetadata({ ...payload, salt: other.salt }, hash), false);
});

test('two payments carrying the same detail do not hash alike', async () => {
  const first = await metadataHash(newPaymentMetadata('SALA', 'March salary'));
  const second = await metadataHash(newPaymentMetadata('SALA', 'March salary'));
  assert.notDeepEqual(first, second);
});

test('a payload with no salt is refused rather than hashed', async () => {
  await assert.rejects(
    () => metadataHash({ salt: new Uint8Array(4), purposeCode: 'SALA', remittanceInformation: '' }),
    /salt must be 32 bytes/,
  );
});

test('a recorded hash of the wrong length never verifies', async () => {
  const payload = newPaymentMetadata('SALA', 'March salary');
  const hash = await metadataHash(payload);
  assert.equal(await verifyMetadata(payload, hash.slice(0, 16)), false);
});

// The hash the browser computes has to be the hash the chain recorded, and the
// protobuf encoding underneath it has to be the same encoding.
//
// Both are read from testdata/vectors/confidentiality.json, which the Go suite
// reads too. This test used to rebuild the expected wire bytes by hand while
// the Go side pinned a hex constant: the two agreed, but neither could make the
// other fail, so the drift they were meant to catch would have gone through
// both of them. One file makes the contract real.
//
// The wire bytes are asserted as well as the digest. A digest alone says the
// two disagree; the encoding says where — a reordered field, a default that
// stopped being omitted.
test('payment metadata hashing matches the shared cross-language vectors', async () => {
  assert.ok(vectors.payment_metadata.length > 0, 'the vectors are empty, so this test would pass vacuously');

  for (const v of vectors.payment_metadata) {
    const payload = {
      salt: fromHex(v.salt_hex),
      purposeCode: v.purpose_code,
      remittanceInformation: v.remittance_information,
    };

    const wire = Uint8Array.from(PaymentMetadata.encode(payload).finish());
    assert.equal(toHex(wire), v.wire_hex, `${v.name}: the protobuf encoding of the payload has changed`);
    assert.equal(
      toHex(new Uint8Array(createHash('sha256').update(wire).digest())),
      v.hash_hex,
      `${v.name}: the digest does not match the encoding beside it`,
    );

    assert.equal(toHex(await metadataHash(payload)), v.hash_hex, v.name);
    assert.equal(await verifyMetadata(payload, fromHex(v.hash_hex)), true, v.name);
  }
});

test('a payment refuses to carry the hash and the plaintext together', async () => {
  const hash = await metadataHash(newPaymentMetadata('SALA', 'March salary'));

  assert.throws(
    () =>
      payment({
        debtor: 'yml1debtor',
        endToEndId: 'E2E-1',
        instructingParticipant: 'yml1one',
        instructedParticipant: 'yml1two',
        creditor: 'yml1creditor',
        denom: 'uyml',
        amount: '1000',
        purposeCode: 'SALA',
        metadataHash: hash,
      }),
    /never both/,
  );
});

test('a payment refuses a settlement jurisdiction that is not alpha-2', () => {
  const base = {
    debtor: 'yml1debtor',
    endToEndId: 'E2E-2',
    instructingParticipant: 'yml1one',
    instructedParticipant: 'yml1two',
    creditor: 'yml1creditor',
    denom: 'uyml',
    amount: '1000',
  };

  const jurisdictionOf = (msg: ReturnType<typeof payment>): string =>
    (msg.value as { settlementJurisdiction: string }).settlementJurisdiction;

  assert.equal(jurisdictionOf(payment({ ...base, settlementJurisdiction: 'NG' })), 'NG');
  // Optional today, so an omitted one is not an error here either.
  assert.equal(jurisdictionOf(payment(base)), '');

  for (const bad of ['nga', 'ng', 'NGA', 'N', 'N1']) {
    assert.throws(() => payment({ ...base, settlementJurisdiction: bad }), /alpha-2/, `accepted ${bad}`);
  }
});
