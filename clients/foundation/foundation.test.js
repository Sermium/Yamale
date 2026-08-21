// Tests for the foundation console's judgement.
//
// The three things worth testing here are the three a custodian cannot check by
// looking at the page: whether a membership change is a shape the chain will
// accept, whether a proposal's effect has been described or merely displayed,
// and whether a vote count means what the page says it means.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  MAX_EXECUTION_PERIOD_SECONDS,
  displayLabel,
  carriedOut,
  execCommand,
  parseExecutions,
  parseSubmissions,
  setChainId,
  staleAgainstGroup,
  toBaseUnits,
  auditGroup,
  custodianIdentity,
  describeMessage,
  describeProposal,
  executability,
  formatCoin,
  proposalState,
  resultingMembers,
  swapPlan,
  tally,
  toCustodians,
  tooLong,
  voteCommand,
} from './foundation.js';

const A = 'yml1n54q7l9ll4atcdhlcxqv0tw4qzdh6ew2h04ged';
const B = 'yml1rpzxcl2t3g4y0nrzncxxj7yyccm04la2jwne84';
const C = 'yml16h07xutkege53xcjaas6s8pnkt3kvruq5sufet';
const D = 'yml1kappryh2vaf78vd474pzj3zkpg04vwx2zt5f7s';
const E = 'yml1yd94ndw74k3ku9uuqf5u83rxusgtvdl0t5fsj5';
const NEW = 'yml1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq';
const POLICY = 'yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj';

const FIVE = [A, B, C, D, E];
const REGISTRY = { uyml: { symbol: 'YML', exponent: 6, base: 'uyml' } };
const YES_ = 'VOTE_OPTION_YES';

const custodians = (weights = [1, 1, 1, 1, 1]) =>
  toCustodians(
    FIVE.map((address, i) => ({
      member: { address, weight: String(weights[i]), metadata: `chris${i + 1} (FP-${i + 1})` },
    })),
  );

// ------------------------------------------------------- custodian identity

test('custodians are ordered by name, not by the address order the chain returns', () => {
  const listed = toCustodians([
    { member: { address: E, weight: '1', metadata: 'chris5 (E)' } },
    { member: { address: A, weight: '1', metadata: 'chris10 (J)' } },
    { member: { address: B, weight: '1', metadata: 'chris2 (B)' } },
    { member: { address: C, weight: '1', metadata: '' } },
  ]);
  assert.deepEqual(listed.map((c) => c.name), ['chris2', 'chris5', 'chris10', null]);
});

test('a custodian name and fingerprint come out of the member metadata', () => {
  assert.deepEqual(custodianIdentity('chris2 (XYEC-D45D)'), {
    name: 'chris2',
    fingerprint: 'XYEC-D45D',
  });
});

test('metadata with no fingerprint reports none rather than inventing one', () => {
  // A custodian shown a name and no fingerprint must be able to tell that there
  // was nothing to check, which is why this is null and not the empty string.
  assert.deepEqual(custodianIdentity('chris2'), { name: 'chris2', fingerprint: null });
  assert.deepEqual(custodianIdentity(''), { name: null, fingerprint: null });
});

// ------------------------------------------------------------- group audit

test('a group matching the constitution passes its audit', () => {
  const audit = auditGroup({
    custodians: custodians(),
    threshold: 3,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 3 },
  });
  assert.equal(audit.ok, true);
  assert.deepEqual(audit.findings, []);
});

test('a group with the wrong number of custodians is reported as a breach', () => {
  const audit = auditGroup({
    custodians: custodians().slice(0, 4),
    threshold: 3,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 3 },
  });
  assert.equal(audit.ok, false);
  assert.ok(audit.findings.some((f) => f.severity === 'breach' && /fixes it at 5/.test(f.text)));
});

test('a threshold that disagrees with the constitution is a breach', () => {
  const audit = auditGroup({
    custodians: custodians(),
    threshold: 2,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 3 },
  });
  assert.ok(audit.findings.some((f) => f.severity === 'breach' && /2 signatures/.test(f.text)));
});

test('a custodian with no weight is a breach, not a member', () => {
  // Five people, one of whom cannot vote, is a four-of-five group wearing a
  // five-of-five label.
  const audit = auditGroup({
    custodians: custodians([1, 1, 1, 1, 0]),
    threshold: 3,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 3 },
  });
  assert.ok(audit.findings.some((f) => f.severity === 'breach' && /no voting weight/.test(f.text)));
});

test('a threshold that is not a majority of the weight is a breach', () => {
  const audit = auditGroup({
    custodians: custodians([1, 1, 1, 1, 1]),
    threshold: 2,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 2 },
  });
  assert.ok(
    audit.findings.some((f) => f.severity === 'breach' && /not a majority/.test(f.text)),
    'two of five lets two disjoint trios both pass something',
  );
});

test('a custodian with no recorded fingerprint is flagged, not passed over', () => {
  // The fingerprint is the only thing on the screen a custodian can check
  // against the paper from the ceremony. A row without one has been verified
  // against nothing, and must not look the same as a row that has.
  const audit = auditGroup({
    custodians: toCustodians(
      FIVE.map((address, i) => ({
        member: { address, weight: '1', metadata: i === 2 ? 'chris3' : `chris${i + 1} (FP-${i})` },
      })),
    ),
    threshold: 3,
    invariants: { foundation_custodian_count: 5, foundation_signature_threshold: 3 },
  });
  assert.ok(
    audit.findings.some((f) => f.severity === 'warn' && /no key fingerprint/.test(f.text)),
  );
});

test('an unreadable constitution stops the audit claiming anything', () => {
  const audit = auditGroup({ custodians: custodians(), threshold: 3, invariants: null });
  assert.equal(audit.ok, false);
  assert.equal(audit.findings[0].severity, 'unknown');
});

// -------------------------------------------------------- membership edits

test('a weight of zero removes and any other weight adds', () => {
  assert.equal(resultingMembers(FIVE, [{ address: A, weight: '0' }]).size, 4);
  assert.equal(resultingMembers(FIVE, [{ address: NEW, weight: '1' }]).size, 6);
});

test('a swap keeps the group the same size', () => {
  const result = resultingMembers(FIVE, [
    { address: A, weight: '0' },
    { address: NEW, weight: '1' },
  ]);
  assert.equal(result.size, 5);
  assert.equal(result.has(A), false);
  assert.equal(result.has(NEW), true);
});

test('an unreadable weight throws rather than being treated as a removal', () => {
  // Number('') is 0, so a blank weight would silently become a removal — which
  // is the one interpretation that shrinks the group.
  assert.throws(() => resultingMembers(FIVE, [{ address: A, weight: 'x' }]), /unreadable weight/);
});

test('a valid swap plan removes and adds in one message', () => {
  const plan = swapPlan({ current: FIVE, outgoing: C, incoming: NEW, expectedCount: 5 });
  assert.equal(plan.valid, true);
  assert.equal(plan.resulting, 5);
  assert.deepEqual(
    plan.updates.map((u) => [u.address, u.weight]),
    [
      [C, '0'],
      [NEW, '1'],
    ],
  );
});

test('a swap plan refuses a bare removal', () => {
  const plan = swapPlan({ current: FIVE, outgoing: C, incoming: '', expectedCount: 5 });
  assert.equal(plan.valid, false);
  assert.deepEqual(plan.updates, [], 'no command should be composable from this');
  assert.ok(plan.problems.some((p) => /incoming custodian's address/.test(p)));
});

test('a swap plan refuses an incoming address already in the group', () => {
  // This is the one that looks like a swap and is a removal: adding an existing
  // member is a no-op on the set, so the group drops to four and the chain
  // refuses the whole thing after a week of voting.
  const plan = swapPlan({ current: FIVE, outgoing: C, incoming: D, expectedCount: 5 });
  assert.equal(plan.valid, false);
  assert.ok(plan.problems.some((p) => /already a custodian/.test(p)));
});

test('a swap plan checks the resulting size against the constitution itself', () => {
  // A well-formed swap that still lands on the wrong number. Nothing else in
  // the plan is wrong here, so only the count check can catch it — which is the
  // check the chain's ante decorator actually performs.
  const plan = swapPlan({ current: FIVE, outgoing: C, incoming: NEW, expectedCount: 4 });
  assert.equal(plan.resulting, 5);
  assert.equal(plan.valid, false);
  assert.ok(plan.problems.some((p) => /fixes it at 4/.test(p) && /refuse/.test(p)));
});

test('a swap plan refuses a departing address that is not a custodian', () => {
  const plan = swapPlan({ current: FIVE, outgoing: NEW, incoming: 'yml1other', expectedCount: 5 });
  assert.equal(plan.valid, false);
  assert.ok(plan.problems.some((p) => /not in this group/.test(p)));
});

// --------------------------------------------------- describing a proposal

test('a foundation payment is described in words with the amount scaled', () => {
  const d = describeMessage(
    {
      '@type': '/cosmos.bank.v1beta1.MsgSend',
      from_address: POLICY,
      to_address: B,
      amount: [{ denom: 'uyml', amount: '5000000000' }],
    },
    { policyAddress: POLICY, registry: REGISTRY, names: { [B]: 'chris2' } },
  );
  assert.equal(d.understood, true);
  assert.match(d.headline, /^Pay 5,000 YML to chris2 /);
  assert.deepEqual(d.concerns, []);
});

test('a payment drawn on an account other than the foundation is flagged', () => {
  const d = describeMessage(
    {
      '@type': '/cosmos.bank.v1beta1.MsgSend',
      from_address: B,
      to_address: C,
      amount: [{ denom: 'uyml', amount: '1' }],
    },
    { policyAddress: POLICY, registry: REGISTRY },
  );
  assert.equal(d.understood, true);
  assert.ok(d.concerns.some((c) => /not the foundation account/.test(c)));
});

test('a custodian swap is described as a replacement', () => {
  const d = describeMessage(
    {
      '@type': '/cosmos.group.v1.MsgUpdateGroupMembers',
      admin: POLICY,
      group_id: '1',
      member_updates: [
        { address: C, weight: '0' },
        { address: NEW, weight: '1' },
      ],
    },
    { currentMembers: FIVE, expectedCount: 5, names: { [C]: 'chris3' } },
  );
  assert.equal(d.understood, true);
  assert.match(d.headline, /^Replace chris3 /);
  assert.match(d.headline, /with /);
  assert.deepEqual(d.concerns, []);
});

test('a swap is described as one seat changing hands, not two', () => {
  // "replace X with Y as custodians of the foundation" reads as though two
  // seats moved, which is a different and more alarming proposal.
  const swap = describeMessage(
    { '@type': '/cosmos.group.v1.MsgUpdateGroupMembers', group_id: '1', member_updates: [
      { address: C, weight: '0' }, { address: NEW, weight: '1' }] },
    { currentMembers: FIVE, expectedCount: 5 },
  );
  assert.match(swap.headline, /as a custodian of the foundation$/);
  assert.doesNotMatch(swap.headline, /as custodians/);

  // Two genuinely leaving and two joining is plural.
  const double = describeMessage(
    { '@type': '/cosmos.group.v1.MsgUpdateGroupMembers', group_id: '1', member_updates: [
      { address: C, weight: '0' }, { address: D, weight: '0' },
      { address: NEW, weight: '1' }, { address: 'yml1other', weight: '1' }] },
    { currentMembers: FIVE, expectedCount: 5 },
  );
  assert.match(double.headline, /as custodians of the foundation$/);
});

test('a membership change that would shrink the group is flagged as refusable', () => {
  const d = describeMessage(
    {
      '@type': '/cosmos.group.v1.MsgUpdateGroupMembers',
      admin: POLICY,
      group_id: '1',
      member_updates: [{ address: C, weight: '0' }],
    },
    { currentMembers: FIVE, expectedCount: 5 },
  );
  assert.ok(
    d.concerns.some((c) => /fixes it at 5/.test(c) && /refuse/.test(c)),
    'a custodian must be told this cannot pass before they vote on it',
  );
});

test('an undecodable message is refused rather than shown as JSON', () => {
  const d = describeMessage({ '@type': '/blockchain.somemodule.v1.MsgDoSomething', foo: 'bar' });
  assert.equal(d.understood, false);
  assert.ok(d.concerns.some((c) => /Do not vote on it from this page/.test(c)));
});

test('a proposal is only understood when every message in it is', () => {
  const ctx = { policyAddress: POLICY, registry: REGISTRY };
  const good = { '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: POLICY, to_address: B,
    amount: [{ denom: 'uyml', amount: '1' }] };
  const bad = { '@type': '/blockchain.x.v1.MsgUnknown' };

  assert.equal(describeProposal({ messages: [good] }, ctx).understood, true);
  assert.equal(describeProposal({ messages: [good, bad] }, ctx).understood, false);
  // A proposal with no messages is not "fully understood" — it does nothing,
  // and reporting that as understood would let it through the page's guard.
  assert.equal(describeProposal({ messages: [] }, ctx).understood, false);
});

// ------------------------------------------------------------ vote counting

test('votes are counted by weight and the undecided are named', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [
      { voter: A, option: 'VOTE_OPTION_YES' },
      { voter: B, option: 'VOTE_OPTION_YES' },
      { voter: C, option: 'VOTE_OPTION_NO' },
    ],
    threshold: 3,
  });
  assert.equal(counted.yes, 2);
  assert.equal(counted.no, 1);
  assert.equal(counted.undecided, 2);
  assert.equal(counted.reached, false);
  assert.equal(counted.stillNeeded, 1);
  assert.equal(counted.couldStillPass, true);
  assert.deepEqual(
    counted.rows.filter((r) => r.option === null).map((r) => r.custodian.name),
    ['chris4', 'chris5'],
  );
});

test('two yes votes on a three-of-five do not reach the threshold', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [
      { voter: A, option: 'VOTE_OPTION_YES' },
      { voter: B, option: 'VOTE_OPTION_YES' },
    ],
    threshold: 3,
  });
  assert.equal(counted.reached, false);
  assert.equal(counted.stillNeeded, 1);
});

test('three yes votes reach it', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [A, B, C].map((voter) => ({ voter, option: 'VOTE_OPTION_YES' })),
    threshold: 3,
  });
  assert.equal(counted.reached, true);
  assert.equal(counted.stillNeeded, 0);
});

test('a proposal three of five have refused can no longer pass', () => {
  // The page must stop calling this "open for voting" — the remaining two
  // custodians cannot change the outcome, and inviting them to try wastes a
  // week of everybody believing it is still live.
  const counted = tally({
    custodians: custodians(),
    votes: [A, B, C].map((voter) => ({ voter, option: 'VOTE_OPTION_NO' })),
    threshold: 3,
  });
  assert.equal(counted.couldStillPass, false);
});

test('abstentions use up weight without blocking', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [
      { voter: A, option: 'VOTE_OPTION_ABSTAIN' },
      { voter: B, option: 'VOTE_OPTION_ABSTAIN' },
      { voter: C, option: 'VOTE_OPTION_ABSTAIN' },
    ],
    threshold: 3,
  });
  assert.equal(counted.abstain, 3);
  assert.equal(counted.couldStillPass, false, 'only two votes remain and three are needed');
});

test('a decided proposal reports its recorded tally, not "nobody voted"', () => {
  // x/group prunes the votes and keeps the totals, so a rejected proposal has a
  // result and an empty vote list. Counting the empty list would report the
  // three custodians who refused it as never having voted.
  const counted = tally({
    custodians: custodians(),
    votes: [],
    threshold: 3,
    finalTally: { yes_count: '0', no_count: '3', no_with_veto_count: '0', abstain_count: '0' },
  });
  assert.equal(counted.individualVotes, false);
  assert.equal(counted.no, 3);
  assert.equal(counted.undecided, 2);
  assert.equal(counted.couldStillPass, false);
});

test('a decided proposal that passed reads as having reached the threshold', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [],
    threshold: 3,
    finalTally: { yes_count: '3', no_count: '0', no_with_veto_count: '0', abstain_count: '0' },
  });
  assert.equal(counted.reached, true);
  assert.equal(counted.individualVotes, false);
});

test('an open proposal with no votes is not mistaken for a pruned one', () => {
  // Its final_tally_result is all zeros, which must not be read as a result.
  const counted = tally({
    custodians: custodians(),
    votes: [],
    threshold: 3,
    finalTally: { yes_count: '0', no_count: '0', no_with_veto_count: '0', abstain_count: '0' },
  });
  assert.equal(counted.individualVotes, true);
  assert.equal(counted.undecided, 5);
  assert.equal(counted.couldStillPass, true);
});

test('live votes are preferred over the chain tally while a proposal is open', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [{ voter: A, option: YES_ }, { voter: B, option: YES_ }],
    threshold: 3,
    // Zeros, as x/group reports for an open proposal.
    finalTally: { yes_count: '0', no_count: '0', no_with_veto_count: '0', abstain_count: '0' },
  });
  assert.equal(counted.yes, 2, 'the votes on the chain, not the not-yet-computed tally');
  assert.equal(counted.individualVotes, true);
});

test('the threshold is counted in weight, not in people', () => {
  // The foundation gives every custodian one vote, but x/group's threshold is a
  // weight and nothing stops a group being built with uneven weights. A console
  // that counted heads would report a proposal as passing on two votes that the
  // chain scores as four — or refuse one the chain would carry out.
  const uneven = toCustodians([
    { member: { address: A, weight: '3', metadata: 'chris1 (A)' } },
    { member: { address: B, weight: '1', metadata: 'chris2 (B)' } },
    { member: { address: C, weight: '1', metadata: 'chris3 (C)' } },
  ]);

  const heavy = tally({ custodians: uneven, votes: [{ voter: A, option: YES_ }], threshold: 3 });
  assert.equal(heavy.yes, 3, "one custodian carrying weight 3 casts 3");
  assert.equal(heavy.reached, true);

  const light = tally({
    custodians: uneven,
    votes: [{ voter: B, option: YES_ }, { voter: C, option: YES_ }],
    threshold: 3,
  });
  assert.equal(light.yes, 2, 'two custodians of weight 1 cast 2, not 2 heads worth 3');
  assert.equal(light.reached, false);
  assert.equal(light.stillNeeded, 1);
});

test('a vote from a former custodian is surfaced, not silently dropped', () => {
  const counted = tally({
    custodians: custodians(),
    votes: [{ voter: NEW, option: 'VOTE_OPTION_YES' }],
    threshold: 3,
  });
  assert.equal(counted.yes, 0);
  assert.equal(counted.stale.length, 1);
});

// -------------------------------------------------------- proposal states

const NOW = 1_760_000_000;
const iso = (s) => new Date(s * 1000).toISOString();

test('a submitted proposal inside its window is open', () => {
  const s = proposalState(
    { status: 'PROPOSAL_STATUS_SUBMITTED', voting_period_end: iso(NOW + 86400) },
    NOW,
  );
  assert.equal(s.phase, 'open');
  assert.equal(s.votingClosed, false);
});

test('passed and not executed is its own state, not "done"', () => {
  const s = proposalState(
    {
      status: 'PROPOSAL_STATUS_ACCEPTED',
      executor_result: 'PROPOSAL_EXECUTOR_RESULT_NOT_RUN',
      voting_period_end: iso(NOW - 100),
    },
    NOW,
  );
  assert.equal(s.phase, 'awaiting-exec');
  assert.match(s.note, /has to be\s+executed|Nothing has moved yet/);
});

test('passed and executed is done', () => {
  const s = proposalState(
    {
      status: 'PROPOSAL_STATUS_ACCEPTED',
      executor_result: 'PROPOSAL_EXECUTOR_RESULT_SUCCESS',
      voting_period_end: iso(NOW - 100),
    },
    NOW,
  );
  assert.equal(s.phase, 'executed');
});

test('passed then failed at execution is not reported as done', () => {
  const s = proposalState(
    {
      status: 'PROPOSAL_STATUS_ACCEPTED',
      executor_result: 'PROPOSAL_EXECUTOR_RESULT_FAILURE',
      voting_period_end: iso(NOW - 100),
    },
    NOW,
  );
  assert.equal(s.phase, 'failed');
  assert.equal(s.tone, 'bad');
});

test('a passed proposal nobody executed in time is reported as lost', () => {
  const s = proposalState(
    {
      status: 'PROPOSAL_STATUS_ACCEPTED',
      executor_result: 'PROPOSAL_EXECUTOR_RESULT_NOT_RUN',
      voting_period_end: iso(NOW - MAX_EXECUTION_PERIOD_SECONDS - 1),
    },
    NOW,
  );
  assert.equal(s.phase, 'expired');
});

test('a proposal voided by a membership change says so', () => {
  const s = proposalState({ status: 'PROPOSAL_STATUS_ABORTED' }, NOW);
  assert.equal(s.phase, 'aborted');
  assert.match(s.note, /changed after this was proposed/);
});

test('voting closed with no recorded outcome is not "rejected"', () => {
  const s = proposalState(
    { status: 'PROPOSAL_STATUS_SUBMITTED', voting_period_end: iso(NOW - 10) },
    NOW,
  );
  assert.equal(s.phase, 'closing');
});

// ------------------------------------------------- proposals the group outran

test('a proposal raised against an older group version can never pass', () => {
  // x/group aborts these, but only when somebody tallies — until then the
  // status reads SUBMITTED and the deadline is still in the future, so the
  // custodians can spend a week signing something already dead.
  const s = staleAgainstGroup(
    { group_version: '1', group_policy_version: '1' },
    { groupVersion: '2', policyVersion: '1' },
  );
  assert.equal(s.stale, true);
  assert.match(s.reasons[0], /custodians have changed/);
});

test('a change to the decision policy also strands a proposal', () => {
  const s = staleAgainstGroup(
    { group_version: '2', group_policy_version: '1' },
    { groupVersion: '2', policyVersion: '2' },
  );
  assert.equal(s.stale, true);
  assert.match(s.reasons[0], /decision policy has changed/);
});

test('a proposal matching the current group is not stale', () => {
  const s = staleAgainstGroup(
    { group_version: '2', group_policy_version: '1' },
    { groupVersion: '2', policyVersion: '1' },
  );
  assert.equal(s.stale, false);
  assert.deepEqual(s.reasons, []);
});

test('unknown versions are not treated as a mismatch', () => {
  // A node that did not return the group info must not make every proposal
  // look void — that would tell the custodians to abandon live business.
  assert.equal(staleAgainstGroup({}, { groupVersion: '2' }).stale, false);
  assert.equal(
    staleAgainstGroup({ group_version: '1' }, { groupVersion: undefined }).stale,
    false,
  );
});

test('a stale proposal is labelled void, ahead of anything about its votes', () => {
  const stale = { stale: true, reasons: ['the custodians have changed'] };
  const atThreshold = votesOf([[A, YES_], [B, YES_], [C, YES_]]);
  assert.equal(displayLabel(openState(), atThreshold, stale).label, 'void — the group has changed');
});

test('a stale proposal is never offered for execution', () => {
  // Executing it aborts it. Offering that as the way to carry it out would be
  // offering to destroy the proposal.
  const stale = { stale: true, reasons: ['x'] };
  const atThreshold = votesOf([[A, YES_], [B, YES_], [C, YES_]]);
  assert.equal(executability(openState(), atThreshold, stale).can, false);
  assert.equal(executability(openState(), atThreshold).can, true, 'and still offered when fresh');
});

// ---------------------------------------------------------- display label

const openState = (offset = 500000) =>
  proposalState({ status: 'PROPOSAL_STATUS_SUBMITTED', voting_period_end: iso(NOW + offset) }, NOW);

const votesOf = (opts) =>
  tally({
    custodians: custodians(),
    votes: opts.map(([voter, option]) => ({ voter, option })),
    threshold: 3,
  });

test('a proposal at threshold is never labelled "open for voting"', () => {
  // x/group leaves the status at SUBMITTED until somebody executes, so the raw
  // status says "open" about a proposal that is already agreed. A custodian
  // reading that goes away and waits.
  const counted = votesOf([[A, YES_], [B, YES_], [C, YES_]]);
  const shown = displayLabel(openState(), counted);
  assert.equal(shown.label, 'agreed — not yet carried out');
  assert.notEqual(shown.label, 'open for voting');
});

test('a proposal that can no longer pass says so rather than "open"', () => {
  const counted = votesOf([[A, 'VOTE_OPTION_NO'], [B, 'VOTE_OPTION_NO'], [C, 'VOTE_OPTION_NO']]);
  assert.equal(displayLabel(openState(), counted).label, 'cannot pass');
});

test('a genuinely open proposal is labelled by what it still needs', () => {
  const counted = votesOf([[A, YES_]]);
  assert.equal(displayLabel(openState(), counted).label, '2 more to go');
});

test('a decided proposal keeps the state label', () => {
  const state = proposalState(
    { status: 'PROPOSAL_STATUS_ACCEPTED', executor_result: 'PROPOSAL_EXECUTOR_RESULT_SUCCESS',
      voting_period_end: iso(NOW - 10) }, NOW,
  );
  assert.equal(displayLabel(state, votesOf([[A, YES_], [B, YES_], [C, YES_]])).label, 'done');
});

// ------------------------------------------------------------ executability

test('a proposal at threshold can be executed before its deadline', () => {
  // The foundation policy sets no minimum execution period, so waiting for the
  // seventh day is a choice and not a requirement.
  const state = proposalState(
    { status: 'PROPOSAL_STATUS_SUBMITTED', voting_period_end: iso(NOW + 500000) },
    NOW,
  );
  const counted = tally({
    custodians: custodians(),
    votes: [A, B, C].map((voter) => ({ voter, option: 'VOTE_OPTION_YES' })),
    threshold: 3,
  });
  const e = executability(state, counted);
  assert.equal(e.can, true);
  assert.match(e.why, /no waiting period/);
});

test('a proposal short of the threshold cannot be executed', () => {
  const state = proposalState(
    { status: 'PROPOSAL_STATUS_SUBMITTED', voting_period_end: iso(NOW + 500000) },
    NOW,
  );
  const counted = tally({
    custodians: custodians(),
    votes: [{ voter: A, option: 'VOTE_OPTION_YES' }],
    threshold: 3,
  });
  assert.equal(executability(state, counted).can, false);
});

// -------------------------------------------------- the pruned-proposal log
//
// Shapes taken verbatim from a local chain: attribute values are JSON-encoded,
// so a proposal id arrives as the four characters `"2"`.

const EXEC_TX = {
  txhash: 'AAA',
  height: '532',
  timestamp: '2026-08-21T18:59:00Z',
  events: [
    {
      type: 'cosmos.group.v1.EventProposalPruned',
      attributes: [
        { key: 'proposal_id', value: '"2"' },
        { key: 'status', value: '"PROPOSAL_STATUS_ACCEPTED"' },
        { key: 'tally_result', value: '{"yes_count":"3","abstain_count":"0","no_count":"0","no_with_veto_count":"0"}' },
      ],
    },
    {
      type: 'cosmos.group.v1.EventExec',
      attributes: [
        { key: 'proposal_id', value: '"2"' },
        { key: 'result', value: '"PROPOSAL_EXECUTOR_RESULT_SUCCESS"' },
        { key: 'logs', value: '""' },
      ],
    },
  ],
};

const NOOP_TX = {
  txhash: 'BBB',
  height: '537',
  events: [
    {
      type: 'cosmos.group.v1.EventExec',
      attributes: [
        { key: 'proposal_id', value: '"3"' },
        { key: 'result', value: '"PROPOSAL_EXECUTOR_RESULT_NOT_RUN"' },
      ],
    },
  ],
};

test('an execution is read out of the log with its final tally', () => {
  const [e] = parseExecutions([EXEC_TX]);
  assert.equal(e.proposalId, '2');
  assert.equal(e.succeeded, true);
  assert.equal(e.tally.yes_count, '3');
  assert.equal(e.height, 532);
});

test('an execution that changed nothing is not reported as a success', () => {
  // `group exec` on a proposal short of the threshold returns code 0 — the
  // transaction succeeded, the proposal did not. Treating that as history would
  // tell a custodian money moved when it did not.
  const [e] = parseExecutions([NOOP_TX]);
  assert.equal(e.succeeded, false);
  assert.equal(e.tally, null);
  assert.deepEqual(carriedOut(parseExecutions([NOOP_TX]), new Map()), []);
});

test('unquoted event attributes are read the same way', () => {
  // Older nodes emit the values without JSON quoting.
  const [e] = parseExecutions([{
    txhash: 'C', height: '9',
    events: [{ type: 'cosmos.group.v1.EventExec', attributes: [
      { key: 'proposal_id', value: '7' },
      { key: 'result', value: 'PROPOSAL_EXECUTOR_RESULT_SUCCESS' },
    ] }],
  }]);
  assert.equal(e.proposalId, '7');
  assert.equal(e.succeeded, true);
});

test('what an executed proposal did is recovered from its submitting transaction', () => {
  const submissions = parseSubmissions(
    [{ body: { messages: [{
      '@type': '/cosmos.group.v1.MsgSubmitProposal',
      title: 'Return 5,000 YML',
      summary: 'Restitution.',
      proposers: [A],
      messages: [{ '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: POLICY, to_address: B,
        amount: [{ denom: 'uyml', amount: '5000000000' }] }],
    }] } }],
    [{ txhash: 'SUB', height: '178', events: [{
      type: 'cosmos.group.v1.EventSubmitProposal',
      attributes: [{ key: 'proposal_id', value: '"2"' }],
    }] }],
  );

  const history = carriedOut(parseExecutions([EXEC_TX]), submissions);
  assert.equal(history.length, 1);
  assert.equal(history[0].proposal.title, 'Return 5,000 YML');
  assert.equal(history[0].proposal.messages[0]['@type'], '/cosmos.bank.v1beta1.MsgSend');
});

test('an execution whose submission is out of range still appears', () => {
  // Without this the page would silently drop a payment that did happen,
  // because the transaction that raised it fell outside the search window.
  const history = carriedOut(parseExecutions([EXEC_TX]), new Map());
  assert.equal(history.length, 1);
  assert.equal(history[0].proposal, null);
});

test('carried-out proposals come back newest first', () => {
  const older = { ...EXEC_TX, txhash: 'OLD', height: '100' };
  const history = carriedOut(parseExecutions([older, EXEC_TX]), new Map());
  assert.deepEqual(history.map((h) => h.height), [532, 100]);
});

// ---------------------------------------------------------------- commands

/** Commands are wrapped for reading; this is the single line a shell would see. */
const oneLine = (cmd) => cmd.replace(/\\\n\s*/g, '');

test('the vote command carries the metadata positional the subcommand requires', () => {
  // `tx group vote` takes four arguments. Omitting the fourth makes cobra read
  // the first flag as the metadata and fail somewhere unrelated.
  const cmd = oneLine(voteCommand({ proposalId: '7', voter: A, option: 'VOTE_OPTION_YES' }));
  assert.match(cmd, new RegExp(`tx group vote 7 ${A} VOTE_OPTION_YES ''`));
  assert.match(cmd, new RegExp(`--from ${A}`));
});

test('the deciding vote can carry --exec so the money moves in the same step', () => {
  const cmd = voteCommand({ proposalId: '7', voter: A, option: 'VOTE_OPTION_YES', alsoExecute: true });
  assert.match(cmd, /--exec 1/);
  assert.doesNotMatch(
    voteCommand({ proposalId: '7', voter: A, option: 'VOTE_OPTION_YES' }),
    /--exec/,
  );
});

test('commands carry the chain id the page was told, not a compiled-in one', () => {
  // --chain-id is covered by the signature, so a wrong one fails as a signature
  // error and sends a custodian to look at their key instead of at the flag.
  setChainId('foundation-local');
  assert.match(oneLine(voteCommand({ proposalId: '1', voter: A, option: YES_ })),
    /--chain-id foundation-local/);
  assert.match(oneLine(execCommand({ proposalId: '1', executor: A })),
    /--chain-id foundation-local/);

  setChainId('yamale-devnet-2');
  assert.match(oneLine(execCommand({ proposalId: '1', executor: A })),
    /--chain-id yamale-devnet-2/);
});

test('an unknown chain id leaves a visible blank rather than a plausible guess', () => {
  setChainId('');
  assert.match(oneLine(execCommand({ proposalId: '1', executor: A })), /--chain-id '<chain-id>'/);
  setChainId('foundation-local');
});

test('metadata longer than the module allows is caught before composing', () => {
  assert.equal(tooLong('a'.repeat(255)), false);
  assert.equal(tooLong('a'.repeat(256)), true);
  // Bytes, not characters: the module's limit is on the encoded length.
  assert.equal(tooLong('é'.repeat(128)), true);
});

// ----------------------------------------------------------------- amounts

test('whole units become base units exactly, without a float in the path', () => {
  assert.equal(toBaseUnits('1234.56', 6), '1234560000');
  assert.equal(toBaseUnits('12.30', 6), '12300000');
  assert.equal(toBaseUnits('5000', 6), '5000000000');
  assert.equal(toBaseUnits('0.000001', 6), '1');
  assert.equal(toBaseUnits('0', 6), '0');
  // 12.30 * 1e6 is 12299999.999999998 in IEEE754.
  assert.equal(toBaseUnits('0.07', 2), '7');
});

test('an amount with more decimals than the currency has is refused, not rounded', () => {
  // Rounding here would compose a proposal for an amount nobody typed.
  assert.throws(() => toBaseUnits('1.0000005', 6), /6 decimal places and that has 7/);
  assert.throws(() => toBaseUnits('1.5', 0), /no subdivisions/);
});

test('a non-numeric amount is refused rather than becoming NaN', () => {
  assert.throws(() => toBaseUnits('', 6), /not a plain amount/);
  assert.throws(() => toBaseUnits('5e6', 6), /not a plain amount/);
  assert.throws(() => toBaseUnits('-5', 6), /not a plain amount/);
});

test('an amount is scaled by the denom exponent', () => {
  assert.equal(formatCoin({ denom: 'uyml', amount: '5000000000' }, { registry: REGISTRY }), '5,000 YML');
  assert.equal(formatCoin({ denom: 'uyml', amount: '1' }, { registry: REGISTRY }), '0.000001 YML');
});

test('an unknown denom is shown in base units rather than divided by a guess', () => {
  // Guessing six decimals on a denom nobody registered is a factor of a million
  // on a restitution payment.
  assert.equal(formatCoin({ denom: 'uzzz', amount: '1000' }, { registry: REGISTRY }), '1,000 uzzz');
});
