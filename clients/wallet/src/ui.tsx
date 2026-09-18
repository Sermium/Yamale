/**
 * The pieces every screen in this wallet is built from.
 *
 * The shell itself — the desk, the frame, the app bar, the tab bar, the cards
 * and the rows — comes from `clients/shared/yamale.css` (`.y-stage`, `.y-phone`,
 * `.y-appbar`, `.y-screen`, `.y-tabbar`, `.y-cards`, `.y-row`). What lives here
 * is the handful of things a *wallet* needs that a generic mobile shell does
 * not: a figure that cannot break, and a way of saying "we could not read this"
 * that cannot be mistaken for zero.
 */
import type { ReactNode } from 'react';
import { formatAmount, resolveDenom, t } from '@yamale/chain';

/* ------------------------------------------------------------- the figure */

/**
 * How large a figure may be drawn before it stops fitting on one line.
 *
 * A number never breaks across two lines — `100,000,0` above `00` has shipped
 * from this repository twice, and it is not a typographic blemish: it is a
 * balance misread by a factor of a hundred. `white-space: nowrap` alone only
 * moves the failure to an overflow, so the size is chosen from the length
 * instead.
 *
 * The arithmetic, so the next person can check it rather than trust it. IBM
 * Plex Mono advances exactly 0.6em per glyph. The narrowest place a figure is
 * drawn is the balance hero on a 360px phone: 360 − 32 (screen padding) − 32
 * (hero padding) ≈ 290px of room. Each row below keeps `chars × 0.6 × size`
 * under that, with the tightest row (32 characters at 0.95rem) landing at
 * 292px — the point at which the table stops stepping down, because past it
 * the figure is an integer overflow rather than a balance.
 */
function fitRem(chars: number): number {
  if (chars <= 8) return 2.3;
  if (chars <= 11) return 2.0;
  if (chars <= 14) return 1.7;
  if (chars <= 18) return 1.4;
  if (chars <= 23) return 1.15;
  return 0.95;
}

export type AmountSize = 'hero' | 'row' | 'body';

/**
 * An amount of a currency.
 *
 * `amount` of `null` or `undefined` is not zero and is not a dash. It renders
 * the words "could not be read", because a wallet that prints `0` when the node
 * did not answer has told somebody they have nothing, and somebody who believes
 * they have nothing stops looking for it. A dash is no better: on a balance line
 * it reads as a zero with a flourish.
 */
export function Amount({
  amount,
  denom,
  size = 'body',
  withSymbol = true,
  className,
}: {
  amount: string | bigint | null | undefined;
  denom: string;
  size?: AmountSize;
  withSymbol?: boolean;
  className?: string;
}) {
  const info = resolveDenom(denom);

  if (amount === null || amount === undefined) {
    return <span className={`wal-figure wal-figure--none ${className ?? ''}`}>{t('wal.notRead')}</span>;
  }

  /*
   * Every decimal the chain actually holds, not the decimals the currency is
   * quoted in.
   *
   * `formatAmount` defaults to `info.minorUnits` — two places for the naira's
   * kobo, ZERO for the CFA franc, which has no subunit in practice. That is the
   * right default for a price and a catastrophic one here: typing 250,75 into
   * the amount field and reaching a review screen that says "250 XOF" was found
   * on the live chain while building this. The amount that moves was 250.75.
   * A confirmation screen showing a figure that is not the figure being signed
   * is worse than no confirmation screen.
   *
   * Passing the denom's own exponent turns the rounding off. Trailing zeros are
   * still trimmed inside formatAmount, so a whole number of francs still prints
   * as a whole number of francs — the decimal appears only when there is one,
   * which is precisely when hiding it would be a lie.
   */
  const figure = formatAmount(amount, denom, { withSymbol: false, maxDecimals: info.exponent });
  const style = size === 'hero' ? { fontSize: `${fitRem(figure.length)}rem` } : undefined;

  return (
    <span className={`wal-amount wal-amount--${size} ${className ?? ''}`}>
      <span className="wal-figure" style={style}>
        {figure}
      </span>
      {withSymbol && <span className="wal-amount__sym">{info.symbol}</span>}
    </span>
  );
}

/**
 * A fact the interface could not obtain, said in words.
 *
 * Separate from an error: nothing is wrong with the money, the question simply
 * could not be asked. The raw fault stays behind a disclosure because "it did
 * not work" is not something anybody can act on.
 */
export function Unreadable({ what, error }: { what: string; error?: unknown }) {
  const raw = error instanceof Error ? error.message : error ? String(error) : '';
  return (
    <div className="wal-note wal-note--warn">
      <strong>{what}</strong>
      {raw && (
        <details className="wal-raw">
          <summary>{t('sign.whatTheChainSaid')}</summary>
          <pre className="wal-raw__pre y-scroll">{raw}</pre>
        </details>
      )}
    </div>
  );
}

/* -------------------------------------------------------------- furniture */

export function Note({
  tone = 'warn',
  title,
  children,
}: {
  tone?: 'ok' | 'warn' | 'bad' | 'plain';
  title?: string;
  children?: ReactNode;
}) {
  return (
    <div className={`wal-note wal-note--${tone}`}>
      {title && <strong>{title}</strong>}
      {title && children ? ' ' : null}
      {children}
    </div>
  );
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: ReactNode;
  children: ReactNode;
}) {
  return (
    <label className="wal-field">
      <span className="wal-field__label">{label}</span>
      {children}
      {hint && <span className="wal-field__hint">{hint}</span>}
    </label>
  );
}

/** The heading every screen opens with, plus an optional sentence under it. */
export function ScreenTitle({ children, lede }: { children: ReactNode; lede?: ReactNode }) {
  return (
    <>
      <h1 className="y-screen__title">{children}</h1>
      {lede && <p className="wal-lede">{lede}</p>}
    </>
  );
}

/** A back control, on screens that are a step inside a tab rather than a tab. */
export function Back({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <button type="button" className="wal-back" onClick={onClick}>
      <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor"
           strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        {/* Drawn as a chevron pointing at the inline start, and flipped with the
            document direction — in Arabic "back" points the other way. */}
        <path d="M12 4 L6 10 L12 16" />
      </svg>
      {children}
    </button>
  );
}

/* ----------------------------------------------------------------- icons */

/**
 * The tab glyphs, and the reason each one is the shape it is.
 *
 * Icon *and* label, always. An unlabelled glyph is a guess, and a wrong guess
 * on these screens spends money — the same reasoning that put words under the
 * tabs in Yamale Pay.
 */
export function TabIcon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    // A note and coins: money you hold.
    wallet: 'M3 7h14a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2V7zm0 0l2-3h10l2 3M16 12h1',
    // An arrow leaving.
    send: 'M4 12h12m0 0l-4-4m4 4l-4 4',
    // An arrow arriving, over a tray.
    receive: 'M10 3v9m0 0l-3.5-3.5M10 12l3.5-3.5M3.5 14v1.5a1.5 1.5 0 001.5 1.5h10a1.5 1.5 0 001.5-1.5V14',
    // A lens: looking something up.
    search: 'M8.5 14a5.5 5.5 0 100-11 5.5 5.5 0 000 11zm4-1.5L17 17',
    // A figure: the account that is yours.
    person: 'M10 9.5a3 3 0 100-6 3 3 0 000 6zm0 2c-3.2 0-5.5 1.9-5.5 4V17h11v-1.5c0-2.1-2.3-4-5.5-4z',
  };
  return (
    <svg className="y-tab__icon" viewBox="0 0 20 20" aria-hidden="true"
         fill="none" stroke="currentColor" strokeWidth="1.5"
         strokeLinecap="round" strokeLinejoin="round">
      <path d={paths[name] ?? ''} />
    </svg>
  );
}
