/**
 * What a deal actually means, computed the way the chain computes it.
 *
 * Everything in this file is pure: it takes decoded records and returns numbers
 * and words. Nothing here fetches, and that is what makes it testable against
 * the keeper's own arithmetic rather than against a screenshot.
 *
 * ===========================================================================
 * THE RULE THIS FILE FOLLOWS
 * ===========================================================================
 *
 * Every figure a person acts on is reproduced from x/syndicate's own
 * expression, in the same order, with the same rounding. Not because a
 * discrepancy would be large — it would usually be one unit — but because a
 * platform whose console and whose chain disagree about a fee by one unit is a
 * platform nobody can tell has disagreed about something larger.
 *
 * Three places this matters, and all three are tested:
 *
 *   ENTRY FEE. `raised * (sourcing + syndication) / 10000`, integer division,
 *   remainder to the drawdown. Computed on the RAISE, not on the target.
 *
 *   ALLOCATION. Tokens are minted one per smallest unit of settlement raised,
 *   and allocated against the GROSS commitment. An investor's share is their
 *   cheque over the raise. Allocating against the net would charge the fee
 *   twice — once in cash and once in ownership — and the arithmetic would still
 *   balance, which is what makes it the easy mistake.
 *
 *   ENTITLEMENT. `balance * (dealIndex - positionIndex) / 1e18`, plus whatever
 *   was already accrued. The index is a 1e18 fixed point, so it is BigInt
 *   throughout; as a double it loses precision before the first distribution.
 *
 * ===========================================================================
 * ARITHMETIC IS BIGINT, DISPLAY IS STRINGS
 * ===========================================================================
 *
 * `cosmos.Int` is arbitrary precision and arrives as a decimal string. Nothing
 * in this file converts one to a Number. The formatters at the bottom go from
 * BigInt base units straight to a display string without passing through a
 * double, so a figure is never rounded by the act of being shown.
 */

/** One hundred percent, in basis points. x/syndicate/types/keys.go. */
export const MAX_BPS = 10_000n;

/** The fixed-point scale the distribution index is carried at. Same file. */
export const INDEX_SCALE = 1_000_000_000_000_000_000n;

/** A deal token's denomination prefix. Same file. */
export const DENOM_PREFIX = 'syn/';

const big = (v) => BigInt(String(v ?? '0').trim() || '0');

/* ========================================================================== */
/*  The book                                                                  */
/* ========================================================================== */

/**
 * How full a deal is, and — the question that actually matters — whether it
 * will happen at all.
 *
 * Two percentages rather than one. A book at 40% of target sounds
 * disappointing and a book at 40% of target that has passed its minimum is a
 * deal that is going to close; those are different facts and a single bar
 * showing only the first buries the second. `viable` is the one an investor
 * deciding whether to commit is actually asking about.
 *
 * `room` is what is left, because "there is room for 12,000 more" is the number
 * somebody sizing a cheque needs, and it is also the number the chain will
 * refuse them against.
 */
export function book(deal) {
  const target = big(deal.target);
  const minimum = big(deal.minimum);
  const raised = big(deal.raised);

  // Basis points rather than a float. The bar is drawn from this and a
  // percentage that came from a division of doubles would be off in the last
  // place on exactly the deals where the last place is a whole cheque.
  const pct = (part, whole) =>
    (whole > 0n ? Number((part * MAX_BPS) / whole) / 100 : 0);

  return {
    target,
    minimum,
    raised,
    room: raised >= target ? 0n : target - raised,
    shortfall: raised >= minimum ? 0n : minimum - raised,
    filledPct: Math.min(100, pct(raised, target)),
    // Where the minimum sits along the target bar, so the threshold can be
    // drawn rather than described.
    minimumPct: Math.min(100, pct(minimum, target)),
    viable: raised >= minimum,
    full: raised >= target,
  };
}

/**
 * The fee waterfall, in money.
 *
 * Basis points are what the deal is written in and money is what a person
 * decides on. Both are returned; the page leads with the money.
 *
 * The management fee is NOT in this waterfall, and leaving it out is the
 * decision rather than the omission. It comes off each distribution as it is
 * paid, not off the raise — so putting it here would tell an investor they are
 * paying it today, and putting it nowhere would tell them it does not exist.
 * It has its own function below.
 */
export function entryFees(deal, amount = deal.raised) {
  const base = big(amount);
  const sourcing = BigInt(deal.fees?.sourcing_bps ?? 0);
  const syndication = BigInt(deal.fees?.syndication_bps ?? 0);
  const bps = sourcing + syndication;

  // Integer division, matching CloseDeal. The remainder stays with the
  // invested amount, which is the direction that favours the company.
  const fee = (base * bps) / MAX_BPS;

  return {
    bps: Number(bps),
    sourcingBps: Number(sourcing),
    syndicationBps: Number(syndication),
    gross: base,
    fee,
    invested: base - fee,
  };
}

/**
 * What the management fee takes out of one distribution.
 *
 * Charged per distribution rather than annually against committed capital, and
 * the reason belongs on the screen: an annual fee on an illiquid position has
 * to be collected from somebody — by calling more capital, which a syndicate
 * cannot do, or by selling down the position, which it cannot either. Charged
 * here, the platform is paid when and only when the investors are.
 */
export function managementFee(deal, distribution) {
  const gross = big(distribution);
  const bps = BigInt(deal.fees?.management_bps ?? 0);
  const fee = (gross * bps) / MAX_BPS;
  return { bps: Number(bps), gross, fee, net: gross - fee };
}

/**
 * What one cheque buys, before the deal closes.
 *
 * A projection and labelled as one: the share depends on what everybody else
 * commits, so the honest statement is "if the book closes here". Computed
 * against the CURRENT raise including this cheque, which is the only figure
 * that exists yet.
 */
export function projectedShare(deal, cheque) {
  const amount = big(cheque);
  const raised = big(deal.raised);
  const total = raised + amount;
  if (total === 0n) return { shareBps: 0, tokens: 0n };
  return {
    // One token per smallest unit raised, so the cheque IS the allocation.
    tokens: amount,
    shareBps: Number((amount * MAX_BPS) / total),
  };
}

/**
 * What the deal has returned, as a proportion of what was put in.
 *
 * `distributed` is NET of the management fee — it is what actually reached the
 * holders — and `raised` is what they subscribed. So this is the figure an
 * investor means by "how much of my capital has come back", and it is exact at
 * the deal level because both numbers are on the deal.
 *
 * Reported as a PERCENTAGE OF CAPITAL and as a multiple, because those are the
 * two ways the same fact gets said in this business and each is unreadable to
 * somebody used to the other. Nothing here is annualised: an IRR needs the date
 * of every cash flow, the chain holds the distribution total rather than its
 * schedule, and a number that looked like an IRR but ignored timing would be
 * worse than no number at all.
 *
 * It is a RETURN TO DATE and never a valuation. A deal that has returned 40% of
 * capital may be worth three times that or nothing; what remains invested is not
 * something a register of payments can know, and the page must not imply it
 * does.
 */
export function returnToDate(deal) {
  const raised = big(deal.raised);
  const distributed = big(deal.distributed);
  if (raised === 0n) {
    return { pctOfCapital: 0, multiple: 0, bps: 0, distributed, raised, any: false };
  }

  // Basis points then divided, so the ratio is not taken through a float on
  // numbers that are arbitrary precision on the wire.
  const bps = Number((distributed * MAX_BPS) / raised);
  return {
    raised,
    distributed,
    any: distributed > 0n,
    bps,
    pctOfCapital: bps / 100,
    multiple: bps / 10000,
  };
}

/**
 * A return expressed the other way round: "0.83x". Two decimals always, because
 * a multiple printed as "1x" reads as exactly break-even and 1.004 is not.
 */
export function multiple(bps) {
  return `${(Number(bps) / 10000).toFixed(2)}×`;
}

/**
 * What a holding of this size has been paid across the deal's life.
 *
 * `balance / supply * distributed`, in that order so the division is taken last
 * and the result is not rounded twice. Computed from the balance rather than
 * from the `share_bps` the query returns, because that field is a rounded
 * integer of basis points and a percentage applied to money is a percentage
 * rounding money.
 *
 * THE CONDITION IS PART OF THE ANSWER, and the caller must print it. This is
 * what a position of this size is entitled to over the WHOLE life of the deal,
 * which equals what this holder has been paid only if they have held it since
 * the close. Somebody who bought on the secondary market is credited here with
 * distributions that were paid to the seller, before they owned anything.
 *
 * The chain could not answer it exactly even if asked. The index mechanism
 * carries an accrual per position, not a register of payments per holder — that
 * is precisely what makes distributing to thousands of holders cost one write
 * instead of thousands — so "what has THIS account actually received" is not a
 * fact the state retains. `claimable` is exact and is the figure to lead with;
 * this one is context.
 */
export function holderShare(deal, balance) {
  const held = big(balance);
  const supply = big(deal.supply);
  const distributed = big(deal.distributed);
  if (supply === 0n || held === 0n) {
    return { attributed: 0n, shareBps: 0, whole: supply > 0n && held === supply };
  }
  return {
    attributed: (held * distributed) / supply,
    shareBps: Number((held * MAX_BPS) / supply),
    whole: held === supply,
  };
}

/* ========================================================================== */
/*  Entitlement                                                               */
/* ========================================================================== */

/**
 * What a holder has accrued, recomputed here rather than only read.
 *
 * The chain answers this — the Entitlement query exists precisely so that a
 * client need not — and this function is for the screen that shows the
 * WORKING: balance, the index the position was last settled at, the index the
 * deal has reached, and the multiplication between them. A holder who has been
 * paid less than they expected needs to see which of those four numbers is not
 * what they thought, and a single total tells them nothing.
 *
 * It also makes the recomputation checkable against the chain's answer, and
 * syndicate.test.js asserts they agree.
 */
export function entitlement({ dealIndex, positionIndex, accrued, balance }) {
  const idx = big(dealIndex);
  const at = big(positionIndex);
  const held = big(balance);
  const already = big(accrued);

  const delta = idx > at ? idx - at : 0n;
  const unsettled = delta > 0n && held > 0n ? (held * delta) / INDEX_SCALE : 0n;

  return { delta, unsettled, settled: already, total: already + unsettled };
}

/**
 * A holder's share of a closed deal, in basis points.
 *
 * Their balance over the supply, and nothing else — no historical ownership,
 * no snapshot. That is the whole claim the product is sold on: a distribution
 * reaches whoever holds the token at that moment, including somebody who bought
 * it in a private sale the platform was never told about.
 */
export function shareBps(balance, supply) {
  const held = big(balance);
  const total = big(supply);
  if (total === 0n) return 0;
  return Number((held * MAX_BPS) / total);
}

/* ========================================================================== */
/*  What state a deal is in, said in words                                    */
/* ========================================================================== */

/**
 * The state, and what it means for the person reading.
 *
 * `can` is the practical half: what is possible right now. It is derived from
 * the state AND the clock, because OPEN past its closing date is a materially
 * different position from OPEN before it — in the first, anybody may abort the
 * deal and take their money back if the minimum was missed, and that is the
 * single most useful thing an investor in a failing raise can be told.
 */
export function standing(deal, now) {
  const b = book(deal);
  const closed = Number(now) >= Number(deal.closes_at);

  switch (deal.state) {
    case 'DEAL_STATE_OPEN':
      if (!closed) {
        return {
          state: 'open',
          // The tone follows viability, not merely "is it open". A card drawn
          // calm while its own sentence says the minimum is not covered is the
          // page disagreeing with itself, and the colour is what a reader
          // scanning a list takes away. It matches the bar, which is coloured
          // by the same fact.
          tone: b.viable ? 'ok' : 'warn',
          label: 'Open',
          what: b.viable
            ? 'The minimum is covered. Commitments are still being taken up to the allocation.'
            // Formatted, not interpolated raw. A shortfall of `1600000` in a
            // sentence next to "1.40 YML raised" reads as a different currency
            // and a catastrophic gap; it is 1.60 YML. Found by rendering the
            // page against a seeded chain, which is the only place a figure's
            // UNITS can be wrong while every test passes.
            : `Below its minimum by ${money(b.shortfall, deal.settlement_denom)}. `
              + 'If the window runs out here, everybody is refunded in full.',
          can: ['commit', 'withdraw'],
        };
      }
      if (b.viable) {
        return {
          state: 'closing', tone: 'warn', label: 'Window run',
          what: 'The window has run and the minimum is covered. The operator may now close it.',
          can: ['withdraw'],
        };
      }
      return {
        state: 'failed', tone: 'bad', label: 'Minimum missed',
        what: 'The window has run and the minimum was not met. Anybody may abort this deal — '
          + 'nobody needs the operator\'s cooperation to get their money back.',
        can: ['withdraw', 'abort'],
      };

    case 'DEAL_STATE_CLOSED':
      return {
        state: 'closed', tone: 'ok', label: 'Funded',
        what: 'The company has been funded and the tokens are in issue. '
          + 'Distributions are paid to whoever holds them at the time.',
        can: ['claim', 'list', 'buy'],
      };

    case 'DEAL_STATE_ABORTED':
      return {
        state: 'aborted', tone: 'mute', label: 'Aborted',
        what: 'This deal did not happen. Every commitment was refunded in full, fees included — '
          + 'no service was delivered, so nothing was charged for the attempt.',
        can: [],
      };

    case 'DEAL_STATE_REALISED':
      return {
        state: 'realised', tone: 'mute', label: 'Realised',
        what: 'The investment has been exited and the final distribution paid. Trading has '
          + 'stopped: every token is now a claim on a known fixed sum, and a market still '
          + 'quoting one would be a trap for whoever bought last.',
        can: ['claim'],
      };

    default:
      return {
        state: 'unknown', tone: 'mute', label: deal.state,
        what: 'This console does not recognise that state, which means the chain is running '
          + 'newer code than this page.',
        can: [],
      };
  }
}

/**
 * Whether a deal token can move at all.
 *
 * Two separate refusals live behind this and they are worth naming separately,
 * because a holder told only "transfer refused" will assume the first when it
 * is usually the second:
 *
 *   THE DEAL IS REALISED. Every transfer of the token is halted, deliberately.
 *   There is nothing left to buy.
 *
 *   THE OTHER SIDE IS NOT ONBOARDED. A deal token is an ordinary bank
 *   denomination and moves by MsgSend, by authz, through a treasury — so the
 *   KYC gate is a send restriction rather than a check in the platform's own
 *   handlers, and it binds on every one of those paths.
 */
export function transferable(deal) {
  if (!deal.denom) {
    return { ok: false, why: 'This deal has issued no tokens yet — it has not closed.' };
  }
  if (deal.state === 'DEAL_STATE_REALISED') {
    return { ok: false, why: 'Realised. Transfers of this token are halted by the chain.' };
  }
  if (deal.state !== 'DEAL_STATE_CLOSED') {
    return { ok: false, why: `A ${deal.state.replace('DEAL_STATE_', '').toLowerCase()} deal has no live token.` };
  }
  return {
    ok: true,
    why: 'Both sides must have completed onboarding. The chain enforces that on the transfer '
      + 'itself, not in the platform\'s software, so it binds however the tokens are moved.',
  };
}

/* ========================================================================== */
/*  Money, as a person reads it                                               */
/* ========================================================================== */

/**
 * The settlement currencies a deal can be denominated in.
 *
 * Everything on this chain is held in millionths so that a fee or an accrual
 * has room to be exact. `minor` is what a person QUOTES it in, which is a
 * different question — two places for a dollar, none in practice for a CFA
 * franc — and printing six decimals on a franc is how a demonstrator stops
 * looking like a financial product.
 */
export const DENOMS = {
  uusdc: { symbol: 'USDC', exponent: 6, minor: 2, name: 'USD Coin' },
  ueurc: { symbol: 'EURC', exponent: 6, minor: 2, name: 'Euro Coin' },
  uusd: { symbol: 'USD', exponent: 6, minor: 2, name: 'US Dollar' },
  uxof: { symbol: 'XOF', exponent: 6, minor: 0, name: 'West African CFA franc' },
  uxaf: { symbol: 'XAF', exponent: 6, minor: 0, name: 'Central African CFA franc' },
  uyml: { symbol: 'YML', exponent: 6, minor: 2, name: 'Yamale' },
};

/**
 * What a denomination is, including when we do not know.
 *
 * An unknown denom is reported as unknown rather than guessed at. A stablecoin
 * registered after this page was written, or an IBC voucher, has an exponent
 * this file cannot invent — and dividing by a million on the assumption that
 * every denom starts with a `u` is how a position gets displayed at a
 * millionth of its value with no indication anything is wrong.
 */
export function denomInfo(denom) {
  const known = DENOMS[denom];
  if (known) return { ...known, denom, known: true };

  if (denom.startsWith(DENOM_PREFIX)) {
    // A deal token. One per smallest unit of settlement raised, so it has the
    // same exponent as whatever the deal settles in — but this function does
    // not have the deal, so it says so and the caller passes the settlement
    // denom when it matters.
    return { denom, symbol: `deal ${denom.slice(DENOM_PREFIX.length)}`, exponent: 0, minor: 0, name: 'Deal token', known: true };
  }
  return { denom, symbol: denom, exponent: 0, minor: 0, name: '', known: false };
}

/**
 * Base units to a display string, without going through a double.
 *
 * The whole and fractional parts are separated by integer division on BigInt,
 * then the fraction is rendered as digits. A position worth 9,007,199,254.740993
 * USDC is beyond a double's integer precision, and this is a product that
 * intends to hold sums like that.
 *
 * Digits are grouped with thin spaces rather than commas, because this console
 * is read in French and in English and a comma is a decimal separator in one of
 * them. A grouped figure that could be read as a hundredth of itself is worse
 * than an ungrouped one.
 */
export function amount(base, denom, { decimals } = {}) {
  const info = denomInfo(denom);
  const places = decimals ?? info.minor ?? 0;
  const v = big(base);
  const negative = v < 0n;
  const abs = negative ? -v : v;

  const scale = 10n ** BigInt(info.exponent);
  const whole = info.exponent === 0 ? abs : abs / scale;
  const frac = info.exponent === 0 ? 0n : abs % scale;

  let text = group(whole.toString());
  if (places > 0) {
    // Truncated, never rounded up. A balance shown as more than it is invites
    // somebody to try to send it.
    const digits = frac.toString().padStart(info.exponent, '0').slice(0, places);
    text += `.${digits}`;
  }
  return `${negative ? '−' : ''}${text}`;
}

/** The same figure with its unit, which is what a sentence needs. */
export function money(base, denom, opts) {
  const info = denomInfo(denom);
  const value = amount(base, denom, opts);
  if (!info.known) {
    // Honest about the unit rather than silently printing base units as though
    // they were whole ones.
    return `${group(big(base).toString())} ${denom} (base units)`;
  }
  return `${value} ${info.symbol}`;
}

/** Thin spaces every three digits, from the right. */
export function group(digits) {
  return String(digits).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
}

/** Basis points as a percentage, at the precision basis points actually carry. */
export function pct(bps) {
  const v = Number(bps) / 100;
  return `${Number.isInteger(v) ? v : v.toFixed(2)}%`;
}

/* ========================================================================== */
/*  Time                                                                      */
/* ========================================================================== */

/**
 * How long until a deal closes, in the units a person thinks in.
 *
 * Past the deadline this returns the elapsed time with `past` set, rather than
 * a negative number or the word "closed". A raise whose window ran out
 * yesterday and which nobody has aborted is a real and common state, and
 * "closed 1 day ago" is the sentence that makes somebody go and abort it.
 */
export function until(deadline, now) {
  const seconds = Number(deadline) - Number(now);
  const past = seconds < 0;
  let s = Math.abs(seconds);

  const days = Math.floor(s / 86400); s -= days * 86400;
  const hours = Math.floor(s / 3600); s -= hours * 3600;
  const minutes = Math.floor(s / 60);

  let text;
  if (days >= 2) text = `${days} days`;
  else if (days === 1) text = `1 day ${hours}h`;
  else if (hours >= 1) text = `${hours}h ${minutes}m`;
  else text = `${minutes}m`;

  return { past, days, text, phrase: past ? `${text} ago` : `in ${text}` };
}

/** A Unix second as a date, or a dash. Dates only: the hour is never the point. */
export function day(unix) {
  const n = Number(unix);
  if (!n) return '—';
  return new Date(n * 1000).toISOString().slice(0, 10);
}

/** An address, shortened but never silently — the full value is the title. */
export function shortAddr(a, keep = 12) {
  const s = String(a ?? '');
  return s.length <= keep + 4 ? s : `${s.slice(0, keep)}…${s.slice(-4)}`;
}
