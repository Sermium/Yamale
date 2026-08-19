import { useEffect, useState } from 'react';

import { displayName, onAddressBookChange, type DisplayName } from './addressbook.ts';

/**
 * Name an account, resolving every tier including the chain's.
 *
 * `displayName` is deliberately synchronous — it is a pure rule over data the
 * caller already has — but the user-ID tier needs a chain query. This hook is
 * the join: it resolves the ID once, caches it in the client, and re-renders
 * the name when it arrives.
 *
 * Shared rather than reimplemented per app. Four copies of "how is an account
 * named" drift into four different answers on the same screen, and the whole
 * point of the rule is that the explorer, the wallet and the Safe agree.
 *
 * The address renders immediately and is replaced by the name when the lookup
 * returns. Waiting for the query before showing anything would make every list
 * flash empty, and an address is the correct answer until a better one arrives.
 */
export function useDisplayName(
  address: string,
  lookup: (address: string) => Promise<string | null>,
): DisplayName {
  const [userId, setUserId] = useState<string | null>(null);
  const [, bump] = useState(0);

  useEffect(() => {
    if (!address) return;
    let cancelled = false;
    void lookup(address).then((id) => !cancelled && setUserId(id));
    return () => {
      cancelled = true;
    };
  }, [address, lookup]);

  // A name saved in one component should appear in the others without a
  // reload; the address book fires an event precisely so this is possible.
  useEffect(() => onAddressBookChange(() => bump((n) => n + 1)), []);

  return displayName(address, () => userId ?? undefined);
}

/**
 * Render an account by name.
 *
 * `full` shows the address alongside the name — for confirmation screens and
 * expert views, where a name with no address is unverifiable.
 */
export function AccountName({
  address,
  lookup,
  full,
}: {
  address: string;
  lookup: (address: string) => Promise<string | null>;
  full?: boolean;
}) {
  const shown = useDisplayName(address, lookup);
  if (!full || shown.kind === 'address') return <span>{shown.label}</span>;
  return (
    <span>
      {shown.label} <span className="mono muted small">{truncate(address)}</span>
    </span>
  );
}

function truncate(address: string): string {
  return address.length <= 20 ? address : `${address.slice(0, 10)}…${address.slice(-4)}`;
}
