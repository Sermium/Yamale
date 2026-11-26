/**
 * Countries, for the country an account belongs to: the list x/alias accepts
 * and each country's name in the person's language. Shared by every app's
 * signup and profile, so they all offer the same list.
 */

// ISO 3166-1 alpha-2, the codes x/alias accepts (the reserved ones are refused on the chain).
export const COUNTRIES = ('AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ ' +
  'CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE ' +
  'GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP ' +
  'KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF ' +
  'NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN ' +
  'SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW').split(' ');

export function countryNames(lang: string): { code: string; name: string }[] {
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

/** A country's name in the language given, e.g. countryName('SN', 'fr') → "Sénégal". */
export function countryName(code: string, lang: string): string {
  try { return new Intl.DisplayNames([lang, 'en'], { type: 'region' }).of(code) ?? code; } catch { return code; }
}
