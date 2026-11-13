import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { abciQuery, chainHead, setRpcBase, txSearch } from './abci.ts';

const realFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = realFetch; setRpcBase('/api/rpc'); });

function serve(handler: (url: URL) => unknown) {
  const seen: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://test.invalid');
    seen.push(url.toString());
    return new Response(JSON.stringify(handler(url)), { status: 200 });
  }) as typeof fetch;
  return seen;
}

test('an ABCI query quotes its path and hex-encodes its request', async () => {
  setRpcBase('http://node:26957/');
  const seen = serve(() => ({ result: { response: { code: 0, value: 'AQI=', height: '12' } } }));
  const out = await abciQuery('/blockchain.jewel.v1.Query/Jewel', new Uint8Array([8, 7]));
  assert.deepEqual(out, { ok: true, value: new Uint8Array([1, 2]), height: 12 });
  assert.equal(seen[0], 'http://node:26957/abci_query?path=%22%2Fblockchain.jewel.v1.Query%2FJewel%22&data=0x0807');
});

test('a missing record is not-found, a node error is unreachable', async () => {
  serve(() => ({ result: { response: { code: 38, log: 'jewel 9: not found', height: '3' } } }));
  assert.equal((await abciQuery('/p')).ok, false);
  assert.equal(((await abciQuery('/p')) as { reason: string }).reason, 'not-found');
  serve(() => ({ error: { message: 'boom' } }));
  assert.equal(((await abciQuery('/p')) as { reason: string }).reason, 'unreachable');
});

test('tx_search sends its query as a JSON string, newest first, and reads the events', async () => {
  setRpcBase('http://node:26957');
  const seen = serve(() => ({
    result: {
      txs: [{
        hash: 'AB', height: '40',
        tx_result: { code: 0, events: [{ type: 'blockchain.jewel.v1.EventOfferMade',
          attributes: [{ key: 'jewel_id', value: '"4"' }, { key: 'to', value: '"yml1me"' }] }] },
      }],
      total_count: '1',
    },
  }));
  const out = await txSearch(`blockchain.jewel.v1.EventOfferMade.to='"yml1me"'`);
  assert.ok(out.ok);
  assert.equal(out.value[0]!.height, 40);
  assert.equal(out.value[0]!.events[0]!.attributes[1]!.value, '"yml1me"');
  const url = new URL(seen[0]!);
  assert.equal(url.searchParams.get('query'), `"blockchain.jewel.v1.EventOfferMade.to='\\"yml1me\\"'"`);
  assert.equal(url.searchParams.get('order_by'), '"desc"');
});

test('the chain head reads its time as unix seconds', async () => {
  serve(() => ({ result: { node_info: { network: 'yamale-local-1' },
    sync_info: { latest_block_height: '99', latest_block_time: '2026-11-13T10:00:00.123456Z' } } }));
  const out = await chainHead();
  assert.deepEqual(out, { ok: true, height: 99,
    value: { chainId: 'yamale-local-1', height: 99, time: Date.UTC(2026, 10, 13, 10, 0, 0) / 1000 } });
});
