/**
 * How the app names an account instead of printing its address: the shop's
 * name for a shop's own account; the person's name, from their profile, for
 * anyone who gave one; "Team · <shop>" for staff; the account's x/alias
 * identifier (SN-K3M9-7QRT-B) beside any of those, or alone; and a short form
 * of the address only when none exists. The full address is in the title.
 *
 * A shop's name and an identifier come from the chain and cannot be borrowed.
 * A person's name is what they wrote in their profile, which is why the
 * identifier is shown next to it: two people can share a name, never a code.
 */

import { useEffect, useState } from 'react';
import { formatAlias, readAliasOf, readJewelers, t, type Jeweler } from '@yamale/chain';

export const shortAddress = (a: string) => `${a.slice(0, 8)}…${a.slice(-4)}`;

// One read of the jewelers and of each identifier per page lifetime: names
// change rarely, and a list of twenty claims must not cost twenty reads each.
let jewelers: Promise<Jeweler[]> | null = null;
const aliases = new Map<string, Promise<string | null>>();

function loadJewelers(): Promise<Jeweler[]> {
  jewelers ??= readJewelers().then((r) => (r.ok ? r.value : [])).catch(() => []);
  return jewelers;
}

// Labels on screen re-read when an identifier is issued, or names become readable.
const listeners = new Set<() => void>();
const notify = () => { for (const l of listeners) l(); };

// People's names are read with the signed-in account's key, so the reader is
// installed by the session while it is unlocked and removed when it locks.
type NameReader = (address: string) => Promise<string | null>;
let nameReader: NameReader | null = null;
const people = new Map<string, Promise<string | null>>();

export function setNameReader(reader: NameReader | null): void {
  nameReader = reader;
  people.clear();
  notify();
}

/** Drop a remembered name (after the account saves its profile) and refresh the labels. */
export function forgetName(address: string): void {
  people.delete(address);
  notify();
}

function loadName(address: string): Promise<string | null> {
  if (!nameReader) return Promise.resolve(null);
  let p = people.get(address);
  if (!p) {
    p = nameReader(address).catch(() => null);
    people.set(address, p);
  }
  return p;
}

/** Drop what is known of an account's identifier, and refresh every label showing it. */
export function forgetAlias(address: string): void {
  aliases.delete(address);
  notify();
}

function loadAlias(address: string): Promise<string | null> {
  let p = aliases.get(address);
  if (!p) {
    p = readAliasOf(address).then((r) => (r.ok && r.value ? formatAlias(r.value) : null)).catch(() => null);
    aliases.set(address, p);
  }
  return p;
}

export interface AccountLabel { name: string; detail: string | null }

export function labelFor(address: string, js: Jeweler[], alias: string | null, person: string | null = null): AccountLabel {
  const shop = js.find((j) => j.business === address);
  if (shop) return { name: shop.name, detail: alias };
  const team = js.find((j) => j.staff.includes(address));
  const teamLabel = team ? t('traceability.accountTeamOf', { shop: team.name }) : null;
  if (person) return { name: person, detail: [teamLabel, alias].filter(Boolean).join(' · ') || null };
  if (teamLabel) return { name: teamLabel, detail: alias };
  if (alias) return { name: alias, detail: null };
  return { name: shortAddress(address), detail: null };
}

export function useAccountLabel(address: string): AccountLabel {
  const [label, setLabel] = useState<AccountLabel>({ name: shortAddress(address), detail: null });
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const l = () => setVersion((v) => v + 1);
    listeners.add(l);
    return () => { listeners.delete(l); };
  }, []);
  useEffect(() => {
    let live = true;
    Promise.all([loadJewelers(), loadAlias(address), loadName(address)]).then(([js, alias, person]) => {
      if (live) setLabel(labelFor(address, js, alias, person));
    });
    return () => { live = false; };
  }, [address, version]);
  return label;
}

/** An account as people recognise it, with "you" when it is this account. */
export function AccountName({ address, me }: { address: string; me?: string | null }) {
  const { name, detail } = useAccountLabel(address);
  return (
    <span className="accountname" title={address}>
      <span className="accountname__name">{name}</span>
      {detail && <span className="accountname__detail">{detail}</span>}
      {me === address && <span className="accountname__you">{t('traceability.you')}</span>}
    </span>
  );
}

/** The line under the email on the Account page: what others see this account as. */
export function ProfileCode({ address }: { address: string }) {
  const { name, detail } = useAccountLabel(address);
  return <p className="profile__code" title={address}>{detail ? `${name} · ${detail}` : name}</p>;
}
