/**
 * The arithmetic, pinned against x/syndicate's own.
 *
 * Every figure a person acts on here is also computed by the keeper, in Go,
 * with integer division on `math.Int`. These tests exist because a console and
 * a chain that disagree about a fee by one unit is a platform nobody can tell
 * has disagreed about something larger — so each case below states the Go
 * expression it is reproducing.
 *
 *   node --test syndicate.test.js
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import {
  INDEX_SCALE, MAX_BPS, amount, book, day, denomInfo, document, entitlement,
  entryFees, group, holderShare, managementFee, money, multiple, papers, pct,
  projectedShare, returnToDate, shareBps, shortAddr, standing, transferable,
  until,
} from './syndicate.js';

/**
 * The digit group separator, named rather than typed inline.
 *
 * U+202F NARROW NO-BREAK SPACE - not an ordinary space and not a comma. A
 * comma is a decimal separator in French, which this console is also read in,
 * so a grouped figure using one is readable as a hundredth of itself. The
 * no-break half matters too: an ordinary space lets a browser wrap a figure
 * across a line, and half of a number is not a smaller number, it is a
 * different one.
 *
 * Written as an escape rather than as the character, because the two are
 * indistinguishable in a diff and in a review, and a test failing on an
 * invisible difference reads as a test failing for no reason.
 */
const NB = '\u202f';

/** A deal, with the fields the functions under test read. */
const aDeal = (over = {}) => ({
  id: '1',
  reference: 'SYN-2026-001',
  company: 'Kaolack Cold Chain',
  partner: 'Sahel Ventures',
  settlement_denom: 'uusdc',
  target: '500000000000',   // 500,000 USDC
  minimum: '300000000000',  // 300,000 USDC
  minimum_ticket: '5000000000',
  raised: '0',
  denom: '',
  supply: '0',
  distribution_index: '0',
  distributed: '0',
  state: 'DEAL_STATE_OPEN',
  closes_at: '1790000000',
  opened_at: '1788000000',
  closed_at: '0',
  fees: { sourcing_bps: 150, syndication_bps: 100, management_bps: 200 },
  ...over,
});

/* ========================================================================== */
/*  The book                                                                  */
/* ========================================================================== */

test('the book reports both thresholds, not one', () => {
  const b = book(aDeal({ raised: '200000000000' }));

  assert.equal(b.filledPct, 40);
  // The minimum sits at 60% of the target, and that position is what the bar's
  // threshold marker is drawn from.
  assert.equal(b.minimumPct, 60);
  assert.equal(b.viable, false);
  assert.equal(b.shortfall, 100000000000n);
  assert.equal(b.room, 300000000000n);
});

test('a book past its minimum is viable while still well short of target', () => {
  // The case a single fill bar buries: 62% sounds disappointing and is a deal
  // that is going to close.
  const b = book(aDeal({ raised: '310000000000' }));
  assert.equal(b.viable, true);
  assert.equal(b.full, false);
  assert.equal(b.shortfall, 0n);
});

test('a full book has no room and the bar does not exceed 100%', () => {
  // Over-subscription cannot happen — Commit refuses anything above the target
  // — but the bar must not be able to render past its track if it ever did.
  const b = book(aDeal({ raised: '600000000000' }));
  assert.equal(b.full, true);
  assert.equal(b.room, 0n);
  assert.equal(b.filledPct, 100);
});

test('an empty book divides by nothing', () => {
  const b = book(aDeal());
  assert.equal(b.filledPct, 0);
  assert.equal(b.viable, false);
});

/* ========================================================================== */
/*  Fees                                                                      */
/* ========================================================================== */

test('the entry fee reproduces CloseDeal exactly', () => {
  // Go: fees := deal.Raised.Mul(NewInt(entryBps)).Quo(NewInt(MaxBps))
  //     invested := deal.Raised.Sub(fees)
  const d = aDeal({ raised: '400000000000' });
  const f = entryFees(d);

  assert.equal(f.bps, 250);
  assert.equal(f.fee, (400000000000n * 250n) / MAX_BPS);
  assert.equal(f.fee, 10000000000n);
  assert.equal(f.invested, 390000000000n);
  // Nothing evaporates between the ladder's rungs.
  assert.equal(f.fee + f.invested, f.gross);
});

test('the entry fee truncates, and the remainder goes to the company', () => {
  // 7 units at 250bps is 0.175 units. Go's Quo truncates toward zero, so the
  // fee is 0 and the company gets all seven — the direction that favours the
  // company, which is the one to be sure of.
  const f = entryFees(aDeal({ raised: '7' }));
  assert.equal(f.fee, 0n);
  assert.equal(f.invested, 7n);
});

test('the entry fee is computed on the raise, never on the target', () => {
  const d = aDeal({ raised: '300000000000' });
  assert.equal(entryFees(d).fee, (300000000000n * 250n) / MAX_BPS);
  // Explicitly NOT the target's fee, which would overstate it by two thirds.
  assert.notEqual(entryFees(d).fee, (500000000000n * 250n) / MAX_BPS);
});

test('a deal with no fees moves the whole raise to the company', () => {
  const f = entryFees(aDeal({
    raised: '123456789',
    fees: { sourcing_bps: 0, syndication_bps: 0, management_bps: 0 },
  }));
  assert.equal(f.fee, 0n);
  assert.equal(f.invested, 123456789n);
});

test('the management fee comes off a distribution, and matches Distribute', () => {
  // Go: fee := msg.Amount.Amount.Mul(NewInt(ManagementBps)).Quo(NewInt(MaxBps))
  //     net := msg.Amount.Amount.Sub(fee)
  const m = managementFee(aDeal(), '50000000000');
  assert.equal(m.bps, 200);
  assert.equal(m.fee, (50000000000n * 200n) / MAX_BPS);
  assert.equal(m.net, m.gross - m.fee);
});

/* ========================================================================== */
/*  Allocation and entitlement                                                */
/* ========================================================================== */

test('a cheque buys tokens against the gross, not the net', () => {
  // The mistake that still balances: allocating against the net charges the fee
  // twice, once in cash and again in ownership.
  const d = aDeal({ raised: '300000000000' });
  const p = projectedShare(d, '100000000000');

  assert.equal(p.tokens, 100000000000n, 'one token per smallest unit committed');
  // 100k of a 400k book once this cheque lands.
  assert.equal(p.shareBps, 2500);
});

test('entitlement reproduces the keeper index arithmetic', () => {
  // Go: accrued.Add(balance.Mul(dealIndex.Sub(posIndex)).Quo(NewInt(IndexScale)))
  const balance = 100000000000n;         // 100,000 tokens
  const dealIndex = 3n * INDEX_SCALE / 100n;   // three hundredths per token
  const e = entitlement({
    dealIndex: dealIndex.toString(),
    positionIndex: '0',
    accrued: '0',
    balance: balance.toString(),
  });

  assert.equal(e.unsettled, (balance * dealIndex) / INDEX_SCALE);
  assert.equal(e.unsettled, 3000000000n);
  assert.equal(e.total, e.unsettled);
});

test('entitlement adds what was already settled to what has since accrued', () => {
  const e = entitlement({
    dealIndex: (2n * INDEX_SCALE).toString(),
    positionIndex: INDEX_SCALE.toString(),
    accrued: '4200',
    balance: '1000',
  });
  // One whole index unit of movement against a balance of 1000.
  assert.equal(e.delta, INDEX_SCALE);
  assert.equal(e.unsettled, 1000n);
  assert.equal(e.total, 5200n);
});

test('a holder settled up to date accrues nothing further', () => {
  const e = entitlement({
    dealIndex: INDEX_SCALE.toString(),
    positionIndex: INDEX_SCALE.toString(),
    accrued: '77',
    balance: '1000',
  });
  assert.equal(e.unsettled, 0n);
  assert.equal(e.total, 77n);
});

test('a holder with nothing left still keeps what accrued before they sold', () => {
  // The case that would look like a bug: balance zero, claimable positive.
  // Settle credits the seller at the moment of transfer, so the accrued figure
  // survives the sale, and a page reporting zero would tell somebody the
  // platform had kept their distribution.
  const e = entitlement({
    dealIndex: (5n * INDEX_SCALE).toString(),
    positionIndex: '0',
    accrued: '9000',
    balance: '0',
  });
  assert.equal(e.unsettled, 0n);
  assert.equal(e.total, 9000n);
});

test('the index does not lose precision as it would through a double', () => {
  // 1e18 is eight orders of magnitude beyond Number.MAX_SAFE_INTEGER. This is
  // the assertion that fails the moment anything in the path becomes a Number.
  const odd = (INDEX_SCALE + 1n).toString();
  const e = entitlement({
    dealIndex: odd, positionIndex: '0', accrued: '0', balance: INDEX_SCALE.toString(),
  });
  assert.equal(e.delta, INDEX_SCALE + 1n);
  assert.equal(e.unsettled, (INDEX_SCALE * (INDEX_SCALE + 1n)) / INDEX_SCALE);
});

test('a share is a balance over the supply and nothing else', () => {
  assert.equal(shareBps('250000000000', '1000000000000'), 2500);
  assert.equal(shareBps('0', '1000000000000'), 0);
  // No supply is not an error; it is a deal that never closed.
  assert.equal(shareBps('5', '0'), 0);
});

/* ========================================================================== */
/*  Standing                                                                  */
/* ========================================================================== */

const NOW = 1789000000;
const AFTER = 1791000000;   // past closes_at

test('an open deal below its minimum says so, and says what happens next', () => {
  const s = standing(aDeal({ raised: '100000000000' }), NOW);
  assert.equal(s.state, 'open');
  // The tone follows viability rather than merely "is it open". A card drawn
  // calm while its own sentence says the minimum is not covered is the page
  // disagreeing with itself, and the colour is what a reader scanning a list
  // takes away.
  assert.equal(s.tone, 'warn');
  assert.match(s.what, /refunded in full/);
  assert.deepEqual(s.can, ['commit', 'withdraw']);

  // The shortfall is FORMATTED, in the deal's own currency. Rendered against a
  // seeded chain this sentence read "Below its minimum by 1600000" beside
  // "1.40 YML raised" — a figure a million times its own size, in a sentence
  // whose whole job is to say how far short the raise is. Units are the one
  // kind of wrongness every arithmetic test above would pass through.
  assert.match(s.what, new RegExp(`Below its minimum by 200${NB}000\\.00 USDC\\.`));
  assert.doesNotMatch(s.what, /200000000000/);
});

test('an open deal past its minimum is calm about it', () => {
  const s = standing(aDeal({ raised: '350000000000' }), NOW);
  assert.equal(s.tone, 'ok');
  assert.match(s.what, /minimum is covered/);
});

test('a window that ran out short of the minimum tells anybody they may abort', () => {
  // The single most useful thing an investor in a failing raise can be told,
  // and the reason there is no automatic sweeper: nobody needs the operator's
  // cooperation to get their cheque back.
  const s = standing(aDeal({ raised: '100000000000' }), AFTER);
  assert.equal(s.state, 'failed');
  assert.equal(s.tone, 'bad');
  assert.ok(s.can.includes('abort'));
});

test('a window that ran out with the minimum covered is waiting on the operator', () => {
  const s = standing(aDeal({ raised: '350000000000' }), AFTER);
  assert.equal(s.state, 'closing');
  assert.ok(!s.can.includes('abort'), 'a viable deal cannot be aborted by a passer-by');
  assert.ok(s.can.includes('withdraw'), 'an investor can still pull out until it closes');
});

test('an aborted deal says the fees came back too', () => {
  const s = standing(aDeal({ state: 'DEAL_STATE_ABORTED' }), NOW);
  assert.match(s.what, /fees included/);
  assert.deepEqual(s.can, []);
});

test('a realised deal says trading has stopped and why', () => {
  const s = standing(aDeal({ state: 'DEAL_STATE_REALISED' }), NOW);
  assert.match(s.what, /trap for whoever bought last/);
  assert.deepEqual(s.can, ['claim']);
});

test('an unrecognised state is reported as this page being behind the chain', () => {
  const s = standing(aDeal({ state: 'DEAL_STATE_SOMETHING_NEW' }), NOW);
  assert.equal(s.state, 'unknown');
  assert.match(s.what, /newer code than this page/);
});

/* ========================================================================== */
/*  Transferability                                                           */
/* ========================================================================== */

test('an open deal has no token to transfer, and says that rather than "refused"', () => {
  const t = transferable(aDeal());
  assert.equal(t.ok, false);
  assert.match(t.why, /has not closed/);
});

test('a closed deal is transferable subject to onboarding on both sides', () => {
  const t = transferable(aDeal({ state: 'DEAL_STATE_CLOSED', denom: 'syn/1' }));
  assert.equal(t.ok, true);
  assert.match(t.why, /not in the platform's software/);
});

test('a realised deal is halted, and that is a different refusal from the KYC one', () => {
  const t = transferable(aDeal({ state: 'DEAL_STATE_REALISED', denom: 'syn/1' }));
  assert.equal(t.ok, false);
  assert.match(t.why, /halted by the chain/);
});

/* ========================================================================== */
/*  Money                                                                     */
/* ========================================================================== */

test('an amount is scaled by the denomination exponent', () => {
  assert.equal(amount('1500000', 'uusdc'), '1.50');
  assert.equal(amount('500000000000', 'uusdc'), `500${NB}000.00`);
});

test('a franc is printed without a subunit nobody uses', () => {
  assert.equal(amount('1500000', 'uxof'), '1');
  assert.equal(amount('250000000000', 'uxof'), `250${NB}000`);
});

test('an amount truncates rather than rounding up', () => {
  // A balance shown as more than it is invites somebody to try to send it.
  assert.equal(amount('1999999', 'uusdc'), '1.99');
});

test('an unknown denomination is labelled as base units rather than guessed at', () => {
  // A stablecoin registered after this page was written has an exponent this
  // file cannot invent. Dividing by a million on the assumption that every
  // denom begins with `u` displays a position at a millionth of its value with
  // nothing on screen saying so.
  const info = denomInfo('ibc/ABCD');
  assert.equal(info.known, false);
  assert.match(money('1500000', 'ibc/ABCD'), /base units/);
});

test('a figure beyond double precision survives being formatted', () => {
  // 9,007,199,254.740993 USDC — one unit past Number.MAX_SAFE_INTEGER in base
  // units. This is a product that intends to hold sums like this.
  assert.equal(amount('9007199254740993', 'uusdc'), `9${NB}007${NB}199${NB}254.74`);
});

test('digits are grouped with a space, not a comma', () => {
  // This console is read in French and in English, and a comma is a decimal
  // separator in one of them. A grouped figure readable as a hundredth of
  // itself is worse than an ungrouped one.
  assert.equal(group('1234567'), `1${NB}234${NB}567`);
  assert.equal(group('999'), '999');
});

test('a negative amount keeps its sign', () => {
  assert.equal(amount('-1500000', 'uusdc'), '−1.50');
});

test('basis points print at the precision basis points carry', () => {
  assert.equal(pct(250), '2.50%');
  assert.equal(pct(200), '2%');
  assert.equal(pct(10000), '100%');
});

/* ========================================================================== */
/*  Time                                                                      */
/* ========================================================================== */

test('a deadline in the future reads as time remaining', () => {
  const u = until(NOW + 3 * 86400, NOW);
  assert.equal(u.past, false);
  assert.equal(u.phrase, 'in 3 days');
});

test('a deadline in the past reads as elapsed, not as a negative number', () => {
  // A raise whose window ran out yesterday and which nobody has aborted is a
  // real and common state. "closed 1 day ago" is the sentence that makes
  // somebody go and abort it.
  const u = until(NOW - 86400 - 7200, NOW);
  assert.equal(u.past, true);
  assert.equal(u.phrase, '1 day 2h ago');
});

test('under an hour is reported in minutes rather than as zero hours', () => {
  assert.equal(until(NOW + 1800, NOW).text, '30m');
});

test('a date is a date; the hour is never the point', () => {
  assert.equal(day('1788000000'), '2026-08-29');
  assert.equal(day('0'), '—');
  assert.equal(day(0), '—');
});

test('an address is shortened but never silently', () => {
  const a = 'yml1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzzzz';
  assert.match(shortAddr(a), /…/);
  assert.ok(shortAddr(a).endsWith('zzzz'), 'the tail is kept — it is what distinguishes two addresses');
  assert.equal(shortAddr('yml1short'), 'yml1short', 'a short address is not decorated');
});

/* ========================================================================== */
/*  The return                                                                */
/* ========================================================================== */

test('a deal that has paid nothing back has returned nothing, and says so', () => {
  const r = returnToDate(aDeal({ raised: '400000000000', distributed: '0' }));
  assert.equal(r.any, false);
  assert.equal(r.bps, 0);
});

test('a deal with no raise does not divide by zero', () => {
  const r = returnToDate(aDeal({ raised: '0', distributed: '0' }));
  assert.equal(r.bps, 0);
  assert.equal(r.any, false);
});

test('the return is distributions over subscriptions, both of them the deal’s own', () => {
  // 331,000 paid back against 400,000 subscribed.
  const r = returnToDate(aDeal({ raised: '400000000000', distributed: '331000000000' }));
  assert.equal(r.bps, 8275);
  assert.equal(pct(r.bps), '82.75%');
  assert.equal(multiple(r.bps), '0.83×');
  assert.equal(r.any, true);
});

test('a return above capital is reported as such and not capped at 100%', () => {
  // The cap belongs on a progress bar, where the bar cannot be longer than
  // itself. A return that clipped at 100% would turn the only outcome the
  // whole business model depends on into the same figure as breaking even.
  const r = returnToDate(aDeal({ raised: '100000000', distributed: '340000000' }));
  assert.equal(pct(r.bps), '340%');
  assert.equal(multiple(r.bps), '3.40×');
});

test('the ratio survives amounts beyond double precision', () => {
  const r = returnToDate(aDeal({
    raised: '123456789012345678901234567890',
    distributed: '61728394506172839450617283945',
  }));
  assert.equal(r.bps, 5000);
  assert.equal(multiple(r.bps), '0.50×');
});

test('a multiple keeps two decimals, because 1× and 1.004× are not the same claim', () => {
  assert.equal(multiple(10000), '1.00×');
  assert.equal(multiple(10040), '1.00×');  // truncated by toFixed, not dressed up
  assert.equal(multiple(0), '0.00×');
});

/* ========================================================================== */
/*  What one holding has been paid                                            */
/* ========================================================================== */

test('a holding is credited with its own proportion of what the deal distributed', () => {
  const d = aDeal({
    raised: '400000000000', supply: '400000000000', distributed: '331000000000',
  });
  const h = holderShare(d, '100000000000');   // a quarter of the syndicate
  assert.equal(h.shareBps, 2500);
  assert.equal(h.attributed, 82750000000n);   // a quarter of 331,000
  assert.equal(h.whole, false);
});

test('the division is taken last, so the money is not rounded through a percentage', () => {
  // Three sevenths of a million. The honest arithmetic is 3 * 1000000 / 7 =
  // 428571. Going through the rounded share_bps the query returns —
  // 3 * 10000 / 7 = 4285 bps, then 4285 * 1000000 / 10000 — gives 428500,
  // which is 71 units of somebody's money lost to the order of two operations.
  const d = aDeal({ supply: '7', distributed: '1000000', raised: '7' });
  const h = holderShare(d, '3');
  assert.equal(h.attributed, 428571n);
  assert.equal(h.shareBps, 4285);
  // The figure the shortcut would have produced, named so the test fails if
  // somebody ever reintroduces it.
  assert.notEqual(h.attributed, (BigInt(h.shareBps) * 1000000n) / 10000n);
});

test('a sole holder is credited with everything, and is flagged as sole', () => {
  const d = aDeal({ supply: '900', distributed: '450', raised: '900' });
  const h = holderShare(d, '900');
  assert.equal(h.attributed, 450n);
  assert.equal(h.shareBps, 10000);
  assert.equal(h.whole, true);
});

test('a deal with no supply yet attributes nothing rather than dividing by zero', () => {
  const h = holderShare(aDeal(), '0');
  assert.equal(h.attributed, 0n);
  assert.equal(h.shareBps, 0);
});

/* ========================================================================== */
/*  The papers                                                                */
/* ========================================================================== */

/** A document as the chain hands one over: ids are strings, digests are hex. */
const aDoc = (over = {}) => ({
  id: '1',
  deal_id: '1',
  kind: 'DOCUMENT_KIND_TERM_SHEET',
  title: 'Term sheet',
  sha256: 'a'.repeat(64),
  uri: 'ipfs://bafyterm',
  language: 'en',
  attested_by: 'yml1operator',
  attached_at: '1788000000',
  supersedes: '0',
  superseded_by: '0',
  ...over,
});

test('a binding document is marked as one, and a describing document is not', () => {
  // The whole reason the chain carries a kind. An investor scanning a list
  // needs to see which paper puts an obligation on THEM, and a filename cannot
  // tell them.
  const sub = document(aDoc({ kind: 'DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT' }));
  assert.equal(sub.binding, true);

  assert.equal(document(aDoc({ kind: 'DOCUMENT_KIND_DILIGENCE' })).binding, false);
  assert.equal(document(aDoc({ kind: 'DOCUMENT_KIND_TERM_SHEET' })).binding, false,
    'a term sheet is the commercial terms before anybody signed; it binds nobody');
});

test('an unknown kind is labelled rather than dropped', () => {
  // A console a version behind the chain must show the document it cannot
  // classify, not hide it. A paper missing from the list is indistinguishable
  // from a paper that was never attached.
  const d = document(aDoc({ kind: 'DOCUMENT_KIND_SOMETHING_NEWER' }));
  assert.equal(d.label, 'Document');
  assert.equal(d.binding, false);
  assert.equal(d.title, 'Term sheet');
});

test('a superseded document is flagged stale, in both directions', () => {
  const old = document(aDoc({ id: '1', superseded_by: '2' }));
  assert.equal(old.stale, true);
  assert.equal(old.amends, false);

  const current = document(aDoc({ id: '2', supersedes: '1' }));
  assert.equal(current.stale, false);
  assert.equal(current.amends, true);
});

test('a zero id means no relation, not document zero', () => {
  // proto3: an unset uint64 is 0, and document 0 is never issued precisely so
  // the two cannot be confused. A page that read `supersedes: '0'` as a link
  // would offer every document an amendment history it does not have.
  const plain = document(aDoc());
  assert.equal(plain.stale, false);
  assert.equal(plain.amends, false);
});

test('the digest is shortened for the summary and kept whole underneath', () => {
  // A truncated digest cannot be compared against anything, so the short form
  // is a label and the whole one has to stay reachable beside it.
  const full = '0123456789' + 'f'.repeat(44) + 'abcdef0123';
  const d = document(aDoc({ sha256: full }));
  assert.equal(d.sha256, full);
  assert.equal(d.sha256.length, 64);
  assert.equal(d.short, '01234567…cdef0123');
});

test('papers put what binds you first, and each group oldest first', () => {
  // The chain returns attachment order, which is the order the DEAL happened
  // in — right for a history and wrong for a reader whose question is "what am
  // I signing".
  const sorted = papers([
    aDoc({ id: '1', kind: 'DOCUMENT_KIND_DILIGENCE' }),
    aDoc({ id: '2', kind: 'DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT' }),
    aDoc({ id: '3', kind: 'DOCUMENT_KIND_REPORT' }),
    aDoc({ id: '4', kind: 'DOCUMENT_KIND_SHAREHOLDERS_AGREEMENT' }),
  ]);
  assert.deepEqual(sorted.map((d) => d.id), ['2', '4', '1', '3']);
});

test('sorting the papers does not reorder the caller’s array', () => {
  const given = [
    aDoc({ id: '1', kind: 'DOCUMENT_KIND_REPORT' }),
    aDoc({ id: '2', kind: 'DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT' }),
  ];
  papers(given);
  assert.deepEqual(given.map((d) => d.id), ['1', '2']);
});

test('a deal with no papers is an empty list, not a failure', () => {
  assert.deepEqual(papers([]), []);
});
