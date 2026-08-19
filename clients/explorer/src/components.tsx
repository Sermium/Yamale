/**
 * Shared pieces used by both explorers.
 *
 * The loading, empty and error states live here as first-class components
 * rather than being improvised per page. An explorer spends a lot of its life
 * in one of those three states — a fresh chain with no transactions, a node
 * that is down, an account nobody has used — and they are usually the screens
 * nobody designs.
 */

import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  addressHue,
  formatCoins,
  timeAgo,
  truncateAddress,
  displayName,
  truncateHash,
  type Coin,
  type DecodedMessage,
} from '@yamale/chain';

import { useQuery } from '@tanstack/react-query';

import { client, useViewMode } from './chain.ts';

export function Card({
  title,
  actions,
  flush,
  children,
}: {
  title?: ReactNode;
  actions?: ReactNode;
  flush?: boolean;
  children: ReactNode;
}) {
  return (
    <section className="card">
      {(title || actions) && (
        <header className="card__head">
          {title ? <h2 className="card__title">{title}</h2> : <span />}
          {actions}
        </header>
      )}
      <div className={flush ? 'card__body card__body--flush' : 'card__body'}>{children}</div>
    </section>
  );
}

export function Stat({ label, value, note }: { label: string; value: ReactNode; note?: ReactNode }) {
  return (
    <div className="stat">
      <div className="stat__label">{label}</div>
      <div className="stat__value">{value}</div>
      {note ? <div className="stat__note">{note}</div> : null}
    </div>
  );
}

/** Copy-on-click identifier. Nobody should have to select 43 characters by hand. */
export function Copyable({ value, display }: { value: string; display?: ReactNode }) {
  return (
    <button
      type="button"
      className="copy mono"
      title={`${value} — click to copy`}
      onClick={() => {
        void navigator.clipboard?.writeText(value);
      }}
    >
      {display ?? value}
    </button>
  );
}

/**
 * An address, shown as a name when one is known and truncated otherwise, with
 * a colour derived from the address so the same account is recognisable at a
 * glance across pages.
 */
export function AddressLink({ address, name }: { address: string; name?: string }) {
  if (!address) return <span className="faint">—</span>;
  const hue = addressHue(address);
  return (
    <Link to={`/account/${address}`} className="inline" style={{ gap: '0.4rem' }}>
      <span
        aria-hidden="true"
        style={{
          width: 14,
          height: 14,
          borderRadius: 4,
          flex: '0 0 auto',
          background: `hsl(${hue} 55% 55%)`,
        }}
      />
      <AccountLabel address={address} known={name} />
    </Link>
  );
}

/**
 * How an account is named, and how much of it is shown.
 *
 * Four sources, in decreasing order of how much the *reader* trusts them:
 * their own address-book name, a generally-known label such as a validator or
 * module account, the chain-assigned user ID, then the raw address.
 *
 * The reader's own name wins over the well-known one deliberately. If somebody
 * has taken the trouble to label an address, that is the word they will
 * recognise, and overriding it with ours would make their own book useless
 * exactly where it matters.
 *
 * The view mode decides how much is shown, and this is the point of the whole
 * thing: **simple shows the name alone**, because a page of bech32 is what
 * makes an explorer unreadable to anyone who is not an engineer. **Expert shows
 * the name and the address**, because somebody debugging needs to know which
 * account a friendly label actually resolves to — a name with no address is
 * unverifiable, and an explorer that cannot be checked is decoration.
 */
function AccountLabel({ address, known }: { address: string; known?: string }) {
  const { mode } = useViewMode();

  // The chain's own identifier for this account, if it has claimed one.
  //
  // staleTime Infinity because a binding is permanent by design — an identifier
  // is retired, never repointed — so there is nothing to refetch. React Query
  // also dedupes by key, which matters here: a page listing fifty transfers
  // renders this component far more often than there are distinct addresses,
  // and without dedup that would be fifty requests for the same answer.
  const { data: userId } = useQuery({
    queryKey: ['user-id', address],
    queryFn: () => client.userIdOf(address),
    staleTime: Infinity,
    retry: false,
  });

  const resolved = displayName(address, () => userId ?? undefined);
  const label = resolved.kind === 'pseudonym' ? resolved.label : (known ?? resolved.label);
  const named = resolved.kind !== 'address' || Boolean(known);

  if (!named) return <span className="mono">{truncateAddress(address)}</span>;

  if (mode === 'expert') {
    return (
      <span>
        {label} <span className="mono muted">{truncateAddress(address)}</span>
      </span>
    );
  }
  return <span>{label}</span>;
}

export function TxLink({ hash, children }: { hash: string; children?: ReactNode }) {
  return (
    <Link to={`/tx/${hash}`} className="mono">
      {children ?? truncateHash(hash)}
    </Link>
  );
}

/** Success and failure carry a word and a shape, not only a colour. */
export function StatusBadge({ ok, compact }: { ok: boolean; compact?: boolean }) {
  return (
    <span className={ok ? 'badge badge--ok' : 'badge badge--bad'}>
      <span aria-hidden="true">{ok ? '✓' : '✕'}</span>
      {compact ? null : ok ? 'Confirmed' : 'Failed'}
    </span>
  );
}

export function Amount({ coins, direction }: { coins: Coin[] | undefined; direction?: 'in' | 'out' }) {
  if (!coins || coins.length === 0) return null;
  const cls = direction === 'in' ? 'amount amount--in' : 'amount amount--out';
  const sign = direction === 'in' ? '+' : direction === 'out' ? '−' : '';
  return (
    <span className={cls}>
      {sign}
      {sign ? ' ' : ''}
      {formatCoins(coins)}
    </span>
  );
}

export function Loading({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="state" role="status" aria-live="polite">
      <div className="stack" style={{ maxWidth: 420, margin: '0 auto' }}>
        <div className="skeleton" style={{ height: '1.4em' }} />
        <div className="skeleton" style={{ height: '1em', width: '80%' }} />
        <div className="skeleton" style={{ height: '1em', width: '60%' }} />
      </div>
      <p className="small faint" style={{ marginTop: '1rem' }}>
        {label}…
      </p>
    </div>
  );
}

export function Empty({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="state">
      <div className="state__title">{title}</div>
      {hint ? <p className="small" style={{ margin: '0.25rem auto 0', maxWidth: '46ch' }}>{hint}</p> : null}
    </div>
  );
}

/**
 * A failed request explains itself in terms of the explorer, not the network
 * stack: whether the chain is unreachable matters more to a reader than which
 * fetch rejected.
 */
export function ErrorState({ error, what }: { error: unknown; what: string }) {
  const message = error instanceof Error ? error.message : String(error);
  const notFound = /404/.test(message);

  return (
    <div className="state">
      <div className="state__title">{notFound ? `No ${what} found` : `Could not load ${what}`}</div>
      <p className="small" style={{ margin: '0.25rem auto 0', maxWidth: '48ch' }}>
        {notFound
          ? 'Check the reference and try again — it may belong to a different chain, or not exist yet.'
          : 'The chain may be unreachable. This page refreshes on its own, so it will recover when the connection does.'}
      </p>
      <details className="disclosure" style={{ maxWidth: '48ch', margin: '1rem auto 0', textAlign: 'left' }}>
        <summary>Technical detail</summary>
        <pre className="raw">{message}</pre>
      </details>
    </div>
  );
}

/**
 * Progress toward a threshold.
 *
 * The threshold is drawn as a marked line rather than described in the caption,
 * because "62% of 40% needed" is a sentence people have to stop and parse,
 * where a bar crossing a mark is read instantly. The caption is still there for
 * anyone using a screen reader, and the value is announced as text either way —
 * position alone never carries the meaning.
 */
export function Meter({
  value,
  threshold,
  label,
  caption,
  tone = 'neutral',
}: {
  value: number;
  threshold?: number;
  label: string;
  caption?: ReactNode;
  tone?: 'neutral' | 'good' | 'bad';
}) {
  const pct = Math.max(0, Math.min(1, value));
  const fill =
    tone === 'good' ? 'var(--positive)' : tone === 'bad' ? 'var(--negative)' : 'var(--accent, var(--text-muted))';

  return (
    <div className="meter">
      <div
        className="meter__track"
        role="meter"
        aria-valuenow={Math.round(pct * 100)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={label}
      >
        <div className="meter__fill" style={{ width: `${pct * 100}%`, background: fill }} />
        {threshold !== undefined && threshold > 0 && threshold < 1 ? (
          <div className="meter__mark" style={{ left: `${threshold * 100}%` }} aria-hidden="true" />
        ) : null}
      </div>
      {caption ? <div className="small muted meter__caption">{caption}</div> : null}
    </div>
  );
}

/** The raw payload, always one disclosure away wherever something was hidden. */
export function RawJson({ value, label = 'Raw data' }: { value: unknown; label?: string }) {
  return (
    <details className="disclosure">
      <summary>{label}</summary>
      <pre className="raw">{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}

const KIND_ICON: Record<string, string> = {
  transfer: '⇄',
  payment: '⇄',
  staking: '◆',
  governance: '§',
  trade: '⇋',
  treasury: '▣',
  issuance: '＋',
  admin: '⚙',
  other: '·',
};

export function MessageIcon({ message, failed }: { message: DecodedMessage; failed?: boolean }) {
  const cls = failed ? 'row__icon row__icon--bad' : 'row__icon';
  return (
    <span className={cls} aria-hidden="true">
      {failed ? '✕' : KIND_ICON[message.kind] ?? '·'}
    </span>
  );
}

export function RelativeTime({ value }: { value: string }) {
  return (
    <time dateTime={value} title={value}>
      {timeAgo(value)}
    </time>
  );
}
