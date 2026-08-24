/**
 * The explorer's connection to the chain, and the view mode that shapes what it
 * shows.
 */

import { createContext, useContext } from 'react';
import { ChainClient } from '@yamale/chain';

import { stalledAtHeight } from './health.ts';

/**
 * In development everything goes through the Vite proxy, so the node does not
 * need CORS opened up. In production these are set at build time to the public
 * endpoints.
 */
export const REST_URL = import.meta.env.VITE_REST_URL ?? '/api/rest';
export const RPC_URL = import.meta.env.VITE_RPC_URL ?? '/api/rpc';

/**
 * The height every state query is being answered at while the chain is halted,
 * or null when the node is answering from the tip.
 *
 * Set only by the node's own refusal — never guessed — and cleared the moment a
 * plain request succeeds again, so the explorer cannot end up showing Tuesday's
 * state on Thursday because it never un-pinned.
 */
let pinnedHeight: number | null = null;

/** What the strip reads to say "these figures are historical". */
export function readingAtHeight(): number | null {
  return pinnedHeight;
}

/**
 * A fetch that survives a halted chain.
 *
 * A Cosmos node executes every query against the state left by the last block it
 * finalised. A node that is running but has finalised none — the chain halted
 * and the node was restarted — therefore refuses *every* REST query rather than
 * serving stale state, and names the height it is stuck at in the refusal. Ask
 * for that height explicitly through `x-cosmos-block-height` and the same query
 * answers perfectly.
 *
 * So this is the difference between an explorer that is a blank page during an
 * outage and one that still shows the validator set, the balances and the last
 * thing that happened. It matters most exactly when it appears: a chain halts
 * because a validator is gone, and that is when somebody opens this page.
 *
 * The tip is always tried first. An extra failed request during an outage is
 * cheap; a pin that outlives the outage is a page confidently showing history as
 * though it were current.
 */
const stallAwareFetch: typeof fetch = async (input, init) => {
  const response = await fetch(input, init);
  if (response.ok) {
    pinnedHeight = null;
    return response;
  }

  // Read a copy: the caller still needs the body if this turns out not to be a
  // stall.
  const body = await response
    .clone()
    .text()
    .catch(() => '');
  const stalled = stalledAtHeight(body);
  if (!stalled) return response;

  pinnedHeight = stalled;
  return fetch(input, {
    ...init,
    headers: { ...(init?.headers ?? {}), 'x-cosmos-block-height': String(stalled) },
  });
};

export const client = new ChainClient({
  restUrl: REST_URL,
  rpcUrl: RPC_URL,
  fetchImpl: stallAwareFetch,
});

/**
 * Whether the node's REST API is answering, and from where.
 *
 * A separate probe rather than a flag on some other query, because "the RPC
 * answers and REST does not" is a real and common state here — the node's REST
 * sits behind a deny-by-default gate, and a misconfigured one produces a page
 * that looks like a halted chain and is not one.
 */
export async function probeRest(): Promise<{ ok: boolean; stalledAt: number | null }> {
  try {
    const response = await fetch(`${REST_URL}/cosmos/staking/v1beta1/pool`);
    if (response.ok) return { ok: true, stalledAt: null };
    const body = await response.text().catch(() => '');
    return { ok: false, stalledAt: stalledAtHeight(body) };
  } catch {
    return { ok: false, stalledAt: null };
  }
}

/**
 * Which explorer somebody is looking at.
 *
 * `simple` answers "did the money move, and where is mine?" — it shows
 * everyday activity in sentences and hides the machinery entirely.
 *
 * `expert` answers "what exactly happened?" — every message, every hash, gas,
 * signatures and the raw payload.
 *
 * The same URL works in both. A person who found their payment in the simple
 * view can send the link to an engineer, who will see the full transaction at
 * the same address. Making them two different sites would break that, and
 * that hand-off is exactly when an explorer earns its keep.
 */
export type ViewMode = 'simple' | 'expert';

export const ViewModeContext = createContext<{
  mode: ViewMode;
  setMode: (mode: ViewMode) => void;
}>({ mode: 'simple', setMode: () => {} });

export function useViewMode() {
  return useContext(ViewModeContext);
}

export const VIEW_MODE_STORAGE_KEY = 'yamale.explorer.view';

/**
 * Resolves the mode to start in: an explicit `?view=` wins so shared links
 * land somebody where the sender intended, otherwise their own last choice,
 * otherwise simple.
 *
 * Defaulting to simple is deliberate. Somebody who needs the expert view knows
 * it exists and will find the switch; somebody who does not will be lost by a
 * page of hex before they ever look for a toggle.
 */
export function initialViewMode(search: string): ViewMode {
  const requested = new URLSearchParams(search).get('view');
  if (requested === 'simple' || requested === 'expert') return requested;

  try {
    const stored = localStorage.getItem(VIEW_MODE_STORAGE_KEY);
    if (stored === 'simple' || stored === 'expert') return stored;
  } catch {
    // Storage can be unavailable in private modes; the default is fine.
  }

  return 'simple';
}

// Search classification moved to search.ts, where it can be tested without a
// bundler and where it grew from three kinds to six — a user ID, a currency and
// a validator name are all things somebody pastes, and all three used to be
// answered with "that does not look like an account, a transaction or a block
// number".
export { classifySearch, type SearchGuess, type SearchKind } from './search.ts';
