/**
 * Yamale Herd — the shell.
 *
 * A phone, from the shared mobile shell in `clients/shared/yamale.css` —
 * `.y-stage`, `.y-phone`, `.y-appbar`, `.y-screen`, `.y-tabbar`, `.y-cards`,
 * `.y-card`, `.y-row`. Nothing here re-invents one of those, and Yamale Pay is
 * untouched.
 *
 * # Three tabs, where the brief suggested four
 *
 * IDENTIFY, MY HERD, SYNC. The fourth screen in the brief — ANIMAL — is not a
 * tab and deliberately so: a tab is a place the application always is, and the
 * animal screen is a place you arrive at by identifying one or by picking one
 * out of the herd. A tab for it would have to render something when no animal
 * is open, and the only honest thing to render there is "no animal open", which
 * is a quarter of the bottom bar spent on a screen that says nothing. It keeps
 * the tab bar visible — a reader can always leave — and takes over the app bar
 * with a back chevron and that animal's identifier.
 *
 * IDENTIFY is first and is what the application opens on, including on a cold
 * start with nothing downloaded. Somebody is standing in front of a beast.
 *
 * # The device and the operator come from another agent
 *
 * `device.ts` and `identity.ts` are being written separately; `ports.ts`
 * declares the shapes and `NO_DEVICE` / `NO_IDENTITY` are the absences until
 * they land. Every screen renders those absences rather than pretending — a
 * stubbed tag reader would put a fabricated identifier on a screen whose entire
 * purpose is to refuse one.
 */
import { useCallback, useMemo, useState } from 'react';
import { HashRouter, NavLink, Route, Routes, useNavigate, useParams } from 'react-router-dom';
import { t, useLocale } from '@yamale/chain';

import { AnimalScreen } from './Animal.tsx';
import { useDownload, useLocalHerd, useOutbox, useStore } from './data.ts';
import { Identify } from './Identify.tsx';
import { MyHerd } from './MyHerd.tsx';
import { Register } from './Register.tsx';
import type { Known } from './register.ts';
import { NO_IDENTITY } from './ports.ts';
import { makeKit } from './kit.ts';
import { toMuzzleEnrolled } from './store.ts';
import { newEntry } from './outbox.ts';
import { Sync } from './Sync.tsx';

/* ------------------------------------------------------------------ theme */

export type Theme = 'light' | 'dark' | 'system';

export function savedTheme(): Theme {
  try {
    const v = localStorage.getItem('yamale.herd.theme');
    if (v === 'light' || v === 'dark') return v;
  } catch { /* private mode */ }
  return 'system';
}

export function applyTheme(next: Theme) {
  if (next === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', next);
  try {
    if (next === 'system') localStorage.removeItem('yamale.herd.theme');
    else localStorage.setItem('yamale.herd.theme', next);
  } catch { /* private mode */ }
}

/**
 * Three states cycled from the app bar, in one glyph.
 *
 * `system` is a state and not the absence of one. It matters more here than on
 * a desk surface: this application is read in direct sun at noon and in a byre
 * at dusk on the same day, and the device's own automatic switch is the only
 * thing that tracks that.
 */
function ThemeButton() {
  const [theme, setTheme] = useState<Theme>(savedTheme);
  const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';
  const label = theme === 'dark' ? t('herd.themeDark')
    : theme === 'light' ? t('herd.themeLight') : t('herd.themeSystem');

  return (
    <button type="button" className="appbar__btn"
            aria-label={`${t('herd.theme')}: ${label}`}
            onClick={() => { setTheme(next); applyTheme(next); }}>
      <span aria-hidden="true">{theme === 'dark' ? '◐' : theme === 'light' ? '○' : '◑'}</span>
    </button>
  );
}

/**
 * What this device knows, for the registration duplicate check.
 *
 * Built from the OFFLINE replica, and that is the whole reason the check exists
 * on the device at all. The chain enforces identifier uniqueness too, and it
 * cannot be asked anything in a pen with no signal — which is where
 * registration happens.
 *
 * Every field here was briefly wired to an empty collection while the screen
 * was being built, which typechecked, rendered, and made the entire check
 * inert: `check` would have returned `ready` for an animal the replica was
 * holding a record of. Worth saying because nothing about an empty Map looks
 * wrong at a call site.
 */
function knownFrom(local: ReturnType<typeof useLocalHerd>[0]): Known {
  return {
    byNationalId: new Map(
      local.animals.filter((a) => a.nationalId).map((a) => [a.nationalId, a.id]),
    ),
    byTag: new Map(local.animals.filter((a) => a.tag).map((a) => [a.tag, a.id])),

    // References already spent on THIS device, from its own outbox. A
    // registration that has not reached the chain is still a registration, and
    // an agent who lost signal after the first one must not be able to spend
    // the same form again on the second.
    usedReferences: new Set(
      local.outbox
        .filter((e) => e.detail?.registration === true)
        .map((e) => String(e.detail?.reference ?? ''))
        .filter((r) => r !== ''),
    ),

    // Muzzle and face enrolments only. toMuzzleEnrolled returns null for an
    // iris, so an iris enrolment cannot arrive at a matcher that would read its
    // descriptor and find nothing there.
    enrolments: local.enrolments
      .map(toMuzzleEnrolled)
      .filter((e): e is NonNullable<typeof e> => e !== null),
  };
}

/* ------------------------------------------------------------- the bottom */

function TabIcon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    // A viewfinder over a mark: reading an animal.
    identify: 'M3 7V4h3 M14 4h3v3 M17 13v3h-3 M6 16H3v-3 M10 7.5a2.5 2.5 0 100 5 2.5 2.5 0 000-5',
    // Three beasts, which is what a herd looks like from a distance.
    herd: 'M4 14a2.5 2.5 0 015 0 M4 14v2 M9 14v2 M11 11a2.5 2.5 0 015 0 M11 11v5 M16 11v5 M5 9l1.5-3h3L11 9',
    // Two arrows in a loop: what has gone out and what has come back.
    sync: 'M4 8a6 6 0 019.8-3.4 M16 12a6 6 0 01-9.8 3.4 M4 4v4h4 M16 16v-4h-4',
    // A form with a plus: this is the tab that CREATES something, and it is
    // the only one that does. Reusing the viewfinder would have said "another
    // way to look at an animal", which is the opposite of what it is.
    register: 'M5 3h7l3 3v11a1 1 0 01-1 1H5a1 1 0 01-1-1V4a1 1 0 011-1z M12 3v3h3 M9.5 9v5 M7 11.5h5',
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
 * Three unlabelled glyphs is a guessing game, and this application is used by
 * people who were handed the tablet last week. In five languages the word does
 * more work than the picture.
 */
function TabBar({ pending }: { pending: number }) {
  const tabs = [
    { to: '/', end: true, label: t('herd.navIdentify'), icon: 'identify', badge: 0 },
    { to: '/register', end: false, label: t('herd.navRegister'), icon: 'register', badge: 0 },
    { to: '/herd', end: false, label: t('herd.navHerd'), icon: 'herd', badge: 0 },
    { to: '/sync', end: false, label: t('herd.navSync'), icon: 'sync', badge: pending },
  ];
  return (
    <nav className="y-tabbar" aria-label={t('herd.navLabel')}>
      {tabs.map((tab) => (
        <NavLink key={tab.to} to={tab.to} end={tab.end}
                 className={({ isActive }) => `y-tab${isActive ? ' y-tab--on' : ''}`}>
          <span className="tab__mark">
            <TabIcon name={tab.icon} />
            {/* A count, not a dot. "Seven things have not reached the chain" is
                actionable; a dot is an anxiety. */}
            {tab.badge > 0 ? <span className="tab__badge num">{tab.badge}</span> : null}
          </span>
          <span className="y-tab__label">{tab.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

/* ---------------------------------------------------------------- the top */

function AppBar({ back, title, subtitle }: {
  back?: () => void;
  title: string;
  subtitle: string;
}) {
  return (
    <header className="y-appbar">
      {back ? (
        <button type="button" className="appbar__btn appbar__back" onClick={back}
                aria-label={t('herd.back')}>
          <svg viewBox="0 0 20 20" width="20" height="20" fill="none" stroke="currentColor"
               strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M12 4 6 10l6 6" />
          </svg>
        </button>
      ) : (
        <img className="y-appbar__mark" src="mark.svg" alt="" width={28} height={28} />
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

function AnimalRoute({ local }: { local: ReturnType<typeof useLocalHerd>[0] }) {
  const { animalId = '' } = useParams();
  const navigate = useNavigate();
  const known = local.index?.byId.get(animalId) ?? null;

  return (
    <>
      <AppBar back={() => navigate(-1)}
              title={known?.nationalId || t('herd.animalN', { id: animalId })}
              subtitle={known?.name || t('herd.eyebrow')} />
      <AnimalScreen local={local} />
    </>
  );
}

function Screen({ children }: { children: React.ReactNode }) {
  return (
    <>
      <AppBar title={t('herd.title')} subtitle={t('herd.eyebrow')} />
      {children}
    </>
  );
}

export function App() {
  useLocale();
  const store = useStore();
  const [local, reload] = useLocalHerd(store);
  const [download, startDownload] = useDownload(store, reload);
  const queue = useOutbox(store, local.outbox, reload);

  // The real field kit, built once for the life of the application.
  //
  // `device.ts` has landed and `kit.ts` joins it to the ports these screens
  // ask in. What it reports is still mostly absence, and deliberately so: the
  // IriShield's USB protocol and the stick reader's GATT service have not been
  // confirmed by their vendors, so those sources are not constructed at all and
  // the capabilities say which details are missing. The screens already render
  // an absence properly, which is why this is a one-line change rather than a
  // rewrite.
  //
  // `identity` is still NO_IDENTITY, and that one is not a gap in this
  // application: x/herd has no enrolment message to sign yet, so there is
  // nothing for a signer to do. outbox.ts holds captures rather than queueing
  // them for exactly that reason.
  const device = useMemo(() => makeKit(), []);
  const identity = NO_IDENTITY;
  const operator = identity.operator();

  const pending = local.outbox.filter((e) => e.state !== 'sent').length;
  const goToSync = useCallback(() => { window.location.hash = '#/sync'; }, []);

  return (
    <HashRouter>
      <div className="y-stage">
        <a className="skip" href="#main">{t('herd.skip')}</a>

        <header className="y-stage__bar">
          <a className="stage__brand" href="/">
            <svg viewBox="0 0 64 64" width="22" height="22" aria-hidden="true">
              <rect x="4" y="4" width="56" height="56" rx="7" fill="#12253F" />
              <path d="M17 17 L32 32 L47 17" fill="none" stroke="#FFFFFF" strokeWidth="7.2" />
              <path d="M32 32 L32 49.5" fill="none" stroke="#A87B3C" strokeWidth="7.2" />
            </svg>
            <span>Yamale</span>
          </a>
          <span className="y-stage__title">{t('herd.title')}</span>
          <a className="stage__link" href="/">{t('app.backToSite')}</a>
        </header>

        <div className="y-phone">
          <div className="y-phone__notch" />
          <div className="y-phone__screen" id="main" tabIndex={-1}>
            <Routes>
              <Route path="/" element={
                <Screen>
                  <Identify local={local} device={device} operator={operator} queue={queue} />
                </Screen>
              } />
              {/* Registration is its own route rather than a mode on
                  Identify. Identify answers a question; this asserts something
                  the register did not have, and a person should know which one
                  they are doing before they start. */}
              <Route path="/register" element={
                <Screen>
                  <Register
                    device={device}
                    operator={operator}
                    herdId={local.meta?.herdId ?? ''}
                    known={knownFrom(local)}
                    onRegistered={(r) => {
                      if (!operator || !local.meta) return;
                      void queue([
                        // The muzzle, when one was taken, as its own capture
                        // tied to the same reference. It cannot be an enrolment
                        // yet: an enrolment is keyed to an animal id and the id
                        // does not exist until an office creates the record.
                        ...(r.muzzle
                          ? [
                              newEntry({
                                kind: 'capture',
                                animalId: null,
                                herdId: r.herdId,
                                observedAt: r.muzzle.capturedAt,
                                deviceId: device.id(),
                                agent: operator.address,
                                position: null,
                                detail: {
                                  modality: 'muzzle',
                                  quality: r.muzzle.quality,
                                  reference: r.reference,
                                  descriptor: r.muzzle.descriptor,
                                },
                              }),
                            ]
                          : []),
                        newEntry({
                          kind: 'registration',
                          // No animal id: the record does not exist yet, and
                          // the chain assigns the id when a registry office
                          // creates it. Filing this against a guess would be
                          // inventing the very record it exists to ask for.
                          animalId: null,
                          herdId: r.herdId,
                          observedAt: Date.now(),
                          deviceId: device.id(),
                          agent: operator.address,
                          detail: { ...r, registration: true },
                        }),
                      ]);
                    }}
                  />
                </Screen>
              } />
              <Route path="/herd" element={
                <Screen><MyHerd local={local} onDownload={goToSync} /></Screen>
              } />
              <Route path="/sync" element={
                <Screen>
                  <Sync local={local} operator={operator} identity={identity}
                        download={download} startDownload={startDownload}
                        queue={queue} reload={reload} />
                </Screen>
              } />
              <Route path="/a/:animalId" element={<AnimalRoute local={local} />} />
            </Routes>
            <TabBar pending={pending} />
          </div>

          {/* Where any confirming sheet portals to: a child of the frame, so it
              covers the device rather than the browser. */}
          <div id="sheet-root" />
        </div>
      </div>
    </HashRouter>
  );
}
