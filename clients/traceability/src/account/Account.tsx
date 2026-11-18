/**
 * Making an account, opening it, protecting it, and getting it back.
 *
 * The words matter more than the layout here. A person must understand three
 * things before they rely on this account: the key is made and kept on this
 * phone; the password opens it and nobody can reset it; protection lets the
 * platform help after a lost phone, but only slowly and in the open.
 */

import { useId, useState, type FormEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { t } from '@yamale/chain';

import { QrCode } from '../../../app/src/qr.tsx';
import { SERVICES } from '../config.ts';
import { AppBar, Section } from '../ui.tsx';
import { ACCOUNT_EXISTS, MIN_PASSWORD, WrongPassword } from './device.ts';
import { claimRecovered } from './recover.ts';
import { useSession } from './session.tsx';

/** `onBusy` lets a screen keep this form mounted while the account is set up behind it. */
export function CreateForm({ onDone, onBusy }: { onDone?: () => void; onBusy?: (busy: boolean) => void }) {
  const uid = useId();
  const session = useSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [again, setAgain] = useState('');
  const [step, setStep] = useState<'form' | 'making' | 'sponsoring' | 'protecting' | 'failed'>('form');
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
    // The account now exists. Sponsorship and protection are best effort: the
    // account screen shows what is still missing and offers to retry each.
    setStep('sponsoring');
    await session.ensureSponsored().catch(() => undefined);
    setStep('protecting');
    await session.protect(password).catch(() => undefined);
    onBusy?.(false);
    onDone?.();
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
      {step === 'failed' && <p className="note note--warn" role="alert">{t(error, { n: MIN_PASSWORD })}</p>}
      <button className="btn btn--primary" disabled={tooShort || differs}>{t('traceability.createButton')}</button>
    </form>
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
  if (!session.account) {
    return (
      <div className="stack">
        <p>{t('traceability.needAccount')}</p>
        <CreateForm />
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
        {(!a || setup) && <Section title={t('traceability.createTitle')}><CreateForm onBusy={setSetup} /></Section>}
        {!a && <Section title={t('traceability.recoverTitle')}><RecoverForm /></Section>}
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
              <button className="btn btn--danger" onClick={() => setConfirmForget(true)}>{t('traceability.forget')}</button>
            </div>
            {confirmForget && (
              <div className="banner banner--bad" role="alert">
                <p>{a.protection === 'enrolled' ? t('traceability.forgetProtected') : t('traceability.forgetUnprotected')}</p>
                <div className="actions">
                  <button className="btn btn--danger" onClick={session.forget}>{t('traceability.forgetConfirm')}</button>
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
