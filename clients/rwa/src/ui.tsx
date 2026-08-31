/**
 * The small pieces every screen is built out of.
 *
 * Each of these exists because the alternative was the same three lines of JSX
 * written slightly differently in four places — and on this surface the
 * variations are not cosmetic. An amount rendered without its symbol, an
 * address printed in full, a state signalled by colour alone: each is a defect
 * that would have shipped once per screen instead of once.
 */
import { useState, type ReactNode } from 'react';
import {
  EMPTY_AMOUNT,
  formatAmount,
  resolveDenom,
  t,
  truncateAddress,
} from '@yamale/chain';

/* --------------------------------------------------------------- amounts */

/**
 * Money, in the unit a person reads.
 *
 * Base units in, display units out, at the display boundary and nowhere else.
 * An absent amount is an en dash rather than a zero, because "this account has
 * never held any of this" and "this account holds none of it" are different
 * facts and only one of them is a balance.
 */
export function Amount({ amount, denom, className = '' }: {
  amount: string | null | undefined;
  denom: string;
  className?: string;
}) {
  if (amount === null || amount === undefined || amount === '') {
    return <span className={`y-num amount amount--empty ${className}`}>{EMPTY_AMOUNT}</span>;
  }
  return (
    <span className={`y-num amount ${className}`} title={`${amount} ${denom}`}>
      {formatAmount(amount, denom)}
    </span>
  );
}

/** The unit itself, for a column header or a legend. */
export function Symbol({ denom }: { denom: string }) {
  return <span className="y-mono">{resolveDenom(denom).symbol}</span>;
}

/**
 * A percentage, to one place, with the sign of smallness preserved.
 *
 * A holding of a thousandth of a per cent must not render as 0.0%. Somebody
 * owns that, and a screen that rounds it away has told them they own nothing.
 */
export function Percent({ value, places = 2 }: { value: number | null; places?: number }) {
  if (value === null || !Number.isFinite(value)) {
    return <span className="y-num amount--empty">{EMPTY_AMOUNT}</span>;
  }
  const shown = value > 0 && value < 10 ** -places
    ? `<${(10 ** -places).toFixed(places)}%`
    : `${value.toFixed(places).replace(/\.?0+$/, '')}%`;
  return <span className="y-num">{shown}</span>;
}

/* ------------------------------------------------------------ identifiers */

/**
 * An account, named where possible and never printed in full by default.
 *
 * Nobody reads a bech32 address character by character to confirm they are in
 * the right place. What is shown is short enough to sit on a line; clicking
 * copies the whole of it; the reveal is for auditing rather than for reading.
 */
export function Address({ value, label }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const [open, setOpen] = useState(false);

  if (!value) return <span className="amount--empty">{EMPTY_AMOUNT}</span>;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // Refused on insecure origins and in some in-app browsers. Selecting the
      // text is the fallback that always works.
      const el = document.createElement('textarea');
      el.value = value;
      document.body.appendChild(el);
      el.select();
      document.execCommand('copy');
      el.remove();
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  };

  return (
    <span className="addr">
      <button type="button" className="addr__face y-mono" onClick={copy}
              aria-label={`${t('app.copy')}: ${value}`}>
        {open ? value : (label ?? truncateAddress(value))}
        <span className="addr__ico" aria-hidden="true">{copied ? '✓' : '⧉'}</span>
      </button>
      <button type="button" className="addr__reveal" aria-expanded={open}
              onClick={() => setOpen((v) => !v)}>
        {open ? t('app.hide') : t('app.reveal')}
      </button>
    </span>
  );
}

/* ------------------------------------------------------------------ state */

export type Tone = 'ok' | 'warn' | 'bad' | 'mute' | 'brass';

/**
 * A state, carried in the word first and the colour second.
 *
 * Never colour alone: the label is the signal, and the wash behind it is
 * reinforcement. A monochrome screenshot of any of these still reads
 * correctly, which is also what makes them legible to a reader who cannot
 * distinguish the hues.
 */
export function Chip({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`y-chip y-chip--${tone}`}>{children}</span>;
}

/** A sentence that needs to be noticed, with its tone in its wording. */
export function Note({ tone, title, children }: {
  tone: Tone;
  title?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <p className={`note note--${tone}`} role={tone === 'bad' ? 'alert' : undefined}>
      {title && <strong className="note__title">{title}</strong>}
      {title && children ? ' ' : null}
      {children}
    </p>
  );
}

/**
 * A bar with a stated denominator.
 *
 * Used for the challenge window and for attestation progress, which are the two
 * places on this surface where a reader has to judge "how much of this is
 * left". A bar with no number beside it is decoration; the number is the
 * content and the bar is what makes it read at a glance.
 */
export function Meter({ value, of, tone = 'ok', label }: {
  value: number;
  of: number;
  tone?: Tone;
  label: string;
}) {
  const pct = of > 0 ? Math.max(0, Math.min(100, (value / of) * 100)) : 0;
  return (
    <div className="meter" role="img" aria-label={label}>
      <div className={`meter__track meter__track--${tone}`}>
        <div className={`meter__fill meter__fill--${tone}`} style={{ inlineSize: `${pct}%` }} />
      </div>
    </div>
  );
}

/* ------------------------------------------------------------- structure */

export function Panel({ eyebrow, title, children, className = '' }: {
  eyebrow?: ReactNode;
  title?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel ${className}`}>
      {eyebrow && <p className="y-eyebrow panel__eyebrow">{eyebrow}</p>}
      {title && <h3 className="panel__title">{title}</h3>}
      {children}
    </section>
  );
}

/** A label and a value on one row, which is most of what these screens are. */
export function Field({ label, children, wide = false }: {
  label: ReactNode;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <div className={`field ${wide ? 'field--wide' : ''}`}>
      <dt className="y-label field__label">{label}</dt>
      <dd className="field__value">{children}</dd>
    </div>
  );
}

export function Fields({ children }: { children: ReactNode }) {
  return <dl className="fields">{children}</dl>;
}

/**
 * The raw values, one click deeper.
 *
 * Every screen here hides something — base units behind a formatted figure, a
 * full address behind a truncation, an enum behind a sentence. Experts lose
 * trust fast when they cannot audit what a screen is claiming, and on a surface
 * sold to supervisors that trust is the product.
 */
export function Raw({ children }: { children: ReactNode }) {
  return (
    <details className="raw">
      <summary className="raw__summary">{t('rwa.raw')}</summary>
      <div className="raw__body y-scroll">{children}</div>
    </details>
  );
}

/**
 * Nothing to show, said properly.
 *
 * This app will spend most of its life here, so the empty state is not a
 * placeholder: it says what is absent, who would have to act for it to stop
 * being absent, and at what block the claim was read. An empty page that
 * renders perfectly tells the reader nothing about whether it is empty or
 * broken.
 */
export function Empty({ title, children, at }: {
  title: ReactNode;
  children?: ReactNode;
  at?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty__mark" aria-hidden="true">
        <svg viewBox="0 0 48 48" width="48" height="48" fill="none" stroke="currentColor"
             strokeWidth="1.25" strokeLinejoin="round">
          <path d="M8 18 24 8l16 10v22H8z" />
          <path d="M18 40V26h12v14" />
        </svg>
      </div>
      <h2 className="empty__title">{title}</h2>
      {children && <div className="empty__body">{children}</div>}
      {at && <p className="empty__at y-mono">{at}</p>}
    </div>
  );
}

/** The chain did not answer. Never the same shape as an empty result. */
export function Unreachable({ detail, onRetry }: { detail?: string; onRetry?: () => void }) {
  return (
    <div className="empty empty--bad">
      <h2 className="empty__title">{t('rwa.unreachable')}</h2>
      <p className="empty__body">{t('rwa.unreachableWhy')}</p>
      {onRetry && (
        <button type="button" className="btn btn--ghost" onClick={onRetry}>{t('rwa.retry')}</button>
      )}
      {detail && <p className="empty__at y-mono">{detail}</p>}
    </div>
  );
}

export function Loading({ what }: { what: string }) {
  return <p className="loading" role="status">{what}</p>;
}
