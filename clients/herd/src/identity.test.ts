/**
 * The identifiers, tested against the mistakes that do not announce themselves.
 *
 * Almost everything here is a check the chain does not do. `x/herd` trims a
 * string, refuses it if empty, and indexes it — so every malformed identifier
 * this file rejects is one that would otherwise have been accepted as a new
 * animal. The tests are written from that angle rather than from "does the
 * regex work".
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  COMMITMENT_DOMAIN,
  COMMITTABLE_MODALITIES,
  NATIONAL_ID_SERIAL_DIGITS,
  TAG_NATIONAL_MAX,
  canonicaliseNationalId,
  commitCapture,
  commitmentPreimage,
  encodeTemplate,
  formatNationalId,
  formatTagCode,
  formatTagCodeGrouped,
  isCanonicalNationalId,
  isEnrollableTag,
  parseNationalId,
  parseTagCode,
  sha256Hex,
  tagAuthorityOf,
  toHex,
  type Modality,
  type NationalIdentifier,
} from './identity.ts';

function value<T>(got: { ok: true; value: T } | { ok: false; reason: string; detail: string }): T {
  assert.equal(got.ok, true, got.ok ? '' : `${got.reason}: ${got.detail}`);
  return (got as { ok: true; value: T }).value;
}

function reason(got: { ok: boolean; reason?: string }): string {
  assert.equal(got.ok, false, 'expected this to be refused');
  return got.reason!;
}

/* ------------------------------------------- the national identifier */

test('the identifier x/herd already issues parses into its four parts', () => {
  // The example from Animal.national_id's own comment, and the one every
  // fixture in x/herd/keeper uses.
  const id = value(parseNationalId('221/682/021029/L'));
  assert.deepEqual(id, { country: '221', flock: '682', serial: '021029', breed: 'L' });
  assert.equal(formatNationalId(id), '221/682/021029/L');
});

test('surrounding whitespace is dropped, because the chain drops it too', () => {
  // MsgIdentifyAnimal does strings.TrimSpace. If this did not, the client would
  // compute one string and the index would hold another.
  assert.equal(formatNationalId(value(parseNationalId('  221/682/021029/L\n'))), '221/682/021029/L');
});

test('the wrong number of fields is refused', () => {
  assert.equal(reason(parseNationalId('221/682/021029')), 'herd.idFieldCount');
  assert.equal(reason(parseNationalId('221/682/021029/L/X')), 'herd.idFieldCount');
  assert.equal(reason(parseNationalId('221-682-021029-L')), 'herd.idFieldCount');
});

test('an empty identifier is refused before anything else', () => {
  assert.equal(reason(parseNationalId('')), 'herd.idEmpty');
  assert.equal(reason(parseNationalId('   ')), 'herd.idEmpty');
});

test('a dialling code is not written with a leading zero', () => {
  // 0221 is somebody transcribing a phone number, trunk prefix and all. It
  // would index as a distinct country and quietly split the register in two.
  assert.equal(reason(parseNationalId('0221/682/021029/L')), 'herd.idCountry');
  assert.equal(reason(parseNationalId('2211/682/021029/L')), 'herd.idCountry');
  assert.equal(reason(parseNationalId('SN/682/021029/L')), 'herd.idCountry');
});

test('flock and serial must be digits', () => {
  assert.equal(reason(parseNationalId('221/68A/021029/L')), 'herd.idFlock');
  assert.equal(reason(parseNationalId('221/682/02102X/L')), 'herd.idSerial');
  assert.equal(reason(parseNationalId('221/682//L')), 'herd.idSerial');
});

test('the breed is exactly one letter', () => {
  assert.equal(reason(parseNationalId('221/682/021029/LD')), 'herd.idBreed');
  assert.equal(reason(parseNationalId('221/682/021029/9')), 'herd.idBreed');
  assert.equal(reason(parseNationalId('221/682/021029/')), 'herd.idBreed');
});

test('a lower-case breed letter is normalised, not refused', () => {
  // Refusing it would make an operator retype a correct identifier. Leaving it
  // alone would index `l` beside `L` as two animals.
  const id = value(parseNationalId('221/682/021029/l'));
  assert.equal(id.breed, 'L');
  assert.equal(formatNationalId(id), '221/682/021029/L');
});

test('an unpadded serial parses but is not canonical', () => {
  // THE failure this file exists for. `221/682/1029/L` is a well-formed
  // identifier and the chain will happily register it as a second animal
  // alongside `221/682/001029/L`, because uniqueness is over the string.
  const short = value(parseNationalId('221/682/1029/L'));
  assert.equal(isCanonicalNationalId(short), false);

  const fixed = value(canonicaliseNationalId(short));
  assert.equal(formatNationalId(fixed), '221/682/001029/L');
  assert.equal(isCanonicalNationalId(fixed), true);

  assert.notEqual(formatNationalId(short), formatNationalId(fixed),
    'these are two different animals to x/herd, which is the whole problem');
});

test('canonicalising refuses to truncate an over-long serial', () => {
  // Padding a short serial is safe. Shortening a long one would point the
  // record at a different animal, so it is a refusal rather than a repair.
  const long = value(parseNationalId('221/682/0210299/L'));
  assert.equal(reason(canonicaliseNationalId(long)), 'herd.idSerialTooLong');
});

test('the canonical serial width matches what the register issues', () => {
  assert.equal(NATIONAL_ID_SERIAL_DIGITS, 6);
  for (const issued of ['000001', '021029', '100001', '400001']) {
    const id = value(parseNationalId(`221/682/${issued}/L`));
    assert.equal(isCanonicalNationalId(id), true, issued);
  }
});

test('format round-trips every identifier parse accepts', () => {
  for (const text of ['221/682/021029/L', '1/1/000001/A', '221/682/000000/Z']) {
    const id: NationalIdentifier = value(parseNationalId(text));
    assert.equal(formatNationalId(id), text);
  }
});

/* --------------------------------------------- ISO 11784/11785 tag codes */

test('a fifteen-digit code splits three and twelve', () => {
  const tag = value(parseTagCode('982000000000003'));
  assert.equal(tag.country, '982');
  assert.equal(tag.national, '000000000003');
  assert.equal(formatTagCode(tag), '982000000000003');
});

test('separators readers and paperwork use are normalised away', () => {
  // Same transponder, four spellings. Without this the chain holds four
  // animals, because ByTag is an index over the literal string.
  const forms = ['982000000000003', '982 000 000 000 003', '982.000000000003', '982-000-000-000-003'];
  const canonical = forms.map((f) => formatTagCode(value(parseTagCode(f))));
  assert.deepEqual(new Set(canonical), new Set(['982000000000003']));
});

test('a letter in a tag code is a misread and stops', () => {
  assert.equal(reason(parseTagCode('98200000O000003')), 'herd.tagNotDigits');
  assert.equal(reason(parseTagCode('')), 'herd.tagEmpty');
});

test('fourteen or sixteen digits is refused', () => {
  // A dropped or doubled digit. There is no checksum to catch it later, so
  // length is the only structural defence there is.
  assert.equal(reason(parseTagCode('98200000000000')), 'herd.tagLength');
  assert.equal(reason(parseTagCode('9820000000000034')), 'herd.tagLength');
});

test('the first three digits are not always a country', () => {
  // 982 is an ICAR manufacturer code, and it is what every x/herd fixture uses.
  assert.equal(value(parseTagCode('982000000000003')).authority, 'manufacturer');
  // 686 is Senegal's ISO 3166-1 numeric code — NOT 221, which is the dialling
  // code the national identifier uses. Two numbers for one country, in two
  // fields of the same record.
  assert.equal(value(parseTagCode('686000000000003')).authority, 'country');
  assert.equal(value(parseTagCode('999000000000003')).authority, 'test');
  assert.equal(reason(parseTagCode('000000000000003')), 'herd.tagCountry');
});

test('the authority bands are at the published boundaries', () => {
  assert.equal(tagAuthorityOf(0), 'invalid');
  assert.equal(tagAuthorityOf(1), 'country');
  assert.equal(tagAuthorityOf(899), 'country');
  assert.equal(tagAuthorityOf(900), 'manufacturer');
  assert.equal(tagAuthorityOf(998), 'manufacturer');
  assert.equal(tagAuthorityOf(999), 'test');
});

test('a test transponder must not be enrolled against a real animal', () => {
  assert.equal(isEnrollableTag(value(parseTagCode('999000000000001'))), false);
  assert.equal(isEnrollableTag(value(parseTagCode('982000000000001'))), true);
  assert.equal(isEnrollableTag(value(parseTagCode('686000000000001'))), true);
});

test('twelve digits can express a national code no transponder can hold', () => {
  // The check nobody does. ISO 11784 gives the national code 38 bits, so the
  // ceiling is 2^38 - 1 = 274,877,906,943 — well below the 999,999,999,999 that
  // twelve digits allows. A code above it is not a transponder that exists.
  assert.equal(TAG_NATIONAL_MAX, 274877906943n);

  assert.equal(value(parseTagCode('982274877906943')).national, '274877906943');
  assert.equal(reason(parseTagCode('982274877906944')), 'herd.tagNationalRange');
  assert.equal(reason(parseTagCode('982999999999999')), 'herd.tagNationalRange');
});

test('the 38-bit bound is checked past the safe-integer range with BigInt', () => {
  // 999999999999 happens to be a safe integer, so a Number comparison would
  // pass this test and fail on nothing — which is why the bound is a BigInt and
  // why this test says so rather than relying on it.
  assert.ok(TAG_NATIONAL_MAX < BigInt(Number.MAX_SAFE_INTEGER));
  assert.equal(reason(parseTagCode('982' + '9'.repeat(12))), 'herd.tagNationalRange');
});

test('the grouped form is for a screen and never for a message', () => {
  const tag = value(parseTagCode('982000000000003'));
  assert.equal(formatTagCodeGrouped(tag), '982 000 000 000 003');
  assert.notEqual(formatTagCodeGrouped(tag), formatTagCode(tag),
    'submitting the grouped form would index the spaces');
});

test('a mistyped digit produces another valid code, and nothing here catches it', () => {
  // Stated as a test because it is a property of ISO 11784, not a gap in this
  // parser: there is no check digit. Both of these parse. The only defences are
  // reading the tag instead of typing it and the second independent marker the
  // design argues for.
  const typed = value(parseTagCode('982000000000003'));
  const fatFingered = value(parseTagCode('982000000000008'));
  assert.notEqual(formatTagCode(typed), formatTagCode(fatFingered));
  assert.equal(typed.authority, fatFingered.authority);
});

/* ------------------------------------------------- the capture commitment */

test('SHA-256 is wired up correctly, against the published vectors', async () => {
  // FIPS 180-4's own one-block and multi-block examples. What is actually under
  // test is the encoding either side of the platform's digest: TextEncoder, the
  // ArrayBuffer round trip, and toHex's zero padding.
  const enc = new TextEncoder();
  assert.equal(
    await sha256Hex(enc.encode('abc')),
    'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
  );
  assert.equal(
    await sha256Hex(new Uint8Array(0)),
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
  );
  assert.equal(
    await sha256Hex(enc.encode('abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq')),
    '248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1',
  );
});

test('toHex pads, which is the bug that makes two hashes collide', () => {
  assert.equal(toHex(new Uint8Array([0x00, 0x0f, 0xff, 0x01])), '000fff01');
  assert.equal(toHex(new Uint8Array(0)), '');
});

test('the preimage is length-prefixed so two captures cannot collide', () => {
  // The concatenation ambiguity, made concrete. Without the length prefixes,
  // modality "coat" + payload [0x01] and modality "coa" + payload [0x74, 0x01]
  // would produce identical bytes and identical commitments — two different
  // enrolments indistinguishable on chain.
  const a = commitmentPreimage('coat', new Uint8Array([0x01]));
  const b = commitmentPreimage('face', new Uint8Array([0x01]));
  assert.notDeepEqual(a, b);

  const long = commitmentPreimage('coat', new Uint8Array([0x74, 0x01]));
  assert.notDeepEqual(a, long);
  assert.equal(long.length, a.length + 1);
});

test('the domain separator is in every preimage', () => {
  const bytes = commitmentPreimage('iris', new Uint8Array([9]));
  const text = new TextDecoder().decode(bytes);
  assert.ok(text.includes(COMMITMENT_DOMAIN));
});

test('the same capture commits to the same hash every time', async () => {
  // The property the whole scheme rests on: the holder of the template can
  // recompute the commitment years later and check it against the chain. A
  // salt would break exactly this, which is why there is not one.
  const payload = new Uint8Array([1, 2, 3, 4, 5]);
  const first = value(await commitCapture('iris', payload));
  const second = value(await commitCapture('iris', Uint8Array.from(payload)));
  assert.equal(first, second);
  assert.equal(first.length, 64);
});

test('the same bytes under a different modality commit differently', async () => {
  const payload = new Uint8Array([1, 2, 3]);
  const iris = value(await commitCapture('iris', payload));
  const muzzle = value(await commitCapture('muzzle', payload));
  assert.notEqual(iris, muzzle);
});

test('a one-bit change makes the commitment unrelated', async () => {
  // The property that makes it a good commitment and a useless biometric —
  // design §1.2. Two captures of the same eye on the same day have different
  // hashes, so this can never answer "which animal is this".
  const a = value(await commitCapture('iris', new Uint8Array([0b0000_0000])));
  const b = value(await commitCapture('iris', new Uint8Array([0b0000_0001])));
  assert.notEqual(a, b);
});

test('a tag number is refused: hashing it hides nothing', async () => {
  // Fewer than 10^15 codes. Anybody with the chain can enumerate them and learn
  // which animals a keeper enrolled, so a commitment over one is a hash wearing
  // privacy's clothes. Tag numbers go in Animal.tag in clear, where the
  // uniqueness index can see them.
  const enc = new TextEncoder();
  for (const modality of ['tag', 'tattoo', 'national-id'] as Modality[]) {
    assert.equal(
      reason(await commitCapture(modality, enc.encode('982000000000003'))),
      'herd.commitmentNotSecret',
      modality,
    );
  }
  for (const modality of COMMITTABLE_MODALITIES) {
    assert.equal((await commitCapture(modality, new Uint8Array([1]))).ok, true, modality);
  }
});

test('an empty capture is refused rather than committed', async () => {
  // A zero-length payload hashes perfectly well, and the resulting enrolment
  // would look exactly like a real one. It means a camera returned nothing.
  assert.equal(reason(await commitCapture('muzzle', new Uint8Array(0))), 'herd.commitmentEmpty');
});

test('a template commits to its bit count, not only to its bytes', async () => {
  // 2040 bits and 2048 bits both occupy 256 bytes, and iris.ts compares them
  // differently — templates of different lengths are not comparable at all. Two
  // templates that behave differently must not commit identically.
  const code = new Uint8Array(32).fill(0xa5);
  const mask = new Uint8Array(32).fill(0xff);
  const full = encodeTemplate({ code, mask, bits: 256 });
  const short = encodeTemplate({ code, mask, bits: 248 });
  assert.notDeepEqual(full, short);
  assert.notEqual(
    value(await commitCapture('iris', full)),
    value(await commitCapture('iris', short)),
  );
});

test('a template encoding cannot be confused by moving bytes between the arrays', () => {
  // code 33 bytes / mask 31 bytes against code 32 / mask 32, same total.
  const a = encodeTemplate({ code: new Uint8Array(33), mask: new Uint8Array(31), bits: 248 });
  const b = encodeTemplate({ code: new Uint8Array(32), mask: new Uint8Array(32), bits: 248 });
  assert.equal(a.length, b.length);
  assert.notDeepEqual(a, b);
});
