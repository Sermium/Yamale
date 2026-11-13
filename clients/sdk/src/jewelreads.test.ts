import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { setRpcBase } from './abci.ts';
import { JewelerStatus } from './generated/blockchain/jewel/v1/jewel.ts';
import { QueryJewelResponse, QueryJewelersResponse } from './generated/blockchain/jewel/v1/query.ts';
import { QueryNFTsResponse } from './generated/cosmos/nft/v1beta1/query.ts';
import {
  isActive, jewelsHeldBy, offerSearchQuery, offersAddressedTo, readJewel, readJewelers, roleOf, tokenToJewelId,
} from './jewelreads.ts';

const realFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = realFetch; setRpcBase('/api/rpc'); });

const b64 = (b: Uint8Array) => Buffer.from(b).toString('base64');
const answer = (bytes: Uint8Array) => ({ result: { response: { code: 0, value: b64(bytes), height: '20' } } });

function jewelReply(id: string, offerTo: string, expiresAt: string) {
  return answer(QueryJewelResponse.encode(QueryJewelResponse.fromPartial({
    jewel: { id, jewelerId: '1', inscription: 'MD20260001', ownerCount: 1, ownerSince: ['1790000000'], registeredAt: '1789990000' },
    owner: 'yml1seller',
    offer: offerTo ? { jewelId: id, from: 'yml1seller', to: offerTo, expiresAt, maker: 'yml1seller' } : undefined,
    benefits: [],
  })).finish());
}

function route(table: Record<string, unknown>) {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    const hit = Object.keys(table).find((k) => url.includes(k));
    if (!hit) throw new Error(`unrouted ${url}`);
    return new Response(JSON.stringify(table[hit]), { status: 200 });
  }) as typeof fetch;
}

test('a jewel reads with its owner and its open offer', async () => {
  route({ 'abci_query': jewelReply('4', 'yml1me', '1790100000') });
  const out = await readJewel('4');
  assert.ok(out.ok);
  assert.equal(out.value.jewel.inscription, 'MD20260001');
  assert.equal(out.value.owner, 'yml1seller');
  assert.equal(out.value.offer?.to, 'yml1me');
  assert.deepEqual(out.value.jewel.ownerSince, ['1790000000']);
});

test('the offer query matches the JSON-encoded address', () => {
  assert.equal(offerSearchQuery('yml1me'), `blockchain.jewel.v1.EventOfferMade.to='"yml1me"'`);
});

test('offers addressed to me are the ones still open to me', async () => {
  const hit = (id: string) => ({ hash: id, height: '9', tx_result: { code: 0, events: [{
    type: 'blockchain.jewel.v1.EventOfferMade',
    attributes: [{ key: 'jewel_id', value: `"${id}"` }, { key: 'to', value: '"yml1me"' }] }] } });
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = decodeURIComponent(String(input));
    if (url.includes('tx_search')) {
      return new Response(JSON.stringify({ result: { txs: [hit('4'), hit('9'), hit('4'), hit('11')] } }));
    }
    // 4: still open to me. 9: re-offered to someone else. 11: expired.
    const data = new URL(String(input), 'http://x').searchParams.get('data')!;
    const reply = data === '0x0804' ? jewelReply('4', 'yml1me', '1790100000')
      : data === '0x0809' ? jewelReply('9', 'yml1other', '1790100000')
      : jewelReply('11', 'yml1me', '1789000000');
    return new Response(JSON.stringify(reply));
  }) as typeof fetch;
  const out = await offersAddressedTo('yml1me', 1_790_000_500);
  assert.ok(out.ok);
  assert.deepEqual(out.value.map((v) => v.jewel.id), ['4']);
});

test('the jewels an account holds come from x/nft, jewel tokens only', async () => {
  route({ 'abci_query': answer(QueryNFTsResponse.encode(QueryNFTsResponse.fromPartial({
    nfts: [{ classId: 'jewel-1', id: 'jewel-12' }, { classId: 'jewel-2', id: 'jewel-3' }, { classId: 'title-1', id: 'deed-5' }],
  })).finish()) });
  const out = await jewelsHeldBy('yml1me');
  assert.ok(out.ok);
  assert.deepEqual(out.value, ['3', '12']);
  assert.equal(tokenToJewelId('jewel-007'), null);
});

test('jewelers read across pages, and a role is found by business or staff', async () => {
  const page1 = QueryJewelersResponse.encode(QueryJewelersResponse.fromPartial({
    jewelers: [{ id: '1', name: 'Maison Diop', business: 'yml1biz', staff: ['yml1staff'], status: JewelerStatus.JEWELER_ACTIVE }],
    pagination: { nextKey: new Uint8Array([1]) },
  })).finish();
  const page2 = QueryJewelersResponse.encode(QueryJewelersResponse.fromPartial({
    jewelers: [{ id: '2', name: 'Atelier Ba', business: 'yml1ba', staff: [], status: JewelerStatus.JEWELER_SUSPENDED }],
  })).finish();
  let calls = 0;
  globalThis.fetch = (async () => new Response(JSON.stringify(answer(calls++ === 0 ? page1 : page2)))) as typeof fetch;
  const out = await readJewelers();
  assert.ok(out.ok);
  assert.equal(out.value.length, 2);
  assert.deepEqual(roleOf('yml1staff', out.value), { jeweler: out.value[0], business: false, staff: true });
  assert.equal(roleOf('yml1ba', out.value)?.business, true);
  assert.equal(isActive(out.value[1]!), false);
  assert.equal(roleOf('yml1nobody', out.value), null);
});
