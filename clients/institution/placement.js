/**
 * Reading a placement request, and deciding whether this institution may act.
 *
 * Pure: nothing here fetches. The chain facts arrive as an argument, which is
 * what makes every refusal below testable without a node.
 *
 * The refusals are the product. A screen that printed a command for every paste
 * would send an institution to the chain to be refused, and the chain's refusal
 * arrives without the sentence explaining which of several rules was broken.
 */

/** ISO 3166-1 permanently reserves ZZ; x/alias uses it as the foundation marker. */
export const FOUNDATION_MARKER = 'ZZ';

const FIELD = (text, label) => {
  const m = new RegExp(`^${label}:\\s*(.+)$`, 'm').exec(text);
  return m ? m[1].trim() : '';
};

/**
 * Parse the document `clients/sdk/src/placement.ts` prints.
 *
 * The header is required. Recovering an address out of arbitrary text would let
 * this screen offer to place an account nobody requested, which is the one
 * mistake an institution cannot detect by reading the screen.
 */
export function parseRequest(text) {
  const body = String(text ?? '');
  if (!/^PLACEMENT REQUEST\s*$/m.test(body)) {
    return { problem: 'That is not a placement request. Paste the whole document the holder gave you, header included.' };
  }
  const address = FIELD(body, 'account');
  const countryField = FIELD(body, 'country');
  const institution = FIELD(body, 'onboarded by');

  const country = (countryField.split(/\s+/)[0] ?? '').toUpperCase();
  if (!address) return { problem: 'The request names no account.' };
  if (!/^[A-Z]{2}$/.test(country)) return { problem: 'The request names no country, or one that is not two letters.' };

  return { address, country, institution: institution === '(name the institution)' ? '' : institution };
}

/**
 * What this institution may do about a parsed request, given what the chain says.
 *
 * `blocking` stops the command being printed at all. `warnings` do not — they
 * are things worth saying beside a command that will work.
 */
export function judge(parsed, facts) {
  const blocking = [];
  const warnings = [];

  if (!parsed || parsed.problem) {
    return { ok: false, action: 'refuse', blocking: [parsed?.problem ?? 'Nothing to judge.'], warnings };
  }

  if (!facts.exists) {
    blocking.push('The chain has never seen this account. It holds no balance and has signed nothing, so there is nothing yet to place.');
  }

  if (parsed.country === FOUNDATION_MARKER) {
    blocking.push(
      `${FOUNDATION_MARKER} is the marker a foundation administrator carries, derived from the role and never recorded. `
      + 'The chain refuses it as a jurisdiction, so that it cannot be handed to an ordinary account.',
    );
  }

  if (facts.jurisdiction) {
    blocking.push(
      `This account is already recorded in ${facts.jurisdiction.country}, by ${facts.jurisdiction.recorded_by}. `
      + 'Changing it is a correction, and a correction is a foundation administrator’s act rather than yours: '
      + 'a participant able to rewrite a country it had already recorded could move a customer out from under an '
      + 'authority mid-investigation.',
    );
  }

  const named = (parsed.institution ?? '').trim().toLowerCase();
  if (!named) {
    warnings.push('The request names no institution. Check it is yours to act on before you sign anything.');
  } else if (!facts.participants.some((p) => p.name.trim().toLowerCase() === named)) {
    blocking.push(
      `"${parsed.institution}" is not an approved participant on this chain. Only an approved participant may `
      + 'record where an account belongs, so this recording would be refused.',
    );
  }

  return {
    ok: blocking.length === 0,
    action: blocking.length === 0 ? 'record' : 'refuse',
    blocking,
    warnings,
  };
}
