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

import { requestOnboarding } from '../api.ts';
import { CHAIN_ID } from '../config.ts';
import { forgetAlias } from './names.tsx';
import type { Session } from './session.tsx';

// ISO 3166-1 alpha-2, the codes x/alias accepts (the reserved ones are refused on the chain).
const COUNTRIES = ('AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ ' +
  'CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE ' +
  'GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP ' +
  'KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF ' +
  'NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN ' +
  'SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW').split(' ');

function countryNames(lang: string): { code: string; name: string }[] {
  let names: Intl.DisplayNames | null = null;
  try { names = new Intl.DisplayNames([lang, 'en'], { type: 'region' }); } catch { /* old engine: codes only */ }
  return COUNTRIES.map((code) => ({ code, name: names?.of(code) ?? code }))
    .sort((a, b) => a.name.localeCompare(b.name, lang));
}

/** The country the browser's language names (fr-SN → SN), offered as a first guess. */
export function guessCountry(): string {
  try {
    const region = new Intl.Locale(navigator.language).maximize().region ?? '';
    return COUNTRIES.includes(region) ? region : '';
  } catch {
    return '';
  }
}

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
