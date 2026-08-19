/**
 * The public side of the address book.
 *
 * Deliberately a curated list rather than every account on the chain. The alias
 * module answers point lookups — "who is K3M9-7QRT-B", "what is this account
 * called" — and has no rpc that enumerates aliases. That absence is worth
 * keeping: on a payments chain, a directory anyone can page through is a
 * downloadable list of everybody who holds money, complete with the identifier
 * needed to pay them. It is the customer list, and publishing it is a decision
 * nobody should be able to make by adding a query.
 *
 * So "public" here means *listed* — an account that has chosen to be findable,
 * the way a business publishes a number and a person does not. Everyone else
 * stays reachable by exact ID, which is how you reach someone who gave you
 * theirs, and unreachable by browsing, which is how they stay unlisted.
 *
 * Held in the client for now. The moment a real chain has real listed
 * merchants, this belongs on-chain as an opt-in flag on the alias, so that a
 * business can list and delist itself without an app release — the entry is
 * theirs, not ours. Written here as data rather than markup so that migration
 * is a change of source, not a rewrite of the screen.
 */

export type EntryKind = 'system' | 'service';

export interface DirectoryEntry {
  /** The user ID, when the account has one and can be paid. */
  id?: string;
  /** The account address, for entries that exist but are not payable. */
  address?: string;
  label: string;
  /** What this account is, in one short line. */
  note: string;
  kind: EntryKind;
}

export const DIRECTORY: DirectoryEntry[] = [
  {
    // Listed precisely because a stranger has to be able to find them. A
    // moderator nobody can look up is a moderator nobody can appeal to, and
    // the escrow screen asks people to name one before they part with money.
    //
    // This identifier is from before user IDs carried a country, and the
    // jurisdiction upgrade tombstoned every one of that shape — it resolves to
    // nothing until the moderator's account is placed in a jurisdiction and
    // registers again. Replace it with the reissued identifier, which will read
    // as NG-…; leaving a dead one here sends somebody to an empty answer at the
    // moment they are trying to find an arbiter.
    id: 'ZW1AKM9AT',
    label: 'Chris — Moderator',
    note: 'Decides disputed secured payments.',
    kind: 'service',
  },
  {
    address: 'yml1zqn5c7gwlqr6wk34w5af2hc53vzjkjf9ag3knh',
    label: 'Faucet',
    note: 'Hands out test money when an account is empty.',
    kind: 'system',
  },
];

/** Case- and punctuation-insensitive match across an entry's visible text. */
export function matches(entry: DirectoryEntry, query: string): boolean {
  const q = query.trim().toLowerCase().replace(/[\s-]/g, '');
  if (!q) return true;
  return [entry.label, entry.note, entry.id ?? '', entry.address ?? '']
    .some((field) => field.toLowerCase().replace(/[\s-]/g, '').includes(q));
}
