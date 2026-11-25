/**
 * Making an account, signing in to it, and signing out.
 *
 * The words matter more than the layout here. A person must understand three
 * things before they rely on this account: the key is made on this phone and
 * locked with the password; signing in anywhere takes the email, the password
 * and a code; nobody can reset the password. The slow, operator-run recovery
 * is only for someone who has lost their code.
 */

import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { SERVICES } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { ACCOUNT_EXISTS, MIN_PASSWORD, WrongPassword } from './device.ts';
import { claimRecovered } from './recover.ts';
import { useSession } from './session.tsx';
import { beginAuthenticator, confirmAuthenticator, deviceLabel, signIn, startSignIn, type SignInFailure } from './signin.ts';

const lang = () => (typeof document !== 'undefined' ? document.documentElement.lang : '') || 'en';
const digits = (v: string) => v.replace(/[^0-9]/g, '');

/** `onBusy` lets a screen keep this form mounted while the account is set up behind it. */
export function CreateForm({ onDone, onBusy }: { onDone?: () => void; onBusy?: (busy: boolean) => void }) {
  const uid = useId();
  const session = useSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [again, setAgain] = useState('');
  const [factor, setFactor] = useState<'email' | 'totp'>('email');
  const [step, setStep] = useState<'form' | 'making' | 'sponsoring' | 'protecting' | 'failed' | 'unprotected' | 'authenticator'>('form');
  const [error, setError] = useState('');  // a catalogue key, never raw English

  const tooShort = password.length > 0 && password.length < MIN_PASSWORD;
  const differs = again.length > 0 && again !== password;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (tooShort || differs || !email.includes('@')) return;
    onBusy?.(true);
    try {
      setStep('making');
      await session.create(email, password);
    } catch (err) {
      const m = err instanceof Error ? err.message : '';
      setError(m === ACCOUNT_EXISTS ? 'traceability.accountExists' : m.includes('at least') ? 'traceability.passwordShort' : 'traceability.createFailed');
      setStep('failed');
      onBusy?.(false);
      return;
    }
    // The account now exists on this device. Sponsorship is best effort; the
    // sealed copies are not, because without them the account cannot be signed
    // into anywhere else, and signing out would lose it.
    setStep('sponsoring');
    await session.ensureSponsored().catch(() => undefined);
    await saveCopies();
  }

  async function saveCopies() {
    setStep('protecting');
    const out = await session.protect(password).catch(() => 'unavailable' as const);
    if (out !== 'enrolled') {
      setError(out === 'email-in-use' ? 'traceability.protectEmailInUse' : 'traceability.protectRequired');
      setStep('unprotected');
      return;
    }
    if (factor === 'totp') { setStep('authenticator'); return; }
    finish();
  }

  function finish() {
    setPassword(''); setAgain('');
    onBusy?.(false);
    onDone?.();
  }

  if (step === 'unprotected') {
    return (
      <div className="stack">
        <p className="note note--warn" role="alert">{t(error)}</p>
        <button type="button" className="btn btn--primary" onClick={saveCopies}>{t('traceability.protectButton')}</button>
      </div>
    );
  }
  if (step === 'authenticator') {
    return <AuthenticatorSetup email={email} password={password} onDone={finish} />;
  }
  if (step !== 'form' && step !== 'failed') {
    return <p className="loading" role="status">{t(`traceability.creating.${step}`)}</p>;
  }
  return (
    <form className="stack" onSubmit={submit}>
      <p className="hint">{t('traceability.createLede')}</p>
      <label className="y-label" htmlFor={uid + '-email'}>{t('traceability.email')}</label>
      <input id={uid + '-email'} className="bigfield" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      <label className="y-label" htmlFor={uid + '-pw'}>{t('traceability.password')}</label>
      <input id={uid + '-pw'} className="bigfield" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
      {tooShort && <p className="hint">{t('traceability.passwordShort', { n: MIN_PASSWORD })}</p>}
      <label className="y-label" htmlFor={uid + '-pw2'}>{t('traceability.passwordAgain')}</label>
      <input id={uid + '-pw2'} className="bigfield" type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} required />
      {differs && <p className="hint">{t('traceability.passwordDiffers')}</p>}
      <p className="note">{t('traceability.passwordWarning')}</p>
      <fieldset className="stack">
        <legend className="y-label">{t('traceability.factor')}</legend>
        <label className="tick"><input type="radio" name={uid + '-factor'} checked={factor === 'email'} onChange={() => setFactor('email')} /> {t('traceability.factorEmail')}</label>
        <label className="tick"><input type="radio" name={uid + '-factor'} checked={factor === 'totp'} onChange={() => setFactor('totp')} /> {t('traceability.factorTotp')}</label>
      </fieldset>
      {step === 'failed' && <p className="note note--warn" role="alert">{t(error, { n: MIN_PASSWORD })}</p>}
      <button className="btn btn--primary" disabled={tooShort || differs}>{t('traceability.createButton')}</button>
    </form>
  );
}

/** One secret, enrolled at both services, shown as a QR code and confirmed with one code. */
export function AuthenticatorSetup({ email, password, onDone }: { email: string; password: string; onDone: () => void }) {
  const uid = useId();
  const [begun, setBegun] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState('');
  const [state, setState] = useState<'starting' | 'idle' | 'busy' | 'wrong' | 'unavailable'>('starting');
  useEffect(() => {
    let live = true;
    beginAuthenticator(SERVICES, email, password).then((b) => {
      if (!live) return;
      if (b.ok) { setBegun(b); setState('idle'); } else setState('unavailable');
    });
    return () => { live = false; };
  }, [email, password]);
  if (state === 'starting') return <p className="loading" role="status">{t('traceability.loading')}</p>;
  if (!begun) return <p className="note note--warn" role="alert">{t('traceability.signInFailed.unavailable')}</p>;
  return (
    <form className="stack" onSubmit={async (e) => {
      e.preventDefault();
      setState('busy');
      const out = await confirmAuthenticator(SERVICES, email, password, code);
      if (out.ok) { onDone(); return; }
      setState(out.reason === 'wrong' ? 'wrong' : 'unavailable');
    }}>
      <p className="hint">{t('traceability.authScan')}</p>
      <QrCode text={begun.uri} size={200} />
      <p className="hint">{t('traceability.authSecret')} <code className="num">{begun.secret.match(/.{1,4}/g)!.join(' ')}</code></p>
      <label className="y-label" htmlFor={uid + '-code'}>{t('traceability.factorAnswer')}</label>
      <input id={uid + '-code'} className="bigfield num" inputMode="numeric" autoComplete="one-time-code" maxLength={8}
             value={code} onChange={(e) => setCode(digits(e.target.value))} required />
      {state === 'wrong' && <p className="note note--warn" role="alert">{t('traceability.authFailed')}</p>}
      {state === 'unavailable' && <p className="note note--warn" role="alert">{t('traceability.signInFailed.unavailable')}</p>}
      <button className="btn btn--primary" disabled={state === 'busy' || code.length < 6}>{t('traceability.authConfirm')}</button>
    </form>
  );
}

type SignInStep =
  | { kind: 'creds' }
  | { kind: 'code'; factor: 'totp' }
  | { kind: 'code'; factor: 'email'; nonce: string; issued: number };

/** Email and password, then the code: both sealed copies come back and open here. */
export function SignInForm({ onDone }: { onDone?: () => void }) {
  const uid = useId();
  const session = useSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [step, setStep] = useState<SignInStep>({ kind: 'creds' });
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<{ reason: SignInFailure | 'adopt'; minutes: number } | null>(null);
  const fail = (reason: SignInFailure | 'adopt', retryAfter?: number) =>
    setFailure({ reason, minutes: retryAfter ? Math.ceil(retryAfter / 60) : 1 });

  async function start() {
    setBusy(true); setFailure(null);
    const out = await startSignIn(SERVICES, email, password, lang());
    setBusy(false);
    if (!out.ok) { fail(out.reason, out.retryAfter); return; }
    setCode('');
    setStep(out.factor === 'totp' ? { kind: 'code', factor: 'totp' } : { kind: 'code', factor: 'email', nonce: out.nonce, issued: out.issued });
  }

  async function finish() {
    if (step.kind !== 'code') return;
    setBusy(true); setFailure(null);
    const out = await signIn(SERVICES, {
      email, password, factor: step.factor, answer: code,
      ...(step.factor === 'email' ? { nonce: step.nonce, issued: step.issued } : {}),
      device: deviceLabel(navigator.userAgent), lang: lang(),
    });
    if (!out.ok) { setBusy(false); fail(out.reason, out.retryAfter); return; }
    try {
      await session.adopt(out.privateKey, email, password);
    } catch {
      out.privateKey.fill(0);
      setBusy(false);
      fail('adopt');
      return;
    }
    setPassword(''); setCode('');
    onDone?.();
  }

  const failureNote = failure && (
    <p className="note note--warn" role="alert">{t(`traceability.signInFailed.${failure.reason}`, { min: failure.minutes })}</p>
  );
  if (step.kind === 'creds') {
    return (
      <form className="stack" onSubmit={(e) => { e.preventDefault(); void start(); }}>
        <label className="y-label" htmlFor={uid + '-email'}>{t('traceability.email')}</label>
        <input id={uid + '-email'} className="bigfield" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <label className="y-label" htmlFor={uid + '-pw'}>{t('traceability.password')}</label>
        <input id={uid + '-pw'} className="bigfield" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        {failureNote}
        <button className="btn btn--primary" disabled={busy}>{busy ? t('traceability.signingIn') : t('traceability.signInNext')}</button>
      </form>
    );
  }
  return (
    <form className="stack" onSubmit={(e) => { e.preventDefault(); void finish(); }}>
      <p className="hint">{step.factor === 'email' ? t('traceability.codeSentTo', { email }) : t('traceability.codeFromApp')}</p>
      <label className="y-label" htmlFor={uid + '-code'}>{t('traceability.factorAnswer')}</label>
      <input id={uid + '-code'} className="bigfield num" inputMode="numeric" autoComplete="one-time-code" maxLength={8} autoFocus
             value={code} onChange={(e) => setCode(digits(e.target.value))} required />
      {failureNote}
      <div className="actions">
        <button className="btn btn--primary" disabled={busy || code.length < 6}>{busy ? t('traceability.signingIn') : t('traceability.signInButton')}</button>
        {step.factor === 'email' && <button type="button" className="btn" disabled={busy} onClick={() => void start()}>{t('traceability.sendAnotherCode')}</button>}
        <button type="button" className="btn" disabled={busy} onClick={() => { setStep({ kind: 'creds' }); setFailure(null); }}>{t('traceability.cancel')}</button>
      </div>
    </form>
  );
}

/** The two ways in, signing in first: someone with an account must not make a second one by mistake. */
export function WaysIn({ onDone, onBusy }: { onDone?: () => void; onBusy?: (busy: boolean) => void }) {
  const [mode, setMode] = useState<'signin' | 'create'>('signin');
  const [busy, setBusyState] = useState(false);
  // While an account is being made, the screen around this must keep it
  // mounted: the account exists before its copies are saved and its
  // authenticator is set, and swapping the form out would cut both off.
  const setBusy = (b: boolean) => { setBusyState(b); onBusy?.(b); };
  return (
    <div className="stack">
      {!busy && (
        <div className="actions" role="tablist">
          <button type="button" role="tab" aria-selected={mode === 'signin'} className={`btn${mode === 'signin' ? ' btn--primary' : ''}`}
                  onClick={() => setMode('signin')}>{t('traceability.signInTitle')}</button>
          <button type="button" role="tab" aria-selected={mode === 'create'} className={`btn${mode === 'create' ? ' btn--primary' : ''}`}
                  onClick={() => setMode('create')}>{t('traceability.createTitle')}</button>
        </div>
      )}
      {mode === 'signin' ? <SignInForm onDone={onDone} /> : <CreateForm onDone={onDone} onBusy={setBusy} />}
      {mode === 'create' && !busy && <p className="hint">{t('traceability.newAccountWarning')}</p>}
    </div>
  );
}

export function UnlockForm({ onDone }: { onDone?: () => void }) {
  const uid = useId();
  const session = useSession();
  const [password, setPassword] = useState('');
  const [state, setState] = useState<'idle' | 'busy' | 'wrong'>('idle');
  async function submit(e: FormEvent) {
    e.preventDefault();
    setState('busy');
    try {
      await session.unlock(password);
      setPassword('');
      onDone?.();
    } catch (err) {
      setState(err instanceof WrongPassword ? 'wrong' : 'idle');
    }
  }
  return (
    <form className="stack" onSubmit={submit}>
      <label className="y-label" htmlFor={uid + '-unlock'}>{t('traceability.passwordToOpen')}</label>
      <input id={uid + '-unlock'} className="bigfield" type="password" autoComplete="current-password" value={password}
             onChange={(e) => setPassword(e.target.value)} required />
      {state === 'wrong' && <p className="note note--warn" role="alert">{t('traceability.passwordWrong')}</p>}
      <button className="btn btn--primary" disabled={state === 'busy'}>
        {state === 'busy' ? t('traceability.opening') : t('traceability.openButton')}
      </button>
    </form>
  );
}

/** Anything that signs sits inside this. */
export function UnlockGate({ children }: { children: ReactNode }) {
  const session = useSession();
  const [making, setMaking] = useState(false);
  if (!session.account || making) {
    return (
      <div className="stack">
        <p>{t('traceability.needAccount')}</p>
        <WaysIn onBusy={setMaking} />
      </div>
    );
  }
  if (!session.unlocked) return <UnlockForm />;
  return <>{children}</>;
}

function AccountCode({ address }: { address: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <details className="code">
      <summary>{t('traceability.showCode')}</summary>
      <p className="hint">{t('traceability.codeExplain')}</p>
      <QrCode text={address} size={200} />
      <button type="button" className="btn" onClick={async () => {
        try { await navigator.clipboard.writeText(address); setCopied(true); } catch { /* no clipboard on plain http */ }
      }}>{copied ? t('traceability.copied') : t('traceability.copyCode')}</button>
    </details>
  );
}

function ProtectButton() {
  const session = useSession();
  const [password, setPassword] = useState('');
  const [state, setState] = useState<'idle' | 'busy' | 'unavailable' | 'inuse' | 'wrong' | 'failed'>('idle');
  return (
    <form className="stack" onSubmit={async (e) => {
      e.preventDefault();
      setState('busy');
      try {
        const out = await session.protect(password);
        setState(out === 'enrolled' ? 'idle' : out === 'email-in-use' ? 'inuse' : 'unavailable');
      } catch (err) { setState(err instanceof WrongPassword ? 'wrong' : 'failed'); }
    }}>
      <label className="y-label" htmlFor="pp">{t('traceability.passwordToProtect')}</label>
      <input id="pp" className="bigfield" type="password" autoComplete="current-password" value={password}
             onChange={(e) => setPassword(e.target.value)} required />
      {state === 'unavailable' && <p className="note">{t('traceability.protectUnavailable')}</p>}
      {state === 'inuse' && <p className="note note--warn" role="alert">{t('traceability.protectEmailInUse')}</p>}
      {state === 'wrong' && <p className="note note--warn" role="alert">{t('traceability.passwordWrong')}</p>}
      {state === 'failed' && <p className="note note--warn">{t('traceability.protectFailed')}</p>}
      <button className="btn" disabled={state === 'busy'}>{t('traceability.protectButton')}</button>
    </form>
  );
}

function RecoverForm() {
  const session = useSession();
  const [f, setF] = useState({ recoveryId: '', email: '', password: '', factor: 'email' as 'email' | 'totp', answer: '' });
  const [state, setState] = useState<{ kind: 'idle' | 'busy' } | { kind: 'failed'; reason: string }>({ kind: 'idle' });
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });
  return (
    <form className="stack" onSubmit={async (e) => {
      e.preventDefault();
      setState({ kind: 'busy' });
      const out = await claimRecovered(f, SERVICES);
      if (!out.ok) { setState({ kind: 'failed', reason: out.reason }); return; }
      try {
        await session.adopt(out.privateKey, f.email, f.password);
      } catch {
        // A claimed key that was not adopted must not linger in memory.
        out.privateKey.fill(0);
        setState({ kind: 'failed', reason: 'adopt' });
        return;
      }
      setState({ kind: 'idle' });
    }}>
      <p className="hint">{t('traceability.recoverLede')}</p>
      <label className="y-label" htmlFor="rid">{t('traceability.recoveryId')}</label>
      <input id="rid" className="bigfield" value={f.recoveryId} onChange={set('recoveryId')} required />
      <label className="y-label" htmlFor="remail">{t('traceability.email')}</label>
      <input id="remail" className="bigfield" type="email" value={f.email} onChange={set('email')} required />
      <label className="y-label" htmlFor="rpw">{t('traceability.password')}</label>
      <input id="rpw" className="bigfield" type="password" autoComplete="current-password" value={f.password} onChange={set('password')} required />
      <label className="y-label" htmlFor="rfactor">{t('traceability.factor')}</label>
      <select id="rfactor" className="bigfield" value={f.factor} onChange={set('factor')}>
        <option value="email">{t('traceability.factorEmail')}</option>
        <option value="totp">{t('traceability.factorTotp')}</option>
      </select>
      <label className="y-label" htmlFor="ranswer">{t('traceability.factorAnswer')}</label>
      <input id="ranswer" className="bigfield" inputMode="numeric" value={f.answer} onChange={set('answer')} required />
      {state.kind === 'failed' && <p className="note note--warn" role="alert">{t(`traceability.recoverFailed.${state.reason}`)}</p>}
      <button className="btn btn--primary" disabled={state.kind === 'busy'}>{t('traceability.recoverButton')}</button>
      <p className="hint">{t('traceability.forgottenPassword')}</p>
    </form>
  );
}

export function AccountScreen() {
  const session = useSession();
  const [confirmForget, setConfirmForget] = useState(false);
  const [setup, setSetup] = useState(false);
  const [sponsorNote, setSponsorNote] = useState<'unavailable' | 'refused' | null>(null);
  const a = session.account;
  return (
    <>
      <AppBar title={t('traceability.accountTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen">
        {(!a || setup) && <Section title={t('traceability.accountTitle')}><WaysIn onBusy={setSetup} /></Section>}
        {!a && !setup && (
          <details className="block">
            <summary>{t('traceability.lostFactorTitle')}</summary>
            <RecoverForm />
          </details>
        )}
        {a && !setup && !session.unlocked && <Section title={a.email}><UnlockForm /></Section>}
        {a && !setup && session.unlocked && (<>
          <Section title={a.email}>
            <div className="actions">
              <span className={`y-chip ${a.protection === 'enrolled' ? 'y-chip--ok' : 'y-chip--warn'}`}>
                {a.protection === 'enrolled' ? t('traceability.protected') : t('traceability.onlyHere')}
              </span>
              <span className={`y-chip ${a.sponsored ? 'y-chip--ok' : 'y-chip--mute'}`}>
                {a.sponsored ? t('traceability.sponsored') : t('traceability.notSponsored')}
              </span>
            </div>
            {!a.sponsored && <button className="btn" onClick={async () => {
              const out = await session.ensureSponsored();
              setSponsorNote(out === 'sponsored' ? null : out);
            }}>{t('traceability.sponsorRetry')}</button>}
            {sponsorNote && !a.sponsored && <p className="note" role="status">{t(`traceability.sponsorOutcome.${sponsorNote}`)}</p>}
            <AccountCode address={a.address} />
          </Section>
          {a.protection !== 'enrolled' && <Section title={t('traceability.protectTitle')}><ProtectButton /></Section>}
          <Section title={t('traceability.forJewelers')}><Link className="btn" to="/apply">{t('traceability.applyLink')}</Link></Section>
          <Section title={t('traceability.thisDevice')}>
            <div className="actions">
              <button className="btn" onClick={session.lock}>{t('traceability.lock')}</button>
              <button className="btn btn--danger" onClick={() => setConfirmForget(true)}>{t('traceability.signOut')}</button>
            </div>
            {confirmForget && (
              <div className="banner banner--bad" role="alert">
                <p>{a.protection === 'enrolled' ? t('traceability.signOutProtected') : t('traceability.forgetUnprotected')}</p>
                <div className="actions">
                  <button className="btn btn--danger" onClick={session.forget}>{a.protection === 'enrolled' ? t('traceability.signOut') : t('traceability.forgetConfirm')}</button>
                  <button className="btn" onClick={() => setConfirmForget(false)}>{t('traceability.cancel')}</button>
                </div>
              </div>
            )}
          </Section>
        </>)}
      </main>
    </>
  );
}
