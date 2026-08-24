import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import {
  VIEW_MODE_STORAGE_KEY,
  ViewModeContext,
  initialViewMode,
  type ViewMode,
} from './chain.ts';
import { HomePage } from './pages/Home.tsx';
import { SearchPage } from './pages/Search.tsx';
import { StatusStrip } from './StatusStrip.tsx';
import { TransactionPage } from './pages/Transaction.tsx';
import { AccountPage } from './pages/Account.tsx';
import { BlockPage } from './pages/Block.tsx';
import { NotFoundPage } from './pages/NotFound.tsx';
import { StakingPage } from './pages/Staking.tsx';
import { GovernancePage } from './pages/Governance.tsx';
import { TradePage } from './pages/Trade.tsx';
import { PricesPage } from './pages/Prices.tsx';
import { CapabilitiesPage } from './pages/Capabilities.tsx';
import { EnforcementPage } from './pages/Enforcement.tsx';
import { WalletProviderScope } from './wallet.tsx';
import { AccountBadge } from './AccountBadge.tsx';
import { CopyAnnouncer } from './Identifier.tsx';
import { LanguagePicker, t} from '@yamale/chain';

export function App() {
  const location = useLocation();
  const [mode, setModeState] = useState<ViewMode>(() => initialViewMode(location.search));

  const setMode = useCallback((next: ViewMode) => {
    setModeState(next);
    try {
      localStorage.setItem(VIEW_MODE_STORAGE_KEY, next);
    } catch {
      // Not being able to remember the preference is survivable.
    }
  }, []);

  useEffect(() => {
    document.title = mode === 'expert' ? 'Yamale Explorer — detailed' : 'Yamale Explorer';
  }, [mode]);

  return (
    <ViewModeContext.Provider value={{ mode, setMode }}>
      <WalletProviderScope>
      <div className="shell">
        <Masthead />
        {/* Liveness is furniture, not a footnote: it sits above every route
            rather than in the corner of one card on the front page. "Is the
            chain healthy" is the first question this audience asks, and it
            stops being the first question the moment somebody navigates. */}
        <StatusStrip />
        {/* One live region for every copy affordance on the page. */}
        <CopyAnnouncer />
        <main className="page">
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/tx/:hash" element={<TransactionPage />} />
            <Route path="/account/:address" element={<AccountPage />} />
            <Route path="/block/:height" element={<BlockPage />} />
            <Route path="/staking" element={<StakingPage />} />
            <Route path="/governance" element={<GovernancePage />} />
            <Route path="/trade" element={<TradePage />} />
            <Route path="/prices" element={<PricesPage />} />
            <Route path="/enforcement" element={<EnforcementPage />} />
            <Route path="/capabilities" element={<CapabilitiesPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </main>
      </div>
      </WalletProviderScope>
    </ViewModeContext.Provider>
  );
}

function Masthead() {
  const navigate = useNavigate();
  const [term, setTerm] = useState('');

  /**
   * Search accepts whatever somebody has in their clipboard and works out what
   * it is. Asking a person to pick "address / transaction / block" from a
   * dropdown before searching is asking them to already know the answer.
   *
   * Everything goes to one route, which classifies it and either redirects or
   * shows what it found. The header used to do the classifying, which is why it
   * could only ever accept the three kinds decidable without a round trip — and
   * why a Yamale user ID, the identifier a citizen actually holds, came back
   * "that does not look like an account, a transaction or a block number".
   */
  function onSubmit(event: FormEvent) {
    event.preventDefault();
    const value = term.trim();
    if (!value) return;
    navigate(`/search?q=${encodeURIComponent(value)}`);
  }

  return (
    <header className="masthead">
      <div className="masthead__inner">
        {/* The mark goes home to the network, not to this app's own front page.
            Each app already has its own first nav item for that; what nothing
            offered was a way back out to the site that ties them together.
            Built from location.hostname so it is right over LAN, Tailscale or a
            domain without a rebuild. */}
        <a href="/" className="wordmark">
          <svg className="brand__mark" viewBox="0 0 64 64" aria-hidden="true"><rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F"/><path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" strokeWidth="7.2"/><path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" strokeWidth="7.2"/></svg>
          Yamale
        </a>

        <nav className="mainnav" aria-label="Sections">
          <NavLink to="/" end>{t('exp.activity')}</NavLink>
          <NavLink to="/staking">{t('exp.staking')}</NavLink>
          <NavLink to="/governance">{t('exp.governance')}</NavLink>
          <NavLink to="/trade">{t('exp.trade')}</NavLink>
          <NavLink to="/prices">{t('exp.prices')}</NavLink>
          <NavLink to="/enforcement">{t('exp.enforcement')}</NavLink>
          <NavLink to="/capabilities">{t('exp.capabilities')}</NavLink>
        </nav>

        <form className="search" onSubmit={onSubmit} role="search">
          <label htmlFor="search-input" className="visually-hidden">
            {t('xp.search.label')}
          </label>
          <input
            id="search-input"
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            placeholder={t('xp.search.placeholder')}
            spellCheck={false}
            autoComplete="off"
          />
        </form>

        {/* One control, not two. The account and the level of detail are both
            "yours", so they share a panel rather than competing with the
            navigation as separate header widgets. */}
        <LanguagePicker className="lang" />
        <AccountBadge />
      </div>

    </header>
  );
}
