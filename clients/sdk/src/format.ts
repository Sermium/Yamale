/**
 * Turning chain identifiers and timestamps into things people can read.
 *
 * The rule throughout: never make somebody parse a machine value to answer a
 * human question. "Two minutes ago" answers "is this recent?"; a block height
 * does not.
 */

/**
 * Shortens an address for display: `yml12urcg…qaqm`.
 *
 * Addresses are shown truncated everywhere by default, always with the full
 * value available on copy or hover. Reading 43 bech32 characters to confirm you
 * are looking at the right account is not something anyone can actually do
 * reliably, so an interface that demands it is not offering real verification.
 */
export function truncateAddress(address: string, lead = 10, tail = 4): string {
  if (!address) return '';
  if (address.length <= lead + tail + 1) return address;
  return `${address.slice(0, lead)}…${address.slice(-tail)}`;
}

/** Shortens a transaction or block hash: `0FCDFC24…3553B`. */
export function truncateHash(hash: string, lead = 8, tail = 5): string {
  if (!hash) return '';
  if (hash.length <= lead + tail + 1) return hash;
  return `${hash.slice(0, lead)}…${address_tail(hash, tail)}`;
}

function address_tail(value: string, tail: number): string {
  return value.slice(-tail);
}

/**
 * A stable, readable label for an address when no name is known.
 *
 * Derived from the address itself so the same account always reads the same
 * way, which is what lets somebody recognise a counterparty across screens
 * without memorising bech32.
 */
export function addressLabel(address: string): string {
  return truncateAddress(address);
}

/** Deterministic hue for an address, for identicon-style colour coding. */
export function addressHue(address: string): number {
  let hash = 0;
  for (let i = 0; i < address.length; i++) {
    hash = (hash * 31 + address.charCodeAt(i)) % 360;
  }
  return hash;
}

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const MONTH = 30 * DAY;
const YEAR = 365 * DAY;

/**
 * Renders an elapsed time the way somebody would say it.
 *
 * Deliberately coarse: "3 days ago" is more useful than "3 days, 4 hours and 12
 * minutes ago" for the question this usually answers, which is whether
 * something is current.
 */
export function timeAgo(timestamp: string | Date, now: Date = new Date()): string {
  const then = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  if (Number.isNaN(then.getTime())) return 'unknown';

  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000);

  if (seconds < 0) return 'just now';
  if (seconds < 10) return 'just now';
  if (seconds < MINUTE) return `${seconds} seconds ago`;
  if (seconds < 2 * MINUTE) return 'a minute ago';
  if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)} minutes ago`;
  if (seconds < 2 * HOUR) return 'an hour ago';
  if (seconds < DAY) return `${Math.floor(seconds / HOUR)} hours ago`;
  if (seconds < 2 * DAY) return 'yesterday';
  if (seconds < WEEK) return `${Math.floor(seconds / DAY)} days ago`;
  if (seconds < MONTH) return `${Math.floor(seconds / WEEK)} weeks ago`;
  if (seconds < YEAR) return `${Math.floor(seconds / MONTH)} months ago`;
  return `${Math.floor(seconds / YEAR)} years ago`;
}

/** Renders a future instant as remaining time: "12 days left". */
export function timeUntil(timestamp: string | Date, now: Date = new Date()): string {
  const then = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  if (Number.isNaN(then.getTime())) return 'unknown';

  const seconds = Math.floor((then.getTime() - now.getTime()) / 1000);
  if (seconds <= 0) return 'now';
  if (seconds < MINUTE) return 'less than a minute left';
  if (seconds < HOUR) return `${plural(Math.floor(seconds / MINUTE), 'minute')} left`;
  if (seconds < DAY) return `${plural(Math.floor(seconds / HOUR), 'hour')} left`;
  if (seconds < MONTH) return `${plural(Math.floor(seconds / DAY), 'day')} left`;
  return `${plural(Math.floor(seconds / MONTH), 'month')} left`;
}

/** A duration in seconds as a phrase: "21 days", "2 hours". */
export function formatDuration(seconds: number): string {
  if (seconds < MINUTE) return `${seconds} seconds`;
  if (seconds < HOUR) return plural(Math.round(seconds / MINUTE), 'minute');
  if (seconds < DAY) return plural(Math.round(seconds / HOUR), 'hour');
  if (seconds < YEAR) return plural(Math.round(seconds / DAY), 'day');
  return plural(Math.round(seconds / YEAR), 'year');
}

function plural(count: number, unit: string): string {
  return `${count} ${unit}${count === 1 ? '' : 's'}`;
}

/** Full timestamp for the expert view and tooltips. */
export function formatTimestamp(timestamp: string | Date, locale = 'en-GB'): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  if (Number.isNaN(date.getTime())) return 'unknown';
  return date.toLocaleString(locale, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  });
}

/** Thousands-separated integer, for gas figures and block heights. */
export function formatNumber(value: number | string, locale = 'en-US'): string {
  const n = typeof value === 'string' ? Number(value) : value;
  if (!Number.isFinite(n)) return String(value);
  return n.toLocaleString(locale);
}

/**
 * A decimal with a bounded number of places: `0.4213456` → `0.4213`.
 *
 * Pinned to one locale like everything else here. Letting the browser choose
 * would render `0,4213` on a French machine beside an `en-US` amount of
 * `98,165.35` in the same view, where the comma means opposite things two lines
 * apart — and `0,4213` misread as four thousand is not a rounding error.
 */
export function formatDecimal(value: number, maxDecimals = 6, locale = 'en-US'): string {
  if (!Number.isFinite(value)) return '—';
  return value.toLocaleString(locale, { maximumFractionDigits: maxDecimals });
}

/** A percentage from a 0–1 ratio: `0.0834` → `8.34%`. */
export function formatPercent(ratio: number, decimals = 2): string {
  if (!Number.isFinite(ratio)) return '—';
  return `${(ratio * 100).toFixed(decimals)}%`;
}
