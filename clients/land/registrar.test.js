// Tests for the console's judgement about who may sign what.
//
// The thing worth testing here is not that a command string comes out with the
// right flags in it. It is that this module REFUSES to compose a signature it
// has no business composing — because the security model of x/land is that a
// registry office is an M-of-N group account, and a page that could produce an
// office's signature on its own would be a way around the whole module.
//
// So the bulk of what follows is refusals, and assertions about which of the
// three shapes each of the twelve messages takes. Those are the tests that
// would still fail if somebody "improved" this console by adding a Freeze
// button, or by letting an office message be signed from a browser because it
// was easier than composing a proposal.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ACTIONS,
  ACTION_NAMES,
  CHAIN,
  CODESPACE,
  GAS,
  MESSAGES,
  REFUSALS,
  actionsBy,
  admissionCommand,
  admissionProposal,
  explainRefusal,
  groupPreamble,
  landTx,
  officeProposal,
  officeTx,
  personalMessage,
  proposable,
  proposalMetadata,
  sh,
  signable,
} from './registrar.js';

import { GROUP_SUBMIT_PROPOSAL, read, str, strs, u64 } from './proto.js';

// Real addresses off this chain: a key and a 32-byte derived account, so the
// group-account shape in these tests is the shape a group account actually has.
const REGISTRAR = 'yml1n54q7l9ll4atcdhlcxqv0tw4qzdh6ew2h04ged';
const OFFICE = 'yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj';
const CITIZEN = 'yml1rpzxcl2t3g4y0nrzncxxj7yyccm04la2jwne84';
// x/gov's module account: the only authority MsgRegisterAuthority is accepted
// from. A proposal naming anything else passes its vote and then fails to run.
const GOV = 'yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386';

/* ================================================================ coverage */

test('the console covers all twelve of x/land’s messages', () => {
  // Twelve. The console used to compose two of them as structured messages and
  // hand the rest over as strings to copy, which is what this file exists to
  // stop happening again silently.
  assert.equal(ACTION_NAMES.length, 12);
  assert.deepEqual([...ACTION_NAMES].sort(), [
    'AttachDeed', 'AttestTransfer', 'AuthoriseFractionalisation',
    'CompleteTransfer', 'FreezeParcel', 'Object', 'ProposeTransfer',
    'RecordEncumbrance', 'RegisterAuthority', 'RegisterParcel',
    'SetRestriction', 'ValidateTransfer',
  ]);
});

test('every action states who may sign it, why it takes its shape, and what undoes it', () => {
  // A control whose form is unexplained reads as an arbitrary restriction, and
  // an irreversible one presented without saying so is the failure this whole
  // console is written against.
  for (const name of ACTION_NAMES) {
    const a = ACTIONS[name];
    assert.ok(a.who && a.who.length > 8, `${name} must say who signs it`);
    assert.ok(a.why && a.why.length > 40, `${name} must say why it takes its shape`);
    assert.equal(typeof a.undo, 'string', `${name} must say what can be undone`);
    assert.ok(['sign', 'propose', 'command', 'governance'].includes(a.how),
      `${name} has an unknown shape ${a.how}`);
  }
});

test('an action with no undo says out loud that it is terminal', () => {
  // Object stops a transfer for good and CompleteTransfer moves title. Both are
  // signed in a browser, which makes the sentence before the signature the only
  // protection there is.
  for (const name of ACTION_NAMES) {
    if (ACTIONS[name].undo === '') {
      assert.ok(ACTIONS[name].terminal && ACTIONS[name].terminal.length > 60,
        `${name} is irreversible and must say what it does before it is signed`);
    }
  }
  assert.deepEqual(ACTION_NAMES.filter((n) => ACTIONS[n].undo === '').sort(),
    ['CompleteTransfer', 'Object']);
});

/* ============================================== the three shapes, asserted */

test('exactly three messages are signed in the browser, and they are the personal ones', () => {
  // Object: open to anybody, no standing. ProposeTransfer: the holder's own
  // land. CompleteTransfer: mechanical and open to anybody. Every one of those
  // is one person with one key, which is the only case a browser can serve
  // honestly.
  assert.deepEqual(actionsBy('sign').sort(),
    ['CompleteTransfer', 'Object', 'ProposeTransfer']);
});

test('every office message is a proposal, never a signature', () => {
  // The keeper resolves each of these through activeAuthority, and
  // RegisterAuthority refuses to admit an office that is not a group account
  // (ErrOfficeNotGroup). So the signer of all of them is a group policy
  // address, which no browser holds.
  assert.deepEqual(actionsBy('propose').sort(), [
    'AttachDeed', 'AttestTransfer', 'AuthoriseFractionalisation',
    'RecordEncumbrance', 'RegisterParcel', 'SetRestriction', 'ValidateTransfer',
  ]);
});

test('the freeze is the one office message this console will not start', () => {
  // Deliberate, and not because the chain forbids it — a freeze proposal would
  // work exactly like the other seven. See the reasoning at the top of
  // registrar.js: the office's vote is unchanged either way, and what a button
  // would lower is the cost of the first move.
  assert.deepEqual(actionsBy('command'), ['FreezeParcel']);
  assert.equal(proposable('FreezeParcel'), false);
  assert.equal(signable('FreezeParcel'), false);
});

test('admitting an office is governance, and is not offered as a form', () => {
  assert.deepEqual(actionsBy('governance'), ['RegisterAuthority']);
  assert.equal(ACTIONS.RegisterAuthority.sub, null,
    'autocli skips it; there is no tx subcommand to compose');
});

test('the four shapes account for all twelve and overlap nowhere', () => {
  const counted = ['sign', 'propose', 'command', 'governance']
    .flatMap((how) => actionsBy(how));
  assert.equal(counted.length, 12);
  assert.equal(new Set(counted).size, 12);
});

/* ================================================= the refusals that matter */

test('the browser refuses to sign an office message as a person', () => {
  // The enforcement of the whole argument. Without this the rule lives only in
  // a comment, and the first screen written in a hurry breaks it.
  for (const name of actionsBy('propose')) {
    assert.throws(() => personalMessage(name, { creator: REGISTRAR }),
      /not signed by a person/, `${name} must not be personally signable`);
  }
  assert.throws(() => personalMessage('FreezeParcel', { creator: OFFICE }),
    /not signed by a person/);
  assert.throws(() => personalMessage('RegisterAuthority', {}), /not signed by a person/);
});

test('the browser refuses to propose a message that is nobody’s office act', () => {
  assert.throws(() => officeProposal('Object',
    { office: OFFICE, proposer: REGISTRAR, fields: {} }), /not an office proposal/);
  assert.throws(() => officeProposal('RegisterAuthority',
    { office: OFFICE, proposer: REGISTRAR, fields: {} }), /not an office proposal/);
  assert.throws(() => officeProposal('FreezeParcel',
    { office: OFFICE, proposer: REGISTRAR, fields: {} }), /not an office proposal/);
});

test('an unknown action is refused rather than treated as signable', () => {
  assert.throws(() => signable('MsgSeizeLand'), /no such land action/);
  assert.throws(() => proposable('Nonsense'), /no such land action/);
});

/* ============================================== what the browser does sign */

test('an objection is signed by the objector and carries their reason', () => {
  const packed = personalMessage('Object', {
    creator: CITIZEN, transfer_id: 12,
    reason: 'my late father’s estate has not been distributed',
  });
  const f = read(packed);
  assert.equal(str(f, 1), '/blockchain.land.v1.MsgObject');
  const msg = read(f.get(2)[0]);
  assert.equal(str(msg, 1), CITIZEN);
  assert.equal(u64(msg, 2), '12');
  assert.ok(str(msg, 3).includes('estate'));
});

test('an objection to transfer 0 is composable', () => {
  // Transfer ids come off collections.Sequence with no zero skipped, unlike
  // parcel ids, so the first transfer this registry records is transfer 0. A
  // console that treated 0 as "unset" would refuse to let anyone object to it.
  const msg = read(read(personalMessage('Object',
    { creator: CITIZEN, transfer_id: 0, reason: 'not his to sell' })).get(2)[0]);
  assert.equal(u64(msg, 2), '0');
  assert.equal(str(msg, 1), CITIZEN);
});

test('a completion names whoever is sending it, which may be a stranger', () => {
  const msg = read(read(personalMessage('CompleteTransfer',
    { creator: CITIZEN, transfer_id: 3 })).get(2)[0]);
  assert.equal(str(msg, 1), CITIZEN);
});

/* ============================================ what the browser proposes */

test('a proposal names the office as the message’s creator and the registrar as proposer', () => {
  // The mistake this guards: swapping them. A proposal proposed by the office
  // is one no registrar can sign; a land message whose creator is a registrar
  // is refused by the keeper AFTER the office has voted on it, which wastes
  // the office's quorum on something that could never have worked.
  const { typeUrl, bytes } = officeProposal('ValidateTransfer', {
    office: OFFICE, proposer: REGISTRAR, fields: { transfer_id: 12 },
  });
  assert.equal(typeUrl, GROUP_SUBMIT_PROPOSAL);

  const f = read(bytes);
  assert.equal(str(f, 1), OFFICE, 'the policy is the office');
  assert.deepEqual(strs(f, 2), [REGISTRAR], 'the proposer is the registrar');

  const inner = read(f.get(4)[0]);
  assert.equal(str(inner, 1), '/blockchain.land.v1.MsgValidateTransfer');
  const land = read(inner.get(2)[0]);
  assert.equal(str(land, 1), OFFICE, 'the land message is created by the office');
  assert.equal(u64(land, 2), '12');
});

test('a creator passed in by a caller cannot override the office', () => {
  // A screen that filled `creator` from a connected wallet would compose a
  // proposal the office votes on and the keeper then refuses.
  const f = read(officeProposal('RegisterParcel', {
    office: OFFICE, proposer: REGISTRAR,
    fields: { creator: REGISTRAR, geometry_hash: 'abc', cadastral_ref: 'X/1', holder: CITIZEN },
  }).bytes);
  const land = read(read(f.get(4)[0]).get(2)[0]);
  assert.equal(str(land, 1), OFFICE);
});

test('a proposal without an office or a proposer is refused', () => {
  assert.throws(() => officeProposal('AttachDeed',
    { proposer: REGISTRAR, fields: {} }), /needs the office/);
  assert.throws(() => officeProposal('AttachDeed',
    { office: OFFICE, fields: {} }), /needs the registrar/);
});

test('proposal metadata says what the office is being asked to do, and fits', () => {
  assert.equal(proposalMetadata('ValidateTransfer', 'transfer 12, parcel YM-KIN-2024-00187'),
    'Confirm the seller against the paper file — transfer 12, parcel YM-KIN-2024-00187');
  const long = proposalMetadata('SetRestriction', 'x'.repeat(500));
  assert.ok(long.length <= 255);
  assert.ok(long.endsWith('…'));
});

/* ================================================ the two composed by hand */

test('a freeze command carries the grounds and the office’s two steps', () => {
  const cmd = officeTx({
    sub: 'freeze-parcel',
    args: ['7', 'court order 2026/114, fraud inquiry'],
    from: 'registry-kinshasa',
  });
  assert.ok(cmd.includes('tx land freeze-parcel'));
  assert.ok(cmd.includes("'court order 2026/114, fraud inquiry'"), 'the grounds are quoted');
  assert.ok(cmd.includes('--generate-only'), 'an office cannot broadcast');
  assert.ok(cmd.includes('tx group submit-proposal'), 'the office’s own M-of-N comes second');
});

test('an admission proposal names x/gov as the authority, not a person', () => {
  // The chain compares the message's authority against its own gov module
  // account and refuses anything else. A proposal naming the proposer passes
  // its vote and then fails to execute, which is the worst of both.
  const doc = JSON.parse(admissionProposal({
    govAuthority: GOV, office: OFFICE, name: 'Kinshasa Lands Office',
    jurisdiction: 'cd', title: 'Admit the Kinshasa Lands Office', summary: 'A new office.',
  }));
  assert.equal(doc.messages[0]['@type'], '/blockchain.land.v1.MsgRegisterAuthority');
  assert.equal(doc.messages[0].authority, GOV);
  assert.equal(doc.messages[0].office, OFFICE);
  assert.equal(doc.messages[0].jurisdiction, 'CD', 'normalised, as the keeper does');
  assert.equal(doc.messages[0].active, true);
  assert.ok(admissionCommand(REGISTRAR).includes('tx gov submit-proposal'));
});

/* ============================================================ plumbing */

test('the type URL prefix is blockchain, not yamale.blockchain', () => {
  for (const url of Object.values(MESSAGES)) {
    assert.ok(url.startsWith('/blockchain.land.v1.'), url);
    assert.ok(!url.includes('yamale'), url);
  }
});

test('a command quotes only what needs quoting', () => {
  assert.equal(sh('YM-KIN-2024-00187'), 'YM-KIN-2024-00187');
  assert.equal(sh('ACC/GA/2019/00412'), 'ACC/GA/2019/00412');
  assert.equal(sh('4200 USD'), "'4200 USD'");
  assert.equal(sh("it's his"), "'it'\\''s his'");
  assert.equal(sh(''), "''");
});

test('a composed command wraps rather than running off the panel', () => {
  const cmd = landTx({
    sub: 'register-parcel',
    args: ['9f2b1c'.repeat(10), 'ACC/GA/2019/00412', CITIZEN],
    from: 'registry-kinshasa',
  });
  for (const line of cmd.split('\n')) {
    assert.ok(line.length <= 82, `line too long to read on a phone: ${line.length}`);
  }
  assert.ok(cmd.split('\n').slice(0, -1).every((l) => l.endsWith('\\')));
});

test('the group preamble names the office rather than saying “the office”', () => {
  assert.ok(groupPreamble('Kinshasa Lands Office').startsWith('Kinshasa Lands Office is a group'));
});

test('gas is set and the chain is named in one place', () => {
  assert.equal(CHAIN.id, 'yamale-devnet-2');
  assert.ok(GAS >= 200000);
});

/* ============================================================= refusals */

test('a land refusal is matched exactly on codespace and code', () => {
  const r = explainRefusal({ codespace: 'land', code: 23, log: 'failed to execute message' });
  assert.equal(r.known, true);
  assert.ok(r.says.includes('must give a reason'));
  assert.equal(r.code, 23);
  assert.equal(r.raw, 'failed to execute message');
});

test('a refusal with only a log line is matched on the chain’s own wording', () => {
  // A CheckTx rejection carries the log and not always the pair.
  const r = explainRefusal({
    log: 'failed to execute message; message index: 0: this ground is already titled: parcel 4',
  });
  assert.equal(r.known, true);
  assert.equal(r.code, 6);
  assert.ok(r.says.includes('owned twice'));
});

test('the same code in another module is not read as a land refusal', () => {
  // Code 23 in x/bank is not "an objection must give a reason".
  const r = explainRefusal({ codespace: 'bank', code: 23, log: 'something else entirely' });
  assert.equal(r.known, false);
  assert.equal(r.says, '');
});

test('an unknown refusal is reported as unknown, never smoothed over', () => {
  const r = explainRefusal({ codespace: 'sdk', code: 99, log: 'out of gas' });
  assert.equal(r.known, false);
  assert.equal(r.raw, 'out of gas');
});

test('the two refusals that are not about the land are named as such', () => {
  assert.ok(explainRefusal({ log: 'account sequence mismatch, expected 4, got 3' })
    .says.includes('Nothing was recorded'));
  assert.ok(explainRefusal({ log: 'yml1abc: key not found' })
    .says.includes('never seen that account'));
});

test('every refusal carries a sentence and the chain’s own wording', () => {
  assert.equal(CODESPACE, 'land');
  for (const [code, entry] of Object.entries(REFUSALS)) {
    assert.ok(entry.text.length > 5, `${code} must carry the chain's wording`);
    assert.ok(entry.says.length > 20, `${code} must say what it means for the reader`);
  }
  // Every code x/land registers, from 1 to 35, with none skipped silently.
  const codes = Object.keys(REFUSALS).map(Number).sort((a, b) => a - b);
  assert.equal(codes[0], 1);
  assert.equal(codes[codes.length - 1], 35);
  assert.equal(codes.length, 35);
});
