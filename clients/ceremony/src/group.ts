// The group, computed in the browser.
//
// This is the pure function at the centre of a distributed ceremony, and it is in
// the browser rather than on the coordinator on purpose. Building the group from
// five submissions depends on nothing local, so every custodian's page computes
// it and nobody is trusted to have done it honestly. The coordinator relays; it
// does not assemble on anyone's behalf.
//
// A relay who substituted a submission changes this fingerprint on all five
// devices at once, and the five custodians read it to each other over a call.
// That is the only check that catches a hostile relay, because the five pages
// share no channel they can trust — which is the whole problem a hosted ceremony
// creates and the reason the air-gapped binary is still the stronger option.
//
// Byte-for-byte agreement with tools/ceremony/group.go and distributed.go is
// therefore not tidiness. If two honest instances could produce different bytes
// from the same five submissions, the comparison would fail for innocent
// reasons, the custodians would learn to shrug it off, and the check would be
// gone. src/wire.test.ts pins every byte below against the Go binary through
// testdata/vectors/ceremony.json.

import { sha256 } from '@noble/hashes/sha2.js';
import { goJSONArray, goJSONObject, goJSONString, indentGoJSON, parseGoDuration, protoDuration } from './gojson.ts';
import { ACCOUNT_PREFIX, addressBytes, bech32Address, decodeBech32, fingerprintOf, hdPath, verify } from './key.ts';
import {
  GROUP_DOMAIN,
  SECP256K1_PUBKEY_TYPE,
  VALID_ROLES,
  canonBytes,
  canonCount,
  canonField,
  compareGoStrings,
  concatBytes,
  fromBase64,
  longDigest,
  toBase64,
  paramsCanonical,
  possessionMessage,
  type CeremonyParams,
  type Identity,
  type OfficeParams,
  type Submission,
} from './wire.ts';

// GROUP_MODULE and GROUP_POLICY_TABLE_PREFIX are the two SDK constants a policy
// address is derived from.
//
// Written out here because this bundle does not link the SDK, and pinned by the
// fixture's policy_derivation block: if either moves upstream, the Go test goes
// red and says that every address this ceremony has ever produced has moved with
// it. A browser silently deriving the old address while the chain derived the new
// one would put the wrong destination in genesis for every future seizure.
const GROUP_MODULE = 'group';
const GROUP_POLICY_TABLE_PREFIX = 0x20;

const THRESHOLD_POLICY_TYPE = '/cosmos.group.v1.ThresholdDecisionPolicy';
const GROUP_ID = 1;

// FOUNDATION_LABEL is what a group with no office is called.
//
// A constant rather than a literal in three places, because it is inside the
// group metadata the group fingerprint covers: the foundation's bytes must not
// move because the country path was added.
export const FOUNDATION_LABEL = 'Yamale foundation';

// MAX_GROUP_METADATA is x/group's limit, in bytes.
//
// Not ours: the keeper's assertMetadataLength refuses metadata longer than
// Config.MaxMetadataLen, which defaults to 255 and this chain does not override.
// clients/foundation/foundation.js carries the same number.
//
// Checked here because the two paths fail differently and one fails silently.
// x/group's genesis validation never checks metadata length, so an over-long
// string imports into a genesis file happily — the foundation path would never
// notice. A country office's group is created by MsgCreateGroupWithPolicy on a
// running chain, which IS checked, so the transaction would fail after the whole
// ceremony was over and the keys were on paper.
export const MAX_GROUP_METADATA = 255;

const utf8Length = (value: string): number => new TextEncoder().encode(value).length;

// groupLabel is what this ceremony's group is called, on chain, permanently.
//
// It is the one field a human reads to find out what a group is, which is why it
// is derived from the parameters rather than hard-coded. A country office
// recorded as "Yamale foundation" would be a lie in exactly the place nobody
// would think to check.
export function groupLabel(p: CeremonyParams): string {
  if (!p.office) return FOUNDATION_LABEL;
  return `${p.ceremony} (${p.office.country})`;
}

const utf8 = new TextEncoder();

function addressHash(typ: Uint8Array, key: Uint8Array): Uint8Array {
  return sha256(concatBytes(sha256(typ), key));
}

// policyAddress derives the address x/group gives the seq-th group policy
// account ever created on a chain.
//
// The derivation depends on the module name, a table prefix and the sequence
// number — and on nothing else. Not the members, not the threshold, not the
// admin, not the chain id. Which means the address is knowable before genesis
// and commits to NOTHING about who controls it: the same address is produced by
// a 3-of-5 of these five custodians and by a 1-of-1 of an attacker.
//
// That is why the ceremony puts the whole group in genesis rather than pasting
// the address in and creating the group later. A genesis naming this address
// while the group is created by transaction afterwards is a promise that whoever
// wins the race to create policy number one owns every asset the chain ever
// seizes.
export function policyAddress(seq: number): string {
  const derivationKey = new Uint8Array(8);
  new DataView(derivationKey.buffer).setBigUint64(0, BigInt(seq), false);

  // The first derivation key is concatenated into the module hash rather than
  // derived from it — address.Module appends it to the module name and the zero
  // separator, and only the SECOND and later keys go through Derive. Deriving
  // both in sequence produces a perfectly plausible address that no chain has
  // ever used, which is exactly the kind of mistake the policy_addresses
  // vectors exist to catch.
  const seed = concatBytes(
    utf8.encode(GROUP_MODULE),
    new Uint8Array([0]),
    new Uint8Array([GROUP_POLICY_TABLE_PREFIX]),
  );
  let addr = addressHash(utf8.encode('module'), seed);
  addr = addressHash(addr, derivationKey);
  return bech32Address(ACCOUNT_PREFIX, addr);
}

export function validateParams(p: CeremonyParams): void {
  if (p.ceremony_id.trim() === '') {
    throw new Error('the ceremony has no id, so a submission could not be tied to it');
  }
  if (p.chain_id.trim() === '') {
    throw new Error('chain_id is required: an address is only meaningful against a named chain');
  }
  if (p.custodians.length < 3) {
    throw new Error(`${p.custodians.length} custodians is not a group worth distributing`);
  }
  const seen = new Set<string>();
  for (const name of p.custodians) {
    const trimmed = name.trim();
    if (trimmed === '') throw new Error('a custodian on the roster has no name');
    if (seen.has(trimmed)) {
      throw new Error(`"${trimmed}" appears twice on the roster; two custodians cannot be told apart by name`);
    }
    seen.add(trimmed);
  }
  if (p.threshold < 2) {
    throw new Error(
      `a threshold of ${p.threshold} means one custodian acts alone, which is the single key this ceremony replaces`,
    );
  }
  if (p.threshold >= p.custodians.length) {
    throw new Error(
      `a threshold of ${p.threshold} over ${p.custodians.length} custodians leaves no redundancy: losing one key ` +
        'would freeze the foundation account forever, with the chain still sending seizures to it',
    );
  }
  const period = parseGoDuration(p.voting_period);
  // Guarded here because the SDK's own check is narrower than it looks: it
  // refuses a voting period of exactly zero and says nothing about a negative
  // one, and x/group's genesis validation never goes deeper. A period of "-1h"
  // produces a genesis that imports cleanly and a group whose proposals expire
  // before they are made.
  if (period <= 0n) {
    throw new Error(
      `a voting period of ${p.voting_period} gives the other custodians no window to vote in, so this group ` +
        'could never execute anything three of them agreed on',
    );
  }
  // Bounded because this side holds the sequence as a JavaScript number, which
  // is exact only to 2^53. Beyond that this page would derive a DIFFERENT
  // POLICY ADDRESS from the binary — silently, for the one value that decides
  // where every seized asset on the chain is sent.
  if (!Number.isSafeInteger(p.policy_seq) || p.policy_seq < 0 || p.policy_seq > 2 ** 40) {
    throw new Error(
      `policy_seq ${p.policy_seq} is past anything a chain could have reached, or past the range this page ` +
        'holds exactly',
    );
  }
  validateOffice(p.office);
}

// CHAIN_WIDE and FOUNDATION_COUNTRY are the two two-character values that are
// legal somewhere on this chain and are never a country office's perimeter.
//
// "*" is the chain-wide scope, which is the foundation's alone. "ZZ" marks the
// ABSENCE of a national perimeter. They are refused with their own messages
// rather than falling through to "not a country", because one of them reads to a
// human like authority over everywhere while conferring authority over nowhere,
// and the other is the highest privilege on the chain.
const CHAIN_WIDE = '*';
const FOUNDATION_COUNTRY = 'ZZ';

// validateOffice checks the country-office half.
//
// The assigned-country list is NOT duplicated here. This page cannot import
// x/alias's table and a copy of it would drift, so the shape and the two reserved
// values are checked here and the membership of the list is left to the
// coordinator, which does import it — params.validate() on the Go side refuses an
// unassigned code before any invitation is issued, so the page never sees one.
// What this catches is a page and a binary that disagree about the SHAPE, which
// is what would move the fingerprint.
export function validateOffice(office: OfficeParams | null | undefined): void {
  if (!office) return;

  const country = office.country;
  // Refused rather than normalised. The country is inside the parameters
  // fingerprint the super users read aloud before generating, so a value this
  // page silently rewrote would be a value none of them agreed to — the same rule
  // checkCanonicalTimestamp applies to generated_at, and for the same reason.
  if (country !== country.toUpperCase()) {
    throw new Error(
      `the office's country is "${country}" and this ceremony writes "${country.toUpperCase()}" — two uppercase ` +
        'letters. A value silently rewritten is a value the super users did not read aloud before generating',
    );
  }
  if (country === CHAIN_WIDE) {
    throw new Error(
      `"${country}" is the chain-wide scope, which is the foundation's alone and is not a country. An office ` +
        'holds authority inside one perimeter; a ceremony that could name it would be a ceremony for handing a ' +
        'national office authority over every country',
    );
  }
  if (country === FOUNDATION_COUNTRY) {
    throw new Error(
      `"${country}" is the reserved code that marks the ABSENCE of a national perimeter, not a country. An ` +
        'office recorded there would hold authority over nowhere while reading to a human as authority over ' +
        'everywhere. A ceremony with no perimeter is the foundation\'s: leave the country blank',
    );
  }
  if (!/^[A-Z]{2}$/.test(country)) {
    throw new Error(`the office's country is "${country}"; a country code here is exactly two uppercase letters, A to Z`);
  }

  if (office.roles.length === 0) {
    throw new Error(
      'the office holds no roles, so the chain would refuse every action it ever attempted. An office worth a ' +
        'key ceremony holds at least one',
    );
  }
  const seen = new Set<string>();
  for (const role of office.roles) {
    if (role !== role.toUpperCase().trim()) {
      throw new Error(
        `the role "${role}" is not written the way this chain spells it, "${role.toUpperCase().trim()}". The ` +
          'roles are covered by the fingerprint the super users read aloud, so this is refused rather than tidied up',
      );
    }
    // Refused explicitly rather than by absence from the table. ROLE_UNSPECIFIED
    // is spellable and it is the enum's zero value; proto3 cannot tell a zero
    // from a field nobody filled in, which is why it is reserved. An office that
    // named it would produce a grant the chain rejects AFTER the custodians had
    // voted for it.
    if (role === 'ROLE_UNSPECIFIED') {
      throw new Error(
        `"${role}" is the unset default and is never a role. Proto3 cannot tell a zero from a field nobody ` +
          'filled in, which is why it is reserved',
      );
    }
    if (!(VALID_ROLES as readonly string[]).includes(role)) {
      throw new Error(`"${role}" is not a role this chain has. They are ${VALID_ROLES.join(', ')}`);
    }
    if (seen.has(role)) {
      throw new Error(
        `${role} is listed twice. The roles are a set, and a list that repeats one reads on the record as though ` +
          'the office were granted it twice',
      );
    }
    seen.add(role);
  }
}

function onRoster(roster: string[], name: string): boolean {
  return roster.some((candidate) => candidate.trim() === name.trim());
}

// verifySubmission checks a submission relayed from somebody else's device.
//
// Everything derivable is re-derived rather than read. The address and the
// fingerprint in an incoming submission are claims to be checked against the
// public key, not facts — a submission whose address named an attacker while its
// public key belonged to an honest custodian would otherwise put the attacker in
// the group, and the fingerprint read aloud, the presence check and the record
// would all agree with it because they all read that field.
//
// Returns the identity this page will use, which is the derived one.
export function verifySubmission(params: CeremonyParams, s: Submission): Identity {
  if (s.ceremony_id !== params.ceremony_id) {
    throw new Error(
      `this submission is for ceremony "${s.ceremony_id}" and we are running "${params.ceremony_id}". ` +
        'It is either from a different ceremony or from somebody who was told a different id',
    );
  }
  if (s.identity.role !== 'custodian') {
    throw new Error(
      `${s.identity.name} is recorded as "${s.identity.role}", not a custodian; ` +
        'a validator operator key does not belong in the foundation group',
    );
  }
  if (!onRoster(params.custodians, s.identity.name)) {
    throw new Error(
      `"${s.identity.name}" is not on the roster of ${params.custodians.length} custodians agreed at the start ` +
        'of this ceremony. Either the roster is wrong or this submission is from somebody who was not invited',
    );
  }
  if (s.identity.pubkey['@type'] !== SECP256K1_PUBKEY_TYPE) {
    throw new Error(`${s.identity.name}'s key is a "${s.identity.pubkey['@type']}"; this chain's accounts are secp256k1`);
  }

  const raw = fromBase64(s.identity.pubkey.key);
  if (raw.length !== 33) {
    throw new Error(`${s.identity.name}'s public key is ${raw.length} bytes; a compressed secp256k1 key is 33`);
  }
  checkCanonicalTimestamp(s.identity.name, s.identity.generated_at);
  checkHDPath(s.identity.name, s.identity.hd_path);
  if (!verify(fromBase64(s.possession), possessionMessage(params.ceremony_id, s.identity), raw)) {
    throw new Error(
      `${s.identity.name}'s proof of possession does not verify. Whoever produced this does not hold the key it ` +
        'announces — do not put this address in the group, and say so on the call',
    );
  }

  const derived = identityFromPubKey(s.identity.name, raw, s.identity.hd_path, s.identity.generated_at);
  derived.ceremony = params.ceremony;

  if (s.identity.address !== '' && s.identity.address !== derived.address) {
    throw new Error(
      `${s.identity.name}'s submission claims address ${s.identity.address} but its public key derives ` +
        `${derived.address}. It has been edited`,
    );
  }
  if (s.identity.fingerprint !== '' && s.identity.fingerprint !== derived.fingerprint) {
    throw new Error(
      `${s.identity.name}'s submission claims fingerprint ${s.identity.fingerprint} but its public key derives ` +
        `${derived.fingerprint}. It has been edited`,
    );
  }
  return derived;
}

// identityFromPubKey rebuilds the public record from the public half alone.
//
// The split is the point rather than a convenience: a custodian on another
// device transmits a public key, and every other page DERIVES the address and
// the fingerprint from it here instead of reading the ones the submission
// claims.
// checkCanonicalTimestamp refuses any generated_at that is not UTC, whole
// seconds, trailing Z.
//
// It is the one place this page and tools/ceremony can disagree while both being
// honest. assembleGroup below picks the LATEST generated_at by comparing the
// strings, which is chronological only because every value has the same width
// and the same zone; Go parses them and re-emits the winner normalised. A
// submission carrying "2026-03-02T11:15:00+02:00" — valid RFC 3339, correctly
// signed, the same instant as an earlier Z value that sorts before it — would
// have this page compute one group fingerprint and the binary compute another,
// which is the failure the read-aloud step cannot tell apart from an attack.
//
// Refused rather than normalised: a value quietly rewritten is a value the
// custodian did not sign.
export function checkCanonicalTimestamp(name: string, value: string): void {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    throw new Error(`${name}'s submission has an unreadable generated_at "${value}"`);
  }
  const canonical = `${parsed.toISOString().slice(0, 19)}Z`;
  if (canonical !== value) {
    throw new Error(
      `${name}'s submission is timestamped "${value}" and this ceremony writes "${canonical}" — UTC, whole ` +
        'seconds, trailing Z. Two spellings of one instant would give this page and the coordinator different ' +
        'group fingerprints from the same five submissions',
    );
  }
}

// checkHDPath refuses a key derived somewhere other than this chain's path.
//
// The path is inside the possession signature, so it cannot be altered in
// transit — but it is chosen by whoever generated the key, and it is what the
// record tells somebody to derive at years from now. A record naming a different
// coin type would send a custodian recovering the account to an address that is
// not in the group.
export function checkHDPath(name: string, path: string): void {
  const base = hdPath(0).replace(/\/0$/, '');
  if (!path.startsWith(`${base}/`)) {
    throw new Error(
      `${name}'s submission was derived at "${path}", and this chain's accounts live under ${base}. A key ` +
        'derived somewhere else is a key the recovery instructions on the record would not find',
    );
  }
}

function identityFromPubKey(name: string, pub: Uint8Array, path: string, generatedAt: string): Identity {
  return {
    name,
    role: 'custodian',
    address: bech32Address(ACCOUNT_PREFIX, addressBytes(pub)),
    pubkey: { '@type': SECP256K1_PUBKEY_TYPE, key: toBase64(pub) },
    fingerprint: fingerprintOf(pub),
    hd_path: path,
    generated_at: generatedAt,
  };
}

export type Assembled = {
  params: CeremonyParams;
  custodians: Identity[];
  policy_address: string;
  fingerprint: string;
  genesis: string;
  constitution: string;
  members: string;
  computed_at: string;
};

// assembleGroup is the pure function every instance runs.
//
// Nothing local reaches the output. In particular the timestamp inside the
// genesis fragment is the LATEST generated_at among the submissions rather than
// the current time — a value carried in the signed inputs, identical on all five
// devices. A custodian with a badly wrong clock skews it, and that shows up in
// the fingerprint everybody is about to read aloud, which is the correct place
// for it to show up.
export function assembleGroup(params: CeremonyParams, submissions: Submission[]): Assembled {
  validateParams(params);
  if (submissions.length !== params.custodians.length) {
    throw new Error(
      `the roster has ${params.custodians.length} custodians and ${submissions.length} submissions are present. ` +
        missingFrom(params, submissions),
    );
  }

  const custodians: Identity[] = [];
  let latest = '';
  for (const s of submissions) {
    const id = verifySubmission(params, s);
    custodians.push(id);
    // Compared as strings, which is safe ONLY because checkCanonicalTimestamp
    // has already refused anything that is not RFC 3339 in UTC with a Z and
    // second precision. For that one format lexical order is chronological
    // order; for any other spelling of the same instant it is not, and the
    // winner would differ from the one Go picks.
    if (id.generated_at > latest) latest = id.generated_at;
  }

  const documents = buildGroup(
    custodians,
    purposeFor(params),
    params.threshold,
    params.voting_period,
    params.policy_seq,
    latest,
  );
  const sorted = [...custodians].sort((a, b) => compareGoStrings(a.address, b.address));

  const assembled: Assembled = {
    params,
    custodians: sorted,
    policy_address: documents.policyAddress,
    fingerprint: '',
    genesis: documents.genesis,
    constitution: documents.constitution,
    members: documents.members,
    computed_at: latest,
  };
  assembled.fingerprint = longDigest(GROUP_DOMAIN, assembledCanonical(assembled));
  return assembled;
}

// assembledCanonical is what the group fingerprint is taken over.
//
// It includes the raw genesis fragment, not a summary of it. The value the
// custodians compare has to certify the bytes that will be spliced into the file
// that launches the chain; a fingerprint over the membership alone would agree
// across five devices that had each computed a different genesis.
export function assembledCanonical(a: Assembled): Uint8Array {
  const parts: Uint8Array[] = [
    canonField(GROUP_DOMAIN),
    canonBytes(paramsCanonical(a.params)),
    canonField(a.policy_address),
    canonField(a.computed_at),
    canonCount(a.custodians.length),
  ];
  for (const custodian of a.custodians) {
    parts.push(canonField(custodian.address), canonField(custodian.pubkey.key), canonField(custodian.name));
  }
  parts.push(canonBytes(utf8.encode(a.genesis)), canonBytes(utf8.encode(a.constitution)));
  return concatBytes(...parts);
}

// presence returns where a custodian's own address sits in the assembled group.
//
// This is the substitution check, and it is the one new risk a ceremony without a
// room introduces. Five people watching one screen WAS the verification. Remove
// the room and a custodian's own key could simply be left out of the group they
// are about to attest to, and nothing they had seen so far would say so.
export function presence(a: Assembled, address: string, fingerprint: string): void {
  for (let i = 0; i < a.custodians.length; i++) {
    const custodian = a.custodians[i] as Identity;
    if (custodian.address !== address) continue;
    if (custodian.fingerprint !== fingerprint) {
      throw new Error(
        `your address is in the group at position ${i + 1} but under fingerprint ${custodian.fingerprint}, ` +
          `and your key's fingerprint is ${fingerprint}. Those cannot both be true; stop and say so on the call`,
      );
    }
    return;
  }
  throw new Error(
    `YOUR KEY IS NOT IN THIS GROUP. ${address} does not appear among the ${a.custodians.length} custodians. ` +
      'Do not attest to it. Whoever assembled the material you were given left you out or replaced you, and the ' +
      'group they are proposing is not the one this ceremony agreed',
  );
}

// missingFrom names who has not been received.
//
// A relay can withhold or delay and that is tolerable, because it is visible —
// but only if the interface says which submissions are absent. A progress
// spinner would make a relay stalling one custodian indistinguishable from a
// slow connection.
export function missingFrom(params: CeremonyParams, submissions: Submission[]): string {
  const present = new Set(submissions.map((s) => s.identity.name.trim()));
  const missing = params.custodians.filter((name) => !present.has(name.trim()));
  if (missing.length === 0) return 'Every name on the roster has a submission.';
  return `Still missing: ${missing.join(', ')}.`;
}

type GroupDocuments = {
  policyAddress: string;
  // The two strings recorded on chain as what this group is, returned so that a
  // caller — and the cross-language suite — reads the values buildGroup actually
  // used rather than recomposing them and agreeing with itself.
  metadata: string;
  policyMetadata: string;
  genesis: string;
  // The empty string for a country office, and that is the value under test
  // rather than an omission. Go leaves its field nil; canonBytes has to turn both
  // into the same four zero bytes, or a country's super users and their
  // coordinator compute different group fingerprints from the same submissions.
  constitution: string;
  members: string;
};

// GroupPurpose is which of the two groups this ceremony is building.
//
// Two things differ between the foundation's 3-of-5 and a country office's
// M-of-N, and neither is cosmetic:
//
//   - The label, which goes into the group and policy metadata recorded on chain
//     permanently and is the one field a human reads to find out what a group is.
//
//   - The constitutional invariants fragment. For an office it is a
//     ready-to-splice JSON saying "send every seized asset on this chain to
//     Senegal's payments office", so it is not produced at all. Not produced
//     rather than produced-with-a-warning, because nobody should have to know not
//     to use a document the page handed them.
export type GroupPurpose = { label: string; office: boolean };

export function foundationPurpose(): GroupPurpose {
  return { label: FOUNDATION_LABEL, office: false };
}

export function purposeFor(params: CeremonyParams): GroupPurpose {
  return { label: groupLabel(params), office: Boolean(params.office) };
}

// groupMetadata is the string recorded on chain, permanently, as what this group
// is.
//
// The label is a parameter rather than a literal because it is the only thing a
// human reads to tell one group from another. For the foundation it is
// "Yamale foundation" and the bytes are unchanged from before the country path
// existed; for a country office it names the office and its perimeter.
export function groupMetadata(label: string, custodians: Identity[], threshold: number): string {
  const names = custodians.map((c) => `${c.name} ${c.fingerprint}`);
  return `${label}, ${threshold} of ${custodians.length}: ${names.join('; ')}`;
}

// buildGroup turns the custodians' public records into the documents that create
// the group.
//
// The admin is the group policy itself. That is the point of the whole design: an
// admin outside the group would be a single key that could rewrite the
// membership, which is the single key this ceremony exists to abolish.
export function buildGroup(
  input: Identity[],
  purpose: GroupPurpose,
  threshold: number,
  votingPeriod: string,
  seq: number,
  createdAt: string,
): GroupDocuments {
  if (input.length < threshold) {
    throw new Error(`threshold ${threshold} cannot be met by ${input.length} custodians: this group could never act`);
  }
  if (threshold < 2) {
    throw new Error(`threshold ${threshold} means one custodian acts alone, which is the single key this ceremony replaces`);
  }
  // A threshold equal to the membership has no redundancy at all: one lost key
  // and the foundation account is frozen permanently with the chain still
  // sending seized assets to it. Refused rather than warned about, because it
  // looks like the safest choice and is the least safe one available.
  if (threshold === input.length) {
    throw new Error(
      `threshold ${threshold} of ${input.length} requires every custodian: losing one key would freeze the ` +
        'foundation account forever, with the chain still sending seizures to it',
    );
  }

  // Sorted on a copy, by address, so the documents depend on the set of
  // custodians and not on the order submissions happened to arrive in. That is
  // what lets a second person rebuild the genesis fragment from the same five
  // submissions and compare it byte for byte, rather than taking one run on
  // trust.
  const custodians = [...input].sort((a, b) => compareGoStrings(a.address, b.address));

  const seen = new Map<string, string>();
  for (const custodian of custodians) {
    if (custodian.role !== 'custodian') {
      throw new Error(
        `${custodian.name} is recorded as "${custodian.role}", not a custodian; ` +
          'a validator operator key does not belong in the foundation group',
      );
    }
    const decoded = decodeBech32(custodian.address);
    if (decoded.prefix !== ACCOUNT_PREFIX) {
      throw new Error(`${custodian.name}'s address is for "${decoded.prefix}", not this chain`);
    }
    // Twenty bytes is a key; thirty-two is something derived — a module account
    // or another group's policy. A custodian has to be a person holding a key,
    // because a group policy sitting in this group could submit a proposal to
    // it, and the messages a proposal executes never pass the ante chain.
    if (decoded.bytes.length !== 20) {
      throw new Error(
        `${custodian.name}'s address is ${decoded.bytes.length} bytes, so it is a derived account rather than a ` +
          'key. Custodians are people with keys; a module or group account here would be a member nobody signs for',
      );
    }
    const other = seen.get(custodian.address);
    if (other !== undefined) {
      throw new Error(
        `${other} and ${custodian.name} have the same address ${custodian.address} — one of them holds two votes, ` +
          'and this is not the group you think it is',
      );
    }
    seen.set(custodian.address, custodian.name);
  }

  const policyAddr = policyAddress(seq);
  const metadata = groupMetadata(purpose.label, custodians, threshold);
  const policyMetadata = `${purpose.label} ${threshold}-of-${custodians.length}`;
  // Refused here rather than discovered by the transaction that creates the
  // group. See MAX_GROUP_METADATA: the genesis path does not check this, so the
  // foundation would never notice, and an office's create-group transaction would
  // fail after the ceremony was over.
  for (const [what, value] of [
    ['group metadata', metadata],
    ['group policy metadata', policyMetadata],
  ] as const) {
    const size = utf8Length(value);
    if (size > MAX_GROUP_METADATA) {
      throw new Error(
        `the ${what} is ${size} bytes and x/group refuses anything over ${MAX_GROUP_METADATA}, so the ` +
          'transaction that creates this group would fail after the ceremony was over. Shorten the office ' +
          `name: it is the part of "${purpose.label}" that this ceremony chose`,
      );
    }
  }
  const created = goJSONString(createdAt);
  const period = protoDuration(parseGoDuration(votingPeriod));

  const decisionPolicy = goJSONObject([
    ['@type', goJSONString(THRESHOLD_POLICY_TYPE)],
    ['threshold', goJSONString(String(threshold))],
    [
      'windows',
      goJSONObject([
        ['voting_period', goJSONString(period)],
        // Zero, always. The delay that protects anybody here is
        // x/enforcement's, already fixed by the constitution; a second delay
        // stacked on top would only mean the foundation cannot act on an
        // outcome the chain has already reached.
        ['min_execution_period', goJSONString('0s')],
      ]),
    ],
  ]);

  const genesis = goJSONObject([
    ['group_seq', goJSONString(String(GROUP_ID))],
    [
      'groups',
      goJSONArray([
        goJSONObject([
          ['id', goJSONString(String(GROUP_ID))],
          ['admin', goJSONString(policyAddr)],
          ['metadata', goJSONString(metadata)],
          ['version', goJSONString('1')],
          ['total_weight', goJSONString(String(custodians.length))],
          ['created_at', created],
        ]),
      ]),
    ],
    [
      'group_members',
      goJSONArray(
        custodians.map((custodian) =>
          goJSONObject([
            ['group_id', goJSONString(String(GROUP_ID))],
            [
              'member',
              goJSONObject([
                ['address', goJSONString(custodian.address)],
                // Equal weight, always. Weighting custodians would make some
                // signatures worth more than others, and the reason five people
                // hold this is that no one of them is more trusted than the rest.
                ['weight', goJSONString('1')],
                ['metadata', goJSONString(`${custodian.name} (${custodian.fingerprint})`)],
                ['added_at', created],
              ]),
            ],
          ]),
        ),
      ),
    ],
    ['group_policy_seq', goJSONString(String(seq))],
    [
      'group_policies',
      goJSONArray([
        goJSONObject([
          ['address', goJSONString(policyAddr)],
          ['group_id', goJSONString(String(GROUP_ID))],
          ['admin', goJSONString(policyAddr)],
          ['metadata', goJSONString(metadata)],
          ['version', goJSONString('1')],
          ['decision_policy', decisionPolicy],
          ['created_at', created],
        ]),
      ]),
    ],
    // Emitted even though they are empty, because protobuf JSON in the SDK
    // emits defaults and the fingerprint covers these bytes. An omitted
    // "votes":[] is a different genesis fragment as far as the digest is
    // concerned.
    ['proposal_seq', goJSONString('0')],
    ['proposals', '[]'],
    ['votes', '[]'],
  ]);

  // Withheld for a country office, and left as the empty string so that
  // canonBytes(utf8.encode('')) produces the same four zero bytes Go's
  // canonBytes(nil) produces. The three fields in it are the FOUNDATION's: where
  // the chain sends every seizure, how many custodians the foundation has, and how
  // many must sign. Produced for a country office it reads as an instruction to
  // hand that office the whole chain's seized assets, and a genesis built from it
  // would start perfectly happily.
  const constitution = purpose.office
    ? ''
    : indentGoJSON([
        ['enforcement_recovery_destination', goJSONString(policyAddr)],
        ['foundation_custodian_count', String(custodians.length)],
        ['foundation_signature_threshold', String(threshold)],
      ]);

  const members = membersDocument(custodians);

  return { policyAddress: policyAddr, metadata, policyMetadata, genesis, constitution, members };
}

// membersDocument is the file `blockchaind tx group create-group-with-policy`
// reads, for a chain that is already running rather than one being launched.
//
// Shown on the coordinator's screen with a copy action instead of written to a
// directory somebody has to go and find. It is not covered by the group
// fingerprint — the genesis fragment is what a launch uses — so it is formatted
// for a human to read.
function membersDocument(custodians: Identity[]): string {
  const rows = custodians.map(
    (custodian) =>
      '    {\n' +
      `      "address": ${goJSONString(custodian.address)},\n` +
      '      "weight": "1",\n' +
      `      "metadata": ${goJSONString(`${custodian.name} (${custodian.fingerprint})`)}\n` +
      '    }',
  );
  return `{\n  "members": [\n${rows.join(',\n')}\n  ]\n}`;
}
