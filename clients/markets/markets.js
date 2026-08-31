/**
 * The markets console's judgement, separated from its rendering.
 *
 * Three modules on this chain put a price on something — x/oracle agrees what a
 * currency is worth, x/amm prices a trade against a curve, x/stablecoin decides
 * who may bring a currency into existence — and none of them had an interface.
 * This file is everything the console decides; index.html is everything it
 * draws.
 *
 * The split is not tidiness. Every function below is a number somebody is about
 * to act on, and a rendered page cannot be checked. A swap quote that disagrees
 * with the keeper by one unit is a transaction that fails at the node with
 * `ErrSlippage` after the trader has already signed; a staleness calculation
 * that rounds the wrong way is a price shown as usable in the minute it stopped
 * being usable. So the arithmetic lives here, in pure functions over what the
 * chain returned, and it has tests.
 *
 * Everything is BigInt. Reserves on this chain are already 3×10^10 base units
 * and a product of two reserves passes 2^53 immediately; a swap quote computed
 * in doubles is wrong before anybody has done anything unusual.
 */

// ---------------------------------------------------------------------------
// The chain
// ---------------------------------------------------------------------------

/** The chain these commands are for. One place, so a devnet reset is one edit. */
export const CHAIN = {
  id: 'yamale-devnet-2',
  bin: 'blockchaind',
  home: '~/.blockchain',
  fees: '2000uyml',
  gas: '400000',
};

/**
 * Message type URLs.
 *
 * The prefix is `blockchain.`, not `yamale.blockchain.` — the proto package is
 * `blockchain.amm.v1` while the Go module path is `yamale/blockchain/...`, and
 * the REST route is `/yamale/blockchain/amm/v1/...`. Three spellings of the same
 * module, of which exactly one is the type URL. Verified against the generated
 * tx.pb.go in each module's types package.
 */
export const MESSAGES = {
  swap: '/blockchain.amm.v1.MsgSwap',
  createPool: '/blockchain.amm.v1.MsgCreatePool',
  joinPool: '/blockchain.amm.v1.MsgJoinPool',
  exitPool: '/blockchain.amm.v1.MsgExitPool',
  submitRates: '/blockchain.oracle.v1.MsgSubmitExchangeRates',
  delegateFeeder: '/blockchain.oracle.v1.MsgDelegateFeeder',
  applyAppraiser: '/blockchain.oracle.v1.MsgApplyAppraiser',
  approveAppraiser: '/blockchain.oracle.v1.MsgApproveAppraiser',
  submitAppraisal: '/blockchain.oracle.v1.MsgSubmitAppraisal',
  registerCurrency: '/blockchain.stablecoin.v1.MsgRegisterCurrency',
  mintCoin: '/blockchain.stablecoin.v1.MsgMintCoin',
  burnCoin: '/blockchain.stablecoin.v1.MsgBurnCoin',
  approveIssuer: '/blockchain.stablecoin.v1.MsgApproveIssuer',
};

// ---------------------------------------------------------------------------
// Denominations
// ---------------------------------------------------------------------------
//
// Ported from clients/sdk/src/denom.ts, which is TypeScript with a build step
// and therefore cannot be imported by a page that deliberately has neither.
// THE CANONICAL COPY LIVES THERE. The behaviour is deliberately identical, and
// the tests below assert the two properties that make it so: never round up,
// and never invent an exponent.

/** The unit people read, and how many decimals to print it to. */
export const KNOWN_DENOMS = {
  uyml: { base: 'uyml', symbol: 'YML', exponent: 6, name: 'Yamale' },
  uusd: { base: 'uusd', symbol: 'USD', exponent: 6, name: 'US Dollar' },
  uusdc: { base: 'uusdc', symbol: 'USDC', exponent: 6, minorUnits: 2, name: 'USD Coin' },
  ueurc: { base: 'ueurc', symbol: 'EURC', exponent: 6, minorUnits: 2, name: 'Euro Coin' },
  uchf: { base: 'uchf', symbol: 'CHF', exponent: 6, name: 'Swiss Franc' },
  ueur: { base: 'ueur', symbol: 'EUR', exponent: 6, name: 'Euro' },
  ugbp: { base: 'ugbp', symbol: 'GBP', exponent: 6, name: 'Pound Sterling' },
  ujpy: { base: 'ujpy', symbol: 'JPY', exponent: 6, minorUnits: 0, name: 'Japanese Yen' },
  uaoa: { base: 'uaoa', symbol: 'AOA', exponent: 6, minorUnits: 2, name: 'Angolan Kwanza' },
  ubif: { base: 'ubif', symbol: 'BIF', exponent: 6, minorUnits: 0, name: 'Burundian Franc' },
  ubwp: { base: 'ubwp', symbol: 'BWP', exponent: 6, minorUnits: 2, name: 'Botswana Pula' },
  ucdf: { base: 'ucdf', symbol: 'CDF', exponent: 6, minorUnits: 2, name: 'Congolese Franc' },
  ucve: { base: 'ucve', symbol: 'CVE', exponent: 6, minorUnits: 2, name: 'Cape Verdean Escudo' },
  udjf: { base: 'udjf', symbol: 'DJF', exponent: 6, minorUnits: 0, name: 'Djiboutian Franc' },
  udzd: { base: 'udzd', symbol: 'DZD', exponent: 6, minorUnits: 2, name: 'Algerian Dinar' },
  uegp: { base: 'uegp', symbol: 'EGP', exponent: 6, minorUnits: 2, name: 'Egyptian Pound' },
  uern: { base: 'uern', symbol: 'ERN', exponent: 6, minorUnits: 2, name: 'Eritrean Nakfa' },
  uetb: { base: 'uetb', symbol: 'ETB', exponent: 6, minorUnits: 2, name: 'Ethiopian Birr' },
  ughs: { base: 'ughs', symbol: 'GHS', exponent: 6, minorUnits: 2, name: 'Ghanaian Cedi' },
  ugmd: { base: 'ugmd', symbol: 'GMD', exponent: 6, minorUnits: 2, name: 'Gambian Dalasi' },
  ugnf: { base: 'ugnf', symbol: 'GNF', exponent: 6, minorUnits: 0, name: 'Guinean Franc' },
  ukes: { base: 'ukes', symbol: 'KES', exponent: 6, minorUnits: 2, name: 'Kenyan Shilling' },
  ukmf: { base: 'ukmf', symbol: 'KMF', exponent: 6, minorUnits: 0, name: 'Comorian Franc' },
  ulrd: { base: 'ulrd', symbol: 'LRD', exponent: 6, minorUnits: 2, name: 'Liberian Dollar' },
  ulsl: { base: 'ulsl', symbol: 'LSL', exponent: 6, minorUnits: 2, name: 'Lesotho Loti' },
  ulyd: { base: 'ulyd', symbol: 'LYD', exponent: 6, minorUnits: 3, name: 'Libyan Dinar' },
  umad: { base: 'umad', symbol: 'MAD', exponent: 6, minorUnits: 2, name: 'Moroccan Dirham' },
  umga: { base: 'umga', symbol: 'MGA', exponent: 6, minorUnits: 2, name: 'Malagasy Ariary' },
  umru: { base: 'umru', symbol: 'MRU', exponent: 6, minorUnits: 2, name: 'Mauritanian Ouguiya' },
  umur: { base: 'umur', symbol: 'MUR', exponent: 6, minorUnits: 2, name: 'Mauritian Rupee' },
  umwk: { base: 'umwk', symbol: 'MWK', exponent: 6, minorUnits: 2, name: 'Malawian Kwacha' },
  umzn: { base: 'umzn', symbol: 'MZN', exponent: 6, minorUnits: 2, name: 'Mozambican Metical' },
  unad: { base: 'unad', symbol: 'NAD', exponent: 6, minorUnits: 2, name: 'Namibian Dollar' },
  ungn: { base: 'ungn', symbol: 'NGN', exponent: 6, minorUnits: 2, name: 'Nigerian Naira' },
  urwf: { base: 'urwf', symbol: 'RWF', exponent: 6, minorUnits: 0, name: 'Rwandan Franc' },
  uscr: { base: 'uscr', symbol: 'SCR', exponent: 6, minorUnits: 2, name: 'Seychellois Rupee' },
  usdg: { base: 'usdg', symbol: 'SDG', exponent: 6, minorUnits: 2, name: 'Sudanese Pound' },
  usle: { base: 'usle', symbol: 'SLE', exponent: 6, minorUnits: 2, name: 'Sierra Leonean Leone' },
  usos: { base: 'usos', symbol: 'SOS', exponent: 6, minorUnits: 2, name: 'Somali Shilling' },
  ussp: { base: 'ussp', symbol: 'SSP', exponent: 6, minorUnits: 2, name: 'South Sudanese Pound' },
  ustn: { base: 'ustn', symbol: 'STN', exponent: 6, minorUnits: 2, name: 'São Tomé and Príncipe Dobra' },
  uszl: { base: 'uszl', symbol: 'SZL', exponent: 6, minorUnits: 2, name: 'Swazi Lilangeni' },
  utnd: { base: 'utnd', symbol: 'TND', exponent: 6, minorUnits: 3, name: 'Tunisian Dinar' },
  utzs: { base: 'utzs', symbol: 'TZS', exponent: 6, minorUnits: 2, name: 'Tanzanian Shilling' },
  uugx: { base: 'uugx', symbol: 'UGX', exponent: 6, minorUnits: 0, name: 'Ugandan Shilling' },
  uxaf: { base: 'uxaf', symbol: 'XAF', exponent: 6, minorUnits: 0, name: 'Central African CFA Franc' },
  uxof: { base: 'uxof', symbol: 'XOF', exponent: 6, minorUnits: 0, name: 'West African CFA Franc' },
  uzar: { base: 'uzar', symbol: 'ZAR', exponent: 6, minorUnits: 2, name: 'South African Rand' },
  uzmw: { base: 'uzmw', symbol: 'ZMW', exponent: 6, minorUnits: 2, name: 'Zambian Kwacha' },
  uzwg: { base: 'uzwg', symbol: 'ZWG', exponent: 6, minorUnits: 2, name: 'Zimbabwe Gold' },
};

const LP_DENOM_PREFIX = 'amm/pool/';

/**
 * Resolves a denom to its display information.
 *
 * An unknown denom is shown as itself with exponent 0 rather than assumed to be
 * a millionth of something. Inventing an exponent misstates an amount by a
 * factor of a million, which is worse than printing a unit somebody can look up.
 */
export function resolveDenom(denom, registry = KNOWN_DENOMS) {
  const known = registry[denom];
  if (known) return known;

  if (denom.startsWith(LP_DENOM_PREFIX)) {
    const id = denom.slice(LP_DENOM_PREFIX.length);
    return { base: denom, symbol: `Pool ${id} shares`, exponent: 0, name: `Liquidity shares in pool ${id}` };
  }
  if (denom.startsWith('ibc/')) {
    return { base: denom, symbol: `IBC ${denom.slice(4, 10)}…`, exponent: 0, name: 'Transferred asset' };
  }
  return { base: denom, symbol: denom, exponent: 0, name: denom };
}

/** Converts a base-unit amount to its display value. Never rounds. */
export function toDisplayAmount(base, exponent) {
  const negative = typeof base === 'string' ? base.trimStart().startsWith('-') : base < 0n;
  let value;
  try {
    value = typeof base === 'bigint' ? base : BigInt(String(base).trim());
  } catch {
    return '0';
  }
  if (value < 0n) value = -value;

  const sign = negative ? '-' : '';
  if (exponent <= 0) return sign + value.toString();

  const divisor = 10n ** BigInt(exponent);
  const whole = value / divisor;
  const fraction = value % divisor;
  if (fraction === 0n) return sign + whole.toString();

  const fractionStr = fraction.toString().padStart(exponent, '0').replace(/0+$/, '');
  return `${sign}${whole.toString()}.${fractionStr}`;
}

/**
 * Thousands separators, through Intl as a BigInt rather than a Number.
 *
 * A pool reserve here is 30,000,000,000 base units today and a total supply will
 * pass 2^53; `Number(digits).toLocaleString()` is exact only below that, and it
 * fails by silently printing a different figure.
 */
export function groupDigits(value, locale = 'en-US') {
  const [whole, fraction] = String(value).split('.');
  const negative = whole.startsWith('-');
  const digits = (negative ? whole.slice(1) : whole).replace(/[^0-9]/g, '');
  let grouped;
  try {
    grouped = BigInt(digits || '0').toLocaleString(locale);
  } catch {
    grouped = digits || '0';
  }
  const sign = negative ? '-' : '';
  return fraction ? `${sign}${grouped}.${fraction}` : `${sign}${grouped}`;
}

/**
 * What a zero or absent amount renders as. An en dash, not a zero: an account
 * with no balance in a currency has never been paid in it, which is a different
 * fact from holding zero of it.
 */
export const EMPTY_AMOUNT = '–';

/**
 * Formats a base-unit amount for a person: `12500000uyml` → `12.5 YML`.
 *
 * Truncates to what the currency is quoted in, never rounds up. This is where
 * the zero-decimal currencies bite: `299999769uxof` is 299.999769 XOF, and the
 * franc has no subunit, so it prints as **299 XOF** and sits in a column beside
 * a genuine 300 XOF looking like an error. It is not an error — it is a third
 * of a franc short — and `truncatedFrom` below exists so the page can say so
 * rather than leave the reader to wonder.
 */
export function formatAmount(base, denom, options = {}) {
  const { withSymbol = true, group = true, maxDecimals, registry } = options;
  const info = resolveDenom(denom, registry);

  let display = toDisplayAmount(base, info.exponent);
  const places = maxDecimals ?? info.minorUnits;

  if (places !== undefined && display.includes('.')) {
    const [whole, fraction] = display.split('.');
    const trimmed = fraction.slice(0, places).replace(/0+$/, '');
    display = trimmed ? `${whole}.${trimmed}` : whole;
  }
  if (group) display = groupDigits(display);
  return withSymbol ? `${display} ${info.symbol}` : display;
}

/**
 * The exact figure, when the printed one was truncated — otherwise null.
 *
 * A page that prints `299 XOF` and nothing else has told the reader something
 * false by omission. This returns `299.999769` so it can be shown beside it.
 */
export function truncatedFrom(base, denom, registry) {
  const info = resolveDenom(denom, registry);
  const exact = toDisplayAmount(base, info.exponent);
  const shown = formatAmount(base, denom, { withSymbol: false, group: false, registry });
  return exact === shown ? null : exact;
}

/**
 * What a person typed, as base units.
 *
 * Strings and BigInt at every step. `Math.round(Number(input) * 10 ** exponent)`
 * — the form this replaces everywhere in these clients — is wrong twice: Number
 * loses precision above 2^53, and `0.07 * 1e6` is 70000.00000000001 in binary
 * floating point, so whether the rounding lands on the amount the person typed
 * depends on which decimal they happened to choose.
 *
 * Returns null for anything that is not a non-negative decimal, so the page can
 * disable the button rather than submit a guess.
 */
export function toBaseUnits(input, exponent) {
  if (!Number.isInteger(exponent) || exponent < 0 || exponent > 30) return null;

  // Group separators a person or a locale may have inserted: ordinary spaces,
  // non-breaking and narrow no-break spaces (fr), and apostrophes (de-CH).
  const cleaned = String(input).replace(/[\s  ']/g, '');
  if (cleaned === '') return null;

  const dots = (cleaned.match(/\./g) ?? []).length;
  const commas = (cleaned.match(/,/g) ?? []).length;
  if (dots > 1 || commas > 1 || (dots > 0 && commas > 0)) return null;

  const normalised = cleaned.replace(',', '.');
  if (!/^\d*(?:\.\d*)?$/.test(normalised) || normalised === '.') return null;

  const [whole = '', fraction = ''] = normalised.split('.');
  const kept = fraction.slice(0, exponent);
  const truncated = fraction.length > exponent && /[1-9]/.test(fraction.slice(exponent));

  const digits = `${whole || '0'}${kept.padEnd(exponent, '0')}`;
  return { base: BigInt(digits).toString(), truncated };
}

/** The same, resolving the exponent from the denom rather than from a guess. */
export function toBaseUnitsOf(input, denom, registry = KNOWN_DENOMS) {
  return toBaseUnits(input, resolveDenom(denom, registry).exponent);
}

// ---------------------------------------------------------------------------
// Addresses
// ---------------------------------------------------------------------------

/**
 * Whether a feeder address is the validator's own account rather than a
 * delegated hot key.
 *
 * x/oracle's FeederDelegation query returns the validator's own account when no
 * delegation has been made, so "who votes for this validator" and "has this
 * validator delegated" are different questions and the query answers only the
 * first. Getting the second wrong tells an operator they have a hot key set up
 * when their operator key is the one signing every vote period — which is the
 * exact risk feeder delegation exists to remove.
 *
 * A valoper address and its account address are the same 20 bytes under two
 * bech32 prefixes, so the encoded data section is identical and only the
 * human-readable part and the 6-character checksum differ. Comparing the middle
 * is therefore an exact test, and it needs no bech32 decoder in a page that has
 * no dependencies. A prefix-length guess — `startsWith(valoper.slice(0, 6))` —
 * is not: `ymlvaloper1cggu…` and `yml1cggu…` share no six-character prefix, so
 * that form calls every validator delegated. It did here.
 */
export function isOwnAccount(valoper, feeder) {
  if (!valoper || !feeder) return false;
  const dataOf = (addr) => {
    const sep = addr.lastIndexOf('1');
    if (sep < 1 || addr.length - sep < 8) return null;
    return addr.slice(sep + 1, -6);
  };
  const a = dataOf(String(valoper));
  const b = dataOf(String(feeder));
  return a !== null && a === b;
}

// ---------------------------------------------------------------------------
// x/amm — the swap curve
// ---------------------------------------------------------------------------

/** Basis points denominator, as the keeper spells it. */
export const BPS = 10000n;

/**
 * The pool's output for a given input, byte-for-byte the keeper's arithmetic.
 *
 * x/amm/keeper/msg_server_swap.go:
 *
 *     feeBps           := 10000 - pool.SwapFeeBps
 *     amountInAfterFee := tokenInAmount * feeBps / 10000        // Int.Quo, truncating
 *     amountOut        := reserveOut * amountInAfterFee / (reserveIn + amountInAfterFee)
 *
 * Both divisions are `math.Int.Quo`, which truncates toward zero. On
 * non-negative values that is floor, which is what BigInt `/` does, so the two
 * agree exactly — and they must, because a quote one unit above what the keeper
 * will pay is a `min_amount_out` the keeper rejects with ErrSlippage after the
 * trader has signed.
 *
 * THE ROUNDING DIRECTION IS THE PROTECTION, and the keeper's comment says why:
 * truncating this division rounds the trader's output *down*, leaving the
 * fractional remainder in the pool. The algebraically equivalent
 *
 *     reserveOut - (reserveIn * reserveOut) / (reserveIn + amountInAfterFee)
 *
 * truncates the subtrahend instead and therefore rounds the output *up*, paying
 * out up to one unit more than the curve allows on every swap and bleeding the
 * pool. This function must never be "simplified" into that form. The test
 * `swap output never exceeds the curve` is what stops it.
 *
 * The fee is not taken out of the pool: the full `tokenInAmount` is added to the
 * input reserve while only `amountInAfterFee` is used to price the trade, so the
 * fee stays as reserve and belongs to the liquidity providers. Uniswap v2.
 */
export function swapOut(reserveIn, reserveOut, amountIn, swapFeeBps) {
  const rIn = BigInt(reserveIn);
  const rOut = BigInt(reserveOut);
  const aIn = BigInt(amountIn);
  const fee = BigInt(swapFeeBps);
  if (aIn <= 0n || rIn <= 0n || rOut <= 0n) return 0n;

  const amountInAfterFee = (aIn * (BPS - fee)) / BPS;
  if (amountInAfterFee <= 0n) return 0n;
  return (rOut * amountInAfterFee) / (rIn + amountInAfterFee);
}

/**
 * A price as a decimal string, to `places` digits, truncated.
 *
 * Prices are the one place a console is tempted to reach for floating point.
 * This does the division in BigInt at a fixed scale instead, so a price is
 * exactly as precise as it claims to be and never a rounding artefact.
 */
export function ratio(numerator, denominator, places = 6) {
  const n = BigInt(numerator);
  const d = BigInt(denominator);
  if (d === 0n) return null;
  const scale = 10n ** BigInt(places);
  const scaled = (n * scale) / d;
  return toDisplayAmount(scaled, places);
}

/**
 * Everything the swap screen needs, in one object, from base units only.
 *
 * `priceImpactBps` compares the price this trade actually gets against the price
 * an infinitesimal trade would get — the marginal price at the current reserves.
 * It is a property of the *curve*, not of the fee, but the fee is inside the
 * number a trader receives, so both are reported and the impact figure below is
 * computed after the fee, which is what the trader actually experiences.
 *
 * `minReceived` is the number that goes in the transaction. It is the only
 * figure on the screen that is a promise: everything else is a quote against
 * reserves that any block may move.
 */
export function swapQuote({ reserveIn, reserveOut, amountIn, swapFeeBps, slippageBps = 50 }) {
  const rIn = BigInt(reserveIn);
  const rOut = BigInt(reserveOut);
  const aIn = BigInt(amountIn);
  const feeBps = BigInt(swapFeeBps);

  const out = swapOut(rIn, rOut, aIn, feeBps);
  const feeAmount = aIn - (aIn * (BPS - feeBps)) / BPS;

  // The minimum the trader will accept, truncated downward: rounding a floor
  // upward would set a minimum the pool cannot meet at the quoted price.
  const minReceived = (out * (BPS - BigInt(slippageBps))) / BPS;

  // What the same input would fetch at the marginal price, with no curve
  // movement and no fee: aIn * rOut / rIn. The shortfall against that is the
  // full cost of trading — impact plus fee — which is the honest comparison,
  // because a trader choosing not to trade keeps all of it.
  const idealOut = rIn > 0n ? (aIn * rOut) / rIn : 0n;
  const costBps = idealOut > 0n ? Number(((idealOut - out) * BPS) / idealOut) : 0;

  // Impact alone: the same comparison with the fee removed from the input, so a
  // pool's 0.3% fee is not reported as 0.3% of price impact on a tiny trade.
  const afterFee = (aIn * (BPS - feeBps)) / BPS;
  const idealAfterFee = rIn > 0n ? (afterFee * rOut) / rIn : 0n;
  const impactBps = idealAfterFee > 0n ? Number(((idealAfterFee - out) * BPS) / idealAfterFee) : 0;

  return {
    amountIn: aIn,
    amountOut: out,
    feeAmount,
    minReceived,
    impactBps,
    costBps,
    /** Reserves as they would stand after this trade, for the pool panel. */
    newReserveIn: rIn + aIn,
    newReserveOut: rOut - out,
    /** True when the trade cannot be priced at all — an empty or wrong side. */
    empty: out === 0n,
  };
}

/**
 * How alarmed to be about a price impact, and in words.
 *
 * The thresholds are a judgement, not a chain parameter, and they are stated
 * here rather than scattered through the markup so that changing them is one
 * edit and so that the test can pin them. A trader is warned *before* signing;
 * a warning after the fact is a receipt.
 */
export const IMPACT_THRESHOLDS = { notable: 100, high: 300, severe: 1000 }; // bps

export function impactVerdict(impactBps) {
  if (impactBps >= IMPACT_THRESHOLDS.severe) {
    return {
      level: 'bad',
      title: 'This trade moves the price against you by more than 10%',
      detail:
        'The pool is small relative to what you are selling. Splitting the trade will not ' +
        'help — the curve is the same either way — but a smaller trade costs proportionally less.',
    };
  }
  if (impactBps >= IMPACT_THRESHOLDS.high) {
    return {
      level: 'warn',
      title: 'This trade moves the price against you by more than 3%',
      detail: 'You are a large share of this pool. Check the minimum received before you sign.',
    };
  }
  if (impactBps >= IMPACT_THRESHOLDS.notable) {
    return { level: 'warn', title: 'Price impact above 1%', detail: 'Ordinary for a pool this size.' };
  }
  return { level: 'ok', title: 'Price impact under 1%', detail: '' };
}

// ---------------------------------------------------------------------------
// x/oracle — freshness
// ---------------------------------------------------------------------------

/**
 * How much life is left in a price.
 *
 * The rule this module is built on: **a value too old to trust is not a value**.
 * `max_rate_age_seconds` is not a display hint — past it, x/oracle's own
 * consumers must treat the price as unknown, and a transaction that depends on
 * it stops rather than proceeds on the last number anybody happened to report.
 *
 * So the console does not show "updated 14 minutes ago" and leave the reader to
 * do the subtraction against a parameter they have not seen. It shows how long
 * the price has left, and it shows the same fact as a bar that empties, because
 * "expiring" is a thing that should look like it is happening.
 *
 * The boundary is `age > maxAge` → expired, matching the keeper: at exactly
 * maxAge the price is still usable. Getting that backwards would show a usable
 * price as dead, which merely annoys, or a dead one as usable, which does not.
 */
export const EXPIRING_FRACTION = 0.75;

export function freshness(updatedAt, nowSeconds, maxAgeSeconds) {
  const updated = Number(updatedAt);
  const now = Number(nowSeconds);
  const maxAge = Number(maxAgeSeconds);

  if (!updated) return { state: 'none', ageSeconds: null, remaining: null, fraction: 0 };

  const age = now - updated;
  const remaining = maxAge - age;

  // A rate stamped in the future is stale, and this must match the keeper
  // rather than be reasoned about independently. x/oracle's IsStale reads
  //
  //     if observedAt <= 0 || observedAt > now { return true }
  //
  // so a future stamp is refused, not trusted: a value that claims to be from
  // a time that has not happened is a clock disagreement somewhere, and the
  // module would rather stop than act on it. A console that drew it as fresh
  // would be telling a reader a transaction will go through when the chain has
  // already decided it will not. `maxAgeSeconds == 0` disables staleness
  // entirely there, which is why the check below comes after this one.
  if (age < 0) {
    return { state: 'expired', ageSeconds: age, remaining: 0, fraction: 1, clockSkew: true };
  }
  if (!maxAge) return { state: 'fresh', ageSeconds: age, remaining: null, fraction: 0 };

  const fraction = Math.min(age / maxAge, 1);
  const state = age > maxAge ? 'expired' : fraction >= EXPIRING_FRACTION ? 'expiring' : 'fresh';
  return { state, ageSeconds: age, remaining, fraction };
}

/** "4 minutes", "2 hours", "3 days" — a duration a person reads, never a height. */
export function span(seconds) {
  const s = Math.abs(Math.round(Number(seconds)));
  if (s < 60) return `${s} second${s === 1 ? '' : 's'}`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m} minute${m === 1 ? '' : 's'}`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} hour${h === 1 ? '' : 's'}`;
  const d = Math.round(h / 24);
  if (d < 60) return `${d} day${d === 1 ? '' : 's'}`;
  const mo = Math.round(d / 30);
  return `${mo} month${mo === 1 ? '' : 's'}`;
}

/**
 * How much of the validator set stood behind a rate, in words.
 *
 * `voting_power_bps` is the share of stake that contributed to the median.
 * `vote_threshold_bps` is the minimum for a rate to be agreed at all. A rate at
 * the threshold and a rate at unanimity are both "the price"; they do not
 * deserve equal confidence, and the difference is invisible unless said.
 */
export function agreement(votingPowerBps, thresholdBps) {
  const power = Number(votingPowerBps || 0);
  const threshold = Number(thresholdBps || 0);
  const pct = (power / 100).toFixed(power % 100 === 0 ? 0 : 2);
  if (!power) return { level: 'mute', text: 'agreement not recorded' };
  if (power >= 9900) return { level: 'ok', text: `${pct}% of stake — effectively unanimous` };
  if (power <= threshold + 500) return { level: 'warn', text: `${pct}% of stake — a bare quorum agreed this` };
  return { level: 'ok', text: `${pct}% of stake agreed this` };
}

/**
 * What a validator's miss record means.
 *
 * x/oracle records misses rather than slashing for them, deliberately: "on a
 * small permissioned network an automatic slash mostly punishes the operator
 * whose VM rebooted". That makes the counter the only signal there is, and a
 * counter nobody displays is not a signal.
 *
 * A validator that has missed every window has never reported a price. That is
 * not "unreliable" — it is a feeder that was never started, and it is the
 * difference between a feed that is broken and a feed that does not exist.
 */
export function reporting(misses, windows) {
  const m = Number(misses || 0);
  const w = Number(windows || 0);
  if (!w) return { level: 'mute', rate: null, text: 'no voting rounds yet' };
  const missRate = m / w;
  const pct = (missRate * 100).toFixed(1).replace(/\.0$/, '');
  if (m >= w) return { level: 'bad', rate: missRate, text: 'has never reported a price' };
  if (missRate > 0.5) return { level: 'bad', rate: missRate, text: `missed ${pct}% of rounds` };
  if (missRate > 0.05) return { level: 'warn', rate: missRate, text: `missed ${pct}% of rounds` };
  return { level: 'ok', rate: missRate, text: `reported in ${(100 - missRate * 100).toFixed(1).replace(/\.0$/, '')}% of rounds` };
}

// ---------------------------------------------------------------------------
// x/stablecoin — registered is not approved
// ---------------------------------------------------------------------------

/**
 * Whether a denom can actually be minted, and what stands in the way.
 *
 * This is the distinction the module is built around and the one that costs
 * people an afternoon: `register-currency` succeeds. It returns code 0. It puts
 * an IssuerApplication in state with status `pending` and it publishes nothing.
 * The currency is not mintable. `mint-coin` on it fails with **code 1104**,
 * `sender is not the approved issuer for this denom`, and that message is easy
 * to read as "you used the wrong key" when what it means is "no governance
 * proposal has passed for this denom".
 *
 * Approval is a separate, authority-gated message — MsgApproveIssuer, signer
 * `authority`, the x/gov module account — which only exists at the end of a
 * governance proposal. Nothing an issuer signs can produce it.
 *
 * So the console never renders a currency as a single row. It renders the two
 * facts separately: registered, and approved. `state` is the verdict; `blocker`
 * is the sentence to put in front of somebody whose mint just failed.
 */
export function mintability(denom, application, approvedIssuer) {
  const status = String(application?.status ?? '').toLowerCase();

  if (!application) {
    return {
      state: 'unregistered',
      level: 'mute',
      label: 'not registered',
      blocker: `Nothing has been registered for ${denom}. A mint would fail with code 1104.`,
      canMint: false,
    };
  }
  if (approvedIssuer?.issuer) {
    return {
      state: 'approved',
      level: 'ok',
      label: 'mintable',
      blocker: null,
      canMint: true,
      issuer: approvedIssuer.issuer,
    };
  }
  if (status === 'rejected') {
    return {
      state: 'rejected',
      level: 'bad',
      label: 'refused',
      blocker: `Governance refused ${denom}. It may be registered again; it cannot be minted as it stands.`,
      canMint: false,
    };
  }
  return {
    state: 'pending',
    level: 'warn',
    label: 'registered, not approved',
    blocker:
      `${denom} is registered and cannot be minted. Registration succeeded — it returned code 0 — ` +
      `but minting needs a governance proposal carrying MsgApproveIssuer to pass first. ` +
      `Until it does, mint-coin fails with code 1104, "sender is not the approved issuer for this denom", ` +
      `however correct the issuer's key is.`,
    canMint: false,
  };
}

/**
 * Chain errors a person using this console can actually hit, in their words.
 *
 * The raw string stays available — an operator debugging a node needs it — but
 * the sentence in front of the person says what happened and what to do. The
 * codes are from x/amm/types/errors.go and x/stablecoin/types/errors.go.
 */
export const ERRORS = {
  'amm/1103': { what: 'That pool does not exist.', next: 'Check the pool id against the list.' },
  'amm/1104': {
    what: 'This pool does not trade those two currencies.',
    next: 'A pool holds exactly two denoms; pick a pair from the pool page.',
  },
  'amm/1107': {
    what: 'The pool would have paid less than your minimum.',
    next: 'The reserves moved between the quote and the block. Re-quote, or raise your slippage tolerance.',
  },
  'stablecoin/1104': {
    what: 'This denom has no approved issuer.',
    next: 'Registration is not approval. A governance proposal carrying MsgApproveIssuer has to pass first.',
  },
  'stablecoin/1101': {
    what: 'That denom is already registered or already pending.',
    next: 'Look it up on the currencies page before registering it again.',
  },
};

// ---------------------------------------------------------------------------
// Who signs what
// ---------------------------------------------------------------------------

/**
 * For each message, who the chain requires as signer and therefore what this
 * page may offer.
 *
 * Read off `option (cosmos.msg.v1.signer)` in each module's tx.proto, not
 * guessed from the field names. Three outcomes, and the choice is forced by the
 * signer rather than by convenience:
 *
 *   'browser'  — the signer is one person with one key, and the message is
 *                theirs alone. A wallet can sign it. This is the swap.
 *
 *   'proposal' — the signer is `authority`, which is the x/gov module account.
 *                No key exists for it and none can. The only thing that produces
 *                one of these messages is a governance proposal that passed, so
 *                the page composes the proposal rather than pretending to a
 *                button. MsgApproveIssuer and MsgApproveAppraiser are here.
 *
 *   'command'  — the signer is one key, but not one that belongs in a browser.
 *                A validator's feeder key sits on the validator host and votes
 *                every 12 blocks from a process, not from a page somebody has
 *                open; an issuer's mint key is a treasury key. Pasting either
 *                into a web page is the wrong instinct to encourage, so the page
 *                composes the command for the terminal where the key already is.
 *
 * The important asymmetry the brief names: a *feeder* is usually a plain key, so
 * MsgSubmitExchangeRates could technically be signed in a browser — and is still
 * 'command', because the thing that should be sending it is a price bot on a
 * schedule, not a human at a keyboard. An *issuer* usually is not a plain key at
 * all. Neither gets a button here.
 */
export const SIGNERS = {
  [MESSAGES.swap]: {
    signer: 'sender',
    mode: 'browser',
    why: 'One trader, one key, one trade. Nothing else has to agree.',
  },
  [MESSAGES.joinPool]: {
    signer: 'sender',
    mode: 'browser',
    why: 'The liquidity provider signs for their own deposit.',
  },
  [MESSAGES.exitPool]: {
    signer: 'sender',
    mode: 'browser',
    why: 'The liquidity provider signs for their own withdrawal.',
  },
  [MESSAGES.createPool]: {
    signer: 'creator',
    mode: 'command',
    why:
      'Permissionless in the module, but the first deposit sets the price the ' +
      'pool opens at, and getting it wrong is an immediate arbitrage loss. It ' +
      'belongs where somebody can check the two amounts before sending them.',
  },
  [MESSAGES.submitRates]: {
    signer: 'feeder',
    mode: 'command',
    why:
      'A plain key, and still not a browser one: rates are due every vote ' +
      'period, so the thing that should hold this key is a process on the ' +
      'validator host, not a page a person keeps open.',
  },
  [MESSAGES.delegateFeeder]: {
    signer: 'operator',
    mode: 'command',
    why:
      'Signed by the validator operator key — the one key whose compromise ' +
      'costs the most, and the reason feeder delegation exists at all. It does ' +
      'not go near a browser.',
  },
  [MESSAGES.applyAppraiser]: {
    signer: 'creator',
    mode: 'browser',
    why: 'Anyone may apply to be a valuer; it commits nothing and moves nothing.',
  },
  [MESSAGES.submitAppraisal]: {
    signer: 'appraiser',
    mode: 'command',
    why:
      'One key, but it signs a number a lender will act on, alongside the hash ' +
      'of the report it came from. That pairing is done at a desk with the ' +
      'document open, not in a form.',
  },
  [MESSAGES.approveAppraiser]: {
    signer: 'authority',
    mode: 'proposal',
    why: 'The x/gov module account. No key exists for it; only a passed proposal produces it.',
  },
  [MESSAGES.registerCurrency]: {
    signer: 'creator',
    mode: 'command',
    why:
      'Registration alone mints nothing, but it claims a denom permanently and ' +
      'names the account that would issue it — a treasury key, not a browser key.',
  },
  [MESSAGES.mintCoin]: {
    signer: 'issuer',
    mode: 'command',
    why:
      'The key that brings money into existence. An issuer is a bank or a ' +
      'central bank, and this key is held the way such a key is held.',
  },
  [MESSAGES.burnCoin]: {
    signer: 'issuer',
    mode: 'command',
    why: 'The same key as the mint, and the same reason.',
  },
  [MESSAGES.approveIssuer]: {
    signer: 'authority',
    mode: 'proposal',
    why:
      'The x/gov module account. This is the message that makes a registered ' +
      'currency mintable, and nothing an issuer signs can produce it.',
  },
};

// ---------------------------------------------------------------------------
// Composing commands
// ---------------------------------------------------------------------------

/** Shell-quotes a value only when it needs it, so ordinary denoms stay readable. */
export function sh(value) {
  const s = String(value ?? '');
  if (s !== '' && /^[A-Za-z0-9_@%+=:,.\/-]+$/.test(s)) return s;
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

const commonFlags = (from) => [
  `--from ${sh(from)}`,
  `--chain-id ${CHAIN.id}`,
  `--home ${CHAIN.home}`,
  `--fees ${CHAIN.fees}`,
  `--gas ${CHAIN.gas}`,
];

/**
 * One `blockchaind tx <module> …` command, wrapped for reading.
 *
 * Composed as a CLI command rather than as a transaction document for the
 * reason registrar.js gives: a document this page builds is a document this
 * page can get wrong, checked against nothing until it is broadcast, whereas a
 * command is parsed by the binary that owns the message and fails at the
 * terminal with the field named, in front of the person who can fix it.
 *
 * `args` are positionals in the order autocli declares them — verified against
 * each module's autocli.go, because which of the two a field is turns out to be
 * load-bearing.
 */
export function tx({ module: mod, sub, args = [], flags = [], from }) {
  const lines = [`${CHAIN.bin} tx ${mod} ${sub}`];
  [...args.map(sh), ...flags, ...commonFlags(from)].forEach((part) => {
    const last = lines[lines.length - 1];
    if (last.length + part.length + 1 <= 78) lines[lines.length - 1] = `${last} ${part}`;
    else lines.push(`  ${part}`);
  });
  return lines.map((l, i) => (i < lines.length - 1 ? `${l} \\` : l)).join('\n');
}

/**
 * The swap, as the terminal wants it.
 *
 * Positional order from x/amm/module/autocli.go:
 *   swap [pool-id] [token-in-denom] [token-in-amount] [token-out-denom] [min-amount-out]
 *
 * `minAmountOut` is passed rather than defaulted to zero. A swap sent with a
 * minimum of 0 accepts any price the pool happens to be at when the block
 * lands, which is the whole failure mode the slippage field exists to prevent.
 */
export function swapCommand({ poolId, inDenom, inAmount, outDenom, minOut, from = 'YOUR_KEY' }) {
  return tx({
    module: 'amm',
    sub: 'swap',
    args: [poolId, inDenom, inAmount, outDenom, minOut],
    from,
  });
}

/**
 * A governance proposal to approve an issuer, in the two steps it actually takes.
 *
 * The message is authority-gated, so the CLI has no `approve-issuer` subcommand
 * — autocli skips it. The only route is a `gov submit-proposal` carrying the
 * message as JSON, and the authority in that JSON must be the x/gov module
 * account, not the proposer.
 */
export function approveIssuerProposal({ denom, authority, title, summary, deposit = '10000000uyml', from = 'YOUR_KEY' }) {
  const doc = JSON.stringify(
    {
      messages: [
        {
          '@type': MESSAGES.approveIssuer,
          authority,
          denom,
          approve: true,
        },
      ],
      metadata: '',
      deposit,
      title: title || `Approve an issuer for ${denom}`,
      summary:
        summary ||
        `Make ${denom} mintable by the account that registered it. Until this passes, ` +
          `mint-coin on ${denom} fails with code 1104.`,
    },
    null,
    2,
  );
  const submit = [
    `${CHAIN.bin} tx gov submit-proposal proposal.json \\`,
    `  --from ${sh(from)} --chain-id ${CHAIN.id} --home ${CHAIN.home} \\`,
    `  --fees ${CHAIN.fees} --gas ${CHAIN.gas}`,
  ].join('\n');
  return { doc, submit };
}

/**
 * The command a validator runs to start reporting prices.
 *
 * One `--rates` per denom, not a JSON array — autocli binds a repeated message
 * field as a repeatable flag, and an array fails with "unexpected token [",
 * which reads like a malformed payload rather than the wrong shape entirely.
 * x/oracle's own autocli Long text says so; this composes the form that works.
 */
export function submitRatesCommand({ validator, rates, from = 'FEEDER_KEY' }) {
  const flags = rates.map((r) => `--rates ${sh(JSON.stringify({ denom: r.denom, rate: r.rate }))}`);
  return tx({ module: 'oracle', sub: 'submit-rates', args: [validator], flags, from });
}
