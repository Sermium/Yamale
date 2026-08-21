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
import { ACCOUNT_PREFIX, addressBytes, bech32Address, decodeBech32, fingerprintOf, verify } from './key.ts';
import {
  GROUP_DOMAIN,
  SECP256K1_PUBKEY_TYPE,
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
  parseGoDuration(p.voting_period);
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
    // Compared as strings, which is safe because every generated_at is
    // RFC3339 in UTC with a Z and second precision: lexical order is
    // chronological order, and no locale or timezone can move it.
    if (id.generated_at > latest) latest = id.generated_at;
  }

  const documents = buildGroup(custodians, params.threshold, params.voting_period, params.policy_seq, latest);
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
  genesis: string;
  constitution: string;
  members: string;
};

function groupMetadata(custodians: Identity[], threshold: number): string {
  const names = custodians.map((c) => `${c.name} ${c.fingerprint}`);
  return `Yamale foundation, ${threshold} of ${custodians.length}: ${names.join('; ')}`;
}

// buildGroup turns the custodians' public records into the documents that create
// the group.
//
// The admin is the group policy itself. That is the point of the whole design: an
// admin outside the group would be a single key that could rewrite the
// membership, which is the single key this ceremony exists to abolish.
export function buildGroup(
  input: Identity[],
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
  const metadata = groupMetadata(custodians, threshold);
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

  const constitution = indentGoJSON([
    ['enforcement_recovery_destination', goJSONString(policyAddr)],
    ['foundation_custodian_count', String(custodians.length)],
    ['foundation_signature_threshold', String(threshold)],
  ]);

  const members = membersDocument(custodians);

  return { policyAddress: policyAddr, genesis, constitution, members };
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
