// Tests for the foundation console's judgement.
//
// The three things worth testing here are the three a custodian cannot check by
// looking at the page: whether a membership change is a shape the chain will
// accept, whether a proposal's effect has been described or merely displayed,
// and whether a vote count means what the page says it means.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ASSIGNED_COUNTRIES,
  CHAIN_WIDE,
  FOUNDATION_COUNTRY,
  MAX_EXECUTION_PERIOD_SECONDS,
  ROLES,
  assignedCountries,
  checkAddress,
  checkScope,
  classifyHolder,
  displayLabel,
  findGrant,
  grantPlan,
  grantRoleMessage,
  normaliseScope,
  officeSummary,
  revokePlan,
  revokeRoleMessage,
  roleByName,
  carriedOut,
  execCommand,
  parseExecutions,
  parseSubmissions,
  proposalDocument,
  submitCommand,
  auditMessages,
  parseMessages,
  setChainId,
  shellSafe,
  stalledAtHeight,
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

// --- a halted chain -----------------------------------------------------------

const STALLED = JSON.stringify({
  code: 2,
  message: 'codespace sdk code 26: invalid height: context did not contain latest ' +
    'block height in either check state or finalize block state (2733)',
  details: [],
});

test('stalledAtHeight reads the last committed height out of the node error', () => {
  assert.equal(stalledAtHeight(STALLED), 2733);
  assert.equal(stalledAtHeight(JSON.parse(STALLED)), 2733, 'parsed body too');
});

test('stalledAtHeight ignores unrelated failures', () => {
  assert.equal(stalledAtHeight('{"code":5,"message":"not found"}'), null);
  assert.equal(stalledAtHeight(''), null);
  assert.equal(stalledAtHeight(null), null);
  assert.equal(stalledAtHeight(undefined), null);
  // A 401 body from the proxy must not be mistaken for a stall: retrying it with
  // a height header would turn "you are not allowed to read this" into a second
  // identical refusal, and the banner that explains the allowlist would never
  // be shown.
  assert.equal(stalledAtHeight('<html><title>401 Authorization Required</title>'), null);
});

test('stalledAtHeight refuses height zero', () => {
  // A node that has never committed a block reports (0), and height 0 means
  // "latest" — which is the request that just failed. Retrying it is a loop.
  const fresh = STALLED.replace('(2733)', '(0)');
  assert.equal(stalledAtHeight(fresh), null);
});

test('a copied command carries no carriage returns', () => {
  // Built from character codes rather than escapes, so this test means the
  // same thing whatever this file's own line endings are — which is exactly
  // the point: the bug is that a source file's endings leak into the command
  // text a custodian copies.
  const CR = String.fromCharCode(13);
  const NL = String.fromCharCode(10);
  assert.equal(shellSafe('one' + CR + NL + 'two'), 'one' + NL + 'two');
  assert.equal(shellSafe('no returns here'), 'no returns here');
  assert.equal(shellSafe(null), '');
  assert.ok(!shellSafe('a' + CR + 'b').includes(CR), 'no carriage return survives');
});

// --- the general proposal form ------------------------------------------------

const FOUNDATION = 'yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj';
const OUTSIDER = 'yml1yd94ndw74k3ku9uuqf5u83rxusgtvdl0t5fsj5';

test('messages are accepted in each of the three shapes a person pastes', () => {
  const msg = { '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: FOUNDATION };

  // A bare list, which is what a colleague sends.
  assert.deepEqual(parseMessages(JSON.stringify([msg])).messages, [msg]);
  // A single message, which is what module documentation shows.
  assert.deepEqual(parseMessages(JSON.stringify(msg)).messages, [msg]);
  // A whole proposal document, which is what this page emits and the CLI takes.
  assert.deepEqual(
    parseMessages(JSON.stringify({ group_policy_address: FOUNDATION, messages: [msg] })).messages,
    [msg],
  );
});

test('broken input is refused with the reason, not silently emptied', () => {
  assert.match(parseMessages('{oh dear').error, /not valid JSON/);
  assert.match(parseMessages('[]').error, /no messages/);
  assert.match(parseMessages('   ').error, /Nothing pasted/);
  assert.match(parseMessages('"a string"').error, /Expected a message/);
  assert.match(parseMessages('42').error, /Expected a message/);
});

test('a message the foundation does not sign is a fatal problem, not a note', () => {
  // This is the failure worth catching: it passes submission, collects three
  // signatures, waits out the voting period and only then fails.
  const [a] = auditMessages(
    [{ '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: OUTSIDER }],
    { policyAddress: FOUNDATION },
  );
  assert.equal(a.problems.length, 1);
  assert.match(a.problems[0], /not the foundation account/);
  assert.match(a.problems[0], /fail at execution/);
});

test('the foundation signing its own message raises nothing', () => {
  const [a] = auditMessages(
    [{ '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: FOUNDATION, to_address: OUTSIDER }],
    { policyAddress: FOUNDATION },
  );
  assert.deepEqual(a.problems, []);
  // to_address is not a signer field, so naming somebody else there is the
  // entire point of a payment and must not be flagged.
  assert.deepEqual(a.concerns, []);
});

test('authority and admin are checked, since they are the signer where they appear', () => {
  const [params] = auditMessages(
    [{ '@type': '/yamale.blockchain.oracle.v1.MsgUpdateParams', authority: OUTSIDER }],
    { policyAddress: FOUNDATION },
  );
  assert.match(params.problems[0], /"authority"/);

  const [members] = auditMessages(
    [{ '@type': '/cosmos.group.v1.MsgUpdateGroupMembers', admin: OUTSIDER }],
    { policyAddress: FOUNDATION },
  );
  assert.match(members.problems[0], /"admin"/);
});

test('an ambiguous signer field is raised as a concern, not asserted as an error', () => {
  // delegator_address is usually the signer and sometimes just a party, so the
  // page says which of those it cannot tell rather than claiming a failure.
  const [a] = auditMessages(
    [{ '@type': '/cosmos.staking.v1beta1.MsgDelegate', delegator_address: OUTSIDER }],
    { policyAddress: FOUNDATION },
  );
  assert.deepEqual(a.problems, []);
  assert.equal(a.concerns.length, 1);
  assert.match(a.concerns[0], /If it is just a party to the message, this is fine/);
});

test('a missing or malformed type url is caught before anybody signs', () => {
  const [none] = auditMessages([{ from_address: FOUNDATION }], { policyAddress: FOUNDATION });
  assert.match(none.problems[0], /no "@type"/);

  const [bad] = auditMessages([{ '@type': 'MsgSend' }], { policyAddress: FOUNDATION });
  assert.match(bad.problems[0], /not a type URL/);

  // A well-formed one this page has never heard of is NOT a problem. Refusing
  // unknown messages would defeat the entire purpose of a general form.
  const [ok] = auditMessages(
    [{ '@type': '/some.future.v1.MsgWhatever' }], { policyAddress: FOUNDATION },
  );
  assert.deepEqual(ok.problems, []);
});

test('every message is audited, not just the first', () => {
  const audit = auditMessages([
    { '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: FOUNDATION },
    { '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: OUTSIDER },
  ], { policyAddress: FOUNDATION });
  assert.equal(audit.length, 2);
  assert.deepEqual(audit[0].problems, []);
  assert.equal(audit[1].problems.length, 1);
  assert.equal(audit[1].index, 1, 'the index is reported so the message can be pointed at');
});

test('auditing without a policy address checks shape only', () => {
  // The page always supplies one; this guards against a caller that does not,
  // which must not silently treat every address as acceptable *or* fail.
  const [a] = auditMessages([{ '@type': '/cosmos.bank.v1beta1.MsgSend', from_address: OUTSIDER }]);
  assert.deepEqual(a.problems, []);
  assert.deepEqual(a.concerns, []);
});

// -------------------------------------------------------- country-scoped roles
//
// Every refusal here has the same shape of consequence: a rule not enforced
// before composing is a rule enforced after three custodians have signed, waited
// out a voting period, and watched the execution fail. So the refusals are what
// is tested hardest, and each one is tested for its *reason* being present in the
// text, not just for a problem count — a page that refuses without saying why
// sends a custodian to the wrong place.

/** An office group, as the chain returns one. Its own admin, two of three. */
const OFFICE = 'yml1lands0000000000000000000000000000000000000000000000000000000';
const officePolicy = (over = {}) => ({
  address: OFFICE,
  group_id: '7',
  admin: OFFICE,
  decision_policy: { '@type': '/cosmos.group.v1.ThresholdDecisionPolicy', threshold: '2' },
  ...over,
});
const officeMembers = () => [
  { member: { address: A, weight: '1', metadata: 'Adaeze (AAAA-1111)' } },
  { member: { address: B, weight: '1', metadata: 'Bola (BBBB-2222)' } },
  { member: { address: C, weight: '1', metadata: 'Chidi (CCCC-3333)' } },
];

/** The verdict the page reaches after a successful group-policy lookup. */
const IS_GROUP = { verdict: 'group', groupId: '7', problem: null };

const grantArgs = (over = {}) => ({
  policyAddress: POLICY,
  holder: OFFICE,
  role: 'ROLE_REGISTRY_AUTHORITY',
  jurisdiction: 'ZA',
  holderVerdict: IS_GROUP,
  existingGrants: [],
  ...over,
});

// --- the role table ---

test('the five roles are the five the chain has, numbered as the proto numbers them', () => {
  assert.deepEqual(
    ROLES.map((r) => [r.number, r.name]),
    [
      [1, 'ROLE_REGISTRY_AUTHORITY'],
      [2, 'ROLE_MONETARY_AUTHORITY'],
      [3, 'ROLE_PAYMENTS_AUTHORITY'],
      [4, 'ROLE_ENFORCEMENT_AUTHORITY'],
      [5, 'ROLE_SUPERVISOR'],
    ],
  );
  assert.equal(roleByName('ROLE_UNSPECIFIED'), null, 'the zero value is not a role on offer');
});

test('the two roles nothing can use are marked, and say what they do not switch on', () => {
  const dead = ROLES.filter((r) => !r.live).map((r) => r.name);
  assert.deepEqual(dead, ['ROLE_ENFORCEMENT_AUTHORITY', 'ROLE_SUPERVISOR']);

  const enforcement = roleByName('ROLE_ENFORCEMENT_AUTHORITY');
  assert.match(enforcement.caveat, /bonded validator/);
  assert.match(enforcement.caveat, /emergency_authority/);

  const supervisor = roleByName('ROLE_SUPERVISOR');
  assert.match(supervisor.caveat, /Nothing on this chain consults this role/);

  // And the three that work carry no caveat at all, so the warning means
  // something when it appears.
  for (const r of ROLES.filter((x) => x.live)) assert.equal(r.caveat, null);

  // The picker label says a dead role is dead in words short enough to survive a
  // <select> on a phone. A label clipped to "Enforcement authority — x/enforce…"
  // would read as a working capability, which is the opposite of the truth.
  for (const r of ROLES) {
    assert.ok(r.picker.length <= 22, `${r.name} picker label is ${r.picker.length} chars`);
    assert.equal(/nothing/.test(r.picker), !r.live, `${r.name} picker must match its liveness`);
  }
});

// --- the scope ---

test('an assigned country code is accepted, in either case', () => {
  assert.deepEqual(checkScope('ZA'), { scope: 'ZA', problem: null });
  assert.deepEqual(checkScope('za'), { scope: 'ZA', problem: null });
  assert.deepEqual(checkScope('  sn  '), { scope: 'SN', problem: null });
});

test('the foundation may not grant the chain-wide scope, and is told why', () => {
  const { scope, problem } = checkScope('*');
  assert.equal(scope, CHAIN_WIDE, 'the marker is never folded into a country code');
  assert.match(problem, /may not grant the chain-wide scope/);
  assert.match(problem, /Only governance/);
  // The consequence, not just the rule: this is the one refusal whose absence
  // costs a whole voting period.
  assert.match(problem, /collect three signatures.*then fail/s);
});

test('normalising can never invent the chain-wide marker', () => {
  // The property is about every input, not only the assigned ones — a mutation
  // pass found that a version asserting this over the country list alone let a
  // fold of the reserved code through, and the reserved code is exactly what
  // somebody reaching for chain-wide authority types.
  assert.equal(normaliseScope('*'), CHAIN_WIDE, 'the marker itself passes through');
  const inputs = [
    ...assignedCountries(), ...assignedCountries().map((c) => c.toLowerCase()),
    'zz', 'ZZ', 'Zz', '', ' ', 'star', 'all', 'any', 'ALL', '**', '*a', 'a*',
    '%', '.', '-', 'xx', 'XX', 'null', 'undefined', '0',
  ];
  for (const input of inputs) {
    assert.notEqual(normaliseScope(input), CHAIN_WIDE, `${JSON.stringify(input)} became "*"`);
  }
  assert.notEqual(normaliseScope(null), CHAIN_WIDE);
  assert.notEqual(normaliseScope(undefined), CHAIN_WIDE);
});

test("the foundation's own reserved code is refused as a country an office sits in", () => {
  const { problem } = checkScope(FOUNDATION_COUNTRY);
  assert.match(problem, /reserved code/);
  assert.match(problem, /absence\* of a national perimeter|absence of a national perimeter/);
  // It must not be swallowed by the generic "not an assigned code" branch, whose
  // wording would send the reader looking for a typo in a code they typed on
  // purpose.
  assert.doesNotMatch(problem, /ISO has assigned/);
  assert.equal(ASSIGNED_COUNTRIES.has(FOUNDATION_COUNTRY), false);
});

test('two letters that are not a country are refused, and named', () => {
  for (const code of ['NX', 'QK', 'ZX']) {
    const { problem } = checkScope(code);
    assert.match(problem, new RegExp(`"${code}" is not an ISO 3166-1`));
  }
  assert.match(checkScope('ZAA').problem, /not an ISO 3166-1/);
  assert.match(checkScope('Z').problem, /not an ISO 3166-1/);
});

test('an empty jurisdiction is refused rather than defaulted', () => {
  assert.match(checkScope('').problem, /Name the country/);
  assert.match(checkScope(null).problem, /Name the country/);
});

test('the mirrored ISO table matches the chain on the codes this deployment uses', () => {
  // The whole table is a duplicate of x/alias/types/iso3166.go, so this asserts
  // the properties that would break a grant rather than re-listing it: the
  // countries in the repository's own guides are present, the reserved code is
  // absent, and nothing two-letter-shaped slipped in that is not two letters.
  for (const code of ['ZA', 'SN', 'GH', 'NG', 'KE', 'GB', 'US', 'CH']) {
    assert.ok(ASSIGNED_COUNTRIES.has(code), `${code} should be an assigned code`);
  }
  assert.equal(ASSIGNED_COUNTRIES.size, 249, 'the assigned list is 249 codes');
  for (const code of ASSIGNED_COUNTRIES) assert.match(code, /^[A-Z]{2}$/);
});

// --- the holder ---

test('a plain key is refused before composing, not after voting', () => {
  // Verbatim from the live node: a group-policy lookup on a plain key answers
  // HTTP 500 with this body.
  const verdict = classifyHolder({
    status: 500,
    body: { code: 2, message: 'codespace sdk code 38: not found: group policy', details: [] },
  });
  assert.equal(verdict.verdict, 'plain-key');
  assert.match(verdict.problem, /not an x\/group account/);
  assert.match(verdict.problem, /one key is one bribe/);

  const plan = grantPlan(grantArgs({ holderVerdict: verdict }));
  assert.equal(plan.ready, false);
  assert.deepEqual(plan.messages, [], 'nothing is composed for a plain-key holder');
  assert.ok(plan.problems.some((p) => /not an x\/group account/.test(p)));
});

test('a group policy is accepted and its group id carried through', () => {
  const verdict = classifyHolder({ status: 200, body: { info: officePolicy() } });
  assert.deepEqual(verdict, { verdict: 'group', groupId: '7', problem: null });
});

test('an address the chain cannot decode is told apart from a plain key', () => {
  // Both messages verbatim from yamale-devnet-2. The second is the one that
  // matters in practice — a transposed pair of characters in a hand-retyped
  // address — and an earlier version of the pattern here said "checksum failed",
  // which matches neither.
  for (const message of [
    'decoding bech32 failed: invalid character not part of charset: 111',
    'decoding bech32 failed: invalid checksum (expected 3xm8uj got 3xm8ju)',
    'invalid checksum (expected 3xm8uj got 3xm8ju)',
  ]) {
    const verdict = classifyHolder({ status: 500, body: { code: 2, message } });
    assert.equal(verdict.verdict, 'malformed', message);
    assert.match(verdict.problem, /cannot read that as an address/);
  }
});

test('a lookup that did not answer is unknown, and still blocks the grant', () => {
  const verdict = classifyHolder({ status: 'unreachable', body: '' });
  assert.equal(verdict.verdict, 'unknown');
  // The distinction that matters: it must not read as a verdict about the
  // address, because the custodian would go and check the address.
  assert.match(verdict.problem, /lookup that failed, and not necessarily the address/);

  const plan = grantPlan(grantArgs({ holderVerdict: verdict }));
  assert.equal(plan.ready, false, 'an unchecked holder fails closed');
});

test('a 200 that names no group is unknown rather than accepted', () => {
  const verdict = classifyHolder({ status: 200, body: { info: null } });
  assert.equal(verdict.verdict, 'unknown');
  assert.equal(grantPlan(grantArgs({ holderVerdict: verdict })).ready, false);
});

test('a grant is not composed while the holder lookup is still outstanding', () => {
  const plan = grantPlan(grantArgs({ holderVerdict: null }));
  assert.equal(plan.ready, false);
  assert.ok(plan.problems.some((p) => /Waiting on the group-policy lookup/.test(p)));
});

test('a malformed holder address is refused without a chain lookup at all', () => {
  const plan = grantPlan(grantArgs({ holder: 'not-an-address', holderVerdict: null }));
  assert.equal(plan.ready, false);
  assert.ok(plan.problems.some((p) => /not a Yamale account address/.test(p)));
  // And it does not also nag about the lookup, which would be two complaints
  // about one mistake.
  assert.ok(!plan.problems.some((p) => /Waiting on the group-policy lookup/.test(p)));
});

test('an empty holder asks for one rather than reporting a bad address', () => {
  assert.match(checkAddress('', 'address of the office').problem, /^Give the address of the office/);
});

test('an address that is only the prefix is refused, not accepted for starting right', () => {
  // A mutation pass found the length floor was unguarded: "yml1" alone passed,
  // which is the shape of a half-pasted address rather than a mistyped one — and
  // a half-paste is what happens when a custodian copies out of a terminal.
  for (const stub of ['yml1', 'yml1abc', POLICY.slice(0, 41), B.slice(0, 41)]) {
    assert.match(checkAddress(stub).problem, /not a Yamale account address/, stub);
  }
  assert.match(checkAddress('ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j').problem,
    /not a Yamale account address/, 'a validator operator address is not an account');
  // The real ones, at both lengths this chain uses: 42 for a key account and 62
  // for a group policy.
  assert.equal(B.length, 42);
  assert.equal(POLICY.length, 62);
  assert.equal(checkAddress(B).problem, null);
  assert.equal(checkAddress(POLICY).problem, null);
  // And a grant is not composed from a truncated one.
  assert.equal(grantPlan(grantArgs({ holder: 'yml1' })).ready, false);
});

// --- inspecting the office before signing ---

test('the office a role goes to is shown as its membership and threshold', () => {
  const office = officeSummary({
    policy: officePolicy(),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  assert.equal(office.threshold, 2);
  assert.equal(office.totalWeight, 3);
  assert.equal(office.groupId, '7');
  assert.deepEqual(office.members.map((m) => m.name), ['Adaeze', 'Bola', 'Chidi']);
  assert.deepEqual(office.members.map((m) => m.fingerprint), ['AAAA-1111', 'BBBB-2222', 'CCCC-3333']);
  assert.deepEqual(office.concerns, [], 'a two-of-three office administering itself is clean');
});

test('an office that is really the foundation is flagged, with the reason it happens', () => {
  const office = officeSummary({
    policy: officePolicy({ address: POLICY, admin: POLICY }),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  assert.ok(office.concerns.some((c) => /the foundation itself/.test(c)));
  // The cause, not just the fact. This is the bug a live ceremony run actually
  // hit, and the address looked correct.
  assert.ok(office.concerns.some((c) => /sequence number/.test(c)));
  assert.ok(office.concerns.some((c) => /policy sequence 1/.test(c)));

  // It is a concern, not a refusal: the chain permits it on purpose.
  const plan = grantPlan(grantArgs({ holder: POLICY, office }));
  assert.equal(plan.ready, true);
  assert.ok(plan.concerns.some((c) => /the foundation itself/.test(c)));
});

test('a group somebody else administers is flagged as a threshold that is advisory', () => {
  const office = officeSummary({
    policy: officePolicy({ admin: OUTSIDER }),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  assert.ok(office.concerns.some((c) => /admin is/.test(c) && /advisory/.test(c)));
  assert.ok(office.concerns.some((c) => /granting it to that outsider/.test(c)));
});

test('an office that decides on one signature is flagged as not an office', () => {
  const office = officeSummary({
    policy: officePolicy({ decision_policy: { threshold: '1' } }),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  assert.ok(office.concerns.some((c) => /decides on 1 signature/.test(c)));
  assert.ok(office.concerns.some((c) => /single member can act alone/.test(c)));
});

test('an office whose threshold exceeds its weight can never use what it is granted', () => {
  const office = officeSummary({
    policy: officePolicy({ decision_policy: { threshold: '4' } }),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  assert.ok(office.concerns.some((c) => /never reach its own threshold/.test(c)));
});

test('an unreadable member weight is reported, never counted as zero', () => {
  const members = officeMembers();
  members[0].member.weight = '1.5';
  const office = officeSummary({ policy: officePolicy(), members, foundationAddress: POLICY });
  assert.equal(office.members.find((m) => m.name === 'Adaeze').weight, null);
  assert.equal(office.totalWeight, 2, 'the unreadable weight is excluded, not read as zero');
  assert.ok(office.concerns.some((c) => /could not be read/.test(c)));
});

test('an unreadable threshold is null rather than zero', () => {
  const office = officeSummary({
    policy: officePolicy({ decision_policy: { threshold: '' } }),
    members: officeMembers(),
  });
  assert.equal(office.threshold, null);
  // A zero would have tripped the "decides on 0 signatures" branch and told a
  // custodian something the chain never said.
  assert.ok(!office.concerns.some((c) => /decides on/.test(c)));
});

test('an office with no members returned is not shown as an empty roster', () => {
  const office = officeSummary({ policy: officePolicy(), members: [] });
  assert.ok(office.concerns.some((c) => /returned no members/.test(c)));
});

test("the office's concerns reach the plan a custodian signs", () => {
  const office = officeSummary({
    policy: officePolicy({ admin: OUTSIDER }),
    members: officeMembers(),
    foundationAddress: POLICY,
  });
  const plan = grantPlan(grantArgs({ office }));
  assert.ok(plan.concerns.some((c) => /advisory/.test(c)),
    'inspecting the office is pointless if the finding does not travel');
});

// --- the composed grant ---

test('a grant composes the message the chain registered, with the enum by name', () => {
  const plan = grantPlan(grantArgs({ role: 'ROLE_PAYMENTS_AUTHORITY', jurisdiction: 'za' }));
  assert.equal(plan.ready, true);
  assert.deepEqual(plan.problems, []);
  assert.deepEqual(plan.messages, [{
    // Verified against cdc.MarshalInterfaceJSON on the chain's own types: the
    // proto package is blockchain.alias.v1, NOT yamale.blockchain.alias.v1,
    // which is only the prefix on the module's REST paths.
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY,
    holder: OFFICE,
    role: 'ROLE_PAYMENTS_AUTHORITY',
    jurisdiction: 'ZA',
  }]);
});

test('the authority on a composed grant is the foundation and nothing else', () => {
  // x/group signs a proposal's messages as the policy account, so any other
  // authority passes submission, collects three signatures, and fails at
  // execution.
  const m = grantRoleMessage({
    policyAddress: POLICY, holder: OFFICE, role: 'ROLE_SUPERVISOR', jurisdiction: 'ZA',
  });
  assert.equal(m.authority, POLICY);
  const r = revokeRoleMessage({
    policyAddress: POLICY, holder: OFFICE, role: 'ROLE_SUPERVISOR', jurisdiction: 'ZA',
  });
  assert.equal(r.authority, POLICY);
  assert.equal(r['@type'], '/blockchain.alias.v1.MsgRevokeRole');
});

test('the composed grant is normalised, so a lowercase code is not sent as typed', () => {
  const plan = grantPlan(grantArgs({ jurisdiction: 'sn' }));
  assert.equal(plan.messages[0].jurisdiction, 'SN');
});

test('a grant of a role nothing can use still composes, and says so', () => {
  for (const role of ['ROLE_ENFORCEMENT_AUTHORITY', 'ROLE_SUPERVISOR']) {
    const plan = grantPlan(grantArgs({ role }));
    assert.equal(plan.ready, true, `${role} is still grantable`);
    assert.ok(plan.concerns.includes(roleByName(role).caveat),
      `${role} must carry its caveat to the point of signing`);
  }
});

test('a grant of a working role carries no caveat', () => {
  const plan = grantPlan(grantArgs({ role: 'ROLE_MONETARY_AUTHORITY' }));
  assert.deepEqual(plan.concerns, []);
});

test('an unset role is refused with the proto3 reason', () => {
  const unset = grantPlan(grantArgs({ role: 'ROLE_UNSPECIFIED' }));
  assert.equal(unset.ready, false);
  assert.ok(unset.problems.some((p) => /ROLE_UNSPECIFIED is refused/.test(p)));

  const missing = grantPlan(grantArgs({ role: '' }));
  assert.equal(missing.ready, false);
  assert.ok(missing.problems.some((p) => /Choose the role/.test(p)));
});

test('a grant naming the chain-wide scope composes nothing', () => {
  const plan = grantPlan(grantArgs({ jurisdiction: '*' }));
  assert.equal(plan.ready, false);
  assert.deepEqual(plan.messages, []);
  assert.ok(plan.problems.some((p) => /may not grant the chain-wide scope/.test(p)));
});

test('re-granting an existing triple is allowed and described as a rewrite', () => {
  const existing = [{
    holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA',
    granted_by: POLICY, granted_at_height: '27100',
  }];
  const plan = grantPlan(grantArgs({ existingGrants: existing }));
  assert.equal(plan.ready, true, 'the chain accepts it; it rewrites attribution');
  assert.ok(plan.concerns.some((c) => /already holds ROLE_REGISTRY_AUTHORITY in ZA/.test(c)));
  assert.ok(plan.concerns.some((c) => /27100/.test(c)), 'the height it was first granted at');
});

test('a grant of a different role to the same office is not reported as a duplicate', () => {
  const existing = [{
    holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA', granted_by: POLICY,
  }];
  const plan = grantPlan(grantArgs({ role: 'ROLE_MONETARY_AUTHORITY', existingGrants: existing }));
  assert.deepEqual(plan.concerns, []);
});

test('a grant of the same role in a different country is not a duplicate either', () => {
  const existing = [{
    holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA', granted_by: POLICY,
  }];
  const plan = grantPlan(grantArgs({ jurisdiction: 'SN', existingGrants: existing }));
  assert.deepEqual(plan.concerns, []);
  assert.equal(plan.messages[0].jurisdiction, 'SN');
});

// --- the composed revocation ---

const HELD = [
  {
    holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA',
    granted_by: POLICY, granted_at_height: '27100',
  },
  {
    holder: OFFICE, role: 'ROLE_SUPERVISOR', jurisdiction: 'SN',
    granted_by: OUTSIDER, granted_at_height: '9',
  },
];

const revokeArgs = (over = {}) => ({
  policyAddress: POLICY,
  holder: OFFICE,
  role: 'ROLE_REGISTRY_AUTHORITY',
  jurisdiction: 'ZA',
  existingGrants: HELD,
  ...over,
});

test('a revocation composes the message the chain registered', () => {
  const plan = revokePlan(revokeArgs());
  assert.equal(plan.ready, true);
  assert.deepEqual(plan.messages, [{
    '@type': '/blockchain.alias.v1.MsgRevokeRole',
    authority: POLICY,
    holder: OFFICE,
    role: 'ROLE_REGISTRY_AUTHORITY',
    jurisdiction: 'ZA',
  }]);
});

test('revoking a grant that was never made is refused, and what is held is listed', () => {
  const plan = revokePlan(revokeArgs({ role: 'ROLE_MONETARY_AUTHORITY' }));
  assert.equal(plan.ready, false);
  assert.deepEqual(plan.messages, []);
  const [problem] = plan.problems;
  assert.match(problem, /does not hold ROLE_MONETARY_AUTHORITY in ZA/);
  assert.match(problem, /ROLE_REGISTRY_AUTHORITY in ZA/, 'what it does hold, so the typo is visible');
  assert.match(problem, /ROLE_SUPERVISOR in SN/);
});

test('revoking the right role in the wrong country is refused', () => {
  // The country is part of what is revoked rather than implied, so this is the
  // mistake the chain refuses and the one a console has to catch: "revoke their
  // supervisor role" is ambiguous between one perimeter and all of them.
  const plan = revokePlan(revokeArgs({ role: 'ROLE_SUPERVISOR', jurisdiction: 'ZA' }));
  assert.equal(plan.ready, false);
  assert.match(plan.problems[0], /does not hold ROLE_SUPERVISOR in ZA/);

  const right = revokePlan(revokeArgs({ role: 'ROLE_SUPERVISOR', jurisdiction: 'SN' }));
  assert.equal(right.ready, true);
});

test('a holder with no grants at all is told that, not shown an empty list', () => {
  const plan = revokePlan(revokeArgs({ existingGrants: [] }));
  assert.equal(plan.ready, false);
  assert.match(plan.problems[0], /holds no role grants at all/);
});

test('a revocation is not composed while the grant list is still outstanding', () => {
  const plan = revokePlan(revokeArgs({ existingGrants: null }));
  assert.equal(plan.ready, false);
  assert.ok(plan.problems.some((p) => /Waiting on this holder's grants/.test(p)));
});

test('a revocation does NOT require the holder to be a group', () => {
  // Deliberate, and the opposite of the grant rule. A grant that somehow reached
  // a plain key is precisely the grant that most needs removing, and demanding a
  // group here would make the bad grant the one nobody could revoke.
  const plan = revokePlan(revokeArgs({
    holder: B,
    existingGrants: [{ holder: B, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA', granted_by: POLICY }],
  }));
  assert.equal(plan.ready, true);
  assert.equal(plan.messages[0].holder, B);
});

test('revoking a grant governance made is allowed and flagged as what it is', () => {
  const plan = revokePlan(revokeArgs({ role: 'ROLE_SUPERVISOR', jurisdiction: 'SN' }));
  assert.equal(plan.ready, true);
  assert.ok(plan.concerns.some((c) => /not by the foundation/.test(c)));
  assert.ok(plan.concerns.some((c) => /reduction of the validator set's power/.test(c)));
});

test("revoking the foundation's own grant carries no such flag", () => {
  const plan = revokePlan(revokeArgs());
  assert.deepEqual(plan.concerns, []);
});

test('a revocation naming the chain-wide scope composes nothing', () => {
  const plan = revokePlan(revokeArgs({ jurisdiction: '*' }));
  assert.equal(plan.ready, false);
  assert.deepEqual(plan.messages, []);
  assert.ok(plan.problems.some((p) => /may not grant the chain-wide scope/.test(p)));
});

test('an unpicked grant is one complaint, not two', () => {
  // A revocation names a role and a country together, so an unset pair is one
  // mistake. Two sentences here would ask a custodian to "name the country this
  // role is being granted in" on the form that takes one away.
  const plan = revokePlan(revokeArgs({ role: '', jurisdiction: '' }));
  assert.equal(plan.ready, false);
  assert.deepEqual(plan.problems, ['Choose which grant to remove.']);
});

test('a half-picked grant still gets both halves checked', () => {
  // The pairing above must not become a way for one bad half to travel unchecked.
  const noCountry = revokePlan(revokeArgs({ role: 'ROLE_SUPERVISOR', jurisdiction: '' }));
  assert.ok(noCountry.problems.some((p) => /Name the country/.test(p)));

  const noRole = revokePlan(revokeArgs({ role: '', jurisdiction: 'ZA' }));
  assert.ok(noRole.problems.some((p) => /Choose which grant to remove/.test(p)));

  const chainWide = revokePlan(revokeArgs({ role: '', jurisdiction: '*' }));
  assert.ok(chainWide.problems.some((p) => /may not grant the chain-wide scope/.test(p)),
    'an unset role must not hide a scope only governance may touch');
});

test('a revocation with an unset role is refused with the revocation reason', () => {
  const plan = revokePlan(revokeArgs({ role: 'ROLE_UNSPECIFIED' }));
  assert.equal(plan.ready, false);
  assert.ok(plan.problems.some((p) => /Revoke whatever role was left unset/.test(p)));
});

test('a grant is found by role and country together, never by either alone', () => {
  assert.equal(findGrant(HELD, 'ROLE_SUPERVISOR', 'SN').granted_by, OUTSIDER);
  assert.equal(findGrant(HELD, 'ROLE_SUPERVISOR', 'ZA'), null);
  assert.equal(findGrant(HELD, 'ROLE_REGISTRY_AUTHORITY', 'SN'), null);
  assert.equal(findGrant(null, 'ROLE_SUPERVISOR', 'SN'), null);
});

// --- reading one on the voting screen ---

test('a role grant in a proposal is described in words, not as a type URL', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY,
    holder: OFFICE,
    role: 'ROLE_REGISTRY_AUTHORITY',
    jurisdiction: 'ZA',
  }, { policyAddress: POLICY, names: { [OFFICE]: 'South Africa lands commission' } });

  assert.equal(d.understood, true);
  assert.match(d.headline, /Grant Registry authority in ZA to South Africa lands commission/);
  assert.deepEqual(d.concerns, []);
  assert.ok(d.detail.some((x) => x.label === 'What consults it' && /x\/land/.test(x.value)));
});

test('a revocation in a proposal reads as a removal, not as a grant', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgRevokeRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_PAYMENTS_AUTHORITY', jurisdiction: 'SN',
  }, { policyAddress: POLICY });
  assert.match(d.headline, /^Revoke Payments authority in SN from/);
  assert.ok(d.detail.some((x) => x.label === 'Taken from'));
});

test('a grant of a role nothing can use is flagged on the voting screen too', () => {
  // The custodian who votes is not the custodian who composed it, so the caveat
  // has to be on the screen where the decision is taken.
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_SUPERVISOR', jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.equal(d.understood, true);
  assert.ok(d.concerns.some((c) => /Nothing on this chain consults this role/.test(c)));
});

test('a revocation of a dead role is not flagged with the grant caveat', () => {
  // Removing a role nothing consults has no such caveat: it takes away exactly
  // what the grant conferred, which is nothing, and saying otherwise would be
  // noise on the one action that should be easy.
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgRevokeRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_SUPERVISOR', jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.deepEqual(d.concerns, []);
});

test('a grant signed by anybody but the foundation is flagged on the voting screen', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: OUTSIDER, holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.ok(d.concerns.some((c) => /not the foundation account/.test(c)));
  assert.ok(d.concerns.some((c) => /fail at execution/.test(c)));
});

test('a grant of the chain-wide scope is flagged as one that will fail', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: '*',
  }, { policyAddress: POLICY });
  assert.match(d.headline, /in \*/);
  assert.ok(d.concerns.some((c) => /only governance may grant or revoke/.test(c)));
});

test('a grant naming a country that does not exist is flagged', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'NX',
  }, { policyAddress: POLICY });
  assert.ok(d.concerns.some((c) => /"NX" is not an assigned ISO/.test(c)));
});

test('a grant with the role left unset is flagged rather than described as a role', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: OFFICE, jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.ok(d.concerns.some((c) => /The role is unset/.test(c)));
  assert.ok(!d.detail.some((x) => x.label === 'What consults it'));
});

test('a grant to the foundation itself is flagged on the voting screen', () => {
  const d = describeMessage({
    '@type': '/blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: POLICY, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.ok(d.concerns.some((c) => /holder is the foundation itself/.test(c)));
  assert.ok(d.concerns.some((c) => /sequence number alone/.test(c)));
});

test('the type URL a REST path would suggest is not understood', () => {
  // /yamale/blockchain/alias/v1/… is the REST prefix; blockchain.alias.v1 is the
  // proto package. Guessing the first as a type URL yields a message the
  // interface registry cannot resolve, and this page must not describe it as
  // though it would work.
  const d = describeMessage({
    '@type': '/yamale.blockchain.alias.v1.MsgGrantRole',
    authority: POLICY, holder: OFFICE, role: 'ROLE_REGISTRY_AUTHORITY', jurisdiction: 'ZA',
  }, { policyAddress: POLICY });
  assert.equal(d.understood, false);
});

test('a composed grant document carries no carriage return', () => {
  // Every command on this page ends up in a shell, and a CR gives
  // `$'\r': command not found` — an error naming neither the cause nor the file.
  const plan = grantPlan(grantArgs());
  const doc = proposalDocument({
    policyAddress: POLICY, proposer: A, messages: plan.messages,
    title: 'Grant the South Africa lands commission its registry role',
    summary: 'Two of three, verified against the ceremony record.',
  });
  assert.equal(doc.indexOf('\r'), -1);
  assert.equal(shellSafe(doc), doc);
  assert.equal(shellSafe(submitCommand({ proposer: A })), submitCommand({ proposer: A }));
});
