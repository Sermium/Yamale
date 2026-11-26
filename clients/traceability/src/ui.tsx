import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { Icon } from './icons.tsx';
import { formatDate, t } from '@yamale/chain';

export type Failure = { ok: false; reason: 'unreachable'; detail: string };

/** A route id is a positive integer, and nothing else is worth a fetch. */
export const isId = (s: string): boolean => /^[1-9][0-9]*$/.test(s);

/**
 * Load once per change of deps; the second element reloads on demand. A loader
 * that throws does not leave the page on "Loading…": for outcome-shaped results
 * it becomes an 'unreachable' failure, otherwise `onError` supplies the value.
 */
export function useLoad<T extends { ok: boolean }>(load: () => Promise<T>, deps: unknown[]): [T | Failure | null, () => void];
export function useLoad<T>(load: () => Promise<T>, deps: unknown[], onError: (detail: string) => T): [T | null, () => void];
export function useLoad<T>(load: () => Promise<T>, deps: unknown[], onError?: (detail: string) => T): [T | Failure | null, () => void] {
  const [value, setValue] = useState<T | Failure | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setValue(null);
    load().then((v) => { if (live) setValue(v); }).catch((e: unknown) => {
      if (!live) return;
      const detail = e instanceof Error ? e.message : String(e);
      setValue(onError ? onError(detail) : { ok: false, reason: 'unreachable', detail });
    });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return [value, useCallback(() => setTick((n) => n + 1), [])];
}

export function day(unix: number | string): string {
  return formatDate(new Date(Number(unix) * 1000));
}

type Theme = 'light' | 'dark' | 'system';
const THEME_KEY = 'yamale.traceability.theme';

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

/** The theme on screen now: the saved choice, or else what the device asks for. */
function shownTheme(): 'light' | 'dark' {
  const saved = savedTheme();
  if (saved !== 'system') return saved;
  try { return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'; } catch { return 'light'; }
}

/**
 * Two states, light and dark, from whatever is on screen. Until it is pressed
 * the app follows the device; pressing it fixes the other one. A third
 * "follow the device" step looked identical to one of the other two and read
 * as a broken switch.
 */
export function ThemeButton() {
  const [theme, setTheme] = useState<'light' | 'dark'>(shownTheme);
  const next = theme === 'dark' ? 'light' : 'dark';
  return (
    <button type="button" className="appbar__btn"
            aria-label={t(next === 'light' ? 'traceability.themeToLight' : 'traceability.themeToDark')}
            onClick={() => { setTheme(next); applyTheme(next); }}>
      <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
    </button>
  );
}

export function AppBar({ title, eyebrow, back }: { title: string; eyebrow: string; back?: boolean }) {
  const navigate = useNavigate();
  return (
    <header className="y-appbar">
      {back ? (
        <button type="button" className="appbar__btn appbar__back" onClick={() => navigate(-1)} aria-label={t('traceability.back')}>
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
  const key = outcome.reason === 'not-found' ? 'traceability.problemNotFound'
    : outcome.reason === 'offline' ? 'traceability.problemOffline'
    : outcome.reason === 'refused' ? 'traceability.problemRefused' : 'traceability.problemUnreachable';
  return (
    <div className="note note--warn" role="status">
      <p>{t(key)}</p>
      <details className="advanced"><summary>{t('traceability.advanced')}</summary><pre>{outcome.detail}</pre></details>
    </div>
  );
}

export function Loading() {
  return <p className="loading" role="status">{t('traceability.loading')}</p>;
}

export function NotFound() {
  return (
    <>
      <AppBar title={t('traceability.title')} eyebrow="" back />
      <main className="y-screen"><p className="note">{t('traceability.notFoundPage')}</p></main>
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

/** Share a link without our servers in the path: the phone's own share sheet, a copy, or mail. */
export function ShareButtons({ link, title, text }: { link: string; title: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const mail = `mailto:?subject=${encodeURIComponent(title)}&body=${encodeURIComponent(`${text}\n\n${link}`)}`;
  return (
    <div className="actions">
      {typeof navigator !== 'undefined' && 'share' in navigator && (
        <button type="button" className="btn" onClick={() => navigator.share({ title, text, url: link }).catch(() => {})}>
          {t('traceability.share')}
        </button>
      )}
      <button type="button" className="btn" onClick={async () => {
        try { await navigator.clipboard.writeText(link); setCopied(true); } catch { /* no clipboard on plain http */ }
      }}>
        {copied ? t('traceability.copied') : t('traceability.copyLink')}
      </button>
      <a className="btn" href={mail}>{t('traceability.sendByEmail')}</a>
    </div>
  );
}
