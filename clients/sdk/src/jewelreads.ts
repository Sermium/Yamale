/**
 * Every read the traceability app makes of the jewel register.
 *
 * Chain state answers all of them but one: "which offers are addressed to
 * me" is not an index the module keeps, so it comes from tx_search on
 * EventOfferMade and is then checked against the jewel as it stands. An
 * event says an offer was made; only the jewel says it is still open.
 */

import { abciQuery, decoded, txSearch, type Outcome } from './abci.ts';
import { JewelerStatus, type Benefit, type Claim, type Jewel, type Jeweler, type Offer, type Reassignment } from './generated/blockchain/jewel/v1/jewel.ts';
import type { Params } from './generated/blockchain/jewel/v1/params.ts';
import {
  QueryClaimsByJewelerRequest, QueryClaimsByJewelerResponse,
  QueryJewelRequest, QueryJewelResponse, QueryJewelerRequest, QueryJewelerResponse,
  QueryJewelersRequest, QueryJewelersResponse, QueryJewelsByInscriptionRequest,
  QueryJewelsByInscriptionResponse, QueryParamsRequest, QueryParamsResponse,
} from './generated/blockchain/jewel/v1/query.ts';
import { QueryNFTsRequest, QueryNFTsResponse } from './generated/cosmos/nft/v1beta1/query.ts';
import { QueryAllowanceRequest, QueryAllowanceResponse } from './generated/cosmos/feegrant/v1beta1/query.ts';

const SVC = '/blockchain.jewel.v1.Query';

export interface JewelView {
  jewel: Jewel;
  owner: string;
  offer?: Offer;
  reassignment?: Reassignment;
  benefits: Benefit[];
  /** Whether an ownership card is issued for it. The card's key is not needed here. */
  hasCard: boolean;
  /** The pending claim on its card, if any. */
  claim?: Claim;
}

export async function readJewel(id: string): Promise<Outcome<JewelView>> {
  const out = decoded(
    await abciQuery(`${SVC}/Jewel`, QueryJewelRequest.encode(QueryJewelRequest.fromPartial({ id })).finish()),
    (b) => QueryJewelResponse.decode(b));
  if (!out.ok) return out;
  const r = out.value;
  if (!r.jewel) return { ok: false, reason: 'not-found', detail: `jewel ${id}` };
  return {
    ok: true, height: out.height,
    value: {
      jewel: r.jewel, owner: r.owner, offer: r.offer, reassignment: r.reassignment, benefits: r.benefits,
      hasCard: r.hasCard, claim: r.claim,
    },
  };
}

export async function readJeweler(id: string): Promise<Outcome<Jeweler>> {
  const out = decoded(
    await abciQuery(`${SVC}/Jeweler`, QueryJewelerRequest.encode(QueryJewelerRequest.fromPartial({ id })).finish()),
    (b) => QueryJewelerResponse.decode(b));
  if (!out.ok) return out;
  if (!out.value.jeweler) return { ok: false, reason: 'not-found', detail: `jeweler ${id}` };
  return { ok: true, height: out.height, value: out.value.jeweler };
}

export async function readJewelers(): Promise<Outcome<Jeweler[]>> {
  const all: Jeweler[] = [];
  let key: Uint8Array<ArrayBuffer> = new Uint8Array(0);
  let height = 0;
  for (let page = 0; page < 50; page++) {
    const req = QueryJewelersRequest.fromPartial({ pagination: { key, limit: '100' } });
    const out = decoded(await abciQuery(`${SVC}/Jewelers`, QueryJewelersRequest.encode(req).finish()),
      (b) => QueryJewelersResponse.decode(b));
    if (!out.ok) return out;
    height = out.height;
    all.push(...out.value.jewelers);
    const next = out.value.pagination?.nextKey;
    if (!next || next.length === 0) return { ok: true, value: all, height };
    key = new Uint8Array(next);
  }
  return { ok: false, reason: 'refused', detail: 'more than 50 pages of jewelers' };
}

export async function readJewelParams(): Promise<Outcome<Params>> {
  const out = decoded(await abciQuery(`${SVC}/Params`, QueryParamsRequest.encode({}).finish()),
    (b) => QueryParamsResponse.decode(b));
  if (!out.ok) return out;
  if (!out.value.params) return { ok: false, reason: 'not-found', detail: 'params' };
  return { ok: true, height: out.height, value: out.value.params };
}

/** Every jewel with this code, across jewelers: two may use the same one. */
export async function findByInscription(normalised: string): Promise<Outcome<Jewel[]>> {
  const req = QueryJewelsByInscriptionRequest.fromPartial({ inscription: normalised });
  const out = decoded(await abciQuery(`${SVC}/JewelsByInscription`, QueryJewelsByInscriptionRequest.encode(req).finish()),
    (b) => QueryJewelsByInscriptionResponse.decode(b));
  if (!out.ok) return out;
  return { ok: true, height: out.height, value: out.value.jewels };
}

export function tokenToJewelId(tokenId: string): string | null {
  const m = /^jewel-([1-9][0-9]*)$/.exec(tokenId);
  return m ? m[1]! : null;
}

/** Ownership lives in x/nft; x/jewel keeps no by-owner index. */
export async function jewelsHeldBy(address: string): Promise<Outcome<string[]>> {
  const req = QueryNFTsRequest.fromPartial({ owner: address, pagination: { limit: '500' } });
  const out = decoded(await abciQuery('/cosmos.nft.v1beta1.Query/NFTs', QueryNFTsRequest.encode(req).finish()),
    (b) => QueryNFTsResponse.decode(b));
  if (!out.ok) return out;
  const ids = out.value.nfts
    .filter((n) => n.classId.startsWith('jewel-'))
    .map((n) => tokenToJewelId(n.id))
    .filter((id): id is string => id !== null)
    .sort((a, b) => (BigInt(a) < BigInt(b) ? -1 : 1));
  return { ok: true, height: out.height, value: ids };
}

export function offerSearchQuery(address: string): string {
  return `blockchain.jewel.v1.EventOfferMade.to='"${address}"'`;
}

function attr(hit: { attributes: { key: string; value: string }[] }, key: string): string | null {
  const a = hit.attributes.find((x) => x.key === key);
  if (!a) return null;
  try { return String(JSON.parse(a.value)); } catch { return a.value; }
}

/** Offers made to this account that are still open to it at `now` (unix seconds). */
export async function offersAddressedTo(address: string, now: number): Promise<Outcome<JewelView[]>> {
  const found = await txSearch(offerSearchQuery(address));
  if (!found.ok) return found;
  const ids = new Set<string>();
  for (const tx of found.value) {
    if (tx.code !== 0) continue;
    for (const e of tx.events) {
      if (e.type !== 'blockchain.jewel.v1.EventOfferMade' || attr(e, 'to') !== address) continue;
      const id = attr(e, 'jewel_id');
      if (id) ids.add(id);
    }
  }
  const open: JewelView[] = [];
  for (const id of ids) {
    const view = await readJewel(id);
    if (!view.ok) {
      if (view.reason === 'not-found') continue;
      return view;
    }
    if (view.value.offer?.to === address && Number(view.value.offer.expiresAt) > now) open.push(view.value);
  }
  return { ok: true, height: found.height, value: open };
}

/** A jeweler's pending claims, each with its jewel, oldest first as the chain lists them. */
export async function claimsByJeweler(jewelerId: string): Promise<Outcome<{ jewel: Jewel; claim: Claim }[]>> {
  const all: { jewel: Jewel; claim: Claim }[] = [];
  let key: Uint8Array<ArrayBuffer> = new Uint8Array(0);
  let height = 0;
  for (let page = 0; page < 50; page++) {
    const req = QueryClaimsByJewelerRequest.fromPartial({ jewelerId, pagination: { key, limit: '100' } });
    const out = decoded(await abciQuery(`${SVC}/ClaimsByJeweler`, QueryClaimsByJewelerRequest.encode(req).finish()),
      (b) => QueryClaimsByJewelerResponse.decode(b));
    if (!out.ok) return out;
    height = out.height;
    for (const c of out.value.claims) if (c.jewel && c.claim) all.push({ jewel: c.jewel, claim: c.claim });
    const next = out.value.pagination?.nextKey;
    if (!next || next.length === 0) break;
    key = next as Uint8Array<ArrayBuffer>;
  }
  return { ok: true, height, value: all };
}

export function claimSearchQuery(address: string): string {
  return `blockchain.jewel.v1.EventClaimFiled.claimer='"${address}"'`;
}

/**
 * Claims this account filed that are still pending. The chain keeps no index by
 * claimer, so they are found from EventClaimFiled and then checked against the
 * jewel as it stands: an event says a claim was filed, only the jewel says it
 * is still waiting.
 */
export async function claimsFiledBy(address: string): Promise<Outcome<JewelView[]>> {
  const found = await txSearch(claimSearchQuery(address));
  if (!found.ok) return found;
  const ids = new Set<string>();
  for (const tx of found.value) {
    if (tx.code !== 0) continue;
    for (const e of tx.events) {
      if (e.type !== 'blockchain.jewel.v1.EventClaimFiled' || attr(e, 'claimer') !== address) continue;
      const id = attr(e, 'jewel_id');
      if (id) ids.add(id);
    }
  }
  const pending: JewelView[] = [];
  for (const id of ids) {
    const view = await readJewel(id);
    if (!view.ok) {
      if (view.reason === 'not-found') continue;
      return view;
    }
    if (view.value.claim?.claimer === address) pending.push(view.value);
  }
  return { ok: true, height: found.height, value: pending };
}

export async function feeAllowanceExists(granter: string, grantee: string): Promise<Outcome<boolean>> {
  const req = QueryAllowanceRequest.encode(QueryAllowanceRequest.fromPartial({ granter, grantee })).finish();
  const out = await abciQuery('/cosmos.feegrant.v1beta1.Query/Allowance', req);
  if (!out.ok) return out.reason === 'not-found' ? { ok: true, value: false, height: 0 } : out;
  const decodedOut = decoded(out, (b) => QueryAllowanceResponse.decode(b));
  if (!decodedOut.ok) return decodedOut;
  return { ok: true, value: decodedOut.value.allowance !== undefined, height: out.height };
}

export function isActive(j: Jeweler): boolean {
  return j.status === JewelerStatus.JEWELER_ACTIVE;
}

export function roleOf(address: string, jewelers: Jeweler[]): { jeweler: Jeweler; business: boolean; staff: boolean } | null {
  for (const j of jewelers) {
    const business = j.business === address;
    const staff = j.staff.includes(address);
    if (business || staff) return { jeweler: j, business, staff };
  }
  return null;
}
