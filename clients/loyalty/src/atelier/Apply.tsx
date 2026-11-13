/**
 * A business applies from the account that will be its own. The office
 * reviews applications and admits a jeweler by a decision of its members;
 * nothing here admits anybody.
 */

import { useState, type FormEvent } from 'react';
import { t } from '@yamale/chain';

import { UnlockGate } from '../account/Account.tsx';
import { useSession } from '../account/session.tsx';
import { submitApplication } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { AppBar } from '../ui.tsx';
import { FailureNote } from './FailureNote.tsx';

export function Apply() {
  const session = useSession();
  const [f, setF] = useState({ name: '', contact: '', email: '', phone: '', country: '', message: '' });
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [failed, setFailed] = useState<{ text: string; raw: string } | null>(null);
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true); setFailed(null);
    try {
      const out = await submitApplication(session.withKey((k) => k),
        { business: session.address!, ...f, country: f.country.trim().toUpperCase() }, CHAIN_ID);
      if (out.ok) setSent(true);
      else setFailed({ text: t('loyalty.applyFailed'), raw: `${out.status} ${out.error}`.trim() });
    } catch (err) {
      setFailed({ text: t('loyalty.applyFailed'), raw: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <AppBar title={t('loyalty.applyTitle')} eyebrow={t('loyalty.title')} back />
      <main className="y-screen">
        <UnlockGate>
          {sent ? <p className="banner banner--ok" role="status">{t('loyalty.applySent')}</p> : (
            <form className="stack" onSubmit={submit}>
              <p className="hint">{t('loyalty.applyLede')}</p>
              {(['name', 'contact', 'email', 'phone', 'country'] as const).map((k) => (
                <div key={k} className="stack">
                  <label className="y-label" htmlFor={`a-${k}`}>{t(`loyalty.apply.${k}`)}</label>
                  <input id={`a-${k}`} className="bigfield" value={f[k]} onChange={set(k)}
                         type={k === 'email' ? 'email' : 'text'} maxLength={k === 'country' ? 2 : k === 'name' || k === 'contact' ? 120 : 254}
                         minLength={k === 'country' || k === 'name' || k === 'contact' ? 2 : undefined} pattern={k === 'country' ? '[A-Za-z]{2}' : undefined}
                         autoCapitalize={k === 'country' ? 'characters' : undefined} required={k !== 'phone'} />
                </div>
              ))}
              <label className="y-label" htmlFor="a-message">{t('loyalty.apply.message')}</label>
              <textarea id="a-message" className="bigfield" rows={4} maxLength={2000} value={f.message} onChange={set('message')} />
              {failed && <FailureNote text={failed.text} raw={failed.raw} />}
              <button className="btn btn--primary" disabled={busy}>{t('loyalty.applyButton')}</button>
            </form>
          )}
        </UnlockGate>
      </main>
    </>
  );
}
