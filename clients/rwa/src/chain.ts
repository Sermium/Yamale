/**
 * Everything this app asks the chain, and the four things it can ask it to do.
 *
 * Reads and writes are in one file on purpose, the way Yamale Pay does it: the
 * list of things an application can do to somebody's money should be readable
 * in a minute. Here that list is four items long — claim, redeem, dispute, and
 * nothing else. This app cannot mint, cannot fractionalise, cannot report a
 * sale and cannot move title, because an investor does none of those and an
 * interface that could would be a sponsor's console wearing an investor's name.
 *
 * Reads of x/tokenisation and x/land go over ABCI (see abci.ts for why), and
 * the two bank reads go over REST, which the proxy does allowlist.
 */
import { ChainSigner, translateError, type TranslatedError } from '@yamale/chain';
import type { OfflineSigner } from '@cosmjs/proto-signing';

import {
  QueryDenomOwnersRequest,
  QueryDenomOwnersResponse,
} from '../../sdk/src/generated/cosmos/bank/v1beta1/query.ts';
import type {
  Auction,
  Book,
  Listing,
  MarketParams,
  Order,
} from './market.ts';
import {
  QueryAuctionsRequest,
  QueryAuctionsResponse,
  QueryBookRequest,
  QueryBookResponse,
  QueryListingsRequest,
  QueryListingsResponse,
  QueryParamsRequest as QueryMarketParamsRequest,
  QueryParamsResponse as QueryMarketParamsResponse,
} from '../../sdk/src/generated/blockchain/market/v1/query.ts';
import {
  auctionStateToJSON,
  listingStateToJSON,
  orderStateToJSON,
  sideToJSON,
  type Auction as WireAuction,
  type Listing as WireListing,
  type Order as WireOrder,
} from '../../sdk/src/generated/blockchain/market/v1/market.ts';
import {
  QueryAssetRequest,
  QueryAssetResponse,
  QueryAssetsRequest,
  QueryAssetsResponse,
  QueryCollectionsResponse,
  QueryCovenantsRequest,
  QueryCovenantsResponse,
  QueryEntitlementRequest,
  QueryEntitlementResponse,
} from '../../sdk/src/generated/blockchain/tokenisation/v1/query.ts';
import {
  Position as WirePosition,
  Status as WireStatus,
  VerificationMode as WireVerification,
  covenantKindToJSON,
  statusToJSON,
  verificationModeToJSON,
  type Asset as WireAsset,
  type Collection as WireCollection,
  type Covenant as WireCovenant,
  type CovenantKind as WireCovenantKind,
  type SaleReport as WireSale,
  type Vault as WireVault,
} from '../../sdk/src/generated/blockchain/tokenisation/v1/tokenisation.ts';
import {
  QueryFractionalisationAuthorityRequest,
  QueryFractionalisationAuthorityResponse,
  QueryParcelRequest,
  QueryParcelResponse,
} from '../../sdk/src/generated/blockchain/land/v1/query.ts';
import { statusToJSON as parcelStatusToJSON, type Parcel as WireParcel } from '../../sdk/src/generated/blockchain/land/v1/parcel.ts';

import { decoded, query, type Outcome } from './abci.ts';
import { toHex as digestHex, type Covenant, type CovenantKind } from './covenant.ts';
import { positionKey, type Position } from './position.ts';
import type {
  Asset,
  Coin,
  Collection,
  LandAuthorisation,
  SaleReport,
  Vault,
  Verification,
  VehicleStatus,
} from './vehicle.ts';

export const CHAIN_ID = 'yamale-devnet-2';
const RPC = `${window.location.origin}/api/rpc/`;
const REST = '/api/rest';

/* ------------------------------------------------------------- conversion */

/**
 * Enum number to the name the proto declares.
 *
 * Through the generated `*ToJSON` helpers rather than through a hand-written
 * switch, because those are regenerated from the same .proto the chain compiles
 * and a hand-written map goes stale the first time a mode is added — silently,
 * as an unrecognised verification mode falling through to "unspecified", which
 * this app grades as *no protection at all*. Getting that wrong in the safe
 * direction would still be wrong: it would libel a sound collection.
 */
function verificationName(v: WireVerification): Verification {
  const name = verificationModeToJSON(v);
  return name === 'UNRECOGNIZED' ? 'VERIFICATION_UNSPECIFIED' : (name as Verification);
}

function statusName(s: WireStatus): VehicleStatus {
  const name = statusToJSON(s);
  return name === 'UNRECOGNIZED' ? 'STATUS_UNSPECIFIED' : (name as VehicleStatus);
}

function toCollection(c: WireCollection): Collection {
  return {
    id: c.id,
    authority: c.authority,
    verification: verificationName(c.verification),
    attestationThreshold: c.attestationThreshold,
    challengeWindowSeconds: Number(c.challengeWindowSeconds),
    disputeBondBps: c.disputeBondBps,
  };
}

function toAsset(a: WireAsset): Asset {
  return {
    id: a.id,
    collectionId: a.collectionId,
    owner: a.owner,
    uri: a.uri,
    fractionDenom: a.fractionDenom,
    holderShareBps: a.holderShareBps,
    status: statusName(a.status),
    parcelId: a.parcelId,
  };
}

function toVault(v: WireVault | undefined): Vault | null {
  // A vault with no denom is the zero value the response carries for an asset
  // that has never been fractionalised — the field is non-nullable on the wire,
  // so "absent" arrives as an empty message rather than as undefined.
  if (!v || v.denom === '') return null;
  return {
    assetId: v.assetId,
    cumulativePerToken: v.cumulativePerToken,
    funded: (v.funded ?? []).map((c) => ({ denom: c.denom, amount: c.amount })),
    denom: v.denom,
    // `held`, `periodIndex` and `periodStart` were on the chain's Vault and
    // dropped here, which is why the distributions panel could say what had
    // been paid in and nothing about what was still there or what was accruing.
    held: (v.held ?? []).map((c) => ({ denom: c.denom, amount: c.amount })),
    periodIndex: v.periodIndex ?? '0',
    periodStart: Number(v.periodStart ?? '0'),
  };
}

/**
 * A filing's kind, through the generated helper for the same reason
 * `verificationName` uses one: a hand-written map goes stale the first time a
 * kind is added, and it does it silently — an unrecognised kind falling through
 * to "unspecified", which this app renders as a document that asks nothing of
 * anybody. A new kind that carries a deadline would be filed as a leaflet.
 */
function covenantKindName(k: WireCovenantKind): CovenantKind {
  const name = covenantKindToJSON(k);
  return name === 'UNRECOGNIZED' ? 'COVENANT_KIND_UNSPECIFIED' : (name as CovenantKind);
}

function toCovenant(c: WireCovenant): Covenant {
  return {
    id: c.id,
    assetId: c.assetId,
    kind: covenantKindName(c.kind),
    title: c.title,
    // Hex at the wire boundary, once. Every screen that shows a digest shows
    // the same string, and the bytes never reach a component that might
    // truncate them differently.
    digest: digestHex(c.sha256),
    uri: c.uri,
    periodStart: Number(c.periodStart ?? '0'),
    periodEnd: Number(c.periodEnd ?? '0'),
    actsBy: Number(c.actsBy ?? '0'),
    publishedBy: c.publishedBy,
    publishedAt: Number(c.publishedAt ?? '0'),
    supersedes: c.supersedes ?? '0',
    supersededBy: c.supersededBy ?? '0',
  };
}

function toSale(s: WireSale | undefined): SaleReport | null {
  if (!s) return null;
  return {
    assetId: s.assetId,
    price: s.price ?? { denom: '', amount: '0' },
    reporter: s.reporter,
    reportedAt: s.reportedAt ?? new Date(0),
    claimableAt: s.claimableAt ?? new Date(0),
    attestors: s.attestors ?? [],
    disputed: s.disputed,
  };
}

/* ------------------------------------------------------------------ reads */

const Q = {
  denomOwners: '/cosmos.bank.v1beta1.Query/DenomOwners',
  collections: '/blockchain.tokenisation.v1.Query/Collections',
  assets: '/blockchain.tokenisation.v1.Query/Assets',
  asset: '/blockchain.tokenisation.v1.Query/Asset',
  entitlement: '/blockchain.tokenisation.v1.Query/Entitlement',
  covenants: '/blockchain.tokenisation.v1.Query/Covenants',
  parcel: '/blockchain.land.v1.Query/Parcel',
  landAuth: '/blockchain.land.v1.Query/FractionalisationAuthority',
  /**
   * The raw store, which is how a Position is read at all.
   *
   * x/tokenisation exposes no RPC that returns one, so `cost_basis`, `cost_qty`
   * and `token_seconds` have no gRPC path. CometBFT serves the store directly
   * and the proxy does not gate it. See position.ts `positionKey` for the key
   * layout and why depending on it is defensible.
   */
  store: '/store/tokenisation/key',

  // The secondary market. Every one of these is a read a prospective buyer
  // makes BEFORE they have an account, which is the whole reason they go over
  // ABCI with the rest: somebody deciding whether a shareholding is worth
  // owning should not have to identify themselves to find out what it trades
  // at.
  marketParams: '/blockchain.market.v1.Query/Params',
  listings: '/blockchain.market.v1.Query/Listings',
  auctions: '/blockchain.market.v1.Query/Auctions',
  book: '/blockchain.market.v1.Query/Book',
} as const;

export async function collections(): Promise<Outcome<Collection[]>> {
  const raw = await query(Q.collections);
  return decoded(raw, (b) => QueryCollectionsResponse.decode(b).collections.map(toCollection));
}

/** Every asset, or every asset in one collection. */
export async function assets(collectionId = ''): Promise<Outcome<Asset[]>> {
  const request = QueryAssetsRequest.encode({
    collectionId,
    // A limit rather than the node's default of 100-with-a-count, because the
    // count costs a full store walk and nothing on these screens shows one.
    pagination: { key: new Uint8Array(0), offset: '0', limit: '200', countTotal: false, reverse: false },
  }).finish();

  const raw = await query(Q.assets, request);
  return decoded(raw, (b) => QueryAssetsResponse.decode(b).assets.map(toAsset));
}

export interface VehicleRecord {
  asset: Asset;
  vault: Vault | null;
  sale: SaleReport | null;
}

/**
 * One vehicle: the title, the vault and any sale reported against it.
 *
 * The three arrive together because x/tokenisation returns them together, and
 * it does that on purpose — an asset read without its vault says nothing about
 * whether the income it promises has arrived, and one read without its sale
 * hides the number that decides what every holder is owed.
 */
export async function vehicle(assetId: string): Promise<Outcome<VehicleRecord>> {
  const request = QueryAssetRequest.encode({ assetId }).finish();
  const raw = await query(Q.asset, request);
  return decoded(raw, (b) => {
    const r = QueryAssetResponse.decode(b);
    return { asset: toAsset(r.asset!), vault: toVault(r.vault), sale: toSale(r.sale) };
  });
}

/**
 * What one holder may take out of a vault right now.
 *
 * Queried rather than derived. It is not computable from a token balance: it
 * depends on what has been paid in, what this holder has already taken, and
 * what they held over each stretch between distributions. A screen that
 * multiplied a balance by an index it read once would be wrong for anybody who
 * had bought or sold since.
 */
export interface Holder {
  address: string;
  /** Tokens held, in base units. A string: supplies here exceed a double. */
  amount: string;
}

/**
 * The register of who owns a tokenisation, read from the bank.
 *
 * x/tokenisation has no holders query and does not need one: the shareholding
 * IS an ordinary bank denomination, so "who holds this" is bank's question and
 * always was. Its positions are keyed (asset, holder) with no list read, which
 * is why this goes to `cosmos.bank.v1beta1.Query/DenomOwners` instead.
 *
 * Sorted largest first, because the question a cap table is opened for is
 * concentration: who could carry a vote alone, and how much of this is one
 * person. Alphabetical by address answers nothing anybody asked.
 *
 * The limit is deliberate and so is what happens past it. A register of
 * thousands is the SUCCESS case for this platform -- the whole argument is that
 * the buyer pool goes from ten to thousands -- so a page that silently showed
 * the first two hundred and called it the holders would understate
 * concentration precisely when it matters least and mislead precisely when it
 * matters most. The caller is told whether more exist.
 */
export async function holders(denom: string, limit = 200): Promise<Outcome<{
  rows: Holder[];
  more: boolean;
}>> {
  const request = QueryDenomOwnersRequest.encode({
    denom,
    pagination: {
      key: new Uint8Array(0), offset: '0', limit: String(limit + 1),
      countTotal: false, reverse: false,
    },
  }).finish();

  const raw = await query(Q.denomOwners, request);
  return decoded(raw, (b) => {
    const owners = QueryDenomOwnersResponse.decode(b).denomOwners ?? [];
    const rows = owners.map((o) => ({
      address: o.address ?? '',
      amount: o.balance?.amount ?? '0',
    }));
    const more = rows.length > limit;
    if (more) rows.length = limit;
    // BigInt, not Number: a supply of 100,000,000 tokens is comfortable but the
    // next vehicle's may not be, and a sort that silently ties every balance
    // above 2^53 would put the cap table in the wrong order without erroring.
    rows.sort((x, y) => (BigInt(y.amount) > BigInt(x.amount) ? 1 : -1));
    return { rows, more };
  });
}

/**
 * Everything published against one tokenisation.
 *
 * `currentOnly` is false and stays false. The query offers to hide filings that
 * have been corrected and this app declines, because a dispute is usually about
 * what was said at a date rather than what is said now — a superseded filing is
 * dimmed and labelled by the panel, never dropped.
 *
 * Unpaginated because the query is: x/tokenisation returns a vehicle's whole
 * register in one response, walked from a key set keyed by asset.
 */
export async function covenants(assetId: string): Promise<Outcome<Covenant[]>> {
  const request = QueryCovenantsRequest.encode({ assetId, currentOnly: false }).finish();
  const raw = await query(Q.covenants, request);
  return decoded(raw, (b) => QueryCovenantsResponse.decode(b).covenants.map(toCovenant));
}

/**
 * One holder's position: what they paid, and what is accruing on it.
 *
 * Read out of the raw store rather than through a query, because there is no
 * query. See position.ts `positionKey`.
 *
 * An absent position comes back as `ok` with a null value rather than as
 * 'not-found', and the distinction is the whole reason this signature is shaped
 * this way: the store answers a missing key with code 0 and an empty value, so
 * "this holder has no position" IS the chain's answer and must not be rendered
 * as a node that failed to reply. A cost panel that said "could not read" when
 * the truth is "nothing was ever recorded" would send somebody looking for a
 * network fault that does not exist.
 */
export async function position(
  assetId: string,
  holder: string,
): Promise<Outcome<Position | null>> {
  if (!holder) return { ok: true, value: null, height: 0 };

  const raw = await query(Q.store, positionKey(assetId, holder));
  return decoded(raw, (bytes) => {
    if (bytes.length === 0) return null;
    const p: WirePosition = WirePosition.decode(bytes);
    return {
      assetId: p.assetId,
      holder: p.holder,
      accrued: p.accrued,
      costBasis: p.costBasis,
      costQty: p.costQty,
      tokenSeconds: p.tokenSeconds,
      tokenSecondsPeriod: p.tokenSecondsPeriod,
      lastTouch: Number(p.lastTouch ?? '0'),
    } satisfies Position;
  });
}

export async function entitlement(assetId: string, holder: string): Promise<Outcome<Coin>> {
  const request = QueryEntitlementRequest.encode({ assetId, holder }).finish();
  const raw = await query(Q.entitlement, request);
  return decoded(raw, (b) => {
    const owed = QueryEntitlementResponse.decode(b).owed;
    return { denom: owed?.denom ?? '', amount: owed?.amount ?? '0' };
  });
}

export interface ParcelRecord {
  id: string;
  cadastralRef: string;
  holder: string;
  authority: string;
  status: string;
  restrictions: { kind: string; value: string; detail: string; lifted: boolean }[];
  encumbrances: { kind: string; holder: string; detail: string; released: boolean }[];
  frozen: boolean;
  freezeReason: string;
}

export async function parcel(parcelId: string): Promise<Outcome<ParcelRecord>> {
  const request = QueryParcelRequest.encode({ id: parcelId }).finish();
  const raw = await query(Q.parcel, request);
  return decoded(raw, (b) => {
    const p: WireParcel = QueryParcelResponse.decode(b).parcel!;
    // The current freeze, not the history. A lifted freeze stays on the record
    // — deliberately, so a second freeze by a different office can be argued
    // with — but only an unlifted one stops a dealing today.
    const live = (p.freezes ?? []).find((f) => !f.lifted);
    return {
      id: p.id,
      cadastralRef: p.cadastralRef,
      holder: p.holder,
      authority: p.authority,
      status: parcelStatusToJSON(p.status),
      restrictions: (p.restrictions ?? []).map((r) => ({
        kind: r.kind, value: r.value, detail: r.detail, lifted: r.lifted,
      })),
      encumbrances: (p.encumbrances ?? []).map((e) => ({
        kind: e.kind, holder: e.holder, detail: e.detail, released: e.released,
      })),
      frozen: live !== undefined,
      freezeReason: live?.reason ?? '',
    };
  });
}

export interface AuthorisationRecord {
  authorisation: LandAuthorisation;
  live: boolean;
}

/**
 * The registry's standing permission to sell shares in a parcel, and whether it
 * still stands.
 *
 * `live` is the chain's own answer, computed in the keeper so that a wallet and
 * the keeper cannot disagree about what "live" means — a disagreement only ever
 * discovered by somebody whose money has already moved. It is carried through
 * untouched.
 */
export async function landAuthorisation(parcelId: string): Promise<Outcome<AuthorisationRecord>> {
  const request = QueryFractionalisationAuthorityRequest.encode({ parcelId }).finish();
  const raw = await query(Q.landAuth, request);
  return decoded(raw, (b) => {
    const r = QueryFractionalisationAuthorityResponse.decode(b);
    const a = r.authorisation!;
    return {
      live: r.live,
      authorisation: {
        parcelId: a.parcelId,
        right: a.right,
        maxShareBps: a.maxShareBps,
        expiresAt: Number(a.expiresAt),
        grantedBy: a.grantedBy,
        grantedAt: Number(a.grantedAt),
        withdrawn: a.withdrawn,
        withdrawnAt: Number(a.withdrawnAt),
      },
    };
  });
}

/* ------------------------------------------------------------- bank, over REST */

/**
 * A denomination's total supply — the denominator of every percentage on these
 * screens.
 *
 * Over REST rather than ABCI because `/api/rest/cosmos/bank/` *is* allowlisted
 * by the proxy, and because there is no generated TS for the cosmos bank
 * queries in this SDK: reaching them over ABCI would mean hand-writing an
 * encoder for a message somebody else already maintains.
 */
export async function supplyOf(denom: string): Promise<string | null> {
  if (!denom) return null;
  try {
    const res = await fetch(
      `${REST}/cosmos/bank/v1beta1/supply/by_denom?denom=${encodeURIComponent(denom)}`,
      { signal: AbortSignal.timeout(9000) },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json?.amount?.amount ?? null;
  } catch {
    return null;
  }
}

export async function balanceOf(address: string, denom: string): Promise<string | null> {
  if (!address || !denom) return null;
  try {
    const res = await fetch(
      `${REST}/cosmos/bank/v1beta1/balances/${address}/by_denom?denom=${encodeURIComponent(denom)}`,
      { signal: AbortSignal.timeout(9000) },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return json?.balance?.amount ?? null;
  } catch {
    return null;
  }
}

/** Every coin an account holds, for finding which vehicles it is in. */
export async function balances(address: string): Promise<Coin[] | null> {
  if (!address) return null;
  try {
    const res = await fetch(
      `${REST}/cosmos/bank/v1beta1/balances/${address}?pagination.limit=500`,
      { signal: AbortSignal.timeout(9000) },
    );
    if (!res.ok) return null;
    const json = await res.json();
    return (json?.balances ?? []) as Coin[];
  } catch {
    return null;
  }
}

/* ----------------------------------------------------------------- writes */

export interface Signed {
  ok: boolean;
  hash: string;
  height: number;
  error?: TranslatedError;
}

function signerFor(offline: OfflineSigner): ChainSigner {
  return new ChainSigner(offline, {
    rpcUrl: RPC,
    chainId: CHAIN_ID,
    // Fees are covered by a grant on this network, so the number a holder sees
    // leaving their account is the number that leaves it.
    gasPrice: 0,
  });
}

async function submit(
  offline: OfflineSigner,
  typeUrl: string,
  value: Record<string, unknown>,
  gas: number,
): Promise<Signed> {
  try {
    const signer = signerFor(offline);
    const res = await signer.submit([{ typeUrl, value }], '', gas);
    return { ok: res.succeeded, hash: res.hash, height: res.height, error: res.error };
  } catch (err) {
    // Reached when the node is unreachable rather than when it refuses — a
    // refusal comes back through submit() as a translated error.
    return {
      ok: false,
      hash: '',
      height: 0,
      error: translateError(err instanceof Error ? err.message : String(err)),
    };
  }
}

/**
 * Take the income owed, without giving up the shareholding.
 *
 * Reversible in the only sense that matters: it changes nothing about what is
 * owned. It can be called again the next time the vault is funded.
 */
export function claim(offline: OfflineSigner, holder: string, assetId: string): Promise<Signed> {
  return submit(offline, '/blockchain.tokenisation.v1.MsgClaim', { holder, assetId }, 250_000);
}

/**
 * Burn shares for their part of the proceeds. This is the exit.
 *
 * The burn *is* the claim — there is no later step and no way back. `amount` is
 * a count of shares in base units, and every screen that reaches this has to
 * have shown what it will pay and what it will destroy before it was called.
 */
export function redeem(
  offline: OfflineSigner,
  holder: string,
  assetId: string,
  amount: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.tokenisation.v1.MsgRedeem', { holder, assetId, amount }, 300_000);
}

/**
 * Challenge a reported sale price, staking the collection's bond.
 *
 * The bond leaves the challenger's account in this block. It comes back if the
 * dispute succeeds and is forfeited to the vault — never to the sponsor — if it
 * does not. The amount must have been stated before this is called; see
 * vehicle.ts `disputeBond`, which computes it the way the keeper does.
 */
export function disputeSale(
  offline: OfflineSigner,
  challenger: string,
  assetId: string,
  reason: string,
): Promise<Signed> {
  return submit(
    offline,
    '/blockchain.tokenisation.v1.MsgDisputeSale',
    { challenger, assetId, reason },
    300_000,
  );
}

/* ========================================================================== */
/*  The secondary market                                                      */
/* ========================================================================== */
//
// Reads first, then the things a holder can ask the chain to do with a
// shareholding they own. That list is deliberately shorter than what the module
// offers: this app composes no MsgUpdateParams and never will, because an
// investor does not set the depth of the book.

function toListing(l: WireListing): Listing {
  return {
    id: l.id, assetId: l.assetId, denom: l.denom, seller: l.seller,
    quantity: l.quantity,
    price: { denom: l.price?.denom ?? '', amount: l.price?.amount ?? '0' },
    state: listingStateToJSON(l.state) as Listing['state'],
    listedAt: Number(l.listedAt ?? '0'),
    filledBy: l.filledBy ?? '',
    filledAt: Number(l.filledAt ?? '0'),
  };
}

function toAuction(a: WireAuction): Auction {
  return {
    id: a.id, assetId: a.assetId, denom: a.denom, seller: a.seller,
    quantity: a.quantity,
    reserve: { denom: a.reserve?.denom ?? '', amount: a.reserve?.amount ?? '0' },
    closesAt: Number(a.closesAt ?? '0'),
    bestBidder: a.bestBidder ?? '',
    bestBid: { denom: a.bestBid?.denom ?? '', amount: a.bestBid?.amount ?? '0' },
    state: auctionStateToJSON(a.state) as Auction['state'],
    openedAt: Number(a.openedAt ?? '0'),
    settledAt: Number(a.settledAt ?? '0'),
  };
}

function toOrder(o: WireOrder): Order {
  return {
    id: o.id, assetId: o.assetId, denom: o.denom, trader: o.trader,
    side: sideToJSON(o.side) as Order['side'],
    quantity: o.quantity, remaining: o.remaining,
    // Normalised to a string here rather than at every call site: a uint64
    // arrives as a number on some decoder paths and a string on others, and a
    // price that is sometimes a number is a price that is sometimes NaN.
    unitPrice: String(o.unitPrice ?? '0'),
    settlementDenom: o.settlementDenom ?? '',
    state: orderStateToJSON(o.state) as Order['state'],
    placedAt: Number(o.placedAt ?? '0'),
  };
}

/** How deep one order may sweep, and how long an auction may run. */
export async function marketParams(): Promise<Outcome<MarketParams>> {
  const request = QueryMarketParamsRequest.encode({}).finish();
  const raw = await query(Q.marketParams, request);
  return decoded(raw, (b) => {
    const p = QueryMarketParamsResponse.decode(b).params;
    return {
      maxFillsPerOrder: Number(p?.maxFillsPerOrder ?? 0),
      minAuctionPeriod: Number(p?.minAuctionPeriod ?? '0'),
      maxAuctionPeriod: Number(p?.maxAuctionPeriod ?? '0'),
    } satisfies MarketParams;
  });
}

/**
 * Every fixed-price offer over a share.
 *
 * `liveOnly` is false by default, and that is the useful direction: the filled
 * ones are the only record this chain keeps of what a shareholding has actually
 * fetched. A page showing only what is for sale would be hiding the price
 * history behind a flag.
 */
export async function listings(denom: string, liveOnly = false): Promise<Outcome<Listing[]>> {
  const request = QueryListingsRequest.encode({ denom, liveOnly }).finish();
  const raw = await query(Q.listings, request);
  return decoded(raw, (b) => QueryListingsResponse.decode(b).listings.map(toListing));
}

export async function auctions(denom: string, openOnly = false): Promise<Outcome<Auction[]>> {
  const request = QueryAuctionsRequest.encode({ denom, openOnly }).finish();
  const raw = await query(Q.auctions, request);
  return decoded(raw, (b) => QueryAuctionsResponse.decode(b).auctions.map(toAuction));
}

/** What is bid and offered right now, best price first on each side. */
export async function book(denom: string, depth = 20): Promise<Outcome<Book>> {
  const request = QueryBookRequest.encode({ denom, depth }).finish();
  const raw = await query(Q.book, request);
  return decoded(raw, (b) => {
    const r = QueryBookResponse.decode(b);
    return {
      bids: r.bids.map(toOrder),
      asks: r.asks.map(toOrder),
      settlementDenom: r.settlementDenom ?? '',
    } satisfies Book;
  });
}

/* --------------------------------------------------------- and the doing */

/**
 * Offer a parcel at a fixed total.
 *
 * The shares leave the seller HERE, not at fill. That is the module's decision
 * and it is the right one — a listing backed by shares the seller still holds
 * can be listed twice or transferred away, and the first buyer to pay discovers
 * there is nothing to deliver — but it means the screen calling this has to
 * have said so first. After this the parcel is gone from the holding until the
 * listing is taken or withdrawn.
 */
export function listParcel(
  offline: OfflineSigner,
  seller: string,
  denom: string,
  quantity: string,
  price: Coin,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgList',
    { seller, denom, quantity, price }, 350_000);
}

/** Take a listing whole. There is no partial fill; that is what the book is for. */
export function buyListing(
  offline: OfflineSigner,
  buyer: string,
  listingId: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgBuy', { buyer, listingId }, 350_000);
}

export function cancelListing(
  offline: OfflineSigner,
  seller: string,
  listingId: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgCancelListing',
    { seller, listingId }, 250_000);
}

/**
 * Offer a parcel to the best bid received by a deadline.
 *
 * `closesAt` is an ABSOLUTE time in seconds, never a duration, and the caller
 * computes it from a clock rather than passing a span: a duration measured from
 * whenever the transaction is included means a seller signing "one week" during
 * congestion gets an auction closing at a moment they did not choose, with
 * their shares escrowed for the difference. See market.ts `auctionWindow`.
 */
export function openAuction(
  offline: OfflineSigner,
  seller: string,
  denom: string,
  quantity: string,
  reserve: Coin,
  closesAt: number,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgOpenAuction',
    { seller, denom, quantity, reserve, closesAt: String(closesAt) }, 350_000);
}

/**
 * Beat the standing bid.
 *
 * The money leaves the bidder in this block and the previous best bid is
 * refunded in the same message. A screen calling this has to have said that the
 * amount is committed until somebody outbids them or the auction settles.
 */
export function bid(
  offline: OfflineSigner,
  bidder: string,
  auctionId: string,
  amount: Coin,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgBid', { bidder, auctionId, amount }, 350_000);
}

/**
 * Close out an auction past its deadline.
 *
 * Offered to everybody, including the winner and a passer-by, because the
 * module allows it deliberately: needing the seller's signature would let a
 * seller who disliked the result strand the winner's capital by doing nothing.
 */
export function settleAuction(
  offline: OfflineSigner,
  settler: string,
  auctionId: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgSettleAuction',
    { settler, auctionId }, 400_000);
}

export function cancelAuction(
  offline: OfflineSigner,
  seller: string,
  auctionId: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgCancelAuction',
    { seller, auctionId }, 250_000);
}

/**
 * Cross what can be crossed and rest the remainder.
 *
 * `unitPrice` is settlement base units per one share, and it is a LIMIT: a bid
 * will not pay more, an ask will not take less. Callers must have produced it
 * with market.ts `unitPriceFromDisplay` — the factor between a typed price and
 * this field is the settlement denom's exponent, and passing a typed number
 * straight through is the most expensive mistake available on this surface.
 *
 * The gas allowance carries the fill loop, which is bounded by the chain's own
 * max_fills_per_order rather than by anything this app decides.
 */
export function placeOrder(
  offline: OfflineSigner,
  trader: string,
  denom: string,
  side: 'SIDE_BID' | 'SIDE_ASK',
  quantity: string,
  unitPrice: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgPlaceOrder',
    { trader, denom, side: side === 'SIDE_BID' ? 1 : 2, quantity, unitPrice }, 600_000);
}

export function cancelOrder(
  offline: OfflineSigner,
  trader: string,
  orderId: string,
): Promise<Signed> {
  return submit(offline, '/blockchain.market.v1.MsgCancelOrder', { trader, orderId }, 250_000);
}
