/**
 * The address book, in two halves that must not be confused.
 *
 * **Public**, on the chain: the mapping from a user id or claimed name to an
 * account. Anyone can resolve it, nobody can forge it, and it is what makes a
 * payment code safe to hand to a stranger.
 *
 * **Private**, on the device: what *you* call somebody. "Mama", "the school",
 * "Tuesday supplier". This never leaves the phone. A contact list is a social
 * graph, and a social graph on a public ledger tells anyone who watches which
 * villages trade with which, who a journalist pays, and who stopped paying
 * rent — none of which needs consensus and all of which is dangerous.
 *
 * So: resolution is public, naming is private, and the two are joined only in
 * the reader's own interface.
 */
// The chain's own normalisation, not the payment-code one next door. They
// differ in exactly one place and it matters: the payment-code version folds I
// and L onto 1 and O onto 0 at every position, which over a user ID's first two
// characters would put CI and CL onto the same country, and SI and SL onto
// another — every one of those a real country, and the fold silently sending
// money to the wrong perimeter's account or to nobody.
import { normaliseUserId as normalise } from '@yamale/chain';

const STORE = 'yamale.app.book';

export interface Contact {
  /** The chain user id, e.g. NG-K3M9-7QRT-B, or a claimed name. */
  id: string;
  /** What this user calls them. Private, device-only, never published. */
  label: string;
  /** Last time they were paid, so the list can order by use rather than
   *  alphabetically — the person you paid yesterday is the one you want. */
  lastUsed?: number;
}

export function contacts(): Contact[] {
  try {
    const raw = localStorage.getItem(STORE);
    const list: Contact[] = raw ? JSON.parse(raw) : [];
    return list.sort((a, b) => (b.lastUsed ?? 0) - (a.lastUsed ?? 0));
  } catch {
    return [];
  }
}

export function save(contact: Contact): void {
  const list = contacts().filter((c) => normalise(c.id) !== normalise(contact.id));
  list.unshift(contact);
  localStorage.setItem(STORE, JSON.stringify(list));
}

export function remove(id: string): void {
  localStorage.setItem(STORE, JSON.stringify(contacts().filter((c) => normalise(c.id) !== normalise(id))));
}

export function touch(id: string): void {
  const found = contacts().find((c) => normalise(c.id) === normalise(id));
  if (found) save({ ...found, lastUsed: Date.now() });
}

/** The private label if there is one, otherwise the public id. Never an
 *  address — this is the only function screens should use to name anybody. */
export function displayName(id: string): string {
  const found = contacts().find((c) => normalise(c.id) === normalise(id));
  return found?.label ?? id;
}

/**
 * Resolve a user id to an account, through the chain's public book.
 *
 * Returns null rather than throwing when nothing is registered: an unknown id
 * is an ordinary outcome of somebody mistyping, not an error condition.
 */
export async function resolve(id: string): Promise<string | null> {
  const clean = normalise(id);
  try {
    const res = await fetch(`/api/rest/yamale/blockchain/alias/v1/alias/${clean}`);
    if (!res.ok) return null;
    const json = await res.json();
    return json.alias?.address ?? json.address ?? null;
  } catch {
    return null;
  }
}

/** The user's own id, so they can be paid. */
export async function myUserId(address: string): Promise<string | null> {
  try {
    const res = await fetch(`/api/rest/yamale/blockchain/alias/v1/alias_of/${address}`);
    if (!res.ok) return null;
    const json = await res.json();
    return json.alias?.id ?? json.id ?? null;
  } catch {
    return null;
  }
}

// --- recent, which is not the same as saved --------------------------------
//
// Somebody paid once is not a contact. Writing them straight into the address
// book would fill it with strangers — a market stall paid on holiday, a
// one-time refund — and a list nobody trusts is a list nobody reads, which is
// how the wrong recipient eventually gets tapped.
//
// So they land here instead: remembered, offered, and promoted to a contact
// only when the person says so.

const RECENT_STORE = 'yamale.app.recent';
const RECENT_MAX = 12;

export interface Recent {
  id: string;
  /** Their own name from the payment code, if the code carried one. Never a
   *  label this user chose — that only exists once they save them. */
  seenAs?: string;
  lastPaid: number;
}

export function recents(): Recent[] {
  try {
    const raw = localStorage.getItem(RECENT_STORE);
    const list: Recent[] = raw ? JSON.parse(raw) : [];
    const saved = new Set(contacts().map((c) => normalise(c.id)));
    // A person who has since been saved belongs in one list, not both.
    return list
      .filter((r) => !saved.has(normalise(r.id)))
      .sort((a, b) => b.lastPaid - a.lastPaid);
  } catch {
    return [];
  }
}

export function remember(id: string, seenAs?: string): void {
  const list = recents().filter((r) => normalise(r.id) !== normalise(id));
  list.unshift({ id: id.toUpperCase(), seenAs, lastPaid: Date.now() });
  localStorage.setItem(RECENT_STORE, JSON.stringify(list.slice(0, RECENT_MAX)));
}

export function forgetRecent(id: string): void {
  const list = recents().filter((r) => normalise(r.id) !== normalise(id));
  localStorage.setItem(RECENT_STORE, JSON.stringify(list));
}

/** Has this user paid them before, or saved them? Drives the extra
 *  confirmation on a first payment. */
export function isKnown(id: string): boolean {
  const key = normalise(id);
  return contacts().some((c) => normalise(c.id) === key)
    || recents().some((r) => normalise(r.id) === key);
}

/**
 * Who a counterparty is, in terms a person recognises.
 *
 * The order is contact name, then user ID, and never the account address. An
 * address is the one identifier in this app that is not meant to be read: the
 * whole point of user IDs is that nobody has to look at bech32, and a history
 * screen that falls back to it undoes the abstraction at exactly the moment
 * somebody is trying to remember who they paid.
 *
 * An address with no alias is somebody outside the app rather than a mystery,
 * so it gets a plain label rather than forty characters of noise.
 */
const nameCache = new Map<string, string>();

/**
 * Accounts that belong to the system rather than to a person.
 *
 * These have no user ID and never will — an alias identifies somebody who can
 * be paid, and nobody pays the faucet. Without this they surface in a history
 * as an unnamed party, which reads as an unexplained transfer from a stranger
 * when it is in fact the test money the app handed out at sign-in. Naming them
 * is the difference between a statement that explains itself and one that
 * raises a question.
 *
 * Held here rather than on the chain because it is a labelling concern, not a
 * consensus one: getting it wrong shows a wrong word, not a wrong balance.
 */
const WELL_KNOWN: Record<string, string> = {
  // The devnet faucet spends from the foundation account.
  yml1zqn5c7gwlqr6wk34w5af2hc53vzjkjf9ag3knh: 'Faucet',
};

export async function nameForAddress(address: string, fallback: string): Promise<string> {
  if (!address) return fallback;
  // Checked before the alias lookup: these accounts have no alias, so asking
  // the chain about them is a guaranteed round trip to nothing.
  const known = WELL_KNOWN[address];
  if (known) return known;

  const hit = nameCache.get(address);
  if (hit !== undefined) return hit;

  let label = fallback;
  try {
    const res = await fetch(`/api/rest/yamale/blockchain/alias/v1/alias_of/${address}`);
    if (res.ok) {
      const id = (await res.json())?.alias?.id ?? null;
      if (id) label = displayName(id) || id;
    }
  } catch {
    // Offline or the lookup is down: the fallback label is still better than
    // showing the raw address, so failure changes nothing here.
  }
  nameCache.set(address, label);
  return label;
}
