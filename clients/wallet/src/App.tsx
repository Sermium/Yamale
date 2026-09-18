/**
 * Yamale Wallet — a phone on a desk.
 *
 * This surface used to be a dashboard: a masthead, a horizontal menu, an
 * account dropdown, and tables of denoms. That is the right shape for somebody
 * supervising a registry and the wrong one for somebody checking what they hold
 * on the way to work — and a wallet is the most obviously phone-shaped thing
 * there is. So it now wears the shell Yamale Pay was designed as, taken from
 * `clients/shared/yamale.css` rather than copied from Pay: `.y-stage` is the
 * desk, `.y-phone` the device, `.y-appbar` and `.y-tabbar` the two fixed bars,
 * `.y-screen` the one job in between.
 *
 * Five tabs, and the split is by question rather than by feature:
 *
 *   Money    what do I have, can it move, who pays the fee
 *   Send     paying somebody
 *   Receive  how somebody pays me, and the devnet faucet
 *   Look up  what does *that* account hold — needs no key and risks nothing
 *   Account  who this device is, how it is attached, and the settings
 *
 * The one screen that is not in the frame is `/connect`. It is a window another
 * application opens at a size it chose, so drawing a picture of a phone inside
 * it would be a picture of a device inside a device. It keeps its own layout.
 */
import { useEffect, useState } from 'react';
import { Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { t, useLocale } from '@yamale/chain';

import { ConnectPage } from './pages/Connect.tsx';
import { Holdings } from './Holdings.tsx';
import { Attach } from './screens/Attach.tsx';
import { SendScreen } from './screens/Send.tsx';
import { ReceiveScreen } from './screens/Receive.tsx';
import { LookupScreen } from './screens/Lookup.tsx';
import { AccountScreen } from './screens/Account.tsx';
import { getUnlocked, lock, vaultSummary } from './vault.ts';
import { ScreenTitle, TabIcon } from './ui.tsx';

export function App() {
  // Subscribing here re-renders every screen below when the language changes,
  // which is what a full reload used to accomplish.
  useLocale();

  return (
    <Routes>
      {/* The approval window, outside the frame on purpose. */}
      <Route path="/connect" element={<div className="wal-window"><ConnectPage /></div>} />
      <Route path="*" element={<Phone />} />
    </Routes>
  );
}

type Tab = 'money' | 'send' | 'receive' | 'lookup' | 'account';

const PATHS: Record<Tab, string> = {
  money: '/',
  send: '/send',
  receive: '/receive',
  lookup: '/lookup',
  account: '/account',
};

function Phone() {
  const navigate = useNavigate();
  const location = useLocation();

  // The vault is written by screens inside this same tab, so no storage event
  // ever fires; re-reading on navigation is what keeps the frame honest about
  // whether there is an account at all.
  const [account, setAccount] = useState(() => vaultSummary());
  useEffect(() => setAccount(vaultSummary()), [location.pathname, location.key]);

  const tab: Tab =
    location.pathname.startsWith('/send') ? 'send'
    : location.pathname.startsWith('/receive') ? 'receive'
    : location.pathname.startsWith('/lookup') || location.pathname.startsWith('/a/') ? 'lookup'
    : location.pathname.startsWith('/account') || location.pathname.startsWith('/attach') ? 'account'
    : 'money';

  const refresh = () => setAccount(vaultSummary());

  return (
    <div className="y-stage">
      {/* The bar above the desk. Hidden below 430px by the shared stylesheet,
          where the frame becomes the page and a conceit costs screen height on
          the screen that has least of it. */}
      <header className="y-stage__bar">
        <a className="wal-brand" href="/">
          <svg viewBox="0 0 64 64" aria-hidden="true" width="22" height="22">
            <rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F" />
            <path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" strokeWidth="7.2" />
            <path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" strokeWidth="7.2" />
          </svg>
          <span>Yamale</span>
        </a>
        <span className="y-stage__title">Yamale Wallet</span>
        <a className="wal-stageLink" href="/">{t('app.backToSite')}</a>
      </header>

      <div className="y-phone">
        <div className="y-phone__notch" />
        <div className="y-phone__screen">
          <AppBar account={account} onLock={refresh} />

          <main className="y-screen">
            <Routes>
              <Route path="/" element={
                account
                  ? <Money address={account.address} />
                  : <Attach onAttached={() => { refresh(); navigate('/'); }}
                            onWatch={() => navigate('/lookup')} />
              } />
              <Route path="/attach" element={
                <Attach onAttached={() => { refresh(); navigate('/'); }}
                        onWatch={() => navigate('/lookup')} />
              } />
              <Route path="/send" element={<SendScreen onGoHome={() => navigate('/')} />} />
              <Route path="/receive" element={<ReceiveScreen onAttach={() => navigate('/attach')} />} />
              <Route path="/lookup" element={
                <LookupScreen address={null} onOpen={(a) => navigate(`/a/${a}`)}
                              onClose={() => navigate('/lookup')} />
              } />
              <Route path="/a/:address" element={<Watched />} />
              <Route path="/account" element={
                <AccountScreen onDetached={() => { refresh(); navigate('/'); }}
                               onAttach={() => navigate('/attach')} />
              } />
              <Route path="*" element={
                <ScreenTitle lede={t('msg.notAnAddress')}>{t('wal.nothingHere')}</ScreenTitle>
              } />
            </Routes>
          </main>

          <nav className="y-tabbar">
            {([
              ['money', t('wal.tabMoney'), 'wallet'],
              ['send', t('action.send'), 'send'],
              ['receive', t('action.receive'), 'receive'],
              ['lookup', t('nav.lookUp'), 'search'],
              ['account', t('wal.tabAccount'), 'person'],
            ] as const).map(([id, label, icon]) => (
              <button key={id} type="button"
                      className={tab === id ? 'y-tab y-tab--on' : 'y-tab'}
                      aria-current={tab === id ? 'page' : undefined}
                      onClick={() => navigate(PATHS[id])}>
                <TabIcon name={icon} />
                {/* The label shrinks on a 390px screen and must not vanish. An
                    unlabelled glyph is a guess, and a wrong guess here spends
                    money. */}
                <span className="y-tab__label">{label}</span>
              </button>
            ))}
          </nav>
        </div>
      </div>
    </div>
  );
}

function AppBar({ account, onLock }: {
  account: { label: string; address: string } | null;
  onLock: () => void;
}) {
  const unlocked = Boolean(getUnlocked());
  return (
    <header className="y-appbar">
      {/* Through BASE_URL, not a root-absolute path. The app is served under
          /wallet/, so "/mark.svg" asks the site's origin and gets its 404 — and
          a bare "mark.svg" resolves against the current route, which breaks the
          moment somebody is on /a/<address>. */}
      <img className="y-appbar__mark" src={`${import.meta.env.BASE_URL}mark.svg`}
           alt="" width={30} height={30} />
      <div className="wal-appbarWho">
        <div className="y-appbar__eyebrow">{t('wal.appEyebrow')}</div>
        <div className="y-appbar__name">{account ? account.label : t('wal.noAccountShort')}</div>
      </div>
      <div className="y-appbar__right">
        {/* Only shown while a key is actually in memory, because a padlock that
            is always there says nothing. Locking is the everyday act in a
            self-custody wallet: there is no session to sign out of — this
            wallet *is* the key holder — so it drops the key and keeps the
            account. */}
        {account && unlocked && (
          <button type="button" className="wal-lockBtn" onClick={() => { lock(); onLock(); }}
                  title={t('wal.lockNow')} aria-label={t('wal.lockNow')}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor"
                 strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <rect x="4" y="11" width="16" height="9" rx="2" />
              <path d="M8 11V7a4 4 0 0 1 8 0v4" />
            </svg>
          </button>
        )}
      </div>
    </header>
  );
}

function Money({ address }: { address: string }) {
  return (
    <>
      <ScreenTitle>{t('wal.tabMoney')}</ScreenTitle>
      <Holdings address={address} own />
    </>
  );
}

function Watched() {
  const { address } = useParams();
  const navigate = useNavigate();
  return (
    <LookupScreen address={address ?? ''} onOpen={(a) => navigate(`/a/${a}`)}
                  onClose={() => navigate('/lookup')} />
  );
}
