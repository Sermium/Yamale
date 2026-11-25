import { isActive, readJewelers, roleOf, type Jeweler } from '@yamale/chain';

import { useLoad } from '../../ui.tsx';

export type RoleState =
  | { state: 'loading' } | { state: 'none' }
  | { state: 'failed'; outcome: { reason: string; detail: string } }
  | { state: 'role'; jeweler: Jeweler; business: boolean; staff: boolean; active: boolean };

const NO_JEWELERS = { ok: true as const, value: [] as Jeweler[], height: 0 };

/** What this account is to a jeweler, read from the chain: its business, its staff, or neither. */
export function useRole(address: string | null): [RoleState, () => void] {
  const [jewelers, reload] = useLoad(() => (address ? readJewelers() : Promise.resolve(NO_JEWELERS)), [address]);
  if (!address) return [{ state: 'none' }, reload];
  if (!jewelers) return [{ state: 'loading' }, reload];
  if (!jewelers.ok) return [{ state: 'failed', outcome: jewelers }, reload];
  const r = roleOf(address, jewelers.value);
  return [r ? { state: 'role', ...r, active: isActive(r.jeweler) } : { state: 'none' }, reload];
}
