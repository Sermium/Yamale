import { useState } from 'react';
import { t, type SubmitResult } from '@yamale/chain';

import { SponsorshipUnavailable } from '../account/session.tsx';
import { errorKey } from './errors.ts';

export type ActionState =
  | { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; result: SubmitResult }
  | { kind: 'failed'; text: string; raw: string };

/** One signed action: busy while it runs, then the chain's verdict in words. */
export function useAction(): [ActionState, (fn: () => Promise<SubmitResult>) => Promise<SubmitResult | null>] {
  const [state, setState] = useState<ActionState>({ kind: 'idle' });
  async function run(fn: () => Promise<SubmitResult>): Promise<SubmitResult | null> {
    setState({ kind: 'busy' });
    try {
      const result = await fn();
      if (result.succeeded) {
        setState({ kind: 'done', result });
        return result;
      }
      const raw = result.error?.raw ?? result.raw.rawLog ?? '';
      const key = errorKey(raw);
      setState({ kind: 'failed', text: key ? t(`traceability.err.${key}`) : (result.error?.message ?? t('traceability.err.unknown')), raw });
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err);
      setState({ kind: 'failed', text: err instanceof SponsorshipUnavailable ? t('traceability.err.sponsorship') : t('traceability.err.unknown'), raw });
    }
    return null;
  }
  return [state, run];
}

export function ActionStatus({ state, done }: { state: ActionState; done?: string }) {
  if (state.kind === 'busy') return <p className="loading" role="status">{t('traceability.sending')}</p>;
  if (state.kind === 'done' && done) return <p className="banner banner--ok" role="status">{done}</p>;
  if (state.kind !== 'failed') return null;
  return (
    <div className="note note--warn" role="alert">
      <p>{state.text}</p>
      {state.raw && <details className="advanced"><summary>{t('traceability.advanced')}</summary><pre>{state.raw}</pre></details>}
    </div>
  );
}

/** A value from a typed event of a committed transaction, JSON-decoded. */
export function eventValue(result: SubmitResult, type: string, key: string): string | null {
  for (const e of result.raw.events ?? []) {
    if (e.type !== type) continue;
    const a = e.attributes.find((x) => x.key === key);
    if (a) {
      try { return String(JSON.parse(a.value)); } catch { return a.value; }
    }
  }
  return null;
}

/** Every value of a typed event's attribute in a committed transaction, in message order. */
export function eventValues(result: SubmitResult, type: string, key: string): string[] {
  const out: string[] = [];
  for (const e of result.raw.events ?? []) {
    if (e.type !== type) continue;
    const a = e.attributes.find((x) => x.key === key);
    if (!a) continue;
    try { out.push(String(JSON.parse(a.value))); } catch { out.push(a.value); }
  }
  return out;
}
