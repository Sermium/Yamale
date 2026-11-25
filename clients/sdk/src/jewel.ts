/**
 * The jewel register, as the app speaks to it.
 *
 * Three formats here are also implemented in Go, in x/jewel/types: the
 * inscription normaliser, the claim sign bytes and the voucher sign bytes.
 * They are checked against the same file,
 * x/jewel/types/testdata/vectors.json, so the browser and the chain cannot
 * drift apart without a test saying so.
 *
 * Every id and every time is a decimal string, because that is how the
 * generated types carry uint64 and int64.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { ripemd160 } from '@noble/hashes/legacy';
import { sha256 } from '@noble/hashes/sha2';
import { toBech32 } from '@cosmjs/encoding';

import { AllowedMsgAllowance, BasicAllowance } from './generated/cosmos/feegrant/v1beta1/feegrant.ts';
import type { Coin } from './denom.ts';
import type { EncodeObject } from './signing.ts';

const T = '/blockchain.jewel.v1.';
const utf8 = new TextEncoder();
const DECIMAL = /^(0|[1-9][0-9]*)$/;
const ID = /^[1-9][0-9]*$/;

/* ------------------------------------------------------------- encodings */

export function toHex(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += x.toString(16).padStart(2, '0');
  return s;
}

export function fromHex(h: string): Uint8Array {
  if (!/^([0-9a-fA-F]{2})*$/.test(h)) throw new Error('not hex');
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(2 * i, 2 * i + 2), 16);
  return out;
}

export function toBase64(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

export function toBase64Url(b: Uint8Array): string {
  return toBase64(b).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function fromBase64Url(s: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]*$/.test(s)) throw new Error('not base64url');
  const padded = s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4);
  const bin = atob(padded);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/* ----------------------------------------------------------- inscriptions */

/**
 * What the chain stores for an engraved code: upper case, with spaces and the
 * separators - . / _ removed, 4 to 40 characters. Returns null where the chain
 * would refuse.
 *
 * Upper-cased one code point at a time, and a code point whose upper case is
 * more than one character is refused. That is what Go's strings.ToUpper does
 * by never applying multi-character mappings: "ß" stays "ß" there and is
 * refused, where a whole-string JavaScript toUpperCase would turn it into
 * "SS" and accept it.
 */
export function normaliseInscription(raw: string): string | null {
  let out = '';
  for (const ch of raw) {
    const up = ch.toUpperCase();
    if (up.length === 1 && /^[A-Z0-9]$/.test(up)) {
      out += up;
    } else if (ch === ' ' || ch === '-' || ch === '.' || ch === '/' || ch === '_') {
      continue;
    } else {
      return null;
    }
  }
  return out.length >= 4 && out.length <= 40 ? out : null;
}

/* ------------------------------------------------------------- sign bytes */

export function claimSignBytes(chainId: string, jewelId: string, offerExpiresAt: string, claimer: string): Uint8Array {
  if (!ID.test(jewelId)) throw new Error(`jewel id ${jewelId}`);
  if (!DECIMAL.test(offerExpiresAt)) throw new Error(`offer expiry ${offerExpiresAt}`);
  return utf8.encode(`yamale-jewel-claim/v2:${chainId}:${jewelId}:${offerExpiresAt}:${claimer}`);
}

/**
 * What an ownership card's key signs to file a claim. It names the card, so a
 * claim made with a reprinted (cancelled) card fails, and the claimer, so a
 * claim copied from the network cannot be refiled for anybody else.
 */
export function cardClaimSignBytes(chainId: string, jewelId: string, cardKey: string, claimer: string): Uint8Array {
  if (!ID.test(jewelId)) throw new Error(`jewel id ${jewelId}`);
  if (!/^0[23][0-9a-f]{64}$/.test(cardKey)) throw new Error('card key must be a lower-case compressed public key');
  return utf8.encode(`yamale-jewel-card/v1:${chainId}:${jewelId}:${cardKey}:${claimer}`);
}

export interface VoucherFields { benefitId: string; jewelId: string; nonce: string; expiresAt: string }

export function voucherSignBytes(chainId: string, v: VoucherFields): Uint8Array {
  if (!ID.test(v.benefitId) || !ID.test(v.jewelId) || !DECIMAL.test(v.expiresAt)) {
    throw new Error('voucher fields must be decimal');
  }
  return utf8.encode(
    `yamale-jewel-voucher/v1:${chainId}:${v.benefitId}:${v.jewelId}:${v.nonce}:${v.expiresAt}`);
}

/** Not on the chain: the app alone reads it, to show who shared a link. */
export function ownershipProofBytes(chainId: string, jewelId: string, issuedAt: string): Uint8Array {
  return utf8.encode(`yamale-jewel-owner/v1:${chainId}:${jewelId}:${issuedAt}`);
}

/* ------------------------------------------------------------------- keys */

/** A Cosmos secp256k1 signature: SHA-256 of the message, low-S, r‖s. */
export function signArbitrary(privateKey: Uint8Array, message: Uint8Array): Uint8Array {
  return secp256k1.sign(sha256(message), privateKey, { lowS: true }).toCompactRawBytes();
}

export function verifyArbitrary(pubKey: Uint8Array, message: Uint8Array, signature: Uint8Array): boolean {
  if (pubKey.length !== 33 || signature.length !== 64) return false;
  try {
    return secp256k1.verify(signature, sha256(message), pubKey, { lowS: true });
  } catch {
    return false;
  }
}

export function compressedPublicKey(privateKey: Uint8Array): Uint8Array {
  return secp256k1.getPublicKey(privateKey, true);
}

export function addressOfPublicKey(pubKey: Uint8Array, prefix = 'yml'): string {
  return toBech32(prefix, ripemd160(sha256(pubKey)));
}

/* ------------------------------------------------------------ claim links */

export function newClaimKey(): { privateKey: Uint8Array; claimKey: string } {
  const privateKey = secp256k1.utils.randomPrivateKey();
  return { privateKey, claimKey: toHex(compressedPublicKey(privateKey)) };
}

/**
 * The key goes in the fragment. A browser sends the path and the query to the
 * server and keeps the fragment to itself, so the private half reaches no
 * server, ours included.
 */
export function claimLink(origin: string, jewelId: string, privateKey: Uint8Array): string {
  return `${origin.replace(/\/+$/, '')}/claim#${jewelId}.${toHex(privateKey)}`;
}

export function parseClaimFragment(fragment: string): { jewelId: string; privateKey: Uint8Array } | null {
  const m = /^#?([1-9][0-9]*)\.([0-9a-fA-F]{64})$/.exec(fragment);
  if (!m) return null;
  const privateKey = fromHex(m[2]!.toLowerCase());
  // Zero, or at or above the curve order, is no key: refuse it here rather than throw while rendering.
  if (!secp256k1.utils.isValidPrivateKey(privateKey)) return null;
  return { jewelId: m[1]!, privateKey };
}

/* ------------------------------------------------------- ownership cards */

/**
 * The QR printed on an ownership card. The secret sits in the fragment, which
 * a browser never sends to a server, as in a claim link.
 */
export function cardLink(origin: string, jewelId: string, privateKey: Uint8Array): string {
  return `${origin.replace(/\/+$/, '')}/card#${jewelId}.${toHex(privateKey)}`;
}

export function parseCardFragment(fragment: string): { jewelId: string; privateKey: Uint8Array } | null {
  const m = /^#?([1-9][0-9]{0,19})\.([0-9a-fA-F]{64})$/.exec(fragment.trim());
  if (!m) return null;
  const privateKey = fromHex(m[2]!.toLowerCase());
  if (!secp256k1.utils.isValidPrivateKey(privateKey)) return null;
  return { jewelId: m[1]!, privateKey };
}

/* --------------------------------------------------------------- vouchers */

export interface VoucherCode { voucher: VoucherFields; pubKey: Uint8Array; signature: Uint8Array }

const defaultRandom = (n: number) => crypto.getRandomValues(new Uint8Array(n));

/**
 * The owner's consent to one use of a benefit.
 *
 * chainNow is the latest block time, in unix seconds, not the phone's clock.
 * The chain refuses a voucher that expires more than 300 s after the block
 * that redeems it, and a phone two minutes fast would produce exactly that.
 * 240 s leaves the staff a minute to scan.
 */
export function makeVoucher(
  privateKey: Uint8Array, chainId: string, benefitId: string, jewelId: string,
  chainNow: number, ttlSeconds = 240, random: (n: number) => Uint8Array = defaultRandom,
): VoucherCode {
  const voucher: VoucherFields = {
    benefitId, jewelId, nonce: toHex(random(16)), expiresAt: String(Math.floor(chainNow) + ttlSeconds),
  };
  return {
    voucher,
    pubKey: compressedPublicKey(privateKey),
    signature: signArbitrary(privateKey, voucherSignBytes(chainId, voucher)),
  };
}

/** YJV1.<benefit>.<jewel>.<nonce>.<expires>.<pub key>.<signature>, keys base64url. */
export function encodeVoucherCode(code: VoucherCode): string {
  const v = code.voucher;
  return ['YJV1', v.benefitId, v.jewelId, v.nonce, v.expiresAt,
    toBase64Url(code.pubKey), toBase64Url(code.signature)].join('.');
}

export function decodeVoucherCode(text: string): VoucherCode | null {
  const parts = text.trim().split('.');
  if (parts.length !== 7 || parts[0] !== 'YJV1') return null;
  const [, benefitId, jewelId, nonce, expiresAt, pub, sig] = parts as [string, ...string[]];
  if (!ID.test(benefitId!) || !ID.test(jewelId!) || !DECIMAL.test(expiresAt!)) return null;
  if (!/^[0-9a-f]{1,64}$/.test(nonce!)) return null;
  try {
    const pubKey = fromBase64Url(pub!);
    const signature = fromBase64Url(sig!);
    if (pubKey.length !== 33 || signature.length !== 64) return null;
    return { voucher: { benefitId: benefitId!, jewelId: jewelId!, nonce: nonce!, expiresAt: expiresAt! }, pubKey, signature };
  } catch {
    return null;
  }
}

/** The account that signed, if the signature holds for this chain. */
export function voucherSigner(code: VoucherCode, chainId: string): string | null {
  return verifyArbitrary(code.pubKey, voucherSignBytes(chainId, code.voucher), code.signature)
    ? addressOfPublicKey(code.pubKey) : null;
}

/* ------------------------------------------------------ ownership proofs */

/** The query string of /own/<jewel id>?t=…&k=…&s=… */
export function makeOwnershipProof(privateKey: Uint8Array, chainId: string, jewelId: string, issuedAt: number): string {
  const t = String(Math.floor(issuedAt));
  const sig = signArbitrary(privateKey, ownershipProofBytes(chainId, jewelId, t));
  return `t=${t}&k=${toBase64Url(compressedPublicKey(privateKey))}&s=${toBase64Url(sig)}`;
}

export function verifyOwnershipProof(chainId: string, jewelId: string, query: string): { address: string; issuedAt: number } | null {
  const p = new URLSearchParams(query.startsWith('?') ? query.slice(1) : query);
  const t = p.get('t') ?? '';
  if (!DECIMAL.test(t)) return null;
  try {
    const pub = fromBase64Url(p.get('k') ?? '');
    const sig = fromBase64Url(p.get('s') ?? '');
    if (!verifyArbitrary(pub, ownershipProofBytes(chainId, jewelId, t), sig)) return null;
    return { address: addressOfPublicKey(pub), issuedAt: Number(t) };
  } catch {
    return null;
  }
}

/* --------------------------------------------------------------- messages */

/**
 * Enough for the heaviest jewel message, an amendment at its 50-entry cap with a
 * 500-byte note: amendment 50 of 50 measured at 1,247,310 gas on 2026-11-13
 * (scripts/devnet/measure-amend-gas.sh); 30% headroom, rounded up to 50,000.
 * The gas price is zero on devnet-2, so the limit costs nothing until it is not.
 */
export const JEWEL_GAS = 1_650_000;

/** What a jeweler's business pays for when its staff sign. */
export const STAFF_MESSAGES: readonly string[] = [
  'MsgRegisterJewel', 'MsgAmendJewel', 'MsgOfferJewel', 'MsgCancelOffer',
  'MsgAttachBenefit', 'MsgRedeemBenefit', 'MsgIssueCard', 'MsgApproveClaim', 'MsgRejectClaim',
].map((m) => T + m);

const msg = (name: string, value: Record<string, unknown>): EncodeObject => ({ typeUrl: T + name, value });

export const registerJewel = (staff: string, inscription: string, recordHash: string, recordUri: string) =>
  msg('MsgRegisterJewel', { staff, inscription, recordHash, recordUri });
export const amendJewel = (staff: string, jewelId: string, recordHash: string, recordUri: string, note: string) =>
  msg('MsgAmendJewel', { staff, jewelId, recordHash, recordUri, note });
export const offerJewelToAccount = (owner: string, jewelId: string, to: string) =>
  msg('MsgOfferJewel', { owner, jewelId, to, claimKey: '' });
export const offerJewelByLink = (owner: string, jewelId: string, claimKey: string) =>
  msg('MsgOfferJewel', { owner, jewelId, to: '', claimKey });
export const cancelJewelOffer = (owner: string, jewelId: string) => msg('MsgCancelOffer', { owner, jewelId });
export const acceptJewelOffer = (recipient: string, jewelId: string) => msg('MsgAcceptOffer', { recipient, jewelId });
export const reportJewelStolen = (owner: string, jewelId: string) => msg('MsgReportStolen', { owner, jewelId });
export const clearJewelStolen = (owner: string, jewelId: string) => msg('MsgClearStolen', { owner, jewelId });
export const vetoJewelReassignment = (owner: string, jewelId: string) => msg('MsgVetoReassignment', { owner, jewelId });
export const completeJewelReassignment = (creator: string, jewelId: string) =>
  msg('MsgCompleteReassignment', { creator, jewelId });
export const setJewelStaff = (business: string, jewelerId: string, add: string[], remove: string[]) =>
  msg('MsgSetStaff', { business, jewelerId, add, remove });

export function attachJewelBenefit(staff: string, jewelId: string, kind: string, allowance: number,
  periodSeconds: number, expiresAt: number): EncodeObject {
  return msg('MsgAttachBenefit', {
    staff, jewelId, kind, allowance, period: String(periodSeconds), expiresAt: String(expiresAt),
  });
}

/** `offerExpiresAt` is the open offer's expires_at as the chain holds it: the signature is good for that offer only. */
export function claimJewelOffer(
  claimer: string, jewelId: string, offerExpiresAt: string, claimPrivateKey: Uint8Array, chainId: string,
): EncodeObject {
  return msg('MsgClaimOffer', {
    claimer, jewelId,
    claimPubKey: compressedPublicKey(claimPrivateKey),
    claimSignature: signArbitrary(claimPrivateKey, claimSignBytes(chainId, jewelId, offerExpiresAt, claimer)),
  });
}

/** Issues a card for stock, replacing any earlier one. `cardKey` is its public key in lower-case hex. */
export const issueJewelCard = (staff: string, jewelId: string, cardKey: string) =>
  msg('MsgIssueCard', { staff, jewelId, cardKey });
export const approveJewelClaim = (staff: string, jewelId: string) => msg('MsgApproveClaim', { staff, jewelId });
export const rejectJewelClaim = (staff: string, jewelId: string) => msg('MsgRejectClaim', { staff, jewelId });
export const withdrawJewelClaim = (claimer: string, jewelId: string) => msg('MsgWithdrawClaim', { claimer, jewelId });
/** The business account names the account the shop moves to; an empty newBusiness withdraws the proposal. */
export const proposeBusinessMove = (business: string, jewelerId: string, newBusiness: string) =>
  msg('MsgProposeBusinessMove', { business, jewelerId, newBusiness });
/** The named account accepts: it becomes the business account and receives the stock. */
export const acceptBusinessMove = (newBusiness: string, jewelerId: string) =>
  msg('MsgAcceptBusinessMove', { newBusiness, jewelerId });

/** Files a claim with the card's secret, which signs the claimer's address and never leaves this device. */
export function fileJewelClaim(claimer: string, jewelId: string, cardPrivateKey: Uint8Array, chainId: string): EncodeObject {
  const pub = compressedPublicKey(cardPrivateKey);
  return msg('MsgFileClaim', {
    claimer, jewelId,
    cardPubKey: pub,
    cardSignature: signArbitrary(cardPrivateKey, cardClaimSignBytes(chainId, jewelId, toHex(pub), claimer)),
  });
}

export function redeemJewelBenefit(staff: string, code: VoucherCode): EncodeObject {
  return msg('MsgRedeemBenefit', {
    staff, voucher: code.voucher, ownerPubKey: code.pubKey, ownerSignature: code.signature,
  });
}

/** The business pays its staff's fees, for staff work only. */
export function grantStaffFees(business: string, staff: string, spendLimit: Coin[], expiresAt: Date): EncodeObject {
  const basic = BasicAllowance.encode(BasicAllowance.fromPartial({ spendLimit, expiration: expiresAt })).finish();
  const allowed = AllowedMsgAllowance.encode(AllowedMsgAllowance.fromPartial({
    allowance: { typeUrl: '/cosmos.feegrant.v1beta1.BasicAllowance', value: basic },
    allowedMessages: [...STAFF_MESSAGES],
  })).finish();
  return {
    typeUrl: '/cosmos.feegrant.v1beta1.MsgGrantAllowance',
    value: {
      granter: business, grantee: staff,
      allowance: { typeUrl: '/cosmos.feegrant.v1beta1.AllowedMsgAllowance', value: allowed },
    },
  };
}
