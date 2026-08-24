import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { buildFeed, decode, everydayFeed, messageLabel, tierOf, voteWord, type FeedTransaction } from './feed.ts';
import { decodeMessage, type DecodeContext } from '../../sdk/src/decode.ts';

/**
 * Fixtures taken verbatim from yamale-devnet-2, because the rows this module
 * exists to fix are the rows that chain is actually serving. A synthetic
 * payload would have let me get the field names wrong in the test and the
 * implementation identically.
 */

const CUSTODIAN_A = 'yml15m9y2wd42lll06tqnuhz0q9jx64a78pt72wtfw';
const CUSTODIAN_B = 'yml13duwtfpckr0ef44jd2arc8l3ss0lm54xtnutc2';
const CUSTODIAN_C = 'yml1d0dc0lpg093kqyga9ccfmqcnlx9u6rs0dpjka9';
const ALICE = 'yml1rxtapcknmh58vngn5xmkm4rd7zf4knpuwa6szg';
const POLICY = 'yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr';

const SET_JURISDICTION = {
  '@type': '/blockchain.alias.v1.MsgSetJurisdiction',
  recorder: POLICY,
  account: ALICE,
  country: 'KE',
};

const SUBMIT = {
  '@type': '/cosmos.group.v1.MsgSubmitProposal',
  group_policy_address: POLICY,
  proposers: [CUSTODIAN_A],
  metadata: '',
  messages: [SET_JURISDICTION],
  exec: 'EXEC_UNSPECIFIED',
  title: 'Correct alice to KE',
  summary: 'Correct alice to KE',
};

const vote = (voter: string) => ({
  '@type': '/cosmos.group.v1.MsgVote',
  proposal_id: '2',
  voter,
  option: 'VOTE_OPTION_YES',
  metadata: '',
  exec: 'EXEC_UNSPECIFIED',
});

const EXEC = {
  '@type': '/cosmos.group.v1.MsgExec',
  proposal_id: '2',
  executor: CUSTODIAN_A,
};

const SEND = {
  '@type': '/cosmos.bank.v1beta1.MsgSend',
  from_address: CUSTODIAN_A,
  to_address: ALICE,
  amount: [{ denom: 'uyml', amount: '1250500000' }],
};

const UPDATE_PARAMS = {
  '@type': '/blockchain.oracle.v1.MsgUpdateParams',
  authority: POLICY,
  params: {},
};

/** A transaction, as the client hands it over: messages already decoded. */
function tx(
  height: number,
  messages: Array<Record<string, any>>,
  options: {
    succeeded?: boolean;
    events?: Array<{ type: string; attributes: Array<{ key: string; value: string }> }>;
    error?: string;
    ctx?: DecodeContext;
  } = {},
): FeedTransaction {
  return {
    hash: `HASH${height}`,
    height,
    timestamp: '2026-08-23T18:00:00Z',
    succeeded: options.succeeded ?? true,
    messages: messages.map((m) => decodeMessage(m, options.ctx ?? {})),
    error: options.error ? { message: options.error } : undefined,
    raw: { tx_response: { events: options.events ?? [] } },
  };
}

const submitEvent = (id: string) => ({
  type: 'cosmos.group.v1.EventSubmitProposal',
  attributes: [{ key: 'proposal_id', value: `"${id}"` }],
});

const execEvents = (id: string, yes: number, result = 'PROPOSAL_EXECUTOR_RESULT_SUCCESS') => [
  {
    type: 'cosmos.group.v1.EventProposalPruned',
    attributes: [
      { key: 'proposal_id', value: `"${id}"` },
      { key: 'status', value: '"PROPOSAL_STATUS_ACCEPTED"' },
      {
        key: 'tally_result',
        value: `{"yes_count":"${yes}","abstain_count":"0","no_count":"0","no_with_veto_count":"0"}`,
      },
    ],
  },
  {
    type: 'cosmos.group.v1.EventExec',
    attributes: [
      { key: 'proposal_id', value: `"${id}"` },
      { key: 'result', value: `"${result}"` },
    ],
  },
];

/** The window the live chain was serving, newest first. */
function devnetWindow(): FeedTransaction[] {
  return [
    tx(28873, [EXEC], { events: execEvents('2', 3) }),
    tx(28871, [vote(CUSTODIAN_C)]),
    tx(28869, [vote(CUSTODIAN_B)]),
    tx(28868, [vote(CUSTODIAN_A)]),
    tx(28865, [SUBMIT], { events: [submitEvent('2')] }),
  ];
}

// --- the decoders the shared one is missing -----------------------------------

test('a message the shared decoder does not know still reads as a sentence', () => {
  // Before: "Set Jurisdiction on the alias module" — a type URL with the
  // slashes taken out. x/alias messages are the top of the live feed.
  const bare = decodeMessage(SET_JURISDICTION);
  assert.equal(bare.kind, 'other', 'the shared decoder has no entry for this');

  const better = decode(SET_JURISDICTION);
  assert.equal(better.summary, "yml1rxtapc…6szg's country was recorded as KE");
  assert.equal(better.kind, 'governance');
});

test('a known name replaces the address in the sentence', () => {
  const better = decode(SET_JURISDICTION, { names: { [ALICE]: 'Alice' } });
  assert.equal(better.summary, "Alice's country was recorded as KE");
});

test('the local table never overrides a decoder the SDK already has', () => {
  // Two decoders disagreeing about the same message is how two surfaces come to
  // describe one transaction differently.
  const send = decode(SEND);
  assert.equal(send.summary, decodeMessage(SEND).summary);
});

// --- outcome versus step ------------------------------------------------------

test('a vote is a step and an execution is an outcome', () => {
  assert.equal(tierOf(decode(vote(CUSTODIAN_A))), 'step');
  assert.equal(tierOf(decode(EXEC)), 'outcome');
  assert.equal(tierOf(decode(SUBMIT)), 'step');
});

test('money moving is an outcome, housekeeping is routine', () => {
  assert.equal(tierOf(decode(SEND)), 'outcome');
  assert.equal(tierOf(decode(UPDATE_PARAMS)), 'routine');
});

test('nothing that failed is presented as an outcome', () => {
  // Nothing became true. A row that reads like a completed transfer because the
  // colour is the only difference is the failure mode this guards.
  assert.notEqual(tierOf(decode(SEND), true), 'outcome');
});

// --- what an execution actually did ------------------------------------------

test('an executed request is described by its effect, not by its number', () => {
  const feed = buildFeed(devnetWindow(), { names: { [ALICE]: 'Alice' } });
  const outcome = feed.find((e) => e.typeUrl === '/cosmos.group.v1.MsgExec')!;

  // The SDK can only say "Request 2 reached enough approvals and was carried
  // out", because MsgExec carries a proposal id and nothing else. The effect
  // comes from the submission a few rows down — the only place it survives,
  // since x/group prunes a proposal the moment it runs.
  assert.equal(outcome.headline, "Alice's country was recorded as KE");
  assert.equal(outcome.tier, 'outcome');
  assert.deepEqual(outcome.approval?.tally, { yes: 3, no: 0, abstain: 0, veto: 0 });
  assert.equal(outcome.approval?.ran, 'success');
  assert.equal(outcome.approval?.title, 'Correct alice to KE');
});

test('an execution with no submission in the window says what it can', () => {
  // Scrolled past, or a window that starts after the request was made. The row
  // must not claim knowledge it does not have.
  const feed = buildFeed([tx(28873, [EXEC], { events: execEvents('2', 3) })]);
  const outcome = feed[0]!;
  assert.match(outcome.headline, /Request 2/);
  assert.deepEqual(outcome.approval?.actions, []);
  assert.deepEqual(outcome.approval?.tally, { yes: 3, no: 0, abstain: 0, veto: 0 });
});

test('an approval that passed but whose action failed is not an outcome', () => {
  // The custodians agreed and the chain refused the message. Reporting that as
  // done would be the most expensive wrong sentence on the page.
  const feed = buildFeed([
    tx(28873, [EXEC], { events: execEvents('2', 3, 'PROPOSAL_EXECUTOR_RESULT_FAILURE') }),
    tx(28865, [SUBMIT], { events: [submitEvent('2')] }),
  ]);
  const exec = feed.find((e) => e.typeUrl === '/cosmos.group.v1.MsgExec')!;
  assert.equal(exec.approval?.ran, 'failure');
  assert.notEqual(exec.tier, 'outcome');
});

test('a submitted request is described by what it would do', () => {
  const feed = buildFeed(devnetWindow());
  const submission = feed.find((e) => e.typeUrl === '/cosmos.group.v1.MsgSubmitProposal')!;
  assert.equal(submission.headline, 'Approval requested: Correct alice to KE');
  assert.equal(submission.tier, 'step');
});

test('a vote names the request it is about', () => {
  const feed = buildFeed(devnetWindow());
  const votes = feed.filter((e) => e.typeUrl === '/cosmos.group.v1.MsgVote');
  assert.equal(votes.length, 3);
  assert.equal(votes[0]!.request?.option, 'yes');
  assert.equal(votes[0]!.request?.title, 'Correct alice to KE');
});

// --- the everyday view -------------------------------------------------------

test('a concluded procedure collapses to its outcome', () => {
  // The whole complaint, in one assertion: four rows of equal weight for one
  // event become one row that says what happened.
  const full = buildFeed(devnetWindow());
  assert.equal(full.length, 5, 'the expert view keeps every message');

  const everyday = everydayFeed(full);
  assert.equal(everyday.length, 1);
  assert.equal(everyday[0]!.tier, 'outcome');
  assert.match(everyday[0]!.headline, /country was recorded as KE/);
});

test('votes on a request still in flight are kept', () => {
  // Collapsing an unfinished procedure would hide the only rows there are.
  const inFlight = [
    tx(28871, [vote(CUSTODIAN_C)]),
    tx(28865, [SUBMIT], { events: [submitEvent('2')] }),
  ];
  const everyday = everydayFeed(buildFeed(inFlight));
  assert.equal(everyday.length, 2);
});

test('housekeeping is dropped and money is kept', () => {
  const feed = buildFeed([tx(100, [UPDATE_PARAMS]), tx(99, [SEND])]);
  const everyday = everydayFeed(feed);
  assert.equal(everyday.length, 1);
  assert.equal(everyday[0]!.typeUrl, '/cosmos.bank.v1beta1.MsgSend');
});

test('a failure is never hidden from the everyday view', () => {
  // The live chain has two of these — a jurisdiction correction refused because
  // only a foundation administrator may make it. Somebody whose payment was
  // refused needs the row that says so more than any other row on the page.
  const failed = tx(28862, [SET_JURISDICTION], {
    succeeded: false,
    error: "this account's jurisdiction is already recorded",
  });
  const everyday = everydayFeed(buildFeed([failed]));
  assert.equal(everyday.length, 1);
  assert.equal(everyday[0]!.failed, true);
  assert.match(everyday[0]!.failureReason ?? '', /already recorded/);
});

test('names resolved after the fetch still reach the sentence', () => {
  // The client decodes on the way in, before the page has resolved any account
  // to a name. If the feed reused that first decoding, the sentence would name a
  // party by its bech32 prefix while a chip two lines below showed the same
  // account's user ID — which is one row disagreeing with itself about who was
  // paid.
  const txs = [tx(99, [SEND])]; // decoded with no names at all
  const feed = buildFeed(txs, {
    names: { [CUSTODIAN_A]: 'KE-M1BM-Z66Y-P', [ALICE]: 'NG-K3M9-7QRT-5' },
  });
  assert.equal(feed[0]!.headline, 'KE-M1BM-Z66Y-P sent 1,250.5 YML to NG-K3M9-7QRT-5');
});

// --- amounts reach the row ---------------------------------------------------

test('a row carries the amount in base units for the display edge to convert', () => {
  const feed = buildFeed([tx(99, [SEND])]);
  assert.deepEqual(feed[0]!.coins, [{ denom: 'uyml', amount: '1250500000' }]);
});

test('an approved payment carries the amount its inner message moved', () => {
  // The reason the old feed had no amounts anywhere: the money is in the
  // proposal's inner message, and nothing was looking there.
  const payment = {
    '@type': '/cosmos.group.v1.MsgSubmitProposal',
    group_policy_address: POLICY,
    proposers: [CUSTODIAN_A],
    messages: [SEND],
    title: 'Pay Alice',
  };
  const feed = buildFeed([
    tx(300, [EXEC], { events: execEvents('2', 3) }),
    tx(299, [payment], { events: [submitEvent('2')] }),
  ]);
  const outcome = feed.find((e) => e.typeUrl === '/cosmos.group.v1.MsgExec')!;
  assert.deepEqual(outcome.coins, [{ denom: 'uyml', amount: '1250500000' }]);
});

// --- small helpers ----------------------------------------------------------

test('vote options read as words', () => {
  assert.equal(voteWord('VOTE_OPTION_YES'), 'yes');
  assert.equal(voteWord('VOTE_OPTION_NO_WITH_VETO'), 'no with veto');
  assert.equal(voteWord('VOTE_OPTION_ONE'), 'yes', 'the numeric aliases the API also uses');
  assert.equal(voteWord(undefined), '');
});

test('a type URL becomes a readable badge', () => {
  assert.equal(messageLabel('/blockchain.paymsg.v1.MsgSendPayment'), 'Send Payment');
  assert.equal(messageLabel('/cosmos.group.v1.MsgExec'), 'Exec');
});

test('an event attribute is unquoted once, in one place', () => {
  // Attribute values arrive JSON-encoded: the id 2 comes over the wire as the
  // three characters "2". Forgetting that gives a proposal id of '"2"', which
  // matches nothing and silently disables the whole correlation.
  const feed = buildFeed(devnetWindow());
  const outcome = feed.find((e) => e.approval)!;
  assert.equal(outcome.approval?.proposalId, '2');
});
