import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { test } from 'node:test';

import {
  METADATA_HASH_BYTES,
  METADATA_SALT_BYTES,
  metadataHash,
  newPaymentMetadata,
  verifyMetadata,
} from './metadata.ts';
import { payment } from './signing.ts';

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

// The hash the browser computes has to be the hash the chain recorded. This
// pins the encoding rather than trusting that two protobuf implementations
// happen to agree: the digest is recomputed here from the wire bytes the
// generated encoder produced, so a change in field order or a dropped default
// shows up as a failure instead of as an unverifiable payment months later.
test('the hash is SHA-256 over the protobuf encoding of the payload', async () => {
  const payload = {
    salt: new Uint8Array(METADATA_SALT_BYTES).fill(7),
    purposeCode: 'SALA',
    remittanceInformation: 'March salary',
  };

  // Field 1 (salt): tag 0x0a, length 32. Field 2 (purpose_code): tag 0x12,
  // length 4. Field 3 (remittance_information): tag 0x1a, length 12.
  const expectedWire = Buffer.concat([
    Buffer.from([0x0a, 0x20]),
    Buffer.alloc(32, 7),
    Buffer.from([0x12, 0x04]),
    Buffer.from('SALA', 'utf8'),
    Buffer.from([0x1a, 0x0c]),
    Buffer.from('March salary', 'utf8'),
  ]);
  const expected = new Uint8Array(createHash('sha256').update(expectedWire).digest());

  assert.deepEqual(await metadataHash(payload), expected);
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
