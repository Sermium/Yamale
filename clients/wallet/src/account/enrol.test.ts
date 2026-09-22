/**
 * Enrolment, and the one assertion that cannot be skipped.
 *
 * The guarantee this feature is sold on is that enrolling an existing account
 * does not move it. Everything else here is plumbing; the test that matters is
 * that the key derived for the services is the key that controls the address
 * the wallet already shows.
 *
 * Get the HD path wrong and every visible signal still says success — the
 * deposit is accepted, a recovery returns a key — and the key opens a different
 * account from the one holding the money. There is no later moment at which
 * anybody would notice.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';

import {
  addressOf,
  enrol,
  EnrolError,
  privateKeyFromMnemonic,
  servicesAvailable,
} from './enrol.ts';
import { restore, ROLE_CUSTODIAN, ROLE_DEVICE, ROLE_RECOVERY } from './protect.ts';
import type { Holding } from './deposit.ts';

// A throwaway mnemonic. Never used anywhere but here.
const MNEMONIC =
  'surround miss nominee dream gap cross assault thank captain prosper drop duty ' +
  'group candy wealth weather scale put';
const EMAIL = 'amara@example.org';
const PASSWORD = 'seven horses eating apples slowly';

const SERVICES = { custodianUrl: 'https://custodian.test', recoveryUrl: 'https://recovery.test' };

/** A stand-in for the two services, which records what it was told. */
function fakeServices(options: { status?: number; address?: string } = {}) {
  const seen: { url: string; body: any }[] = [];
  const fetchImpl = (async (url: string | URL, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body ?? '{}'));
    seen.push({ url: String(url), body });
    const status = options.status ?? 200;
    return new Response(
      JSON.stringify(
        status === 200 ? { address: options.address ?? 'yml1theaccount', role: body.holding.role } : { error: 'no' },
      ),
      { status, headers: { 'content-type': 'application/json' } },
    );
  }) as unknown as typeof fetch;
  return { seen, fetchImpl };
}

function decodeHolding(raw: any): Holding {
  const bytes = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
  return {
    role: raw.role,
    salt: bytes(raw.salt),
    nonce: bytes(raw.nonce),
    time: raw.time,
    memory: raw.memory,
    threads: raw.threads,
    share: { x: raw.share.x, y: bytes(raw.share.y) },
  };
}

/* ------------------------------------------------- the address is preserved */

test('the derived key controls the address the wallet already had', async () => {
  // The reference is CosmJS itself, constructed exactly as vault.ts constructs
  // it. If this ever fails, enrolment is filing the wrong key against a real
  // account and the failure is silent everywhere else.
  const wallet = await DirectSecp256k1HdWallet.fromMnemonic(MNEMONIC, { prefix: 'yml' });
  const [account] = await wallet.getAccounts();

  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  assert.equal(
    await addressOf(privateKey),
    account!.address,
    'the enrolment path derives a different key from the one the wallet signs with',
  );
});

test('a different account index is a different address', async () => {
  // Confirms the index is actually used, so the assertion above is about the
  // path rather than about two functions that both ignore their arguments.
  const zero = await privateKeyFromMnemonic(MNEMONIC, 0);
  const one = await privateKeyFromMnemonic(MNEMONIC, 1);
  assert.notEqual(await addressOf(zero), await addressOf(one));
});

/* --------------------------------------------------------- what is sent */

test('each service is told a different holding, and neither is told the device one', async () => {
  const { seen, fetchImpl } = fakeServices();
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  const { deviceHolding } = await enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl);

  assert.equal(seen.length, 2, 'both services should have been told exactly once');
  const roles = seen.map((s) => s.body.holding.role);
  assert.deepEqual(roles, [ROLE_CUSTODIAN, ROLE_RECOVERY]);

  // A service given two of the three could attempt a recovery alone.
  for (const s of seen) {
    assert.notEqual(s.body.holding.role, ROLE_DEVICE, 'a service was sent the device holding');
    assert.notEqual(
      s.body.holding.share.x,
      deviceHolding.share.x,
      'a service was sent the share the device keeps',
    );
  }
  assert.notEqual(
    seen[0]!.body.holding.share.x,
    seen[1]!.body.holding.share.x,
    'both services were sent the same share',
  );
});

test('the mnemonic and the private key are never in a request', async () => {
  const { seen, fetchImpl } = fakeServices();
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  await enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl);

  const keyHex = Array.from(privateKey, (b) => b.toString(16).padStart(2, '0')).join('');
  for (const s of seen) {
    const wire = JSON.stringify(s.body);
    assert.ok(!wire.includes(MNEMONIC), 'the mnemonic was sent');
    assert.ok(!wire.includes(keyHex), 'the private key was sent');
    assert.ok(!wire.includes(btoa(String.fromCharCode(...privateKey))), 'the private key was sent');
  }
});

test('the holdings the services were given, plus the password, rebuild the key', async () => {
  // End to end through the wire format. Encoding the holdings wrongly — a
  // base64 slip, a dropped field — would still produce two plausible deposits,
  // and only a reconstruction proves they are the right bytes.
  const { seen, fetchImpl } = fakeServices();
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  await enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl);

  const returned = seen.map((s) => decodeHolding(s.body.holding));
  const restored = await restore(returned, PASSWORD);
  assert.deepEqual(restored, privateKey, 'the two service holdings did not rebuild the key');
  assert.equal(await addressOf(restored), await addressOf(privateKey));
});

test('the device holding also works, paired with either service', async () => {
  const { seen, fetchImpl } = fakeServices();
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  const { deviceHolding } = await enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl);

  for (const s of seen) {
    const restored = await restore([deviceHolding, decodeHolding(s.body.holding)], PASSWORD);
    assert.deepEqual(restored, privateKey, `device + ${s.body.holding.role} failed`);
  }
});

/* ------------------------------------------------------------ failures */

test('a refusal from a service is reported by name', async () => {
  const { fetchImpl } = fakeServices({ status: 400 });
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  await assert.rejects(
    () => enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl),
    (e: unknown) => e instanceof EnrolError && e.role === ROLE_CUSTODIAN,
  );
});

test('an already-enrolled service is not a failure', async () => {
  // 409 means this email already has an account there. Re-running an enrolment
  // that half-completed is how somebody finishes the half that is missing, so
  // treating it as an error would leave them permanently stuck.
  const { fetchImpl } = fakeServices({ status: 409 });
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  const { address, deviceHolding } = await enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl);

  // No service reported an address, so it is derived locally rather than left
  // blank — the caller needs to know which account this was.
  assert.equal(address, await addressOf(privateKey));
  assert.ok(deviceHolding.share.y.length > 0);
});

test('services that disagree about the address stop the enrolment', async () => {
  // Each service derives the address from the public key it verified. If they
  // report different ones, something between the device and one of them
  // changed the deposit, and continuing would file half an account.
  let call = 0;
  const fetchImpl = (async () =>
    new Response(JSON.stringify({ address: call++ === 0 ? 'yml1one' : 'yml1two' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch;

  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  await assert.rejects(
    () => enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl),
    (e: unknown) => e instanceof EnrolError && /different addresses/.test((e as Error).message),
  );
});

test('an unreachable service is reported as unreachable, not as a refusal', async () => {
  const fetchImpl = (async () => {
    throw new TypeError('Failed to fetch');
  }) as unknown as typeof fetch;
  const privateKey = await privateKeyFromMnemonic(MNEMONIC);
  await assert.rejects(
    () => enrol(privateKey, EMAIL, PASSWORD, SERVICES, fetchImpl),
    (e: unknown) => e instanceof EnrolError && /could not be reached/.test((e as Error).message),
  );
});

/* ---------------------------------------------------------- availability */

function reachability(map: Record<string, number | 'hang' | 'throw'>) {
  return (async (url: string | URL, init?: RequestInit) => {
    const key = String(url).includes('custodian') ? 'custodian' : 'recovery';
    const outcome = map[key];
    if (outcome === 'throw') throw new TypeError('Failed to fetch');
    if (outcome === 'hang') {
      // Never resolves on its own: the abort signal is what has to end it.
      return new Promise<Response>((_, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new Error('aborted')));
      });
    }
    return new Response('{}', { status: outcome as number });
  }) as unknown as typeof fetch;
}

test('both services have to answer, because one is not half a protection', async () => {
  // Deposited with one service only, the account is recoverable from the device
  // alone — which is the situation enrolment exists to leave. Offering the form
  // would promise something it cannot deliver.
  assert.equal(
    await servicesAvailable(SERVICES, reachability({ custodian: 200, recovery: 200 })),
    true,
  );
  assert.equal(
    await servicesAvailable(SERVICES, reachability({ custodian: 200, recovery: 'throw' })),
    false,
    'one reachable service was treated as enough',
  );
  assert.equal(
    await servicesAvailable(SERVICES, reachability({ custodian: 'throw', recovery: 200 })),
    false,
    'one reachable service was treated as enough',
  );
});

test('a route that exists but is not the service is not available', async () => {
  // The realistic failure on a host where nginx serves the site but has no
  // account routes: the catch-all answers, with a 404. A truthy Response is not
  // an available service.
  assert.equal(
    await servicesAvailable(SERVICES, reachability({ custodian: 404, recovery: 404 })),
    false,
  );
  assert.equal(
    await servicesAvailable(SERVICES, reachability({ custodian: 200, recovery: 502 })),
    false,
  );
});

test('a service that never answers times out rather than hanging the screen', async () => {
  // A network that blackholes the request would otherwise leave the panel on
  // "checking" for as long as the browser allows, which reads as broken rather
  // than as absent.
  const started = Date.now();
  const available = await servicesAvailable(
    SERVICES,
    reachability({ custodian: 'hang', recovery: 200 }),
    150,
  );
  assert.equal(available, false);
  assert.ok(Date.now() - started < 2000, 'the probe did not give up');
});
