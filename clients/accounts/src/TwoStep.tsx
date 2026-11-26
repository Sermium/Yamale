/**
 * The second step of signing in, chosen once for every Yamale app: an emailed
 * code, an authenticator app, or a PIN. It lives in the account services, not
 * in any app, so whatever is chosen here is what every app asks for.
 *
 * Changing it needs the password and the CURRENT second step answered first:
 * otherwise whoever learned the password could switch the account to a factor
 * of their own (tools/custodian, handleFactorChoose).
 */

import { useState, type FormEvent } from 'react';
import { t } from '@yamale/chain';

import {
  beginAuthenticator, chooseFactor, confirmAuthenticator, readFactors, startSignIn,
  type Factor, type FactorState, type Proof,
} from '../../shared/account/signin.ts';
import { PasswordField } from '../../shared/account/PasswordField.tsx';
import type { Services } from '../../wallet/src/account/enrol.ts';
import { QrCode } from '../../app/src/qr.tsx';

const FACTORS: Factor[] = ['email', 'totp', 'pin'];
const digits = (v: string, n: number) => v.replace(/\D/g, '').slice(0, n);

type Step =
  | { kind: 'locked' }
  | { kind: 'overview'; state: FactorState }
  | { kind: 'prove'; state: FactorState; target: Factor; nonce?: string; issued?: number }
  | { kind: 'pin'; proof: Proof }
  | { kind: 'totp'; proof: Proof; secret: string; uri: string }
  | { kind: 'done'; factor: Factor };

export function TwoStep({ email, services, lang }: { email: string; services: Services; lang: string }) {
  const [password, setPassword] = useState('');
  const [step, setStep] = useState<Step>({ kind: 'locked' });
  const [answer, setAnswer] = useState('');
  const [pin, setPin] = useState('');
  const [pinAgain, setPinAgain] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const failed = (reason: string) => { setBusy(false); setError(t(`accounts.fail.${reason}`)); };

  async function open(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError('');
    const r = await readFactors(services, email, password);
    if (!r.ok) { failed(r.reason); return; }
    setBusy(false);
    setStep({ kind: 'overview', state: r.value });
  }

  /** Begin a change: ask for the current factor, emailing a code when that is it. */
  async function change(state: FactorState, target: Factor) {
    setError(''); setAnswer('');
    if (state.current === 'email') {
      setBusy(true);
      const s = await startSignIn(services, email, password, lang);
      setBusy(false);
      if (!s.ok) { failed(s.reason); return; }
      if (s.factor === 'email') { setStep({ kind: 'prove', state, target, nonce: s.nonce, issued: s.issued }); return; }
    }
    setStep({ kind: 'prove', state, target });
  }

  async function prove(e: FormEvent) {
    e.preventDefault();
    if (step.kind !== 'prove') return;
    const proof: Proof = { factor: step.state.current as Factor, answer, nonce: step.nonce, issued: step.issued };
    setError('');
    if (step.target === 'pin') { setStep({ kind: 'pin', proof }); setPin(''); setPinAgain(''); return; }
    setBusy(true);
    if (step.target === 'totp') {
      const b = await beginAuthenticator(services, email, password);
      if (!b.ok) { failed(b.reason); return; }
      setBusy(false); setAnswer('');
      setStep({ kind: 'totp', proof, secret: b.secret, uri: b.uri });
      return;
    }
    const r = await chooseFactor(services, email, password, 'email', proof);
    if (!r.ok) { failed(r.reason); return; }
    setBusy(false);
    setStep({ kind: 'done', factor: 'email' });
  }

  async function savePin(e: FormEvent) {
    e.preventDefault();
    if (step.kind !== 'pin') return;
    if (pin.length < 6) { setError(t('account.pinShape')); return; }
    if (pin !== pinAgain) { setError(t('account.pinMismatch')); return; }
    setBusy(true); setError('');
    const r = await chooseFactor(services, email, password, 'pin', step.proof, pin);
    if (!r.ok) { failed(r.reason); return; }
    setBusy(false); setPin(''); setPinAgain('');
    setStep({ kind: 'done', factor: 'pin' });
  }

  async function confirmTotp(e: FormEvent) {
    e.preventDefault();
    if (step.kind !== 'totp') return;
    setBusy(true); setError('');
    const r = await confirmAuthenticator(services, email, password, answer, fetch, step.proof);
    if (!r.ok) { failed(r.reason); return; }
    setBusy(false);
    setStep({ kind: 'done', factor: 'totp' });
  }

  const errorNote = error && <p className="note note--warn" role="alert">{error}</p>;
  const cancel = <button type="button" className="btn btn--quiet" onClick={() => { setStep({ kind: 'locked' }); setError(''); }}>{t('accounts.back')}</button>;

  let body;
  switch (step.kind) {
    case 'locked':
      body = (
        <form className="stack" onSubmit={open}>
          <p className="hint">{t('account.twoStepUnlock')}</p>
          <label className="y-label" htmlFor="ts-pw">{t('accounts.password')}</label>
          <PasswordField id="ts-pw" className="field" autoComplete="current-password" required value={password}
                         onChange={(e) => setPassword(e.target.value)} />
          {errorNote}
          <button className="btn" disabled={busy || !password}>{busy ? t('accounts.working') : t('account.twoStepOpen')}</button>
        </form>
      );
      break;
    case 'overview':
      body = (
        <div className="stack">
          <ul className="methods">
            {FACTORS.map((f) => {
              const current = step.state.current === f;
              return (
                <li key={f} className={`method${current ? ' method--current' : ''}`}>
                  <div className="method__text">
                    <p className="method__name">{t(`account.method.${f}`)}</p>
                    <p className="hint">{t(`account.method.${f}.about`)}</p>
                  </div>
                  {current
                    ? <span className="method__badge">{t('account.methodCurrent')}</span>
                    : <button type="button" className="btn method__use" disabled={busy} onClick={() => void change(step.state, f)}>{t('account.methodUse')}</button>}
                </li>
              );
            })}
          </ul>
          {errorNote}
        </div>
      );
      break;
    case 'prove': {
      const cur = step.state.current;
      body = (
        <form className="stack" onSubmit={prove}>
          <p className="lede">{cur === 'email' ? t('accounts.codeEmailed', { email }) : cur === 'pin' ? t('account.pinAsk') : t('accounts.codeAuthenticator')}</p>
          <p className="hint">{t('account.proveWhy')}</p>
          {cur === 'pin'
            ? <PasswordField className="field field--code" inputMode="numeric" autoComplete="off" maxLength={12} autoFocus
                             value={answer} onChange={(e) => setAnswer(digits(e.target.value, 12))} aria-label={t('account.method.pin')} />
            : <input className="field field--code" inputMode="numeric" autoComplete="one-time-code" maxLength={8} autoFocus
                     value={answer} onChange={(e) => setAnswer(digits(e.target.value, 8))} aria-label={t('accounts.code')} />}
          {errorNote}
          <button className="btn btn--primary" disabled={busy || answer.length < 6}>{busy ? t('accounts.working') : t('accounts.confirm')}</button>
          {cancel}
        </form>
      );
      break;
    }
    case 'pin':
      body = (
        <form className="stack" onSubmit={savePin}>
          <p className="hint">{t('account.pinChoose')}</p>
          <label className="y-label" htmlFor="ts-pin">{t('account.newPin')}</label>
          <PasswordField id="ts-pin" className="field field--code" inputMode="numeric" autoComplete="off" maxLength={12} autoFocus
                         value={pin} onChange={(e) => setPin(digits(e.target.value, 12))} />
          <label className="y-label" htmlFor="ts-pin2">{t('account.newPinAgain')}</label>
          <PasswordField id="ts-pin2" className="field field--code" inputMode="numeric" autoComplete="off" maxLength={12}
                         value={pinAgain} onChange={(e) => setPinAgain(digits(e.target.value, 12))} />
          {errorNote}
          <button className="btn btn--primary" disabled={busy || pin.length < 6}>{busy ? t('accounts.working') : t('account.pinSave')}</button>
          {cancel}
        </form>
      );
      break;
    case 'totp':
      body = (
        <form className="stack" onSubmit={confirmTotp}>
          <p className="hint">{t('account.totpScan')}</p>
          <div className="qr"><QrCode text={step.uri} size={200} /></div>
          <p className="hint">{t('account.totpManual')}</p>
          <code className="secret">{step.secret}</code>
          <label className="y-label" htmlFor="ts-totp">{t('account.totpCode')}</label>
          <input id="ts-totp" className="field field--code" inputMode="numeric" autoComplete="one-time-code" maxLength={8}
                 value={answer} onChange={(e) => setAnswer(digits(e.target.value, 8))} />
          {errorNote}
          <button className="btn btn--primary" disabled={busy || answer.length < 6}>{busy ? t('accounts.working') : t('accounts.confirm')}</button>
          {cancel}
        </form>
      );
      break;
    case 'done':
      body = (
        <div className="stack">
          <p className="note" role="status">{t('account.methodSaved', { method: t(`account.method.${step.factor}`) })}</p>
          <button type="button" className="btn" onClick={() => setStep({ kind: 'locked' })}>{t('accounts.back')}</button>
        </div>
      );
      break;
  }

  return (
    <section className="twostep" aria-labelledby="twostep-title">
      <h2 id="twostep-title" className="twostep__title">{t('account.twoStepTitle')}</h2>
      <p className="hint">{t('account.twoStepLede')}</p>
      {body}
    </section>
  );
}
