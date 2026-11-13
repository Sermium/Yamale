import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import { completeJewelReassignment, LanguagePicker, t, useLocale, type JewelView } from '@yamale/chain';

import { AccountScreen, UnlockGate } from './account/Account.tsx';
import { SessionProvider, useSession } from './account/session.tsx';
import { ActionStatus, useAction } from './actions/useAction.tsx';
import { Apply } from './atelier/Apply.tsx';
import { Amend } from './atelier/Amend.tsx';
import { Atelier } from './atelier/Atelier.tsx';
import { AtelierJewel } from './atelier/AtelierJewel.tsx';
import { Profile } from './atelier/Profile.tsx';
import { Redeem } from './atelier/Redeem.tsx';
import { Register } from './atelier/Register.tsx';
import { useRole } from './atelier/role.ts';
import { Staff } from './atelier/Staff.tsx';
import { Claim } from './owner/Claim.tsx';
import { MyJewels } from './owner/MyJewels.tsx';
import { Owned } from './owner/Owned.tsx';
import { NotFound } from './ui.tsx';
import { JewelPage } from './verify/JewelPage.tsx';
import { Proof } from './verify/Proof.tsx';
import { Search } from './verify/Search.tsx';

function TabBar() {
  const session = useSession();
  const [role] = useRole(session.address);
  const tabs = [
    { to: '/', end: true, label: t('loyalty.navVerify') },
    { to: '/mine', end: false, label: t('loyalty.navMine') },
    ...(role.state === 'role' ? [{ to: '/atelier', end: false, label: t('loyalty.navAtelier') }] : []),
    { to: '/account', end: false, label: t('loyalty.navAccount') },
  ];
  return (
    <nav className="y-tabbar" aria-label={t('loyalty.navLabel')}>
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
      <p className="hint">{t('loyalty.reassignmentDue')}</p>
      <UnlockGate>
        <button type="button" className="btn" disabled={action.kind === 'busy'} onClick={async () => {
          if (await run(() => session.submitAsOwner([completeJewelReassignment(session.address!, view.jewel.id)]))) reload();
        }}>
          {t('loyalty.completeReassignment')}
        </button>
        <ActionStatus state={action} done={t('loyalty.reassignmentDone')} />
      </UnlockGate>
    </div>
  );
}

export function App() {
  useLocale();
  return (
    <BrowserRouter>
      <SessionProvider>
      <div className="y-stage">
        <a className="skip" href="#main">{t('loyalty.skip')}</a>
        <header className="y-stage__bar">
          <a className="stage__brand" href="/"><img src="/mark.png" alt="" width={22} height={22} /><span>Yamale</span></a>
          <span className="y-stage__title">{t('loyalty.title')}</span>
          <LanguagePicker className="stage__lang" />
        </header>
        <div className="y-phone">
          <div className="y-phone__notch" />
          <div className="y-phone__screen" id="main" tabIndex={-1}>
            <Routes>
              <Route path="/" element={<Search />} />
              <Route path="/j/:jeweler/:jewel" element={
                <JewelPage actions={(view, now, reload) => <CompleteReassignment view={view} now={now} reload={reload} />} />} />
              <Route path="/own/:jewel" element={<Proof />} />
              <Route path="/mine" element={<MyJewels />} />
              <Route path="/mine/:jewel" element={<Owned />} />
              <Route path="/claim" element={<Claim />} />
              <Route path="/atelier" element={<Atelier />} />
              <Route path="/atelier/register" element={<Register />} />
              <Route path="/atelier/staff" element={<Staff />} />
              <Route path="/atelier/redeem" element={<Redeem />} />
              <Route path="/atelier/look" element={<Profile />} />
              <Route path="/apply" element={<Apply />} />
              <Route path="/atelier/jewel/:jewel" element={<AtelierJewel />} />
              <Route path="/atelier/jewel/:jewel/amend" element={<Amend />} />
              <Route path="/account" element={<AccountScreen />} />
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
