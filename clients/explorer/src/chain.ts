/**
 * The explorer's connection to the chain, and the view mode that shapes what it
 * shows.
 */

import { createContext, useContext } from 'react';
import { ChainClient } from '@yamale/chain';

/**
 * In development everything goes through the Vite proxy, so the node does not
 * need CORS opened up. In production these are set at build time to the public
 * endpoints.
 */
export const client = new ChainClient({
  restUrl: import.meta.env.VITE_REST_URL ?? '/api/rest',
  rpcUrl: import.meta.env.VITE_RPC_URL ?? '/api/rpc',
});

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

/** Classifies a pasted search term so the explorer can route it. */
export type SearchKind = 'address' | 'tx' | 'height' | 'unknown';

export function classifySearch(term: string): SearchKind {
  const value = term.trim();
  if (!value) return 'unknown';
  if (/^\d+$/.test(value)) return 'height';
  if (/^[0-9A-Fa-f]{64}$/.test(value)) return 'tx';
  if (/^yml[0-9a-z]{38,}$/.test(value)) return 'address';
  return 'unknown';
}
