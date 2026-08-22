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
  FOUNDATION_LABEL,
  MAX_GROUP_METADATA,
  assembleGroup,
  assembledCanonical,
  buildGroup,
  foundationPurpose,
  groupLabel,
  groupMetadata,
  policyAddress,
  presence,
  purposeFor,
  validateParams,
  verifySubmission,
} from './group.ts';
import {
  VALID_ROLES,
  attestationCanonical,
  canonBytes,
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

type GroupVector = {
  computed_at: string;
  policy_address: string;
  label: string;
  metadata: string;
  policy_metadata: string;
  genesis_json: string;
  constitution_json: string;
  canonical_hex: string;
  fingerprint: string;
};

type Vectors = {
  policy_derivation: { module: string; group_policy_table_prefix: number };
  role_names: string[];
  policy_addresses: Array<{ seq: number; address: string }>;
  duration_vectors: Array<{ duration: string; windows_json: string }>;
  params: CeremonyParams;
  params_canonical_hex: string;
  params_fingerprint: string;
  office_params: CeremonyParams;
  office_params_canonical_hex: string;
  office_params_fingerprint: string;
  office_group: GroupVector;
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
  group: GroupVector;
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
  assert.ok(vectors.office_params.office, 'the fixture must carry the office ceremony');
  assert.ok(vectors.office_group.fingerprint.length > 0);
});

// The role table, pinned the same way the two SDK constants are.
//
// This page cannot import x/alias's enum, so it carries a copy, and a copy that
// nothing checks is a copy that drifts. A role added to the chain and not here
// would be a role the coordinator's form silently refused; one removed from the
// chain and not here would be a ceremony whose super users read a fingerprint
// aloud for authority the chain will never grant.
test("the page's role table is the chain's role table", () => {
  assert.deepEqual([...VALID_ROLES], vectors.role_names);
  assert.ok(!vectors.role_names.includes('ROLE_UNSPECIFIED'), 'the unset default is never a role');
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

// The office half of the params encoding, which is the new one and therefore the
// one that can diverge without anybody noticing.
//
// The foundation's bytes are the old bytes plus a fixed empty tail, so they would
// keep matching even if the office block had been implemented differently on each
// side. These would not.
test('the office params canonical bytes and fingerprint match Go', () => {
  assert.equal(toHex(paramsCanonical(vectors.office_params)), vectors.office_params_canonical_hex);
  assert.equal(paramsFingerprint(vectors.office_params), vectors.office_params_fingerprint);
  assert.notEqual(vectors.office_params_fingerprint, vectors.params_fingerprint);
});

// An absent office and an explicitly-empty one must encode identically.
//
// Go's nil pointer and this page's undefined both have to produce canonField('')
// followed by a zero count. If they did not, a foundation ceremony read by one
// side as "no office" and by the other as "an office with nothing in it" would
// produce two different fingerprints from the same parameters.
test('no office encodes the same as an empty one', () => {
  const absent = { ...vectors.params };
  delete absent.office;
  const explicitlyNull = { ...vectors.params, office: null };
  const empty = { ...vectors.params, office: { country: '', roles: [] } };
  assert.equal(toHex(paramsCanonical(absent)), vectors.params_canonical_hex);
  assert.equal(toHex(paramsCanonical(explicitlyNull)), vectors.params_canonical_hex);
  assert.equal(toHex(paramsCanonical(empty)), vectors.params_canonical_hex);

  // And the tail really is eight zero bytes: four for the empty country, four
  // for the zero role count.
  assert.ok(vectors.params_canonical_hex.endsWith('0000000000000000'));
});

// The office is in the fingerprint so that keys generated for one country cannot
// be reused for an office over another. That is only true if changing either
// value moves the fingerprint, so it is asserted rather than assumed.
test('the country and the roles are covered by the fingerprint', () => {
  const office = vectors.office_params.office as { country: string; roles: string[] };
  const elsewhere = { ...vectors.office_params, office: { ...office, country: 'NG' } };
  assert.notEqual(paramsFingerprint(elsewhere), vectors.office_params_fingerprint);

  const fewer = { ...vectors.office_params, office: { ...office, roles: office.roles.slice(0, 1) } };
  assert.notEqual(paramsFingerprint(fewer), vectors.office_params_fingerprint);

  // But the ORDER must not move it: two coordinators typing the same two roles
  // in different orders have to produce the same value, or the read-aloud check
  // fails for a reason that has nothing to do with an attacker.
  const reversed = { ...vectors.office_params, office: { ...office, roles: [...office.roles].reverse() } };
  assert.notDeepEqual([...office.roles].reverse(), office.roles, 'the fixture roles must not already be sorted');
  assert.equal(paramsFingerprint(reversed), vectors.office_params_fingerprint);
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

// officeSubmissions rebuilds the office ceremony's submissions from the same
// phrases, signed over the OFFICE ceremony id.
//
// Regenerated rather than stored, because the derivation and the signing are
// already pinned by the foundation vectors; what is under test here is the group
// the office parameters produce from them.
function officeSubmissions(): Submission[] {
  return vectors.office_params.custodians.map((name) => {
    const custodian = vectors.custodians.find((c) => c.name === name);
    assert.ok(custodian, `no custodian vector for ${name}`);
    const key = deriveKey(custodian.phrase, custodian.index);
    const id = identityOf(name, key, new Date(custodian.generated_at));
    return signSubmission(vectors.office_params.ceremony_id, id, key.priv);
  });
}

test("an office's group is byte-identical to the one Go builds", () => {
  const assembled = assembleGroup(vectors.office_params, officeSubmissions());
  assert.equal(assembled.genesis, vectors.office_group.genesis_json);
  assert.equal(assembled.policy_address, vectors.office_group.policy_address);
  assert.equal(assembled.computed_at, vectors.office_group.computed_at);
  assert.equal(toHex(assembledCanonical(assembled)), vectors.office_group.canonical_hex);
  assert.equal(assembled.fingerprint, vectors.office_group.fingerprint);
  assert.notEqual(assembled.fingerprint, vectors.group.fingerprint);
});

// The label, on both paths, because it is what the group is called on chain
// permanently and it is inside the bytes the fingerprint covers.
test('the group label names the office, not the foundation', () => {
  assert.equal(groupLabel(vectors.params), FOUNDATION_LABEL);
  assert.equal(groupLabel(vectors.params), vectors.group.label);
  assert.equal(groupLabel(vectors.office_params), vectors.office_group.label);
  assert.ok(!vectors.office_group.label.includes(FOUNDATION_LABEL));

  for (const [params, vector] of [
    [vectors.params, vectors.group],
    [vectors.office_params, vectors.office_group],
  ] as const) {
    const submissions = params === vectors.params ? fixtureSubmissions() : officeSubmissions();
    const identities = submissions.map((s) => verifySubmission(params, s));
    const documents = buildGroup(
      identities,
      purposeFor(params),
      params.threshold,
      params.voting_period,
      params.policy_seq,
      vector.computed_at,
    );
    assert.equal(documents.metadata, vector.metadata);
    assert.equal(documents.policyMetadata, vector.policy_metadata);
    assert.ok(documents.genesis.includes(vector.label.replace(/&/g, '\\u0026')) || documents.genesis.includes(vector.label));
  }
});

// The single most likely place for the two languages to part company.
//
// Go leaves assembled.Constitution nil; this page holds ''. canonBytes has to
// turn both into the same four zero bytes. A null on one side and a "null" or
// "{}" on the other would give a country's super users a different group
// fingerprint from their coordinator's — the one failure the read-aloud step
// cannot tell apart from an attack.
test('an office gets no constitutional invariants fragment, and its absence hashes identically', () => {
  const assembled = assembleGroup(vectors.office_params, officeSubmissions());
  assert.equal(assembled.constitution, '');
  assert.equal(vectors.office_group.constitution_json, '');

  // The foundation still gets one.
  assert.notEqual(vectors.group.constitution_json, '');

  // Four zero bytes, from an empty string, from a zero-length array, and at the
  // end of the office's canonical bytes.
  assert.deepEqual([...canonBytes(new TextEncoder().encode(''))], [0, 0, 0, 0]);
  assert.deepEqual([...canonBytes(new Uint8Array(0))], [0, 0, 0, 0]);
  assert.ok(vectors.office_group.canonical_hex.endsWith('00000000'));
});

// x/group refuses metadata over 255 bytes and nothing on the genesis path checks
// it, so the foundation would never have noticed while an office's
// create-group transaction would have failed after the ceremony was over.
test('metadata longer than x/group accepts is refused', () => {
  const identities = officeSubmissions().map((s) => verifySubmission(vectors.office_params, s));
  const long = 'Autorité nationale de régulation des paiements '.repeat(8);
  assert.throws(
    () =>
      buildGroup(identities, { label: long, office: true }, 2, vectors.office_params.voting_period, 7, vectors.office_group.computed_at),
    new RegExp(`x/group refuses anything over ${MAX_GROUP_METADATA}`),
  );

  // The boundary is not off by one: the office's real label must still build.
  const documents = buildGroup(
    identities,
    purposeFor(vectors.office_params),
    vectors.office_params.threshold,
    vectors.office_params.voting_period,
    vectors.office_params.policy_seq,
    vectors.office_group.computed_at,
  );
  assert.ok(new TextEncoder().encode(documents.metadata).length <= MAX_GROUP_METADATA);
});

// The office parameters, refused on this side too. The coordinator is where a
// person types these, and a page that accepted what the binary refuses would send
// somebody round a loop with a server error instead of a sentence.
test('office parameters this chain cannot honour are refused', () => {
  validateParams(vectors.office_params);

  const office = vectors.office_params.office as { country: string; roles: string[] };
  const withOffice = (patch: Partial<{ country: string; roles: string[] }>) => ({
    ...vectors.office_params,
    office: { ...office, ...patch },
  });

  assert.throws(() => validateParams(withOffice({ country: 'sn' })), /two uppercase letters/);
  assert.throws(() => validateParams(withOffice({ country: '*' })), /chain-wide scope/);
  assert.throws(() => validateParams(withOffice({ country: 'ZZ' })), /ABSENCE of a national perimeter/);
  assert.throws(() => validateParams(withOffice({ country: 'SEN' })), /exactly two uppercase letters/);
  assert.throws(() => validateParams(withOffice({ roles: [] })), /holds no roles/);
  assert.throws(() => validateParams(withOffice({ roles: ['ROLE_TREASURER'] })), /not a role this chain has/);
  assert.throws(() => validateParams(withOffice({ roles: ['ROLE_UNSPECIFIED'] })), /unset default/);
  assert.throws(() => validateParams(withOffice({ roles: ['role_supervisor'] })), /not written the way this chain spells it/);
  assert.throws(
    () => validateParams(withOffice({ roles: ['ROLE_SUPERVISOR', 'ROLE_SUPERVISOR'] })),
    /listed twice/,
  );

  // And no office at all stays legal: that is the foundation ceremony.
  const foundation = { ...vectors.office_params };
  delete foundation.office;
  validateParams(foundation);
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
    () => buildGroup(identities, foundationPurpose(), 3, vectors.params.voting_period, 1, vectors.group.computed_at),
    /derived account rather than a key/,
  );
});

// The timestamp format is the one value a correctly-signed submission can choose
// that this page and the Go binary would read differently: assembleGroup picks
// the latest generated_at by comparing strings, which is chronological only for
// the canonical form. So anything else has to be a refusal on both sides.
test('a generated_at that is not UTC to the second is refused', () => {
  const submissions = fixtureSubmissions();
  for (const spelling of ['2026-03-02T11:15:00+02:00', '2026-03-02T09:15:00.500Z', '2026-03-02T09:15:00-00:00']) {
    const tampered = structuredClone(submissions[0] as Submission);
    tampered.identity.generated_at = spelling;
    assert.throws(() => verifySubmission(vectors.params, tampered), /UTC, whole seconds, trailing Z/, spelling);
  }
});

test("a key derived on another chain's path is refused", () => {
  const tampered = structuredClone(fixtureSubmissions()[0] as Submission);
  tampered.identity.hd_path = "m/44'/60'/0'/0/0";
  assert.throws(() => verifySubmission(vectors.params, tampered), /this chain's accounts live under/);
});

// Both bounds exist because beyond them this page and the binary would DISAGREE
// rather than refuse, and one of the two values decides where every seized asset
// on the chain is sent.
test('parameters the browser cannot represent exactly are refused', () => {
  validateParams(vectors.params);

  assert.throws(() => validateParams({ ...vectors.params, policy_seq: 2 ** 60 }), /past the range this page/);
  assert.throws(() => validateParams({ ...vectors.params, voting_period: '-1h' }), /no window to vote in/);
  assert.throws(() => validateParams({ ...vectors.params, voting_period: '0s' }), /no window to vote in/);
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
