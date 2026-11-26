/**
 * Money requests between accounts of the apps: asking an existing user, and
 * answering what others asked. The mechanics are in moneyRequests.ts.
 */

import { useEffect, useState, type FormEvent } from 'react';
import { getLocale, t } from '@yamale/chain';

import type { Signer } from './account.ts';
import { askFor, cancel, decline, myRequests, payRequest, type MoneyRequest } from './moneyRequests.ts';
import { CURRENCIES, display, parseAmount, defaultDenom } from './money.ts';

const short = (a: string) => `${a.slice(0, 8)}…${a.slice(-4)}`;
const when = (unix: number) => new Date(unix * 1000).toLocaleDateString(getLocale());

function useRequests(signer: Signer, version: number) {
  const [data, setData] = useState<{ incoming: MoneyRequest[]; outgoing: MoneyRequest[] } | null>(null);
  useEffect(() => {
    let live = true;
    void myRequests(signer).then((r) => { if (live && r.ok) setData(r.value); });
    return () => { live = false; };
  }, [signer, version]);
  return data;
}

/** On the home screen: what others asked of this account, to pay or decline. */
export function IncomingRequests({ signer }: { signer: Signer }) {
  const [version, setVersion] = useState(0);
  const data = useRequests(signer, version);
  const [busy, setBusy] = useState('');
  const [note, setNote] = useState<{ id: string; key: string } | null>(null);
  const open = (data?.incoming ?? []).filter((q) => q.status === 'open');
  if (open.length === 0) return null;
  return (
    <section className="stack">
      <h3 className="section-title">{t('app.requestsIncoming')}</h3>
      {open.map((q) => (
        <div key={q.id} className="card">
          <strong>{t('app.requestAsks', { name: q.from_name || short(q.from), amount: display(q.amount, q.denom) })}</strong>
          {q.note && <p className="muted">« {q.note} »</p>}
          <p className="muted">{when(q.created)}</p>
          {note?.id === q.id && <p className={note.key === 'app.requestPaid' ? 'notice' : 'notice notice--bad'}>{t(note.key)}</p>}
          <button type="button" className="primary" disabled={busy === q.id} onClick={async () => {
            setBusy(q.id);
            const out = await payRequest(signer, q);
            setBusy('');
            setNote({ id: q.id, key: out === 'paid' ? 'app.requestPaid' : out === 'transfer-failed' ? 'app.requestPayFailed' : 'app.requestNotRecorded' });
            setVersion((v) => v + 1);
          }}>{busy === q.id ? t('app.working') : t('app.requestPay', { amount: display(q.amount, q.denom) })}</button>
          <button type="button" className="ghost" disabled={busy === q.id} onClick={async () => {
            setBusy(q.id);
            await decline(signer, q.id);
            setBusy('');
            setVersion((v) => v + 1);
          }}>{t('app.requestDecline')}</button>
        </div>
      ))}
    </section>
  );
}

/** In the Request tab: ask an existing user, and follow what was asked. */
export function AskUser({ signer }: { signer: Signer }) {
  const [to, setTo] = useState('');
  const [denom, setDenom] = useState(defaultDenom);
  const [amount, setAmount] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [version, setVersion] = useState(0);
  const data = useRequests(signer, version);

  const parsed = amount.trim() === '' ? null : parseAmount(amount, denom);
  const ready = to.trim().length > 3 && parsed && BigInt(parsed.base) > 0n;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!ready || !parsed) return;
    setBusy(true); setResult(null);
    const r = await askFor(signer, { to, amount: parsed.base, denom, note, lang: getLocale(), amountText: display(parsed.base, denom) });
    setBusy(false);
    if (r.ok) {
      setResult({ ok: true, text: t('app.requestSent', { name: r.value.to_name || short(r.value.to) }) });
      setTo(''); setAmount(''); setNote('');
      setVersion((v) => v + 1);
      return;
    }
    setResult({ ok: false, text: t(r.status === 404 ? 'app.requestNobody' : r.status === 429 ? 'app.requestTooMany' : 'app.requestFailed') });
  }

  const outgoing = data?.outgoing ?? [];
  return (
    <div className="stack-lg">
      <form className="form" onSubmit={submit}>
        <strong>{t('app.requestTitle')}</strong>
        <p className="muted">{t('app.requestLede')}</p>
        <label>
          <span>{t('app.requestTo')}</span>
          <input value={to} onChange={(e) => setTo(e.target.value)} autoCapitalize="none" required placeholder="nom@exemple.com · ZA-K3M9-7QRT-B" />
        </label>
        <label>
          <span>{t('app.emailCurrency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {CURRENCIES.map((c) => <option key={c.denom} value={c.denom}>{c.code} · {c.name}</option>)}
          </select>
        </label>
        <label>
          <span>{t('app.emailAmount')}</span>
          <input className="y-num" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} required />
        </label>
        <label>
          <span>{t('app.requestNote')}</span>
          <input value={note} maxLength={140} onChange={(e) => setNote(e.target.value)} />
        </label>
        {result && <p className={result.ok ? 'notice' : 'notice notice--bad'}>{result.text}</p>}
        <button className="primary" disabled={!ready || busy}>{busy ? t('app.working') : t('app.requestSend')}</button>
      </form>
      {outgoing.length > 0 && (
        <section className="stack">
          <h3 className="section-title">{t('app.requestsOutgoing')}</h3>
          {outgoing.map((q) => (
            <div key={q.id} className="card">
              <strong>{display(q.amount, q.denom)}</strong>
              <p className="muted">{t('app.requestFrom', { name: q.to_name || short(q.to), date: when(q.created) })}</p>
              {q.note && <p className="muted">« {q.note} »</p>}
              <p className="muted">{t(`app.requestStatus.${q.status}`)}</p>
              {q.status === 'open' && (
                <button type="button" className="ghost" onClick={async () => { await cancel(signer, q.id); setVersion((v) => v + 1); }}>
                  {t('app.requestCancel')}
                </button>
              )}
            </div>
          ))}
        </section>
      )}
    </div>
  );
}
