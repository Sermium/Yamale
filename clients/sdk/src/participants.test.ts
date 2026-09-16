import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { ChainClient } from './client.ts';

const clientWith = (body: unknown) =>
  new ChainClient({
    restUrl: '/api/rest',
    rpcUrl: '/api/rpc',
    fetchImpl: (async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })) as typeof fetch,
  });

test('an empty register is an empty list, not a failure', async () => {
  // This is the LIVE state of devnet-2, measured 2026-09-15. A dropdown that
  // threw here would show a loading error on the one screen that most needs to
  // say something true.
  const c = clientWith({ approved_participant: [], pagination: { next_key: null, total: '0' } });
  assert.deepEqual(await c.approvedParticipants(), []);
});

test('participants come back with the three fields a dropdown needs', async () => {
  const c = clientWith({
    approved_participant: [
      { participant: 'yml1bank', code: 'SN-CBAO', name: 'CBAO Senegal', payload_store_url: '' },
    ],
    pagination: { next_key: null, total: '1' },
  });
  assert.deepEqual(await c.approvedParticipants(), [
    { participant: 'yml1bank', code: 'SN-CBAO', name: 'CBAO Senegal' },
  ]);
});
