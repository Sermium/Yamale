/**
 * The identifiers an animal carries, and the commitment the chain holds in
 * place of a template.
 *
 * Three things live here and nothing else: the human-readable national
 * identifier `x/herd` already issues, the ISO 11784/11785 transponder code, and
 * the hash that goes on chain when a capture is enrolled. All three are pure —
 * no camera, no reader, no chain — which is the whole reason they are separated
 * from device.ts. They can be got wrong quietly, so they are tested hard.
 *
 * # Zero imports, deliberately
 *
 * `iris.ts` has none and this has none, so `node --test src/*.test.ts` can run
 * both without touching the SDK barrel, which reaches `.tsx` the runner cannot
 * load. The commitment uses `globalThis.crypto.subtle`, which is on the global
 * in Node 18+ and in every secure browser context, rather than importing
 * `node:crypto` — that would work under the test runner and break the bundle.
 *
 * # The thing to understand before changing anything here
 *
 * `x/herd` does not validate either identifier. `MsgIdentifyAnimal` trims the
 * string, refuses it if it is empty, and indexes it byte-for-byte
 * (`x/herd/keeper/msg_server_registry.go`). So:
 *
 *   - `221/682/1029/L` and `221/682/021029/L` are two different animals to the
 *     chain, and `982 000 000 000 003` and `982000000000003` are two different
 *     transponders.
 *   - Every uniqueness guarantee the registry offers is a guarantee about
 *     *strings*, not about animals.
 *
 * Canonicalisation is therefore not cosmetic and it is not the chain's job. It
 * is this file's job, and it has to happen before a message is signed. A
 * duplicate created by padding is invisible to every query the registry has.
 */

/* ------------------------------------------------------------ the result */

/**
 * What a parse produced.
 *
 * `reason` is a catalogue key rather than a sentence, because this layer is not
 * user-facing and a sentence written here would be a sentence in one of five
 * languages. `detail` is for a log and is not for a screen.
 */
export type Parsed<T> =
  | { ok: true; value: T }
  | { ok: false; reason: string; detail: string };

function bad<T>(reason: string, detail: string): Parsed<T> {
  return { ok: false, reason, detail };
}

/* ------------------------------------------- the national identifier */

/**
 * `221/682/021029/L` — country dialling code, flock, serial, breed letter.
 *
 * The form the Ladoum sector already issues, quoted in `Animal.national_id`'s
 * own comment. Four fields, three slashes, and the parts kept as strings rather
 * than numbers because the zero padding is part of the identifier: see the file
 * header on why `1029` and `021029` are not the same animal to the chain.
 */
export interface NationalIdentifier {
  /** Country dialling code. `221` is Senegal. 1 to 3 digits, no leading zero. */
  country: string;
  /** The flock. Digits, as issued; `682` throughout the Ladoum register. */
  flock: string;
  /** The serial within the flock. Six digits, zero-padded, by convention. */
  serial: string;
  /** The breed letter. `L` is Ladoum. One letter, upper case. */
  breed: string;
}

/**
 * How many digits a serial carries when it is issued in the canonical form.
 *
 * Every identifier in `x/herd`'s own fixtures is six digits, zero-padded
 * (`000001`, `021029`, `400001`). That is an observed convention and not a rule
 * anybody wrote down, which is exactly why `parseNationalId` accepts other
 * lengths and `isCanonicalNationalId` is a separate question.
 */
export const NATIONAL_ID_SERIAL_DIGITS = 6;

const DIGITS = /^[0-9]+$/;

/**
 * Parse a national identifier.
 *
 * Surrounding whitespace is dropped, because the chain drops it too and a
 * string that differs from the index only by a space would otherwise register a
 * second animal. Nothing else is altered: this reports what it was given.
 *
 * # What this checks, and what it cannot
 *
 * Checkable here: four fields, the right kinds of characters in each, a country
 * that is not zero-padded, a breed that is one letter.
 *
 * NOT checkable here, by anybody offline:
 *
 *   - whether `221` is a dialling code that exists. It is not validated against
 *     a list, because shipping a list of dialling codes into a field client is
 *     a maintenance liability for a check that catches almost nothing.
 *   - whether flock `682` is a flock the registry knows.
 *   - whether this serial was ever issued, or is already taken. Only the chain
 *     knows, and it answers by refusing the message.
 *   - whether `L` is a breed anybody has registered. `x/herd` holds no breed
 *     table; the letter is a convention between the registry and the sector.
 *
 * A well-formed identifier is therefore a claim about shape and nothing more.
 */
export function parseNationalId(input: string): Parsed<NationalIdentifier> {
  const text = input.trim();
  if (text === '') return bad('herd.idEmpty', 'empty');

  const parts = text.split('/');
  if (parts.length !== 4) {
    return bad(
      'herd.idFieldCount',
      `expected four fields separated by "/", got ${parts.length}`,
    );
  }

  const [country, flock, serial, breed] = parts as [string, string, string, string];

  if (!DIGITS.test(country)) {
    return bad('herd.idCountry', `country dialling code is not digits: ${JSON.stringify(country)}`);
  }
  // A dialling code is never written with a leading zero — the zero belongs to
  // the trunk prefix a caller dials, not to the code. `021` here is a
  // transcription of a phone number, not a country.
  if (country.length > 3 || country.startsWith('0')) {
    return bad('herd.idCountry', `not a dialling code: ${JSON.stringify(country)}`);
  }

  if (!DIGITS.test(flock)) {
    return bad('herd.idFlock', `flock is not digits: ${JSON.stringify(flock)}`);
  }
  if (!DIGITS.test(serial)) {
    return bad('herd.idSerial', `serial is not digits: ${JSON.stringify(serial)}`);
  }
  if (!/^[A-Za-z]$/.test(breed)) {
    return bad('herd.idBreed', `breed is not a single letter: ${JSON.stringify(breed)}`);
  }

  return { ok: true, value: { country, flock, serial, breed: breed.toUpperCase() } };
}

/** Render an identifier back to the string the chain indexes. */
export function formatNationalId(id: NationalIdentifier): string {
  return `${id.country}/${id.flock}/${id.serial}/${id.breed.toUpperCase()}`;
}

/**
 * True when the identifier is in the form the register issues.
 *
 * Separate from "valid" on purpose. A non-canonical identifier is one the chain
 * will accept and index, and which will then never be found by anybody
 * searching for the canonical spelling of the same animal. The app should
 * canonicalise and, where it cannot, say which one it is about to write.
 */
export function isCanonicalNationalId(id: NationalIdentifier): boolean {
  return (
    id.serial.length === NATIONAL_ID_SERIAL_DIGITS &&
    id.breed === id.breed.toUpperCase() &&
    !id.country.startsWith('0')
  );
}

/**
 * Pad a serial to the issued width.
 *
 * Refuses rather than truncates when the serial is already too long: a serial
 * of seven digits is either a different numbering scheme or a typo, and
 * silently dropping a digit would point the record at another animal.
 */
export function canonicaliseNationalId(id: NationalIdentifier): Parsed<NationalIdentifier> {
  if (id.serial.length > NATIONAL_ID_SERIAL_DIGITS) {
    return bad(
      'herd.idSerialTooLong',
      `serial ${id.serial} is longer than the ${NATIONAL_ID_SERIAL_DIGITS} digits the register issues`,
    );
  }
  return {
    ok: true,
    value: {
      country: id.country,
      flock: id.flock,
      serial: id.serial.padStart(NATIONAL_ID_SERIAL_DIGITS, '0'),
      breed: id.breed.toUpperCase(),
    },
  };
}

/* --------------------------------------------- ISO 11784/11785 tag codes */

/**
 * A transponder code, decomposed.
 *
 * ISO 11784 defines a 64-bit code and ISO 11785 the 134.2 kHz air interface
 * that carries it. The 64 bits are, as published:
 *
 *   bit  1      animal application flag
 *   bits 2–15   reserved (14 bits)
 *   bit  16     data block follows
 *   bits 17–26  country code (10 bits)
 *   bits 27–64  national identification code (38 bits)
 *
 * A reader hands the operator the last two fields rendered as decimal: the
 * country zero-padded to three digits and the national code zero-padded to
 * twelve, giving the familiar fifteen-digit string. That string is what
 * `Animal.tag` holds.
 *
 * THE FIELD WIDTHS ARE THE POINT. Ten bits of country is 0–1023, which three
 * digits covers. Thirty-eight bits of national code is 0–274,877,906,943, which
 * twelve digits OVER-covers — so a fifteen-digit string can be perfectly
 * well-formed and still not be a code any transponder could hold. That check is
 * below, and as far as we can tell nobody else does it.
 */
export interface TagCode {
  /** Three digits as written, zero-padded. */
  country: string;
  /** Twelve digits as written, zero-padded. */
  national: string;
  /** What the country field actually denotes. See `TagAuthority`. */
  authority: TagAuthority;
}

/**
 * What the first three digits mean, which is not always "a country".
 *
 * This trips people up constantly and it is visible in `x/herd`'s own fixtures,
 * where every tag begins `982`. That is not a country: it is an ICAR-assigned
 * manufacturer code. An animal tagged `982…` carries a transponder from a
 * manufacturer, registered in no national numbering space at all.
 *
 *   - `country`      001–899, an ISO 3166-1 NUMERIC country code
 *   - `manufacturer` 900–998, ICAR manufacturer codes (900 is the shared one)
 *   - `test`         999, reserved for test transponders — must never be enrolled
 *   - `invalid`      000, which is not assigned
 *
 * A trap worth naming, because both numbers appear in this application and they
 * are different: Senegal is **221** in a national identifier (the dialling
 * code) and **686** in a tag code (ISO 3166-1 numeric). Reusing one for the
 * other produces a string that validates and points at nothing.
 *
 * UNVERIFIED: the exact upper bound of the ICAR manufacturer range and whether
 * every value in 001–899 is presently assigned. The band boundaries below are
 * the published ones; the membership of each band changes as ICAR assigns codes
 * and is not something a client should try to hold.
 */
export type TagAuthority = 'country' | 'manufacturer' | 'test' | 'invalid';

/** 2^38 − 1. The largest national code the 38-bit field can carry. */
export const TAG_NATIONAL_MAX = 274877906943n;

/** The fifteen digits of the decimal rendering: three plus twelve. */
export const TAG_COUNTRY_DIGITS = 3;
export const TAG_NATIONAL_DIGITS = 12;

export function tagAuthorityOf(country: number): TagAuthority {
  if (country === 0) return 'invalid';
  if (country === 999) return 'test';
  if (country >= 900) return 'manufacturer';
  return 'country';
}

/**
 * Parse a transponder code.
 *
 * Accepts the separators readers and paperwork actually use — spaces, dots and
 * hyphens between groups — and normalises them away, because the chain indexes
 * the string byte-for-byte and `982 000 000 000 003` would otherwise become a
 * second animal. It does NOT accept anything else: a stray letter is a misread
 * and is worth stopping on.
 *
 * # There is no check digit, and that is the important sentence
 *
 * ISO 11785 protects the code *on the air* with a CRC, so what the reader
 * received is almost certainly what the transponder sent. It protects nothing
 * afterwards. The fifteen digits carry no checksum of their own, so a digit
 * typed wrong, transposed or dropped in re-entry produces another perfectly
 * valid code — and, because the chain enforces uniqueness on the string, it
 * will be accepted as a different animal without complaint.
 *
 * The only defences are reading the tag rather than typing it, and the two
 * independent markers §3 of the design argues for. This function cannot help.
 */
export function parseTagCode(input: string): Parsed<TagCode> {
  const text = input.trim();
  if (text === '') return bad('herd.tagEmpty', 'empty');

  // Separators out. Anything else surviving is a fault, not a formatting habit.
  const digits = text.replace(/[ .\-_]/g, '');
  if (!DIGITS.test(digits)) {
    return bad('herd.tagNotDigits', `not a decimal transponder code: ${JSON.stringify(text)}`);
  }

  const want = TAG_COUNTRY_DIGITS + TAG_NATIONAL_DIGITS;
  if (digits.length !== want) {
    return bad(
      'herd.tagLength',
      `expected ${want} digits, got ${digits.length}: ${JSON.stringify(digits)}`,
    );
  }

  const country = digits.slice(0, TAG_COUNTRY_DIGITS);
  const national = digits.slice(TAG_COUNTRY_DIGITS);

  const authority = tagAuthorityOf(Number(country));
  if (authority === 'invalid') {
    return bad('herd.tagCountry', 'country code 000 is not assigned');
  }

  // The 38-bit field. BigInt because 274,877,906,943 is beyond a safe integer
  // only by luck of the numbers involved, and relying on that is how a bound
  // check quietly stops checking.
  if (BigInt(national) > TAG_NATIONAL_MAX) {
    return bad(
      'herd.tagNationalRange',
      `national code ${national} exceeds the 38-bit field maximum ${TAG_NATIONAL_MAX}`,
    );
  }

  return { ok: true, value: { country, national, authority } };
}

/** The fifteen digits, as `Animal.tag` holds them. This is what gets signed. */
export function formatTagCode(tag: TagCode): string {
  return `${tag.country}${tag.national}`;
}

/**
 * Grouped for a screen: `982 000 000 000 003`.
 *
 * For display only. Never submit this: the chain would index the spaces.
 */
export function formatTagCodeGrouped(tag: TagCode): string {
  const groups = tag.national.match(/.{1,3}/g) ?? [];
  return [tag.country, ...groups].join(' ');
}

/**
 * True when a code may be enrolled against a real animal.
 *
 * `999…` is a test transponder. They exist, they are handed out at trade shows
 * and they are fitted to demonstration animals, and one in a national register
 * is a record pointing at a chip that was never meant to identify anything.
 */
export function isEnrollableTag(tag: TagCode): boolean {
  return tag.authority !== 'test' && tag.authority !== 'invalid';
}

/* ------------------------------------------------- the capture commitment */

/**
 * What goes on the chain in place of a template.
 *
 * Design §2: the chain holds *who enrolled what, when*, and never the template
 * itself. A biometric template is not a secret that can be rotated — if it
 * leaks it has leaked for the life of the animal — and for a breeding programme
 * it is commercially valuable in itself. So the enrolment record carries a hash
 * and the template stays in the biometric service.
 *
 * # What a commitment is good for, and what it is not
 *
 * It answers exactly one question: *is this the template that was enrolled?*
 * Anyone holding the template can recompute the hash and check it against the
 * chain, which is what makes an enrolment auditable after the fact and what
 * stops the service quietly substituting a template later.
 *
 * It does NOT identify an animal, and it cannot — §1.2 of the design says so at
 * length. Change one bit and the hash is unrelated. That is the property that
 * makes it a good commitment and a useless biometric.
 *
 * # Deterministic, and therefore not hiding
 *
 * There is no salt. There cannot be one that is thrown away, because the whole
 * value of the commitment is that a holder of the template can reproduce it
 * years later. The consequence has to be said plainly:
 *
 *   - For an iris or muzzle template — thousands of high-entropy bits — the
 *     hash reveals nothing. Nobody enumerates that space.
 *   - For a TYPED IDENTIFIER it reveals everything. There are fewer than 10^15
 *     transponder codes, and an adversary with the chain can hash all of them
 *     in an afternoon and learn which animals a keeper enrolled.
 *
 * Which is why `commitCapture` takes a modality and refuses to commit the
 * low-entropy ones. A tag number is not a secret and does not belong behind a
 * hash pretending to be one — it goes in `Animal.tag` in clear, where the
 * registry's uniqueness index can actually see it.
 */

/** Which body part or marker a capture came from. */
export type Modality = 'iris' | 'muzzle' | 'face' | 'coat' | 'tag' | 'tattoo' | 'national-id';

/**
 * The modalities whose captures carry enough entropy for a hash to hide them.
 *
 * Biometric templates only. Everything else is an identifier in a small space.
 */
export const COMMITTABLE_MODALITIES: readonly Modality[] = ['iris', 'muzzle', 'face', 'coat'];

/**
 * The domain separator.
 *
 * Mixed in so that the same bytes committed as a muzzle and as a coat give
 * different commitments, and so that a hash computed by this application can
 * never collide with one computed for anything else on this chain. The version
 * is in it because changing the encoding below without changing this string
 * would silently invalidate every enrolment already recorded.
 */
export const COMMITMENT_DOMAIN = 'yamale/herd/capture/v1';

/**
 * The bytes that get hashed, built so that no two different captures can
 * produce the same input.
 *
 * Every variable-length part is preceded by its length. Without that,
 * `modality="iris"` + payload `X` and `modality="i"` + payload `risX` would
 * concatenate to the same bytes and commit to the same thing — the classic
 * concatenation ambiguity, and the reason this is a separate exported function
 * with its own test rather than three lines inside the hash call.
 *
 * Layout, all lengths big-endian uint32:
 *
 *     domain (UTF-8, length-prefixed)
 *     modality (UTF-8, length-prefixed)
 *     payload (length-prefixed)
 */
export function commitmentPreimage(modality: Modality, payload: Uint8Array): Uint8Array {
  const enc = new TextEncoder();
  const domain = enc.encode(COMMITMENT_DOMAIN);
  const kind = enc.encode(modality);

  const out = new Uint8Array(4 + domain.length + 4 + kind.length + 4 + payload.length);
  const view = new DataView(out.buffer);
  let at = 0;

  view.setUint32(at, domain.length, false);
  at += 4;
  out.set(domain, at);
  at += domain.length;

  view.setUint32(at, kind.length, false);
  at += 4;
  out.set(kind, at);
  at += kind.length;

  view.setUint32(at, payload.length, false);
  at += 4;
  out.set(payload, at);

  return out;
}

/**
 * The canonical bytes of an iris template.
 *
 * `bits` is in the encoding because a code and a mask of the same byte length
 * can describe different bit counts — 2040 bits and 2048 bits both occupy 256
 * bytes — and two templates that compare differently must not commit
 * identically. The arrays are length-prefixed for the reason above.
 *
 * Structurally typed rather than imported from `iris.ts`, to keep this file's
 * zero imports. The shape is `IrisTemplate` and a change to one must be made in
 * both.
 */
export function encodeTemplate(template: {
  code: Uint8Array;
  mask: Uint8Array;
  bits: number;
}): Uint8Array {
  const out = new Uint8Array(4 + 4 + template.code.length + 4 + template.mask.length);
  const view = new DataView(out.buffer);
  let at = 0;

  view.setUint32(at, template.bits, false);
  at += 4;
  view.setUint32(at, template.code.length, false);
  at += 4;
  out.set(template.code, at);
  at += template.code.length;
  view.setUint32(at, template.mask.length, false);
  at += 4;
  out.set(template.mask, at);

  return out;
}

/**
 * SHA-256 of the preimage, as lower-case hex.
 *
 * Async because `crypto.subtle` is, and it is used rather than a hash written
 * here because the platform's is the audited one. It needs a secure context in
 * a browser — which this application has anyway, since WebUSB, Web Bluetooth
 * and `getUserMedia` all require one.
 *
 * Refuses a modality whose space is small enough to enumerate. See the section
 * comment: a commitment over a tag number is a hash that looks like privacy and
 * is not.
 */
export async function commitCapture(
  modality: Modality,
  payload: Uint8Array,
): Promise<Parsed<string>> {
  if (!COMMITTABLE_MODALITIES.includes(modality)) {
    return bad(
      'herd.commitmentNotSecret',
      `${modality} is an identifier in a space small enough to enumerate; ` +
        `a commitment over it hides nothing and must not be used as though it did`,
    );
  }
  if (payload.length === 0) {
    return bad('herd.commitmentEmpty', 'refusing to commit to an empty capture');
  }

  const subtle = globalThis.crypto?.subtle;
  if (!subtle) {
    // Not a hypothetical: a page served over plain http on a LAN has no
    // `crypto.subtle`, and the failure has to name the cause or somebody will
    // spend a morning on it.
    return bad(
      'herd.noWebCrypto',
      'crypto.subtle is unavailable — this needs a secure context (https or localhost)',
    );
  }

  const digest = await subtle.digest('SHA-256', plainBytes(commitmentPreimage(modality, payload)));
  return { ok: true, value: toHex(new Uint8Array(digest)) };
}

/**
 * SHA-256, hex.
 *
 * Exported so the tests can pin it against the published FIPS 180-4 vectors.
 * That is worth doing even though the implementation is the platform's: what is
 * under test is the byte handling either side of it — `TextEncoder`, the
 * `ArrayBuffer` round trip and `toHex` — and every one of those has a way of
 * being wrong that produces a plausible-looking hex string.
 */
export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) throw new Error('crypto.subtle is unavailable — needs a secure context');
  return toHex(new Uint8Array(await subtle.digest('SHA-256', plainBytes(bytes))));
}

/**
 * A view onto an ordinary ArrayBuffer.
 *
 * Two reasons, and the second is the one that bites. TypeScript 5.7 made
 * `Uint8Array` generic in its buffer, so a plain `Uint8Array` may be backed by
 * a `SharedArrayBuffer` as far as the type system knows, and `BufferSource`
 * will not accept that. Nor, at runtime, will several `crypto.subtle`
 * implementations — a shared buffer can be written to by another thread while
 * the digest is being computed, so they reject it rather than hash something
 * that changed underneath them. The copy is cheap next to the hash and it makes
 * both problems go away.
 */
function plainBytes(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(bytes.length);
  out.set(bytes);
  return out;
}

/** Lower-case hex, no prefix. The form the chain's string field holds. */
export function toHex(bytes: Uint8Array): string {
  let out = '';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}
