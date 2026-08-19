/**
 * User IDs, client side.
 *
 * A port of the chain's own x/alias/types/id.go, and the only one in this
 * repository. Before this file there were three partial copies — an alphabet in
 * the transfer app, a hyphen grouper in the wallet, a shape regex in the
 * faucet — and none of them checked the check character. The result was that a
 * mistyped ID went to the node and came back "not found", which reads to a
 * person as "that account does not exist" rather than "you typed it wrong".
 *
 *     NG-K3M9-7QRT-B
 *     ^^ country      ISO 3166-1 alpha-2, plain letters
 *        ^^^^^^^^     payload, Crockford Base32
 *                ^    check character, Luhn mod 32 over both
 *
 * Every function here must agree with the Go exactly. They are the same
 * algorithm computed in two places, and a disagreement means a client that
 * rejects identifiers the chain issued, or accepts ones it never would.
 */

/** 0-9 then A-Z without I, L, O, U. Crockford's order, 32 symbols. */
export const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

/** How many characters of an identifier the country prefix occupies. */
export const COUNTRY_LENGTH = 2;

const MIN_PAYLOAD = 8;
const MAX_PAYLOAD = 16;

/**
 * The code carried by an account with no national perimeter — the foundation
 * administrators, and nobody else. ISO 3166-1 reserves ZZ permanently, so it
 * can never collide with a country.
 */
export const FOUNDATION_COUNTRY = 'ZZ';

const VALUE = new Map<string, number>();
for (let i = 0; i < ALPHABET.length; i++) VALUE.set(ALPHABET[i], i);
// Crockford's input folding: the transcription errors people actually make.
// These never appear in output.
VALUE.set('I', 1);
VALUE.set('L', 1);
VALUE.set('O', 0);

/**
 * Every officially assigned ISO 3166-1 alpha-2 code.
 *
 * Kept in step with x/alias/types/iso3166.go. A prefix that is merely two
 * letters would let NX or QK be presented as a perimeter; the chain refuses to
 * issue one, and this is what lets the client say so before a request is sent.
 */
const ASSIGNED = new Set(
  ('AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ ' +
    'BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ ' +
    'CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ ' +
    'DE DJ DK DM DO DZ EC EE EG EH ER ES ET ' +
    'FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY ' +
    'HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT ' +
    'JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ ' +
    'LA LB LC LI LK LR LS LT LU LV LY ' +
    'MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ ' +
    'NA NC NE NF NG NI NL NO NP NR NU NZ OM ' +
    'PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA ' +
    'RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ ' +
    'TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ ' +
    'UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW').split(' '),
);

/** Whether a code is a country the chain will record an account in. */
export function assignedCountry(code: string): boolean {
  return ASSIGNED.has(code.toUpperCase());
}

/** Whether a code may appear as an identifier's prefix — the assigned list plus
 *  the foundation's reserved code, and nothing else. */
export function issuableCountry(code: string): boolean {
  const c = code.toUpperCase();
  return c === FOUNDATION_COUNTRY || ASSIGNED.has(c);
}

/**
 * Strip formatting, uppercase, and fold the payload's confusable characters, so
 * "ng-k3m9-7qrt-b" and "NGK3M97QRTB" are the same identifier.
 *
 * The first two characters are left unfolded because they are the country and
 * the fold is not injective over the 26 letters: it would put CI and CL onto
 * the same prefix, and SI and SL onto another — Côte d'Ivoire indistinguishable
 * from Chile, Slovenia from Sierra Leone.
 */
export function normaliseUserId(id: string): string {
  let out = '';
  for (const raw of id) {
    if (raw === '-' || raw === ' ') continue;
    let c = raw.toUpperCase();
    if (out.length >= COUNTRY_LENGTH) {
      if (c === 'I' || c === 'L') c = '1';
      else if (c === 'O') c = '0';
    }
    out += c;
  }
  return out;
}

/**
 * Luhn mod 32 over the country prefix and the payload together.
 *
 * The prefix is covered because a typo in the country is exactly the error a
 * check character exists to catch, and the one typo with no other defence: NG
 * and NE are neighbours in the list and on the map. Prefix characters take
 * their value from A=0..Z=25 and payload characters from the Crockford table.
 */
function checkCharacter(country: string, payload: string): string {
  const base = 32;
  let sum = 0;
  let factor = 2;

  const fold = (value: number) => {
    let addend = factor * value;
    factor = 3 - factor;
    addend = Math.floor(addend / base) + (addend % base);
    sum += addend;
  };

  for (let i = payload.length - 1; i >= 0; i--) fold(VALUE.get(payload[i]) ?? 0);
  for (let i = country.length - 1; i >= 0; i--) fold(country.charCodeAt(i) - 65);

  return ALPHABET[(base - (sum % base)) % base];
}

/**
 * Whether an identifier is well formed, carries a prefix the chain will issue,
 * and agrees with its own check character.
 *
 * Call it before building a transaction or a lookup, so a mistyped ID is caught
 * in the input box rather than reported as a missing account. It says nothing
 * about whether the prefix is *true* of the account — that is chain state,
 * checked at issuance, which is why a lying one is never handed out.
 */
export function validUserId(id: string): boolean {
  const n = normaliseUserId(id);
  if (n.length < COUNTRY_LENGTH + MIN_PAYLOAD + 1) return false;
  if (n.length > COUNTRY_LENGTH + MAX_PAYLOAD + 1) return false;

  const country = n.slice(0, COUNTRY_LENGTH);
  if (!issuableCountry(country)) return false;

  const payload = n.slice(COUNTRY_LENGTH, -1);
  for (const c of payload) if (!VALUE.has(c)) return false;

  return checkCharacter(country, payload) === n[n.length - 1];
}

/**
 * The country an identifier names, or '' if it is too short to have one.
 *
 * Read off the identifier rather than fetched separately: it is the only copy
 * of that fact, and the one the person is looking at.
 */
export function userIdCountry(id: string): string {
  const n = normaliseUserId(id);
  return n.length < COUNTRY_LENGTH ? '' : n.slice(0, COUNTRY_LENGTH);
}

/**
 * NGK3M97QRTB → NG-K3M9-7QRT-B.
 *
 * The country is its own group, because the point of putting it in the
 * identifier is that an operator can see at a glance which national perimeter
 * an account belongs to — and it can only be seen at a glance if it is not run
 * together with the payload.
 */
export function formatUserId(id: string): string {
  const n = normaliseUserId(id);
  if (n.length < COUNTRY_LENGTH + 2) return n;

  const groups = [n.slice(0, COUNTRY_LENGTH)];
  const payload = n.slice(COUNTRY_LENGTH, -1);
  for (let i = 0; i < payload.length; i += 4) groups.push(payload.slice(i, i + 4));
  groups.push(n[n.length - 1]);
  return groups.join('-');
}
