/**
 * Information design: the figures a reader came for, the folders they choose
 * between, and concentration drawn rather than listed.
 *
 * These mirror `clients/rwa/src/ui.tsx` in shape and token usage. They are
 * reimplemented here rather than imported across an app boundary, because the
 * two consoles are separate Vite builds with separate dependency trees and a
 * cross-app import couples their release cycles for four small components.
 *
 * What they exist to fix, measured on this surface before the change:
 *
 *   The staking page set its figures at 1.25rem against a 0.78rem label — a 1.6x
 *   ratio, which is not enough for a number to read AS a number. "174,375 YML"
 *   and "securing the chain" carried the same visual weight, so the eye had no
 *   reason to land on the figure first, and a page whose whole purpose is to
 *   report figures had made no claim about which of its words were the ones
 *   that mattered.
 *
 *   Every page was one scrolling column. Recent blocks lived below the activity
 *   feed and only in expert mode, so "show me the blocks" meant switching a
 *   global mode and then scrolling past forty rows of something else.
 */
import type { ReactNode } from 'react';

export type Tone = 'ok' | 'warn' | 'bad' | 'mute';

/**
 * A figure, large and tabular, with its unit attached and its qualifier below.
 *
 * `hint` is where a figure's provenance or its limit goes. Some of these are
 * exact counts the node reports and some are derived from a window of forty
 * transactions, and a band that presented both in the same voice would be
 * claiming a precision the chain does not have.
 */
export function Kpi({
  label,
  value,
  unit,
  hint,
  tone,
}: {
  label: ReactNode;
  value: ReactNode;
  unit?: string;
  hint?: ReactNode;
  tone?: Tone;
}) {
  return (
    <div className={tone ? `kpi kpi--${tone}` : 'kpi'}>
      <p className="kpi__label">{label}</p>
      <p className="kpi__value y-num">
        {value}
        {unit ? <span className="kpi__unit">{unit}</span> : null}
      </p>
      {hint ? <p className="kpi__hint">{hint}</p> : null}
    </div>
  );
}

export function KpiBand({ children }: { children: ReactNode }) {
  return <div className="kpiband">{children}</div>;
}

/**
 * Folders, because one scrolling column is not a document.
 *
 * Tabs rather than an accordion: these are peers and a reader wants one at a
 * time. An accordion invites everything open at once, which is the column again
 * with extra clicks.
 *
 * The count belongs on the tab. "Blocks" tells a reader nothing about whether
 * opening it is worth the click; "Blocks 12" does, and on a page where one
 * folder is routinely empty that is the difference between a folder and a
 * dead end.
 */
export function Folders({
  tabs,
  active,
  onSelect,
  label,
}: {
  tabs: { id: string; label: string; count?: number | null }[];
  active: string;
  onSelect: (id: string) => void;
  label: string;
}) {
  return (
    <div className="folders" role="tablist" aria-label={label}>
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={tab.id === active}
          className={tab.id === active ? 'folder folder--on' : 'folder'}
          onClick={() => onSelect(tab.id)}
        >
          {tab.label}
          {tab.count !== null && tab.count !== undefined ? (
            <span className="folder__count y-num">{tab.count}</span>
          ) : null}
        </button>
      ))}
    </div>
  );
}

/**
 * Concentration, drawn.
 *
 * A validator set is opened to answer one question — how much of this chain is
 * one operator — and a column of percentages answers it slowly. The ring answers
 * it before the reader has read anything, which matters here because the honest
 * answer on devnet-2 is uncomfortable: two validators, one holding about 70%.
 *
 * Everything past the first eight is one "others" arc rather than hundreds of
 * hairlines, because a large validator set is the SUCCESS case and a chart that
 * became unreadable at that point would fail exactly when the argument was won.
 *
 * Stroked arcs on one circle: no library, no canvas, and it inherits the
 * theme's tokens like everything else.
 */
export function Donut({
  slices,
  hole = 0.62,
  label,
}: {
  slices: { label: string; fraction: number; tone: string }[];
  hole?: number;
  label: string;
}) {
  const r = 100;
  const width = r * 2 * (1 - hole);
  const rad = r - width / 2;
  const circumference = 2 * Math.PI * rad;

  let offset = 0;
  return (
    <svg className="donut" viewBox="0 0 200 200" role="img" aria-label={label}>
      <circle cx="100" cy="100" r={rad} fill="none" strokeWidth={width} className="donut__track" />
      {slices.map((s) => {
        const len = Math.max(0, s.fraction) * circumference;
        const el = (
          <circle
            key={s.label}
            cx="100"
            cy="100"
            r={rad}
            fill="none"
            strokeWidth={width}
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

/**
 * The colours a concentration chart may use, in order.
 *
 * Deliberately semantic rather than a rainbow: the largest holder is the one
 * the reader is being warned about, so it takes the warning colour when it is
 * over a third and the neutral one otherwise. Beyond that the arcs only need to
 * be distinguishable from each other, so they step through the surface tokens
 * rather than inventing hues the rest of the product does not have.
 */
export const ARC_TONES = [
  'var(--accent)',
  'var(--brass)',
  'var(--text-muted)',
  'var(--ok)',
  'var(--text-faint)',
  'var(--surface-3)',
];

/** A legend key: the colour, in the same size and shape the ring drew it. */
export function Swatch({ tone }: { tone: string }) {
  return <span className="swatch" style={{ background: tone }} aria-hidden="true" />;
}
