/**
 * How long is left, in the reader's language, without a catalogue entry per
 * unit per locale.
 *
 * The SDK's `formatDuration` returns English — "21 days" — which is right for a
 * log line and wrong on the one figure this app asks somebody to act inside. A
 * challenge window counted down in English to a francophone or arabophone
 * reader is a deadline written in a language they are not reading the rest of
 * the page in.
 *
 * `Intl.NumberFormat`'s unit style already holds every translation, in every
 * locale the platform ships, including Arabic's plural agreement and its
 * numerals. Twenty catalogue keys would be twenty chances to get one wrong.
 *
 * Rounding is deliberately downward and never to zero. A window with fifty
 * minutes left must not read "1 hour" — somebody planning around that loses
 * ten minutes of a deadline — and one with twenty seconds left must not read
 * "0 minutes", which reads as expired.
 */

export interface Split {
  value: number;
  unit: 'day' | 'hour' | 'minute' | 'second';
}

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** The largest whole unit that still describes this span. */
export function splitDuration(seconds: number): Split {
  const s = Math.max(0, Math.floor(seconds));
  if (s >= DAY) return { value: Math.floor(s / DAY), unit: 'day' };
  if (s >= HOUR) return { value: Math.floor(s / HOUR), unit: 'hour' };
  if (s >= MINUTE) return { value: Math.floor(s / MINUTE), unit: 'minute' };
  return { value: s, unit: 'second' };
}

/**
 * The span as a phrase in `locale`.
 *
 * Falls back to the plain number and the English unit name only if the
 * platform refuses the unit style, which no browser this ships to does — but a
 * thrown formatter inside a render path would take the page with it, and the
 * page is more useful than the phrase.
 */
export function formatDuration(seconds: number, locale: string): string {
  const { value, unit } = splitDuration(seconds);
  try {
    return new Intl.NumberFormat(locale, {
      style: 'unit',
      unit,
      unitDisplay: 'long',
    }).format(value);
  } catch {
    return `${value} ${unit}${value === 1 ? '' : 's'}`;
  }
}

/**
 * A date, written the way the reader writes dates.
 *
 * Long month rather than a numeric one on purpose: 03/04/2026 is two different
 * days depending on which side of an ocean it is read, and every date on this
 * surface is a deadline somebody is planning around.
 */
export function formatDate(at: Date, locale: string): string {
  if (Number.isNaN(at.getTime())) return '';
  try {
    return new Intl.DateTimeFormat(locale, {
      day: 'numeric', month: 'long', year: 'numeric',
      hour: '2-digit', minute: '2-digit', timeZoneName: 'short',
    }).format(at);
  } catch {
    return at.toISOString();
  }
}
