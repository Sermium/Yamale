/**
 * The reasoning behind the oversight console, separated from the drawing of it.
 *
 * Everything in this file is a pure function of chain state. That is not a
 * style preference: this console makes a claim — that one validator can stop
 * money in a block and that taking it needs two thirds — and a claim like that
 * has to be checkable. Every function here is tested in oversight.test.js
 * against the arithmetic the keeper actually performs, so the page's headline
 * is a computation somebody can re-run rather than a sentence somebody wrote.
 *
 * Where this file reimplements a keeper rule, it says which file it is
 * mirroring, and the test asserts the mirror. A console that computes its own
 * answer and quietly disagrees with the chain is worse than one that says
 * nothing: the moment it disagrees is the moment somebody acts on the wrong
 * number.
 */

import { CASE_STATUS, CASE_ACTION, VOTE_OPTION, CYCLE_STATUS, DENOM_STATUS, enumName } from './proto.js';

// ---------------------------------------------------------------------------
// The threshold.
// ---------------------------------------------------------------------------

/**
 * The power a case needs to pass.
 *
 * Mirrors `Params.RequiredPower` in x/enforcement/types/params.go. Two details
 * are load-bearing and both are easy to get wrong by writing the "obvious"
 * expression:
 *
 *   - it rounds UP. `total * bps / 10000` truncated would let a case pass one
 *     unit of power short of two thirds, and "two thirds, near enough" is not
 *     a sentence this module gets to say.
 *   - `total` is the power bonded when the case OPENED, not the power that
 *     voted and not the power bonded now. A threshold measured against turnout
 *     is passed by two validators on a quiet night; a threshold measured
 *     against the live set moves under the vote as validators unbond.
 */
export function requiredPower(totalPowerAtOpen, thresholdBps) {
  const total = Number(totalPowerAtOpen);
  const bps = Number(thresholdBps);
  if (!Number.isFinite(total) || total <= 0) return 0;
  const numerator = total * bps;
  const required = Math.floor(numerator / 10000);
  return required * 10000 !== numerator ? required + 1 : required;
}

/**
 * Where a case stands against the bar, and — the part a reader actually needs
 * — whether the bar is still reachable.
 *
 * The keeper resolves a case eagerly in both directions (msg_server_case.go):
 * it passes the moment yes reaches the bar, and it REJECTS the moment
 * `total - no` falls below it, because at that point no combination of the
 * remaining validators can get there. A console that only draws a progress bar
 * toward "yes" shows a case at 40% that is already decided against, which
 * reads as "still in play" to the person it is about.
 */
export function caseStanding(c, params) {
  const total = Number(c.total_power_at_open) || 0;
  const yes = Number(c.yes_power) || 0;
  const no = Number(c.no_power) || 0;
  const abstain = Number(c.abstain_power) || 0;
  const bps = Number(params && params.threshold_bps) || 0;
  const required = requiredPower(total, bps);

  // The most yes-power the case could still gather: everything not already
  // committed against it. Abstentions are spent power — a validator that
  // abstained cannot come back and vote yes.
  const stillPossible = Math.max(0, total - no - abstain);

  return {
    total,
    yes,
    no,
    abstain,
    required,
    // Shares of the power bonded at open, in basis points, so the page can
    // draw the bar and the mark on the same scale the chain uses.
    yesBps: total > 0 ? Math.round((yes * 10000) / total) : 0,
    noBps: total > 0 ? Math.round((no * 10000) / total) : 0,
    abstainBps: total > 0 ? Math.round((abstain * 10000) / total) : 0,
    thresholdBps: bps,
    met: total > 0 && yes >= required,
    // False means the case is arithmetically lost even though it is still
    // formally open.
    reachable: total > 0 && stillPossible >= required,
    shortBy: Math.max(0, required - yes),
    uncast: Math.max(0, total - yes - no - abstain),
  };
}

/**
 * The asymmetry, computed against the live validator set rather than asserted.
 *
 * This is the single most important thing the console says, so it is the thing
 * least allowed to be a sentence somebody typed. Given the actual voting power
 * on the chain right now:
 *
 *   - ANY ONE bonded validator can open a case, and opening a case freezes the
 *     target in that block. So the number of validators needed to stop money
 *     is 1, always, regardless of power.
 *   - Taking it needs `threshold_bps` of the bonded total. `minimumToSeize` is
 *     the smallest number of validators that could reach that, found by taking
 *     the largest first — which is the most favourable case for a would-be
 *     coalition and therefore the honest lower bound.
 *
 * `largestSingleShareBps` answers the question a central bank asks next: can
 * the biggest validator do it alone?
 */
export function seizureCoalition(validatorSet, thresholdBps) {
  const powers = (validatorSet || []).map((v) => Number(v.power) || 0)
    .filter((p) => p > 0)
    .sort((a, b) => b - a);
  const total = powers.reduce((a, b) => a + b, 0);
  const required = requiredPower(total, thresholdBps);

  let running = 0;
  let count = 0;
  for (const p of powers) {
    if (running >= required) break;
    running += p;
    count += 1;
  }
  // If even the whole set cannot reach the bar, no coalition exists. That is
  // not a hypothetical: a threshold above 10000 bps would do it, and so would
  // an empty set.
  const achievable = running >= required && required > 0;

  return {
    validators: powers.length,
    totalPower: total,
    required,
    thresholdBps: Number(thresholdBps) || 0,
    toFreeze: powers.length ? 1 : 0,
    minimumToSeize: achievable ? count : null,
    largestSingleShareBps: total > 0 && powers.length ? Math.round((powers[0] * 10000) / total) : 0,
    largestCanSeizeAlone: achievable && powers.length > 0 && powers[0] >= required,
  };
}

// ---------------------------------------------------------------------------
// Freezes.
// ---------------------------------------------------------------------------

/**
 * How long a freeze has left.
 *
 * The clock is BLOCK HEIGHT, never wall time — the module reads block time in
 * exactly one place and it is not this one. So the honest primary number is
 * blocks, and the hours figure is derived from a measured block interval and
 * labelled as an estimate. A land record showed this project that an estimated
 * date presented as a fact is a date somebody eventually cites.
 *
 * `expires_at_height == 0` does NOT mean "unset". It means the freeze is
 * PERMANENT: `makePermanent` in x/enforcement/keeper/keeper.go zeroes the field
 * and removes the expiry-queue entry when a case passes. Reading that zero as a
 * missing value would render the most serious state on the page — a freeze
 * that will never lift by itself — as a blank.
 */
export function freezeCountdown(freeze, height, secondsPerBlock) {
  const expires = Number(freeze && freeze.expires_at_height) || 0;
  const frozenAt = Number(freeze && freeze.frozen_at_height) || 0;
  const now = Number(height) || 0;

  if (expires === 0) {
    return {
      permanent: true,
      blocksLeft: null,
      lapsed: false,
      heldFor: Math.max(0, now - frozenAt),
      estimate: null,
    };
  }
  const blocksLeft = expires - now;
  return {
    permanent: false,
    blocksLeft,
    // The EndBlocker lifts every freeze whose expiry height is <= the current
    // height, so a non-positive number means the release is due this block or
    // is overdue — which, if it persists, is itself worth seeing.
    lapsed: blocksLeft <= 0,
    heldFor: Math.max(0, now - frozenAt),
    estimate: secondsPerBlock > 0 && blocksLeft > 0
      ? blocksLeft * secondsPerBlock : null,
  };
}

/** A Freeze whose case_id is 0 has no case attached — ids start at 1. See the
 *  note in chain.js. This distinction is the difference between "frozen under
 *  case 4, which you can read" and "frozen, and the record does not say by
 *  what", and the second one is the one worth an alarm. */
export function freezeHasCase(freeze) {
  return Number(freeze && freeze.case_id) > 0;
}

// ---------------------------------------------------------------------------
// The seizure window.
// ---------------------------------------------------------------------------

/**
 * The rolling cap, as headroom rather than as raw totals.
 *
 * Two caps bind independently and the console must not merge them, because
 * they fail differently: the count cap binds every denomination including one
 * issued after the parameters were last set, and the value cap binds only the
 * denominations it names. A denomination absent from the cap list is not
 * capped by value at all, and saying nothing about it would read as "zero
 * seized of zero allowed".
 */
export function windowHeadroom(w) {
  const caps = w.cap || [];
  const seized = w.seized || [];
  const seizedOf = (d) => (seized.find((c) => c.denom === d) || { amount: '0' }).amount;

  return {
    startHeight: Number(w.window_start_height) || 0,
    currentHeight: Number(w.current_height) || 0,
    lengthBlocks: Math.max(0, (Number(w.current_height) || 0) - (Number(w.window_start_height) || 0)),
    count: Number(w.seizure_count) || 0,
    maxCount: Number(w.max_seizures) || 0,
    countExhausted: (Number(w.max_seizures) || 0) > 0
      && (Number(w.seizure_count) || 0) >= (Number(w.max_seizures) || 0),
    denoms: caps.map((c) => ({
      denom: c.denom,
      cap: c.amount,
      seized: seizedOf(c.denom),
      remaining: ((w.remaining || []).find((r) => r.denom === c.denom) || { amount: c.amount }).amount,
    })),
    // Anything seized in a denomination the cap list does not name. Only the
    // count cap holds it back.
    uncapped: seized.filter((s) => !caps.some((c) => c.denom === s.denom))
      .map((s) => ({ denom: s.denom, seized: s.amount })),
  };
}

// ---------------------------------------------------------------------------
// x/netting: the state of the window.
// ---------------------------------------------------------------------------

/**
 * What the settlement window is doing, including the state the project has
 * written down as an open question and never had a way to see.
 *
 * docs/scope/gaps.md, open question 2: setting `cycle_blocks = 0` makes the
 * EndBlocker return before closing anything, "so the open window never
 * settles, held slices stop being retried, and every participant in it has an
 * exposure with no settlement date until a second proposal passes."
 *
 * The detection is not the obvious one. "An end height in the past" does not
 * work, in both directions:
 *
 *   - when netting is off, the query handler deliberately reports
 *     `closes_at_height = 0` rather than computing a height, so a stalled
 *     window shows no past end height at all;
 *   - when netting is on, `closes_at_height` is recomputed from the CURRENT
 *     cycle_blocks, so shortening it mid-window legitimately yields a height
 *     already passed. That is not a stall.
 *
 * So: `cycle_blocks == 0` is the disabled marker, and what separates STALLED
 * from merely OFF is whether the frozen window has anything trapped in it.
 */
export function nettingWindowState({ params, cycle, closesAtHeight, held, height }) {
  const cycleBlocks = Number(params && params.cycle_blocks) || 0;
  const status = Number(cycle && cycle.status) || 0;
  const outcomes = (cycle && cycle.outcomes) || [];
  const heldSlices = held || [];

  // A denomination with obligations admitted to this window. gross_amount is a
  // decimal string and is always emitted, so "0" is the empty case.
  const withTraffic = outcomes.filter(
    (o) => o.gross_amount && o.gross_amount !== '0' && o.gross_amount !== '',
  );

  const openedAt = Number(cycle && cycle.opened_at_height) || 0;
  const now = Number(height) || 0;

  if (cycleBlocks === 0) {
    const trapped = status === 1 /* OPEN */ && withTraffic.length > 0;
    const frozenHeld = heldSlices.length > 0;
    if (trapped || frozenHeld) {
      return {
        state: 'stalled',
        cycleBlocks: 0,
        openedAt,
        stalledForBlocks: openedAt > 0 ? Math.max(0, now - openedAt) : null,
        trappedDenoms: withTraffic.map((o) => o.denom),
        frozenHeldSlices: heldSlices.length,
        headline: 'The settlement window is open and cannot close.',
        detail: 'Netting is switched off (cycle_blocks = 0), and the end blocker '
          + 'returns before it closes anything. The obligations already admitted to '
          + 'this window will not settle, held slices are no longer being retried, '
          + 'and the collateral they committed cannot be released by any message. '
          + 'It takes a second governance proposal setting cycle_blocks back above '
          + 'zero to close it.',
      };
    }
    return {
      state: 'off',
      cycleBlocks: 0,
      openedAt,
      stalledForBlocks: null,
      trappedDenoms: [],
      frozenHeldSlices: 0,
      headline: 'Netting is switched off. Every payment settles gross.',
      detail: 'cycle_blocks is 0, so no window closes and every obligation is '
        + 'settled one by one out of the sender\'s own balance. Nothing is trapped: '
        + 'the open window holds no obligations. This is a configuration, not a fault.',
    };
  }

  if (heldSlices.length > 0 || status === 3 /* HELD */) {
    return {
      state: 'held',
      cycleBlocks,
      openedAt,
      closesAt: Number(closesAtHeight) || 0,
      blocksToClose: (Number(closesAtHeight) || 0) - now,
      trappedDenoms: outcomes.filter((o) => Number(o.status) === 3).map((o) => o.denom),
      frozenHeldSlices: heldSlices.length,
      headline: 'A settlement slice did not clear.',
      detail: 'The chain retries a held slice unchanged at every cycle boundary. '
        + 'It is never recomputed, reassigned or cancelled, and there is no path '
        + 'that gives up on it. New business is unaffected: the next window keeps '
        + 'taking traffic.',
    };
  }

  return {
    state: 'open',
    cycleBlocks,
    openedAt,
    closesAt: Number(closesAtHeight) || 0,
    blocksToClose: (Number(closesAtHeight) || 0) - now,
    trappedDenoms: [],
    frozenHeldSlices: 0,
    headline: 'The settlement window is open.',
    detail: 'It closes at the next block height divisible by cycle_blocks. '
      + 'The close happens in the end blocker, so there is no transaction and no '
      + 'receipt — the evidence is the cycle record and the settlement event.',
  };
}

/**
 * A participant's exposure, from their position entries.
 *
 * `available` is clamped at zero by the keeper, so it can never explain a
 * shortfall on its own; `net_position` is signed and negative means they owe
 * the system. The pair that matters is locked-against-reserve, because that is
 * what the net debit cap is checked on and what a withdrawal is refused
 * against.
 */
export function exposure(entries) {
  return (entries || []).map((e) => {
    const reserve = BigInt(e.reserve || '0');
    const locked = BigInt(e.locked || '0');
    const net = BigInt(e.net_position || '0');
    return {
      denom: e.denom,
      reserve: reserve.toString(),
      locked: locked.toString(),
      available: (reserve > locked ? reserve - locked : 0n).toString(),
      net: net.toString(),
      owes: net < 0n,
      // Every basis point of the posted collateral currently committed. A
      // participant at 10000 can submit nothing further in this denomination.
      utilisationBps: reserve > 0n ? Number((locked * 10000n) / reserve) : 0,
      fullyCommitted: reserve > 0n && locked >= reserve,
    };
  });
}

// ---------------------------------------------------------------------------
// What the chain refuses.
//
// The most important panel on the enforcement half, and the one most at risk
// of being marketing. Two rules keep it honest:
//
//   1. Every entry names where the refusal lives. A refusal enforced by a
//      constitutional pin, one enforced by parameter validation, and one
//      enforced by a line in a handler are three different strengths of
//      promise, and flattening them into "the chain refuses" overstates the
//      weakest.
//   2. `pinned` is true ONLY for the four parameters x/enforcement's
//      constitutional.go actually checks. The guide says the delay, the window
//      cap and the ombudsman are INTENDED to become invariants; they are not
//      today. A console claiming governance cannot change them would be
//      telling a reassuring lie about the exact powers a reader came to check.
// ---------------------------------------------------------------------------

export const REFUSALS = [
  {
    what: 'Seized funds can only ever go to one address.',
    how: 'No message in the module takes a destination. The only code that moves '
      + 'money reads params.recovery_destination, and the bank send restriction '
      + 'that blocks a frozen account makes exactly one exception — that same '
      + 'address. No case can pay whoever opened it.',
    where: 'x/enforcement/keeper/execute.go, restriction.go',
    pinned: true,
    pinNote: 'Pinned to x/constitution: changing it needs a constitutional amendment, '
      + 'not a parameter change.',
  },
  {
    what: 'A seizure needs more than half the bonded power, by any route.',
    how: 'threshold_bps must be greater than 5000 and at most 10000. Parameter '
      + 'validation refuses anything lower, including via a governance amendment.',
    where: 'x/enforcement/types/params.go',
    pinned: true,
    pinNote: 'Pinned to x/constitution alongside the voting period and the '
      + 'provisional freeze length.',
  },
  {
    what: 'A seizure cannot be opened without an external legal instrument.',
    how: 'There is no parameter that turns this off. A requirement governance can '
      + 'vote away is a default, not a requirement.',
    where: 'x/enforcement/keeper/msg_server_case.go',
    pinned: false,
    pinNote: 'Enforced in the handler, so it cannot be relaxed by a parameter — '
      + 'only by a binary upgrade.',
  },
  {
    what: 'The ombudsman can stop a case and can never start one.',
    how: 'The veto is the only message the ombudsman may sign. Opening, voting and '
      + 'sweeping all refuse that signer outright, so appointing an ombudsman adds '
      + 'a check without adding a power.',
    where: 'x/enforcement/keeper/msg_server_ombudsman.go',
    pinned: false,
    pinNote: 'The ombudsman address is an ordinary parameter today. The module guide '
      + 'intends it to become a constitutional invariant; it is not one yet.',
  },
  {
    what: 'A veto cannot un-take money.',
    how: 'The veto is accepted only while a case is still voting or held. On a case '
      + 'that has already executed it is refused. The delay before execution is the '
      + 'entire window in which the veto exists.',
    where: 'x/enforcement/keeper/msg_server_ombudsman.go',
    pinned: false,
    pinNote: '',
  },
  {
    what: 'Mass expropriation is arithmetically impossible, not merely unpopular.',
    how: 'A rolling window caps both the value seized per denomination and the number '
      + 'of seizures whatever they are worth. The count cap is what stops the value '
      + 'cap being walked around by choosing a currency nobody thought to price.',
    where: 'x/enforcement/keeper/window.go',
    pinned: false,
    pinNote: 'Parameters today. The guide intends them to become invariants; they are '
      + 'not yet, so a governance proposal could raise them.',
  },
  {
    what: 'Reversing a case gives back the account, never the money.',
    how: 'MsgReverseCase lifts the freeze and marks the case reversed. There is no '
      + 'message anywhere in the module that returns seized funds.',
    where: 'x/enforcement/keeper/execute.go',
    pinned: false,
    pinNote: 'Restitution is the recovery destination\'s job, off this module.',
  },
  {
    what: 'A frozen account can still be paid.',
    how: 'The restriction blocks sending only. Freezing stops somebody moving money; '
      + 'it does not stop their salary arriving.',
    where: 'x/enforcement/keeper/restriction.go',
    pinned: false,
    pinNote: '',
  },
  {
    what: 'Module accounts can never be frozen.',
    how: 'Every freeze path checks the target against the blocked-address set and the '
      + 'module-account types before it does anything.',
    where: 'x/enforcement/keeper/msg_server_case.go',
    pinned: false,
    pinNote: '',
  },
  {
    what: 'An unset authority means nobody, never anybody.',
    how: 'With no ombudsman appointed, the veto message is refused outright rather '
      + 'than accepted from any signer. The same fail-closed rule applies to the '
      + 'scope registry: if it is unwired, every freeze is refused.',
    where: 'x/enforcement/keeper/msg_server_ombudsman.go, keeper.go',
    pinned: false,
    pinNote: '',
  },
  {
    what: 'Nothing in netting can be recomputed, reassigned or cancelled.',
    how: 'The Msg service has four messages and none of them mutates an existing '
      + 'obligation, position or closed cycle. A slice that does not clear is retried '
      + 'unchanged at every boundary, forever.',
    where: 'x/netting/keeper/',
    pinned: false,
    pinNote: 'A property of the message set, so changing it takes a binary upgrade.',
  },
  {
    what: 'A netting participant cannot owe more than they have posted.',
    how: 'The cap is checked on the net position rather than on the obligation, at '
      + 'submission time. Collateral is posted before anything can be owed.',
    where: 'x/netting/keeper/msg_server_obligation.go',
    pinned: false,
    pinNote: '',
  },
];

// ---------------------------------------------------------------------------
// Signing: the per-message decision.
//
// Following clients/land's rule — read cosmos.msg.v1.signer off each proto and
// decide, per message, what a browser may do about it. The three answers are
// 'sign' (the browser may compose and submit it), 'propose' (it belongs in an
// x/group proposal, because the signer is a group account and one person
// clicking is not the decision the account represents), and 'command' (the
// console prints the CLI invocation and signs nothing).
//
// The starting position for this module is NOT the browser. x/enforcement
// contains the only messages on this chain that take a citizen's money, and
// every one of its authorities is either a validator operator key or an x/group
// policy account. A freeze button in a web page is wrong for a reason that has
// nothing to do with cryptography: the message is a legal act performed by an
// office, and the office's own procedure — a proposal its members vote on, or a
// validator's operator key on an operator's machine — is the record that makes
// it accountable afterwards. A button collapses that into a click nobody can
// audit, and the person it happens to cannot ask who decided.
//
// So this console signs NOTHING. It is a reading surface. Where an action
// exists, it emits the command and names who must run it.
// ---------------------------------------------------------------------------

export const SIGNING = [
  {
    type: '/blockchain.enforcement.v1.MsgOpenCase',
    signer: 'opener',
    decision: 'command',
    who: 'A bonded validator\'s operator key, or a holder of ROLE_ENFORCEMENT_AUTHORITY '
      + 'for the target\'s recorded country.',
    why: 'Opening a case freezes the target in the same block. This is the fastest '
      + 'power in the module and the one a single signature can use, so it is also '
      + 'the one that most needs the signature to be made deliberately, on the '
      + 'signer\'s own machine, under a key that names an office. A validator\'s '
      + 'operator key does not belong in a browser at all.',
    command: 'blockchaind tx enforcement open-case <target> freeze --reason <reason> --from <operator>',
  },
  {
    type: '/blockchain.enforcement.v1.MsgEmergencyFreeze',
    signer: 'authority',
    decision: 'command',
    who: 'A holder of ROLE_ENFORCEMENT_AUTHORITY, scoped to the target\'s country. '
      + 'On this chain those are x/group policy accounts.',
    why: 'Same reasoning as opening a case, and more so: this path exists precisely '
      + 'for the cases where the validator vote is too slow, which is exactly when '
      + 'a convenient button would get used carelessly. The freeze it imposes is '
      + 'still provisional, still lapses unless validators confirm it, and still '
      + 'cannot take anything.',
    command: 'blockchaind tx enforcement emergency-freeze <target> --reason <reason> --from <authority>',
  },
  {
    type: '/blockchain.enforcement.v1.MsgVoteCase',
    signer: 'voter',
    decision: 'command',
    who: 'A bonded validator\'s operator key.',
    why: 'An operator key. It signs blocks; it is not loaded into a web page to click '
      + 'yes on a seizure.',
    command: 'blockchaind tx enforcement vote-case <case-id> yes --from <operator>',
  },
  {
    type: '/blockchain.enforcement.v1.MsgOmbudsmanVeto',
    signer: 'ombudsman',
    decision: 'command',
    who: 'The ombudsman named in the module parameters. None is appointed on this '
      + 'chain today.',
    why: 'The one message whose whole purpose is to be usable quickly, which is the '
      + 'best argument for a button and still not a good enough one: the ombudsman '
      + 'is an office appointed outside the validator set, and its veto is a formal '
      + 'act that has to be attributable to the office rather than to whoever had '
      + 'the page open. The reason string is stored forever and appended to the '
      + 'case, so it is drafted, not typed into a box.',
    command: 'blockchaind tx enforcement ombudsman-veto <case-id> --reason <reason> --from <ombudsman>',
  },
  {
    type: '/blockchain.enforcement.v1.MsgSweep',
    signer: 'sender',
    decision: 'command',
    who: 'Anyone. The sweep is permissionless once a case has passed and its delay '
      + 'has elapsed.',
    why: 'The only message here a browser could defensibly sign, since the signer is '
      + 'unprivileged and the message carries no discretion — it executes a decision '
      + 'already taken. It is still a command, because a console that can move '
      + 'somebody\'s money on a click is a different kind of object from one that '
      + 'cannot, and this one is worth keeping in the second category. Note the '
      + 'chain refuses a sweep while a case is HELD: that refusal is what stops the '
      + 'permissionless path bypassing the delay and the veto window.',
    command: 'blockchaind tx enforcement sweep <case-id> --from <anyone>',
  },
  {
    type: '/blockchain.enforcement.v1.MsgUpdateParams',
    signer: 'authority',
    decision: 'propose',
    who: 'The gov module account.',
    why: 'A governance proposal. Four of these parameters are additionally pinned to '
      + 'x/constitution and cannot be moved by a proposal at all.',
    command: 'compose a MsgUpdateParams inside a gov proposal',
  },
  {
    type: '/blockchain.netting.v1.MsgPostReserve',
    signer: 'participant',
    decision: 'command',
    who: 'An approved participant institution.',
    why: 'A treasury operation at a bank, run from the bank\'s own systems against '
      + 'its own key. There is no citizen in this flow and no reason for a browser '
      + 'to hold the key.',
    command: 'blockchaind tx netting post-reserve <participant> <amount> --from <participant>',
  },
  {
    type: '/blockchain.netting.v1.MsgSubmitObligation',
    signer: 'from_participant',
    decision: 'command',
    who: 'The debtor institution, and only the debtor.',
    why: 'Submitted by a batch process against a hash of an off-chain reconciliation, '
      + 'thousands at a time. This is machine-to-chain traffic; a human interface for '
      + 'it would be the wrong shape even if the key were available.',
    command: 'blockchaind tx netting submit-obligation <from> <to> <denom> <amount> --batch-hash <hash>',
  },
];

// ---------------------------------------------------------------------------
// Presentation helpers that carry a judgement, so they are tested too.
// ---------------------------------------------------------------------------

/** Basis points as a percentage, to two places. 6667 is two thirds and must
 *  not render as "67%" — the difference between 66.67% and 67% is a real
 *  difference in how much power is needed. */
export function bpsPercent(bps) {
  return `${(Number(bps || 0) / 100).toFixed(2)}%`;
}

/**
 * A number of blocks as an estimated duration.
 *
 * Returns null rather than guessing when there is no measured block interval.
 * "about 12 hours" computed from an assumed 5-second block is a claim about
 * somebody's freeze that the console has no evidence for.
 */
export function blocksAsDuration(blocks, secondsPerBlock) {
  const n = Number(blocks);
  const s = Number(secondsPerBlock);
  if (!Number.isFinite(n) || !Number.isFinite(s) || s <= 0 || n <= 0) return null;
  const secs = n * s;
  const unit = (v, word) => `about ${v} ${word}${v === 1 ? '' : 's'}`;
  // The boundaries are at the unit, not at a comfortable multiple of it, so an
  // hour renders as "about 1 hour" rather than "about 60 minutes".
  if (secs < 90) return unit(Math.round(secs), 'second');
  if (secs < 3600) return unit(Math.round(secs / 60), 'minute');
  if (secs < 86400) return unit(Math.round(secs / 3600), 'hour');
  return unit(Math.round(secs / 86400), 'day');
}

/** The separator between groups of digits. A NARROW NO-BREAK SPACE rather than
 *  an ordinary one, because an amount that wraps across a line break stops
 *  being one number and starts looking like two — and the numbers on this page
 *  are the ones a reader compares by eye. */
export const GROUP_SEPARATOR = ' ';

/** An amount in base units, grouped, with the denomination kept beside it.
 *  No conversion to a display unit: this console does not know the exponent of
 *  an arbitrary denomination, and inventing one would misstate a seizure by
 *  six orders of magnitude. */
export function amount(coin) {
  if (!coin) return '—';
  const raw = String(coin.amount === undefined ? coin : coin.amount);
  const neg = raw.startsWith('-');
  const digits = neg ? raw.slice(1) : raw;
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, GROUP_SEPARATOR);
  return `${neg ? '−' : ''}${grouped}${coin.denom ? ` ${coin.denom}` : ''}`;
}

export function statusName(status) { return enumName(CASE_STATUS, status); }
export function actionName(action) { return enumName(CASE_ACTION, action); }
export function voteName(v) { return enumName(VOTE_OPTION, v); }
export function cycleStatusName(v) { return enumName(CYCLE_STATUS, v); }
export function denomStatusName(v) { return enumName(DENOM_STATUS, v); }

/** The tone a status should carry. A case that was VETOED or REVERSED is a
 *  check that worked, not a failure, and colouring it red would tell the
 *  reader the opposite of what happened. */
export function statusTone(status) {
  switch (Number(status)) {
    case 1: return 'warn';  // VOTING — live, needs attention
    case 2: return 'bad';   // PASSED — a seizure was decided
    case 7: return 'warn';  // HELD — decided, waiting out the delay
    case 3: return 'ok';    // REJECTED
    case 4: return 'ok';    // EXPIRED
    case 5: return 'mute';  // WITHDRAWN
    case 6: return 'ok';    // REVERSED — the check worked
    case 8: return 'ok';    // VETOED — the check worked
    default: return 'mute';
  }
}
