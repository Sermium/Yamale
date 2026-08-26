/**
 * The payment instruction, as ISO 20022 would express it.
 *
 * This app sends `cosmos.bank.v1beta1.MsgSend`. The chain has a message for a
 * real payment instruction — `blockchain.paymsg.v1.MsgSendPayment`, which
 * writes a queryable PaymentRecord carrying an end-to-end id, a purpose code,
 * both agents and a settlement jurisdiction — and it is unreachable from this
 * app, because it refuses any payment whose instructing and instructed
 * participants are not governance-approved, and `yamale-devnet-2` has none.
 *
 * The temptation is to hide that. The site advertises a payment carrying its
 * reference and its purpose code, so a screen that quietly sends a bare
 * transfer is the product lying about itself. This module takes the other
 * option: build the instruction that *would* be sent, field for field, and
 * state for each field where it actually ends up today. A payer sees the
 * purpose code they chose, sees that it travels in the transaction memo rather
 * than in a PaymentRecord, and sees which precondition is what stops it.
 *
 * Nothing here touches the network or React, so all of it is testable without
 * either. The live half — what the chain says about approved participants — is
 * in standing.ts.
 */

/** ------------------------------------------------------------------ purpose
 *
 * ISO 20022 ExternalPurpose1Code. Real codes, not a local invention: a purpose
 * code that is not in the published list is a purpose code no correspondent
 * bank can act on, which makes it decoration.
 *
 * A short list rather than the full several hundred. The full set is a
 * reference document, and a person paying a market trader is not going to read
 * one; these are the categories that actually occur on this chain's stated use
 * cases — trade, wages, family remittance, rent, tax — plus an honest OTHR.
 */
export interface Purpose {
  /** The four-letter ISO code that travels. */
  code: string;
  /** The catalogue key for its label. Rendering is the caller's job. */
  key: string;
}

export const PURPOSES: Purpose[] = [
  { code: 'GDDS', key: 'iso.purpGDDS' },
  { code: 'SCVE', key: 'iso.purpSCVE' },
  { code: 'SUPP', key: 'iso.purpSUPP' },
  { code: 'SALA', key: 'iso.purpSALA' },
  { code: 'FAMI', key: 'iso.purpFAMI' },
  { code: 'RENT', key: 'iso.purpRENT' },
  { code: 'TAXS', key: 'iso.purpTAXS' },
  { code: 'OTHR', key: 'iso.purpOTHR' },
];

/** The catalogue key for a code, or null when the code is not one we offer. */
export function purposeKey(code: string): string | null {
  return PURPOSES.find((p) => p.code === code)?.key ?? null;
}

/** ------------------------------------------------------------ end-to-end id
 *
 * ISO 20022 EndToEndId: at most 35 characters, unique per instructing party,
 * and the one identifier that survives from the payer's ledger to the payee's.
 * It is what a reconciliation actually joins on, so it is generated here rather
 * than left to the payer to invent.
 *
 * The date is in it on purpose. An id that is only randomness is unsearchable
 * by a human being holding a paper invoice; `YML-20260826-K3M9QRTB` can be
 * eyeballed against a statement for the right month before anybody reaches for
 * a query.
 *
 * Randomness is injected rather than read from `crypto` inside, so a test can
 * assert the exact string instead of a shape.
 */
const ID_ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

export function endToEndId(when: Date, entropy: Uint8Array): string {
  const y = when.getUTCFullYear().toString().padStart(4, '0');
  const m = (when.getUTCMonth() + 1).toString().padStart(2, '0');
  const d = when.getUTCDate().toString().padStart(2, '0');

  let tail = '';
  for (let i = 0; i < 8; i++) {
    // Modulo over a 32-symbol alphabet from a byte is very slightly biased
    // toward the first 224/256 symbols. That is irrelevant here: this is a
    // collision-avoidance id scoped to one instructing party, not a secret.
    tail += ID_ALPHABET[(entropy[i] ?? 0) % ID_ALPHABET.length];
  }
  return `YML-${y}${m}${d}-${tail}`;
}

/** The same thing, with the browser's randomness. */
export function freshEndToEndId(now: Date = new Date()): string {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  return endToEndId(now, bytes);
}

/** ------------------------------------------------------------------- memo
 *
 * What the ledger can actually be made to carry today.
 *
 * A bank transfer has one free-text field. Putting only the payer's reference
 * in it throws away the purpose code and the end-to-end id, which are the two
 * fields that make a payment reconcilable rather than merely visible. So the
 * memo carries all three in a form that is one line, legible to a person
 * reading raw ledger output, and parseable back into fields by this app.
 *
 *     ym1;e2e=YML-20260826-K3M9QRTB;purp=GDDS;rmt=Invoice 4471
 *
 * `rmt` is last and takes the entire remainder of the string, so a remittance
 * line containing a semicolon survives a round trip. `e2e` and `purp` are
 * generated from fixed alphabets and cannot contain one.
 *
 * A memo that does not begin `ym1;` is treated as remittance information in
 * full, which is what every payment sent by this app before today looks like.
 * Reading old payments as if they were malformed new ones would have made the
 * history screen wrong for exactly the payments that already exist.
 */
export const MEMO_PREFIX = 'ym1;';

/**
 * The chain's `max_memo_characters`, which is the Cosmos SDK default here.
 *
 * Enforced client-side because the alternative is a payment that is signed,
 * broadcast, and refused by the node for a reason the payer cannot see — after
 * they have pressed the button.
 */
export const MEMO_LIMIT = 256;

export interface Remittance {
  /** ISO 20022 EndToEndId. Empty when the memo did not carry one. */
  e2e: string;
  /** ISO 20022 Purpose/Cd. Empty when the memo did not carry one. */
  purpose: string;
  /** ISO 20022 RemittanceInformation/Ustrd — what the payer typed. */
  remittance: string;
  /** False for a plain memo, so a caller can say "no structure" rather than guess. */
  structured: boolean;
}

export function encodeMemo(r: Omit<Remittance, 'structured'>): string {
  const parts: string[] = [];
  if (r.e2e) parts.push(`e2e=${r.e2e}`);
  if (r.purpose) parts.push(`purp=${r.purpose}`);
  // Always last: everything after `rmt=` is the value, semicolons included.
  if (r.remittance) parts.push(`rmt=${r.remittance}`);
  return parts.length === 0 ? '' : MEMO_PREFIX + parts.join(';');
}

export function decodeMemo(memo: string): Remittance {
  if (!memo.startsWith(MEMO_PREFIX)) {
    return { e2e: '', purpose: '', remittance: memo, structured: false };
  }

  const body = memo.slice(MEMO_PREFIX.length);
  const out: Remittance = { e2e: '', purpose: '', remittance: '', structured: true };

  let rest = body;
  while (rest !== '') {
    if (rest.startsWith('rmt=')) { out.remittance = rest.slice(4); break; }
    const cut = rest.indexOf(';');
    const token = cut < 0 ? rest : rest.slice(0, cut);
    rest = cut < 0 ? '' : rest.slice(cut + 1);

    if (token.startsWith('e2e=')) out.e2e = token.slice(4);
    else if (token.startsWith('purp=')) out.purpose = token.slice(5);
    // An unrecognised token is ignored rather than treated as an error: a
    // later version of this app may add a field, and an older build reading
    // its payments should still show the fields it does understand.
  }

  return out;
}

/** ------------------------------------------------------------- instruction
 *
 * The `MsgSendPayment` this payment would be. Field names are the proto's, so
 * anybody comparing this screen against x/paymsg is comparing like with like.
 */
export interface Instruction {
  debtor: string;
  endToEndId: string;
  instructingParticipant: string;
  instructedParticipant: string;
  creditor: string;
  denom: string;
  /** Base units. Never a display figure — see the rule in money.ts. */
  amount: string;
  purposeCode: string;
  remittanceInformation: string;
  settlementJurisdiction: string;
}

export interface Draft {
  /** The payer's address. Empty before an account is connected. */
  debtorAddress: string;
  /** The payer's user id, which is where the settlement jurisdiction comes from. */
  debtorUserId: string;
  /** The payee's address, once resolved. Empty while it is not. */
  creditorAddress: string;
  /** The payee's user id as typed. */
  creditorUserId: string;
  denom: string;
  /** Base units, as a string. */
  amount: string;
  purposeCode: string;
  remittanceInformation: string;
  endToEndId: string;
}

/**
 * The country whose authority settles this, taken from the payer's own user id.
 *
 * Every account on this chain is country-gated and its identifier begins with
 * the ISO 3166-1 alpha-2 code of its perimeter, so the jurisdiction is a fact
 * about the account rather than something to ask the payer for. `ZZ` — the
 * foundation's reserved code — is not a settlement jurisdiction and is
 * reported as empty, because no national authority answers to it.
 */
export function settlementJurisdiction(debtorUserId: string): string {
  const prefix = debtorUserId.replace(/-/g, '').slice(0, 2).toUpperCase();
  if (!/^[A-Z]{2}$/.test(prefix) || prefix === 'ZZ') return '';
  return prefix;
}

export function instructionFor(d: Draft): Instruction {
  return {
    debtor: d.debtorAddress,
    endToEndId: d.endToEndId,
    // Empty, and that is the finding rather than a gap in this function. There
    // is no approved participant on this chain for either side to name.
    instructingParticipant: '',
    instructedParticipant: '',
    creditor: d.creditorAddress,
    denom: d.denom,
    amount: d.amount,
    purposeCode: d.purposeCode,
    remittanceInformation: d.remittanceInformation,
    settlementJurisdiction: settlementJurisdiction(d.debtorUserId),
  };
}

/** ------------------------------------------------------------- where it goes
 *
 * For each field of the instruction, what happens to it on the transfer this
 * app really sends.
 */
export type Carrier =
  /** A field of the transaction itself — sender, recipient, coin. */
  | 'ledger'
  /** Inside the transaction memo, on the ledger and queryable, but free text. */
  | 'memo'
  /** Not sent at all. */
  | 'dropped';

export interface PlannedField {
  /** The ISO 20022 element this corresponds to. */
  iso: string;
  /** The x/paymsg field, so the two can be lined up against the proto. */
  chainField: string;
  /** The value, or empty. */
  value: string;
  carried: Carrier;
  /** Catalogue key explaining a `dropped` field. Null for the others. */
  whyKey: string | null;
}

export function fieldPlan(i: Instruction): PlannedField[] {
  return [
    { iso: 'Dbtr/Id', chainField: 'debtor', value: i.debtor, carried: 'ledger', whyKey: null },
    { iso: 'Cdtr/Id', chainField: 'creditor', value: i.creditor, carried: 'ledger', whyKey: null },
    { iso: 'IntrBkSttlmAmt/Ccy', chainField: 'denom', value: i.denom, carried: 'ledger', whyKey: null },
    { iso: 'IntrBkSttlmAmt', chainField: 'amount', value: i.amount, carried: 'ledger', whyKey: null },
    { iso: 'PmtId/EndToEndId', chainField: 'end_to_end_id', value: i.endToEndId, carried: 'memo', whyKey: null },
    { iso: 'Purp/Cd', chainField: 'purpose_code', value: i.purposeCode, carried: 'memo', whyKey: null },
    {
      iso: 'RmtInf/Ustrd', chainField: 'remittance_information',
      value: i.remittanceInformation, carried: 'memo', whyKey: null,
    },
    {
      iso: 'DbtrAgt/FinInstnId', chainField: 'instructing_participant',
      value: i.instructingParticipant, carried: 'dropped', whyKey: 'iso.whyNoAgent',
    },
    {
      iso: 'CdtrAgt/FinInstnId', chainField: 'instructed_participant',
      value: i.instructedParticipant, carried: 'dropped', whyKey: 'iso.whyNoAgent',
    },
    {
      iso: 'SttlmInf/SttlmCtry', chainField: 'settlement_jurisdiction',
      value: i.settlementJurisdiction, carried: 'dropped', whyKey: 'iso.whyNoRecord',
    },
  ];
}

/** ------------------------------------------------------------- the blocker
 *
 * Why the instruction above is not the message that gets sent.
 *
 * `unknown` is a real answer and is never collapsed into `blocked`. A chain
 * that did not answer is not a chain with zero participants, and showing zero
 * for an unanswered query is how an interface reports a network fault as a
 * fact about the world.
 */
export type Standing =
  | { known: false; whyKey: string }
  | { known: true; participants: ParticipantSummary[]; height: number };

export interface ParticipantSummary {
  address: string;
  code: string;
  name: string;
}

export type Blocker =
  | { kind: 'unknown'; whyKey: string }
  | { kind: 'no-participants' }
  | { kind: 'not-a-customer'; participants: ParticipantSummary[] };

export function blocker(s: Standing): Blocker {
  if (!s.known) return { kind: 'unknown', whyKey: s.whyKey };
  if (s.participants.length === 0) return { kind: 'no-participants' };
  // Participants exist. Whether *this* account is a registered customer of one
  // is a separate question, and the query that answers it sits behind the
  // supervisor credential — so the honest report stops here rather than
  // claiming the path is open.
  return { kind: 'not-a-customer', participants: s.participants };
}
