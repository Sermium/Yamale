/**
 * Sending money to an email address, and receiving it from a link.
 * The mechanics are in emailTransfer.ts; this is what a person sees.
 */

import { useEffect, useState, type FormEvent } from 'react';
import { getLocale, t } from '@yamale/chain';

import type { Signer } from './account.ts';
import {
  claimInto, forgetReceive, pendingReceive, sendToEmail, sentBy, stillWaiting, validCode, type Sent,
} from './emailTransfer.ts';
import { CURRENCIES, display, parseAmount, defaultDenom } from './money.ts';
import { balances } from './topup.ts';

const emailLike = (s: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s.trim());

export function EmailSend({ signer }: { signer: Signer }) {
  const [to, setTo] = useState('');
  const [denom, setDenom] = useState(defaultDenom);
  const [amount, setAmount] = useState('');
  const [withCode, setWithCode] = useState(true);
  const [code, setCode] = useState('');
  const [held, setHeld] = useState<Map<string, string> | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ link: string; notified: boolean; to: string; code: string } | null>(null);
  const [error, setError] = useState('');
  const [listVersion, setListVersion] = useState(0);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let live = true;
    void signer.internalAddress().then(balances).then((m) => { if (live) setHeld(m); });
    return () => { live = false; };
  }, [signer, listVersion]);

  const usable = CURRENCIES.filter((c) => BigInt(held?.get(c.denom) ?? '0') > 0n);
  const parsed = amount.trim() === '' ? null : parseAmount(amount, denom);
  const tooMuch = parsed && held ? BigInt(parsed.base) > BigInt(held.get(denom) ?? '0') : false;
  const ready = emailLike(to) && parsed && BigInt(parsed.base) > 0n && !tooMuch && (!withCode || validCode(code));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!ready || !parsed) return;
    setBusy(true); setError(''); setCopied(false);
    const out = await sendToEmail(signer, {
      to, amount: parsed.base, denom, code: withCode ? code : '', amountText: display(parsed.base, denom), lang: getLocale(),
    });
    setBusy(false);
    if (!out.ok) { setError(t(out.reason === 'funding' ? 'app.emailSendFailed' : 'app.emailStorageFailed')); return; }
    setResult({ link: out.link, notified: out.notified, to: to.trim(), code: withCode ? code : '' });
    setTo(''); setAmount(''); setCode('');
    setListVersion((v) => v + 1);
  }

  if (result) {
    return (
      <div className="stack-lg">
        <div className="card">
          <strong>{t(result.notified ? 'app.emailSent' : 'app.emailNotSent', { email: result.to })}</strong>
          {result.code && <p className="muted">{t('app.emailTellCode', { code: result.code })}</p>}
          {!result.notified && (<>
            <p className="code">{result.link}</p>
            <button type="button" className="ghost" onClick={async () => {
              try { await navigator.clipboard.writeText(result.link); setCopied(true); } catch { /* no clipboard */ }
            }}>{copied ? t('app.copied') : t('app.copyLink')}</button>
          </>)}
          <button type="button" className="primary" onClick={() => setResult(null)}>{t('app.emailAnother')}</button>
        </div>
        <SentList signer={signer} version={listVersion} onChange={() => setListVersion((v) => v + 1)} />
      </div>
    );
  }

  return (
    <div className="stack-lg">
      <form className="form" onSubmit={submit}>
        <p className="muted">{t('app.emailLede')}</p>
        <label>
          <span>{t('app.emailTo')}</span>
          <input type="email" value={to} onChange={(e) => setTo(e.target.value)} required autoComplete="email" />
        </label>
        <label>
          <span>{t('app.emailCurrency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {(usable.length ? usable : CURRENCIES).map((c) => <option key={c.denom} value={c.denom}>{c.code} · {c.name}</option>)}
          </select>
        </label>
        <label>
          <span>{t('app.emailAmount')}</span>
          <input className="y-num" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} required />
        </label>
        {held && <p className="muted">{t('app.emailHeld', { amount: display(held.get(denom) ?? '0', denom) })}</p>}
        {tooMuch && <p className="notice notice--bad">{t('app.emailTooMuch')}</p>}
        <label className="check">
          <input type="checkbox" checked={withCode} onChange={(e) => setWithCode(e.target.checked)} />
          {t('app.emailWithCode')}
        </label>
        {withCode ? (<>
          <label>
            <span>{t('app.emailCode')}</span>
            <input inputMode="numeric" value={code} maxLength={8} onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 8))} />
          </label>
          <p className="muted">{t('app.emailCodeHint')}</p>
        </>) : <p className="notice notice--bad">{t('app.emailNoCodeWarning')}</p>}
        {error && <p className="notice notice--bad">{error}</p>}
        <button className="primary" disabled={!ready || busy}>{busy ? t('app.working') : t('app.emailSend')}</button>
      </form>
      <SentList signer={signer} version={listVersion} onChange={() => setListVersion((v) => v + 1)} />
    </div>
  );
}

function SentList({ signer, version, onChange }: { signer: Signer; version: number; onChange: () => void }) {
  const [list, setList] = useState<(Sent & { waiting: boolean | null })[]>([]);
  const [busy, setBusy] = useState('');
  useEffect(() => {
    let live = true;
    (async () => {
      const mine = sentBy(await signer.internalAddress());
      if (live) setList(mine.map((s) => ({ ...s, waiting: null })));
      const states = await Promise.all(mine.map((s) => stillWaiting(s.address).catch(() => null)));
      if (live) setList(mine.map((s, i) => ({ ...s, waiting: states[i] })));
    })();
    return () => { live = false; };
  }, [signer, version]);
  if (list.length === 0) return null;
  return (
    <section className="stack">
      <h3 className="section-title">{t('app.emailSentTitle')}</h3>
      {list.map((s) => (
        <div key={s.address} className="card">
          <strong>{display(s.amount, s.denom)}</strong>
          <p className="muted">{t('app.emailSentTo', { email: s.to, date: new Date(s.at).toLocaleDateString(getLocale()) })}</p>
          <p className="muted">{s.waiting === null ? t('app.checking') : s.waiting ? t('app.emailWaiting') : t('app.emailDone')}</p>
          {s.waiting && s.code && <p className="muted">{t('app.emailCodeIs', { code: s.code })}</p>}
          {s.waiting && (
            <button type="button" className="ghost" disabled={busy === s.address} onClick={async () => {
              setBusy(s.address);
              await claimInto(s.secret, s.code, await signer.internalAddress());
              setBusy('');
              onChange();
            }}>{busy === s.address ? t('app.working') : t('app.emailTakeBack')}</button>
          )}
        </div>
      ))}
    </section>
  );
}

/** Shown over every screen while a received link waits to be claimed. */
export function ReceivePanel({ signer }: { signer: Signer }) {
  const [pending] = useState(pendingReceive);
  const [code, setCode] = useState('');
  const [state, setState] = useState<'idle' | 'busy' | 'received' | 'empty' | 'failed' | 'gone'>('idle');
  if (!pending || state === 'gone') return null;
  if (state === 'received' || state === 'empty') {
    return (
      <div className="card">
        <strong>{t(state === 'received' ? 'app.receiveDone' : 'app.receiveEmpty')}</strong>
        <button type="button" className="ghost" onClick={() => { forgetReceive(); setState('gone'); }}>{t('app.close')}</button>
      </div>
    );
  }
  return (
    <form className="card form" onSubmit={async (e) => {
      e.preventDefault();
      setState('busy');
      const out = await claimInto(pending.secret, pending.withCode ? code : '', await signer.internalAddress());
      // With a code, nothing found may just be a mistyped code: the form stays.
      if (pending.withCode && out === 'empty') { setState('failed'); return; }
      if (out !== 'failed') forgetReceive();
      setState(out);
    }}>
      <strong>{t('app.receiveTitle')}</strong>
      <p className="muted">{t(pending.withCode ? 'app.receiveWithCode' : 'app.receiveLede')}</p>
      {pending.withCode && (
        <label>
          <span>{t('app.emailCode')}</span>
          <input inputMode="numeric" value={code} maxLength={8} autoFocus onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 8))} />
        </label>
      )}
      {state === 'failed' && <p className="notice notice--bad">{t(pending.withCode ? 'app.receiveWrongCode' : 'app.receiveFailed')}</p>}
      <button className="primary" disabled={state === 'busy' || (pending.withCode && !validCode(code))}>
        {state === 'busy' ? t('app.working') : t('app.receiveButton')}
      </button>
    </form>
  );
}
