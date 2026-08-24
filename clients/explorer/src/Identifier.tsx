/**
 * An identifier, as something a person can act on.
 *
 * Addresses, hashes and user IDs get the same three affordances everywhere, and
 * getting any of them wrong costs real money on this chain:
 *
 *   **Mono.** A bech32 address set in a proportional sans cannot be compared to
 *   another one by eye, and comparing two addresses is the operation somebody
 *   confirming a payment performs most.
 *
 *   **Truncated, with a reveal.** Nobody reads 43 characters reliably, so an
 *   interface that demands it is not offering verification — it is offering the
 *   feeling of verification. The full value is one click away and stays on
 *   screen once revealed.
 *
 *   **Copy on click.** Selecting 43 characters by hand is how a transposition
 *   error gets into a transaction.
 *
 * And a name where the chain knows one. `x/alias` issues every account a user ID
 * carrying its country, and the SDK's `displayName` already ranks the four
 * sources — the reader's own address book, a well-known label, the chain's user
 * ID, then the raw address. A name with no address is unverifiable, so the
 * expert view shows both.
 */

import { useEffect, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { addressHue, displayName, formatUserId, t, truncateAddress, truncateHash } from '@yamale/chain';

import { client, useViewMode } from './chain.ts';

/** How long "Copied" stays on screen. Long enough to read, short enough not to lie. */
const COPIED_MS = 1600;

/**
 * One live region for the whole page, not one per button.
 *
 * A feed of forty rows renders eighty copy affordances. Giving each its own
 * `aria-live` span means eighty live regions on one document, which assistive
 * technology has to poll and which is a known way to make a page slow and
 * chatty for exactly the people it was supposed to help. There is one
 * announcement to make — "Copied" — so there is one region to make it in.
 */
const announcers = new Set<(message: string) => void>();

function announce(message: string): void {
  for (const listener of announcers) listener(message);
}

/** Mounted once, near the root. */
export function CopyAnnouncer() {
  const [message, setMessage] = useState('');

  useEffect(() => {
    const listener = (next: string) => setMessage(next);
    announcers.add(listener);
    return () => {
      announcers.delete(listener);
    };
  }, []);

  useEffect(() => {
    if (!message) return;
    const timer = setTimeout(() => setMessage(''), COPIED_MS);
    return () => clearTimeout(timer);
  }, [message]);

  return (
    <span className="visually-hidden" role="status" aria-live="polite">
      {message}
    </span>
  );
}

/**
 * Puts a value on the clipboard, on the origins this is actually served from.
 *
 * `navigator.clipboard` does not exist on an insecure origin, and this network
 * is reached over plain http on a tailnet address — so the modern API alone
 * makes every copy button on the page a control that does nothing, silently,
 * for the people running the demo. Measured, not assumed: `navigator.clipboard`
 * is `undefined` at http://100.68.207.17:8093.
 *
 * `document.execCommand('copy')` is deprecated and works there. So both are
 * tried, and the caller is told which happened rather than being allowed to
 * print "Copied" over a clipboard that did not change.
 */
async function writeClipboard(value: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return true;
    } catch {
      // Permission refused, or a non-focused document. Fall through.
    }
  }

  try {
    const holder = document.createElement('textarea');
    holder.value = value;
    holder.setAttribute('readonly', '');
    // Off-screen rather than hidden: a display:none element cannot be selected.
    holder.style.position = 'fixed';
    holder.style.top = '-1000px';
    holder.style.opacity = '0';
    document.body.appendChild(holder);
    holder.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(holder);
    return ok;
  } catch {
    return false;
  }
}

export function useCopy(): { copied: boolean; copy: (value: string) => void } {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copied]);

  return {
    copied,
    copy: (value: string) => {
      void writeClipboard(value).then((ok) => {
        // Nothing is claimed when nothing was copied. A "Copied" that is not
        // true is worse than a button that visibly did nothing, because the
        // reader pastes something else into a payment.
        if (!ok) return;
        setCopied(true);
        announce(t('xp.id.copied'));
      });
    },
  };
}

/**
 * Copy, on its own.
 *
 * Separate from the identifier itself because in a feed the identifier's click
 * has a better job — navigating to the account — and selecting 43 bech32
 * characters by hand is still how a transposition error gets into a
 * transaction. So both affordances exist and neither has to give way.
 *
 * The confirmation is a live region rather than only a colour change: it has to
 * reach somebody who cannot see the chip change.
 */
export function CopyButton({ value, label }: { value: string; label: string }) {
  const { copied, copy } = useCopy();
  return (
    <>
      <button
        type="button"
        className="ident__btn"
        onClick={() => copy(value)}
        title={`${t('xp.id.copy')} — ${value}`}
      >
        <span aria-hidden="true">⧉</span>
        <span className="visually-hidden">{`${t('xp.id.copy')} ${label}`}</span>
      </button>
      {/* Seen, not announced: the announcement comes from the single live
          region at the root. */}
      <span className="ident__copied" aria-hidden="true">
        {copied ? t('xp.id.copied') : ''}
      </span>
    </>
  );
}

/**
 * The primitive: a mono, truncated, copyable, revealable value.
 *
 * `label` names what is being copied for a screen reader, because "yml1rx…6szg,
 * button" is not a usable announcement.
 */
export function Identifier({
  value,
  label,
  short,
  to,
}: {
  value: string;
  label: string;
  /** The truncated form. Defaults to an address-shaped truncation. */
  short?: string;
  /** Where the identifier leads, when it leads anywhere. */
  to?: string;
}) {
  const [revealed, setRevealed] = useState(false);

  if (!value) return <span className="faint">—</span>;

  const shown = revealed ? value : (short ?? truncateAddress(value));

  return (
    <span className="ident">
      {to ? (
        <Link to={to} className="ident__value y-mono">
          {shown}
        </Link>
      ) : (
        <span className="ident__value y-mono">{shown}</span>
      )}
      <CopyButton value={value} label={label} />
      <button
        type="button"
        className="ident__btn"
        onClick={() => setRevealed((r) => !r)}
        aria-expanded={revealed}
        title={revealed ? t('xp.id.shorten') : t('xp.id.reveal')}
      >
        <span aria-hidden="true">{revealed ? '⇤' : '⇥'}</span>
        <span className="visually-hidden">{revealed ? t('xp.id.shorten') : t('xp.id.reveal')}</span>
      </button>
    </span>
  );
}

/**
 * An account: a colour derived from the address, the name if there is one, and
 * a link to its page.
 *
 * The colour is the cheapest recognition aid there is — the same account reads
 * the same way on every page without anybody memorising bech32 — and it is
 * never the only signal, because it is derived from a hash and two accounts can
 * land near each other.
 */
export function Account({
  address,
  known,
  compact,
}: {
  address: string;
  /** A name the caller already knows, e.g. a validator moniker. */
  known?: string;
  /** The inline form: name or truncation, a link, and copy. No reveal. */
  compact?: boolean;
}) {
  const { mode } = useViewMode();

  // A binding is permanent by design — an identifier is retired, never
  // repointed — so there is nothing to refetch. React Query dedupes by key,
  // which matters on a page rendering fifty rows over a dozen distinct
  // accounts.
  const { data: userId } = useQuery({
    queryKey: ['user-id', address],
    queryFn: () => client.userIdOf(address),
    staleTime: Infinity,
    retry: false,
    enabled: Boolean(address),
  });

  if (!address) return <span className="faint">—</span>;

  const resolved = displayName(address, () => userId ?? undefined);
  const name =
    resolved.kind === 'pseudonym'
      ? resolved.label
      : (known ?? (userId ? formatUserId(userId) : undefined));

  const swatch = (
    <span
      className="ident__swatch"
      aria-hidden="true"
      style={{ background: `hsl(${addressHue(address)} 52% 48%)` }}
    />
  );

  if (compact) {
    return (
      <span className="ident ident--compact">
        {swatch}
        <Link to={`/account/${address}`} className={name ? undefined : 'y-mono'}>
          {name ?? truncateAddress(address)}
        </Link>
        <CopyButton value={address} label="Account address" />
      </span>
    );
  }

  // No name to show: the identifier *is* the label, so it carries the copy and
  // the reveal directly.
  if (!name) {
    return (
      <span className="ident-group">
        {swatch}
        <Identifier value={address} label="Account address" to={`/account/${address}`} />
      </span>
    );
  }

  return (
    <span className="ident-group">
      {swatch}
      <Link to={`/account/${address}`} className="ident__name">
        {name}
      </Link>
      {/* A name with no address cannot be checked, and an explorer that cannot
          be checked is decoration. The detailed view always shows both. */}
      {mode === 'expert' ? <Identifier value={address} label="Account address" /> : null}
    </span>
  );
}

/**
 * The account being looked at, under its own page's title.
 *
 * The page used to print all 43 characters of the bech32 as its subtitle, which
 * is the one place an explorer most needs to answer "am I looking at the right
 * account" — and 43 characters is not an answer anybody can check. The name the
 * chain knows leads; the address is truncated behind it with the copy and the
 * reveal.
 */
export function AccountHeading({ address }: { address: string }) {
  const { data: userId } = useQuery({
    queryKey: ['user-id', address],
    queryFn: () => client.userIdOf(address),
    staleTime: Infinity,
    retry: false,
    enabled: Boolean(address),
  });

  const resolved = displayName(address, () => userId ?? undefined);

  return (
    <span className="ident-group">
      <span
        className="ident__swatch"
        aria-hidden="true"
        style={{ background: `hsl(${addressHue(address)} 52% 48%)` }}
      />
      {resolved.kind !== 'address' ? <strong>{resolved.label}</strong> : null}
      <Identifier value={address} label="Account address" />
    </span>
  );
}

/** A transaction hash, linked to its page. */
export function TxHash({ hash, children }: { hash: string; children?: ReactNode }) {
  return (
    <Link to={`/tx/${hash}`} className="y-mono">
      {children ?? truncateHash(hash)}
    </Link>
  );
}

/** A hash with no page behind it — a block hash, an app hash. */
export function Hash({ value, label }: { value: string; label: string }) {
  return <Identifier value={value} label={label} short={truncateHash(value)} />;
}
