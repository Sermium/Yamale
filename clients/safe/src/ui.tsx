/**
 * Information design for the Safe: the figures a treasurer came for, and the
 * folders they choose between.
 *
 * These mirror `clients/rwa/src/ui.tsx` in shape and in token usage, and are
 * reimplemented rather than imported across an app boundary: the consoles are
 * separate Vite builds with separate dependency trees, and a cross-app import
 * would couple their release cycles for two small components.
 *
 * What they fix here is narrower than on the explorer, because this console
 * already had tabs and already led with available-over-committed. What it did
 * not have was a figure set at the size of a figure — `.figure` was step-2
 * against a 0.7rem label — and it had no answer at all, above the fold, to
 * "how much of what this chain holds is already promised away".
 */
import type { ReactNode } from 'react';

export type Tone = 'ok' | 'warn' | 'bad' | 'lock' | 'mute';

/**
 * A figure, large and tabular, with its qualifier underneath.
 *
 * `lock` is this console's own tone and it is the point of the product: a
 * committed balance is not a warning and not an error, it is money under the
 * chain's custody. It takes the brass the custody figure already uses, so the
 * tile and the diagram are visibly talking about the same thing.
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
      <p className="kpi__value">
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
 * A figure carrying one line per currency.
 *
 * Usually one line; a treasury spanning three shows three, rather than a total
 * that added nairas to francs. That total is the number this whole console
 * exists not to show, so the KPI has to be able to refuse to produce it.
 */
export function Stack({ lines }: { lines: ReactNode[] }) {
  return (
    <span className="kpi__stack">
      {lines.map((line, i) => (
        <span key={i}>{line}</span>
      ))}
    </span>
  );
}

/**
 * Folders, because one scrolling column is not a document.
 *
 * The count belongs on the tab. "Commitments" says nothing about whether
 * opening it is worth a click; "Commitments 3" does, and on this console a
 * folder being empty is itself the answer to the question somebody opened it
 * with.
 *
 * A count of `null` means not yet known, and renders as nothing rather than as
 * a zero — "no commitments" and "we have not read the commitments" are
 * different facts and a treasurer acts differently on each.
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
            <span className="folder__count">{tab.count}</span>
          ) : null}
        </button>
      ))}
    </div>
  );
}
