// The judgement calls this page makes about what it read, kept out of the DOM
// so they can be tested.
//
// Almost everything here exists to keep one distinction intact: the difference
// between a fact the chain stated and a fact the page could not obtain. On a
// tour for a finance ministry those are opposite claims, and the natural way to
// write a dashboard collapses them — an unread number and a number that is
// genuinely nought both arrive at the template as falsy, and both render as
// "0". This module makes that shape impossible: a proof is a tagged value, and
// there is no path from a failure to a numeral.

import { NotFound, Unreachable } from './chain.js';

/* --------------------------------------------------------------- outcomes */

/** A proof that was read. `height` is the height it was true at. */
export const proven = (height, rows, note) => ({ state: 'proven', height, rows, note: note ?? '' });

/**
 * A proof that could not be read, and why — in the reader's terms.
 *
 * There are three distinguishable reasons and they mean different things to
 * somebody deciding whether to trust this:
 *
 *   unreachable  the chain is not answering. Says nothing about the mechanism.
 *   denied       the gateway refuses this path to the public. A deployment
 *                choice, not a fault, and worth saying because a reader who
 *                sees a login box will otherwise conclude the data is secret.
 *   absent       the chain answered, and the thing is not there.
 */
export const unread = (reason, detail) => ({ state: 'unread', reason, detail: detail ?? '' });

/**
 * What a thrown error means for the reader.
 *
 * Never returns a number, never returns an empty string. Every branch produces
 * a sentence somebody can act on, because "—" in a proof box is read as "the
 * mechanism does not work" rather than as "the page did not ask".
 */
export function describeFailure(error) {
  if (error instanceof NotFound) {
    return unread('absent', 'The chain answered, and holds no such record.');
  }
  if (error instanceof Unreachable) {
    if (error.status === 401 || error.status === 403) {
      return unread('denied',
        'The public gateway does not publish this path. The record exists on the chain; '
        + 'reading it here needs an operator credential.');
    }
    if (error.status === 'unreachable' || error.status === 503 || error.status === 502) {
      return unread('unreachable', 'Cannot reach the chain.');
    }
    return unread('unreachable', `The node answered ${error.status}.`);
  }
  return unread('unreachable', 'Cannot reach the chain.');
}

/** True when a proof carries a number the page is entitled to state out loud. */
export const isProven = (proof) => proof?.state === 'proven';

/* ---------------------------------------------------------------- numbers */

/**
 * Basis points as a percentage.
 *
 * 6667 is not 66.67% by rounding accident — it is the smallest integer strictly
 * above two thirds, which is the whole design of the seizure threshold. So this
 * trims trailing zeros rather than fixing two decimals, and 6667 reads 66.67%
 * while 8000 reads 80% instead of 80.00%.
 */
export function bps(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  const pct = n / 100;
  return `${Number(pct.toFixed(2))}%`;
}

/** A count with its noun, pluralised. Zero is a number, not a blank. */
export const count = (n, singular, plural) =>
  `${n.toLocaleString('en')} ${n === 1 ? singular : (plural ?? `${singular}s`)}`;

/**
 * A number of blocks, and roughly how long that is at a measured block time.
 *
 * The seconds figure is passed in rather than assumed. This chain's blocks have
 * run at around seven seconds, but a page that hard-codes seven and then meets
 * a chain running at two states a duration that is wrong by a factor of three —
 * and it states it about a freeze that expires by itself, which is precisely
 * the number a supervisor would check.
 */
export function blocksAbout(blocks, secondsPerBlock) {
  const n = Number(blocks);
  if (!Number.isFinite(n) || n <= 0) return null;
  if (!Number.isFinite(secondsPerBlock) || secondsPerBlock <= 0) {
    return `${n.toLocaleString('en')} blocks`;
  }
  return `${n.toLocaleString('en')} blocks (about ${duration(n * secondsPerBlock)})`;
}

/** A rough, spoken duration. Rounded, and labelled "about" by the caller. */
export function duration(seconds) {
  const s = Math.round(Number(seconds));
  if (!Number.isFinite(s) || s < 0) return null;
  if (s < 90) return count(s, 'second');
  const minutes = Math.round(s / 60);
  if (minutes < 90) return count(minutes, 'minute');
  const hours = Math.round(s / 3600);
  if (hours < 48) return count(hours, 'hour');
  const days = Math.round(s / 86400);
  if (days < 60) return count(days, 'day');
  return count(Math.round(days / 30.44), 'month');
}

/**
 * A base-unit amount rendered in its display unit.
 *
 * BigInt throughout and truncation rather than rounding, so a shown figure is
 * never larger than the real one. The denominations here are micro-units
 * (`uyml`, `uxof`), which is six places; a denom this does not recognise is
 * returned unscaled with its own name, because inventing an exponent for an
 * unknown token is how a page shows a million as a unit.
 */
export function amount(base, denom) {
  const raw = String(base ?? '');
  if (!/^\d+$/.test(raw)) return null;
  if (typeof denom === 'string' && /^u[a-z]{3,}$/.test(denom)) {
    const units = BigInt(raw) / 1000000n;
    const fraction = (BigInt(raw) % 1000000n).toString().padStart(6, '0').replace(/0+$/, '');
    const symbol = denom.slice(1).toUpperCase();
    return `${units.toLocaleString('en')}${fraction ? `.${fraction}` : ''} ${symbol}`;
  }
  return `${BigInt(raw).toLocaleString('en')} ${denom ?? ''}`.trim();
}

/* -------------------------------------------------------------- addresses */

/**
 * An account reference, shortened for a line of prose.
 *
 * Both ends are kept. A bech32 address truncated at one end only cannot be
 * compared against another by eye, and comparing two addresses is the single
 * operation a person in the room actually performs on one.
 */
export function elide(address, keep = 10) {
  const a = String(address ?? '');
  if (a.length <= keep * 2 + 1) return a;
  return `${a.slice(0, keep)}…${a.slice(-keep)}`;
}

/* ------------------------------------------------------------------ times */

/** An ISO instant as a date a person reads, in UTC so two readers agree. */
export function when(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString().replace('T', ' ').slice(0, 16) + ' UTC';
}

/** A unix-seconds instant, as the chain stores transfer timestamps. */
export const whenUnix = (seconds) =>
  (Number(seconds) > 0 ? when(new Date(Number(seconds) * 1000).toISOString()) : null);

/**
 * Seconds per block, measured across a real span rather than assumed.
 *
 * Returns null when the span is too short to mean anything: two adjacent blocks
 * differ by whatever the proposer's clock did, and a "block time" computed from
 * them is noise presented to three decimal places.
 */
export function secondsPerBlock(earlier, later) {
  if (!earlier || !later) return null;
  const blocks = later.height - earlier.height;
  if (blocks < 100) return null;
  const ms = new Date(later.at).getTime() - new Date(earlier.at).getTime();
  if (!Number.isFinite(ms) || ms <= 0) return null;
  return ms / 1000 / blocks;
}
