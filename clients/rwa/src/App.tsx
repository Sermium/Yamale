/**
 * Yamale Tokenisation — the shell.
 *
 * A phone on a desk, for the same reason Yamale Pay is one: this is what an
 * investor holds, not what a registrar runs. The frame comes from the shared
 * mobile shell in `clients/shared/yamale.css` — `.y-stage`, `.y-phone`,
 * `.y-appbar`, `.y-screen`, `.y-tabbar` — rather than from a copy of Pay's own
 * classes, so a fix to the shape reaches every surface wearing it.
 *
 * # Three tabs, and why these three
 *
 * The app answers three questions and they are the three tabs:
 *
 *   TOKENISATIONS  what is on offer — public, readable with no account, because
 *                  the moment an investor most needs to check a vehicle is
 *                  before they have anything invested in it.
 *   MY HOLDINGS    what this address owns, what it cost and what it has earned.
 *   ACCOUNT        which address is attached, how, and the appearance and
 *                  language of the app.
 *
 * A vehicle is not a fourth tab. It is a thing you open from the first tab, so
 * it keeps the tab bar (a reader can always leave) and takes over the app bar
 * with a back chevron and the vehicle's name. Its own five sections live in a
 * segmented control under the app bar — see `Segments` in ui.tsx for why they
 * are not in the tab bar.
 *
 * # The sheet root
 *
 * Every signing dialog portals into `#sheet-root`, which is a child of the
 * phone frame rather than of the document body. A confirmation that floats over
 * the whole browser window on a surface shaped like a device reads as a
 * different application; inside the frame it reads as the screen it is.
 */
import { useState } from 'react';
import { HashRouter, NavLink, Route, Routes, useNavigate, useParams } from 'react-router-dom';
import { t, useLocale } from '@yamale/chain';

import { Account as AccountScreen } from './Account.tsx';
import { useAccount } from './account.ts';
import { type Account } from './address.ts';
import { chainNow, useHead } from './data.ts';
import { Holdings } from './Holdings.tsx';
import { Vehicle } from './Vehicle.tsx';
import { Vehicles } from './Vehicles.tsx';
import { descriptorOf } from './vehicle.ts';
import { useVehicle } from './data.ts';

/* ------------------------------------------------------------------ theme */

export type Theme = 'light' | 'dark' | 'system';

export function savedTheme(): Theme {
  try {
    const v = localStorage.getItem('yamale.rwa.theme');
    if (v === 'light' || v === 'dark') return v;
  } catch { /* private mode */ }
  return 'system';
}

export function applyTheme(next: Theme) {
  if (next === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', next);
  try {
    if (next === 'system') localStorage.removeItem('yamale.rwa.theme');
    else localStorage.setItem('yamale.rwa.theme', next);
  } catch { /* private mode */ }
}

/**
 * The three states cycled from the app bar, in one glyph.
 *
 * `system` is a state, not the absence of one: it is the only one that follows
 * the reader's own preference, and a toggle with two positions would strand
 * anybody who had ever pressed it.
 */
function ThemeButton() {
  const [theme, setTheme] = useState<Theme>(savedTheme);

  const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';
  const label = theme === 'dark' ? t('rwa.themeDark')
    : theme === 'light' ? t('rwa.themeLight') : t('rwa.themeSystem');

  return (
    <button type="button" className="appbar__btn" aria-label={`${t('rwa.theme')}: ${label}`}
            onClick={() => { setTheme(next); applyTheme(next); }}>
      <span aria-hidden="true">{theme === 'dark' ? '◐' : theme === 'light' ? '○' : '◑'}</span>
    </button>
  );
}

/* ------------------------------------------------------------- the bottom */

function TabIcon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    // A building with a roof: the asset itself. The same mark the empty state
    // and the site's own tile use, so the app is recognisable from its icon.
    vehicles: 'M3 8 10 3l7 5v9H3z M7.5 17v-5h5v5',
    // A slice taken out of a whole: a shareholding.
    holdings: 'M10 3a7 7 0 107 7h-7V3z M13 3.6A7 7 0 0116.4 7H13V3.6z',
    // A person.
    account: 'M10 10a3 3 0 100-6 3 3 0 000 6zm-6 7v-1c0-2.2 2.7-4 6-4s6 1.8 6 4v1',
  };
  return (
    <svg className="y-tab__icon" viewBox="0 0 20 20" aria-hidden="true"
         fill="none" stroke="currentColor" strokeWidth="1.5"
         strokeLinecap="round" strokeLinejoin="round">
      <path d={paths[name] ?? ''} />
    </svg>
  );
}

/**
 * Icon and label, never icon alone.
 *
 * Three unlabelled glyphs is a guessing game, and on a surface where one of the
 * tabs leads to a sell button the guess costs money. In a five-language
 * interface the word does more work than the picture.
 */
function TabBar() {
  const tabs = [
    { to: '/', end: true, label: t('rwa.navVehicles'), icon: 'vehicles' },
    { to: '/holdings', end: false, label: t('rwa.navHoldings'), icon: 'holdings' },
    { to: '/account', end: false, label: t('rwa.tabAccount'), icon: 'account' },
  ];
  return (
    <nav className="y-tabbar" aria-label={t('rwa.navLabel')}>
      {tabs.map((tab) => (
        <NavLink key={tab.to} to={tab.to} end={tab.end}
                 className={({ isActive }) => `y-tab${isActive ? ' y-tab--on' : ''}`}>
          <TabIcon name={tab.icon} />
          <span className="y-tab__label">{tab.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

/* --------------------------------------------------------------- the top */

/**
 * The bar at the top of the screen.
 *
 * On a list screen it carries the product's name. On a vehicle it carries a
 * back chevron and that vehicle's name, because a phone screen with no title
 * and no way out is how somebody ends up pressing the browser's back button and
 * losing the app.
 */
function AppBar({ back, title, subtitle }: {
  back?: () => void;
  title: string;
  subtitle: string;
}) {
  return (
    <header className="y-appbar">
      {back ? (
        <button type="button" className="appbar__btn appbar__back" onClick={back}
                aria-label={t('rwa.back')}>
          <svg viewBox="0 0 20 20" width="20" height="20" fill="none" stroke="currentColor"
               strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M12 4 6 10l6 6" />
          </svg>
        </button>
      ) : (
        <img className="y-appbar__mark" src={`${import.meta.env.BASE_URL}mark.png`} alt="" width={28} height={28} />
      )}
      <div className="appbar__mid">
        <div className="y-appbar__eyebrow">{subtitle}</div>
        <div className="y-appbar__name appbar__name">{title}</div>
      </div>
      <div className="y-appbar__right">
        <ThemeButton />
      </div>
    </header>
  );
}

/* ------------------------------------------------------------------ routes */

/**
 * One vehicle.
 *
 * The app bar's title is read from the same place the page's is — the asset's
 * uri, which is the only field on it that carries a name. An asset whose uri
 * names nothing falls back to its number rather than to something invented.
 */
function VehicleRoute({ account }: { account: Account }) {
  const { assetId = '' } = useParams();
  const navigate = useNavigate();
  const [record] = useVehicle(assetId);

  const asset = record.loading || !record.outcome.ok ? null : record.outcome.value.asset;
  const name = asset
    ? (descriptorOf(asset.uri).name ?? t('rwa.vehicleN', { id: asset.id }))
    : t('rwa.vehicleN', { id: assetId });

  return (
    <>
      <AppBar back={() => navigate('/')} title={name}
              subtitle={asset?.collectionId || t('rwa.eyebrow')} />
      <Vehicle assetId={assetId} account={account} />
    </>
  );
}

/**
 * A tab's screen.
 *
 * The app bar carries the PRODUCT here, not the screen's own name: the screen
 * says what it is in its own title, and a bar repeating it two lines above is a
 * row of pixels spent saying nothing. On a vehicle the bar earns its space —
 * there it carries the way back and the name of the thing being read.
 */
function Screen({ children }: { children: React.ReactNode }) {
  return (
    <>
      <AppBar title={t('rwa.title')} subtitle={t('rwa.eyebrow')} />
      {children}
    </>
  );
}

export function App() {
  const api = useAccount();
  const head = useHead();
  useLocale();
  const now = chainNow(head);

  return (
    <HashRouter>
      <div className="y-stage">
        <a className="skip" href="#main">{t('rwa.skip')}</a>

        {/* The desk's own bar. Hidden below 430px by the shared shell: on an
            actual phone the frame becomes the page and a caption above a device
            that is not being drawn is just a row of text nobody asked for. */}
        <header className="y-stage__bar">
          <a className="stage__brand" href="/">
            <img src={`${import.meta.env.BASE_URL}mark.png`} alt="" width={22} height={22} />
            <span>Yamale</span>
          </a>
          <span className="y-stage__title">{t('rwa.title')}</span>
          <a className="stage__link" href="/">{t('app.backToSite')}</a>
        </header>

        <div className="y-phone">
          <div className="y-phone__notch" />
          <div className="y-phone__screen" id="main" tabIndex={-1}>
            <Routes>
              <Route path="/" element={
                <Screen><Vehicles now={now.at} head={head} /></Screen>
              } />
              <Route path="/v/:assetId" element={<VehicleRoute account={api.account} />} />
              <Route path="/holdings" element={
                <Screen><Holdings account={api.account} head={head} /></Screen>
              } />
              <Route path="/account" element={
                <Screen><AccountScreen api={api} head={head} /></Screen>
              } />
            </Routes>
            <TabBar />
          </div>

          {/* Where every signing sheet is portalled to. A child of the frame, so
              a sheet covers the device rather than the browser. It is empty
              whenever nothing is being signed, and `:empty` takes it out of the
              way so it cannot swallow a tap. */}
          <div id="sheet-root" />
        </div>
      </div>
    </HashRouter>
  );
}
