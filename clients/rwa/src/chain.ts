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
  QueryAssetRequest,
  QueryAssetResponse,
  QueryAssetsRequest,
  QueryAssetsResponse,
  QueryCollectionsResponse,
  QueryEntitlementRequest,
  QueryEntitlementResponse,
} from '../../sdk/src/generated/blockchain/tokenisation/v1/query.ts';
import {
  Status as WireStatus,
  VerificationMode as WireVerification,
  statusToJSON,
  verificationModeToJSON,
  type Asset as WireAsset,
  type Collection as WireCollection,
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
  collections: '/blockchain.tokenisation.v1.Query/Collections',
  assets: '/blockchain.tokenisation.v1.Query/Assets',
  asset: '/blockchain.tokenisation.v1.Query/Asset',
  entitlement: '/blockchain.tokenisation.v1.Query/Entitlement',
  parcel: '/blockchain.land.v1.Query/Parcel',
  landAuth: '/blockchain.land.v1.Query/FractionalisationAuthority',
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
