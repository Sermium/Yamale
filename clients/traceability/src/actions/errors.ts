/**
 * The chain's refusals a person can hit, mapped to catalogue keys.
 *
 * Matched on the module's own message text, because CosmJS reports a code
 * without its codespace and codes repeat across modules. The texts are those
 * registered in x/jewel/types/errors.go and in x/feegrant.
 */

const RULES: ReadonlyArray<readonly [string, string]> = [
  ['already has an open offer', 'offerOpen'],
  ['has no open offer', 'noOffer'],
  ['this offer has expired', 'offerExpired'],
  ['the recipient already holds this jewel', 'selfOffer'],
  ['this offer was made to someone else', 'notRecipient'],
  ['the claim key or its signature does not match', 'badClaim'],
  ['changed owner after this offer was made', 'offerStale'],
  ['jewel is reported stolen', 'stolen'],
  ["only the jewel's owner may do this", 'notOwner'],
  ['already registered a jewel with this inscription', 'inscriptionTaken'],
  ['this jeweler is suspended', 'suspended'],
  ['you do not work for this jeweler', 'notStaff'],
  ['already works for another jeweler', 'staffTaken'],
  ["only the jeweler's business account", 'notBusiness'],
  ['this benefit has expired', 'benefitExpired'],
  ['allowance for the period is used up', 'allowanceUsed'],
  ['this voucher has already been used', 'voucherUsed'],
  ['the voucher is not valid', 'badVoucher'],
  ['veto window has not closed yet', 'windowOpen'],
  ['changed owner after the reassignment was proposed', 'reassignmentStale'],
  ['note is at most 500 bytes', 'noteTooLong'],
  ['this jewel has no ownership card', 'noCard'],
  ['the card key must be a compressed secp256k1 public key', 'badCardKey'],
  ["this is not the jewel's current ownership card", 'cardNotLive'],
  ["the card's signature does not match this claim", 'badCardClaim'],
  ["this jewel is no longer in the shop's stock", 'notStock'],
  ['a claim on this jewel is already waiting for the shop', 'claimPending'],
  ['no claim on this jewel is waiting for the shop', 'noClaim'],
  ['this claim was filed by someone else', 'notClaimer'],
  ["the shop's own business account cannot claim its stock", 'shopSelfClaim'],
  ["no move of this shop's business account is waiting", 'noBusinessMove'],
  ["the proposed move of this shop's business account has lapsed", 'businessMoveLapsed'],
  ["already runs a jeweler's business or works for another jeweler", 'accountTied'],
  ['already owns pieces from this shop', 'businessOwnsPieces'],
  ["this is already the shop's business account", 'moveToSelf'],
  ['fee-grant not found', 'fees'],
  ['fee allowance expired', 'fees'],
  ['fee limit exceeded', 'fees'],
  ['message does not exist in allowed messages', 'fees'],
  ['insufficient funds', 'fees'],
];

export function errorKey(raw: string): string | null {
  const text = raw.toLowerCase();
  for (const [needle, key] of RULES) {
    if (text.includes(needle.toLowerCase())) return key;
  }
  return null;
}
