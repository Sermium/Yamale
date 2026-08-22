import { FOUNDATION_COUNTRY, assignedCountry } from './alias.ts';
import { getLocale } from './i18n.ts';

/**
 * Where an account stands with respect to a country, and what that costs it.
 *
 * Every account on this chain is country-gated, and the gate is not advisory:
 * `CountryOf` refuses an account nobody has placed, and the identifier issuer
 * refuses in turn, so **an unplaced account gets no user ID at all**. Without a
 * user ID nobody can address a payment to it. It can hold a balance and it can
 * send; it cannot be found.
 *
 * That consequence is the whole reason this module exists. A wallet that
 * generated a key, said "account created", and left somebody to discover weeks
 * later that they cannot be paid would be technically correct and useless. So
 * placement is a state the interface reports, with the consequence attached,
 * rather than a detail buried in a registry.
 *
 * The wallet cannot fix it, and that is deliberate rather than a missing
 * feature: the first recording of a country belongs to the approved participant
 * that onboarded the account, because that is the only party that performed the
 * KYC and therefore the only one that knows the answer. An account free to name
 * its own perimeter would name the one with no authority watching it. So the
 * most a key-generating tool can honestly produce is a *request*.
 */

/** What the chain knows about an account's placement. */
export interface Placement {
  /** The recorded country, or null for an account nobody has placed. */
  country: string | null;
  /** The identifier the chain issued, or null — which follows from country. */
  userId: string | null;
}

export type PlacementState = 'placed' | 'unplaced' | 'inconsistent';

export interface PlacementVerdict {
  state: PlacementState;
  /** One line, for a badge. */
  headline: string;
  /** What it means for the person holding this account. */
  consequence: string;
  /** Who can change it. Never "you". */
  remedy: string;
}

/**
 * Read a placement into something worth showing somebody.
 *
 * The third state is the one worth having. A country with no identifier is not
 * a normal resting state — the chain issues one when it records a country — so
 * it means either that issuance failed or that this client is looking at a
 * chain mid-migration. Reporting it as "placed" would hide a real fault behind
 * a green tick, and reporting it as "unplaced" would send somebody back to an
 * institution that has already done its part.
 */
export function placementVerdict(p: Placement): PlacementVerdict {
  const country = p.country?.toUpperCase() ?? null;

  if (!country) {
    return {
      state: 'unplaced',
      headline: 'This account is not on the rail yet',
      consequence:
        'It has no user ID, because the chain issues one only for an account recorded in a ' +
        'country. You can hold funds and you can send them; nobody can address a payment to ' +
        'you until this is done.',
      remedy:
        'The institution that onboarded you records it — once, and only it. This wallet cannot, ' +
        'and no account may declare its own country.',
    };
  }

  if (!p.userId) {
    return {
      state: 'inconsistent',
      headline: `Recorded in ${country}, but holding no user ID`,
      consequence:
        'The chain issues an identifier when it records a country, so these two disagreeing is ' +
        'not a normal state. Treat the account as unreachable for payments until it resolves.',
      remedy: 'Ask the institution that recorded the country to check the account.',
    };
  }

  return {
    state: 'placed',
    headline: `On the rail in ${country}`,
    consequence: 'Payments can be addressed to this account by its user ID.',
    remedy: 'Only a foundation administrator can change the country now, and doing so reissues the user ID.',
  };
}

/** Why a requested country was refused. Null when it is acceptable. */
export function countryProblem(code: string): string | null {
  const c = (code ?? '').trim().toUpperCase();
  if (!c) return 'Choose the country that will hold your account.';
  if (c.length !== 2) return 'A country is two letters, as in SN or GH.';
  if (c === FOUNDATION_COUNTRY) {
    // Refused rather than accepted-and-rejected-later. ZZ marks the absence of
    // a national perimeter and belongs to foundation administrators; recorded
    // against an ordinary account it would issue an identifier that reads as
    // chain-wide authority.
    return `${FOUNDATION_COUNTRY} is the foundation's reserved code and is not a country an account can be placed in.`;
  }
  if (!assignedCountry(c)) {
    return `${c} is not an assigned ISO 3166-1 country code, so the chain would refuse to record it.`;
  }
  return null;
}

/**
 * The country's name in the reader's own language, with the code as the
 * fallback.
 *
 * `Intl.DisplayNames` rather than a table: a 250-row list would need
 * maintaining, would be wrong in every language but one, and the browser
 * already has this. The code is what the chain stores, so it is always shown
 * alongside — a person confirming their country to an institution over the
 * phone needs the letters, not the translation.
 *
 * Defaults to the interface's own language rather than to English. A French
 * page reading "SN (Senegal)" is the kind of half-translation that tells
 * somebody the product was not really built for them.
 */
export function countryName(code: string, locale?: string): string {
  const c = (code ?? '').trim().toUpperCase();
  if (c.length !== 2) return c;
  try {
    const names = new Intl.DisplayNames([locale ?? getLocale()], { type: 'region' });
    return names.of(c) ?? c;
  } catch {
    return c;
  }
}

export interface PlacementRequest {
  address: string;
  country: string;
  institution: string;
  /** What the holder hands over. */
  document: string;
  /** What the participant runs. Composed, not signed — see below. */
  command: string;
}

/**
 * The request an account holder gives to the institution that onboarded them.
 *
 * A document and a command, and neither is a transaction: this tool holds no
 * key that could sign one, and the key that must sign is the participant's. So
 * what it produces is the exact thing a participant needs in order to act, in a
 * form the holder can send over any channel and the participant can read
 * before running.
 *
 * The address is repeated inside the document rather than left to a covering
 * message, because the failure this prevents is somebody being placed at the
 * wrong address — which succeeds, looks right, and issues an identifier to an
 * account nobody holds.
 */
export function placementRequest(args: {
  address: string;
  country: string;
  institution?: string;
  chainId?: string;
  bin?: string;
}): PlacementRequest | { problem: string } {
  const address = (args.address ?? '').trim();
  const country = (args.country ?? '').trim().toUpperCase();
  const institution = (args.institution ?? '').trim();

  const problem = countryProblem(country);
  if (problem) return { problem };
  if (!address) return { problem: 'No account address to place.' };

  const bin = args.bin ?? 'blockchaind';
  const chainId = args.chainId ?? '';

  const document = [
    'PLACEMENT REQUEST',
    '',
    `account:      ${address}`,
    `country:      ${country} (${countryName(country)})`,
    institution ? `onboarded by: ${institution}` : 'onboarded by: (name the institution)',
    chainId ? `chain:        ${chainId}` : '',
    '',
    'I am asking the institution that onboarded me to record this account as',
    `being in ${country}. I understand that I cannot record it myself, that this`,
    'account has no user ID until it is recorded, and that nobody can address a',
    'payment to me until then.',
  ].filter((line) => line !== '').join('\n');

  const command = [
    `${bin} tx alias set-jurisdiction ${address} ${country} \\`,
    `  --from <the participant's key>${chainId ? ` --chain-id ${chainId}` : ''} \\`,
    '  --fees 500uyml --yes',
  ].join('\n');

  return { address, country, institution, document, command };
}
