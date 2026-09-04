// Tests for the appointment console's judgement.
//
// The thing worth testing here is not that a JSON document comes out with the
// right keys in it. It is that this module REFUSES in the cases where composing
// something plausible would be worse than composing nothing — because
// MsgUpdateParams replaces the whole Params object, so every one of those cases
// is a proposal that passes and silently changes a parameter nobody voted on.
//
// So the bulk of what follows is refusals: an unreadable params object, a
// payload_length that came back as zero, a field this build has never heard of, a
// removal that would change nothing. Those are the tests that would still fail if
// somebody "simplified" this module by adding a default.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ADDRESS_PREFIX,
  KNOWN_PARAM_FIELDS,
  MAX_FOUNDATION_ADMINISTRATORS,
  MAX_PAYLOAD_LENGTH,
  MAX_SUMMARY_LENGTH,
  MAX_TITLE_LENGTH,
  MIN_PAYLOAD_LENGTH,
  ParamsUnreadable,
  UPDATE_PARAMS_TYPE,
  afterSubmitting,
  changeSummary,
  checkAddress,
  couldBeGroupAccount,
  decodeBech32,
  paramsDiff,
  paramsDigest,
  planChange,
  proposalDocument,
  readAliasParams,
  submitCommand,
  updateParamsMessage,
} from './administrators.js';

// Real addresses off this chain: three 20-byte keys and two 32-byte derived
// accounts. Real ones rather than invented, because every one of them has to pass
// a bech32 checksum and an invented string does not.
const KEY_A = 'yml1n54q7l9ll4atcdhlcxqv0tw4qzdh6ew2h04ged';
const KEY_B = 'yml1rpzxcl2t3g4y0nrzncxxj7yyccm04la2jwne84';
const KEY_C = 'yml16h07xutkege53xcjaas6s8pnkt3kvruq5sufet';
const KEY_D = 'yml1kappryh2vaf78vd474pzj3zkpg04vwx2zt5f7s';
const KEY_E = 'yml1yd94ndw74k3ku9uuqf5u83rxusgtvdl0t5fsj5';
const KEY_F = 'yml1nls726x7n7ucd8a6ku0ykdp20c0dl6de5zd782';
const KEY_G = 'yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas';
const KEY_H = 'yml1sy3fxls3xcg9y3n6xm3yczznf3grcae7mtjk5g';
const KEY_I = 'yml14v6gumccm63wvlr8qrhmw4keakkekj8r45ldhq';

const GROUP_1 = 'yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj';

// The governance module account: sha256("gov") truncated to twenty bytes, in this
// chain's prefix. The real value, checked against the live chain, because a test
// that asserted against an invented one would pass while the page composed a
// proposal naming an authority x/alias refuses.
const GOV = 'yml10d07y265gmmuvt4z0w9aw880jnsr700jz5s386';

const params = (payloadLength, administrators = []) => ({ payloadLength, administrators });

// -------------------------------------------------------------------- bech32

test('a real address decodes to its prefix and byte length', () => {
  assert.deepEqual(decodeBech32(KEY_A), { prefix: ADDRESS_PREFIX, bytes: 20 });
  assert.deepEqual(decodeBech32(GROUP_1), { prefix: ADDRESS_PREFIX, bytes: 32 });
});

test('a single mistyped character fails the checksum', () => {
  // The chain does not check these at all — Params.Validate() refuses an empty
  // string, a duplicate and a ninth entry and never asks whether an entry is an
  // address. So this check is the only thing between a typo and a governance
  // vote that appoints nobody while reading as an appointment.
  const corrupted = `${KEY_A.slice(0, -1)}e`;
  assert.notEqual(corrupted, KEY_A);
  const checked = checkAddress(corrupted);
  assert.equal(checked.ok, false);
  assert.match(checked.reason, /checksum/);
});

test('a transposition fails the checksum', () => {
  const chars = [...KEY_A];
  [chars[10], chars[11]] = [chars[11], chars[10]];
  const swapped = chars.join('');
  assert.notEqual(swapped, KEY_A);
  assert.equal(checkAddress(swapped).ok, false);
});

test('a valid address on another chain is refused by prefix, not accepted', () => {
  const checked = checkAddress('cosmos1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu');
  assert.equal(checked.ok, false);
  assert.match(checked.reason, /cosmos/);
  assert.match(checked.reason, /other chain/);
});

test('bech32 rejects the characters bech32 does not have', () => {
  // 1, b, i and o are excluded from the alphabet precisely because they are the
  // ones a person transcribing by hand gets wrong.
  assert.equal(checkAddress('yml1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb').ok, false);
  assert.equal(checkAddress('').ok, false);
  assert.equal(checkAddress('yml1').ok, false);
  assert.equal(checkAddress('notbech32').ok, false);
});

test('bech32 rejects mixed case and surrounding whitespace rather than tidying them', () => {
  assert.equal(checkAddress(`${KEY_A.toUpperCase().slice(0, 10)}${KEY_A.slice(10)}`).ok, false);
  const padded = checkAddress(` ${KEY_A} `);
  assert.equal(padded.ok, false);
  assert.match(padded.reason, /whitespace/);
});

test('a 20-byte address cannot be a group policy and a 32-byte one might be', () => {
  assert.equal(couldBeGroupAccount(KEY_A), false);
  assert.equal(couldBeGroupAccount(GROUP_1), true);
  assert.equal(couldBeGroupAccount('nonsense'), false);
});

// ------------------------------------------------------- reading the parameters

test('the current parameters are read out of the node\'s answer', () => {
  const read = readAliasParams({ params: { payload_length: 8, foundation_administrators: [KEY_A] } });
  assert.deepEqual(read, { payloadLength: 8, administrators: [KEY_A], listAbsent: false });
});

test('payload_length is accepted as a string, because the gateway has sent both', () => {
  assert.equal(readAliasParams({ params: { payload_length: '12', foundation_administrators: [] } }).payloadLength, 12);
});

test('an absent administrator list reads as empty and says so', () => {
  // A repeated field has no third state: absent and empty are the same value on
  // the wire. So this is read rather than refused — but flagged, so the page can
  // say the chain returned no list instead of implying it displayed one.
  const read = readAliasParams({ params: { payload_length: 8 } });
  assert.deepEqual(read.administrators, []);
  assert.equal(read.listAbsent, true);
});

test('a payload_length of zero is refused, never defaulted', () => {
  // The whole module turns on this one. Proto3 cannot tell a zero from a field
  // nobody filled in, so a zero means the value is UNKNOWN — and defaulting it
  // to 8 would compose a proposal that reset the identifier length of a chain
  // that had raised it, showing no change in the diff.
  //
  // The MESSAGE is asserted, not just the throw, and a mutation pass is why. With
  // the unreadable-value branch deleted, every one of these still threw — because
  // the bounds check below it coerces null to 0 and refuses that too — so the
  // test passed while the explanation had become "payload_length reads as null,
  // and the chain accepts 8 to 16". That is a real regression in the one thing
  // this module produces. An operator told their node sent a value out of range
  // will go looking for a chain misconfiguration; an operator told the value is
  // unknown and must not be guessed will re-read the parameters. And the first
  // message positively invites somebody to "fix" it by supplying a default.
  for (const value of [0, '0', undefined, null, 'eight', 8.5, {}, []]) {
    let err = null;
    try {
      readAliasParams({ params: { payload_length: value, foundation_administrators: [] } });
    } catch (e) {
      err = e;
    }
    assert.ok(
      err instanceof ParamsUnreadable,
      `payload_length ${JSON.stringify(value)} should have been refused`,
    );
    assert.match(
      err.message,
      /proto3 cannot tell a zero from a field nobody filled in/,
      `payload_length ${JSON.stringify(value)} was refused for the wrong reason: ${err.message}`,
    );
    assert.match(err.message, /not defaulted here on purpose/);
  }
});

test('a payload_length outside the chain\'s bounds is refused rather than corrected', () => {
  // The other branch, pinned by its own message so the two are distinguishable.
  // A value the chain would refuse and a value this page could not read are
  // different problems with different responses, and a single "it threw"
  // assertion cannot tell them apart.
  for (const value of [MIN_PAYLOAD_LENGTH - 1, MAX_PAYLOAD_LENGTH + 1, 1, 99]) {
    let err = null;
    try {
      readAliasParams({ params: { payload_length: value } });
    } catch (e) {
      err = e;
    }
    assert.ok(err instanceof ParamsUnreadable, `payload_length ${value} should have been refused`);
    assert.match(err.message, /and the chain accepts 8 to 16/);
    assert.doesNotMatch(err.message, /proto3/);
  }
  assert.equal(readAliasParams({ params: { payload_length: MIN_PAYLOAD_LENGTH } }).payloadLength, MIN_PAYLOAD_LENGTH);
  assert.equal(readAliasParams({ params: { payload_length: MAX_PAYLOAD_LENGTH } }).payloadLength, MAX_PAYLOAD_LENGTH);
});

test('a params object with a field this build does not know is refused', () => {
  // The check that will fire years from now, when somebody adds a third
  // parameter to x/alias and nobody remembers this page composes the message
  // that replaces all of them. Without it, this page would compose a proposal
  // setting the new field to its protobuf default and show a diff that never
  // mentioned it.
  let err = null;
  try {
    readAliasParams({
      params: { payload_length: 8, foundation_administrators: [], check_character_alphabet: 'crockford' },
    });
  } catch (e) {
    err = e;
  }
  assert.ok(err instanceof ParamsUnreadable, 'an unknown parameter should be refused');
  assert.match(err.message, /check_character_alphabet/);
  assert.match(err.message, /replaces the whole Params object/);
});

test('the known-field list is exactly the proto\'s fields', () => {
  // A guard on the guard: if somebody adds a name here without adding the field
  // to the composed message, the unknown-field check stops firing and the
  // silent-zero returns.
  assert.deepEqual([...KNOWN_PARAM_FIELDS].sort(), ['foundation_administrators', 'payload_length']);
});

test('an answer with no params object at all is refused', () => {
  for (const body of [null, undefined, [], 'params', {}, { params: null }, { params: [] }, { params: 'x' }]) {
    assert.throws(() => readAliasParams(body), ParamsUnreadable);
  }
});

test('an administrator list that is not a list of strings is refused', () => {
  assert.throws(
    () => readAliasParams({ params: { payload_length: 8, foundation_administrators: 'yml1…' } }),
    ParamsUnreadable,
  );
  assert.throws(
    () => readAliasParams({ params: { payload_length: 8, foundation_administrators: [KEY_A, 7] } }),
    ParamsUnreadable,
  );
});

test('the digest moves when either parameter moves, and not otherwise', () => {
  const base = params(8, [KEY_A]);
  assert.equal(paramsDigest(base), paramsDigest(params(8, [KEY_A])));
  assert.notEqual(paramsDigest(base), paramsDigest(params(9, [KEY_A])));
  assert.notEqual(paramsDigest(base), paramsDigest(params(8, [KEY_A, KEY_B])));
  assert.notEqual(paramsDigest(base), paramsDigest(params(8, [])));
  // Order matters, because the on-chain list is ordered and a proposal that
  // reordered it would be a change.
  assert.notEqual(paramsDigest(params(8, [KEY_A, KEY_B])), paramsDigest(params(8, [KEY_B, KEY_A])));
});

// ------------------------------------------------------------ planning a change

test('appointing the first administrator produces the whole object, before and after', () => {
  const plan = planChange({ params: params(8, []), add: GROUP_1, groupLookup: 'group' });
  assert.deepEqual(plan.problems, []);
  assert.deepEqual(plan.before, { payloadLength: 8, administrators: [] });
  assert.deepEqual(plan.after, { payloadLength: 8, administrators: [GROUP_1] });
  assert.equal(plan.action, 'appoint');
});

test('an appointment carries every existing administrator across', () => {
  // The failure this whole interface exists to prevent: composed by hand, the
  // proposal that appoints one administrator drops the others.
  const existing = [KEY_A, KEY_B, KEY_C];
  const plan = planChange({ params: params(12, existing), add: GROUP_1, groupLookup: 'group' });
  assert.deepEqual(plan.problems, []);
  assert.equal(plan.after.administrators.length, 4);
  for (const a of existing) assert.ok(plan.after.administrators.includes(a), `${a} was dropped`);
  // And payload_length is carried, not reset to the default.
  assert.equal(plan.after.payloadLength, 12);
});

test('the resulting list is sorted, so it depends on the set and not the typing order', () => {
  const one = planChange({ params: params(8, [KEY_B]), add: KEY_A, groupLookup: 'group' });
  const two = planChange({ params: params(8, [KEY_A]), add: KEY_B, groupLookup: 'group' });
  assert.deepEqual(one.after.administrators, two.after.administrators);
});

test('a duplicate is refused, because the chain refuses it after the vote', () => {
  const plan = planChange({ params: params(8, [KEY_A]), add: KEY_A, groupLookup: 'group' });
  assert.equal(plan.after, null);
  assert.equal(plan.problems.length, 1);
  assert.match(plan.problems[0], /already a foundation administrator/);
});

test('the ninth administrator is refused at the cap', () => {
  const eight = [KEY_A, KEY_B, KEY_C, KEY_D, KEY_E, KEY_F, KEY_G, KEY_H];
  assert.equal(eight.length, MAX_FOUNDATION_ADMINISTRATORS);
  const plan = planChange({ params: params(8, eight), add: KEY_I, groupLookup: 'group' });
  assert.equal(plan.after, null);
  assert.match(plan.problems.join(' '), /caps the list at 8/);

  // And the eighth is allowed, so the cap is the cap and not one less.
  const seven = eight.slice(0, 7);
  const ok = planChange({ params: params(8, seven), add: KEY_I, groupLookup: 'group' });
  assert.deepEqual(ok.problems, []);
  assert.equal(ok.after.administrators.length, MAX_FOUNDATION_ADMINISTRATORS);
});

test('a mistyped address is refused, and the refusal says the chain would not catch it', () => {
  const plan = planChange({ params: params(8, []), add: `${KEY_A.slice(0, -1)}e` });
  assert.equal(plan.after, null);
  assert.match(plan.problems[0], /checksum/);
  assert.match(plan.problems[0], /never checks that an entry is an address/);
});

test('a single key is a warning and not a refusal, and says the chain will accept it', () => {
  // The chain does not enforce this: unlike MsgGrantRole, which refuses a holder
  // that is not a group account, an administrator is matched by address equality
  // with no check on the kind of account. So the interface warns and proceeds,
  // and says plainly that nothing downstream will stop it.
  const plan = planChange({ params: params(8, []), add: KEY_A, groupLookup: 'not-group' });
  assert.deepEqual(plan.problems, []);
  assert.deepEqual(plan.after.administrators, [KEY_A]);
  assert.equal(plan.warnings.length, 1);
  assert.match(plan.warnings[0], /THE CHAIN WILL ACCEPT IT/);
  assert.match(plan.warnings[0], /one bribe/);
});

test('a group account with x/group confirming it draws no warning', () => {
  const plan = planChange({ params: params(8, []), add: GROUP_1, groupLookup: 'group' });
  assert.deepEqual(plan.warnings, []);
});

test('when x/group cannot be reached, the length still rules a group policy out', () => {
  const key = planChange({ params: params(8, []), add: KEY_A, groupLookup: 'unknown' });
  assert.match(key.warnings.join(' '), /single public key/);

  const derived = planChange({ params: params(8, []), add: GROUP_1, groupLookup: 'unknown' });
  assert.match(derived.warnings.join(' '), /could not be reached/);
  // And it does not claim the length proves anything, because a module account is
  // 32 bytes too.
  assert.match(derived.warnings.join(' '), /not proof/);
});

test('removing an administrator keeps the others and the payload length', () => {
  const plan = planChange({ params: params(14, [KEY_A, KEY_B, KEY_C]), remove: KEY_B });
  assert.deepEqual(plan.problems, []);
  assert.deepEqual(plan.after, { payloadLength: 14, administrators: [KEY_A, KEY_C] });
  assert.equal(plan.action, 'remove');
});

test('removing an address that is not on the list is refused, not composed as a no-op', () => {
  // "Nothing to remove" is how a proposal that named the wrong address passes
  // while leaving the administrator it meant to remove in place.
  const plan = planChange({ params: params(8, [KEY_A]), remove: KEY_B });
  assert.equal(plan.after, null);
  assert.match(plan.problems[0], /would change nothing while reading as a removal/);
});

test('removing the last administrator is allowed, and says what it costs', () => {
  const plan = planChange({ params: params(8, [KEY_A]), remove: KEY_A });
  assert.deepEqual(plan.problems, []);
  assert.deepEqual(plan.after.administrators, []);
  assert.equal(plan.warnings.length, 1);
  assert.match(plan.warnings[0], /NOBODY can correct a recorded country/);
  assert.match(plan.warnings[0], /safe direction/);
});

test('one change at a time, and at least one', () => {
  const both = planChange({ params: params(8, [KEY_A]), add: GROUP_1, remove: KEY_A });
  assert.equal(both.action, 'none');
  assert.match(both.problems[0], /one appointment or one removal, not both/);

  const neither = planChange({ params: params(8, []) });
  assert.equal(neither.action, 'none');
  assert.match(neither.problems[0], /Name an address/);
});

test('whitespace around a typed address does not defeat the checks', () => {
  const plan = planChange({ params: params(8, [KEY_A]), add: `  ${KEY_A}  ` });
  assert.equal(plan.after, null);
  assert.match(plan.problems[0], /already a foundation administrator/);
});

// -------------------------------------------------------------------- the diff

test('the diff shows every field, including the ones that did not move', () => {
  // Showing only what changed would be a diff that cannot tell a reader that
  // nothing else did — and "nothing else moved" is the assurance a voter needs,
  // because MsgUpdateParams replaces everything.
  const plan = planChange({ params: params(8, [KEY_A]), add: GROUP_1, groupLookup: 'group' });
  const rows = paramsDiff(plan.before, plan.after);
  const payload = rows.find((r) => r.field === 'payload_length');
  assert.equal(payload.before, '8');
  assert.equal(payload.after, '8');
  assert.equal(payload.changed, false);

  const list = rows.find((r) => r.field === 'foundation_administrators');
  assert.equal(list.changed, true);
  assert.deepEqual(
    list.addresses.find((a) => a.address === KEY_A),
    { address: KEY_A, state: 'kept' },
  );
  assert.deepEqual(
    list.addresses.find((a) => a.address === GROUP_1),
    { address: GROUP_1, state: 'added' },
  );
});

test('a removed administrator is marked removed rather than omitted', () => {
  const plan = planChange({ params: params(8, [KEY_A, KEY_B]), remove: KEY_A });
  const list = paramsDiff(plan.before, plan.after).find((r) => r.field === 'foundation_administrators');
  assert.deepEqual(list.addresses.find((a) => a.address === KEY_A).state, 'removed');
  assert.deepEqual(list.addresses.find((a) => a.address === KEY_B).state, 'kept');
});

test('the diff of an empty list to an empty list is still two rows', () => {
  const rows = paramsDiff(params(8, []), params(8, []));
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[1].addresses, []);
});

test('a diff with nothing to compare is empty rather than throwing', () => {
  assert.deepEqual(paramsDiff(null, params(8, [])), []);
  assert.deepEqual(paramsDiff(params(8, []), null), []);
});

// --------------------------------------------------------------- the documents

test('the message carries both fields explicitly, including an empty list', () => {
  // Written out rather than omitted, so the document says the same thing the
  // diff said. An omitted repeated field decodes to the same empty list, but a
  // reader of the JSON cannot tell that from a field somebody forgot.
  const msg = updateParamsMessage({ authority: GOV, params: params(8, []) });
  assert.equal(msg['@type'], UPDATE_PARAMS_TYPE);
  assert.equal(msg.authority, GOV);
  assert.equal(msg.params.payload_length, 8);
  assert.deepEqual(msg.params.foundation_administrators, []);
});

test('the type URL is the proto package, not the REST path', () => {
  // The REST path carries a `yamale` prefix and the proto package does not.
  assert.equal(UPDATE_PARAMS_TYPE, '/blockchain.alias.v1.MsgUpdateParams');
});

test('a message with no authority is refused', () => {
  assert.throws(() => updateParamsMessage({ authority: '', params: params(8, []) }), /authority/);
});

test('the message does not alias the plan\'s array', () => {
  // A shared array would let a later edit of the form mutate a document already
  // shown as final.
  const plan = params(8, [KEY_A]);
  const msg = updateParamsMessage({ authority: GOV, params: plan });
  plan.administrators.push(KEY_B);
  assert.deepEqual(msg.params.foundation_administrators, [KEY_A]);
});

test('the proposal document is the shape submit-proposal reads', () => {
  const doc = JSON.parse(proposalDocument({
    messages: [updateParamsMessage({ authority: GOV, params: params(8, [GROUP_1]) })],
    title: 'Appoint an administrator',
    summary: 'Because.',
    deposit: '10000000uyml',
  }));
  assert.equal(doc.messages.length, 1);
  assert.equal(doc.deposit, '10000000uyml');
  assert.equal(doc.title, 'Appoint an administrator');
  assert.equal(doc.metadata, '');
});

test('a proposal missing any of its parts is refused', () => {
  const base = {
    messages: [{ '@type': UPDATE_PARAMS_TYPE }],
    title: 't',
    summary: 's',
    deposit: '1uyml',
  };
  assert.throws(() => proposalDocument({ ...base, messages: [] }), /no messages/);
  assert.throws(() => proposalDocument({ ...base, title: '' }), /title/);
  assert.throws(() => proposalDocument({ ...base, summary: '' }), /summary/);
  assert.throws(() => proposalDocument({ ...base, deposit: '' }), /deposit/);
});

test('the summary states the power rather than the parameter name', () => {
  // "foundation_administrators" reads like a list of people with logins. A voter
  // who reads only the summary has to have read what it actually confers.
  const plan = planChange({ params: params(8, []), add: GROUP_1, groupLookup: 'group' });
  const summary = changeSummary({ ...plan, reason: 'Ceremony of 2026-08-23.' });
  assert.match(summary, /correct any account's recorded country/);
  assert.match(summary, /reissues its identifier/);
  assert.match(summary, /ZZ/);
  assert.match(summary, /payload_length stays at 8/);
  assert.match(summary, /Ceremony of 2026-08-23/);
  assert.match(summary, /M-of-N group/);
});

test('the summary of a single-key appointment says it is a single key', () => {
  const plan = planChange({ params: params(8, []), add: KEY_A, groupLookup: 'not-group' });
  assert.match(changeSummary(plan), /single key/);
});

test('the summary of the last removal says nobody can correct a country afterwards', () => {
  const plan = planChange({ params: params(8, [KEY_A]), remove: KEY_A });
  assert.match(changeSummary(plan), /No administrator remains/);
});

test("the title and summary limits are the chain's, and they are not the same number", () => {
  // x/gov caps the title at MaxMetadataLen and the summary at forty times that.
  // Conflating them is what made this repository's ceremony tool truncate a
  // summary — with a marker — to a fortieth of the space it actually had, in the
  // one field that states in words what a proposal does.
  assert.equal(MAX_TITLE_LENGTH, 255);
  assert.equal(MAX_SUMMARY_LENGTH, 10200);

  const base = { messages: [{ '@type': UPDATE_PARAMS_TYPE }], deposit: '1uyml' };
  // A summary well over the title limit and well under its own must come through
  // whole, with no truncation marker.
  const long = 'x'.repeat(2000);
  const doc = JSON.parse(proposalDocument({ ...base, title: 'fine', summary: long }));
  assert.equal(doc.summary, long);

  // A title over its limit is refused, not shortened.
  assert.throws(() => proposalDocument({ ...base, title: 'y'.repeat(256), summary: 's' }), /255/);

  // A summary over its limit is truncated, not refused.
  const huge = JSON.parse(proposalDocument({ ...base, title: 't', summary: 'z'.repeat(20000) }));
  assert.ok(huge.summary.length <= MAX_SUMMARY_LENGTH);
  assert.match(huge.summary, /truncated/);
});

test('lengths are counted in bytes, and a truncation does not split a character', () => {
  // The chain measures len(string) in Go, which is UTF-8 bytes. An em dash costs
  // three of them, and the summary this module composes is full of them — a page
  // counting characters would let through a summary the chain refuses.
  const base = { messages: [{ '@type': UPDATE_PARAMS_TYPE }], deposit: '1uyml', title: 't' };
  // 3400 em dashes is 10,200 bytes exactly: at the limit, so untouched.
  const exact = '—'.repeat(3400);
  assert.equal(JSON.parse(proposalDocument({ ...base, summary: exact })).summary, exact);

  // One more is over, so it truncates — and the result must contain no U+FFFD,
  // which is what a naive byte slice through a multi-byte character produces.
  const over = '—'.repeat(3401);
  const cut = JSON.parse(proposalDocument({ ...base, summary: over })).summary;
  assert.ok(!cut.includes('�'), 'truncation split a multi-byte character');
  assert.match(cut, /truncated/);
});

test('the submit command names the chain, and refuses to be printed without one', () => {
  const cmd = submitCommand({ chainId: 'yamale-devnet-2' });
  assert.match(cmd, /tx gov submit-proposal proposal\.json/);
  assert.match(cmd, /--chain-id yamale-devnet-2/);
  assert.throws(() => submitCommand({ chainId: '' }), /chain id/);
});

test('the follow-up says code 0 is acceptance and not execution', () => {
  // Four separate bugs in this repository came from believing otherwise.
  const after = afterSubmitting({ chainId: 'yamale-devnet-2' });
  assert.match(after, /ACCEPTED, not executed/);
  assert.match(after, /query tx/);
  assert.match(after, /query alias params/);
});

// ---------------------------------------------------------------------------
// What the console shows a person, as opposed to what it composes.
//
// These are the arithmetic and the wording, and they are tested for the same
// reason as the refusals above: every one of them is a place where being
// plausibly wrong is worse than being obviously broken. A tally divided by a
// guessed exponent is off by a factor of a million and looks entirely
// reasonable; a quorum bar drawn at a default reports the opposite outcome from
// the chain; "failed" beside a green tally reads as a lost vote when it means a
// won one that never executed.
// ---------------------------------------------------------------------------

import {
  asPercent,
  decimalToBps,
  displayAmount,
  groupDigits,
  outcomeOf,
  readStakingDenom,
  readTallyParams,
  tallyOf,
  timeLeft,
  timestampSeconds,
  wholeAmount,
} from './administrators.js';
import {
  VALIDATORS,
  signingPower,
  voteCommand,
} from './validators.js';

test('an amount is divided by moving a decimal point through a string', () => {
  // The float version of this is 12.299999999999999, and this figure is a
  // number of somebody's tokens.
  assert.equal(displayAmount('12300000', 6), '12.3');
  assert.equal(displayAmount('65000000000', 6), '65,000');
  assert.equal(displayAmount('1', 6), '0.000001');
  assert.equal(displayAmount('0', 6), '0');
  // Past 2^53, where a double stops holding consecutive integers.
  assert.equal(displayAmount('9007199254740993000000', 6), '9,007,199,254,740,993');
  assert.equal(wholeAmount('174900000000', 6), '174,900');
  assert.equal(wholeAmount('999999', 6), '0');
  // No exponent means no division, not a division by one million.
  assert.equal(displayAmount('4200', 0), '4,200');
});

test('an amount that is not a plain integer is returned untouched, never guessed at', () => {
  assert.equal(displayAmount('not a number', 6), 'not a number');
  assert.equal(displayAmount(undefined, 6), '0');
  assert.equal(groupDigits('1234567'), '1,234,567');
});

test('the staking denom comes from the chain, and says when it did not', () => {
  const fromChain = readStakingDenom({
    stakingParams: { params: { bond_denom: 'uyml' } },
    metadata: { metadatas: [{
      base: 'uyml', display: 'yml', symbol: 'YML',
      denom_units: [{ denom: 'uyml', exponent: 0 }, { denom: 'yml', exponent: 6 }],
    }] },
  });
  assert.deepEqual(fromChain,
    { denom: 'uyml', symbol: 'YML', exponent: 6, source: 'chain', fromChain: true });

  // The live case on yamale-devnet-2: the metadata answers, with an entry for
  // every fiat denom and none for the staking denom. "Absent" and "unread" must
  // not collapse into one message — one of them sends somebody to look at a node
  // that is answering perfectly.
  const absent = readStakingDenom({
    stakingParams: { params: { bond_denom: 'uyml' } },
    metadata: { metadatas: [{ base: 'uzar', display: 'zar', symbol: 'ZAR', denom_units: [] }] },
  });
  assert.equal(absent.source, 'absent');
  assert.equal(absent.exponent, 6);
  assert.equal(absent.symbol, 'YML');

  // The fallback is flagged rather than silent: a page that assumed six places
  // on a chain that uses eight would be out by a factor of a hundred, and would
  // look exactly as confident.
  const guessed = readStakingDenom({ stakingParams: null, metadata: null });
  assert.equal(guessed.source, 'unread');
  assert.equal(guessed.fromChain, false);
  assert.equal(guessed.exponent, 6);
});

test('a cosmos decimal becomes basis points without going through a float', () => {
  assert.equal(decimalToBps('0.334000000000000000'), 3340);
  assert.equal(decimalToBps('0.500000000000000000'), 5000);
  assert.equal(decimalToBps('0.667000000000000000'), 6670);
  assert.equal(decimalToBps('1.000000000000000000'), 10000);
  assert.equal(decimalToBps('0'), 0);
  assert.equal(decimalToBps(''), null);
  assert.equal(decimalToBps(undefined), null);
});

test('quorum, threshold and veto are read from either shape the SDK serves', () => {
  const nested = readTallyParams({ tally_params: {
    quorum: '0.334', threshold: '0.5', veto_threshold: '0.334' } });
  assert.deepEqual(nested, { quorumBps: 3340, thresholdBps: 5000, vetoBps: 3340 });

  const flat = readTallyParams({ params: {
    quorum: '0.4', threshold: '0.6', veto_threshold: '0.3' } });
  assert.deepEqual(flat, { quorumBps: 4000, thresholdBps: 6000, vetoBps: 3000 });

  // Missing means null, so the caller omits the bar rather than drawing one at
  // a plausible default.
  assert.deepEqual(readTallyParams({}), { quorumBps: null, thresholdBps: null, vetoBps: null });
});

test('quorum counts abstentions and the threshold does not', () => {
  const params = { quorumBps: 3340, thresholdBps: 5000, vetoBps: 3340 };
  // 60 for, 40 abstaining, out of 200 bonded: turnout is half, so quorum is
  // met — and the threshold is 100%, because an abstention took no side.
  const t = tallyOf({
    tally: { yes_count: '60', no_count: '0', no_with_veto_count: '0', abstain_count: '40' },
    bondedTokens: '200',
    params,
  });
  assert.equal(t.turnoutBps, 5000);
  assert.equal(t.quorumMet, true);
  assert.equal(t.yesBps, 10000);
  assert.equal(t.thresholdMet, true);
  assert.equal(t.vetoed, false);
});

test('a tally below quorum is not reported as passing on its threshold alone', () => {
  const t = tallyOf({
    tally: { yes_count: '10', no_count: '0', no_with_veto_count: '0', abstain_count: '0' },
    bondedTokens: '1000',
    params: { quorumBps: 3340, thresholdBps: 5000, vetoBps: 3340 },
  });
  assert.equal(t.turnoutBps, 100);
  assert.equal(t.quorumMet, false);
  // Unanimous among those who voted, and it still fails. Both facts are
  // reported, separately, because the page draws two bars.
  assert.equal(t.yesBps, 10000);
  assert.equal(t.thresholdMet, true);
});

test('a veto past its threshold is reported however the rest of the tally reads', () => {
  const t = tallyOf({
    tally: { yes_count: '60', no_count: '0', no_with_veto_count: '40', abstain_count: '0' },
    bondedTokens: '100',
    params: { quorumBps: 3340, thresholdBps: 5000, vetoBps: 3340 },
  });
  assert.equal(t.thresholdMet, true);
  assert.equal(t.vetoShareBps, 4000);
  assert.equal(t.vetoed, true);
});

test('a tally with nothing in it is not a tally of zeroes', () => {
  const t = tallyOf({ tally: {}, bondedTokens: '100', params: {} });
  assert.equal(t.anyVotes, false);
  // No bonded total means no share, and null means "cannot be shown" rather
  // than zero — which would render as a bar sitting at the far left.
  const noBond = tallyOf({ tally: { yes_count: '5' }, bondedTokens: '0', params: {} });
  assert.equal(noBond.turnoutBps, null);
  assert.equal(noBond.quorumMet, null);
});

test('a tally is summed as BigInt, so a stake past 2^53 keeps its value', () => {
  const t = tallyOf({
    tally: { yes_count: '9007199254740993', no_count: '1' },
    bondedTokens: '9007199254740994',
    params: { quorumBps: 3340, thresholdBps: 5000, vetoBps: 3340 },
  });
  assert.equal(t.voted, '9007199254740994');
  assert.equal(t.turnoutBps, 10000);
});

test('percentages are rendered without inventing precision', () => {
  assert.equal(asPercent(3340), '33.4%');
  assert.equal(asPercent(5000), '50%');
  assert.equal(asPercent(10000), '100%');
  assert.equal(asPercent(null), '—');
});

test('a message is named by what it does, not by its type URL', () => {
  const upgrade = outcomeOf({
    '@type': '/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade',
    plan: { name: 'netting-and-perimeter', height: '13415' },
  });
  assert.equal(upgrade.understood, true);
  assert.match(upgrade.headline, /netting-and-perimeter/);
  assert.match(upgrade.headline, /13,415/);

  const appoint = outcomeOf({
    '@type': '/blockchain.alias.v1.MsgUpdateParams',
    params: { payload_length: 8, foundation_administrators: ['yml1abc'] },
  });
  assert.equal(appoint.understood, true);
  assert.match(appoint.headline, /1 account may correct any recorded country/);

  const empty = outcomeOf({
    '@type': '/blockchain.alias.v1.MsgUpdateParams',
    params: { payload_length: 8, foundation_administrators: [] },
  });
  assert.match(empty.headline, /nobody may correct/);
});

test('a message this page cannot decode says so rather than showing a type URL', () => {
  const unknown = outcomeOf({ '@type': '/blockchain.enforcement.v1.MsgOpenCase' });
  assert.equal(unknown.understood, false);
  // Still better than the URL: the module and the verb, in words, and an
  // admission attached to them.
  assert.match(unknown.headline, /open case in enforcement/);
  assert.match(unknown.headline, /cannot say what this one does/);

  const nonsense = outcomeOf({ '@type': 'not a type url' });
  assert.equal(nonsense.understood, false);
  assert.match(nonsense.headline, /cannot name/);

  // No type at all is the shape a hand-edited proposal document arrives in.
  assert.equal(outcomeOf({}).understood, false);
});

test('a deadline is a distance in both directions', () => {
  assert.equal(timeLeft(4 * 3600), 'in 4 hours');
  assert.equal(timeLeft(-4 * 3600), '4 hours ago');
  assert.equal(timeLeft(60), 'in under two minutes');
  assert.equal(timeLeft(-30), 'just now');
  assert.equal(timeLeft(3 * 86400), 'in 3 days');
  // The minute band runs to ninety minutes on purpose. A vote closing in
  // seventy-five minutes is a thing somebody can still act on, and "in 1 hour"
  // rounds away the fifteen minutes that decide whether they do.
  assert.equal(timeLeft(3600), 'in 60 minutes');
  assert.equal(timeLeft(75 * 60), 'in 75 minutes');
});

test('a timestamp that cannot be parsed is null rather than 1970', () => {
  assert.equal(timestampSeconds('2026-08-22T09:53:16.745831123Z'), 1787392396);
  assert.equal(timestampSeconds(''), null);
  assert.equal(timestampSeconds('not a date'), null);
  assert.equal(timestampSeconds(undefined), null);
});

test('a vote command names the chain, and refuses to be printed without one', () => {
  const [pi] = VALIDATORS;
  const cmd = voteCommand({
    validator: pi, proposalId: '7', option: 'yes', chainId: 'yamale-devnet-2',
  });
  assert.match(cmd, /tx gov vote 7 yes/);
  assert.match(cmd, /--from alice/);
  assert.match(cmd, /--chain-id yamale-devnet-2/);
  assert.match(cmd, /--home \/opt\/yamale\/node/);
  // Same rule as submitCommand: a command printed without a chain id is a
  // command somebody completes from memory.
  assert.throws(() => voteCommand({
    validator: pi, proposalId: '7', option: 'yes', chainId: '',
  }), /chain id/);
});

test('a composed vote command carries no carriage return', () => {
  // These are pasted into a shell, where a CR gives "$'\r': command not found",
  // an error that names neither the cause nor the file.
  const cmd = voteCommand({
    validator: VALIDATORS[1], proposalId: '1', option: 'no_with_veto', chainId: 'x',
  });
  assert.equal(cmd.includes('\r'), false);
});

// ---------------------------------------------------------------- validators


test('a validator row names the address that actually signs', () => {
  // The console asks "has this validator voted" by looking up an address. It
  // used to look up yml1nls726x…, which holds no balance and no delegation on
  // this chain, so pi was reported as never having voted no matter what was
  // done. The address must be the one the signing service's key resolves to.
  const pi = VALIDATORS.find((v) => v.moniker === 'pi');
  const pi2 = VALIDATORS.find((v) => v.moniker === 'pi-2');
  assert.equal(pi.key, 'alice');
  assert.equal(pi.voter, 'yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg');
  assert.equal(pi2.key, 'pival');
  assert.equal(pi2.voter, 'yml1vlukxvmeg6kjtu658sc7lvlu6uj7c4n4p0fmas');

  // And every validator must carry both, or the page cannot tell an operator
  // that the two differ.
  for (const v of VALIDATORS) {
    assert.ok(v.voter && v.voter.startsWith('yml1'), `${v.moniker} has no voter address`);
    assert.ok(v.operator && v.operator.startsWith('yml1'), `${v.moniker} has no operator address`);
  }
});

test('a signing key with no delegation cannot vote, and says why', () => {
  const pi2 = VALIDATORS.find((v) => v.moniker === 'pi-2');
  const power = signingPower(pi2, []);
  assert.equal(power.canVote, false);
  assert.match(power.note, /counts for nothing/);
  // The note must name the key, because the operator's next question is which
  // key it is talking about.
  assert.match(power.note, /pival/);
});

test('a signing key that is not the operator says so even when it has stake', () => {
  const pi = VALIDATORS.find((v) => v.moniker === 'pi');
  const power = signingPower(pi, [{ balance: { amount: '65000000000' } }]);
  assert.equal(power.canVote, true);
  assert.equal(power.staked, 65000000000n);
  // This is the sentence that stops an operator believing they moved pi's
  // 100 000 000 000 when they moved alice's 65 000 000 000.
  assert.match(power.note, /not with pi's operator account/);
});

test('an operator signing for itself needs no caveat', () => {
  const self = {
    moniker: 'pi', key: 'operator',
    voter: 'yml1same', operator: 'yml1same',
  };
  const power = signingPower(self, [{ balance: { amount: '100' } }]);
  assert.equal(power.canVote, true);
  assert.equal(power.note, '');
});
