/**
 * The address book: your private names for people.
 *
 * This is deliberately **not** on the chain. A nickname needs no consensus —
 * nobody else has to agree that you call someone "Mum" — and an on-chain
 * directory of who-calls-whom-what would publish the social graph of everyone
 * using the network, permanently. The graph is more sensitive than the
 * payments.
 *
 * So it lives in this browser, on this origin, and travels only if the user
 * exports it.
 *
 * See docs/guides/identity.md for how this pairs with the chain-assigned user
 * ID that `x/alias` provides.
 */

import { formatUserId } from './alias.ts';

const STORAGE_KEY = 'yamale.addressbook.v1';

export interface Contact {
  address: string;
  /** What you call them. Yours alone; nobody can write this remotely. */
  pseudonym: string;
  /** Their chain-assigned user ID, recorded when you added them, so a later
   *  mismatch is visible rather than silent. Empty until x/alias exists. */
  userId?: string;
  addedAt: string;
  note?: string;
}

type Book = Record<string, Contact>;

function read(): Book {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Book) : {};
  } catch {
    // A corrupt book must not take the page down with it. An empty one shows
    // raw addresses, which is the honest fallback rather than a blank screen.
    return {};
  }
}

function write(book: Book): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(book));
  // Same-tab listeners: the `storage` event only fires in *other* tabs, so a
  // page that just saved a contact would not refresh its own display.
  window.dispatchEvent(new CustomEvent('yamale:addressbook'));
}

export function listContacts(): Contact[] {
  return Object.values(read()).sort((a, b) => a.pseudonym.localeCompare(b.pseudonym));
}

export function getContact(address: string): Contact | undefined {
  return read()[address];
}

export function saveContact(contact: Omit<Contact, 'addedAt'> & { addedAt?: string }): void {
  const book = read();
  book[contact.address] = {
    ...contact,
    addedAt: contact.addedAt ?? new Date().toISOString(),
  };
  write(book);
}

export function removeContact(address: string): void {
  const book = read();
  delete book[address];
  write(book);
}

export function exportBook(): string {
  return JSON.stringify({ version: 1, contacts: listContacts() }, null, 2);
}

/** Merges rather than replaces: an import should not silently drop contacts
 *  this device has and the file does not. */
export function importBook(json: string): number {
  const parsed = JSON.parse(json) as { contacts?: Contact[] };
  if (!Array.isArray(parsed.contacts)) throw new Error('Not an address book file.');
  const book = read();
  let added = 0;
  for (const contact of parsed.contacts) {
    if (typeof contact?.address !== 'string' || typeof contact?.pseudonym !== 'string') continue;
    book[contact.address] = { ...contact, addedAt: contact.addedAt ?? new Date().toISOString() };
    added += 1;
  }
  write(book);
  return added;
}

/** Registered user IDs, resolved elsewhere and handed in. Keeping the lookup
 *  out of here means this module stays synchronous and testable, and the caller
 *  decides how to cache a chain query. */
export type UserIdLookup = (address: string) => string | undefined;

export interface DisplayName {
  /** What to render. */
  label: string;
  /** Which tier produced it, so callers can style them differently — a
   *  pseudonym is your own word and a user ID is the chain's, and a payment
   *  confirmation should not present them identically. */
  kind: 'pseudonym' | 'userId' | 'address';
  /** Always the underlying address, for links and for showing on demand. */
  address: string;
}

/**
 * The one display rule, used everywhere.
 *
 *   1. a pseudonym in my address book   →  "Acme Ltd"
 *   2. else a registered user ID        →  "NG-K3M9-7QRT-B"
 *   3. else                             →  "yml1chm…7a8p"
 *
 * Written once and shared so the explorer, wallet, Safe and transfer app cannot
 * disagree about who somebody is. Four implementations of this rule would drift
 * into four different answers on the same screen.
 */
export function displayName(address: string, lookupUserId?: UserIdLookup): DisplayName {
  const contact = getContact(address);
  if (contact?.pseudonym) return { label: contact.pseudonym, kind: 'pseudonym', address };

  // Formatted here rather than by each caller. The country prefix is the point
  // of the second tier — an operator has to see which national perimeter an
  // account belongs to at a glance — and it is only a glance if the grouping
  // sets it apart from the payload.
  const userId = lookupUserId?.(address);
  if (userId) return { label: formatUserId(userId), kind: 'userId', address };

  return { label: truncate(address), kind: 'address', address };
}

function truncate(address: string): string {
  return address.length <= 20 ? address : `${address.slice(0, 10)}…${address.slice(-4)}`;
}

/** Subscribe to changes, in this tab and others. Returns an unsubscribe. */
export function onAddressBookChange(handler: () => void): () => void {
  const onStorage = (e: StorageEvent) => {
    if (e.key === STORAGE_KEY) handler();
  };
  window.addEventListener('storage', onStorage);
  window.addEventListener('yamale:addressbook', handler);
  return () => {
    window.removeEventListener('storage', onStorage);
    window.removeEventListener('yamale:addressbook', handler);
  };
}
