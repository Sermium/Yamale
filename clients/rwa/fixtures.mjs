/**
 * Protobuf fixtures for driving the populated screens in a browser.
 *
 * Not shipped and not imported by the app: this is a harness. It encodes the
 * exact responses yamale-devnet-2 would return for a fractionalised vehicle
 * over a parcel with a live authorisation and a sale inside its window, so the
 * screens that only exist when there is data can be measured rather than
 * assumed. The chain has held zero collections for the whole of this work.
 *
 * Run:  node --experimental-strip-types fixtures.mjs
 */
import { QueryAssetResponse, QueryAssetsResponse, QueryCollectionsResponse, QueryEntitlementResponse }
  from '../sdk/src/generated/blockchain/tokenisation/v1/query.ts';
import { QueryFractionalisationAuthorityResponse, QueryParcelResponse }
  from '../sdk/src/generated/blockchain/land/v1/query.ts';

const b64 = (bytes) => Buffer.from(bytes).toString('base64');

const COLLECTION = {
  id: 'cd-farmland',
  authority: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
  verification: 2, // VERIFY_ATTESTORS
  attestationThreshold: 3,
  challengeWindowSeconds: '604800',
  disputeBondBps: 500,
};

// A second collection that protects nobody, so the grade that matters can be
// seen next to one that does not.
const NAKED = {
  id: 'demo-unchecked',
  authority: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
  verification: 0,
  attestationThreshold: 0,
  challengeWindowSeconds: '0',
  disputeBondBps: 0,
};

const ASSET = {
  id: '3',
  collectionId: 'cd-farmland',
  owner: 'yml1f6fyc0ptxh7padqr3hnrw6sm8wjfr93w6cgv39jwm00nd6kh08est202lp',
  uri: 'https://registry.example/cd-0007',
  fractionDenom: 'tok/3/CDFARM',
  holderShareBps: 4000,
  status: 3, // STATUS_REPORTED
  parcelId: '7',
};

const HELD = {
  id: '4', collectionId: 'demo-unchecked',
  owner: 'yml1f6fyc0ptxh7padqr3hnrw6sm8wjfr93w6cgv39jwm00nd6kh08est202lp',
  uri: '', fractionDenom: 'tok/4/RISK', holderShareBps: 9000, status: 2, parcelId: '0',
};

const now = Date.now();

const out = {
  collections: b64(QueryCollectionsResponse.encode({
    collections: [COLLECTION, NAKED], pagination: undefined,
  }).finish()),

  assets: b64(QueryAssetsResponse.encode({
    assets: [ASSET, HELD], pagination: undefined,
  }).finish()),

  asset: b64(QueryAssetResponse.encode({
    asset: ASSET,
    vault: {
      assetId: '3',
      cumulativePerToken: '0.000048000000000000',
      funded: [{ denom: 'ucdf', amount: '480000000' }],
      denom: 'ucdf',
    },
    sale: {
      assetId: '3',
      price: { denom: 'ucdf', amount: '82000000000' },
      reporter: 'yml1f6fyc0ptxh7padqr3hnrw6sm8wjfr93w6cgv39jwm00nd6kh08est202lp',
      reportedAt: new Date(now - 2 * 86400_000),
      claimableAt: new Date(now + 5 * 86400_000),
      attestors: ['yml1n64en27u7qckklkk4twkkun5h6v5dsur7g6l4pfmfhydvfru9upqjevn2z'],
      disputed: false,
    },
  }).finish()),

  // The same vehicle after the sale finalised, so the exit dialog can be seen.
  assetRealised: b64(QueryAssetResponse.encode({
    asset: { ...ASSET, status: 4 },
    vault: { assetId: '3', cumulativePerToken: '0.000048000000000000', funded: [{ denom: 'ucdf', amount: '480000000' }], denom: 'ucdf' },
    sale: { assetId: '3', price: { denom: 'ucdf', amount: '82000000000' }, reporter: ASSET.owner, reportedAt: new Date(now - 9 * 86400_000), claimableAt: new Date(now - 2 * 86400_000), attestors: ['yml1n64en27u7qckklkk4twkkun5h6v5dsur7g6l4pfmfhydvfru9upqjevn2z'], disputed: false },
  }).finish()),

  entitlement: b64(QueryEntitlementResponse.encode({
    owed: { denom: 'ucdf', amount: '9600000' },
  }).finish()),

  landAuth: b64(QueryFractionalisationAuthorityResponse.encode({
    authorisation: {
      parcelId: '7',
      right: 'exploitation agricole',
      maxShareBps: 4000,
      expiresAt: String(Math.floor(now / 1000) + 120 * 86400),
      grantedBy: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
      grantedAt: String(Math.floor(now / 1000) - 30 * 86400),
      withdrawn: false,
      withdrawnAt: '0',
    },
    live: true,
  }).finish()),

  // The same permission after the office withdrew it: the finding this app
  // exists to put on the page.
  landAuthDead: b64(QueryFractionalisationAuthorityResponse.encode({
    authorisation: {
      parcelId: '7',
      right: 'exploitation agricole',
      maxShareBps: 4000,
      expiresAt: String(Math.floor(now / 1000) + 120 * 86400),
      grantedBy: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
      grantedAt: String(Math.floor(now / 1000) - 30 * 86400),
      withdrawn: true,
      withdrawnAt: String(Math.floor(now / 1000) - 3 * 86400),
    },
    live: false,
  }).finish()),

  parcel: b64(QueryParcelResponse.encode({
    parcel: {
      id: '7',
      geometryHash: 'b1946ac92492d2347c6235b4d2611184',
      cadastralRef: 'KIN/GO/2019/00412',
      holder: 'yml1f6fyc0ptxh7padqr3hnrw6sm8wjfr93w6cgv39jwm00nd6kh08est202lp',
      authority: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
      status: 1,
      encumbrances: [{
        kind: 'mortgage', holder: 'yml1n64en27u7qckklkk4twkkun5h6v5dsur7g6l4pfmfhydvfru9upqjevn2z',
        detail: 'Rang 1 — 40 000 000 CDF', recordedAt: '0', released: false,
      }],
      registeredAt: '81000',
      deeds: [],
      restrictions: [{
        kind: 'agricultural_use_only', value: '', detail: 'Arrêté provincial 2021-114',
        imposedBy: 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s',
        imposedAt: '0', lifted: false,
      }],
      vehicleId: '3',
      freezes: [],
    },
  }).finish()),
};

console.log(JSON.stringify(out));
