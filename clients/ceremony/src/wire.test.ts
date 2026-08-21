// The cross-language contract.
//
// Every value in this suite comes out of testdata/vectors/ceremony.json, which
// tools/ceremony/vectors_test.go also reads. Neither side can be made green on
// its own: mutate the fixture and both go red, which is the only arrangement in
// which the comments claiming "the browser and the binary agree" are true of the
// code rather than true by construction.
//
// What would happen without it is specific. A browser that derived even slightly
// differently would hand five custodians addresses that look right and control
// nothing, and nobody would find out until a seizure arrived at an account no
// three people could open. A browser that computed the group fingerprint over
// slightly different bytes would make the read-aloud comparison fail for
// innocent reasons — which is worse than useless, because it teaches five
// custodians to shrug at the one check that catches a hostile relay.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { deriveKey, identityOf, sign, signSubmission, verify } from './key.ts';
import {
  assembleGroup,
  assembledCanonical,
  buildGroup,
  policyAddress,
  presence,
  verifySubmission,
} from './group.ts';
import {
  attestationCanonical,
  fromBase64,
  paramsCanonical,
  paramsFingerprint,
  possessionMessage,
  toBase64,
  toHex,
  type Attestation,
  type CeremonyParams,
  type Submission,
} from './wire.ts';
import { goJSONString, parseGoDuration, protoDuration } from './gojson.ts';

type Vectors = {
  policy_derivation: { module: string; group_policy_table_prefix: number };
  policy_addresses: Array<{ seq: number; address: string }>;
  duration_vectors: Array<{ duration: string; windows_json: string }>;
  params: CeremonyParams;
  params_canonical_hex: string;
  params_fingerprint: string;
  custodians: Array<{
    name: string;
    phrase: string;
    index: number;
    hd_path: string;
    address: string;
    pubkey_base64: string;
    fingerprint: string;
    generated_at: string;
    possession_message_hex: string;
    possession_signature_base64: string;
  }>;
  group: {
    computed_at: string;
    policy_address: string;
    genesis_json: string;
    constitution_json: string;
    canonical_hex: string;
    fingerprint: string;
  };
  attestations: Array<{ attestation: Attestation; canonical_hex: string; signature_base64: string }>;
};

// Located from import.meta.url rather than the working directory, so the suite
// runs the same from clients/ceremony and from the root workspace runner.
const vectors = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../testdata/vectors/ceremony.json', import.meta.url)), 'utf8'),
) as Vectors;

// The emptiness check is the point of reading the file up here. A fixture that
// resolved to the wrong path, or one somebody emptied, would let every case
// below iterate over nothing and pass — the vacuous green a shared file exists
// to eliminate.
test('the shared vectors are present and non-empty', () => {
  assert.ok(vectors.custodians.length >= 5, 'the fixture must carry at least five custodians');
  assert.ok(vectors.policy_addresses.length > 0);
  assert.ok(vectors.duration_vectors.length > 0);
  assert.ok(vectors.attestations.length > 0);
  assert.equal(vectors.policy_derivation.module, 'group');
  assert.equal(vectors.policy_derivation.group_policy_table_prefix, 0x20);
});

test('a group policy address derives the same as the Go binary', () => {
  for (const entry of vectors.policy_addresses) {
    assert.equal(policyAddress(entry.seq), entry.address, `policy ${entry.seq}`);
  }
});

test('a phrase derives the address, public key and fingerprint the Go binary derives', () => {
  for (const custodian of vectors.custodians) {
    const key = deriveKey(custodian.phrase, custodian.index);
    assert.equal(key.path, custodian.hd_path, custodian.name);
    assert.equal(key.address, custodian.address, custodian.name);
    assert.equal(toBase64(key.pub), custodian.pubkey_base64, custodian.name);
    assert.equal(key.fingerprint, custodian.fingerprint, custodian.name);
  }
});

test('the possession message is byte-identical to the one Go signs', () => {
  for (const custodian of vectors.custodians) {
    const key = deriveKey(custodian.phrase, custodian.index);
    const id = identityOf(custodian.name, key, new Date(custodian.generated_at));
    assert.equal(id.generated_at, custodian.generated_at, custodian.name);
    assert.equal(
      toHex(possessionMessage(vectors.params.ceremony_id, id)),
      custodian.possession_message_hex,
      custodian.name,
    );
  }
});

// Byte equality of the signature, not just "it verifies".
//
// Both sides sign deterministically (RFC 6979) with a low-S normalisation, so
// the same key over the same message produces the same sixty-four bytes. Pinning
// the bytes catches a library upgrade that quietly stopped normalising S — which
// verification alone would not, because a high-S signature verifies in some
// implementations and is rejected by the chain.
test('the possession signature is byte-identical to the one Go produces', () => {
  for (const custodian of vectors.custodians) {
    const key = deriveKey(custodian.phrase, custodian.index);
    const id = identityOf(custodian.name, key, new Date(custodian.generated_at));
    const submission = signSubmission(vectors.params.ceremony_id, id, key.priv);
    assert.equal(submission.possession, custodian.possession_signature_base64, custodian.name);
  }
});

test("Go's possession signature verifies in the browser", () => {
  for (const custodian of vectors.custodians) {
    const key = deriveKey(custodian.phrase, custodian.index);
    const id = identityOf(custodian.name, key, new Date(custodian.generated_at));
    assert.ok(
      verify(
        fromBase64(custodian.possession_signature_base64),
        possessionMessage(vectors.params.ceremony_id, id),
        key.pub,
      ),
      custodian.name,
    );
  }
});

test('the params canonical bytes and fingerprint match Go', () => {
  assert.equal(toHex(paramsCanonical(vectors.params)), vectors.params_canonical_hex);
  assert.equal(paramsFingerprint(vectors.params), vectors.params_fingerprint);
});

test('a duration renders the way protobuf JSON renders it', () => {
  for (const entry of vectors.duration_vectors) {
    const rendered =
      `{"voting_period":${goJSONString(protoDuration(parseGoDuration(entry.duration)))},` +
      `"min_execution_period":${goJSONString('0s')}}`;
    assert.equal(rendered, entry.windows_json, entry.duration);
  }
});

function fixtureSubmissions(): Submission[] {
  return vectors.custodians.map((custodian) => {
    const key = deriveKey(custodian.phrase, custodian.index);
    const id = identityOf(custodian.name, key, new Date(custodian.generated_at));
    return signSubmission(vectors.params.ceremony_id, id, key.priv);
  });
}

test('the genesis fragment is byte-identical to the one Go marshals', () => {
  const assembled = assembleGroup(vectors.params, fixtureSubmissions());
  assert.equal(assembled.genesis, vectors.group.genesis_json);
  assert.equal(assembled.constitution, vectors.group.constitution_json);
  assert.equal(assembled.policy_address, vectors.group.policy_address);
  assert.equal(assembled.computed_at, vectors.group.computed_at);
});

test('the group canonical bytes and the fingerprint read aloud match Go', () => {
  const assembled = assembleGroup(vectors.params, fixtureSubmissions());
  assert.equal(toHex(assembledCanonical(assembled)), vectors.group.canonical_hex);
  assert.equal(assembled.fingerprint, vectors.group.fingerprint);
});

test('the order submissions arrive in does not change the fingerprint', () => {
  const forwards = assembleGroup(vectors.params, fixtureSubmissions());
  const backwards = assembleGroup(vectors.params, fixtureSubmissions().reverse());
  assert.equal(backwards.fingerprint, forwards.fingerprint);
  assert.equal(backwards.genesis, forwards.genesis);
});

test('an attestation canonicalises and signs the way Go does', () => {
  for (const entry of vectors.attestations) {
    assert.equal(toHex(attestationCanonical(entry.attestation)), entry.canonical_hex, entry.attestation.name);
    const custodian = vectors.custodians.find((c) => c.name === entry.attestation.name);
    assert.ok(custodian, `no custodian vector for ${entry.attestation.name}`);
    const key = deriveKey(custodian.phrase, custodian.index);
    assert.equal(
      toBase64(sign(attestationCanonical(entry.attestation), key.priv)),
      entry.signature_base64,
      entry.attestation.name,
    );
  }
});

test('a submission whose address has been swapped is refused', () => {
  const submissions = fixtureSubmissions();
  const tampered = structuredClone(submissions[0] as Submission);
  tampered.identity.address = vectors.custodians[1]?.address as string;
  assert.throws(() => verifySubmission(vectors.params, tampered), /claims address/);
});

test('a submission whose public key has been swapped fails possession', () => {
  const submissions = fixtureSubmissions();
  const tampered = structuredClone(submissions[0] as Submission);
  tampered.identity.pubkey.key = vectors.custodians[1]?.pubkey_base64 as string;
  tampered.identity.address = '';
  tampered.identity.fingerprint = '';
  assert.throws(() => verifySubmission(vectors.params, tampered), /proof of possession does not verify/);
});

test('a submission from somebody not on the roster is refused', () => {
  const submissions = fixtureSubmissions();
  const tampered = structuredClone(submissions[0] as Submission);
  tampered.identity.name = 'Somebody Else';
  assert.throws(() => verifySubmission(vectors.params, tampered), /is not on the roster/);
});

test('a submission for a different ceremony is refused', () => {
  const submissions = fixtureSubmissions();
  const tampered = structuredClone(submissions[0] as Submission);
  tampered.ceremony_id = 'AAAAAA-BBBBBB-CCCCCC-DDDDDD';
  assert.throws(() => verifySubmission(vectors.params, tampered), /different ceremony/);
});

// The presence check is the substitution defence, so it is tested from both
// sides: it must pass for a custodian who is in the group and refuse for one who
// has been dropped.
test('a custodian left out of the group is told not to attest', () => {
  const assembled = assembleGroup(vectors.params, fixtureSubmissions());
  const own = vectors.custodians[2];
  assert.ok(own);
  presence(assembled, own.address, own.fingerprint);

  const withoutThem = structuredClone(assembled);
  withoutThem.custodians = withoutThem.custodians.filter((c) => c.address !== own.address);
  assert.throws(() => presence(withoutThem, own.address, own.fingerprint), /YOUR KEY IS NOT IN THIS GROUP/);
});

test('a custodian in the group under the wrong fingerprint is told to stop', () => {
  const assembled = assembleGroup(vectors.params, fixtureSubmissions());
  const own = vectors.custodians[0];
  assert.ok(own);
  assert.throws(() => presence(assembled, own.address, 'AAAA-BBBB'), /cannot both be true/);
});

test('a threshold equal to the roster is refused rather than warned about', () => {
  const submissions = fixtureSubmissions();
  const params = { ...vectors.params, threshold: vectors.params.custodians.length };
  assert.throws(() => assembleGroup(params, submissions), /leaves no redundancy/);
});

test('a group policy address cannot be a custodian', () => {
  const submissions = fixtureSubmissions();
  const identities = submissions.map((s) => verifySubmission(vectors.params, s));
  const first = identities[0];
  assert.ok(first);
  first.address = policyAddress(9);
  assert.throws(
    () => buildGroup(identities, 3, vectors.params.voting_period, 1, vectors.group.computed_at),
    /derived account rather than a key/,
  );
});

// Go escapes the three HTML-significant characters and JSON.stringify does not.
// This is asserted directly as well as through the genesis fragment, because a
// future simplification of the roster would silently remove the only coverage.
test("string escaping matches Go's encoder, not JSON.stringify", () => {
  assert.equal(goJSONString('Chipo & Sons <Trust>'), '"Chipo \\u0026 Sons \\u003cTrust\\u003e"');
  assert.equal(goJSONString('Naledi Ngũgĩ'), '"Naledi Ngũgĩ"');
  assert.equal(goJSONString('a"b\\c\nd\te'), '"a\\"b\\\\c\\nd\\te"');
  assert.equal(goJSONString('\u0001'), '"\\u0001"');
  assert.equal(goJSONString('line\u2028break'), '"line\\u2028break"');
  assert.notEqual(goJSONString('&'), JSON.stringify('&'));
});
