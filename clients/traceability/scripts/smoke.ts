/**
 * Every chain path the traceability app uses, driven from TypeScript against the
 * local devnet and traceability-api. Run after init-local-devnet.sh, seed-jewel.sh
 * and traceability-local.sh. Each check prints ok or FAIL; any FAIL exits 1.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { DirectSecp256k1Wallet } from '@cosmjs/proto-signing';

import { chainHead, setRpcBase } from '../../sdk/src/abci.ts';
import {
  acceptJewelOffer, addressOfPublicKey, attachJewelBenefit, claimJewelOffer, compressedPublicKey, fromHex,
  grantStaffFees, JEWEL_GAS, makeOwnershipProof, makeVoucher, newClaimKey, normaliseInscription,
  offerJewelByLink, offerJewelToAccount, redeemJewelBenefit, registerJewel, verifyOwnershipProof,
} from '../../sdk/src/jewel.ts';
import {
  feeAllowanceExists, findByInscription, jewelsHeldBy, offersAddressedTo, readJewel, readJewelers, roleOf,
} from '../../sdk/src/jewelreads.ts';
import { sha256Hex, signedHeaders } from '../../sdk/src/traceabilityauth.ts';
import { ChainSigner, type EncodeObject, type SubmitResult } from '../../sdk/src/signing.ts';

function env(name: string): string {
  const v = process.env[name];
  if (!v) { console.error(`${name} is required`); process.exit(2); }
  return v;
}
const RPC = env('RPC');
const API = env('API').replace(/\/+$/, '');
const CHAIN = env('CHAIN');
setRpcBase(RPC);
const business = fromHex(env('BUSINESS_KEY'));
const staff = fromHex(env('STAFF_KEY'));
const addr = (k: Uint8Array) => addressOfPublicKey(compressedPublicKey(k));

let failed = 0;
function check(label: string, pass: boolean, detail = ''): void {
  if (pass) { console.log(`  ok    ${label}`); return; }
  failed++;
  console.log(`  FAIL  ${label}${detail ? `: ${detail}` : ''}`);
}

function stop(why: string): never {
  console.log(`
${failed} check(s) failed; ${why}`);
  process.exit(1);
}

async function send(key: Uint8Array, messages: EncodeObject[], granter?: string): Promise<SubmitResult> {
  const wallet = await DirectSecp256k1Wallet.fromKey(key, 'yml');
  const signer = new ChainSigner(wallet, { rpcUrl: RPC, chainId: CHAIN, ...(granter ? { feeGranter: granter } : {}) });
  try { return await signer.submit(messages, '', JEWEL_GAS); } finally { signer.disconnect(); }
}
const rawOf = (r: SubmitResult) => r.error?.raw ?? r.raw.rawLog ?? '';

function eventValue(r: SubmitResult, type: string, key: string): string {
  for (const e of r.raw.events ?? []) {
    if (e.type !== type) continue;
    const a = e.attributes.find((x) => x.key === key);
    if (a) { try { return String(JSON.parse(a.value)); } catch { return a.value; } }
  }
  return '';
}

async function upload(key: Uint8Array, bytes: Uint8Array): Promise<string | null> {
  const hash = sha256Hex(bytes);
  try {
    const res = await fetch(`${API}/v1/files`, { method: 'POST', headers: signedHeaders(key, 'file', CHAIN, hash), body: bytes });
    const body = await res.json().catch(() => ({}));
    return res.status === 201 && body.hash === hash ? hash : null;
  } catch {
    return null;
  }
}

async function sponsor(key: Uint8Array): Promise<{ status: number; body: any }> {
  const res = await fetch(`${API}/v1/sponsor`, { method: 'POST', headers: signedHeaders(key, 'sponsor', CHAIN, addr(key)) });
  return { status: res.status, body: await res.json().catch(() => ({})) };
}

async function chainNow(): Promise<number> {
  const h = await chainHead();
  if (!h.ok) throw new Error(`no chain head: ${h.detail}`);
  return h.value.time;
}

const bizAddr = addr(business);
const staffAddr = addr(staff);

console.log("1  staff fees are the business's");
const has = await feeAllowanceExists(bizAddr, staffAddr);
if (has.ok && has.value) {
  check('the staff allowance is already there', true);
} else {
  const g = await send(business, [grantStaffFees(bizAddr, staffAddr, [{ denom: 'uyml', amount: '5000000' }], new Date(Date.now() + 86_400_000))]);
  check('the business granted its staff an allowance', g.succeeded, rawOf(g));
}
const jewelers = await readJewelers();
const role = jewelers.ok ? roleOf(staffAddr, jewelers.value) : null;
check('the staff key is staff of a jeweler', role?.staff === true);
const jewelerId = role?.jeweler.id ?? '1';

console.log('2  files');
const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, ...new Array<number>(64).fill(7)]);
const photo = await upload(staff, png);
check('a photo is stored under its fingerprint', photo !== null);
if (photo === null) stop('no photo, so nothing after this can be tried');
const code = `SMOKE-${Math.floor(Date.now() / 1000)}`;
const normalised = normaliseInscription(code)!;
const record = new TextEncoder().encode(JSON.stringify({
  version: 1, jewelerId, inscription: normalised, title: 'Smoke ring', metal: 'silver', weightGrams: '1', stones: [],
  origin: { workshop: 'Dakar', stoneSource: '', labCertificate: '' }, photos: [photo], createdAt: new Date().toISOString(),
}));
const recordHash = await upload(staff, record);
check('the record is stored under its fingerprint', recordHash !== null);
if (recordHash === null) stop('no record, so nothing after this can be tried');

console.log('3  registration');
const reg = await send(staff, [registerJewel(staffAddr, code, recordHash, `${API}/v1/files/${recordHash}`)], bizAddr);
check('staff registered a jewel, the business paying', reg.succeeded, rawOf(reg));
const jewelId = eventValue(reg, 'blockchain.jewel.v1.EventJewelRegistered', 'jewel_id');
check('the registration names the jewel', jewelId !== '');
const found = await findByInscription(normalised);
check('the engraved code finds it', found.ok && found.value.some((j) => j.id === jewelId));

console.log('4  sponsorship');
const buyer = secp256k1.utils.randomPrivateKey();
const first = await sponsor(buyer);
check('a new account is sponsored', first.status === 200 && first.body.granted === true, JSON.stringify(first.body));
const granter = String(first.body.granter);
const repeat = await sponsor(buyer);
check('only once', repeat.status === 200 && repeat.body.already === true, JSON.stringify(repeat.body));

console.log('5  a claim by link');
const { privateKey: claimPriv, claimKey } = newClaimKey();
const byLink = await send(staff, [offerJewelByLink(staffAddr, jewelId, claimKey)], bizAddr);
check('staff offered it by link', byLink.succeeded, rawOf(byLink));
const offered = await readJewel(jewelId);
const offerExpires = offered.ok ? offered.value.offer?.expiresAt ?? '' : '';
check('the link offer is open on the chain', offerExpires !== '');
const claim = await send(buyer, [claimJewelOffer(addr(buyer), jewelId, offerExpires, claimPriv, CHAIN)], granter);
check('the buyer claimed it, the platform paying', claim.succeeded, rawOf(claim));
let view = await readJewel(jewelId);
check('the buyer owns it, as owner 1', view.ok && view.value.owner === addr(buyer) && view.value.jewel.ownerCount === 1);

console.log('6  a benefit used once');
const attach = await send(staff, [attachJewelBenefit(staffAddr, jewelId, 'cleaning', 2, 31_536_000, 0)], bizAddr);
const benefitId = eventValue(attach, 'blockchain.jewel.v1.EventBenefitAttached', 'benefit_id');
check('a cleaning benefit is attached', attach.succeeded && benefitId !== '', rawOf(attach));
const voucher = makeVoucher(buyer, CHAIN, benefitId, jewelId, await chainNow());
const use = await send(staff, [redeemJewelBenefit(staffAddr, voucher)], bizAddr);
check("the owner's code is redeemed", use.succeeded, rawOf(use));
const again = await send(staff, [redeemJewelBenefit(staffAddr, voucher)], bizAddr);
check('the same code is refused', !again.succeeded && rawOf(again).includes('already been used'), rawOf(again));
view = await readJewel(jewelId);
check('one use is recorded', view.ok && view.value.benefits.find((b) => b.id === benefitId)?.uses.length === 1);

console.log('7  a resale found by tx_search');
const next = secp256k1.utils.randomPrivateKey();
check('a second account is sponsored', (await sponsor(next)).status === 200);
const toNext = await send(buyer, [offerJewelToAccount(addr(buyer), jewelId, addr(next))], granter);
check('the buyer offered it to the second account', toNext.succeeded, rawOf(toNext));
let seen = false;
for (let i = 0; i < 10 && !seen; i++) {
  const offers = await offersAddressedTo(addr(next), await chainNow());
  seen = offers.ok && offers.value.some((v) => v.jewel.id === jewelId);
  if (!seen) await new Promise((r) => setTimeout(r, 1000));
}
check('the second account finds the offer', seen);
const accept = await send(next, [acceptJewelOffer(addr(next), jewelId)], granter);
check('and accepts it', accept.succeeded, rawOf(accept));
view = await readJewel(jewelId);
check('owner 2, with two dated changes', view.ok && view.value.owner === addr(next) && view.value.jewel.ownerSince.length === 2);

console.log('8  ownership proof');
const proof = verifyOwnershipProof(CHAIN, jewelId, makeOwnershipProof(next, CHAIN, jewelId, await chainNow()));
check('the proof names the owner', proof !== null && view.ok && proof.address === view.value.owner);

console.log("9  the allowance's scope");
const misuse = await send(next, [registerJewel(addr(next), `X-${code}`, recordHash, `${API}/v1/files/${recordHash}`)], granter);
check("the platform's allowance does not pay for staff work", !misuse.succeeded && rawOf(misuse).includes('allowed messages'), rawOf(misuse));

console.log('10 holdings');
const held = await jewelsHeldBy(addr(next));
check('x/nft lists it under the second account', held.ok && held.value.includes(jewelId));

console.log(failed ? `\n${failed} check(s) failed` : '\nall checks passed');
process.exit(failed ? 1 : 0);
