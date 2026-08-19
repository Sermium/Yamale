import assert from 'node:assert/strict';
import { test } from 'node:test';

import { assessProposal, toGovParams } from './gov.ts';
import { toProposal } from './staking.ts';

const params = toGovParams({
  params: {
    quorum: '0.334000000000000000',
    threshold: '0.500000000000000000',
    veto_threshold: '0.334000000000000000',
    min_deposit: [{ denom: 'uyml', amount: '10000000' }],
    voting_period: '172800s',
    max_deposit_period: '172800s',
  },
});

const BONDED = '100000000';

function proposal(overrides: Record<string, unknown> = {}) {
  return toProposal(
    {
      id: '1',
      title: 'Admit a new validator',
      status: 'PROPOSAL_STATUS_VOTING_PERIOD',
      final_tally_result: { yes_count: '0', no_count: '0', abstain_count: '0', no_with_veto_count: '0' },
      total_deposit: [{ denom: 'uyml', amount: '10000000' }],
      ...overrides,
    },
    () => 'does something',
  );
}

test('governance parameters are read as fractions', () => {
  assert.equal(params.quorum, 0.334);
  assert.equal(params.threshold, 0.5);
  assert.equal(params.votingPeriodSeconds, 172800);
});

// The trap this exists to prevent: a lopsided yes vote that still fails,
// because too little of the staked supply turned up.
test('an overwhelming yes below quorum is reported as failing', () => {
  const a = assessProposal(
    proposal({ final_tally_result: { yes_count: '20000000', no_count: '0', abstain_count: '0', no_with_veto_count: '0' } }),
    params,
    BONDED,
  );

  assert.equal(a.yesShare, 1, 'everyone who voted said yes');
  assert.equal(a.quorumMet, false, 'but only 20% of stake voted');
  assert.equal(a.wouldPass, false);
  assert.match(a.verdict, /not enough of the staked tokens have voted/i);
  assert.match(a.blocker ?? '', /further 13\.4% of all staked tokens/);
});

test('a proposal clearing quorum and threshold is reported as passing', () => {
  const a = assessProposal(
    proposal({
      final_tally_result: { yes_count: '30000000', no_count: '10000000', abstain_count: '0', no_with_veto_count: '0' },
    }),
    params,
    BONDED,
  );

  assert.equal(a.quorumMet, true);
  assert.equal(a.yesShare, 0.75);
  assert.equal(a.wouldPass, true);
  assert.equal(a.blocker, null);
  assert.match(a.verdict, /on course to pass/i);
});

// Abstaining is "present but not opposed": it helps reach quorum without
// counting against the proposal. Getting this backwards would show a passing
// proposal as failing.
test('abstentions count toward quorum but not against the yes share', () => {
  const a = assessProposal(
    proposal({
      final_tally_result: { yes_count: '20000000', no_count: '5000000', abstain_count: '15000000', no_with_veto_count: '0' },
    }),
    params,
    BONDED,
  );

  assert.equal(a.quorumProgress, 0.4, 'all four tallies count toward quorum');
  assert.equal(a.yesShare, 0.8, '20 of the 25 decisive votes');
  assert.equal(a.wouldPass, true);
});

test('a veto minority rejects a proposal the majority supports', () => {
  const a = assessProposal(
    proposal({
      final_tally_result: { yes_count: '30000000', no_count: '0', abstain_count: '0', no_with_veto_count: '20000000' },
    }),
    params,
    BONDED,
  );

  assert.equal(a.yesShare, 0.6, 'a clear majority of the decisive votes');
  assert.equal(a.vetoed, true);
  assert.equal(a.wouldPass, false);
  assert.match(a.verdict, /vetoes/);
});

test('a chain with nothing bonded does not divide by zero', () => {
  const a = assessProposal(proposal(), params, '0');
  assert.equal(a.quorumProgress, 0);
  assert.equal(a.wouldPass, false);
});

test('an untouched proposal reports zeroes rather than NaN', () => {
  const a = assessProposal(proposal(), params, BONDED);
  assert.equal(a.yesShare, 0);
  assert.equal(a.quorumProgress, 0);
});

// A proposal in the deposit period is not failing — it has not started. Saying
// so, and saying how far off it is, is the difference between a dead-looking
// list and an actionable one.
test('a proposal still raising its deposit says so, with progress', () => {
  const a = assessProposal(
    proposal({
      status: 'PROPOSAL_STATUS_DEPOSIT_PERIOD',
      total_deposit: [{ denom: 'uyml', amount: '2500000' }],
    }),
    params,
    BONDED,
  );

  assert.equal(a.depositProgress, 0.25);
  assert.match(a.verdict, /needs a larger deposit/);
  assert.equal(a.blocker, null, 'the blocker is about votes, and voting has not started');
});

test('a decided proposal states its outcome rather than a forecast', () => {
  const passed = assessProposal(proposal({ status: 'PROPOSAL_STATUS_PASSED' }), params, BONDED);
  assert.match(passed.verdict, /Passed and applied/);

  const failed = assessProposal(proposal({ status: 'PROPOSAL_STATUS_FAILED' }), params, BONDED);
  assert.match(failed.verdict, /failed to apply/);
});

test('missing governance parameters fall back rather than producing NaN', () => {
  const empty = toGovParams({});
  assert.equal(empty.quorum, 0.334);
  assert.equal(empty.threshold, 0.5);
  assert.deepEqual(empty.minDeposit, []);

  const a = assessProposal(proposal(), empty, BONDED);
  assert.equal(a.depositProgress, 0, 'no minimum known means no progress claim');
});
