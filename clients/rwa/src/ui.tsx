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
  useLocale,
} from '@yamale/chain';

import type { Head } from './abci.ts';
import { CHAIN_ID } from './chain.ts';
import { shortDigest } from './covenant.ts';

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

/**
 * A copy button that says it worked.
 *
 * Split out of `Address` because a digest and a shell command need the same
 * behaviour and the same fallback: `navigator.clipboard` is refused on insecure
 * origins and inside several in-app browsers, which this demo is read in.
 */
export function CopyButton({ value, label, className = '' }: {
  value: string;
  label: ReactNode;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
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
    <button type="button" className={`copybtn ${className}`} onClick={copy}
            aria-label={`${t('app.copy')}: ${value}`}>
      {copied ? t('rwa.copied') : label}
      <span className="copybtn__ico" aria-hidden="true">{copied ? '✓' : '⧉'}</span>
    </button>
  );
}

/**
 * A SHA-256 digest: short on the line, whole within reach, and checkable.
 *
 * Three things, and each is there because leaving it out breaks the proof:
 *
 *   - The SHORT form is a label. Sixty-four hex characters set in a table cell
 *     is a wall nobody reads, so what is shown is both ends of it.
 *   - The WHOLE digest is one click away and copyable. A truncated digest
 *     cannot be compared against anything — it is decoration that looks like
 *     evidence, which is worse than showing none.
 *   - The COMMAND is what turns the digest into a check somebody actually
 *     performs. `sha256sum --check` exits non-zero and says so in words when the
 *     file is not the filing; a bare `sha256sum` prints a hash and leaves a
 *     human eye to compare 64 characters, which is the step everybody skips.
 *
 * The digest is the one thing on this page that proves anything at all: it says
 * a document with these contents existed at this block and was published by the
 * party that controls the collection. It proves nothing about whether the
 * accounts are true — see the panel's own note.
 */
export function Digest({ hex, command }: { hex: string; command: string }) {
  if (!hex) return <span className="amount--empty">{EMPTY_AMOUNT}</span>;

  return (
    <div className="digest">
      <div className="digest__line">
        <CopyButton value={hex} label={<span className="y-mono">{shortDigest(hex)}</span>}
                    className="copybtn--mono" />
      </div>
      <details className="digest__more">
        <summary className="digest__summary">{t('rwa.cov.verifyHow')}</summary>
        <div className="digest__body">
          <p className="digest__label y-label">{t('rwa.cov.digestFull')}</p>
          {/* Breaking is correct HERE and only here: this is a hash, not a
              figure, and every character has to be visible to be compared. */}
          <p className="digest__full y-mono">{hex}</p>

          <p className="digest__label y-label">{t('rwa.cov.verifyCmd')}</p>
          <pre className="digest__cmd y-mono">{command}</pre>
          <CopyButton value={command} label={t('rwa.cov.copyCmd')} />
          <p className="digest__note">{t('rwa.cov.verifyNote')}</p>
        </div>
      </details>
    </div>
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

/**
 * One tap deeper.
 *
 * The rule that turned six-column tables into cards was "two or three facts in
 * the row and the rest one tap deeper". This is where the rest goes. It is a
 * native `<details>` rather than a state hook because a disclosure that
 * survives a re-render for free is a disclosure that cannot get out of step
 * with the row it belongs to.
 */
export function Detail({ summary, children }: { summary: ReactNode; children: ReactNode }) {
  return (
    <details className="more">
      <summary>{summary}</summary>
      <div className="more__body">{children}</div>
    </details>
  );
}

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

/**
 * The block this screen's claims were read at.
 *
 * Not decoration and not a footer ornament. Every claim this app makes about a
 * tokenisation — that no sale has been reported, that an authorisation is live,
 * that nothing has been issued — is a claim about a moment, and a claim shown
 * without the block it was read at is one nobody can check or date. The old
 * desktop layout kept this in a page footer. A phone has no page footer, so it
 * goes at the end of every screen, where the claims it dates are.
 */
export function ChainLine({ head, chainId = false, localClock = false }: {
  head: Head;
  chainId?: boolean;
  /** Print the configured chain id as well — for the account screen. */
  localClock?: boolean;
}) {
  const locale = useLocale();
  return (
    <p className="chainline">
      {head.known
        ? `${head.chainId} · ${t('rwa.block')} ${head.height.toLocaleString(locale)}`
        : t('rwa.chainUnknown')}
      {head.known && head.catchingUp && ` · ${t('rwa.catchingUp')}`}
      {/* Which clock a countdown on this screen was measured against. The
          chain's when it is known, the browser's otherwise — and a reader whose
          laptop is four minutes fast is owed the distinction. */}
      {localClock && ` · ${t('rwa.localClock')}`}
      {chainId && <><br />{CHAIN_ID}</>}
    </p>
  );
}

/* ========================================================================== */
/*  Information design                                                        */
/* ========================================================================== */

/**
 * The figures a reader came for, before the prose explaining them.
 *
 * A tokenisation page previously opened with label/value pairs set at the same
 * size as their labels, so nothing on it read as a figure and the numbers that
 * decide whether to buy were indistinguishable from the collection id. A KPI is
 * a number first: large, tabular, with its unit attached and its qualifier
 * underneath.
 *
 * `hint` is where a figure's limit goes. Several of these are exact and one or
 * two are not, and a band that presented all of them in the same voice would be
 * claiming a precision the chain does not have.
 */
export function Kpi({ label, value, unit, hint, tone }: {
  label: string;
  value: ReactNode;
  unit?: string;
  hint?: ReactNode;
  tone?: Tone;
}) {
  return (
    <div className={`kpi${tone ? ` kpi--${tone}` : ''}`}>
      <p className="kpi__label">{label}</p>
      <p className="kpi__value">
        {value}
        {unit ? <span className="kpi__unit">{unit}</span> : null}
      </p>
      {hint ? <p className="kpi__hint">{hint}</p> : null}
    </div>
  );
}

/**
 * Two figures across, or three where the figures are short.
 *
 * Two is the default because on a 358px column a one-up band of six tiles is a
 * screen and a half of scrolling before the content starts. Three is for the
 * market's bid / ask / spread, where each value is a single price and the
 * comparison between them is the reason they are on screen at all.
 */
export function KpiBand({ children, three = false }: { children: ReactNode; three?: boolean }) {
  return <div className={`kpiband${three ? ' kpiband--three' : ''}`}>{children}</div>;
}

/**
 * The sections of one tokenisation, as a segmented control that scrolls.
 *
 * NOT the bottom tab bar. The tab bar is the app — tokenisations, my holdings,
 * my account — and it means the same thing on every screen. A vehicle's
 * sections are sub-navigation inside one object: they are peers of each other,
 * a reader wants exactly one at a time, and there are five of them. Putting
 * five more items in a bar that already has three would have made the bar
 * eight wide on a 390px screen and made "market" look like a place in the app
 * rather than a thing about this vehicle.
 *
 * Scrolls rather than wraps, because a bar that reflows to two rows moves the
 * content under it every time the reader changes section. The strip carries a
 * fade at its inline end so that "there is more" is visible rather than
 * guessed.
 *
 * `role="tablist"` and `aria-selected` are kept from the folders this replaces:
 * the interaction is the same one, only the shape changed.
 */
export function Segments({ tabs, active, onSelect }: {
  tabs: { id: string; label: string; count?: number | null; flag?: boolean }[];
  active: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="sections">
      <div className="segs" role="tablist" aria-label={t('rwa.sections')}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={tab.id === active}
            className={`seg${tab.id === active ? ' seg--on' : ''}`}
            onClick={() => onSelect(tab.id)}
          >
            {tab.label}
            {tab.count !== null && tab.count !== undefined
              ? <span className="seg__count">{tab.count}</span>
              : null}
            {/*
              A section holding something with a deadline says so on its chip. A
              reader who never opens it is exactly the reader who misses the
              assembly, and the label alone gives them no reason to.

              The dot carries a text alternative rather than colour alone, which
              is the same rule Chip follows.
            */}
            {tab.flag ? (
              <span className="seg__flag" role="img" aria-label={t('rwa.cov.tabFlag')}>●</span>
            ) : null}
          </button>
        ))}
      </div>
    </div>
  );
}

/**
 * Concentration, drawn.
 *
 * A cap table is opened to answer one question — how much of this is one person
 * — and a list of twelve percentages answers it slowly. The ring answers it
 * before the reader has read anything.
 *
 * Everything past the first eight holders is one "others" arc rather than
 * hundreds of hairlines, because a thousand holders is the SUCCESS case for
 * this platform and a chart that became unreadable at that point would fail
 * exactly when the argument was won.
 *
 * Drawn as stroked arcs on one circle: no library, no canvas, and it inherits
 * the theme's tokens like everything else.
 */
export function Donut({ slices, hole = 0.62 }: {
  slices: { label: string; fraction: number; tone: string }[];
  hole?: number;
}) {
  const r = 100;
  const width = r * 2 * (1 - hole);
  const rad = r - width / 2;
  const circumference = 2 * Math.PI * rad;

  let offset = 0;
  return (
    <svg className="donut" viewBox="0 0 200 200" role="img"
         aria-label={slices.map((s) => `${s.label} ${(s.fraction * 100).toFixed(1)}%`).join(', ')}>
      <circle cx="100" cy="100" r={rad} fill="none" strokeWidth={width}
              className="donut__track" />
      {slices.map((s) => {
        const len = Math.max(0, s.fraction) * circumference;
        const el = (
          <circle
            key={s.label}
            cx="100" cy="100" r={rad} fill="none" strokeWidth={width}
            stroke={s.tone}
            strokeDasharray={`${len} ${circumference - len}`}
            strokeDashoffset={-offset}
            transform="rotate(-90 100 100)"
          />
        );
        offset += len;
        return el;
      })}
    </svg>
  );
}
