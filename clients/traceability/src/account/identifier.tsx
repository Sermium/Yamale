/**
 * The account's identifier (SN-K3M9-7QRT-B): how other people recognise it in
 * the app instead of a long address.
 *
 * x/alias issues it only to an account whose country is recorded, and the
 * first recording must come from the participant that registered the account.
 * On devnet that participant is the platform's onboarding account, and the
 * country is the one the person chose here. The account then confirms the
 * participant and claims its identifier in one sponsored transaction. The
 * chain derives the identifier; nobody chooses it.
 */

import { confirmParticipant, registerAlias, t } from '@yamale/chain';

import { countryNames, guessCountry } from '../../../shared/account/countries.ts';
import { requestOnboarding } from '../../../shared/account/people.ts';

export { guessCountry };
import { CHAIN_ID } from '../config.ts';
import { forgetAlias } from './names.tsx';
import type { Session } from './session.tsx';

export function CountrySelect({ id, value, onChange }: { id: string; value: string; onChange: (code: string) => void }) {
  const lang = (typeof document !== 'undefined' && document.documentElement.lang) || 'en';
  return (
    <select id={id} className="bigfield" value={value} onChange={(e) => onChange(e.target.value)} required>
      <option value="">{t('traceability.countryChoose')}</option>
      {countryNames(lang).map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}
    </select>
  );
}

export type ClaimOutcome = 'done' | 'unavailable' | 'refused';

/** Onboard the account in its country, then confirm the participant and claim the identifier. */
export async function claimIdentifier(session: Session, country: string): Promise<ClaimOutcome> {
  const me = session.address;
  if (!me || !country) return 'refused';
  const sponsored = await session.ensureSponsored().catch(() => 'unavailable' as const);
  if (sponsored !== 'sponsored') return sponsored;
  const onboarded = await session.withKey((k) => requestOnboarding(k, me, country, CHAIN_ID));
  if (!onboarded.ok) return onboarded.status === 0 || onboarded.status >= 500 ? 'unavailable' : 'refused';
  const sent = await session.submitAsOwner([confirmParticipant(me, onboarded.participant), registerAlias(me)]);
  forgetAlias(me);
  return sent.succeeded ? 'done' : 'refused';
}
