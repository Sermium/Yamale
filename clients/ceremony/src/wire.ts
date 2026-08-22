// The byte strings this ceremony digests and signs.
//
// This is the browser half of tools/ceremony/distributed.go and key.go, and the
// two must agree byte for byte. Both are pinned to
// testdata/vectors/ceremony.json by src/wire.test.ts on this side and
// vectors_test.go on the other; editing the fixture turns both red.
//
// Why any of this is in the browser at all: the coordinator serves the page but
// must not be able to see a phrase, so the page derives its own key. Which means
// the page also has to compute the group for itself — a page that asked the
// coordinator for the fingerprint would be trusting the one party the 3-of-5
// exists to distrust. So every canonical encoding below is duplicated logic, and
// duplicated deliberately.

import { base32crockford } from '@scure/base';
import { sha256 } from '@noble/hashes/sha2.js';

// The domains keep every digest and every signature in this ceremony from being
// mistaken for, or replayed as, another. They are versioned in lockstep with
// distributed.go: a change to any canonical encoding has to invalidate old
// values rather than silently produce a different fingerprint for the same
// ceremony.
export const PARAMS_DOMAIN = 'yamale-ceremony-params-v2';
export const GROUP_DOMAIN = 'yamale-ceremony-group-v1';
export const POSSESSION_DOMAIN = 'yamale-ceremony-possession-v1';
export const ATTESTATION_DOMAIN = 'yamale-ceremony-attestation-v1';
export const FINGERPRINT_DOMAIN = 'yamale-ceremony-fingerprint-v1';

export const SECP256K1_PUBKEY_TYPE = '/cosmos.crypto.secp256k1.PubKey';

const utf8 = new TextEncoder();

export function concatBytes(...parts: Uint8Array[]): Uint8Array {
  let total = 0;
  for (const part of parts) total += part.length;
  const out = new Uint8Array(total);
  let at = 0;
  for (const part of parts) {
    out.set(part, at);
    at += part.length;
  }
  return out;
}

function uint32be(n: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setUint32(0, n, false);
  return out;
}

// canonField appends one length-prefixed field.
//
// Length-prefixed rather than delimited, so no combination of field values can
// be reassembled into a different combination that digests the same. A custodian
// named "Okafor|Amara" must not be able to produce the same canonical bytes as
// two custodians named "Okafor" and "Amara".
//
// The length is the length in BYTES, not in JavaScript string units. Go counts
// bytes and a name with an accented letter is longer in UTF-8 than in
// UTF-16 code units, so `value.length` here would agree with Go on every ASCII
// roster and disagree the first time a custodian's name was spelled properly.
export function canonField(value: string): Uint8Array {
  const encoded = utf8.encode(value);
  return concatBytes(uint32be(encoded.length), encoded);
}

export function canonBytes(value: Uint8Array): Uint8Array {
  return concatBytes(uint32be(value.length), value);
}

export function canonCount(n: number): Uint8Array {
  return uint32be(n);
}

// crockford is the alphabet every value a person reads aloud or retypes is
// encoded in: no I, L, O or U, so 1/l and 0/O cannot be misread into a
// different valid value.
function crockfordEncode(raw: Uint8Array): string {
  return base32crockford.encode(raw);
}

function digest(domain: string, data: Uint8Array): Uint8Array {
  return sha256(concatBytes(utf8.encode(domain), data));
}

// shortDigest is forty bits in two groups of four: the fingerprint a custodian
// writes on their own sheet, where the threat is a mis-filed envelope rather
// than an adversary.
export function shortDigest(domain: string, data: Uint8Array): string {
  const encoded = crockfordEncode(digest(domain, data).subarray(0, 5));
  return `${encoded.slice(0, 4)}-${encoded.slice(4, 8)}`;
}

// longDigest is eighty bits in four groups of four, for the two values a hostile
// relay would try to collide.
//
// Forty bits is not enough for a value five custodians compare over a telephone
// to detect a substituted group: an attacker who controls the channel can grind
// candidate submission sets until one produces the fingerprint the honest
// custodians expect, and 2^40 is hours of GPU time. Eighty bits is not.
export function longDigest(domain: string, data: Uint8Array): string {
  const encoded = crockfordEncode(digest(domain, data).subarray(0, 10));
  return `${encoded.slice(0, 4)}-${encoded.slice(4, 8)}-${encoded.slice(8, 12)}-${encoded.slice(12, 16)}`;
}

// VALID_ROLES is every role a country office can be granted.
//
// Hard-coded because this bundle cannot import a Go enum, and pinned by the
// fixture's role_names array — exactly the arrangement policy_derivation already
// uses for the two SDK constants. If the chain's enum moves and this does not,
// the Go test says so rather than the page silently offering a role the chain
// does not have, or refusing one it does.
export const VALID_ROLES = [
  'ROLE_ENFORCEMENT_AUTHORITY',
  'ROLE_MONETARY_AUTHORITY',
  'ROLE_PAYMENTS_AUTHORITY',
  'ROLE_REGISTRY_AUTHORITY',
  'ROLE_SUPERVISOR',
] as const;

// OfficeParams is the country-office half of a ceremony's parameters.
//
// Absent for the foundation ceremony, which belongs to no national perimeter.
export type OfficeParams = { country: string; roles: string[] };

// The office is inside the parameters — and therefore inside the fingerprint the
// super users read aloud before generating — because it is what the key is FOR.
// Without it a coordinator could take the keys three people generated "for
// Senegal" and stand up an office granted authority over Nigeria, and nothing any
// of them had seen would have said so.
export type CeremonyParams = {
  ceremony_id: string;
  ceremony: string;
  chain_id: string;
  threshold: number;
  custodians: string[];
  policy_seq: number;
  voting_period: string;
  office?: OfficeParams | null;
};

// paramsCanonical is the byte string the params fingerprint is taken over.
//
// Written out by hand rather than by serialising an object, for the same reason
// Go writes it out by hand: a JSON encoding would make the fingerprint depend on
// key order and on whether an empty field was omitted, and this is the value
// five people compare over a telephone before anybody generates a key.
//
// The office block is last, and an absent office encodes IDENTICALLY to one with
// an empty country and no roles: canonField('') followed by a count of zero. That
// ambiguity is deliberate and unreachable — validateParams refuses an office
// whose country is not an assigned code — and what it buys is that the
// foundation's canonical bytes are the old bytes plus a fixed eight-byte tail
// rather than two shapes a reader has to hold in their head.
export function paramsCanonical(p: CeremonyParams): Uint8Array {
  const names = [...p.custodians].sort(compareGoStrings);
  // Sorted on a copy, for the same reason the names are: the encoding must depend
  // on the SET of roles the office is being granted, not on the order somebody
  // typed them into a form. compareGoStrings rather than the default sort so the
  // order is Go's sort.Strings — UTF-8 bytes — even though role names are ASCII.
  const roles = [...(p.office?.roles ?? [])].sort(compareGoStrings);
  return concatBytes(
    canonField(PARAMS_DOMAIN),
    canonField(p.ceremony_id),
    canonField(p.ceremony),
    canonField(p.chain_id),
    canonField(String(p.threshold)),
    canonField(String(p.policy_seq)),
    canonField(p.voting_period),
    canonCount(names.length),
    ...names.map(canonField),
    canonField(p.office?.country ?? ''),
    canonCount(roles.length),
    ...roles.map(canonField),
  );
}

export function paramsFingerprint(p: CeremonyParams): string {
  return longDigest(PARAMS_DOMAIN, paramsCanonical(p));
}

// compareGoStrings orders strings the way Go's sort.Strings does: by UTF-8
// bytes.
//
// JavaScript's default sort compares UTF-16 code units, which agrees with Go on
// the whole Basic Multilingual Plane and disagrees above it — a custodian name
// containing an emoji or a rare CJK extension character would sort differently
// on the two sides, changing the params canonical bytes and therefore the
// fingerprint everybody has already read aloud. localeCompare would be worse
// still: it depends on the browser's locale.
export function compareGoStrings(a: string, b: string): number {
  const left = utf8.encode(a);
  const right = utf8.encode(b);
  const shared = Math.min(left.length, right.length);
  for (let i = 0; i < shared; i++) {
    const l = left[i] as number;
    const r = right[i] as number;
    if (l !== r) return l - r;
  }
  return left.length - right.length;
}

export type PubKeyJSON = { '@type': string; key: string };

export type Identity = {
  name: string;
  role: 'custodian';
  address: string;
  pubkey: PubKeyJSON;
  fingerprint: string;
  hd_path: string;
  generated_at: string;
  ceremony?: string;
};

export type Submission = {
  ceremony_id: string;
  identity: Identity;
  possession: string;
};

// possessionMessage is what a custodian signs to prove the key is theirs.
//
// The name is in it deliberately: the name is what goes into the group's
// metadata and onto the signed record, so a signature covering only the key
// would let a relay keep an honest custodian's key and attach a different
// person's name to it.
//
// The address and the fingerprint are NOT in it, because both are derived from
// the public key and re-derived by every verifier. Signing a value nobody reads
// would be theatre.
export function possessionMessage(ceremonyID: string, id: Identity): Uint8Array {
  return concatBytes(
    canonField(POSSESSION_DOMAIN),
    canonField(ceremonyID),
    canonField(id.role),
    canonField(id.name),
    canonField(id.pubkey.key),
    canonField(id.hd_path),
    canonField(id.generated_at),
  );
}

export type Attestation = {
  ceremony_id: string;
  name: string;
  address: string;
  group_fingerprint: string;
  policy_address: string;
  transcription_verified: boolean;
  restore_drill_passed: boolean;
  envelope_sealed: boolean;
  virtualised: boolean;
  signed_at: string;
};

export function attestationCanonical(a: Attestation): Uint8Array {
  return concatBytes(
    canonField(ATTESTATION_DOMAIN),
    canonField(a.ceremony_id),
    canonField(a.name),
    canonField(a.address),
    canonField(a.group_fingerprint),
    canonField(a.policy_address),
    canonField(String(a.transcription_verified)),
    canonField(String(a.restore_drill_passed)),
    canonField(String(a.envelope_sealed)),
    canonField(String(a.virtualised)),
    canonField(a.signed_at),
  );
}

export type SignedAttestation = {
  attestation: Attestation;
  pubkey: PubKeyJSON;
  signature: string;
};

export function toBase64(raw: Uint8Array): string {
  let binary = '';
  for (const byte of raw) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function fromBase64(text: string): Uint8Array {
  const binary = atob(text);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

export function toHex(raw: Uint8Array): string {
  return Array.from(raw, (b) => b.toString(16).padStart(2, '0')).join('');
}
