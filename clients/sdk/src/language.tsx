import { useSyncExternalStore } from 'react';
import { LANGUAGES, getLocale, setLocale, onLocaleChange, type Locale } from './i18n';
import { AVAILABLE } from './catalogues';

/**
 * The language picker, shared by every app so the control is in the same place
 * everywhere.
 *
 * Languages are listed by their endonym -- Kiswahili, not Swahili. Somebody
 * hunting for their own language is scanning for the word they use for it, and
 * a list written in English is a list they have to translate before they can
 * read it.
 *
 * Changing language re-renders rather than reloading. setLocale writes `lang`
 * and `dir` straight onto the document, so the mirroring is not something React
 * has to be talked into — and a reload would take the session down with it,
 * because the unlocked signer lives in memory and is never persisted.
 */
/** The active language, re-rendering whatever uses it when it changes. */
export function useLocale(): Locale {
  return useSyncExternalStore(onLocaleChange, getLocale, getLocale);
}

export function LanguagePicker({ className }: { className?: string }) {
  const current = useLocale();
  const shipped = LANGUAGES.filter(l => AVAILABLE.includes(l.code));

  return (
    <select
      className={className}
      value={current}
      aria-label="Language"
      onChange={e => {
        setLocale(e.target.value as Locale);
      }}
    >
      {shipped.map(l => (
        <option key={l.code} value={l.code}>
          {l.endonym}
        </option>
      ))}
    </select>
  );
}
