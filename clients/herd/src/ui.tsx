/**
 * The pieces every screen is built out of, sized for a glove and sunlight.
 *
 * This is a field instrument, and that changes what a component is for. On a
 * desk a control is a thing you click; here it is a thing you hit, one-handed,
 * while a beast you are holding by a halter moves. So the rules the whole file
 * is written against:
 *
 *   - A PRIMARY ACTION IS 56 PIXELS TALL and sits at the bottom of the screen,
 *     within a thumb's arc. 44 is the accessibility floor for a finger; a gloved
 *     finger on a moving target is not that finger.
 *   - EVERY STATE IS A WORD BEFORE IT IS A COLOUR. Direct sun washes out hue
 *     long before it washes out contrast, and half of these screens are read at
 *     midday outdoors.
 *   - A NUMBER NEVER BREAKS ACROSS TWO LINES. `overflow-wrap: anywhere` has
 *     printed `100,000,0` above `00` in this repository twice. A national
 *     identifier is worse than an amount: `221/682/021029/L` has four break
 *     opportunities the browser will happily take, and the halves look like two
 *     perfectly good identifiers.
 *   - NOTHING RENDERS A FIGURE IT DOES NOT HAVE. An absent value is an en dash
 *     with a reason beside it, never a zero and never an empty bar.
 */
import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { t, truncateAddress, useLocale } from '@yamale/chain';

import type { Head } from './abci.ts';
import { statusTone, type Animal, type Tone } from './herd.ts';

/* ------------------------------------------------------------ identifiers */

export const ABSENT = '–';

/**
 * A national identifier, unbreakable.
 *
 * `221/682/021029/L` is four numbers and a letter joined by solidi, and a
 * browser treats every solidus as a place it may wrap. Split across two lines
 * it reads as two shorter identifiers, both of which look plausible and neither
 * of which exists. `word-break: keep-all` plus `overflow-wrap: normal` is the
 * pair that holds it — `nowrap` is deliberately NOT used, because it would push
 * a long identifier under the edge of a 390px screen rather than letting the
 * container scroll.
 */
export function NationalId({ value, className = '' }: { value: string; className?: string }) {
  if (!value) return <span className="absent">{ABSENT}</span>;
  return <span className={`natid ${className}`}>{value}</span>;
}

/** A transponder code, grouped for reading back against a stick reader. */
export function Tag({ value }: { value: string }) {
  if (!value) return <span className="absent">{ABSENT}</span>;
  // Grouped visually and copied whole: the groups are rendered as separate
  // spans inside one unbreakable box rather than by inserting spaces into the
  // string, so selecting it yields the fifteen digits a query wants.
  const groups = value.replace(/(\d{3})(\d{4})(\d{4})(\d{4})/, '$1 $2 $3 $4');
  return <span className="tagcode" title={value}>{groups}</span>;
}

/**
 * An account, short by default and never a bare bech32 wall.
 *
 * Nobody in a pen reads forty-three characters to confirm they are looking at
 * the right office. The truncation is the reading form; the full value is one
 * tap away for an audit.
 */
export function Address({ value, label }: { value: string; label?: string }) {
  const [open, setOpen] = useState(false);
  if (!value) return <span className="absent">{ABSENT}</span>;
  return (
    <button type="button" className="addr" aria-expanded={open}
            onClick={() => setOpen((v) => !v)}>
      {open ? value : (label ?? truncateAddress(value))}
    </button>
  );
}

/** A hash, shortened, because thirty-two bytes is not a thing anyone reads. */
export function Digest({ hex }: { hex: string }) {
  if (!hex) return <span className="absent">{ABSENT}</span>;
  return <span className="digest" title={hex}>{hex.slice(0, 8)}…{hex.slice(-6)}</span>;
}

/**
 * One animal as a row, which is what a table becomes on a phone.
 *
 * The name is not the heading. A Ladoum herd contains several beasts called
 * Ndiaga and the registry does not enforce uniqueness on a name, so the
 * identifier leads and the name follows it — the reverse of what looks nicer
 * and the same as what a certificate does.
 */
export function AnimalRow({ animal, to, end }: {
  animal: Animal;
  to: string;
  end?: ReactNode;
}) {
  return (
    <Link className="y-row row" to={to}>
      <span className="y-row__main">
        <span className="y-row__title"><NationalId value={animal.nationalId} /></span>
        <span className="y-row__sub">
          {animal.name || t('herd.unnamed')}
          {animal.coat ? ` · ${animal.coat}` : ''}
        </span>
      </span>
      <span className="y-row__end">{end ?? <StatusChip status={animal.status} />}</span>
      <svg className="y-row__chev" viewBox="0 0 20 20" width="16" height="16" aria-hidden="true"
           fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"
           strokeLinejoin="round">
        <path d="M8 4l6 6-6 6" />
      </svg>
    </Link>
  );
}

/* ----------------------------------------------------------------- state */

/**
 * A state, in the word first and the colour second.
 *
 * A monochrome photograph of any of these still reads correctly, which is both
 * an accessibility property and a sunlight one.
 */
export function Chip({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`y-chip y-chip--${tone}`}>{children}</span>;
}

export function StatusChip({ status }: { status: Parameters<typeof statusTone>[0] }) {
  return <Chip tone={statusTone(status)}>{t(`herd.status.${status}`)}</Chip>;
}

/** A sentence that has to be noticed, with its tone in its wording. */
export function Note({ tone, title, children }: {
  tone: Tone;
  title?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className={`note note--${tone}`} role={tone === 'bad' ? 'alert' : undefined}>
      {title ? <strong className="note__title">{title}</strong> : null}
      {children ? <div className="note__body">{children}</div> : null}
    </div>
  );
}

/* --------------------------------------------------------------- actions */

/**
 * The button a thumb finds without looking.
 *
 * `primary` is the one thing this screen is for, and there is at most one per
 * screen. `stop` is not a destructive-action button in the usual sense — it is
 * the one that appears when the application is refusing to name an animal, and
 * it is styled to be noticed rather than to be avoided.
 */
export function Button({ kind = 'plain', wide = false, children, ...rest }: {
  kind?: 'primary' | 'plain' | 'ghost' | 'stop';
  wide?: boolean;
  children: ReactNode;
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button type="button" className={`btn btn--${kind}${wide ? ' btn--wide' : ''}`} {...rest}>
      {children}
    </button>
  );
}

/**
 * The bar at the bottom of a screen that holds its primary action.
 *
 * Pinned above the tab bar rather than scrolling with the content. An operator
 * who has scrolled a candidate list must not have to scroll back to reach the
 * only control that matters, and one hand on a halter means one thumb on the
 * screen.
 */
export function ActionBar({ children }: { children: ReactNode }) {
  return <div className="actionbar">{children}</div>;
}

/* ------------------------------------------------------------- structure */

export function Panel({ title, sub, children }: {
  title?: ReactNode;
  sub?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="panel">
      {title ? <h3 className="panel__title">{title}</h3> : null}
      {sub ? <p className="panel__sub">{sub}</p> : null}
      {children}
    </section>
  );
}

/** A label and a value. Most of what a record screen is. */
export function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="field">
      <dt className="y-label field__label">{label}</dt>
      <dd className="field__value">{children}</dd>
    </div>
  );
}

export function Fields({ children }: { children: ReactNode }) {
  return <dl className="fields">{children}</dl>;
}

/** One tap deeper, for the facts a reader checks after they have decided. */
export function More({ summary, children }: { summary: ReactNode; children: ReactNode }) {
  return (
    <details className="more">
      <summary className="more__summary">{summary}</summary>
      <div className="more__body">{children}</div>
    </details>
  );
}

/* ---------------------------------------------------------- the silences */

export function Loading({ what }: { what: string }) {
  return <p className="loading" role="status">{what}</p>;
}

/**
 * Nothing here, said properly.
 *
 * Says what is absent and who would have to act for it to stop being absent. An
 * empty page that renders perfectly tells a reader nothing about whether it is
 * empty or broken, and that has shipped on this project once already.
 */
export function Empty({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return (
    <div className="empty">
      <h2 className="empty__title">{title}</h2>
      {children ? <div className="empty__body">{children}</div> : null}
    </div>
  );
}

/**
 * The chain did not answer — and whether that is a fault or a Tuesday.
 *
 * Two different panels behind one component, because on a field device these
 * are not the same event. No signal is the expected condition and gets a calm
 * grey panel that points at the local copy. A node that is reachable and broken
 * gets a red one, and an operator who sees red twice a year will look at it.
 */
export function Silence({ reason, detail, onRetry }: {
  reason: 'offline' | 'unreachable' | 'refused';
  detail?: string;
  onRetry?: () => void;
}) {
  const offline = reason === 'offline';
  return (
    <div className={`empty ${offline ? 'empty--quiet' : 'empty--bad'}`}>
      <h2 className="empty__title">
        {offline ? t('herd.offlineTitle') : t('herd.unreachableTitle')}
      </h2>
      <p className="empty__body">
        {offline ? t('herd.offlineBody') : t('herd.unreachableBody')}
      </p>
      {onRetry ? <Button onClick={onRetry}>{t('herd.retry')}</Button> : null}
      {detail ? <p className="empty__at">{detail}</p> : null}
    </div>
  );
}

/* ------------------------------------------------------------ the figures */

/**
 * A quantity a person may act on, with its unit attached.
 *
 * `value` of null renders as an en dash and the caller supplies the reason. A
 * zero is a measurement; a dash is the absence of one, and the two must never
 * look alike on a screen where one of them means "nobody has ever enrolled this
 * animal".
 */
export function Figure({ label, value, unit, hint, tone }: {
  label: ReactNode;
  value: ReactNode | null;
  unit?: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
}) {
  return (
    <div className={`fig${tone ? ` fig--${tone}` : ''}`}>
      <p className="fig__label">{label}</p>
      <p className="fig__value">
        {value === null ? <span className="absent">{ABSENT}</span> : value}
        {unit ? <span className="fig__unit">{unit}</span> : null}
      </p>
      {hint ? <p className="fig__hint">{hint}</p> : null}
    </div>
  );
}

export function Figures({ children }: { children: ReactNode }) {
  return <div className="figs">{children}</div>;
}

/**
 * A distance against the two thresholds it is judged by.
 *
 * Drawn as a scale rather than as a bar filling up, because a distance is not a
 * progress: lower is better, and a bar that grew with the number would read as
 * "more" when it means "worse". The two operating points are marked on the
 * scale so the reader can see which side of them they are on, and `iris.ts`'s
 * own warning that these are "written to be calibrated, not believed" is
 * repeated under it.
 */
export function DistanceScale({ distance, match, inconclusive }: {
  distance: number;
  match: number;
  inconclusive: number;
}) {
  const pct = (v: number) => `${Math.max(0, Math.min(1, v / 0.5)) * 100}%`;
  return (
    <div className="scale" role="img"
         aria-label={t('herd.distanceAria', { d: distance.toFixed(2), m: match.toFixed(2) })}>
      <div className="scale__track">
        <div className="scale__band scale__band--match" style={{ inlineSize: pct(match) }} />
        <div className="scale__band scale__band--maybe"
             style={{ insetInlineStart: pct(match), inlineSize: pct(inconclusive - match) }} />
        <div className="scale__pin" style={{ insetInlineStart: pct(distance) }} />
      </div>
      <div className="scale__legend">
        <span>0.00</span>
        <span className="scale__mark">{match.toFixed(2)}</span>
        <span className="scale__mark">{inconclusive.toFixed(2)}</span>
        <span>0.50</span>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------ the footer */

/**
 * What this screen's claims were read at.
 *
 * Not an ornament. Every claim about an animal is a claim about a moment, and
 * on a device that spends its day offline the moment is frequently three days
 * ago. This prints the chain's head when there is one, and the replica's height
 * when the screen is being served from the copy.
 */
export function ReadAt({ head, replicaHeight, from }: {
  head: Head;
  replicaHeight?: number | null;
  from?: 'chain' | 'replica';
}) {
  const locale = useLocale();
  if (from === 'replica' && replicaHeight) {
    return (
      <p className="readat">
        {t('herd.readFromReplica', { height: replicaHeight.toLocaleString(locale) })}
      </p>
    );
  }
  return (
    <p className="readat">
      {head.known
        ? `${head.chainId} · ${t('herd.block')} ${head.height.toLocaleString(locale)}`
        : t('herd.chainUnknown')}
      {head.known && head.catchingUp ? ` · ${t('herd.catchingUp')}` : null}
    </p>
  );
}
