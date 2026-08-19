/**
 * Automatic top-up, so a demonstration never dies on an empty balance.
 *
 * On sign-in, any currency the account holds nothing of is topped up from the
 * faucet. Currencies that already have a balance are left alone, so somebody
 * demonstrating a payment does not have their carefully-arranged figures reset
 * under them mid-sentence.
 *
 * This is a testnet convenience and it is fenced as one: it calls the faucet,
 * which is a devnet service that does not exist on a real network. Nothing
 * above this file assumes money can appear from nowhere, so removing this file
 * is the whole of what it takes to make the app production-shaped.
 */
import { CURRENCIES } from './money.ts';

/** What each empty currency is topped up to, in whole units. */
const TARGET = 1_000;

export interface TopUpResult {
  funded: string[];
  skipped: string[];
}

// Currencies the faucet has refused, remembered so a sign-in does not retry a
// request that just failed and show a banner that never stops appearing.
//
// Remembered *with an expiry*, because the original version treated a refusal
// as permanent and that was wrong twice over. A faucet refuses for two reasons:
// it does not offer the currency at all, or it offers it and has run out. The
// second is temporary, and the chain has now been restocked twice while
// browsers went on believing the currency did not exist. Worse, the memory is
// per-browser and the truth is per-network, so a refusal recorded against one
// chain survives a migration to another and keeps a currency dark on a network
// that serves it perfectly well.
//
// An hour is long enough to stop a retry loop within one session and short
// enough that a restock is picked up without anyone having to clear site data.
const UNAVAILABLE = 'yamale.app.faucet.unavailable.v3';
const REFUSAL_TTL_MS = 60 * 60 * 1000;

function unavailable(): Set<string> {
  try {
    const raw = JSON.parse(localStorage.getItem(UNAVAILABLE) ?? '{}');
    const now = Date.now();
    return new Set(
      Object.entries(raw as Record<string, number>)
        .filter(([, at]) => now - at < REFUSAL_TTL_MS)
        .map(([denom]) => denom),
    );
  } catch {
    return new Set();
  }
}

function markUnavailable(denom: string): void {
  try {
    const raw = JSON.parse(localStorage.getItem(UNAVAILABLE) ?? '{}');
    raw[denom] = Date.now();
    localStorage.setItem(UNAVAILABLE, JSON.stringify(raw));
  } catch {
    localStorage.setItem(UNAVAILABLE, JSON.stringify({ [denom]: Date.now() }));
  }
}

/** True when at least one currency is empty *and* the faucet might serve it. */
export async function needsTopUp(address: string): Promise<boolean> {
  const held = await balances(address);
  const refused = unavailable();
  return CURRENCIES.some(
    (c) => !refused.has(c.denom) && (held.get(c.denom) ?? '0') === '0',
  );
}

export async function topUpEmpty(address: string): Promise<TopUpResult> {
  const funded: string[] = [];
  const skipped: string[] = [];

  const held = await balances(address);

  for (const currency of CURRENCIES) {
    const amount = held.get(currency.denom) ?? '0';
    if (amount !== '0') {
      skipped.push(currency.code);
      continue;
    }
    // Sequential rather than parallel. The faucet signs with one key, so
    // several requests in one block collide on the account sequence and all but
    // the first are silently dropped — the failure looks like a faucet outage
    // and is actually us racing ourselves.
    if (await request(address, currency.denom)) {
      funded.push(currency.code);
    } else {
      // Rate limiting is temporary and worth retrying; a denom the faucet does
      // not hold is not. Both look the same from here, so this errs toward
      // silence — a currency that comes back later is picked up the next time
      // the list is cleared.
      markUnavailable(currency.denom);
    }
  }

  return { funded, skipped };
}

/** Exported so the close-account screen can show what is still held. */
export async function balances(address: string): Promise<Map<string, string>> {
  const map = new Map<string, string>();
  try {
    const res = await fetch(`/api/rest/cosmos/bank/v1beta1/balances/${address}`);
    const json = await res.json();
    for (const b of json.balances ?? []) map.set(b.denom, b.amount);
  } catch {
    // An unreachable node is not a reason to block sign-in. The balance screen
    // will say so in its own words.
  }
  return map;
}

async function request(address: string, denom: string): Promise<boolean> {
  try {
    const res = await fetch('/api/faucet/', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ address, denom, amount: String(TARGET) }),
    });
    return res.ok;
  } catch {
    return false;
  }
}
