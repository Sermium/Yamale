/**
 * Yamale Vehicles — the shell.
 *
 * Three screens, and the order they are in is the order somebody decides in:
 * the vehicles on offer, then one vehicle in detail, then what they already
 * own. Everything on the first two is public and readable without an account,
 * because the moment an investor most needs to check a vehicle is before they
 * have anything invested in it.
 */
import { useState } from 'react';
import { HashRouter, Link, NavLink, Route, Routes, useParams } from 'react-router-dom';
import { LanguagePicker, t, useLocale } from '@yamale/chain';

import { useAccount } from './account.ts';
import { addressOf, canSign, type Account } from './address.ts';
import { CHAIN_ID } from './chain.ts';
import { chainNow, useHead } from './data.ts';
import { Holdings } from './Holdings.tsx';
import { Vehicle } from './Vehicle.tsx';
import { Vehicles } from './Vehicles.tsx';
import { Address, Chip } from './ui.tsx';

/* ------------------------------------------------------------------ theme */

type Theme = 'light' | 'dark' | 'system';

function savedTheme(): Theme {
  try {
    const v = localStorage.getItem('yamale.rwa.theme');
    if (v === 'light' || v === 'dark') return v;
  } catch { /* private mode */ }
  return 'system';
}

function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(savedTheme);

  const apply = (next: Theme) => {
    setTheme(next);
    if (next === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', next);
    try {
      if (next === 'system') localStorage.removeItem('yamale.rwa.theme');
      else localStorage.setItem('yamale.rwa.theme', next);
    } catch { /* private mode */ }
  };

  const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';
  const label = theme === 'dark' ? t('rwa.themeDark')
    : theme === 'light' ? t('rwa.themeLight') : t('rwa.themeSystem');

  return (
    <button type="button" className="bar__btn" onClick={() => apply(next)}
            aria-label={`${t('rwa.theme')}: ${label}`}>
      <span aria-hidden="true">{theme === 'dark' ? '◐' : theme === 'light' ? '○' : '◑'}</span>
      <span className="bar__btnLabel">{label}</span>
    </button>
  );
}

/* ---------------------------------------------------------------- account */

function AccountBar({ api }: { api: ReturnType<typeof useAccount> }) {
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const { account, wallets, connecting, problemKey } = api;

  if (account.mode !== 'none') {
    return (
      <div className="who">
        <Chip tone={canSign(account) ? 'ok' : 'mute'}>
          {canSign(account) ? t('rwa.connected') : t('rwa.watching')}
        </Chip>
        <Address value={addressOf(account)} />
        <button type="button" className="bar__btn" onClick={api.forget}>
          {t('action.disconnect')}
        </button>
      </div>
    );
  }

  if (!open) {
    return (
      <button type="button" className="btn btn--small" onClick={() => setOpen(true)}>
        {t('rwa.attach')}
      </button>
    );
  }

  return (
    <div className="attach">
      <p className="attach__lede">{t('rwa.attachLede')}</p>

      {wallets.length > 0 ? (
        <div className="attach__wallets">
          {wallets.map((w) => (
            <button key={w.id} type="button" className="btn btn--small"
                    disabled={connecting} onClick={() => api.connectWallet(w.id)}>
              {connecting ? t('rwa.connecting') : `${t('action.connect')} ${w.label}`}
            </button>
          ))}
        </div>
      ) : (
        <p className="attach__none">{t('rwa.noWallet')}</p>
      )}

      <form
        className="attach__watch"
        onSubmit={(e) => {
          e.preventDefault();
          if (api.watch(typed)) { setOpen(false); setTyped(''); }
        }}
      >
        <label className="attach__label" htmlFor="watch-address">{t('rwa.watchLabel')}</label>
        <div className="attach__row">
          <input
            id="watch-address"
            className="y-mono"
            value={typed}
            spellCheck={false}
            autoComplete="off"
            placeholder="yml1…"
            onChange={(e) => setTyped(e.target.value)}
          />
          <button type="submit" className="btn btn--small btn--ghost">{t('rwa.watchGo')}</button>
        </div>
        <p className="attach__hint">{t('rwa.watchHint')}</p>
      </form>

      {problemKey && <p className="attach__problem" role="alert">{t(problemKey)}</p>}
    </div>
  );
}

/* ------------------------------------------------------------------- shell */

function VehicleRoute({ account }: { account: Account }) {
  const { assetId = '' } = useParams();
  return <Vehicle assetId={assetId} account={account} />;
}

export function App() {
  const api = useAccount();
  const head = useHead();
  const locale = useLocale();
  const now = chainNow(head);

  return (
    <HashRouter>
      <a className="skip" href="#main">{t('rwa.skip')}</a>

      <header className="bar">
        <div className="bar__inner">
          <Link to="/" className="brand">
            <span className="brand__mark" aria-hidden="true">
              <svg viewBox="0 0 32 32" width="26" height="26" fill="none" stroke="currentColor"
                   strokeWidth="1.8" strokeLinejoin="round">
                <path d="M5 12 16 5l11 7v15H5z" />
                <path d="M12 27v-9h8v9" />
              </svg>
            </span>
            <span className="brand__name">{t('rwa.title')}</span>
          </Link>

          <nav className="bar__nav" aria-label={t('rwa.navLabel')}>
            <NavLink to="/" end className={({ isActive }) => `bar__link ${isActive ? 'is-on' : ''}`}>
              {t('rwa.navVehicles')}
            </NavLink>
            <NavLink to="/holdings" className={({ isActive }) => `bar__link ${isActive ? 'is-on' : ''}`}>
              {t('rwa.navHoldings')}
            </NavLink>
          </nav>

          <div className="bar__right">
            <AccountBar api={api} />
            <ThemeToggle />
            <LanguagePicker className="bar__lang" />
          </div>
        </div>
      </header>

      <main id="main" className="main">
        <Routes>
          <Route path="/" element={<Vehicles now={now.at} />} />
          <Route path="/v/:assetId" element={<VehicleRoute account={api.account} />} />
          <Route path="/holdings" element={<Holdings account={api.account} />} />
        </Routes>
      </main>

      <footer className="foot">
        <div className="foot__inner">
          <p className="foot__note">{t('rwa.footNote')}</p>
          <p className="foot__chain y-mono">
            {/*
              The height is not decoration. Every claim this app makes about a
              vehicle — that no sale has been reported, that an authorisation is
              live, that nothing has been issued — is a claim about a moment,
              and a claim shown without the block it was read at is one nobody
              can check or date.
            */}
            {head.known
              ? `${head.chainId} · ${t('rwa.block')} ${head.height.toLocaleString(locale)}`
              : t('rwa.chainUnknown')}
            {head.known && head.catchingUp && ` · ${t('rwa.catchingUp')}`}
            {head.known && !now.fromChain && ` · ${t('rwa.localClock')}`}
          </p>
          <p className="foot__chain y-mono">{CHAIN_ID}</p>
        </div>
      </footer>
    </HashRouter>
  );
}
