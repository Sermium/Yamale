import assert from 'node:assert/strict';
import { test } from 'node:test';

import { EMPTY_AMOUNT, formatAmount, formatCoins, poolIdFromDenom, resolveDenom, toDisplayAmount } from './denom.ts';
import { describeThroughput, measure } from './performance.ts';
import { decodeMessage, describeProposalAction, summariseTransaction } from './decode.ts';
import { translateError } from './errors.ts';
import { formatDuration, timeAgo, truncateAddress } from './format.ts';

// ---------------------------------------------------------------- amounts

test('base units convert to display units without losing precision', () => {
  assert.equal(toDisplayAmount('12500000', 6), '12.5');
  assert.equal(toDisplayAmount('1', 6), '0.000001');
  assert.equal(toDisplayAmount('1000000', 6), '1');
  assert.equal(toDisplayAmount('0', 6), '0');
  // Beyond Number.MAX_SAFE_INTEGER: a rounded balance is a wrong balance.
  assert.equal(toDisplayAmount('83633333333325000000', 6), '83633333333325');
});

test('amounts format with symbol and grouping', () => {
  assert.equal(formatAmount('12500000', 'uyml'), '12.5 YML');
  assert.equal(formatAmount('1250500000', 'uyml'), '1,250.5 YML');
  assert.equal(formatAmount('12500000', 'uyml', { withSymbol: false }), '12.5');
});

test('capping decimals truncates rather than rounding up', () => {
  // 0.999999 shown to 2dp must not become 1.00 — somebody acting on a
  // rounded-up balance would have the transaction rejected.
  assert.equal(formatAmount('999999', 'uyml', { maxDecimals: 2 }), '0.99 YML');
});

test('unknown denoms are shown as-is rather than guessed at', () => {
  const info = resolveDenom('ufoo');
  assert.equal(info.exponent, 0, 'guessing an exponent would misstate the amount');
  assert.equal(formatAmount('1234', 'ufoo'), '1,234 ufoo');
});

test('pool share denoms are recognised and named', () => {
  assert.equal(poolIdFromDenom('amm/pool/7'), 7);
  assert.equal(poolIdFromDenom('uyml'), null);
  assert.equal(resolveDenom('amm/pool/7').symbol, 'Pool 7 shares');
});

test('coin lists read as a phrase', () => {
  // An en dash, not the word "nothing": this renders into a figure column a
  // supervisor is reading, and the word was English on every locale. The joiner
  // below is translated for the same reason — it used to put an English "and"
  // between two Arabic amounts.
  assert.equal(formatCoins([]), EMPTY_AMOUNT);
  assert.notEqual(EMPTY_AMOUNT, '0', 'a dash is not a zero: no balance and a zero balance differ');
  assert.equal(formatCoins([{ denom: 'uyml', amount: '5000000' }]), '5 YML');
  assert.equal(
    formatCoins([
      { denom: 'uyml', amount: '5000000' },
      { denom: 'uusd', amount: '2500000' },
    ]),
    '5 YML and 2.5 USD',
  );
});

test('the conjunction survives a consumer that never registered a catalogue', () => {
  // t() answers with the key when nothing is registered, and a library function
  // cannot assume its consumer imported the catalogues first. The failure this
  // guards is a figure column reading "5 YML amount.and 2.5 USD".
  const phrase = formatCoins([
    { denom: 'uyml', amount: '5000000' },
    { denom: 'uusd', amount: '2500000' },
  ]);
  assert.doesNotMatch(phrase, /amount\.and/);
});

// ---------------------------------------------------------------- format

test('addresses truncate but stay recognisable', () => {
  const addr = 'yml12urcg48rnzetfd0645p4d7mcw0g369uursqaqm';
  assert.equal(truncateAddress(addr), 'yml12urcg4…qaqm');
  assert.equal(truncateAddress('short'), 'short');
});

test('elapsed time reads the way somebody would say it', () => {
  const now = new Date('2026-08-10T12:00:00Z');
  assert.equal(timeAgo('2026-08-10T11:59:55Z', now), 'just now');
  assert.equal(timeAgo('2026-08-10T11:58:00Z', now), '2 minutes ago');
  assert.equal(timeAgo('2026-08-10T09:00:00Z', now), '3 hours ago');
  assert.equal(timeAgo('2026-08-08T12:00:00Z', now), '2 days ago');
});

test('durations render as phrases', () => {
  assert.equal(formatDuration(1814400), '21 days');
  assert.equal(formatDuration(3600), '1 hour');
});

// ---------------------------------------------------------------- decode

test('a bank send decodes into a sentence', () => {
  const decoded = decodeMessage({
    '@type': '/cosmos.bank.v1beta1.MsgSend',
    from_address: 'yml12urcg48rnzetfd0645p4d7mcw0g369uursqaqm',
    to_address: 'yml19enua9v98xex94zfuy7vatp3ufhcwfc6gyujqs',
    amount: [{ denom: 'uyml', amount: '12500000' }],
  });

  assert.equal(decoded.kind, 'transfer');
  assert.equal(decoded.title, 'Transfer');
  assert.equal(decoded.summary, 'yml12urcg4…qaqm sent 12.5 YML to yml19enua9…ujqs');
  assert.equal(decoded.everyday, true, 'a transfer is exactly what the simple view is for');
});

test('known names are used in place of addresses', () => {
  const decoded = decodeMessage(
    {
      '@type': '/cosmos.staking.v1beta1.MsgDelegate',
      delegator_address: 'yml1alice',
      validator_address: 'ymlvaloper1x',
      amount: { denom: 'uyml', amount: '1000000' },
    },
    { names: { yml1alice: 'Alice', ymlvaloper1x: 'Validator One' } },
  );

  assert.equal(decoded.summary, 'Alice staked 1 YML with Validator One');
});

test('an ISO 20022 payment surfaces its reference and purpose', () => {
  const decoded = decodeMessage({
    '@type': '/blockchain.paymsg.v1.MsgSendPayment',
    debtor: 'yml1debtor',
    creditor: 'yml1creditor',
    denom: 'uyml',
    amount: '250000000',
    end_to_end_id: 'E2E-0001',
    purpose_code: 'SALA',
    remittance_information: 'March salary',
  });

  assert.equal(decoded.kind, 'payment');
  assert.match(decoded.summary, /paid 250 YML/);
  assert.match(decoded.summary, /for "March salary"/);
  const purpose = decoded.details?.find((d) => d.label === 'Purpose');
  assert.equal(purpose?.value, 'Salary', 'purpose codes must be words, not four-letter codes');
});

test('treasury commitments explain themselves', () => {
  const decoded = decodeMessage({
    '@type': '/blockchain.treasury.v1.MsgCreateLock',
    treasury_id: '3',
    admin: 'yml1admin',
    beneficiary: 'yml1bob',
    denom: 'uyml',
    amount: '400000000',
    lock_type: 'LOCK_TYPE_VESTING',
    revocable: true,
  });

  assert.match(decoded.summary, /committed 400 YML/);
  assert.match(decoded.summary, /vesting over time/);
  assert.equal(decoded.details?.find((d) => d.label === 'Can be cancelled')?.value, 'Yes');
});

test('parameter updates are recognised generically', () => {
  const decoded = decodeMessage({ '@type': '/blockchain.emission.v1.MsgUpdateParams', authority: 'yml1gov' });
  assert.equal(decoded.everyday, false);
  assert.equal(decoded.summary, "Governance updated the emission module's settings");
});

test('an unknown message still produces a readable row', () => {
  const decoded = decodeMessage({ '@type': '/blockchain.future.v1.MsgDoSomethingNew', foo: 1 });
  assert.equal(decoded.title, 'Do Something New');
  assert.equal(decoded.summary, 'Do Something New on the future module');
  assert.equal(decoded.everyday, false, 'unknown messages must not leak into the simple view');
  assert.deepEqual(decoded.raw, { '@type': '/blockchain.future.v1.MsgDoSomethingNew', foo: 1 });
});

test('transactions with several messages summarise honestly', () => {
  const send = decodeMessage({
    '@type': '/cosmos.bank.v1beta1.MsgSend',
    from_address: 'a',
    to_address: 'b',
    amount: [{ denom: 'uyml', amount: '1000000' }],
  });
  assert.equal(summariseTransaction([send, send]), '2 × transfer');
  assert.equal(summariseTransaction([]), 'An empty transaction');

  const vote = decodeMessage({ '@type': '/cosmos.gov.v1.MsgVote', voter: 'a', proposal_id: '1', option: 'VOTE_OPTION_YES' });
  assert.equal(summariseTransaction([send, vote]), '2 actions in one transaction');
});

// ---------------------------------------------------------------- errors

test('insufficient funds keeps the real figures', () => {
  const t = translateError(
    'failed to execute message; message index: 0: insufficient funds: insufficient account funds; 100uyml is smaller than 500uyml',
  );
  assert.equal(t.message, 'Not enough funds');
  assert.match(t.reason ?? '', /holds 100uyml/);
  assert.match(t.reason ?? '', /needs 500uyml/);
  assert.equal(t.retryable, false);
});

test('a slippage rejection explains that nothing was lost', () => {
  const t = translateError('swap output is below the minimum requested amount');
  assert.equal(t.message, 'The price moved');
  assert.equal(t.retryable, true);
  assert.match(t.nextStep ?? '', /try again/i);
});

test('locked treasury funds get their own explanation', () => {
  const t = translateError('treasury 2 has 600000uyml available (1000000 held, 400000 locked), needs 800000');
  assert.equal(t.message, 'Not enough available in the treasury');
  assert.match(t.reason ?? '', /cannot be spent by anyone/);
});

test('an account the chain has never seen is explained, not reported as missing', () => {
  // The exact text CosmJS throws, verbatim from a live devnet: a freshly
  // generated key with nothing sent to it yet. It arrives through the catch
  // path rather than as a transaction result, so if it is not translated the
  // raw sentence is the whole of what a user sees.
  const t = translateError(
    "Account 'yml198crxyyhnk9d563dke4et5jw294rurkdle5kzn' does not exist on chain. Send some tokens there before trying to query sequence.",
  );
  assert.equal(t.message, 'This account has never received anything');
  assert.match(t.reason ?? '', /money first arrives/);
  assert.match(t.nextStep ?? '', /send it any amount|test funds/i);
  // Not conflated with having a balance that is too small: the remedy differs.
  assert.notEqual(t.message, 'Not enough funds');
});

test('a proposal naming a plain account says which field is wrong', () => {
  // Verbatim from yamale-devnet-2, code 38 in block 94,512. Caught by the
  // generic "not found" rule this reads as "the thing this transaction refers
  // to does not exist" — which sends somebody looking at the treasury id when
  // the fault is in the policy address.
  const t = translateError(
    'failed to execute message; message index: 0: load group policy: yml1ywwjfdlwnhh5x50ju4yav35n762ax4pg9fz7j0: not found',
  );
  assert.equal(t.message, 'That is not a shared account');
  assert.match(t.reason ?? '', /policy address/);
  // Truncated, like every other identifier: a full bech32 mid-sentence wraps
  // three times on a phone and pushes the amount above it off screen.
  assert.match(t.reason ?? '', /yml1ywwjfd…/);
  assert.doesNotMatch(t.reason ?? '', /yml1ywwjfdlwnhh5x50ju4yav35n762ax4pg9fz7j0/);
});

test('a request that never reached the node is not reported as the chain refusing', () => {
  // The single most common failure on any of these screens, and the one that
  // most misleads: the chain has done nothing, and a treasury that cannot be
  // read is not a treasury that is empty.
  const t = translateError('TypeError: Failed to fetch');
  assert.equal(t.message, 'The node could not be reached');
  assert.equal(t.retryable, true);
  assert.match(t.reason ?? '', /nothing on the chain has changed/i);
});

test('a node answering with a server error is separated from a node not answering', () => {
  const t = translateError('Request failed with 503');
  assert.equal(t.message, 'The node is not answering');
  assert.equal(t.retryable, true);
});

test('a 404 is an answer, not a failure', () => {
  // The distinction a screen needs in order to choose between "there is none"
  // and "unknown". Conflating the two is how an empty page gets rendered over a
  // question nobody managed to ask.
  const t = translateError('Request failed with 404');
  assert.equal(t.message, 'There is no such record');
  assert.equal(t.retryable, false);
});

test('a gateway refusing an endpoint does not read as missing data', () => {
  const t = translateError('Request failed with 401');
  assert.equal(t.message, 'This node refuses that query');
  assert.match(t.reason ?? '', /configuration decision/i);
});

test('an unrecognised error is surfaced verbatim, not hidden', () => {
  const raw = 'some entirely novel failure from a future module';
  const t = translateError(raw);
  assert.equal(t.raw, raw);
  assert.equal(t.reason, raw, 'replacing an unknown error with a vague apology destroys the only information there was');
});

// ---------------------------------------------------- prices in the feed

test('a valuation reads as a sentence, with its date and method', () => {
  const decoded = decodeMessage({
    '@type': '/blockchain.oracle.v1.MsgSubmitAppraisal',
    appraiser: 'yml1jk3vecg9un9yldj5x725n3s7d68tzaj7ctl5zg',
    class_id: 'realestate',
    nft_id: 'building-1',
    value: '2500000000',
    value_denom: 'uusd',
    valued_at: '1786300000',
    method: 'RICS Red Book',
    report_uri: 'ipfs://QmExample',
  });

  assert.equal(decoded.kind, 'admin');
  assert.match(decoded.summary, /valued realestate\/building-1 at 2,500 USD/);
  assert.equal(decoded.everyday, true, 'what an asset is worth is exactly what a person wants to see');
  assert.deepEqual(
    decoded.details?.map((d) => d.label),
    ['Valuation date', 'Method', 'Report'],
  );
});

// Every validator reports every minute. Leaving those in the plain-language feed
// would bury the payment somebody is actually looking for.
test('routine price reports stay out of the everyday feed', () => {
  const decoded = decodeMessage({
    '@type': '/blockchain.oracle.v1.MsgSubmitExchangeRates',
    feeder: 'yml1feeder',
    validator: 'ymlvaloper16v5yy9dgnw9h3g85pgvcw3d2ylg86gndmhl9ml',
    rates: [
      { denom: 'uusd', rate: '1.00' },
      { denom: 'uyml', rate: '0.4213' },
    ],
  });

  assert.equal(decoded.everyday, false);
  assert.match(decoded.summary, /reported 2 prices/);
});

test('withdrawing a valuer’s authority says what happens to their past work', () => {
  const decoded = decodeMessage({
    '@type': '/blockchain.oracle.v1.MsgRevokeAppraiser',
    appraiser: 'yml1valuer',
    reason: 'licence lapsed',
  });

  assert.equal(decoded.kind, 'governance');
  assert.match(decoded.summary, /licence lapsed/);
  assert.match(decoded.summary, /existing valuations remain on record/);
});

// ------------------------------------------------- proposals, in tense

// "If it passes: Governance approved X" reads as though the vote were over.
// A proposal has to describe something that has not happened yet.
test('a proposal describes what would happen, not what did', () => {
  const action = describeProposalAction({
    '@type': '/blockchain.oracle.v1.MsgApproveAppraiser',
    appraiser: 'yml1valuer',
    approve: true,
    class_ids: ['realestate'],
  });

  assert.equal(action, 'Admit yml1valuer as an independent valuer, limited to realestate');
});

test('a refusal reads as a refusal, not as an approval of nothing', () => {
  const action = describeProposalAction({
    '@type': '/blockchain.validatorgov.v1.MsgApproveValidator',
    candidate: 'yml1candidate',
    approve: false,
  });

  assert.match(action, /^Refuse /);
});

test('a parameter change names the module it would change', () => {
  const action = describeProposalAction({ '@type': '/blockchain.amm.v1.MsgUpdateParams' });
  assert.equal(action, "Change the amm module's settings");
});

// An awkward tense beats a proposal whose effect is not described at all.
test('an unrecognised proposal message still says something', () => {
  const action = describeProposalAction({
    '@type': '/cosmos.bank.v1beta1.MsgSend',
    from_address: 'yml1a',
    to_address: 'yml1b',
    amount: [{ denom: 'uyml', amount: '1000000' }],
  });

  assert.match(action, /sent 1 YML/);
});

// The African currency set is the reason minor units exist as a concept
// separate from the chain's exponent. Every currency here is stored in
// millionths; almost none of them are quoted that way.
test('a currency prints to the decimals its users quote it in', () => {
  // Naira: kobo, two places. The ledger still holds all six.
  assert.equal(formatAmount('1359844414', 'ungn'), '1,359.84 NGN');

  // CFA franc: no subunit in practice, so a decimal point is noise.
  assert.equal(formatAmount('568387451', 'uxof'), '568 XOF');
  assert.equal(formatAmount('1000000', 'ubif'), '1 BIF');

  // Dinars are quoted to three places, which is why this is per-currency
  // rather than a blanket two-decimal rule.
  assert.equal(formatAmount('1234567', 'utnd'), '1.234 TND');

  // The native token has no minor unit, so it keeps the precision the ledger
  // holds — nothing is silently rounded off a staking balance.
  assert.equal(formatAmount('1234567', 'uyml'), '1.234567 YML');
});

test('an explicit decimal limit still overrides the currency default', () => {
  assert.equal(formatAmount('1359844414', 'ungn', { maxDecimals: 0 }), '1,359 NGN');
  assert.equal(formatAmount('1359844414', 'ungn', { maxDecimals: 6 }), '1,359.844414 NGN');
});

// Fee grants are the answer to "you must hold YML to move naira", so the two
// ways they fail have to read as what they are rather than as bankruptcy.
test('a missing fee allowance says so, rather than reporting no funds', () => {
  const t = translateError(
    'allow to pay fees for u��: fee-grant not found: not found',
  );
  assert.equal(t.message, 'No fee allowance');
  assert.match(t.reason!, /has not granted an allowance/);
});

test('holding no YML is distinguished from holding nothing', () => {
  const t = translateError('spendable balance 0uyml is smaller than 200uyml: insufficient funds');
  assert.equal(t.message, 'No YML for the network fee');
  // The point of the message: the account may be full of naira.
  assert.match(t.reason!, /other currencies cannot pay it/);
  // And the amount is the amount, not the amount plus the punctuation that
  // followed it in the chain's log.
  assert.match(t.reason!, /the network fee is 200uyml\./);
});

// A quiet chain produces rates that round to zero. Printing "0.0 per second"
// beside "4 transactions" is a sentence that disagrees with itself.
test('a trickle of traffic is described as an interval, not a rate of zero', () => {
  const quiet = measure([
    { height: 1, time: new Date('2026-08-13T00:00:00Z'), transactions: 2 },
    { height: 2, time: new Date('2026-08-13T00:00:50Z'), transactions: 0 },
    { height: 3, time: new Date('2026-08-13T00:01:40Z'), transactions: 2 },
  ])!;
  assert.match(describeThroughput(quiet), /roughly one every \d+ seconds/);

  const busy = measure([
    { height: 1, time: new Date('2026-08-13T00:00:00Z'), transactions: 100 },
    { height: 2, time: new Date('2026-08-13T00:00:05Z'), transactions: 100 },
  ])!;
  assert.match(describeThroughput(busy), /per second sustained/);
});

// The median, not the mean: one restart in a sample must not move the number
// somewhere no block actually was.
test('block time is the median, so one long gap does not move it', () => {
  const samples = Array.from({ length: 20 }, (_, i) => ({
    height: i + 1,
    time: new Date(Date.UTC(2026, 7, 13, 0, 0, i * 5)),
    transactions: 0,
  }));
  samples.push({ height: 21, time: new Date(Date.UTC(2026, 7, 13, 0, 2, 30)), transactions: 0 });

  const p = measure(samples)!;
  assert.equal(p.blockSeconds, 5);
  assert.equal(p.slowestSeconds, 55);
  assert.equal(p.idle, true);
});
