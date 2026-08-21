// One custodian's key, generated in their own browser and never leaving it.
//
// This is the file the whole hosted ceremony rests on. A coordinator serves the
// page; the page generates the phrase here, derives the key here, signs here,
// and posts back a public key and two signatures. Nothing in this module has a
// path to the network, and nothing anywhere in this bundle sends a phrase or a
// private key: see src/storage.test.ts, which asserts that no request body the
// client builds contains either.
//
// The derivation has to match tools/ceremony/key.go exactly. A page deriving
// even slightly differently would hand five custodians addresses that look right
// and control nothing, and the failure would not surface until somebody tried to
// spend from the account that holds every seized asset on the chain. That is why
// nothing here is written from first principles: BIP-39, BIP-32 and secp256k1
// are @scure and @noble, and the outputs are pinned against the Go binary by
// testdata/vectors/ceremony.json.

import { HDKey } from '@scure/bip32';
import { generateMnemonic, mnemonicToSeedSync, validateMnemonic } from '@scure/bip39';
import { wordlist } from '@scure/bip39/wordlists/english';
import { secp256k1 } from '@noble/curves/secp256k1.js';
import { sha256 } from '@noble/hashes/sha2.js';
import { ripemd160 } from '@noble/hashes/legacy.js';
import { bech32 } from '@scure/base';
import {
  FINGERPRINT_DOMAIN,
  SECP256K1_PUBKEY_TYPE,
  possessionMessage,
  shortDigest,
  toBase64,
  type Identity,
  type Submission,
} from './wire.ts';

// ACCOUNT_PREFIX and COIN_TYPE are what make a key a Yamale key. Restated here
// rather than fetched from the coordinator: a page that asked the server which
// prefix to use would let a server that answered "cosmos" hand five custodians
// keys for a different chain, and every check afterwards would agree because
// they would all read the same answer.
export const ACCOUNT_PREFIX = 'yml';
export const COIN_TYPE = 118;

// ENTROPY_BITS is 256, so twenty-four words. Twelve is legal and this page does
// not offer it: the extra sixty-four bits cost one line on a sheet of paper that
// will be written once and read almost never, and the choice is not one a
// custodian should be reasoning about while four other people wait.
export const ENTROPY_BITS = 256;

// BECH32_LIMIT is above the default 90 because a group policy address is
// thirty-two bytes and encodes to sixty-two characters. The default would refuse
// the one address this ceremony exists to produce.
const BECH32_LIMIT = 256;

export function hdPath(index: number): string {
  return `m/44'/${COIN_TYPE}'/0'/0/${index}`;
}

export function newPhrase(): string {
  return generateMnemonic(wordlist, ENTROPY_BITS);
}

// checkPhrase validates the checksum, not just the words.
//
// This is the difference the restore drill turns on. A check that only confirmed
// each word is in the wordlist would pass every realistic transcription error:
// somebody copying by hand writes another real BIP-39 word, so the count is
// right, every word is in the list, and the phrase derives a completely
// different, empty account. The last four bits of a 24-word phrase exist to
// catch exactly that.
export function checkPhrase(phrase: string): boolean {
  return validateMnemonic(normalisePhrase(phrase), wordlist);
}

export function normalisePhrase(phrase: string): string {
  return phrase.trim().toLowerCase().split(/\s+/).filter(Boolean).join(' ');
}

export type KeyPair = {
  priv: Uint8Array;
  pub: Uint8Array;
  address: string;
  fingerprint: string;
  path: string;
};

export function deriveKey(phrase: string, index: number): KeyPair {
  const path = hdPath(index);
  // No BIP-39 passphrase. The chain's own derivation uses an empty one, and a
  // ceremony that offered a passphrase would be a ceremony where a custodian
  // holds a sheet that does not, on its own, recover the key.
  const seed = mnemonicToSeedSync(normalisePhrase(phrase), '');
  const node = HDKey.fromMasterSeed(seed).derive(path);
  if (!node.privateKey || !node.publicKey) {
    throw new Error('the derivation produced no key');
  }
  const priv = node.privateKey;
  const pub = node.publicKey;
  return {
    priv,
    pub,
    address: bech32Address(ACCOUNT_PREFIX, addressBytes(pub)),
    fingerprint: fingerprintOf(pub),
    path,
  };
}

// addressBytes is the Cosmos SDK's secp256k1 account address: RIPEMD-160 of
// SHA-256 of the compressed public key.
export function addressBytes(pub: Uint8Array): Uint8Array {
  return ripemd160(sha256(pub));
}

export function bech32Address(prefix: string, raw: Uint8Array): string {
  return bech32.encode(prefix, bech32.toWords(raw), BECH32_LIMIT);
}

export function decodeBech32(address: string): { prefix: string; bytes: Uint8Array } {
  const decoded = bech32.decode(address as `${string}1${string}`, BECH32_LIMIT);
  return { prefix: decoded.prefix, bytes: Uint8Array.from(bech32.fromWords(decoded.words)) };
}

// fingerprintOf is the short string a custodian writes on their own sheet.
//
// It is what makes a swapped or mis-filed envelope detectable years later: an
// envelope labelled "custodian 3" either recovers to a key whose fingerprint
// matches the one on the record or it does not, and that check needs no network
// and nothing anybody has to be trusted about.
export function fingerprintOf(pub: Uint8Array): string {
  return shortDigest(FINGERPRINT_DOMAIN, pub);
}

export function identityOf(name: string, key: KeyPair, generatedAt: Date): Identity {
  return {
    name,
    role: 'custodian',
    address: key.address,
    pubkey: { '@type': SECP256K1_PUBKEY_TYPE, key: toBase64(key.pub) },
    fingerprint: key.fingerprint,
    hd_path: key.path,
    generated_at: rfc3339Second(generatedAt),
  };
}

// rfc3339Second is the timestamp format the Go side writes and signs.
//
// Truncated to the second, in UTC, with a Z rather than an offset. The latest of
// the five generated_at values ends up inside the genesis fragment, which the
// group fingerprint covers, so a browser emitting milliseconds would give every
// custodian a different fingerprint from the same five submissions.
export function rfc3339Second(when: Date): string {
  return `${when.toISOString().slice(0, 19)}Z`;
}

// sign produces a Cosmos secp256k1 signature: 64 bytes of r||s, low-S
// normalised, over the SHA-256 of the message.
//
// Low-S matters because the chain's verifier rejects the high-S form of the same
// signature. A page that emitted it would produce submissions that verified
// nowhere, roughly half the time, which is the worst possible failure rate: too
// often to be a fluke, rarely enough to look like a network problem.
export function sign(message: Uint8Array, priv: Uint8Array): Uint8Array {
  return secp256k1.sign(sha256(message), priv, { lowS: true }).toCompactRawBytes();
}

export function verify(signature: Uint8Array, message: Uint8Array, pub: Uint8Array): boolean {
  try {
    return secp256k1.verify(signature, sha256(message), pub, { lowS: true });
  } catch {
    return false;
  }
}

export function signSubmission(ceremonyID: string, id: Identity, priv: Uint8Array): Submission {
  return {
    ceremony_id: ceremonyID,
    identity: id,
    possession: toBase64(sign(possessionMessage(ceremonyID, id), priv)),
  };
}

// zero overwrites a key buffer.
//
// Worth stating what this does and does not achieve, because the page says so
// too. It clears the one copy this code holds. It cannot clear a copy the
// JavaScript engine made — strings are immutable and the garbage collector
// decides when a page is reused — and no code in a browser can. The mitigation
// for that is the browser being closed, which is why the last screen says to
// close the tab rather than implying the page has cleaned up after itself.
export function zero(buf: Uint8Array): void {
  buf.fill(0);
}
