/**
 * The payment being composed, shared with the screen outside the phone.
 *
 * The desk beside the phone shows the same payment as an ISO 20022 instruction
 * while it is being typed. That needs one piece of state in two React trees
 * that are siblings, and the alternatives were threading it through App.tsx —
 * which is 2,200 lines and does not need another prop — or a context provider
 * wrapping both, which is the same edit with more ceremony.
 *
 * So: one module-level value and a subscriber set, read through
 * `useSyncExternalStore`. It is the smallest thing that is correct, it has no
 * dependency, and being outside React means the pure half is testable without
 * a renderer.
 */
import { useSyncExternalStore } from 'react';

import type { Draft } from './iso.ts';

export type DraftState = Draft | null;

let current: DraftState = null;
const listeners = new Set<() => void>();

/**
 * Replace the draft.
 *
 * Compared field by field before notifying. Pay re-renders on every keystroke
 * and would otherwise publish a new object each time, waking every subscriber
 * to re-render identical output — and on the desk that is a ten-row table
 * rebuilt per character.
 */
export function publish(next: DraftState): void {
  if (same(current, next)) return;
  current = next;
  for (const l of listeners) l();
}

export function snapshot(): DraftState {
  return current;
}

export function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function same(a: DraftState, b: DraftState): boolean {
  if (a === b) return true;
  if (a === null || b === null) return false;
  return a.debtorAddress === b.debtorAddress
    && a.debtorUserId === b.debtorUserId
    && a.creditorAddress === b.creditorAddress
    && a.creditorUserId === b.creditorUserId
    && a.denom === b.denom
    && a.amount === b.amount
    && a.purposeCode === b.purposeCode
    && a.remittanceInformation === b.remittanceInformation
    && a.endToEndId === b.endToEndId;
}

export function useDraft(): DraftState {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

/** Only for tests, which would otherwise leak state between cases. */
export function reset(): void {
  current = null;
  listeners.clear();
}
