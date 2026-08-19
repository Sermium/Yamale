import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import {
  VIEW_MODE_STORAGE_KEY,
  ViewModeContext,
  classifySearch,
  initialViewMode,
  useViewMode,
  type ViewMode,
} from './chain.ts';
import { HomePage } from './pages/Home.tsx';
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
        <main className="page">
          <Routes>
            <Route path="/" element={<HomePage />} />
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
  const { mode, setMode } = useViewMode();
  const navigate = useNavigate();
  const [term, setTerm] = useState('');
  const [problem, setProblem] = useState<string | null>(null);

  /**
   * Search accepts whatever somebody has in their clipboard and works out what
   * it is. Asking a person to pick "address / transaction / block" from a
   * dropdown before searching is asking them to already know the answer.
   */
  function onSubmit(event: FormEvent) {
    event.preventDefault();
    const value = term.trim();
    if (!value) return;

    switch (classifySearch(value)) {
      case 'address':
        setProblem(null);
        navigate(`/account/${value}`);
        break;
      case 'tx':
        setProblem(null);
        navigate(`/tx/${value.toUpperCase()}`);
        break;
      case 'height':
        setProblem(null);
        navigate(`/block/${value}`);
        break;
      default:
        setProblem('That does not look like an account, a transaction or a block number.');
    }
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
          <label htmlFor="search-input" style={{ position: 'absolute', left: -9999 }}>
            Search by account, transaction or block
          </label>
          <input
            id="search-input"
            value={term}
            onChange={(e) => {
              setTerm(e.target.value);
              if (problem) setProblem(null);
            }}
            placeholder="Search an account, transaction or block number"
            spellCheck={false}
            autoComplete="off"
            aria-invalid={problem ? true : undefined}
            aria-describedby={problem ? 'search-problem' : undefined}
          />
        </form>

        {/* One control, not two. The account and the level of detail are both
            "yours", so they share a panel rather than competing with the
            navigation as separate header widgets. */}
        <LanguagePicker className="lang" />
        <AccountBadge />
      </div>

      {problem ? (
        <div className="masthead__inner" style={{ paddingTop: 0 }}>
          <p id="search-problem" className="small muted" style={{ margin: 0 }} role="alert">
            {problem}
          </p>
        </div>
      ) : null}
    </header>
  );
}
