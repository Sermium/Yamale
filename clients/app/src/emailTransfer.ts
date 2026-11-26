/**
 * Money sent to an email address, for somebody who may not have an account yet.
 *
 * The money goes into a one-time account whose key is derived from a secret
 * carried in a link, and, when the sender chose one, a code the sender passes
 * on another way (a call, a message). The platform emails the link; the
 * recipient opens it, signs in or creates an account, types the code if there
 * is one, and the one-time account is emptied into theirs.
 *
 * The sender keeps the secret and the code on this device, so they can take
 * the money back for as long as nobody has claimed it.
 *
 * What protects the money:
 *   - the secret travels in the URL fragment, which no server receives when the
 *     link is opened (the platform's mailer does see it, once, to send it);
 *   - with a code, the key is scrypt(code, secret) at 64 MiB: the link alone
 *     opens nothing, and each guess at the code costs a second of work;
 *   - without a code, whoever reads the email can take the money, like cash in
 *     an envelope. The screen says so.
 */

import { DirectSecp256k1Wallet } from '@cosmjs/proto-signing';

import { fromBase64Url, toBase64Url } from '../../sdk/src/jewel.ts';
import { addressOf, claimKey, linkFor, validCode, PAY_ORIGIN } from './emailLink.ts';

export { addressOf, claimKey, linkFor, validCode, PAY_ORIGIN };
import { sha256Hex } from '../../sdk/src/traceabilityauth.ts';
import { ACCOUNTS_API } from '../../shared/account/people.ts';
import { Signer } from './account.ts';
import * as chain from './chain.ts';
import { balances } from './topup.ts';

const SENT = 'yamale.app.sent.';
const RECEIVE = 'yamale.app.receive';
const utf8 = new TextEncoder();

/* ------------------------------------------------ sending */

export interface Sent {
  /** The one-time account. */
  address: string;
  to: string;
  amount: string;
  denom: string;
  secret: string;
  code: string;
  at: string;
  notified: boolean;
}

function readSent(sender: string): Sent[] {
  try { return JSON.parse(localStorage.getItem(SENT + sender) ?? '[]') as Sent[]; } catch { return []; }
}

function writeSent(sender: string, list: Sent[]): void {
  try { localStorage.setItem(SENT + sender, JSON.stringify(list)); } catch { /* private mode: the link is shown to copy */ }
}

export const sentBy = (sender: string): Sent[] => readSent(sender).sort((a, b) => b.at.localeCompare(a.at));

export type SendOutcome =
  | { ok: true; link: string; notified: boolean }
  | { ok: false; reason: 'funding' | 'storage' };

/**
 * Fund a one-time account and email its link. Kept on this device BEFORE the
 * money moves, so a crash between the two cannot strand it.
 */
export async function sendToEmail(signer: Signer, input: {
  to: string; amount: string; denom: string; code: string; amountText: string; lang: string;
}): Promise<SendOutcome> {
  const sender = await signer.internalAddress();
  const secret = crypto.getRandomValues(new Uint8Array(32));
  const key = await claimKey(secret, input.code);
  const address = addressOf(key);
  key.fill(0);
  const record: Sent = {
    address, to: input.to.trim(), amount: input.amount, denom: input.denom,
    secret: toBase64Url(secret), code: input.code, at: new Date().toISOString(), notified: false,
  };
  writeSent(sender, [record, ...readSent(sender)]);
  if (!readSent(sender).some((s) => s.address === address)) return { ok: false, reason: 'storage' };

  const paid = await chain.pay(signer, address, input.amount, input.denom, '');
  if (!paid.ok) {
    writeSent(sender, readSent(sender).filter((s) => s.address !== address));
    return { ok: false, reason: 'funding' };
  }
  const link = linkFor(secret, !!input.code);
  const notified = await notify(signer, {
    to: record.to, sender_name: signer.profile.name, amount: input.amountText, link, with_code: !!input.code, lang: input.lang,
  });
  if (notified) writeSent(sender, readSent(sender).map((s) => (s.address === address ? { ...s, notified: true } : s)));
  return { ok: true, link, notified };
}

async function notify(signer: Signer, body: {
  to: string; sender_name: string; amount: string; link: string; with_code: boolean; lang: string;
}): Promise<boolean> {
  try {
    const bytes = utf8.encode(JSON.stringify(body));
    const headers = await signer.apiHeaders('transfer-notify', sha256Hex(bytes));
    const res = await fetch(`${ACCOUNTS_API}/v1/transfers/notify`, {
      method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: bytes as Uint8Array<ArrayBuffer>,
    });
    return res.ok;
  } catch {
    return false;
  }
}

/** Whether a one-time account still holds the money. */
export async function stillWaiting(address: string): Promise<boolean> {
  const held = await balances(address);
  return [...held.values()].some((v) => BigInt(v) > 0n);
}

/* ------------------------------------------------ receiving, and taking back */

/** On load: a link opened here is kept for after sign-in, and taken out of the address bar. */
export function catchReceiveLink(): void {
  if (typeof location === 'undefined' || !location.hash.includes('receive=')) return;
  const params = new URLSearchParams(location.hash.replace(/^#/, ''));
  const secret = params.get('receive');
  if (!secret || !/^[A-Za-z0-9_-]{43}$/.test(secret)) return;
  try { sessionStorage.setItem(RECEIVE, JSON.stringify({ secret, withCode: params.get('code') === '1' })); } catch { /* kept in memory below */ }
  pendingInMemory = { secret, withCode: params.get('code') === '1' };
  history.replaceState(null, '', location.pathname + location.search);
}

let pendingInMemory: { secret: string; withCode: boolean } | null = null;

export function pendingReceive(): { secret: string; withCode: boolean } | null {
  try {
    const raw = sessionStorage.getItem(RECEIVE);
    if (raw) return JSON.parse(raw);
  } catch { /* fall back to memory */ }
  return pendingInMemory;
}

export function forgetReceive(): void {
  pendingInMemory = null;
  try { sessionStorage.removeItem(RECEIVE); } catch { /* nothing kept */ }
}

export type ClaimOutcome = 'received' | 'empty' | 'failed';

/** Empty the one-time account into `to`: the recipient claiming, or the sender taking it back. */
export async function claimInto(secret: string, code: string, to: string): Promise<ClaimOutcome> {
  const key = await claimKey(fromBase64Url(secret), code);
  const address = addressOf(key);
  if (!(await stillWaiting(address))) { key.fill(0); return 'empty'; }
  const wallet = await DirectSecp256k1Wallet.fromKey(key, 'yml');
  const out = await chain.sweepTo(new Signer({ name: '', email: '' }, wallet, key), to);
  return out.ok && out.sent > 0 ? 'received' : 'failed';
}
