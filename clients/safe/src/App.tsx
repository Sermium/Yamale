import { LanguagePicker, t} from '@yamale/chain';
import { NavLink, Route, Routes, useParams } from 'react-router-dom';

import { OperationsPage } from './pages/Operations.tsx';
import { TreasuriesPage } from './pages/Treasuries.tsx';
import { TreasuryPage } from './pages/Treasury.tsx';

/**
 * Yamale Safe — shared treasuries with approvals, policies and commitments.
 *
 * Built on the same model as Orion Safe, and on the same central claim: funds
 * committed to a beneficiary cannot be spent by a later proposal, **even one
 * that clears the signing threshold**. On this chain that is not a contract
 * convention, it is a keeper invariant — x/treasury holds custody in a module
 * account and refuses a spend against locked balance outright.
 *
 * So the interface leads with the number a treasurer can actually act on —
 * available, not total — and says plainly why the difference exists.
 */
export function App() {
  return (
    <div className="app">
      <header className="masthead">
        <div className="wrap masthead__inner">
          {/* Home is the network's site, not this app's front page — the nav
              already covers that. Built from location.hostname so it is right
              over LAN, Tailscale or a domain with no rebuild. */}
          <a href="/" className="brand">
            <img className="brand__mark" src={`${import.meta.env.BASE_URL}mark.png`} alt="" />
            Yamale <span>Safe</span>
          </a>
          <nav className="nav">
            <NavLink to="/" end>
              Treasuries
            </NavLink>
            <NavLink to="/operations">{t('safe.operations')}</NavLink>
          </nav>
          <LanguagePicker className="lang" />
        </div>
      </header>

      <main className="wrap">
        <Routes>
          <Route path="/" element={<TreasuriesPage />} />
          <Route path="/treasury/:id" element={<TreasuryRoute />} />
          <Route path="/operations" element={<OperationsPage />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="wrap foot">
        <p>
          Every approval, rejection and commitment here is an on-chain event. This interface reads
          them; it does not keep a record of its own.
        </p>
      </footer>
    </div>
  );
}

function TreasuryRoute() {
  const { id } = useParams();
  return <TreasuryPage id={id ?? '0'} />;
}

function NotFound() {
  return (
    <section className="card">
      <h1>{t('safe.notHere')}</h1>
      <p className="muted">{t('msg.noTreasuryThere')}</p>
    </section>
  );
}
