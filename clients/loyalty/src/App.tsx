import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import { LanguagePicker, t, useLocale } from '@yamale/chain';

import { AccountScreen } from './account/Account.tsx';
import { SessionProvider } from './account/session.tsx';
import { NotFound } from './ui.tsx';
import { JewelPage } from './verify/JewelPage.tsx';
import { Proof } from './verify/Proof.tsx';
import { Search } from './verify/Search.tsx';

function TabBar() {
  const tabs = [
    { to: '/', end: true, label: t('loyalty.navVerify') },
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
              <Route path="/j/:jeweler/:jewel" element={<JewelPage />} />
              <Route path="/own/:jewel" element={<Proof />} />
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
