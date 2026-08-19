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

import { validUserId } from '@yamale/chain';

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
  /**
   * Why this entry cannot be reached yet, for a listing that is real but whose
   * identifier is not known to this build.
   *
   * Present instead of `id`, never alongside it. Shown to the reader, because
   * "we know who this is and cannot route you to them" is information, and an
   * entry that quietly renders without an identifier is not.
   */
  unavailable?: string;
}

export const DIRECTORY: DirectoryEntry[] = [
  {
    // Listed precisely because a stranger has to be able to find them. A
    // moderator nobody can look up is a moderator nobody can appeal to, and
    // the escrow screen asks people to name one before they part with money.
    //
    // This entry used to carry ZW1AKM9AT, issued before user IDs carried a
    // country. The x/alias v1→v2 migration tombstones every identifier of that
    // shape, so it resolved to nothing — and resolved to nothing *quietly*,
    // which is the worst version: the card rendered, the ID looked plausible,
    // and the lookup came back empty at the moment somebody was trying to find
    // an arbiter.
    //
    // The reissued identifier reads as NG-… and cannot be known without asking
    // a chain that has run the migration, so it is not guessed here. Until
    // somebody reads it off the chain and puts it in, the entry says so.
    unavailable: 'Identifier being reissued after the jurisdiction upgrade.',
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

/**
 * Reject a directory that cannot do its job, at import, before a screen renders.
 *
 * A directory entry is a promise that somebody can be reached. The failure this
 * guards is the one that already happened: an identifier issued under the old
 * scheme sat here after the jurisdiction upgrade tombstoned it, and nothing
 * anywhere said so — not the type checker, not a test, not the screen. It cost
 * nothing to leave and it broke the one path that has to work when a payment
 * has gone wrong.
 *
 * `validUserId` is the client's port of the chain's own check, so a pre-migration
 * identifier — no country prefix, and therefore a failing check character — is
 * rejected here rather than by an empty answer from the node. Throwing is
 * deliberate: this runs at module load, so a bad entry fails the build and the
 * dev server on the commit that introduces it, instead of shipping.
 */
for (const entry of DIRECTORY) {
  if (entry.id !== undefined && entry.unavailable !== undefined) {
    throw new Error(
      `directory: "${entry.label}" has both an id and an unavailable reason — it is one or the other`,
    );
  }
  if (entry.id !== undefined && !validUserId(entry.id)) {
    throw new Error(
      `directory: "${entry.label}" carries the user ID ${entry.id}, which this chain cannot issue. ` +
        'Identifiers issued before the jurisdiction upgrade have no country prefix and were tombstoned ' +
        'by the x/alias v1→v2 migration. Read the reissued identifier off the chain, or give the entry ' +
        'an `unavailable` reason instead.',
    );
  }
  if (entry.id === undefined && entry.address === undefined && entry.unavailable === undefined) {
    throw new Error(`directory: "${entry.label}" has no id, no address and no reason for having neither`);
  }
}

/** Case- and punctuation-insensitive match across an entry's visible text. */
export function matches(entry: DirectoryEntry, query: string): boolean {
  const q = query.trim().toLowerCase().replace(/[\s-]/g, '');
  if (!q) return true;
  return [entry.label, entry.note, entry.id ?? '', entry.address ?? '']
    .some((field) => field.toLowerCase().replace(/[\s-]/g, '').includes(q));
}
