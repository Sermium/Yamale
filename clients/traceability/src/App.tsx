import { BrowserRouter, NavLink, Route, Routes, Link, useLocation } from 'react-router-dom';
import { completeJewelReassignment, LanguagePicker, t, useLocale, type JewelView } from '@yamale/chain';

import { AccountScreen, UnlockGate } from './account/Account.tsx';
import { MoveIn } from './account/MoveIn.tsx';
import { SessionProvider, useSession } from './account/session.tsx';
import { ActionStatus, useAction } from './actions/useAction.tsx';
import { Apply } from './atelier/apply/Apply.tsx';
import { Amend } from './atelier/piece/Amend.tsx';
import { Atelier } from './atelier/Atelier.tsx';
import { Catalogue } from './atelier/catalogue/Catalogue.tsx';
import { DesignReview } from './atelier/catalogue/DesignReview.tsx';
import { Pieces } from './atelier/catalogue/Pieces.tsx';
import { AtelierJewel } from './atelier/piece/AtelierJewel.tsx';
import { Profile } from './atelier/shop/Profile.tsx';
import { Redeem } from './atelier/benefits/Redeem.tsx';
import { Register } from './atelier/register/Register.tsx';
import { useRole } from './atelier/shared/role.ts';
import { Staff } from './atelier/shop/Staff.tsx';
import { Claim } from './owner/Claim.tsx';
import { CardPage } from './card/CardPage.tsx';
import { KeptCardsPage } from './card/KeptCards.tsx';
import { Directory } from './directory/Directory.tsx';
import { Shop } from './directory/Shop.tsx';
import { MyJewels } from './owner/MyJewels.tsx';
import { Owned } from './owner/Owned.tsx';
import { NotFound } from './ui.tsx';
import { AboutImport } from './verify/AboutImport.tsx';
import { JewelPage } from './verify/JewelPage.tsx';
import { Proof } from './verify/Proof.tsx';
import { Search } from './verify/Search.tsx';
import { Market } from './market/Market.tsx';

function TabBar() {
  const session = useSession();
  const [role] = useRole(session.address);
  const tabs = [
    { to: '/', end: true, label: t('traceability.navVerify') },
    { to: '/market', end: false, label: t('traceability.navMarket') },
    { to: '/jewelers', end: false, label: t('traceability.navJewelers') },
    { to: '/mine', end: false, label: t('traceability.navMine') },
    ...(role.state === 'role' ? [{ to: '/atelier', end: false, label: t('traceability.navAtelier') }] : []),
    { to: '/account', end: false, label: t('traceability.navAccount') },
  ];
  return (
    <nav className="y-tabbar" aria-label={t('traceability.navLabel')}>
      {tabs.map((tab) => (
        <NavLink key={tab.to} to={tab.to} end={tab.end}
                 className={({ isActive }) => `y-tab${isActive ? ' y-tab--on' : ''}`}>
          <span className="y-tab__label">{tab.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

/** The spec's "anyone, once the window has passed": shown only to a signed-in visitor. */
function CompleteReassignment({ view, now, reload }: { view: JewelView; now: number; reload: () => void }) {
  const session = useSession();
  const [action, run] = useAction();
  const r = view.reassignment;
  if (!r || !session.address || now < Number(r.windowEnds)) return null;
  return (
    <div className="stack">
      <p className="hint">{t('traceability.reassignmentDue')}</p>
      <UnlockGate>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([completeJewelReassignment(session.address!, view.jewel.id)]))) reload();
        }}>
          {t('traceability.completeReassignment')}
        </button>
        <ActionStatus state={action} done={t('traceability.reassignmentDone')} />
      </UnlockGate>
    </div>
  );
}

/**
 * An account that was never saved to the sign-in services lives in this
 * browser alone: clearing it, or signing out, loses it for good, and no other
 * device can sign in to it. Until 2026-11-24 saving could fail without a word,
 * so accounts in daily use were found unsaved. Said on every page, not only on
 * the account page nobody opens once things work.
 */
function OnlyOnThisDevice() {
  const session = useSession();
  const { pathname } = useLocation();
  const a = session.account;
  if (!a || a.protection === 'enrolled') return null;
  if (session.saving === 'busy') return <p className="banner devicebanner" role="status">{t('traceability.savingAccount')}</p>;
  if (pathname === '/account') return null;
  const why = session.saving === 'idle' ? null : t(`traceability.saveOutcome.${session.saving}`);
  return (
    <div className="banner banner--warn devicebanner" role="alert">
      <span>{session.unlocked ? t('traceability.onlyOnThisDevice') : t('traceability.onlyOnThisDeviceLocked')}</span>
      {why && <span>{why}</span>}
      {session.unlocked && <Link className="btn" to="/account">{t('traceability.saveAccountNow')}</Link>}
    </div>
  );
}

export function App() {
  useLocale();
  return (
    <BrowserRouter>
      <SessionProvider>
      <div className="y-stage">
        <a className="skip" href="#main">{t('traceability.skip')}</a>
        <header className="y-stage__bar">
          <a className="stage__brand" href="/"><img src="/mark.png" alt="" width={22} height={22} /><span>Yamale</span></a>
          <span className="y-stage__title">{t('traceability.title')}</span>
          <LanguagePicker className="stage__lang" />
        </header>
        <div className="y-phone">
          <div className="y-phone__notch" />
          <div className="y-phone__screen" id="main" tabIndex={-1}>
            <OnlyOnThisDevice />
            <Routes>
              <Route path="/" element={<Search />} />
              <Route path="/j/:jeweler/:jewel" element={
                <JewelPage actions={(view, now, reload) => <CompleteReassignment view={view} now={now} reload={reload} />} />} />
              <Route path="/own/:jewel" element={<Proof />} />
              <Route path="/mine" element={<MyJewels />} />
              <Route path="/mine/:jewel" element={<Owned />} />
              <Route path="/claim" element={<Claim />} />
              <Route path="/card" element={<CardPage />} />
              <Route path="/market" element={<Market />} />
              <Route path="/jewelers" element={<Directory />} />
              <Route path="/jewelers/:jeweler" element={<Shop />} />
              <Route path="/atelier" element={<Atelier />} />
              <Route path="/atelier/register" element={<Register />} />
              <Route path="/atelier/staff" element={<Staff />} />
              <Route path="/atelier/cards" element={<KeptCardsPage />} />
              <Route path="/atelier/redeem" element={<Redeem />} />
              <Route path="/atelier/look" element={<Profile />} />
              <Route path="/atelier/catalogue" element={<Catalogue />} />
              <Route path="/atelier/catalogue/design/:id" element={<DesignReview />} />
              <Route path="/atelier/catalogue/pieces" element={<Pieces />} />
              <Route path="/about-import" element={<AboutImport />} />
              <Route path="/apply" element={<Apply />} />
              <Route path="/atelier/jewel/:jewel" element={<AtelierJewel />} />
              <Route path="/atelier/jewel/:jewel/amend" element={<Amend />} />
              <Route path="/account" element={<AccountScreen />} />
              <Route path="/move-in" element={<MoveIn />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
            <TabBar />
          </div>
        </div>
      </div>
      </SessionProvider>
    </BrowserRouter>
  );
}
