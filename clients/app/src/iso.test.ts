import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  MEMO_LIMIT,
  MEMO_PREFIX,
  PURPOSES,
  blocker,
  decodeMemo,
  encodeMemo,
  endToEndId,
  fieldPlan,
  instructionFor,
  purposeKey,
  settlementJurisdiction,
  type Draft,
} from './iso.ts';

test('an end-to-end id is deterministic, dated and inside the ISO length limit', () => {
  const bytes = new Uint8Array([0, 1, 2, 3, 30, 31, 32, 255]);
  const id = endToEndId(new Date(Date.UTC(2026, 7, 26)), bytes);

  // 0->'0', 1->'1', 2->'2', 3->'3', 30->'Y', 31->'Z', 32 wraps to '0', 255 -> 255%32=31 -> 'Z'
  assert.equal(id, 'YML-20260826-0123YZ0Z');
  // ISO 20022 EndToEndId is Max35Text. Longer is not truncated by anybody, it
  // is rejected by the receiving scheme.
  assert.ok(id.length <= 35, `${id.length} characters is over the ISO limit`);
});

test('a short entropy buffer still produces a well-formed id rather than undefined', () => {
  const id = endToEndId(new Date(Date.UTC(2026, 0, 1)), new Uint8Array([7]));
  assert.equal(id, 'YML-20260101-70000000');
});

test('the memo carries every field and reads back identically', () => {
  const memo = encodeMemo({
    e2e: 'YML-20260826-0123YZ0Z',
    purpose: 'GDDS',
    remittance: 'Invoice 4471',
  });

  assert.equal(memo, 'ym1;e2e=YML-20260826-0123YZ0Z;purp=GDDS;rmt=Invoice 4471');

  assert.deepEqual(decodeMemo(memo), {
    e2e: 'YML-20260826-0123YZ0Z',
    purpose: 'GDDS',
    remittance: 'Invoice 4471',
    structured: true,
  });
});

test('a remittance line containing a semicolon survives the round trip', () => {
  // The reason `rmt` is last and greedy. A payer pasting an invoice line with
  // punctuation in it must not have half of it silently dropped.
  const text = 'Order 12; part 2 of 3; net 30';
  const memo = encodeMemo({ e2e: 'E1', purpose: 'SUPP', remittance: text });
  assert.equal(decodeMemo(memo).remittance, text);
});

test('an empty payment produces an empty memo rather than a bare prefix', () => {
  assert.equal(encodeMemo({ e2e: '', purpose: '', remittance: '' }), '');
});

test('a plain memo from an older payment is read as remittance information', () => {
  // Every payment this app sent before today has a memo like this one. Reading
  // it as malformed structure would blank the reference on exactly the
  // payments that already exist.
  const old = decodeMemo('Invoice 4471');
  assert.deepEqual(old, { e2e: '', purpose: '', remittance: 'Invoice 4471', structured: false });

  assert.equal(decodeMemo('').remittance, '');
  assert.equal(decodeMemo('').structured, false);
});

test('an unknown token is skipped, not treated as a failure', () => {
  const r = decodeMemo(`${MEMO_PREFIX}e2e=E1;futr=whatever;purp=RENT;rmt=Rent`);
  assert.equal(r.e2e, 'E1');
  assert.equal(r.purpose, 'RENT');
  assert.equal(r.remittance, 'Rent');
});

test('a full memo fits inside the chain limit', () => {
  // The reference field is capped at 64 characters in the form. With the
  // longest id and a four-letter code that has to leave room to spare, or the
  // node refuses a transaction the payer has already signed.
  const memo = encodeMemo({
    e2e: 'YML-20260826-0123YZ0Z',
    purpose: 'GDDS',
    remittance: 'x'.repeat(64),
  });
  assert.ok(memo.length <= MEMO_LIMIT, `${memo.length} characters exceeds ${MEMO_LIMIT}`);
});

test('the settlement jurisdiction comes from the payer own user id', () => {
  assert.equal(settlementJurisdiction('NG-K3M9-7QRT-B'), 'NG');
  assert.equal(settlementJurisdiction('ngk3m97qrtb'), 'NG');
  // ZZ is the foundation's reserved code. No national authority answers to it,
  // so it is not a settlement jurisdiction.
  assert.equal(settlementJurisdiction('ZZ-K3M9-7QRT-B'), '');
  assert.equal(settlementJurisdiction(''), '');
  assert.equal(settlementJurisdiction('7X-K3M9'), '');
});

const DRAFT: Draft = {
  debtorAddress: 'yml1debtor',
  debtorUserId: 'GH-K3M9-7QRT-B',
  creditorAddress: 'yml1creditor',
  creditorUserId: 'NG-AAAA-BBBB-C',
  denom: 'ughs',
  amount: '125050',
  purposeCode: 'GDDS',
  remittanceInformation: 'Invoice 4471',
  endToEndId: 'YML-20260826-0123YZ0Z',
};

test('the instruction names no agent, because there is none to name', () => {
  const i = instructionFor(DRAFT);
  assert.equal(i.instructingParticipant, '');
  assert.equal(i.instructedParticipant, '');
  assert.equal(i.settlementJurisdiction, 'GH');
  assert.equal(i.amount, '125050');
});

test('the field plan says which fields survive the transfer and which do not', () => {
  const plan = fieldPlan(instructionFor(DRAFT));

  const carried = Object.fromEntries(plan.map((f) => [f.chainField, f.carried]));
  assert.equal(carried['debtor'], 'ledger');
  assert.equal(carried['creditor'], 'ledger');
  assert.equal(carried['amount'], 'ledger');
  assert.equal(carried['denom'], 'ledger');
  assert.equal(carried['end_to_end_id'], 'memo');
  assert.equal(carried['purpose_code'], 'memo');
  assert.equal(carried['remittance_information'], 'memo');
  assert.equal(carried['instructing_participant'], 'dropped');
  assert.equal(carried['instructed_participant'], 'dropped');
  assert.equal(carried['settlement_jurisdiction'], 'dropped');

  // Every dropped field explains itself. A table with a blank reason is a table
  // that makes somebody go and ask.
  for (const f of plan) {
    if (f.carried === 'dropped') assert.ok(f.whyKey, `${f.chainField} is dropped with no reason`);
    else assert.equal(f.whyKey, null);
  }
});

test('an empty jurisdiction is explained by the account, not by the record', () => {
  const jurisdiction = (p: ReturnType<typeof fieldPlan>) =>
    p.find((f) => f.chainField === 'settlement_jurisdiction')!.whyKey;

  // A placed account: the field is dropped only because no payment record is
  // written, which is a fact about the chain rather than about them.
  assert.equal(jurisdiction(fieldPlan(instructionFor(DRAFT))), 'iso.whyNoRecord');

  // An unplaced one: the reason they can act on is that nobody has recorded
  // their jurisdiction, which is also why the chain has issued them no
  // identifier and why nobody can pay them.
  const unplaced = fieldPlan(instructionFor({ ...DRAFT, debtorUserId: '' }));
  assert.equal(jurisdiction(unplaced), 'iso.whyNoJurisdiction');
});

test('every field the memo carries is actually in the memo', () => {
  // The plan and the encoder have to agree. If a field is marked `memo` here
  // and the encoder does not write it, the screen claims something false about
  // where somebody's reference went.
  const i = instructionFor(DRAFT);
  const memo = encodeMemo({
    e2e: i.endToEndId,
    purpose: i.purposeCode,
    remittance: i.remittanceInformation,
  });

  for (const f of fieldPlan(i)) {
    if (f.carried === 'memo' && f.value !== '') {
      assert.ok(memo.includes(f.value), `${f.chainField} is claimed to travel in the memo but is not in it`);
    }
  }
});

test('a chain that did not answer is unknown, never zero', () => {
  const b = blocker({ known: false, whyKey: 'iso.standingUnreachable' });
  assert.equal(b.kind, 'unknown');
  // The bug this guards: reporting an unanswered query as "no participants
  // exist", which is a claim about the world drawn from a network fault.
  assert.notEqual(b.kind, 'no-participants');
});

test('an empty participant list is a finding, and a populated one moves the question on', () => {
  assert.deepEqual(blocker({ known: true, participants: [], height: 80907 }), { kind: 'no-participants' });

  const some = blocker({
    known: true,
    height: 1,
    participants: [{ address: 'yml1bank', code: 'GHBANK001', name: 'Bank Co Ghana' }],
  });
  assert.equal(some.kind, 'not-a-customer');
});

test('every purpose code is four letters from the ISO list and has a label', () => {
  for (const p of PURPOSES) {
    assert.match(p.code, /^[A-Z]{4}$/, `${p.code} is not an ISO 20022 four-letter code`);
    assert.equal(purposeKey(p.code), p.key);
  }
  assert.equal(purposeKey('NOPE'), null);
  // Duplicates would put two identical options in the select.
  assert.equal(new Set(PURPOSES.map((p) => p.code)).size, PURPOSES.length);
});
