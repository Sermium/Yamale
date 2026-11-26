/**
 * How the app names an account instead of printing its address: the shop's
 * name for a shop's own account, "Team · <shop>" for its staff, the account's
 * x/alias identifier (SN-K3M9-7QRT-B) for anyone who has one, and a short form
 * of the address only when none of those exists. The full address stays one
 * hover or long-press away, in the title.
 *
 * Every name here comes from the chain: a shop's name is what its registration
 * says, and an identifier is assigned by the chain rather than chosen, so a
 * label cannot be borrowed by somebody else.
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

function loadAlias(address: string): Promise<string | null> {
  let p = aliases.get(address);
  if (!p) {
    p = readAliasOf(address).then((r) => (r.ok && r.value ? formatAlias(r.value) : null)).catch(() => null);
    aliases.set(address, p);
  }
  return p;
}

export interface AccountLabel { name: string; detail: string | null }

export function labelFor(address: string, js: Jeweler[], alias: string | null): AccountLabel {
  const shop = js.find((j) => j.business === address);
  if (shop) return { name: shop.name, detail: alias };
  const team = js.find((j) => j.staff.includes(address));
  if (team) return { name: t('traceability.accountTeamOf', { shop: team.name }), detail: alias };
  if (alias) return { name: alias, detail: null };
  return { name: shortAddress(address), detail: null };
}

export function useAccountLabel(address: string): AccountLabel {
  const [label, setLabel] = useState<AccountLabel>({ name: shortAddress(address), detail: null });
  useEffect(() => {
    let live = true;
    Promise.all([loadJewelers(), loadAlias(address)]).then(([js, alias]) => {
      if (live) setLabel(labelFor(address, js, alias));
    });
    return () => { live = false; };
  }, [address]);
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
