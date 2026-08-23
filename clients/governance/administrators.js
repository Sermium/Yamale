// Composing the governance proposal that appoints or removes a foundation
// administrator.
//
// A foundation administrator is the account that may correct any account's
// recorded country. That is not an administrative convenience: a correction moves
// a customer out from under the authority investigating them, and it retires and
// reissues their identifier in the same message. They are also the only accounts
// permitted to hold an identifier with no country at all, carrying the reserved
// ZZ code. `x/alias` names them in one parameter and matches by exact address
// equality.
//
// They are appointed by an ordinary governance MsgUpdateParams and by nothing
// else. The foundation's own 3-of-5 cannot do it, and until this file existed
// there was no interface that composed the message at all — the list was empty on
// the live chain and there was no way to appoint anybody.
//
// # The trap this module exists to absorb
//
// MsgUpdateParams carries a `Params` message, not a field mask. Setting it
// REPLACES THE WHOLE OBJECT. So "appoint one administrator" is really "read the
// current parameters, add one address, and resubmit every parameter", and the
// failure mode is not an error — it is a proposal that passes and quietly drops
// the administrators already appointed, or resets payload_length to a default
// nobody voted for. Nothing on the chain catches it: Params.Validate() bounds the
// length and refuses duplicates, and a shorter list than before is a perfectly
// valid list.
//
// Everything below is arranged around that one fact:
//
//   - readAliasParams REFUSES rather than defaulting. If the chain's answer
//     cannot be read in full, no proposal is composed. A default here is the bug.
//   - It refuses a params object carrying a field this build does not know,
//     because a field we do not model is a field we would silently zero.
//   - planChange changes exactly one thing and returns the whole object twice,
//     before and after, so a voter can see what else is moving.
//   - paramsDigest lets the page notice that the parameters moved after a
//     proposal was composed, which turns the document stale in a way that is
//     invisible by looking at it.
//
// Nothing here signs. It composes a proposal document and the command that
// submits it, the same choice clients/foundation and clients/validator made, for
// the same reason: a wallet's approval screen decodes a signing request only as
// far as its message type URLs, so signing in the browser would mean asking
// somebody to approve "/blockchain.alias.v1.MsgUpdateParams" on the strength of
// this page's description of it. A CLI command is parsed by the binary that owns
// the message, in front of the person who can still stop it.

// ---------------------------------------------------------------- the chain's rules

/**
 * The cap on the list, from x/alias/types/params.go.
 *
 * Not about storage. It is there so that widening the one rule the whole
 * perimeter rests on cannot happen by accident: a proposal that appends a hundred
 * addresses fails outright rather than passing because nobody scrolled.
 */
export const MAX_FOUNDATION_ADMINISTRATORS = 8;

/**
 * The bounds Params.Validate() puts on payload_length, from x/alias/types/id.go.
 *
 * Carried here because this module has to resubmit the value, and a value it
 * cannot check is a value it should not send. Below eight the identifier space
 * stops being unguessable; above sixteen nobody can read one aloud.
 */
export const MIN_PAYLOAD_LENGTH = 8;
export const MAX_PAYLOAD_LENGTH = 16;

/** Where the parameters are read from. */
export const ALIAS_PARAMS_PATH = '/yamale/blockchain/alias/v1/params';

/**
 * The message type URL, from the proto package `blockchain.alias.v1`.
 *
 * Note it is not `/yamale.blockchain.alias.v1.…`: the REST path carries the
 * `yamale` prefix and the proto package does not. Getting that wrong produces a
 * document `tx gov submit-proposal` refuses to decode, which is at least a loud
 * failure — but it fails after the text has been written and circulated.
 */
export const UPDATE_PARAMS_TYPE = '/blockchain.alias.v1.MsgUpdateParams';

/**
 * Every field of `Params` this build knows how to carry across a replacement.
 *
 * This list is the safety net for the whole module and it is worth being explicit
 * about why. MsgUpdateParams replaces the entire object, so composing one means
 * writing a value for every field — including any field added to the proto after
 * this page was written. A page that ignored an unknown field would compose a
 * proposal that set it to its protobuf default, and the diff it showed would not
 * mention it, because it does not know it exists.
 *
 * So an unrecognised field is a refusal. It is the one failure this interface
 * cannot make visible, so it is the one it will not proceed through.
 */
export const KNOWN_PARAM_FIELDS = Object.freeze(['payload_length', 'foundation_administrators']);

/** The account prefix on this chain. */
export const ADDRESS_PREFIX = 'yml';

/** Where the module accounts are listed, which is where the authority is read from. */
export const MODULE_ACCOUNTS_PATH = '/cosmos/auth/v1beta1/module_accounts';

/** Where the deposit a proposal needs is read from. */
export const GOV_DEPOSIT_PARAMS_PATH = '/cosmos/gov/v1/params/deposit';

/** Where a group policy is looked up, to tell a group account from a plain key. */
export const groupPolicyPath = (address) => `/cosmos/group/v1/group_policy_info/${encodeURIComponent(address)}`;

/**
 * The governance module account, as a fallback only.
 *
 * `authtypes.NewModuleAddress("gov")` is sha256("gov") truncated to twenty bytes,
 * rendered in this chain's prefix. It is deterministic, so it can be written down
 * — but it is written down here as the LAST resort, because a console with an
 * authority compiled into it is a console that would keep composing confidently
 * against a chain whose gov module was renamed or whose prefix changed, and the
 * proposal would pass its vote and then be refused at execution by x/alias's
 * authority check.
 *
 * readGovAuthority reads it off the chain in preference to this, and the page says
 * which of the two it used.
 */
export const GOV_MODULE_ADDRESS_FALLBACK = 'yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386';

/**
 * The authority x/alias will accept, read off the chain.
 *
 * Read rather than derived, and derived rather than hardcoded, in that order of
 * preference. Deriving it in the browser is not available here: sha256 through
 * SubtleCrypto needs a secure context and this console is served over plain HTTP
 * on a tailnet address, where `crypto.subtle` is undefined. So the chain's own
 * list of module accounts is the primary source and the constant above is the
 * fallback.
 *
 * @param {unknown} response the body of GET /cosmos/auth/v1beta1/module_accounts
 * @returns {string} the gov module account's bech32 address
 * @throws {Error} when the answer does not contain one
 */
export function readGovAuthority(response) {
  const accounts = response && typeof response === 'object' ? response.accounts : null;
  if (!Array.isArray(accounts)) {
    throw new Error('The module accounts could not be read from that answer.');
  }
  for (const account of accounts) {
    // The shape is a ModuleAccount inside an Any, and the gateway has rendered it
    // both flat and nested over this chain's life. Both are checked rather than
    // one, because the failure of guessing is a console that silently falls back
    // to a constant while reporting that it read the chain.
    const name = account?.name ?? account?.base_account?.name;
    const address = account?.base_account?.address ?? account?.address;
    if (name === 'gov' && typeof address === 'string' && address) return address;
  }
  throw new Error('No module account named "gov" in that answer.');
}

/**
 * The minimum deposit, as the single coin string the CLI wants.
 *
 * Read from the chain because it is a governance parameter and this chain has
 * changed it. A proposal submitted with less than the minimum is accepted, sits
 * in the deposit period, and never enters a vote — which looks from the outside
 * exactly like a proposal nobody got round to voting on.
 *
 * Only the first coin is used. A min_deposit of several denominations would need
 * all of them, and this says so rather than quietly paying one.
 */
export function readMinDeposit(response) {
  const coins = response?.params?.min_deposit ?? response?.deposit_params?.min_deposit;
  if (!Array.isArray(coins) || coins.length === 0) {
    throw new Error('The minimum deposit could not be read from the chain.');
  }
  if (coins.length > 1) {
    throw new Error(
      `This chain's minimum deposit is ${coins.length} denominations at once, which this page does not ` +
      'compose. Build the deposit string by hand from `blockchaind query gov params deposit`.',
    );
  }
  const { amount, denom } = coins[0];
  if (!amount || !denom) throw new Error('The minimum deposit came back without an amount or a denomination.');
  return `${amount}${denom}`;
}

/**
 * How long a bech32 address is, in bytes, for the two derivations that matter.
 *
 * 20 bytes is a key: a single secp256k1 public key hashed. 32 bytes is a derived
 * address — an x/group policy, a module account, or anything else built by
 * `address.Derive`. The distinction is a heuristic and is treated as one below:
 * it can prove an address is NOT a group account, and it cannot prove that it is.
 */
export const KEY_ADDRESS_BYTES = 20;
export const DERIVED_ADDRESS_BYTES = 32;

// ---------------------------------------------------------------- bech32

const BECH32_CHARSET = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';
const BECH32_GENERATOR = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];

function polymod(values) {
  let chk = 1;
  for (const value of values) {
    const top = chk >>> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ value;
    for (let i = 0; i < 5; i += 1) {
      if ((top >>> i) & 1) chk ^= BECH32_GENERATOR[i];
    }
  }
  return chk >>> 0;
}

function hrpExpand(hrp) {
  const out = [];
  for (const ch of hrp) out.push(ch.charCodeAt(0) >>> 5);
  out.push(0);
  for (const ch of hrp) out.push(ch.charCodeAt(0) & 31);
  return out;
}

/**
 * A bech32 address, decoded and checksummed.
 *
 * This module verifies the checksum itself, and that is not belt-and-braces: the
 * chain does NOT check these addresses. Params.Validate() refuses an empty
 * string, a duplicate, and a ninth entry, and says nothing whatever about whether
 * the string is an address. A mistyped administrator therefore passes a
 * governance vote, occupies one of the eight slots, and grants the power to
 * nobody — a failure that looks exactly like success until somebody tries to
 * correct a country and finds the account they thought they had appointed is not
 * the account in the list.
 *
 * Six characters of checksum catch every single-character typo and every
 * transposition, which is what a person copying an address off a ceremony record
 * actually does wrong.
 *
 * @returns {{ prefix: string, bytes: number }} the human-readable part and the
 *   length of the decoded payload in bytes.
 * @throws {Error} with a message naming what is wrong, for display.
 */
export function decodeBech32(address) {
  const raw = String(address ?? '');
  if (raw !== raw.trim()) {
    throw new Error('That address has whitespace around it. Trim it before comparing it to anything.');
  }
  if (raw.length === 0) throw new Error('No address.');
  if (raw.length > 90) throw new Error(`That is ${raw.length} characters; a bech32 string is at most 90.`);
  if (raw !== raw.toLowerCase() && raw !== raw.toUpperCase()) {
    throw new Error('That address mixes upper and lower case, which bech32 does not allow.');
  }

  const s = raw.toLowerCase();
  const split = s.lastIndexOf('1');
  if (split < 1) throw new Error('That is not a bech32 address: there is no prefix before a "1".');
  const prefix = s.slice(0, split);
  const dataPart = s.slice(split + 1);
  if (dataPart.length < 6) throw new Error('That address is too short to carry a bech32 checksum.');

  const values = [];
  for (const ch of dataPart) {
    const v = BECH32_CHARSET.indexOf(ch);
    if (v === -1) {
      throw new Error(`"${ch}" is not a bech32 character. Note that 1, b, i and o never appear in one.`);
    }
    values.push(v);
  }
  if (polymod([...hrpExpand(prefix), ...values]) !== 1) {
    throw new Error(
      'That address fails its own checksum, so at least one character is wrong. ' +
      'Copy it again from the record rather than correcting it by eye.',
    );
  }

  // The checksum is the last six characters; what is left is a 5-bit-per-word
  // encoding of the address bytes.
  const words = values.length - 6;
  const bits = words * 5;
  if (bits % 8 >= 5) {
    throw new Error('That address has a malformed bech32 payload.');
  }
  return { prefix, bytes: Math.floor(bits / 8) };
}

/**
 * A yes/no with a reason, for an address typed into a form.
 *
 * Separated from decodeBech32 so the caller does not have to catch to ask a
 * question, and so the prefix check has one implementation. A valid bech32 string
 * with the wrong prefix is the mistake worth naming specially: a `cosmos1…`
 * address is a real address on another chain, and the chain here would store it
 * happily.
 */
export function checkAddress(address, { prefix = ADDRESS_PREFIX } = {}) {
  try {
    const decoded = decodeBech32(address);
    if (decoded.prefix !== prefix) {
      return {
        ok: false,
        reason: `That address belongs to "${decoded.prefix}", not to "${prefix}". ` +
          'It is a valid address on some other chain, and this one would store it as text and grant it nothing.',
      };
    }
    return { ok: true, bytes: decoded.bytes };
  } catch (e) {
    return { ok: false, reason: e.message };
  }
}

/**
 * Whether an address COULD be a group policy account.
 *
 * A 20-byte address cannot be: it is a single public key, so appointing it makes
 * one person the correction authority for every account on the chain. A 32-byte
 * address is derived, which is what an x/group policy is — and also what a module
 * account is, so this is evidence and not proof. The page pairs it with a live
 * lookup against x/group, which is proof; this is what remains true when that
 * lookup cannot be made.
 */
export function couldBeGroupAccount(address) {
  const checked = checkAddress(address);
  return checked.ok && checked.bytes === DERIVED_ADDRESS_BYTES;
}

// ---------------------------------------------------------------- reading the chain

/**
 * Raised when the parameters cannot be read in full.
 *
 * A distinct error type because the caller's response to it is specific and not
 * negotiable: show the reason and compose nothing. Every other problem in this
 * module is a message in a list beside a form; this one stops the form existing.
 */
export class ParamsUnreadable extends Error {
  constructor(message) {
    super(message);
    this.name = 'ParamsUnreadable';
  }
}

function integerFrom(value) {
  // Accepted as a number or a string, because the gateway's marshaller has
  // emitted both for scalar fields over this chain's life and neither is wrong.
  // Rejected for anything else, including a float, because payload_length is a
  // uint32 and 8.5 is not one.
  if (typeof value === 'number') return Number.isInteger(value) ? value : null;
  if (typeof value === 'string' && /^\d+$/.test(value.trim())) return Number(value.trim());
  return null;
}

/**
 * The current parameters, or a refusal.
 *
 * This is the function the whole module hangs off, so it is worth stating what it
 * will not do: it will not return a default. Not for an absent params object, not
 * for a missing payload_length, and not for a payload_length of zero.
 *
 * The last of those is the one that needs the argument. Proto3 cannot tell a zero
 * from a field nobody filled in — a uint32 of 0 and an absent uint32 are the same
 * bytes on the wire and, depending on the marshaller, the same absent key in the
 * JSON. So a payload_length that reads as 0 means EITHER the chain has a zero in
 * it (which Validate would have refused, so it does not) OR this response is not
 * carrying the field. Either way this module does not know the value, and the
 * only safe thing to do with a value you do not know is refuse to resubmit it. A
 * default of 8 here would be a proposal that resets the identifier length of a
 * chain that had raised it, and the diff would show no change at all.
 *
 * The administrator list is different in kind and is treated differently. It is a
 * repeated field, and for a repeated field absent and empty ARE the same value —
 * there is no third state to confuse them with. So an absent list is read as
 * empty, honestly, and `listAbsent` is set so the page can say that the chain
 * returned no list rather than implying it displayed one.
 *
 * @param {unknown} response the parsed body of GET /yamale/blockchain/alias/v1/params
 */
export function readAliasParams(response) {
  if (response === null || typeof response !== 'object' || Array.isArray(response)) {
    throw new ParamsUnreadable(
      "The node's answer is not an object, so the current parameters are unknown. " +
      'No proposal can be composed from that: MsgUpdateParams replaces every parameter at once, ' +
      'so composing one without knowing the current values would set the ones it did not know to their defaults.',
    );
  }
  const params = response.params;
  if (params === null || typeof params !== 'object' || Array.isArray(params)) {
    throw new ParamsUnreadable(
      `The node's answer carries no "params" object, so x/alias's current parameters are unknown. ` +
      'Nothing is composed from an unknown starting point, because MsgUpdateParams sets the whole object ' +
      'and every parameter this page could not read would be replaced by its default.',
    );
  }

  // The check that matters most, and the one that will fire years from now. See
  // KNOWN_PARAM_FIELDS.
  const unknown = Object.keys(params).filter((k) => !KNOWN_PARAM_FIELDS.includes(k));
  if (unknown.length > 0) {
    throw new ParamsUnreadable(
      `x/alias has a parameter this page does not know about: ${unknown.map((u) => `"${u}"`).join(', ')}. ` +
      'MsgUpdateParams replaces the whole Params object, so a proposal composed here would write nothing for ' +
      'that field and the chain would set it to its protobuf default — and the before/after below would not ' +
      'mention it, because this page cannot show a field it has never heard of. ' +
      'Compose this proposal by hand, or update this console.',
    );
  }

  const payloadLength = integerFrom(params.payload_length);
  if (payloadLength === null || payloadLength === 0) {
    throw new ParamsUnreadable(
      'payload_length did not come back as a usable number ' +
      `(the node sent ${JSON.stringify(params.payload_length ?? null)}). ` +
      'It is not defaulted here on purpose: proto3 cannot tell a zero from a field nobody filled in, so a ' +
      'zero or an absent value means this page does not know the current identifier length — and a proposal ' +
      'that guessed it would reset the length of every identifier issued from then on, showing no change in ' +
      'the diff.',
    );
  }
  if (payloadLength < MIN_PAYLOAD_LENGTH || payloadLength > MAX_PAYLOAD_LENGTH) {
    throw new ParamsUnreadable(
      `payload_length reads as ${payloadLength}, and the chain accepts ${MIN_PAYLOAD_LENGTH} to ` +
      `${MAX_PAYLOAD_LENGTH}. A value the chain would refuse cannot be resubmitted, and this page will not ` +
      'quietly correct it — if the chain really holds that value, something has gone wrong that a proposal ' +
      'composed here would hide.',
    );
  }

  const rawList = params.foundation_administrators;
  const listAbsent = rawList === undefined || rawList === null;
  if (!listAbsent && !Array.isArray(rawList)) {
    throw new ParamsUnreadable(
      'foundation_administrators did not come back as a list, so the current administrators are unknown. ' +
      'Resubmitting the parameters without knowing who is already appointed is how a proposal removes ' +
      'somebody nobody voted to remove.',
    );
  }
  const administrators = (listAbsent ? [] : rawList).map((a) => {
    if (typeof a !== 'string') {
      throw new ParamsUnreadable(
        `foundation_administrators contains ${JSON.stringify(a)}, which is not an address. ` +
        'The current list cannot be read, so nothing is composed from it.',
      );
    }
    return a;
  });

  return { payloadLength, administrators, listAbsent };
}

/**
 * A fingerprint of the parameters, for noticing that they moved.
 *
 * The hazard this closes is quiet: somebody reads the parameters, composes a
 * proposal, and takes twenty minutes to circulate it — during which another
 * proposal passes and appoints somebody. The document they hold is now a document
 * that REMOVES that person, and it looks exactly as it did when it was correct.
 * Nothing in the JSON says which parameters it was built from.
 *
 * So the page keeps this digest beside every composed document and re-checks it
 * on every refresh. Deliberately not a cryptographic hash: it is compared against
 * a value this same page computed seconds ago, and the thing it defends against
 * is a race, not an adversary.
 */
export function paramsDigest(params) {
  return JSON.stringify([params.payloadLength, [...params.administrators]]);
}

// ---------------------------------------------------------------- planning a change

/**
 * One appointment or one removal, checked, with the whole object before and after.
 *
 * Exactly one change per proposal, and that is a deliberate restriction rather
 * than a simplification. Two appointments in one message are two decisions on one
 * vote, and the ordinary way that goes wrong is that a voter agrees with one of
 * them. The chain would accept a proposal doing four things at once; this
 * composes one that does one.
 *
 * `problems` are refusals and `warnings` are not. The difference is whether the
 * chain would accept the result: a duplicate or a ninth administrator is refused
 * by Params.Validate(), so composing it would waste a vote, and a single-key
 * administrator is accepted by the chain and is still a bad idea. Collapsing the
 * two would mean either blocking something legitimate or shipping something the
 * chain rejects.
 *
 * @param {object} args
 * @param {{payloadLength: number, administrators: string[]}} args.params current, from readAliasParams
 * @param {string} [args.add] the address to appoint
 * @param {string} [args.remove] the address to remove
 * @param {'group'|'not-group'|'unknown'} [args.groupLookup] what x/group says about `add`
 */
export function planChange({ params, add = '', remove = '', groupLookup = 'unknown' }) {
  const problems = [];
  const warnings = [];
  const appointing = String(add ?? '').trim();
  const removing = String(remove ?? '').trim();

  if (appointing && removing) {
    problems.push(
      'This composes one appointment or one removal, not both. Two changes on one vote is two decisions ' +
      'a voter can only agree with together, and the ordinary outcome is that they agree with one of them. ' +
      'Raise them as two proposals.',
    );
  }
  if (!appointing && !removing) {
    problems.push('Name an address to appoint, or one of the current administrators to remove.');
  }
  if (problems.length > 0) {
    return { action: 'none', problems, warnings, before: null, after: null, subject: '' };
  }

  const before = { payloadLength: params.payloadLength, administrators: [...params.administrators] };
  const action = appointing ? 'appoint' : 'remove';
  const subject = appointing || removing;
  let after = null;

  if (action === 'appoint') {
    const checked = checkAddress(appointing);
    if (!checked.ok) {
      problems.push(
        `${checked.reason} The chain would not catch this: Params.Validate() bounds the list and refuses ` +
        'duplicates, and never checks that an entry is an address at all — so a mistyped one would pass the ' +
        'vote, take one of the eight places, and grant the power to nobody.',
      );
    } else {
      if (before.administrators.includes(appointing)) {
        problems.push(
          `${appointing} is already a foundation administrator. Params.Validate() refuses a list that names ` +
          'one address twice, so this proposal would fail when it executed — after the vote. A list that ' +
          'reads as seven names and grants six is a list nobody can audit by looking at it.',
        );
      }
      if (before.administrators.length >= MAX_FOUNDATION_ADMINISTRATORS) {
        problems.push(
          `There are already ${before.administrators.length} foundation administrators and the chain caps the ` +
          `list at ${MAX_FOUNDATION_ADMINISTRATORS}. The cap is not about storage: it is there so that ` +
          'widening the one rule the whole perimeter rests on cannot happen by accident. Remove somebody ' +
          'first, in its own proposal, so both decisions are voted on separately.',
        );
      }
      // Warnings, not refusals. The chain will accept a single key here, and
      // saying so plainly is the point — an interface that implied otherwise
      // would be lying about where the protection comes from.
      if (groupLookup === 'not-group') {
        warnings.push(
          `x/group has no group policy at ${appointing}, so this is a plain account — in all likelihood a ` +
          'single key. An office that is one key is one bribe, and this particular office can move any ' +
          'customer on the chain out from under the authority investigating them. THE CHAIN WILL ACCEPT IT: ' +
          'x/alias matches administrators by address equality and does not care what kind of account it is, ' +
          'unlike MsgGrantRole, which refuses a holder that is not a group account. Nothing downstream will ' +
          'stop this. Appoint an M-of-N group instead unless you have a reason not to, and put the reason in ' +
          'the summary.',
        );
      } else if (groupLookup === 'unknown' && !couldBeGroupAccount(appointing)) {
        warnings.push(
          `${appointing} is a ${KEY_ADDRESS_BYTES}-byte address, which is a single public key rather than a ` +
          'derived account — an x/group policy address is longer. x/group could not be reached to confirm it, ' +
          'but the length alone rules a group policy out. One key holds the power to correct any account\'s ' +
          'country, and the chain will accept it: unlike a role grant, an administrator is matched by address ' +
          'equality with no check on what kind of account it is.',
        );
      } else if (groupLookup === 'unknown') {
        warnings.push(
          `x/group could not be reached, so this page cannot confirm that ${appointing} is an M-of-N group ` +
          'account. Its length is consistent with one — and equally with a module account or any other ' +
          'derived address, so that is not proof. Confirm it before voting: ' +
          `blockchaind query group group-policy-info ${appointing}`,
        );
      }
      if (problems.length === 0) {
        // Sorted, so the resulting list depends on the SET of administrators and
        // not on the order appointments happened to be proposed in. Two proposals
        // appointing the same two people in opposite orders should not produce two
        // different Params objects, and a diff a reader has to sort in their head
        // is a diff that hides a change.
        after = {
          payloadLength: before.payloadLength,
          administrators: [...before.administrators, appointing].sort(),
        };
      }
    }
  } else {
    if (!before.administrators.includes(removing)) {
      // Refused rather than composed as a no-op. "Nothing to remove" is how a
      // proposal that named the wrong address passes while leaving the
      // administrator it meant to remove in place — the same argument
      // MsgRevokeRole makes for refusing a grant that was never made.
      const checked = checkAddress(removing);
      problems.push(
        `${removing} is not one of the ${before.administrators.length} current foundation administrators, ` +
        'so this proposal would change nothing while reading as a removal. It would pass, and the ' +
        'administrator somebody meant to remove would still hold the power.' +
        (checked.ok ? '' : ` (It is also not a valid address: ${checked.reason})`),
      );
    } else {
      after = {
        payloadLength: before.payloadLength,
        administrators: before.administrators.filter((a) => a !== removing),
      };
      if (after.administrators.length === 0) {
        // Not a refusal. Empty is the documented default and the safe state, and
        // an interface that could not express it would be an interface that
        // cannot undo an appointment.
        warnings.push(
          'This removes the last foundation administrator, leaving the list empty. That is a real state and ' +
          'the documented default: with nobody named, the exemption from the jurisdiction rule grants nothing ' +
          'at all, which is the safe direction. What it costs is that NOBODY can correct a recorded country ' +
          'afterwards, and nobody can hold an identifier with no country — including the foundation itself. ' +
          'Enrolling a new country needs an administrator, because the first institutions in it have no ' +
          'participant to record their jurisdiction. Governance can appoint one again, by another proposal ' +
          'like this one.',
        );
      }
    }
  }

  return { action, problems, warnings, before, after, subject };
}

/**
 * The whole object, before and after, as rows for display.
 *
 * Every field, always — including the ones that did not change. That is the
 * requirement rather than a nicety: the reason to show the whole object is that
 * MsgUpdateParams replaces the whole object, so "payload_length: 8 → 8" is
 * information. A diff that showed only what moved would be a diff that could not
 * tell a reader that nothing else did.
 */
export function paramsDiff(before, after) {
  if (!before || !after) return [];
  const rows = [
    {
      field: 'payload_length',
      before: String(before.payloadLength),
      after: String(after.payloadLength),
      changed: before.payloadLength !== after.payloadLength,
    },
  ];
  const all = [...new Set([...before.administrators, ...after.administrators])].sort();
  if (all.length === 0) {
    rows.push({
      field: 'foundation_administrators',
      before: '(empty)',
      after: '(empty)',
      changed: false,
      addresses: [],
    });
    return rows;
  }
  rows.push({
    field: 'foundation_administrators',
    before: `${before.administrators.length} named`,
    after: `${after.administrators.length} named`,
    changed: before.administrators.length !== after.administrators.length,
    addresses: all.map((address) => ({
      address,
      state: before.administrators.includes(address)
        ? (after.administrators.includes(address) ? 'kept' : 'removed')
        : 'added',
    })),
  });
  return rows;
}

// ---------------------------------------------------------------- the documents

/**
 * The MsgUpdateParams, as it appears inside a proposal.
 *
 * Field names are the proto's own, snake_case, because that is what the amino-JSON
 * decoder in `tx gov submit-proposal` reads. `params` is not nullable in the proto
 * and carries both fields explicitly, including an empty list: writing it out
 * rather than omitting it is what makes the document say the same thing the diff
 * above said.
 */
export function updateParamsMessage({ authority, params }) {
  if (!authority) {
    throw new Error(
      'No authority address. This message is refused by x/alias unless the signer is the governance module ' +
      "account, so a proposal naming anything else would pass the vote and then fail at execution.",
    );
  }
  return {
    '@type': UPDATE_PARAMS_TYPE,
    authority,
    params: {
      payload_length: params.payloadLength,
      foundation_administrators: [...params.administrators],
    },
  };
}

/**
 * The document `blockchaind tx gov submit-proposal` reads.
 *
 * A governance proposal, not an x/group one, and that difference is the reason
 * this lives in the governance console rather than in the foundation's. The
 * foundation's 3-of-5 cannot appoint an administrator: x/alias's UpdateParams is
 * authority-gated to the gov module account and to nothing else. So the people who
 * decide are the whole voting set, over the full voting period, and the account
 * that signs the message is the gov module's own.
 */
export function proposalDocument({ messages, title, summary, deposit, metadata = '' }) {
  if (!Array.isArray(messages) || messages.length === 0) {
    throw new Error('A proposal with no messages would pass and do nothing.');
  }
  if (!title) throw new Error('Give the proposal a title — it is what a voter sees first.');
  if (!summary) throw new Error('Give the proposal a summary. It is the only explanation most voters will read.');
  if (!deposit) throw new Error('A governance proposal needs a deposit, or it never enters the voting period.');
  return JSON.stringify({ messages, metadata, deposit, title, summary }, null, 2);
}

/**
 * What a voter is being asked to agree to, in words.
 *
 * Every appointment says what the power is, because the name of the parameter
 * does not. "foundation_administrators" reads like a list of people with logins;
 * what it actually confers is the ability to move any account on the chain out
 * from under the authority investigating it. A voter who reads only the summary
 * should have read the proposal.
 */
export function changeSummary({ action, subject, before, after, reason }) {
  const kind = couldBeGroupAccount(subject) ? 'a derived account (an M-of-N group, or a module account)' : 'a single key';
  const head = action === 'appoint'
    ? `Appoints ${subject} as a foundation administrator on x/alias, taking the list from ` +
      `${before.administrators.length} to ${after.administrators.length}. It is ${kind}.`
    : `Removes ${subject} as a foundation administrator on x/alias, taking the list from ` +
      `${before.administrators.length} to ${after.administrators.length}.`;
  const power =
    'A foundation administrator may correct any account\'s recorded country — which moves that account out ' +
    'from under the authority investigating it, and retires and reissues its identifier — and may hold an ' +
    `identifier with no country, carrying the reserved ZZ code. payload_length stays at ${after.payloadLength}.`;
  const tail = after.administrators.length === 0
    ? ' No administrator remains: no recorded country can be corrected until governance appoints one again.'
    : '';
  return `${head} ${power}${tail}${reason ? ` ${reason}` : ''}`;
}

/**
 * The command that submits it.
 *
 * `--from` is left as a placeholder rather than filled in from the page, because
 * whoever runs this signs with a key this page has never seen and should not be
 * told which one it is.
 */
export function submitCommand({ chainId, file = 'proposal.json', from = '<your-key>' }) {
  if (!chainId) {
    throw new Error(
      'No chain id. A proposal submitted against the wrong chain is refused, but a command printed without ' +
      'one is a command somebody completes from memory.',
    );
  }
  // The gas is explicit, and that is not a stylistic choice. The 200,000 default
  // runs out part-way through a proposal that carries a message and fails with
  // code 11 — which reads as a rejected proposal rather than as an unfunded
  // transaction, and costs the deposit nothing because the proposal never
  // existed. 600,000 is the figure this chain's own ops service uses.
  return [
    `blockchaind tx gov submit-proposal ${file} \\`,
    `  --from ${from} --chain-id ${chainId} \\`,
    '  --gas 600000 --fees 20000uyml',
  ].join('\n');
}

/**
 * What to do after broadcasting, as text.
 *
 * Present because a broadcast reporting `code: 0` has been ACCEPTED and has not
 * executed, and believing otherwise has caused four separate bugs in this
 * repository. The proposal id is in the queried transaction's events and nowhere
 * in the broadcast's own output.
 */
export function afterSubmitting({ chainId }) {
  return [
    '# code: 0 from the broadcast means ACCEPTED, not executed. Query it:',
    'blockchaind query tx <hash> -o json',
    '# the proposal id is in the submit_proposal event in that output',
    '',
    '# then vote, from each validator, on its own node:',
    `blockchaind tx gov vote <id> yes --from <key> --chain-id ${chainId}`,
    '',
    '# and when the voting period ends, check it actually executed. A proposal',
    '# can PASS and still fail to execute, which leaves the parameters unchanged',
    '# and reports it nowhere anybody is watching:',
    'blockchaind query gov proposal <id> -o json',
    'blockchaind query alias params -o json',
  ].join('\n');
}
