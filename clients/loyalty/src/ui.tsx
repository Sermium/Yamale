import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { formatDate, t } from '@yamale/chain';

/** Load once per change of deps; the second element reloads on demand. */
export function useLoad<T>(load: () => Promise<T>, deps: unknown[]): [T | null, () => void] {
  const [value, setValue] = useState<T | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setValue(null);
    load().then((v) => { if (live) setValue(v); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return [value, useCallback(() => setTick((n) => n + 1), [])];
}

export function day(unix: number | string): string {
  return formatDate(new Date(Number(unix) * 1000));
}

type Theme = 'light' | 'dark' | 'system';
const THEME_KEY = 'yamale.loyalty.theme';

function savedTheme(): Theme {
  try {
    const v = localStorage.getItem(THEME_KEY);
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch { return 'system'; }
}

function applyTheme(theme: Theme) {
  try {
    if (theme === 'system') {
      document.documentElement.removeAttribute('data-theme');
      localStorage.removeItem(THEME_KEY);
    } else {
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem(THEME_KEY, theme);
    }
  } catch { /* private mode: the choice lasts this page only */ }
}

export function ThemeButton() {
  const [theme, setTheme] = useState<Theme>(savedTheme);
  const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'system' : 'dark';
  const label = theme === 'dark' ? t('loyalty.themeDark') : theme === 'light' ? t('loyalty.themeLight') : t('loyalty.themeSystem');
  return (
    <button type="button" className="appbar__btn" aria-label={`${t('loyalty.theme')}: ${label}`}
            onClick={() => { setTheme(next); applyTheme(next); }}>
      <span aria-hidden="true">{theme === 'dark' ? '◐' : theme === 'light' ? '○' : '◑'}</span>
    </button>
  );
}

export function AppBar({ title, eyebrow, back }: { title: string; eyebrow: string; back?: boolean }) {
  const navigate = useNavigate();
  return (
    <header className="y-appbar">
      {back ? (
        <button type="button" className="appbar__btn appbar__back" onClick={() => navigate(-1)} aria-label={t('loyalty.back')}>
          <svg viewBox="0 0 20 20" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8"
               strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 4 6 10l6 6" /></svg>
        </button>
      ) : (
        <img className="y-appbar__mark" src="/mark.png" alt="" width={28} height={28} />
      )}
      <div className="appbar__mid">
        <div className="y-appbar__eyebrow">{eyebrow}</div>
        <div className="y-appbar__name appbar__name">{title}</div>
      </div>
      <div className="y-appbar__right"><ThemeButton /></div>
    </header>
  );
}

export function Problem({ outcome }: { outcome: { reason: string; detail: string } }) {
  const key = outcome.reason === 'not-found' ? 'loyalty.problemNotFound'
    : outcome.reason === 'offline' ? 'loyalty.problemOffline'
    : outcome.reason === 'refused' ? 'loyalty.problemRefused' : 'loyalty.problemUnreachable';
  return (
    <div className="note note--warn" role="status">
      <p>{t(key)}</p>
      <details className="advanced"><summary>{t('loyalty.advanced')}</summary><pre>{outcome.detail}</pre></details>
    </div>
  );
}

export function Loading() {
  return <p className="loading" role="status">{t('loyalty.loading')}</p>;
}

export function NotFound() {
  return (
    <>
      <AppBar title={t('loyalty.title')} eyebrow="" back />
      <main className="y-screen"><p className="note">{t('loyalty.notFoundPage')}</p></main>
    </>
  );
}

export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="block">
      <h2 className="block__title">{title}</h2>
      {children}
    </section>
  );
}
