/**
 * Asking another account of the apps for money, and answering such requests.
 * Kept by the accounts API (tools/traceabilityapi/requests.go); every call is
 * signed with this account's key, and a request is marked paid only when the
 * chain shows the transfer.
 */

import { sha256Hex } from '../../sdk/src/traceabilityauth.ts';
import { ACCOUNTS_API } from '../../shared/account/people.ts';
import type { Signer } from './account.ts';
import * as chain from './chain.ts';

export interface MoneyRequest {
  id: string;
  from: string;
  to: string;
  amount: string;
  denom: string;
  note: string;
  status: 'open' | 'paid' | 'declined' | 'cancelled';
  tx?: string;
  created: number;
  updated: number;
  from_name?: string;
  to_name?: string;
}

export type Outcome<T> = { ok: true; value: T } | { ok: false; status: number; error: string };

const utf8 = new TextEncoder();

async function call<T>(path: string, init: RequestInit): Promise<Outcome<T>> {
  try {
    const res = await fetch(`${ACCOUNTS_API}${path}`, init);
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String((body as { error?: unknown }).error ?? '') };
    return { ok: true, value: body as T };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}

/** Ask somebody, by email or identifier, for an amount. Refused when nobody has that account. */
export async function askFor(signer: Signer, input: {
  to: string; amount: string; denom: string; note: string; lang: string; amountText: string;
}): Promise<Outcome<{ id: string; to: string; to_name: string; notified: boolean }>> {
  const bytes = utf8.encode(JSON.stringify({
    to: input.to.trim(), amount: input.amount, denom: input.denom, note: input.note.trim(), lang: input.lang, amount_text: input.amountText,
  }));
  return call('/v1/requests', {
    method: 'POST',
    headers: { ...(await signer.apiHeaders('request-create', sha256Hex(bytes))), 'Content-Type': 'application/json' },
    body: bytes as Uint8Array<ArrayBuffer>,
  });
}

/** This account's requests, both ways, newest first. */
export async function myRequests(signer: Signer): Promise<Outcome<{ incoming: MoneyRequest[]; outgoing: MoneyRequest[] }>> {
  const me = await signer.internalAddress();
  return call('/v1/requests', { headers: await signer.apiHeaders('requests-read', me) });
}

async function update(signer: Signer, id: string, action: 'paid' | 'decline' | 'cancel', tx = ''): Promise<Outcome<MoneyRequest>> {
  return call(`/v1/requests/${id}/${action}`, {
    method: 'POST',
    headers: { ...(await signer.apiHeaders('request-update', `${id}:${action}:${tx}`)), 'Content-Type': 'application/json' },
    ...(action === 'paid' ? { body: JSON.stringify({ tx }) } : {}),
  });
}

export const decline = (signer: Signer, id: string) => update(signer, id, 'decline');
export const cancel = (signer: Signer, id: string) => update(signer, id, 'cancel');

/** Pay a request: the transfer, then the request marked paid with its hash. */
export async function payRequest(signer: Signer, q: MoneyRequest): Promise<'paid' | 'transfer-failed' | 'not-recorded'> {
  const out = await chain.pay(signer, q.from, q.amount, q.denom, `Yamale request ${q.id}`);
  if (!out.ok) return 'transfer-failed';
  // The chain indexes a committed transaction a moment after the block.
  for (let i = 0; i < 4; i++) {
    const r = await update(signer, q.id, 'paid', out.hash);
    if (r.ok) return 'paid';
    if (r.status !== 409) break;
    await new Promise((res) => setTimeout(res, 2500));
  }
  return 'not-recorded';
}
