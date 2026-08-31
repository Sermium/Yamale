import { useEffect, useState } from 'react';
import { t, LanguagePicker, useLocale, formatUserId, getLocale } from '@yamale/chain';

import { signIn, signUp, signOut, lastEmail, revealPhrase, eraseEverything, type Signer } from './account.ts';
import { CURRENCIES, currencyOf, display, parseAmount, toBaseUnits, rawAmount, groupDigits, defaultDenom, setDefaultDenom } from './money.ts';
import { topUpEmpty, needsTopUp as topUpEmpty_needed, balances } from './topup.ts';
import { encodeSigned, shortCodeSigned, readSigned, describe,
         type PaymentRequest, type Recurrence } from './request.ts';
import { QrCode, QrPanel } from './qr.tsx';
import { Scanner } from './scan.tsx';
// Paying somebody, and the identifier controls it uses, both live in their own
// files now. This module was 2,556 lines holding twenty-three components; the
// screen the whole product exists for should not have been one of them.
import { Pay } from './Pay.tsx';
import { Desk } from './Desk.tsx';
import { CopyRow } from './copy.tsx';
import { PayableNote } from './Identifier.tsx';
import * as book from './book.ts';
import * as biometric from './biometric.ts';
import { DIRECTORY, matches } from './directory.ts';
import * as escrow from './escrow.ts';
import * as chain from './chain.ts';
import * as twofa from './twofactor.ts';
import * as swaps from './swap.ts';
import * as agents from './agents.ts';
import * as ramp from './ramp.ts';
import * as ledger from './ledger.ts';

/**
 * The whole app runs inside a phone frame on a desktop screen, because this is
 * a demonstration of what a citizen holds rather than of what an operator runs.
 *
 * Vocabulary rule for every screen below: no address, no wallet, no key, no
 * seed phrase, no gas, no denom, no hash, no block. A person opening a banking
 * app has never heard those words and does not need to start now. Where the
 * chain requires one, the abstraction layer supplies it — see account.ts and
 * money.ts, which are the only files permitted to know any of this exists.
 */
export function App() {
  const [signer, setSigner] = useState<Signer | null>(null);
  const [topUpDone, setTopUpDone] = useState(0);
  const [topping, setTopping] = useState(false);
  // Subscribing here re-renders every screen below when the language
  // changes, which is what the reload used to accomplish.
  useLocale();
  const [screen, setScreen] = useState<'home' | 'pay' | 'change' | 'contacts' | 'settings'>('home');

  // Top up on sign-in so a demonstration never dies on an empty balance.
  // Only currencies holding nothing are touched, so figures somebody arranged
  // for a demo are not reset under them mid-sentence.
  useEffect(() => {
    if (!signer) return;
    let live = true;
    (async () => {
      const address = await signer.internalAddress();
      // Only say it when it is true. A banner announcing work that is not
      // happening teaches people to ignore banners.
      const needed = await topUpEmpty_needed(address);
      if (live && needed) setTopping(true);
      // Money first, then the identifier: registering costs a fee, so an
      // account with nothing in it cannot pay to be named.
      await topUpEmpty(address);
      await chain.ensureUserId(signer);
      if (live) { setTopping(false); setTopUpDone((n) => n + 1); }
    })();
    return () => { live = false; };
  }, [signer]);

  return (
    <div className="stage">
      <header className="stage__bar">
        <a className="stage__back" href="/">
          <svg className="brand__mark" viewBox="0 0 64 64" aria-hidden="true" width="22" height="22"><rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F"/><path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" strokeWidth="7.2"/><path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" strokeWidth="7.2"/></svg>
          <span>Yamale</span>
        </a>
        <span className="stage__title">Yamale Pay</span>
        <a className="stage__link" href="/">{t("app.backToSite")}</a>
      </header>

      {/* A phone and, on a screen wide enough to have room for it, the same
          payment as the ledger records it. See Desk.tsx for why the desktop case
          got a second column rather than a stretched app. */}
      <div className="stage__two">
    <div className="phone">
      <div className="phone__notch" />
      <div className="phone__screen">
        {signer ? (
          <>
            <Header profile={signer.profile} onSignOut={() => { signOut(); setSigner(null); }} />
            <main className="screen">
              {screen === 'home' && <Home signer={signer} refresh={topUpDone} topping={topping} />}
              {screen === 'pay' && <Transfer signer={signer} />}
              {screen === 'change' && <Exchange signer={signer} />}
              {screen === 'contacts' && <Contacts />}
              {screen === 'settings' && <Settings signer={signer} onClosed={() => { signOut(); setSigner(null); }} />}
            </main>
            <TabBar current={screen} onChange={setScreen} />
          </>
        ) : (
          <Welcome onSignedIn={setSigner} />
        )}
      </div>
    </div>
      <Desk />
      </div>
    </div>
  );
}

function Header({ profile, onSignOut }: { profile: { name: string; userId?: string }; onSignOut: () => void }) {
  return (
    <header className="appbar">
      {/* The same mark the sign-in screen opens with. Inside the app it does
          the work a bank's logo does on a statement: says whose ledger this is,
          on a screen that is otherwise only numbers. */}
      <img className="appbar__mark" src="mark.svg" alt="Yamale" width={30} height={30} />
      <div>
        <div className="appbar__hello">{t('app.hello')}</div>
        <div className="appbar__name">{profile.name}</div>
      </div>
      <div className="appbar__right">
        {/* The word is gone but not the meaning: the label stays for screen
            readers and as the hover tooltip, because an unlabelled red glyph is
            recognisable to people who already know it and a guess to everyone
            else. Red because signing out is the one control here that throws
            work away. */}
        <button className="signout" onClick={onSignOut}
                title={t('app.signOut')} aria-label={t('app.signOut')}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor"
               strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
            <path d="m16 17 5-5-5-5" />
            <path d="M21 12H9" />
          </svg>
        </button>
      </div>
    </header>
  );
}

/**
 * Sign in and sign up, and nothing else.
 *
 * There is no "create wallet", no phrase to write down, and no warning that
 * losing something means losing everything. A key is generated during sign-up
 * and the user is never told — which is the difference between a product and a
 * cryptography exercise.
 */
function Welcome({ onSignedIn }: { onSignedIn: (s: Signer) => void }) {
  const [mode, setMode] = useState<'in' | 'up'>(lastEmail() ? 'in' : 'up');
  const [name, setName] = useState('');
  const [email, setEmail] = useState(lastEmail() ?? '');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  // Offered only when this device can actually do it and this account has
  // enrolled — a button that opens a prompt and fails is worse than no button.
  const [canBio, setCanBio] = useState(false);
  useEffect(() => {
    biometric.available().then((ok) => setCanBio(ok && biometric.enrolled()));
  }, []);

  async function unlockWithBiometric() {
    setError('');
    const creds = await biometric.unlock();
    if (!creds) { setError(t('app.biometricFailed')); return; }
    setBusy(true);
    try {
      const s = await signIn(creds.email, creds.password);
      const enrolment = twofa.enrolment();
      if (enrolment?.method === 'totp' && enrolment.secret) { setPending(s); return; }
      onSignedIn(s);
    } catch {
      setError(t('app.wrongPassword'));
    } finally {
      setBusy(false);
    }
  }

  const [error, setError] = useState('');

  const [pending, setPending] = useState<Signer | null>(null);
  const [otp, setOtp] = useState('');
  const [checking, setChecking] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const s = mode === 'up'
        ? await signUp({ name, email }, password)
        : await signIn(email, password);

      // The password unlocked the account; the second factor decides whether
      // this session gets it. Holding the signer here rather than passing it up
      // means a half-finished sign-in cannot spend anything.
      const enrolled = twofa.enrolment();
      if (enrolled?.method === 'totp' && enrolled.secret) { setPending(s); return; }
      onSignedIn(s);
    } catch (err) {
      setError(String(err).includes('no-account') ? t('app.noAccount') : t('app.wrongPassword'));
    } finally {
      setBusy(false);
    }
  }

  async function confirmOtp(e: React.FormEvent) {
    e.preventDefault();
    await check(otp);
  }

  /**
   * Verify a code, wherever it came from.
   *
   * Split out so that typing the sixth digit and pressing the button run the
   * same path. A code is fixed at six digits, so the button was asking people
   * to confirm something they had already fully expressed — and on a phone it
   * meant reaching past the keypad that was still covering it.
   */
  async function check(code: string) {
    const enrolled = twofa.enrolment();
    if (!pending || !enrolled?.secret || checking) return;
    setChecking(true);
    setError('');
    if (await twofa.verify(enrolled.secret, code)) {
      onSignedIn(pending);
    } else {
      setError(t('app.badCode'));
      setOtp('');
    }
    setChecking(false);
  }

  if (pending) {
    return (
      <div className="welcome">
        <img className="welcome__mark" src="mark.svg" alt="" width={56} height={56} />
        <h1>{t('app.secondStep')}</h1>
        <p className="muted">{t('app.enterCode')}</p>
        <form onSubmit={confirmOtp} className="form">
          <label>
            <span>{t('app.sixDigits')}</span>
            <input inputMode="numeric" autoComplete="one-time-code" maxLength={6}
                   value={otp} disabled={checking} autoFocus
                   onChange={(e) => {
                     // Digits only: a pasted code often arrives with a space in
                     // the middle, and that would never reach six.
                     const digits = e.target.value.replace(/\D/g, '').slice(0, 6);
                     setOtp(digits);
                     if (digits.length === 6) void check(digits);
                   }} />
          </label>
          {error && <p className="notice notice--bad">{error}</p>}
          <button className="primary" disabled={checking || otp.length < 6}>
            {checking ? t('app.checking') : t('app.confirm')}
          </button>
          <button type="button" className="linkish" onClick={() => { setPending(null); setOtp(''); setError(''); }}>
            {t('app.cancel')}
          </button>
        </form>
      </div>
    );
  }

  return (
    <div className="welcome">
      <img className="welcome__mark" src="mark.svg" alt="" width={56} height={56} />
      <h1>{t('app.title')}</h1>
      <p className="muted">{t('app.tagline')}</p>

      <form onSubmit={submit} className="form">
        {mode === 'up' && (
          <label>
            <span>{t('app.name')}</span>
            <input value={name} onChange={(e) => setName(e.target.value)} required autoComplete="name" />
          </label>
        )}
        <label>
          <span>{t('app.email')}</span>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoComplete="email" />
        </label>
        <label>
          <span>{t('app.password')}</span>
          <input
            type="password" value={password} onChange={(e) => setPassword(e.target.value)}
            required minLength={8}
            autoComplete={mode === 'up' ? 'new-password' : 'current-password'}
          />
        </label>

        {error && <p className="notice notice--bad">{error}</p>}

        {canBio && (
          <button type="button" className="ghost" onClick={unlockWithBiometric} disabled={busy}>
            <span className="btn-ico" aria-hidden="true">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor"
                   strokeWidth="1.8" strokeLinecap="round">
                <path d="M12 11v3m0-9a7 7 0 0 0-7 7v2m14-2a7 7 0 0 0-3-5.7M8.5 20a12 12 0 0 1-1.2-5.3M15.5 20a16 16 0 0 0 .5-4" />
              </svg>
            </span>
            {t('app.unlockBiometric')}
          </button>
        )}
        <button className="primary" disabled={busy}>
          {busy ? t('app.working') : mode === 'up' ? t('app.createAccount') : t('app.signIn')}
        </button>
      </form>

      <button className="linkish" onClick={() => setMode(mode === 'up' ? 'in' : 'up')}>
        {mode === 'up' ? t('app.haveAccount') : t('app.noAccountYet')}
      </button>

      <div className="lang-row"><LanguagePicker className="lang" /></div>
    </div>
  );
}
/**
 * A month's statement, as a file the account holder keeps.
 *
 * Plain text rather than PDF: it is legible on any device, it can be pasted
 * into an email, and it does not need a library that would double the size of
 * this app. Every figure in it is reproducible from the chain, so the document
 * is a convenience and never the record.
 */
function downloadStatement(st: ledger.MonthlyStatement, holder: string) {
  // Dates take the app's locale, not the browser's. `undefined` here meant a
  // statement whose headings were in English and whose dates were in French,
  // on the same page, for anyone whose operating system disagreed with their
  // chosen language — which on this product is most of the intended audience.
  const c = currencyOf(st.denom);
  const money = (v: bigint) => display(v.toString(), st.denom);
  const when = new Intl.DateTimeFormat(getLocale(), { month: 'long', year: 'numeric' }).format(st.from);

  const lines = [
    `YAMALE — ${t('app.statement')} — ${when}`,
    `${holder} · ${c?.name}`,
    '',
    `${t('app.openingBalance')}: ${money(st.opening)}`,
    `${t('app.paidIn')}: +${money(st.paidIn)}`,
    `${t('app.paidOut')}: -${money(st.paidOut)}`,
    `${t('app.closingBalance')}: ${money(st.closing)}`,
    '',
    t('app.movements'),
    ...st.movements.map((m) => {
      const d = new Intl.DateTimeFormat(getLocale(), { day: '2-digit', month: '2-digit' }).format(new Date(m.at));
      const sign = m.amount > 0n ? '+' : '-';
      const abs = m.amount < 0n ? -m.amount : m.amount;
      // The reference and the purpose code on the line, because a statement
      // that omits them is a statement nobody can reconcile against a ledger.
      const ref = [m.reference.purpose, m.reference.remittance].filter(Boolean).join(' ');
      return `  ${d}  ${sign}${money(abs)}  ${book.displayName(m.counterparty)}${ref ? `  — ${ref}` : ''}`;
    }),
    '',
    `${t('app.statementNote')}`,
  ];

  const blob = new Blob([lines.join(String.fromCharCode(10))], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `yamale-${c?.code}-${st.from.getFullYear()}-${String(st.from.getMonth() + 1).padStart(2, '0')}.txt`;
  a.click();
  URL.revokeObjectURL(url);
}

function Home({ signer, refresh, topping }: { signer: Signer; refresh: number; topping: boolean }) {
  const [balances, setBalances] = useState<{ denom: string; amount: string }[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [history, setHistory] = useState<ledger.Movement[]>([]);
  const [names, setNames] = useState<Record<string, string>>({});
  const [month, setMonth] = useState(0);

  // History is fetched once and filtered per currency, rather than re-queried
  // on every tap. Two transaction searches is the expensive part; slicing the
  // result is free.
  useEffect(() => {
    let live = true;
    (async () => {
      const address = await signer.internalAddress();
      const all = await ledger.movements(address);
      if (!live) return;
      setHistory(all);

      // Resolved once per distinct counterparty rather than per row: a month of
      // payments to the same person is one lookup, not thirty.
      const parties = [...new Set(all.map((m) => m.counterparty).filter(Boolean))];
      const resolved = await Promise.all(
        parties.map(async (a) => [a, await book.nameForAddress(a, t('app.unknownParty'))] as const),
      );
      if (live) setNames(Object.fromEntries(resolved));
    })();
    return () => { live = false; };
  }, [signer, refresh]);

  useEffect(() => {
    let live = true;
    (async () => {
      const address = await signer.internalAddress();
      const res = await fetch(`/api/rest/cosmos/bank/v1beta1/balances/${address}`);
      const json = await res.json().catch(() => ({ balances: [] }));
      if (live) setBalances(json.balances ?? []);
    })();
    return () => { live = false; };
  }, [signer, refresh]);

  const shown = (balances ?? []).filter((b) => CURRENCIES.some((c) => c.denom === b.denom));

  // A bank account view: one account per currency, the figure dominant, the
  // name subordinate. Somebody opening this app wants to know what they have
  // before anything else, so the balance is the largest thing on the screen and
  // everything else defers to it.
  const primary = shown.find((b) => b.denom === defaultDenom()) ?? shown[0];
  const others = shown.filter((b) => b !== primary);

  if (open) {
    const balance = BigInt(shown.find((b) => b.denom === open)?.amount ?? '0');
    const st = ledger.statement(open, balance, history, month);
    const months = ledger.monthsWithActivity(history.filter((m) => m.denom === open));
    const c = currencyOf(open);

    return (
      <>
        <button className="linkish" onClick={() => { setOpen(null); setMonth(0); }}>
          ← {t('app.tabHome')}
        </button>

        <section className="acct-hero">
          <div className="acct-hero__label">{c?.name}</div>
          <div className="acct-hero__amount">{display(balance.toString(), open)}</div>
        </section>

        {/* A month at a time, because a statement is how people are used to
            being shown money and because "what changed since the 1st" is the
            question they are actually asking. */}
        <div className="months">
          {months.map((m) => (
            <button key={m} type="button"
                    className={m === month ? 'months__on' : undefined}
                    onClick={() => setMonth(m)}>
              {new Intl.DateTimeFormat(getLocale(), { month: 'short', year: '2-digit' })
                .format(new Date(new Date().getFullYear(), new Date().getMonth() - m, 1))}
            </button>
          ))}
        </div>

        <div className="ledger">
          <div><span>{t('app.openingBalance')}</span><span>{display(st.opening.toString(), open)}</span></div>
          <div><span>{t('app.paidIn')}</span><span>+{display(st.paidIn.toString(), open)}</span></div>
          <div><span>{t('app.paidOut')}</span><span>−{display(st.paidOut.toString(), open)}</span></div>
          <div className="ledger__total"><span>{t('app.closingBalance')}</span><span>{display(st.closing.toString(), open)}</span></div>
        </div>

        <button type="button" className="ghost" onClick={() => downloadStatement(st, signer.profile.name)}>
          {t('app.saveStatement')}
        </button>

        <h3 className="screen__subtitle">{t('app.movements')}</h3>
        {st.movements.length === 0 && <p className="muted">{t('app.noMovements')}</p>}
        <ul className="accts">
          {st.movements.map((m) => (
            <li key={m.hash + m.amount.toString()} className="acct-row">
              <span className={m.amount > 0n ? 'acct-row__mark in' : 'acct-row__mark out'}>
                {m.amount > 0n ? '↓' : '↑'}
              </span>
              <span className="acct-row__name">
                {names[m.counterparty] ?? t('app.unknownParty')}
                {/* The reference, on the line it belongs on. This is what
                    somebody matches against an invoice, and until now it went
                    out with every payment and never came back. */}
                {m.reference.remittance && (
                  <span className="movement__ref y-mono" title={m.reference.remittance}>
                    {m.reference.remittance}
                  </span>
                )}
                <span className="muted movement__date">
                  {m.reference.purpose && <span className="movement__purp">{m.reference.purpose}</span>}
                  {new Intl.DateTimeFormat(getLocale(), { day: 'numeric', month: 'short' }).format(new Date(m.at))}
                </span>
              </span>
              <span className="acct-row__amount">
                {m.amount > 0n ? '+' : '−'}{display((m.amount < 0n ? -m.amount : m.amount).toString(), open)}
              </span>
            </li>
          ))}
        </ul>
      </>
    );
  }

  return (
    <>
      {topping && (
        <div className="topping">
          <span className="topping__dots"><i /><i /><i /></span>
          <span>{t('app.toppingUp')}</span>
        </div>
      )}

      {/* Says nothing at all unless the chain has refused this account an
          identifier, which on this network it does — see Identifier.tsx. An
          account nobody can pay is the one thing a payments app must not be
          quiet about. */}
      <PayableNote signer={signer} />

      {balances === null && <p className="muted">{t('app.loading')}</p>}

      {balances !== null && shown.length === 0 && (
        <div className="empty"><p>{t('app.emptyBalance')}</p></div>
      )}

      {primary && (
        <section className="acct-hero acct-hero--tap" onClick={() => setOpen(primary.denom)}>
          <div className="acct-hero__label">{t('app.mainAccount')}</div>
          <div className="acct-hero__amount">{display(primary.amount, primary.denom)}</div>
          <div className="acct-hero__name">{currencyOf(primary.denom)?.name}</div>
        </section>
      )}

      {others.length > 0 && (
        <>
          <h3 className="screen__subtitle">{t('app.otherAccounts')}</h3>
          <ul className="accts">
            {others.map((b) => (
              <li key={b.denom} className="acct-row acct-row--tap" onClick={() => setOpen(b.denom)}>
                {/* The code as a mark, the way a bank uses a card or an icon —
                    it is what someone scans the list for. */}
                <span className="acct-row__mark">{currencyOf(b.denom)?.code}</span>
                <span className="acct-row__name">{currencyOf(b.denom)?.name}</span>
                <span className="acct-row__amount">{display(b.amount, b.denom)}</span>
              </li>
            ))}
          </ul>
        </>
      )}

      {/* No fee note anywhere on this screen. Fees are paid by a grant, so the
          number a person sees leaving their account is the number they sent. */}
    </>
  );
}

function Request({ signer }: { signer: Signer }) {
  const [userId, setUserId] = useState('');
  const [amount, setAmount] = useState('');
  const [currency, setCurrency] = useState(
    currencyOf(defaultDenom())?.code ?? CURRENCIES[0].code,
  );
  const [recurrence, setRecurrence] = useState<Recurrence>('once');
  const [reference, setReference] = useState('');

  useEffect(() => {
    (async () => {
      const address = await signer.internalAddress();
      const id = await book.myUserId(address);
      // Falling back to the address would leak the one thing this app exists to
      // hide, so an account with no id yet simply has nothing to show.
      if (id) setUserId(id);
    })();
  }, [signer]);

  const req: PaymentRequest = {
    to: userId, amount: amount || undefined, currency, recurrence,
    reference: reference || undefined, payeeName: signer.profile.name,
  };

  // Sealing is asynchronous, so the codes are state rather than derived values.
  // A fresh nonce each time means the square changes as the form changes, which
  // is also what stops a payee being trackable by a byte-identical code.
  const [qr, setQr] = useState('');
  const [spoken, setSpoken] = useState('');
  useEffect(() => {
    if (!userId) return;
    let live = true;
    (async () => {
      const sign = (payload: string) => signer.sign(payload);
      const [a, b] = await Promise.all([encodeSigned(req, sign), shortCodeSigned(req, sign)]);
      if (live) { setQr(a); setSpoken(b); }
    })();
    return () => { live = false; };
  }, [signer, userId, amount, currency, recurrence, reference]);

  return (
    <>
      <h2 className="screen__title">{t('app.requestMoney')}</h2>
      {!userId && <p className="muted">{t('app.noUserIdYet')}</p>}
      {userId && (
        <>
          {qr && (
            <QrPanel text={qr} size={190} filename="yamale-request.png"
                     labels={{ copy: t('app.copyImage'), copied: t('app.copied'), share: t('app.shareImage'), save: t('app.saveImage') }} />
          )}
          <CopyRow value={spoken} />
          <p className="muted center">{t('app.codeHint')}</p>
        </>
      )}

      <div className="form">
        <label>
          <span>{t('app.amount')}</span>
          <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)}
                 placeholder={t('app.anyAmount')} />
        </label>
        <label>
          <span>{t('app.currency')}</span>
          <select value={currency} onChange={(e) => setCurrency(e.target.value)}>
            {CURRENCIES.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}
          </select>
        </label>
        <label>
          <span>{t('app.repeats')}</span>
          <select value={recurrence} onChange={(e) => setRecurrence(e.target.value as Recurrence)}>
            <option value="once">{t('app.once')}</option>
            <option value="week">{t('app.weekly')}</option>
            <option value="month">{t('app.monthly')}</option>
            <option value="year">{t('app.yearly')}</option>
          </select>
        </label>
        <label>
          <span>{t('app.reference')}</span>
          <input value={reference} onChange={(e) => setReference(e.target.value)}
                 placeholder={t('app.referenceHint')} />
        </label>
      </div>
    </>
  );
}

/**
 * People, in two halves: what the chain knows publicly and what this device
 * remembers privately.
 *
 * The labels never leave the phone. A contact list is a social graph, and a
 * social graph on a public ledger says which villages trade with which and who
 * a journalist pays.
 */
function Contacts() {
  const [list, setList] = useState(book.contacts());
  const [id, setId] = useState('');
  const [label, setLabel] = useState('');
  const [status, setStatus] = useState('');
  const [scanning, setScanning] = useState(false);
  const [recent, setRecent] = useState(book.recents());
  const [query, setQuery] = useState('');
  const [book_, setBook] = useState<'public' | 'private'>('public');
  const [adding, setAdding] = useState(false);
  /** A chain result for a query that matched nothing already known. */
  const [found, setFound] = useState<{ id: string } | null>(null);
  const [looking, setLooking] = useState(false);

  const norm = (v: string) => v.toLowerCase().replace(/[\s-]/g, '');
  const q = norm(query);

  const publicHits = DIRECTORY.filter((e) => matches(e, query));
  const mine = list.filter((c) => !q || norm(c.label).includes(q) || norm(c.id).includes(q));
  const often = recent
    .filter((r) => !q || norm(book.displayName(r.id) || r.id).includes(q) || norm(r.id).includes(q))
    .filter((r) => !list.some((c) => c.id === r.id));

  /**
   * Nothing known matched, so ask the chain.
   *
   * This is the only way to reach somebody who is neither listed nor already a
   * contact, and it needs the ID in full — which is the point. A search that
   * completed partial IDs would let anyone sweep the chain for accounts, which
   * is exactly what leaving out the list-all rpc prevents.
   */
  useEffect(() => {
    const candidate = query.trim().toUpperCase();
    setFound(null);
    if (candidate.length < 6 || publicHits.length || mine.length || often.length) return;

    let live = true;
    setLooking(true);
    (async () => {
      const address = await book.resolve(candidate);
      if (!live) return;
      setFound(address ? { id: candidate } : null);
      setLooking(false);
    })();
    return () => { live = false; };
  }, [query]);

  function onScan(raw: string) {
    setScanning(false);
    const text = raw.trim();
    if (text.toLowerCase().startsWith('yamale:pay')) {
      const to = new URLSearchParams(text.slice('yamale:pay'.length + 1)).get('to');
      if (to) { setId(to); setAdding(true); return; }
      setStatus(t('app.scanNotAnId'));
      return;
    }
    setId(text.toUpperCase());
    setAdding(true);
  }

  async function add(e: React.FormEvent) {
    e.preventDefault();
    setStatus('');
    const resolved = await book.resolve(id);
    if (!resolved) { setStatus(t('app.unknownId')); return; }
    book.save({ id: id.toUpperCase(), label });
    setList(book.contacts());
    setRecent(book.recents());
    setId(''); setLabel(''); setAdding(false); setQuery('');
  }

  function saveFound(foundId: string) {
    setId(foundId);
    setLabel('');
    setAdding(true);
  }

  return (
    <>
      <h2 className="screen__title">{t('app.contacts')}</h2>

      {scanning && <Scanner onRead={onScan} onClose={() => setScanning(false)} />}

      {/* One box over both books. Somebody looking for a person does not know
          or care which of the two they are in. */}
      <div className="search">
        <span className="search__ico" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
               strokeWidth="2" strokeLinecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>
        </span>
        <input value={query} onChange={(e) => setQuery(e.target.value)}
               placeholder={t('app.searchPeople')} aria-label={t('app.searchPeople')} />
        {query && (
          <button type="button" className="search__clear" onClick={() => setQuery('')}
                  aria-label={t('app.close')}>&times;</button>
        )}
      </div>

      {/* Two books, because they are two different things: one the chain
          publishes and everyone sees the same way, one that exists only in
          this browser and is nobody else's business. A search crosses both —
          the folders are for browsing, not for filtering. */}
      {!query && (
        <div className="folders" role="tablist">
          <button type="button" role="tab" aria-selected={book_ === 'public'}
                  className={book_ === 'public' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                  onClick={() => setBook('public')}>{t('app.publicBook')}</button>
          <button type="button" role="tab" aria-selected={book_ === 'private'}
                  className={book_ === 'private' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                  onClick={() => setBook('private')}>{t('app.myBook')}</button>
        </div>
      )}

      <div className="row-actions">
        <button type="button" className="ghost" onClick={() => { setStatus(''); setScanning(true); }}>
          {t('app.scanTheirCode')}
        </button>
        <button type="button" className="ghost" onClick={() => setAdding((v) => !v)}>
          {t('app.addPerson')}
        </button>
      </div>

      {adding && (
        <form className="form" onSubmit={add}>
          <label>
            <span>{t('app.theirId')}</span>
            <input value={id} onChange={(e) => setId(e.target.value)} placeholder="NG-K3M9-7QRT-B" required />
          </label>
          <label>
            <span>{t('app.callThem')}</span>
            <input value={label} onChange={(e) => setLabel(e.target.value)} required />
          </label>
          {status && <p className="notice notice--bad">{status}</p>}
          <button className="primary">{t('app.savePerson')}</button>
        </form>
      )}

      {/* Listed accounts: system and business accounts that chose to be
          findable. Not everyone on the chain — see directory.ts. */}
      {publicHits.length > 0 && (query || book_ === 'public') && (
        <>
          {query && <h3 className="screen__subtitle">{t('app.publicBook')}</h3>}
          <ul className="cards">
            {publicHits.map((e) => (
              <li key={e.label} className="card">
                <strong>{e.label}</strong>
                <span className="tag">{t(e.kind === 'system' ? 'app.systemAccount' : 'app.service')}</span>
                <div className="muted small-note">{e.note}</div>
                {e.id && <div className="muted">{e.id}</div>}
                {e.unavailable && <div className="notice notice--bad small-note">{e.unavailable}</div>}
              </li>
            ))}
          </ul>
        </>
      )}

      {often.length > 0 && (query || book_ === 'private') && (
        <>
          <h3 className="screen__subtitle">{t('app.oftenUsed')}</h3>
          <ul className="cards">
            {often.map((r) => (
              <li key={r.id} className="card">
                <strong>{book.displayName(r.id) || r.id}</strong>
                <div className="muted">{r.id}</div>
                <button className="linkish" onClick={() => saveFound(r.id)}>{t('app.addPerson')}</button>
              </li>
            ))}
          </ul>
        </>
      )}

      {(query || book_ === 'private') && (
        <>
          {query && <h3 className="screen__subtitle">{t('app.myBook')}</h3>}
          <p className="muted small-note">{t('app.privateNote')}</p>
          {mine.length === 0 && !query && <p className="muted">{t('app.noContactsYet')}</p>}
        </>
      )}

      <ul className="cards">
        {(query || book_ === 'private' ? mine : []).map((c) => (
          <li key={c.id} className="card">
            <strong>{c.label}</strong>
            <div className="muted">{c.id}</div>
            <button className="linkish" onClick={() => { book.remove(c.id); setList(book.contacts()); setRecent(book.recents()); }}>
              {t('app.forget')}
            </button>
          </li>
        ))}
      </ul>

      {looking && <p className="muted">{t('app.searching')}</p>}
      {found && (
        <div className="card">
          <strong>{found.id}</strong>
          <div className="muted small-note">{t('app.foundOnChain')}</div>
          <button className="linkish" onClick={() => saveFound(found.id)}>{t('app.addPerson')}</button>
        </div>
      )}
      {!looking && !found && query && mine.length === 0 && publicHits.length === 0 && often.length === 0 && (
        <p className="muted">{t('app.noMatch')}</p>
      )}
    </>
  );
}



/**
 * Settings, which for now is one thing: handing somebody your ID.
 *
 * Being paid requires the other person to know how to reach you, and the ID is
 * the only thing about an account that is safe to publish — it names you on the
 * chain and discloses nothing else. So it goes out by whatever channel the two
 * people already use: a message, an email, a photograph of a screen. None of
 * them need to be secure, because an ID is public by design.
 */
function Settings({ signer, onClosed }: { signer: Signer; onClosed: () => void }) {
  const [userId, setUserId] = useState('');
  const [copied, setCopied] = useState(false);
  const [showQr, setShowQr] = useState(false);
  const [pane, setPane] = useState<'id' | 'currency' | 'app'>('id');
  const [denom, setDenom] = useState(defaultDenom());
  const [expert, setExpert] = useState(() => localStorage.getItem('yamale.app.expert') === '1');
  const [theme, setThemeState] = useState<'system' | 'light' | 'dark'>(
    () => (localStorage.getItem('yamale.app.theme') as 'system' | 'light' | 'dark') ?? 'system',
  );

  /**
   * Three states, not a switch. "System" is a choice in its own right — it
   * keeps following the phone as the phone dims itself in the evening — and
   * collapsing it into a two-way toggle takes that behaviour away from
   * everybody who never opens this screen.
   */
  function setTheme(next: 'system' | 'light' | 'dark') {
    setThemeState(next);
    localStorage.setItem('yamale.app.theme', next);
    if (next === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', next);
  }
  const [address, setAddress] = useState('');
  const [pw, setPw] = useState('');
  const [phrase, setPhrase] = useState('');
  const [pwError, setPwError] = useState(false);

  // The address is fetched only in expert mode. In simple mode nothing on the
  // screen can render it, which is a stronger guarantee than choosing not to.
  useEffect(() => {
    if (!expert) { setAddress(''); setPhrase(''); setPw(''); return; }
    (async () => setAddress(await signer.internalAddress()))();
  }, [expert, signer]);

  function setMode(on: boolean) {
    setExpert(on);
    localStorage.setItem('yamale.app.expert', on ? '1' : '0');
  }

  async function reveal() {
    setPwError(false);
    const words = await revealPhrase(signer.profile.email, pw);
    if (!words) { setPwError(true); return; }
    setPhrase(words);
    setPw('');
  }

  useEffect(() => {
    (async () => {
      const id = await book.myUserId(await signer.internalAddress());
      if (id) setUserId(id);
    })();
  }, [signer]);

  // Formatted for every surface a person sees or sends. The country is the
  // first group precisely so it can be read at a glance; run together with the
  // payload it is two more characters nobody parses.
  const shown = userId ? formatUserId(userId) : '';
  const message = t('app.shareMessage', { name: signer.profile.name, id: shown });

  async function copy() {
    try {
      await navigator.clipboard.writeText(message);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  async function share() {
    // The platform sheet reaches every app the person already has — WhatsApp,
    // SMS, mail — without this app needing to know any of them exist.
    if (navigator.share) {
      try { await navigator.share({ title: t('app.title'), text: message }); return; } catch { /* cancelled */ }
    }
    copy();
  }

  return (
    <>
      <h2 className="screen__title">{t('app.settings')}</h2>

      {/* Three folders, because settings answer three different questions:
          who am I, what money do I think in, and how should the app behave.
          Flat, they were one column of nine headings that all looked equally
          important. */}
      <div className="folders" role="tablist">
        {([['id', t('app.folderId')], ['currency', t('app.folderCurrency')], ['app', t('app.folderApp')]] as const).map(
          ([id, label]) => (
            <button key={id} type="button" role="tab" aria-selected={pane === id}
                    className={pane === id ? 'folders__tab folders__tab--on' : 'folders__tab'}
                    onClick={() => setPane(id)}>
              {label}
            </button>
          ),
        )}
      </div>

      {pane === 'id' && (
        <>
      <div className="card">
        <div className="muted">{t('app.yourId')}</div>
        <p className="code">{shown || '…'}</p>
        <p className="muted small-note">{t('app.idNote')}</p>
      </div>

      {userId && (
        <>
          {/* The square is shown on demand rather than parked on the page. It
              is only needed when somebody is physically in front of you, and
              until then it is a large block of noise in the settings. */}
          <div className="form">
            <button type="button" className="primary" onClick={share}>{t('app.shareId')}</button>
            <button type="button" className="ghost" onClick={() => setShowQr(true)}>
              <span className="btn-ico" aria-hidden="true">
                <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor"
                     strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="3" y="3" width="7" height="7" rx="1" />
                  <rect x="14" y="3" width="7" height="7" rx="1" />
                  <rect x="3" y="14" width="7" height="7" rx="1" />
                  <path d="M14 14h3v3h-3zM20 14v3M14 20h3M20 20h.01" />
                </svg>
              </span>
              {t('app.showQr')}
            </button>
            <button type="button" className="ghost" onClick={copy}>
              {copied ? t('app.copied') : t('app.copyId')}
            </button>
          </div>

          {showQr && (
            <div className="sheet" role="dialog" aria-modal="true"
                 onClick={() => setShowQr(false)}>
              <div className="sheet__panel" onClick={(e) => e.stopPropagation()}>
                <QrPanel text={userId} size={200} filename="yamale-id.png"
                         labels={{ copy: t('app.copyImage'), copied: t('app.copied'), share: t('app.shareImage'), save: t('app.saveImage') }} />
                <p className="code center">{shown}</p>
                <button type="button" className="ghost" onClick={() => setShowQr(false)}>
                  {t('app.close')}
                </button>
              </div>
            </div>
          )}
        </>
      )}

      <TwoFactor email={signer.profile.email} />

        </>
      )}

      {pane === 'currency' && (
        <>
      {/* Changing this reorders the money screen and nothing else — no
          transaction, no conversion, no effect on what is held. */}
      <h3 className="screen__subtitle">{t('app.defaultCurrency')}</h3>
      <label className="field">
        <select value={denom} onChange={(e) => { setDenom(e.target.value); setDefaultDenom(e.target.value); }}>
          {CURRENCIES.map((c) => (
            <option key={c.denom} value={c.denom}>{c.name} · {c.code}</option>
          ))}
        </select>
      </label>
      <p className="muted small-note">{t('app.defaultCurrencyNote')}</p>

        </>
      )}

      {pane === 'app' && (
        <>
      {/* The app already opens in the phone's language — resolveLocale reads
          navigator.languages before first paint. This is for the case that
          default gets wrong, which is a preference, and preferences live in
          settings. */}
      <h3 className="screen__subtitle">{t('app.language')}</h3>
      <label className="field"><LanguagePicker className="lang" /></label>
      <p className="muted small-note">{t('app.languageNote')}</p>

      {/* Simple is the default and stays the default. Expert is a door, not a
          setting somebody should feel behind for not opening. */}
      <h3 className="screen__subtitle">{t('app.appearance')}</h3>
      <div className="modes">
        {(['system', 'light', 'dark'] as const).map((option) => (
          <button key={option} type="button"
                  className={theme === option ? 'primary' : 'ghost'}
                  onClick={() => setTheme(option)}>
            {t('app.theme_' + option)}
          </button>
        ))}
      </div>

      <BiometricSetting email={signer.profile.email} />

      <h3 className="screen__subtitle">{t('app.mode')}</h3>
      <div className="modes">
        <button type="button" className={expert ? 'ghost' : 'primary'} onClick={() => setMode(false)}>
          {t('app.simpleMode')}
        </button>
        <button type="button" className={expert ? 'primary' : 'ghost'} onClick={() => setMode(true)}>
          {t('app.expertMode')}
        </button>
      </div>
      <p className="muted small-note">{expert ? t('app.modeExpertNote') : t('app.modeSimpleNote')}</p>


      {/* Kept next to the switch that reveals them. Expert mode is the door;
          these are what is behind it. */}
      {expert && (
        <>
          <div className="card">
            <div className="muted">{t('app.technicalAddress')}</div>
            <p className="code">{address}</p>
          </div>

          <div className="card">
            <strong>{t('app.revealPhrase')}</strong>
            {!phrase && (
              <div className="form">
                <label>
                  <span>{t('app.confirmPassword')}</span>
                  <input type="password" value={pw} onChange={(e) => setPw(e.target.value)}
                         autoComplete="current-password" />
                </label>
                {pwError && <p className="notice notice--bad">{t('app.wrongPassword')}</p>}
                <button type="button" className="ghost" onClick={reveal} disabled={!pw}>
                  {t('app.reveal')}
                </button>
              </div>
            )}
            {phrase && (
              <>
                <p className="notice notice--bad">{t('app.phraseWarning')}</p>
                <p className="code phrase">{phrase}</p>
                <button type="button" className="ghost" onClick={() => setPhrase('')}>
                  {t('app.hidePhrase')}
                </button>
              </>
            )}
          </div>
        </>
      )}
        <CloseAccount signer={signer} onClosed={onClosed} />
        </>
      )}

    </>
  );
}

/**
 * Secured payments — the buyer commits, the seller ships, the buyer confirms.
 *
 * The fee is shown as its own line before anything is committed, never folded
 * into the total. Somebody agreeing to pay 1% should see the 1%; a fee that
 * only appears in the arithmetic is a fee somebody will feel tricked by later,
 * and this product is sold on trust.
 */
function Secured({ signer }: { signer: Signer }) {
  const [list, setList] = useState<escrow.Deal[]>([]);
  const [seller, setSeller] = useState('');
  const [amount, setAmount] = useState('');
  const [denom, setDenom] = useState(CURRENCIES[0].denom);
  const [what, setWhat] = useState('');
  const [moderatorId, setModeratorId] = useState(chain.MODERATORS[0].id);

  const [caseFor, setCaseFor] = useState<string | null>(null);
  const [reason, setReason] = useState('');

  useEffect(() => { setList(escrow.deals()); }, []);

  function escalate(id: string, by: escrow.CaseParty) {
    escrow.raiseCase(id, by, reason);
    setCaseFor(null); setReason('');
    setList(escrow.deals());
  }

  const base = amount ? toBaseUnits(amount, denom) : '0';
  const fee = escrow.feeFor(base);
  const total = escrow.totalFor(base);

  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  /**
   * Commit the money on the chain.
   *
   * The local record is written only after the chain accepted it. Writing it
   * first would leave the app showing an escrow that does not exist — and this
   * screen's whole claim is that the money is somewhere specific, so a record
   * that outruns the money is the one lie it must never tell.
   */
  async function start(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setFailed(false);
    try {
      const resolved = await book.resolve(seller);
      if (!resolved) { setFailed(true); return; }

      // The moderator is named by their user id and resolved here, so a deal
      // records the account that was behind that id at the moment it was
      // struck rather than whoever holds the id later.
      const moderator = await book.resolve(moderatorId);
      if (!moderator) { setFailed(true); return; }

      const res = await chain.openEscrow(signer, resolved, moderator, base, denom, what);
      if (!res.ok) { setFailed(true); return; }

      escrow.open({ seller, buyer: signer.profile.name, amount: base, denom, what });
      setList(escrow.deals());
      setSeller(''); setAmount(''); setWhat('');
    } finally {
      setBusy(false);
    }
  }

  function act(id: string, state: escrow.DealState) {
    escrow.update(id, state);
    setList(escrow.deals());
  }

  return (
    <>
      <h2 className="screen__title">{t('app.securedPayment')}</h2>
      <p className="muted small-note">{t('app.securedExplain')}</p>

      <form className="form" onSubmit={start}>
        <label>
          <span>{t('app.payTo')}</span>
          <input value={seller} onChange={(e) => setSeller(e.target.value)} placeholder="NG-K3M9-7QRT-B" required />
        </label>
        <label>
          <span>{t('app.forWhat')}</span>
          <input value={what} onChange={(e) => setWhat(e.target.value)} placeholder={t('app.forWhatHint')} required />
        </label>
        <label>
          <span>{t('app.amount')}</span>
          <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} required />
        </label>
        <label>
          <span>{t('app.currency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {CURRENCIES.map((c) => <option key={c.denom} value={c.denom}>{c.name}</option>)}
          </select>
        </label>

        {/* Chosen before the money moves, never after. */}
        <label>
          <span>{t('app.whoDecides')}</span>
          <select value={moderatorId} onChange={(e) => setModeratorId(e.target.value)}>
            {chain.MODERATORS.map((m) => (
              <option key={m.id} value={m.id}>{m.name} · {m.id}</option>
            ))}
          </select>
        </label>

        {/* Every line stated separately, including the one that is ours. */}
        {amount && (
          <div className="ledger">
            <div><span>{t('app.heldForSeller')}</span><span>{display(base, denom)}</span></div>
            <div><span>{t('app.serviceFee')}</span><span>{display(fee, denom)}</span></div>
            <div className="ledger__total"><span>{t('app.youPay')}</span><span>{display(total, denom)}</span></div>
          </div>
        )}

        {failed && <p className="notice notice--bad">{t('app.escrowFailed')}</p>}
        <button className="primary" disabled={busy}>
          {busy ? t('app.holding') : t('app.startSecured')}
        </button>
        <p className="muted small-note">{t('app.moderationNote')}</p>
      </form>

      {list.length > 0 && <h3 className="screen__subtitle">{t('app.yourDeals')}</h3>}
      <ul className="cards">
        {list.map((d) => (
          <li key={d.id} className="card">
            <strong>{d.what}</strong>
            <div className="muted">{display(d.amount, d.denom)} · {book.displayName(d.seller)}</div>
            <div className={`pill pill--${d.state}`}>{t('app.state_' + d.state)}</div>

            {d.state === 'awaiting_seller' && (
              <div className="confirm__row">
                <button className="linkish" onClick={() => act(d.id, 'in_progress')}>{t('app.sellerAccepted')}</button>
                <button className="linkish" onClick={() => act(d.id, 'refunded')}>{t('app.cancelDeal')}</button>
              </div>
            )}
            {d.state === 'in_progress' && caseFor !== d.id && (
              <div className="confirm__row">
                <button className="linkish" onClick={() => act(d.id, 'released')}>{t('app.gotIt')}</button>
                <button className="linkish" onClick={() => setCaseFor(d.id)}>{t('app.openCase')}</button>
              </div>
            )}

            {/* Either side opens the same form. The moderator is told which,
                because "the buyer says it never came" and "the seller says they
                cannot get confirmed" are different cases with the same shape. */}
            {caseFor === d.id && (
              <div className="confirm">
                <p><strong>{t('app.openCaseTitle')}</strong></p>
                <label className="form">
                  <span>{t('app.caseReason')}</span>
                  <input value={reason} onChange={(e) => setReason(e.target.value)}
                         placeholder={t('app.caseReasonHint')} />
                </label>
                <div className="confirm__row">
                  <button className="ghost" onClick={() => { setCaseFor(null); setReason(''); }}>
                    {t('app.cancel')}
                  </button>
                  <button className="ghost" disabled={!reason} onClick={() => escalate(d.id, 'seller')}>
                    {t('app.asSeller')}
                  </button>
                  <button className="primary" disabled={!reason} onClick={() => escalate(d.id, 'buyer')}>
                    {t('app.asBuyer')}
                  </button>
                </div>
              </div>
            )}

            {d.state === 'in_review' && d.dispute && (
              <p className="muted small-note">
                {t('app.caseOpenedBy', { who: t('app.' + d.dispute.openedBy) })} · {d.dispute.reason}
              </p>
            )}
          </li>
        ))}
      </ul>

      <p className="muted small-note">{t('app.escrowChainNote')}</p>
    </>
  );
}

/**
 * Changing money between currencies.
 *
 * Two numbers are always on screen before anything is signed: what the person
 * will probably get, and the least they will accept. The second is the one that
 * matters — a quote is computed from reserves that move, and a swap sent with
 * no floor is a swap priced by whoever is watching the mempool. Showing it as
 * money rather than a percentage means somebody can decide whether they mind.
 */
/**
 * Money movement that is not a payment: changing one currency for another, and
 * getting money in or out of the chain altogether.
 *
 * Two folders because they answer two different questions. "Change" is internal
 * — you already hold value here and want it in another unit. "Remittance" is the
 * boundary — value entering from, or leaving to, the world outside the chain.
 * Putting them on one screen without the split made the ramp look like just
 * another swap, which is exactly the misunderstanding that loses somebody money.
 */
function Exchange({ signer }: { signer: Signer }) {
  const [folder, setFolder] = useState<'change' | 'remit'>('change');

  return (
    <>
      <div className="folders" role="tablist">
        <button type="button" role="tab" aria-selected={folder === 'change'}
                className={folder === 'change' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                onClick={() => setFolder('change')}>{t('app.tabChange')}</button>
        <button type="button" role="tab" aria-selected={folder === 'remit'}
                className={folder === 'remit' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                onClick={() => setFolder('remit')}>{t('app.tabRemittance')}</button>
      </div>

      {folder === 'change' && <Change signer={signer} />}
      {folder === 'remit' && <Remittance signer={signer} />}
    </>
  );
}

/**
 * The on/off ramp, and the shops that make it real.
 *
 * The order of the screen is the order of the decision: which way is the money
 * going, how much, and then — the part that actually matters to somebody with
 * no bank — who near me will hand it over.
 */
/**
 * Payout methods in the user's own words.
 *
 * "momo" is what the data says and nobody outside the industry would recognise
 * it, so the translation happens here rather than in the shop records — the
 * records stay machine-readable and the screen stays readable.
 */
/**
 * An address, short enough to read but long enough to compare.
 *
 * Both ends, never a prefix alone: addresses on this chain share their first
 * four characters, so "yml1abcd…" tells a reader nothing about which account
 * they are looking at. The tail is where the difference is.
 */
function shortId(address: string): string {
  return address.length > 16 ? `${address.slice(0, 10)}…${address.slice(-6)}` : address;
}

function methodName(method: string): string {
  switch (method) {
    case 'cash': return t('app.methodCash');
    case 'card': return t('app.methodCard');
    case 'bank': return t('app.methodBank');
    case 'momo': return t('app.methodMomo');
    default: return method;
  }
}

function Remittance({ signer }: { signer: Signer }) {
  const [kind, setKind] = useState<ramp.RampKind>('in');
  const [route, setRoute] = useState<ramp.RampRoute>('agent');
  const [denom, setDenom] = useState('uusdc');
  const [amount, setAmount] = useState('');
  const [partner, setPartner] = useState(ramp.PARTNERS[0].id);
  const [here, setHere] = useState<{ lat: number; lon: number } | null>(null);
  const [city, setCity] = useState(agents.CITIES[0].name);
  const [locating, setLocating] = useState(false);
  const [chosen, setChosen] = useState<agents.NearbyAgent | null>(null);
  // Money in can credit somebody else — that is what makes this a remittance
  // rather than an exchange. Money out always leaves the signer's own account.
  const [toSelf, setToSelf] = useState(true);
  const [beneficiary, setBeneficiary] = useState('');
  const [method, setMethod] = useState<string>('cash');
  const [made, setMade] = useState<ramp.RampRequest | null>(null);
  const [requests, setRequests] = useState<ramp.RampRequest[]>(ramp.saved());

  // Ask for location once, on entering the screen. A refusal is normal and
  // falls back to a city list rather than blocking the feature.
  useEffect(() => {
    let live = true;
    setLocating(true);
    agents.locate().then((pos) => {
      if (!live) return;
      setHere(pos);
      setLocating(false);
    });
    return () => { live = false; };
  }, []);

  const origin = here ?? agents.CITIES.find((c) => c.name === city) ?? agents.CITIES[0];
  // Only shops that can actually do this job: they must take the currency being
  // moved, in either direction. Listing one that cannot wastes a journey.
  const found = agents.nearby(origin.lat, origin.lon, denom, 8);

  // The payout the beneficiary picked, from the chosen shop's own list. Falls
  // back to the shop's first so a fee is always shown rather than appearing to
  // be zero.
  const payout = kind === 'out' && chosen
    ? (chosen.payouts.find((x) => x.method === method) ?? chosen.payouts[0])
    : undefined;

  const base = amount ? BigInt(toBaseUnits(amount, denom)) : 0n;
  const feeBps = route === 'agent'
    ? (chosen?.feeBps ?? 100)
    : (ramp.PARTNERS.find((x) => x.id === partner)?.feeBps ?? 100);
  const quote = ramp.quoteRamp(kind, route, denom, base, feeBps,
    route === 'agent' ? (chosen?.name ?? '—') : (ramp.PARTNERS.find((x) => x.id === partner)?.name ?? '—'),
    {
      beneficiary: kind === 'in' && !toSelf ? beneficiary.trim() : undefined,
      payout: payout ? { fiat: payout.fiat, method: payout.method, feeBps: payout.feeBps } : undefined,
    });

  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState('');

  // A shop with no account cannot be paid through an escrow. Said here rather
  // than discovered at the signing step, where it would surface as something
  // about an invalid address.
  const unregistered = route === 'agent' && !!chosen && !chosen.account;

  async function create() {
    if (base <= 0n || busy) return;
    if (route === 'agent' && !chosen) return;
    // A named beneficiary with nothing in the box would credit nobody, and the
    // failure would show up as money that never arrived.
    if (kind === 'in' && !toSelf && !beneficiary.trim()) return;

    setFailed('');
    const req = ramp.newRequest(quote);

    // Money out is the direction this app can commit: the customer holds the
    // tokens, so the customer locks them. Money in is the shop's to lock, and
    // the app's job there is to verify rather than to sign — see depositorSide.
    if (kind === 'out' && route === 'agent' && chosen?.account) {
      setBusy(true);
      const res = await chain.openEscrow(
        signer, chosen.account, chosen.moderator || chosen.account,
        base.toString(), denom, `ramp ${req.reference}`,
      );
      setBusy(false);
      if (!res.ok) { setFailed(t('app.escrowFailed')); return; }
      req.status = 'locked';
    }

    ramp.save(req);
    setMade(req);
    setRequests(ramp.saved());
  }

  async function release(r: ramp.RampRequest) {
    if (!r.lockId || busy) return;
    setBusy(true);
    const res = await chain.releaseEscrow(signer, r.lockId);
    setBusy(false);
    if (!res.ok) { setFailed(t('app.escrowFailed')); return; }
    ramp.updateStatus(r.id, 'settled');
    setRequests(ramp.saved());
  }

  async function raiseCase(r: ramp.RampRequest) {
    if (!r.lockId || busy) return;
    setBusy(true);
    const res = await chain.disputeEscrow(signer, r.lockId, `ramp ${r.reference}`);
    setBusy(false);
    if (!res.ok) { setFailed(t('app.escrowFailed')); return; }
    ramp.updateStatus(r.id, 'disputed');
    setRequests(ramp.saved());
  }

  if (made) {
    return (
      <>
        <h2 className="screen__title">{t('app.rampRequested')}</h2>
        <div className="card card--money">
          <div className="muted">{t('app.showThisCode')}</div>
          <p className="code center">{made.reference}</p>
          <div className="ledger">
            <div><span>{made.kind === 'in' ? t('app.youHandOver') : t('app.youReceive')}</span>
                 <span>{display(made.amount.toString(), made.denom)}</span></div>
            <div><span>{t('app.theirFee')}</span><span>−{display(made.fee.toString(), made.denom)}</span></div>
            <div className="ledger__total"><span>{t('app.netAmount')}</span>
                 <span>{display(made.net.toString(), made.denom)}</span></div>
          </div>
          <p className="muted small-note">{t('app.rampSettleNote', { who: made.counterparty })}</p>
        </div>
        <button type="button" className="primary" onClick={() => { setMade(null); setAmount(''); setChosen(null); }}>
          {t('app.done')}
        </button>
      </>
    );
  }

  return (
    <>
      {/* Direction first: everything below means the opposite thing if this is
          wrong, so it is a segmented control rather than a buried dropdown. */}
      <div className="folders" role="tablist">
        <button type="button" role="tab" aria-selected={kind === 'in'}
                className={kind === 'in' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                onClick={() => setKind('in')}>{t('app.moneyIn')}</button>
        <button type="button" role="tab" aria-selected={kind === 'out'}
                className={kind === 'out' ? 'folders__tab folders__tab--on' : 'folders__tab'}
                onClick={() => setKind('out')}>{t('app.moneyOut')}</button>
      </div>

      <div className="form">
        <label>
          <span>{t('app.currency')}</span>
          <select value={denom} onChange={(e) => setDenom(e.target.value)}>
            {ramp.rampCurrencies().map((c) => (
              <option key={c.denom} value={c.denom}>{c.name} · {c.code}</option>
            ))}
          </select>
        </label>
        <label>
          <span>{t('app.amount')}</span>
          <input inputMode="decimal" value={amount} placeholder="0"
                 onChange={(e) => setAmount(groupDigits(e.target.value))} />
        </label>
      </div>

      {kind === 'in' ? (
        <>
          <h3 className="screen__subtitle">{t('app.landsIn')}</h3>
          <div className="row-actions">
            <button type="button" className={toSelf ? 'primary' : 'ghost'}
                    onClick={() => setToSelf(true)}>{t('app.myAccount')}</button>
            <button type="button" className={!toSelf ? 'primary' : 'ghost'}
                    onClick={() => setToSelf(false)}>{t('app.someoneElse')}</button>
          </div>
          {!toSelf && (
            <>
              <div className="form">
                <label>
                  <span>{t('app.beneficiaryId')}</span>
                  <input value={beneficiary} autoCapitalize="characters" spellCheck={false}
                         onChange={(e) => setBeneficiary(e.target.value)} />
                </label>
              </div>
              <p className="muted small-note">{t('app.beneficiaryHint')}</p>
            </>
          )}
        </>
      ) : (
        <p className="muted small-note">{t('app.fromYourAccount')}</p>
      )}

      <h3 className="screen__subtitle">{t('app.howToSettle')}</h3>
      <div className="row-actions">
        <button type="button" className={route === 'agent' ? 'primary' : 'ghost'}
                onClick={() => setRoute('agent')}>{t('app.viaAgent')}</button>
        <button type="button" className={route === 'partner' ? 'primary' : 'ghost'}
                onClick={() => setRoute('partner')}>{t('app.viaPartner')}</button>
      </div>

      {route === 'partner' && (
        <>
          <div className="form">
            <label>
              <span>{t('app.partner')}</span>
              <select value={partner} onChange={(e) => setPartner(e.target.value)}>
                {ramp.PARTNERS.map((x) => (
                  <option key={x.id} value={x.id}>{x.name} — {(x.feeBps / 100).toFixed(2)}%</option>
                ))}
              </select>
            </label>
          </div>
          <p className="muted small-note">
            {ramp.PARTNERS.find((x) => x.id === partner)?.note}
          </p>
        </>
      )}

      {route === 'agent' && (
        <>
          {locating && <p className="muted">{t('app.findingYou')}</p>}
          {!locating && !here && (
            <div className="form">
              <label>
                <span>{t('app.pickCity')}</span>
                <select value={city} onChange={(e) => setCity(e.target.value)}>
                  {agents.CITIES.map((c) => <option key={c.name} value={c.name}>{c.name}</option>)}
                </select>
              </label>
            </div>
          )}
          {here && <p className="muted small-note">{t('app.usingYourLocation')}</p>}

          <ul className="cards">
            {found.map((a) => (
              <li key={a.id}
                  className={chosen?.id === a.id ? 'card card--money' : 'card acct-row--tap'}
                  onClick={() => setChosen(a)}>
                <strong>{a.name}</strong>
                <span className="tag">{a.km < 1 ? `${Math.round(a.km * 1000)} m` : `${a.km.toFixed(1)} km`}</span>
                <div className="muted small-note">{a.address}, {a.city}</div>
                <div className="muted small-note">
                  {t('app.agentAccepts')}: {a.accepts.map((d) => currencyOf(d)?.code ?? d).join(', ')}
                </div>
                <div className="muted small-note">
                  {t('app.agentPaysOut')}: {a.payouts.map((x) =>
                    `${x.fiat} ${methodName(x.method)}${x.feeBps ? ` +${(x.feeBps / 100).toFixed(2)}%` : ''}`
                  ).join(' · ')}
                </div>
                <div className="muted small-note">
                  {t('app.shopFee')} {(a.feeBps / 100).toFixed(2)}%
                  {' · '}{a.settlements.toLocaleString()} {t('app.settlements')}
                </div>
                <div className="muted small-note">{a.hours} · {a.phone}</div>
              </li>
            ))}
          </ul>
        </>
      )}

      {kind === 'out' && route === 'agent' && chosen && chosen.payouts.length > 1 && (
        <div className="form">
          <label>
            <span>{t('app.howTheyTakeIt')}</span>
            <select value={method} onChange={(e) => setMethod(e.target.value)}>
              {chosen.payouts.map((x) => (
                <option key={x.method + x.fiat} value={x.method}>
                  {x.fiat} — {methodName(x.method)}
                  {x.feeBps ? ` +${(x.feeBps / 100).toFixed(2)}%` : ''}
                </option>
              ))}
            </select>
          </label>
        </div>
      )}

      {route === 'agent' && found.length === 0 && (
        <p className="muted small-note">{t('app.noShopForThat')}</p>
      )}

      {route === 'agent' && chosen && (
        <p className="muted small-note">
          {ramp.depositorSide(kind) === 'customer'
            ? t('app.youLockFirst')
            : t('app.shopLocksFirst')}
          {chosen.moderator ? ` ${t('app.moderatedBy')} ${shortId(chosen.moderator)}.` : ''}
        </p>
      )}

      {unregistered && <p className="muted small-note">{t('app.shopNotRegistered')}</p>}

      {failed && <p className="error-note">{failed}</p>}

      {base > 0n && (route === 'partner' || chosen) && (
        <>
          <div className="ledger">
            <div><span>{kind === 'in' ? t('app.youHandOver') : t('app.youReceive')}</span>
                 <span>{display(base.toString(), denom)}</span></div>
            <div><span>{t('app.theirFee')} ({(feeBps / 100).toFixed(2)}%)</span>
                 <span>−{display(quote.fee.toString(), denom)}</span></div>
            <div className="ledger__total"><span>{t('app.netAmount')}</span>
                 <span>{display(quote.net.toString(), denom)}</span></div>
            {kind === 'in' && (
              <div><span>{t('app.landsIn')}</span>
                   <span>{quote.beneficiary || t('app.myAccount')}</span></div>
            )}
            {quote.payout && (
              <div><span>{t('app.howTheyTakeIt')}</span>
                   <span>{quote.payout.fiat} · {methodName(quote.payout.method)}</span></div>
            )}
          </div>
          <button type="button" className="primary" disabled={busy || unregistered}
                  onClick={create}>
            {busy ? t('app.working')
              : ramp.depositorSide(kind) === 'customer' ? t('app.lockAndRequest')
              : t('app.requestRamp')}
          </button>
        </>
      )}

      {requests.length > 0 && (
        <>
          <h3 className="screen__subtitle">{t('app.yourRequests')}</h3>
          <ul className="cards">
            {requests.slice(0, 5).map((r) => (
              <li key={r.id} className="card">
                <strong>{r.reference}</strong>
                <span className="tag">{r.kind === 'in' ? t('app.moneyIn') : t('app.moneyOut')}</span>
                {r.status === 'locked' && <span className="tag">{t('app.heldInEscrow')}</span>}
                {r.status === 'disputed' && <span className="tag">{t('app.underReview')}</span>}
                {r.status === 'locked' && r.lockId && r.kind === 'out' && (
                  <div className="row-actions">
                    <button type="button" className="primary" disabled={busy}
                            onClick={() => release(r)}>{t('app.cashReceived')}</button>
                    <button type="button" className="ghost" disabled={busy}
                            onClick={() => raiseCase(r)}>{t('app.raiseCase')}</button>
                  </div>
                )}
                <div className="muted small-note">
                  {display(r.net.toString(), r.denom)} · {r.counterparty}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </>
  );
}

function Change({ signer }: { signer: Signer }) {
  const [list, setList] = useState<swaps.Pool[]>([]);
  const [from, setFrom] = useState(CURRENCIES[0].denom);
  const [to, setTo] = useState(CURRENCIES[1].denom);
  const [amount, setAmount] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [failed, setFailed] = useState(false);

  const [held, setHeld] = useState<Map<string, string>>(new Map());

  useEffect(() => { swaps.pools().then(setList); }, []);

  // Balances beside the currency names. Choosing what to give away without
  // seeing what you have means picking a currency, typing an amount, and only
  // then being told it is too much — three steps to learn something that fits
  // on one line.
  useEffect(() => {
    let live = true;
    (async () => {
      const address = await signer.internalAddress();
      try {
        const res = await fetch(`/api/rest/cosmos/bank/v1beta1/balances/${address}`);
        const json = await res.json();
        const map = new Map<string, string>();
        for (const b of json.balances ?? []) map.set(b.denom, b.amount);
        if (live) setHeld(map);
      } catch {
        // An unreachable node is not a reason to block the screen; the amount
        // field still works and the chain refuses anything unaffordable.
      }
    })();
    return () => { live = false; };
  }, [signer, done]);

  const balanceOf = (denom: string) => held.get(denom) ?? '0';
  const label = (c: { denom: string; name: string }) =>
    `${c.name} — ${display(balanceOf(c.denom), c.denom)}`;

  const pool = swaps.findPool(list, from, to);
  const base = amount ? BigInt(toBaseUnits(amount, from)) : 0n;
  const estimate = pool ? swaps.quote(pool, from, base) : 0n;
  const floor = swaps.floorFor(estimate);
  const impact = pool ? swaps.priceImpactBps(pool, from, base, estimate) : 0;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!pool || estimate <= 0n) return;
    setBusy(true); setFailed(false);
    try {
      const res = await chain.swap(
        signer, pool.id, from, base.toString(), to, floor.toString(),
      );
      if (res.ok) { setDone(true); setAmount(''); } else { setFailed(true); }
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <div className="done">
        <div className="done__tick">✓</div>
        <h2>{t('app.changed')}</h2>
        <button className="primary" onClick={() => setDone(false)}>{t('app.done')}</button>
      </div>
    );
  }

  return (
    <>
      <h2 className="screen__title">{t('app.changeMoney')}</h2>

      <form className="form" onSubmit={submit}>
        <label>
          <span>{t('app.youGive')}</span>
          <select value={from} onChange={(e) => setFrom(e.target.value)}>
            {CURRENCIES.map((c) => <option key={c.denom} value={c.denom}>{label(c)}</option>)}
          </select>
        </label>
        <label>
          <span>{t('app.amount')}</span>
          <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} required />
        </label>
        {/* Fractions rather than one "use it all". Most people moving money are
            moving part of it, and a single max button makes the common case the
            one that needs arithmetic. Max stays because it is the one figure
            nobody should retype and risk a digit. */}
        {/* A segmented control, not a row of loose pills. The four choices are
            one decision — how much of what I have — so they read as one object,
            and the balance labels the thing they divide rather than competing
            with them. */}
        <div className="amounts">
          <div className="amounts__seg" role="group">
            {([10, 25, 50, 100] as const).map((pct) => (
              <button key={pct} type="button" className="amounts__btn"
                      onClick={() => setAmount(rawAmount(
                        (BigInt(balanceOf(from)) * BigInt(pct) / 100n).toString(), from))}>
                {pct === 100 ? t('app.max') : `${pct}%`}
              </button>
            ))}
          </div>
        </div>
        {base > 0n && BigInt(balanceOf(from)) < base && (
          <p className="notice notice--bad">{t('send.tooMuch')}</p>
        )}
        <label>
          <span>{t('app.youGet')}</span>
          <select value={to} onChange={(e) => setTo(e.target.value)}>
            {CURRENCIES.filter((c) => c.denom !== from).map((c) => (
              <option key={c.denom} value={c.denom}>{label(c)}</option>
            ))}
          </select>
        </label>

        {!pool && amount && <p className="notice notice--bad">{t('app.noPool')}</p>}

        {pool && estimate > 0n && (
          <div className="ledger">
            <div><span>{t('app.youGetAbout')}</span><span>{display(estimate.toString(), to)}</span></div>
            {/* Named and quantified. Somebody who can see the number can decide
                whether to split the trade; somebody who cannot just thinks the
                rate is bad. */}
            <div className={impact >= 300 ? 'impact impact--high' : undefined}>
              <span>{t('app.priceImpact')}</span>
              <span>{(impact / 100).toFixed(2)}%</span>
            </div>
            {/* The promise, not the hope. */}
            <div className="ledger__total"><span>{t('app.atLeast')}</span><span>{display(floor.toString(), to)}</span></div>
          </div>
        )}

        {failed && <p className="notice notice--bad">{t('app.changeFailed')}</p>}

        <button className="primary"
                disabled={busy || !pool || estimate <= 0n || BigInt(balanceOf(from)) < base}>
          {busy ? t('app.changing') : t('app.changeNow')}
        </button>
        {impact >= 300 && (
          <p className="muted small-note">{t('app.thinPool')}</p>
        )}
        <p className="muted small-note">{t('app.floorNote')}</p>
      </form>
    </>
  );
}

/**
 * Enrolling a second factor.
 *
 * The three methods are listed with their real strength stated, not as a menu
 * of equals. Somebody choosing SMS should know they are choosing the weakest
 * one, and somebody choosing an authenticator app should know it is the only
 * one that keeps working with no signal.
 */
function TwoFactor({ email }: { email: string }) {
  const [current, setCurrent] = useState(twofa.enrolment());
  const [secret, setSecret] = useState('');
  const [otp, setOtp] = useState('');
  const [error, setError] = useState(false);

  function begin() {
    setSecret(twofa.newSecret());
    setOtp(''); setError(false);
  }

  async function confirm() {
    if (await twofa.verify(secret, otp)) {
      const e = { method: 'totp' as const, secret, confirmedAt: Date.now() };
      twofa.save(e);
      setCurrent(e);
      setSecret(''); setOtp('');
    } else {
      setError(true);
    }
  }

  if (current) {
    return (
      <div className="card">
        <strong>{t('app.twoFactor')}</strong>
        <div className="muted">{t('app.twoFactorOn')}</div>
        <button className="linkish" onClick={() => { twofa.disable(); setCurrent(null); }}>
          {t('app.turnOff')}
        </button>
      </div>
    );
  }

  return (
    <div className="card">
      <strong>{t('app.twoFactor')}</strong>

      {!secret && (
        <div className="form">
          <button type="button" className="ghost" onClick={begin}>{t('app.useAuthApp')}</button>
          <p className="muted small-note">{t('app.authAppNote')}</p>

          {/* Built and honestly disabled. A demo that prints itself a pretend
              code teaches the audience the wrong thing about what is real. */}
          <button type="button" className="ghost" disabled>{t('app.useEmail')}</button>
          <p className="muted small-note">{t('app.emailNote')}</p>
          <button type="button" className="ghost" disabled>{t('app.useSms')}</button>
          <p className="muted small-note">{t('app.smsNote')}</p>
        </div>
      )}

      {secret && (
        <>
          <p className="muted small-note">{t('app.scanWithApp')}</p>
          <div className="qr-wrap"><QrCode text={twofa.otpauthUri(secret, email)} size={170} /></div>
          <p className="code">{secret}</p>
          <div className="form">
            <label>
              <span>{t('app.sixDigits')}</span>
              <input inputMode="numeric" maxLength={6} value={otp}
                     onChange={(e) => { setOtp(e.target.value); setError(false); }} />
            </label>
            {error && <p className="notice notice--bad">{t('app.badCode')}</p>}
            {/* Confirmed by a working code, never by pressing "done". An
                enrolment nobody proved is a lockout waiting to happen. */}
            <button type="button" className="primary" onClick={confirm} disabled={otp.length < 6}>
              {t('app.confirm')}
            </button>
            <button type="button" className="linkish" onClick={() => setSecret('')}>{t('app.cancel')}</button>
          </div>
        </>
      )}
    </div>
  );
}

/**
 * Tab icons, drawn as strokes rather than shipped as a font.
 *
 * `currentColor` means the active state needs no second copy of each glyph, and
 * the whole set weighs less than one webfont request — which matters on a
 * connection where every request is paid for by the person making it.
 */
function TabIcon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    // A note and coins: money you hold.
    wallet: 'M3 7h14a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2V7zm0 0l2-3h10l2 3M16 12h1',
    // An arrow leaving.
    send: 'M4 12h12m0 0l-4-4m4 4l-4 4',
    // Two arrows passing, which is what an exchange looks like.
    swap: 'M4 8h11m0 0l-3-3m3 3l-3 3M16 15H5m0 0l3 3m-3-3l3-3',
    // A shield: money held safely.
    shield: 'M10 3l6 2v5c0 4-2.6 6.4-6 8-3.4-1.6-6-4-6-8V5l6-2z',
    // A square with a code in it.
    qr: 'M4 4h4v4H4V4zm8 0h4v4h-4V4zM4 12h4v4H4v-4zm8 2h2v2h-2v-2zm2-2h2v2h-2v-2z',
    // Two figures.
    people: 'M7 9a2.5 2.5 0 100-5 2.5 2.5 0 000 5zm0 2c-2.5 0-4 1.4-4 3v2h8v-2c0-1.6-1.5-3-4-3zm7-2a2 2 0 100-4 2 2 0 000 4zm3 7v-2c0-1.3-1-2.4-3-2.8',
    gear: 'M10 7.5a2.5 2.5 0 100 5 2.5 2.5 0 000-5zM10 2.5l1 2 2.2-.6 1.4 1.4-.6 2.2 2 1v2l-2 1 .6 2.2-1.4 1.4-2.2-.6-1 2H9l-1-2-2.2.6-1.4-1.4.6-2.2-2-1v-2l2-1-.6-2.2L5.8 3.9 8 4.5l1-2h1z',
  };
  return (
    <svg className="tab__icon" viewBox="0 0 20 20" aria-hidden="true"
         fill="none" stroke="currentColor" strokeWidth="1.5"
         strokeLinecap="round" strokeLinejoin="round">
      <path d={paths[name] ?? ''} />
    </svg>
  );
}

/**
 * Sending, escrow and asking to be paid, on one screen.
 *
 * They were three tabs, which made the bar seven wide and put three names in
 * front of somebody whose question was simply "how do I move money". They are
 * one decision with three answers — pay now, pay with protection, ask to be
 * paid — so they belong in one place with the choice made visible rather than
 * hidden in navigation.
 */
function Transfer({ signer }: { signer: Signer }) {
  const [folder, setFolder] = useState<'pay' | 'secured' | 'request'>('pay');

  const folders = [
    { id: 'pay', label: t('app.tabPay') },
    { id: 'secured', label: t('app.tabSecured') },
    { id: 'request', label: t('app.tabRequest') },
  ] as const;

  return (
    <>
      <div className="folders" role="tablist">
        {folders.map((f) => (
          <button key={f.id} type="button" role="tab"
                  aria-selected={folder === f.id}
                  className={folder === f.id ? 'folders__tab folders__tab--on' : 'folders__tab'}
                  onClick={() => setFolder(f.id)}>
            {f.label}
          </button>
        ))}
      </div>

      {folder === 'pay' && <Pay signer={signer} />}
      {folder === 'secured' && <Secured signer={signer} />}
      {folder === 'request' && <Request signer={signer} />}
    </>
  );
}

/**
 * Closing the account.
 *
 * Two things have to be true before this is safe to offer: the money has
 * somewhere to go, and the person understands what does not go away. So the
 * destination is required whenever a balance exists — not optional with a
 * warning, required — and the wording is explicit that the chain keeps the
 * account and the user ID forever, because it does.
 */
function CloseAccount({ signer, onClosed }: { signer: Signer; onClosed: () => void }) {
  const [open, setOpen] = useState(false);
  const [to, setTo] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [holdings, setHoldings] = useState<{ denom: string; amount: string }[]>([]);

  useEffect(() => {
    if (!open) return;
    (async () => {
      const address = await signer.internalAddress();
      const held = await balances(address);
      setHoldings([...held.entries()]
        .filter(([, amount]) => BigInt(amount) > 0n)
        .map(([denom, amount]) => ({ denom, amount })));
    })();
  }, [open, signer]);

  const hasMoney = holdings.length > 0;

  async function close() {
    setError('');
    setBusy(true);
    try {
      if (hasMoney) {
        const destination = await book.resolve(to.trim().toUpperCase());
        if (!destination) { setError(t('app.unknownId')); setBusy(false); return; }

        const result = await chain.sweepTo(signer, destination);
        // Erasing after a failed sweep would destroy the only key that can
        // reach the money. The account stays open and the person keeps trying.
        if (!result.ok) { setError(t('app.sweepFailed')); setBusy(false); return; }
      }
      eraseEverything();
      onClosed();
    } catch {
      setError(t('app.sweepFailed'));
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <>
        <h3 className="screen__subtitle">{t('app.closeAccount')}</h3>
        <button type="button" className="ghost danger" onClick={() => setOpen(true)}>
          {t('app.closeAccount')}
        </button>
      </>
    );
  }

  return (
    <>
      <h3 className="screen__subtitle">{t('app.closeAccount')}</h3>
      <div className="card card--danger">
        <p className="notice notice--bad">{t('app.closeWarning')}</p>

        {hasMoney && (
          <>
            <p><strong>{t('app.closeHasMoney')}</strong></p>
            <ul className="cards">
              {holdings.map((h) => (
                <li key={h.denom} className="acct-row">
                  <span className="acct-row__name">{currencyOf(h.denom)?.name}</span>
                  <span className="acct-row__amount">{display(h.amount, h.denom)}</span>
                </li>
              ))}
            </ul>
            <label>
              <span>{t('app.closeSendTo')}</span>
              <input value={to} onChange={(e) => setTo(e.target.value)}
                     placeholder="NG-K3M9-7QRT-B" autoCapitalize="characters" />
            </label>
          </>
        )}

        {!hasMoney && <p className="muted">{t('app.closeNoMoney')}</p>}
        {error && <p className="notice notice--bad">{error}</p>}

        <div className="row-actions">
          <button type="button" className="ghost" onClick={() => { setOpen(false); setError(''); }}>
            {t('app.cancel')}
          </button>
          <button type="button" className="danger-solid"
                  disabled={busy || (hasMoney && to.trim().length < 6)}
                  onClick={close}>
            {busy ? t('app.checking') : t('app.closeConfirm')}
          </button>
        </div>
      </div>
    </>
  );
}

/**
 * Turning fingerprint unlock on.
 *
 * It asks for the password again rather than reusing the session, because the
 * password is the thing being sealed and the app deliberately does not keep it
 * after sign-in. Asking is also the honest moment to confirm intent: this puts
 * a copy of the password on the device, wrapped, and somebody should say yes to
 * that explicitly.
 */
function BiometricSetting({ email }: { email: string }) {
  const [can, setCan] = useState(false);
  const [on, setOn] = useState(biometric.enrolled());
  const [pw, setPw] = useState('');
  const [asking, setAsking] = useState(false);
  const [note, setNote] = useState('');

  useEffect(() => { biometric.available().then(setCan); }, []);
  if (!can) return null;

  async function turnOn(e: React.FormEvent) {
    e.preventDefault();
    setNote('');
    // Verify the password really is this account's before sealing it, so a
    // typo cannot enrol a credential that unlocks nothing.
    try {
      await signIn(email, pw);
    } catch {
      setNote(t('app.wrongPassword'));
      return;
    }
    const ok = await biometric.enrol(email, pw);
    setPw('');
    setAsking(false);
    setOn(ok);
    if (!ok) setNote(t('app.biometricFailed'));
  }

  return (
    <>
      <h3 className="screen__subtitle">{t('app.biometricSetup')}</h3>
      {on && (
        <>
          <p className="muted small-note">{t('app.biometricOn')}</p>
          <button type="button" className="ghost" onClick={async () => { await biometric.forget(); setOn(false); }}>
            {t('app.biometricOff')}
          </button>
        </>
      )}
      {!on && !asking && (
        <button type="button" className="ghost" onClick={() => setAsking(true)}>
          {t('app.biometricEnable')}
        </button>
      )}
      {!on && asking && (
        <form className="form" onSubmit={turnOn}>
          <label>
            <span>{t('app.confirmPassword')}</span>
            <input type="password" value={pw} onChange={(e) => setPw(e.target.value)}
                   autoComplete="current-password" />
          </label>
          {note && <p className="notice notice--bad">{note}</p>}
          <button className="primary" disabled={!pw}>{t('app.biometricEnable')}</button>
        </form>
      )}
    </>
  );
}

function TabBar({ current, onChange }: { current: string; onChange: (s: 'home' | 'pay' | 'change' | 'contacts' | 'settings') => void }) {
  // Icon *and* label, not icon alone. Seven tabs of unlabelled glyphs is a
  // guessing game, and the guess costs money here. The icon is what the eye
  // finds; the word is what settles what it means — and in a five-language
  // interface the word does more work than the picture.
  const tabs = [
    { id: 'home', label: t('app.tabHome'), icon: 'wallet' },
    { id: 'pay', label: t('app.tabTransfer'), icon: 'send' },
    { id: 'change', label: t('app.tabChange'), icon: 'swap' },
    { id: 'contacts', label: t('app.tabContacts'), icon: 'people' },
    { id: 'settings', label: t('app.tabSettings'), icon: 'gear' },
  ] as const;
  return (
    <nav className="tabbar">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          className={current === tab.id ? 'tab tab--on' : 'tab'}
          onClick={() => onChange(tab.id)}
        >
          <TabIcon name={tab.icon} />
          <span className="tab__label">{tab.label}</span>
        </button>
      ))}
    </nav>
  );
}
