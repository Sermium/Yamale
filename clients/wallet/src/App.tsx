import { useEffect, useState } from 'react';
import { NavLink, Route, Routes, useLocation, useParams } from 'react-router-dom';

import { vaultSummary } from './vault.ts';
import { AccountBadge } from './AccountBadge.tsx';
import { t } from '@yamale/chain';

import { AccountPage } from './pages/Account.tsx';
import { ConnectPage } from './pages/Connect.tsx';
import { CreatePage } from './pages/Create.tsx';
import { FaucetPage } from './pages/Faucet.tsx';
import { ImportPage } from './pages/Import.tsx';
import { SendPage } from './pages/Send.tsx';
import { WatchPage } from './pages/Watch.tsx';

/**
 * Yamale Wallet — two things, kept apart on purpose.
 *
 * **Watching** an address needs no key and carries no risk, so it is the
 * default and works from a link. **Creating** one produces a recovery phrase
 * that must never leave the page, so it is a deliberate act on its own screen
 * with its own warnings.
 *
 * Blending them — the usual "connect or create" landing page — makes the
 * dangerous action one click away from the harmless one, and puts a phrase on
 * screen for people who came to check a balance.
 */
export function App() {
  return (
    <div className="app">
      <header className="masthead">
        <div className="wrap masthead__inner">
          {/* Home is the network's site, not this app's front page — "Look up"
              in the nav already goes there. Built from location.hostname so it
              is right over LAN, Tailscale or a domain with no rebuild. */}
          <a href="/" className="brand">
            <svg className="brand__mark" viewBox="0 0 64 64" aria-hidden="true"><rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F"/><path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" strokeWidth="7.2"/><path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" strokeWidth="7.2"/></svg>
            Yamale <span>Wallet</span>
          </a>
          <Nav />
          <AccountBadge />
        </div>
      </header>

      <main className="wrap">
        <Routes>
          <Route path="/" element={<WatchPage />} />
          <Route path="/create" element={<CreatePage />} />
          <Route path="/import" element={<ImportPage />} />
          <Route path="/send" element={<SendPage />} />
          <Route path="/faucet" element={<FaucetPage />} />
          {/* The approval window other applications open. Not in the nav: it is
              opened by a caller, never navigated to. */}
          <Route path="/connect" element={<ConnectPage />} />
          <Route path="/a/:address" element={<AccountRoute />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      {/* Kept reachable but out of the main menu: loading a different account
          replaces the one on this device, so it belongs where somebody goes
          looking for it rather than where they can hit it by accident. */}
      <footer className="wrap foot">
        <p>
{t('watch.keysNote')}
        </p>
        <FooterAccountLinks />
      </footer>
    </div>
  );
}

/**
 * The menu, which changes with whether this device holds an account.
 *
 * Before there is one, the only useful actions are getting one — so Create and
 * Load lead. After there is one, those two become rare (Load *replaces* the
 * account, which is destructive) and the daily actions are seeing your balance
 * and topping it up, so the menu becomes those.
 *
 * Showing all five at once was the previous state: it put a destructive action
 * permanently beside an everyday one, and made "Watch" a second link to the
 * page the wordmark already goes to.
 */
function Nav() {
  const [account, setAccount] = useState(() => vaultSummary());

  // The vault is written by another page in this same tab, so a storage event
  // never fires; re-reading on navigation is what keeps the menu honest.
  const location = useLocation();
  useEffect(() => setAccount(vaultSummary()), [location.pathname]);

  return (
    <nav className="nav">
      {account ? (
        <>
          <NavLink to="/send">{t('nav.send')}</NavLink>
          <NavLink to={`/a/${account.address}`}>{t('nav.myAccount')}</NavLink>
        </>
      ) : (
        <>
          <NavLink to="/create">{t('nav.create')}</NavLink>
          <NavLink to="/import">{t('nav.load')}</NavLink>
        </>
      )}
      {/* Always present. The faucet is how a tester gets anything to look at,
          and it is needed *most* before there is an account to fund — hiding it
          until one exists had it disappear at exactly the wrong moment. */}
      <NavLink to="/faucet">{t('nav.getFunds')}</NavLink>
      <NavLink to="/" end>{t('nav.lookUp')}</NavLink>
    </nav>
  );
}

/**
 * Shown only when an account already exists. With none, these would duplicate
 * the menu and read as nonsense — "another" and "a different one" than what?
 */
function FooterAccountLinks() {
  const location = useLocation();
  const [account, setAccount] = useState(() => vaultSummary());
  useEffect(() => setAccount(vaultSummary()), [location.pathname]);

  if (!account) return null;
  return (
    <p className="small muted">
      <NavLink to="/create">Create another account</NavLink> ·{' '}
      <NavLink to="/import">Load a different one</NavLink>
    </p>
  );
}

function AccountRoute() {
  const { address } = useParams();
  return <AccountPage address={address ?? ''} />;
}

function NotFound() {
  return (
    <section className="card">
      <h1>Nothing here</h1>
      <p className="muted">{t('msg.notAnAddress')}</p>
    </section>
  );
}

/** Shared by the watch box on two screens. */
export function AddressForm({ onSubmit }: { onSubmit: (address: string) => void }) {
  const [value, setValue] = useState('');
  const trimmed = value.trim();
  // Checked before navigating rather than after: sending somebody to a page
  // that then says "no such account" wastes a round trip to tell them what the
  // prefix already said.
  const looksRight = /^yml1[0-9a-z]{38,}$/.test(trimmed);

  return (
    <form
      className="watch"
      onSubmit={(e) => {
        e.preventDefault();
        if (looksRight) onSubmit(trimmed);
      }}
    >
      <label className="field">
        <span>{t('watch.address')}</span>
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="yml1…"
          spellCheck={false}
          autoComplete="off"
        />
      </label>
      {trimmed !== '' && !looksRight && (
        <p className="notice notice--bad">
          Yamale addresses start with <code>yml1</code> and are 39 characters or more.
        </p>
      )}
      <button type="submit" disabled={!looksRight}>
        {t('watch.show')}
      </button>
    </form>
  );
}
