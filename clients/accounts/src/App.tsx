/**
 * accounts.yamaletech.com: sign in with Yamale once on this device, and send
 * the account to the Yamale app that asked for it.
 *
 * The account is kept here as every app keeps it, sealed under the password
 * (clients/shared/account/device.ts). Signing in takes the email, the password
 * and a code, through the custodian and recovery services. Handing the account
 * to an app takes one confirmation and no password: what travels is the sealed
 * account, encrypted to a key only the asking tab holds (handoff.ts), and the
 * app asks for the password itself before it can use the key.
 */

import { useState, type FormEvent } from 'react';
import { LanguagePicker, t, useLocale } from '@yamale/chain';

import { adoptKey, forgetAccount, readAccount, updateAccount, type StoredAccount } from '../../shared/account/device.ts';
import { APPS, appOrigin, sealForApp } from '../../shared/account/handoff.ts';
import { deviceLabel, signIn, startSignIn, type SignInFailure, type Started } from '../../shared/account/signin.ts';
import type { Services } from '../../wallet/src/account/enrol.ts';
import { TwoStep } from './TwoStep.tsx';
import { PasswordField } from '../../shared/account/PasswordField.tsx';

const SERVICES: Services = { custodianUrl: '/api/custodian', recoveryUrl: '/api/recovery' };
const BASE = import.meta.env.BASE_URL;
const lang = () => document.documentElement.lang || 'en';

interface Request { pk: string; state: string; returnUrl: string; origin: string }

/** What the asking app sent, if it is a Yamale app; 'invalid' when it is not. */
function readRequest(): Request | 'invalid' | null {
  const q = new URLSearchParams(location.search);
  const pk = q.get('pk');
  const state = q.get('state');
  const returnUrl = q.get('return');
  if (!pk && !state && !returnUrl) return null;
  const origin = returnUrl ? appOrigin(returnUrl) : null;
  if (!pk || !state || !returnUrl || !origin || !/^[0-9a-f]{32}$/.test(state)) return 'invalid';
  return { pk, state, returnUrl, origin };
}

function failure(reason: SignInFailure): string {
  return t(`accounts.fail.${reason}`);
}

function SignIn({ onDone }: { onDone: (a: StoredAccount, key: Uint8Array) => void }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [started, setStarted] = useState<Extract<Started, { ok: true }> | null>(null);
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function start(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    const out = await startSignIn(SERVICES, email, password, lang());
    setBusy(false);
    if (!out.ok) { setError(failure(out.reason)); return; }
    setStarted(out);
  }

  async function finish(e: FormEvent) {
    e.preventDefault();
    if (!started) return;
    setBusy(true);
    setError('');
    const out = await signIn(SERVICES, {
      email, password, factor: started.factor, answer: code.trim(),
      ...(started.factor === 'email' ? { nonce: started.nonce, issued: started.issued } : {}),
      device: deviceLabel(navigator.userAgent), lang: lang(),
    });
    if (!out.ok) {
      setBusy(false);
      setError(failure(out.reason));
      if (out.reason === 'expired') { setStarted(null); setCode(''); }
      return;
    }
    await adoptKey(out.privateKey, email, password);
    const account = updateAccount({ protection: 'enrolled' });
    setPassword('');
    setBusy(false);
    // Kept in memory for the hand-off that follows, so the app does not ask again.
    onDone(account, out.privateKey);
  }

  if (started) {
    return (
      <form className="stack" onSubmit={finish}>
        <p className="lede">{started.factor === 'pin' ? t('account.pinAsk') : t(started.factor === 'email' ? 'accounts.codeEmailed' : 'accounts.codeAuthenticator', { email })}</p>
        <label className="y-label" htmlFor="code">{t('accounts.code')}</label>
        <input id="code" className="field field--code" inputMode="numeric" autoComplete="one-time-code" maxLength={12} autoFocus
               value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 12))} />
        {error && <p className="note note--warn" role="alert">{error}</p>}
        <button className="btn btn--primary" disabled={busy || code.length < 6}>{busy ? t('accounts.working') : t('accounts.confirm')}</button>
        <button type="button" className="btn btn--quiet" onClick={() => { setStarted(null); setCode(''); setError(''); }}>{t('accounts.back')}</button>
      </form>
    );
  }
  return (
    <form className="stack" onSubmit={start}>
      <label className="y-label" htmlFor="email">{t('accounts.email')}</label>
      <input id="email" className="field" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      <label className="y-label" htmlFor="password">{t('accounts.password')}</label>
      <PasswordField id="password" className="field" autoComplete="current-password" required value={password}
             onChange={(e) => setPassword(e.target.value)} />
      {error && <p className="note note--warn" role="alert">{error}</p>}
      <button className="btn btn--primary" disabled={busy}>{busy ? t('accounts.working') : t('accounts.signIn')}</button>
    </form>
  );
}

function Handoff({ account, request, openKey, onNotMe }: {
  account: StoredAccount; request: Request; openKey: Uint8Array | null; onNotMe: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const app = APPS[request.origin];
  function go() {
    setBusy(true);
    const back = request.returnUrl.split('#')[0];
    const fragment = sealForApp(account, request.pk, request.origin, request.state, undefined, openKey);
    openKey?.fill(0);
    location.replace(`${back}#${fragment}`);
  }
  return (
    <div className="handoff">
      <p className="handoff__app">{t('accounts.continueTo', { app })}</p>
      <div className="handoff__who">
        <span className="handoff__avatar" aria-hidden="true">{account.email.charAt(0).toUpperCase()}</span>
        <div>
          <p className="handoff__email">{account.email}</p>
          <p className="handoff__origin">{request.origin.replace('https://', '')}</p>
        </div>
      </div>
      <p className="hint">{t('accounts.handoffExplain', { app })}</p>
      <button type="button" className="btn btn--primary" disabled={busy} onClick={go}>{t('accounts.continue')}</button>
      <button type="button" className="btn btn--quiet" onClick={onNotMe}>{t('accounts.notMe')}</button>
    </div>
  );
}

export function App() {
  useLocale();
  const [request] = useState(readRequest);
  const [account, setAccount] = useState<StoredAccount | null>(() => readAccount());
  // The key of an account signed in on this page just now, for its hand-off only.
  const [openKey, setOpenKey] = useState<Uint8Array | null>(null);

  function forget() {
    openKey?.fill(0);
    setOpenKey(null);
    forgetAccount();
    setAccount(null);
  }

  let body;
  if (request === 'invalid') {
    body = <p className="note note--warn" role="alert">{t('accounts.invalidRequest')}</p>;
  } else if (!account) {
    body = (<>
      <p className="lede">{request ? t('accounts.signInFor', { app: APPS[request.origin] }) : t('accounts.signInLede')}</p>
      <SignIn onDone={(a, k) => { setAccount(a); setOpenKey(k); }} />
      <p className="hint">{t('accounts.noAccountYet')}</p>
    </>);
  } else if (request) {
    body = <Handoff account={account} request={request} openKey={openKey} onNotMe={forget} />;
  } else {
    body = (<>
      <p className="lede">{t('accounts.signedInHere', { email: account.email })}</p>
      <TwoStep email={account.email} services={SERVICES} lang={lang()} />
      <button type="button" className="btn btn--quiet" onClick={forget}>{t('accounts.forgetHere')}</button>
    </>);
  }

  return (
    <div className="y-stage">
      <a className="skip" href="#main">{t('accounts.skip')}</a>
      <header className="y-stage__bar">
        <a className="stage__brand" href="https://yamaletech.com/"><img src={`${BASE}mark.png`} alt="" width={22} height={22} /><span>Yamale</span></a>
        <span className="y-stage__title">{t('accounts.title')}</span>
        <LanguagePicker className="stage__lang" />
      </header>
      <div className="y-phone">
        <div className="y-phone__notch" />
        <main className="y-phone__screen screen" id="main" tabIndex={-1}>
          <header className="mast">
            <img className="mast__mark" src={`${BASE}mark.png`} alt="" width={56} height={56} />
            <h1 className="mast__title">{t('accounts.heading')}</h1>
          </header>
          {body}
        </main>
      </div>
    </div>
  );
}
