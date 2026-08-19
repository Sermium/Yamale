/**
 * Translating chain errors into something a person can act on.
 *
 * Chain errors are written for node operators: they name a module, a registered
 * error code, and a Go source location. None of that tells somebody what to do
 * next. Every error a user can actually hit gets three things here — what
 * happened, why, and the single next action — with the raw text preserved for
 * support and debugging.
 */

export interface TranslatedError {
  /** One line, in plain language. */
  message: string;
  /** Why it happened, when that is not obvious from the message. */
  reason?: string;
  /** The one thing to do next. */
  nextStep?: string;
  /** The original error text, always kept. */
  raw: string;
  /** Whether retrying the same transaction unchanged could ever succeed. */
  retryable: boolean;
}

interface Rule {
  match: RegExp;
  translate: (m: RegExpExecArray, raw: string) => Omit<TranslatedError, 'raw'>;
}

/**
 * Ordered most specific first: the first match wins, so a rule that can extract
 * real numbers out of the message must come before the generic version of the
 * same error.
 */
const RULES: Rule[] = [
  {
    // Fee grants are how an institution pays the network fee for its customers,
    // so a customer holding only their own currency can still transact. When
    // the grant is missing or spent, the chain's own message is unusable: it
    // renders the grantee's address as raw bytes, so somebody reading it cannot
    // tell which account it refers to.
    match: /fee-grant not found|allow to pay fees for/i,
    translate: () => ({
      message: 'No fee allowance',
      reason:
        'This transaction asked another account to pay its network fee, but that account has not granted an allowance — or the allowance has run out or expired.',
      nextStep:
        'Ask the sponsoring institution to grant an allowance, or pay the fee from this account directly.',
      retryable: false,
    }),
  },
  {
    // Distinct from ordinary insufficient funds: the account may hold plenty of
    // one currency and still be unable to move it, because fees are payable in
    // the native token. Telling somebody looking at a healthy naira balance
    // that they have no funds is the kind of message that costs a support call.
    match: /spendable balance 0uyml is smaller than ([^\s:]+)/i,
    translate: (m) => ({
      message: 'No YML for the network fee',
      reason: `This account holds no YML, and the network fee is ${m[1]}. Its balances in other currencies cannot pay it.`,
      nextStep:
        'Have the account sponsored with a fee allowance, or send it a small amount of YML.',
      retryable: false,
    }),
  },
  {
    // The SDK embeds the actual figures, which turns a useless error into a
    // useful one — but only if we pull them out.
    match: /insufficient funds: insufficient account funds; (\S+) is smaller than (\S+)/i,
    translate: (m) => ({
      message: 'Not enough funds',
      reason: `This account holds ${m[1]} but the transaction needs ${m[2]}, including the network fee.`,
      nextStep: 'Reduce the amount, or top up the account.',
      retryable: false,
    }),
  },
  {
    match: /insufficient funds/i,
    translate: () => ({
      message: 'Not enough funds',
      reason: 'The account does not hold enough to cover the amount and the network fee.',
      nextStep: 'Reduce the amount, or top up the account.',
      retryable: false,
    }),
  },
  {
    match: /treasury \d+ has (\S+) available.*needs (\S+)/i,
    translate: (m) => ({
      message: 'Not enough available in the treasury',
      reason: `${m[1]} is available to spend and ${m[2]} was requested. Funds committed to a vesting or lock schedule cannot be spent by anyone.`,
      nextStep: 'Spend a smaller amount, or cancel a revocable commitment to free funds.',
      retryable: false,
    }),
  },
  {
    match: /spend exceeds the policy limit/i,
    translate: () => ({
      message: 'Over the spending limit',
      reason: 'This treasury has a limit on how much may be paid out per transaction or per period.',
      nextStep: 'Wait for the period to reset, split the payment, or ask an administrator to raise the limit.',
      retryable: true,
    }),
  },
  {
    match: /destination is not permitted|not on .* allowlist|on .* blocklist/i,
    translate: () => ({
      message: 'Recipient not permitted',
      reason: "This treasury only pays out to approved destinations, and this one is not on the list.",
      nextStep: 'Ask an administrator to add the recipient to the allowlist.',
      retryable: false,
    }),
  },
  {
    match: /treasury is paused/i,
    translate: () => ({
      message: 'This treasury is frozen',
      reason: 'Someone with emergency access has paused it, so no funds can move in or out.',
      nextStep: 'Contact a treasury administrator.',
      retryable: true,
    }),
  },
  {
    match: /nothing has vested yet|has nothing available at this time/i,
    translate: () => ({
      message: 'Nothing to claim yet',
      reason: 'None of this commitment has become available under its schedule.',
      nextStep: 'Check the schedule for when the next portion unlocks.',
      retryable: true,
    }),
  },
  {
    match: /is not the approved issuer|has no approved issuer/i,
    translate: () => ({
      message: 'Not authorised to issue this currency',
      reason: 'Only the issuer that governance approved for this currency may create or redeem it.',
      nextStep: 'Use the approved issuer account, or apply for issuing rights.',
      retryable: false,
    }),
  },
  {
    match: /is not approved by governance; submit a MsgApplyValidator/i,
    translate: () => ({
      message: 'This validator has not been approved',
      reason: 'The chain only accepts validators that governance has voted to admit.',
      nextStep: 'Submit a validator application and wait for the vote.',
      retryable: false,
    }),
  },
  {
    match: /is not approved|not an approved participant/i,
    translate: () => ({
      message: 'Not an approved participant',
      reason: 'Payments must be routed through institutions that governance has approved.',
      nextStep: 'Use an approved institution, or apply for approval.',
      retryable: false,
    }),
  },
  {
    match: /a payment with .*end-to-end id .* already exists|already exists/i,
    translate: () => ({
      message: 'This has already been recorded',
      reason: 'A payment with the same reference has already been settled. References are unique so a payment cannot be made twice by accident.',
      nextStep: 'Use a new reference if this is genuinely a separate payment.',
      retryable: false,
    }),
  },
  {
    match: /swap output is below the minimum requested|slippage/i,
    translate: () => ({
      message: 'The price moved',
      reason: 'The trade would return less than the minimum you accepted, so it was cancelled rather than filled at a worse rate.',
      nextStep: 'Try again at the current price, or allow more slippage.',
      retryable: true,
    }),
  },
  {
    match: /account sequence mismatch, expected (\d+), got (\d+)/i,
    translate: (m) => ({
      message: 'Transactions arrived out of order',
      reason: `The chain expected transaction number ${m[1]} but received ${m[2]}. This usually means an earlier transaction is still pending.`,
      nextStep: 'Wait a moment and try again.',
      retryable: true,
    }),
  },
  {
    match: /out of gas|gas limit|insufficient fee/i,
    translate: () => ({
      message: 'The network fee was too low',
      reason: 'This transaction needed more work than the fee covered.',
      nextStep: 'Try again — the fee will be re-estimated.',
      retryable: true,
    }),
  },
  {
    match: /tx already exists in cache|tx already in mempool/i,
    translate: () => ({
      message: 'Already submitted',
      reason: 'This exact transaction is already waiting to be included.',
      nextStep: 'Wait for it to confirm rather than sending it again.',
      retryable: false,
    }),
  },
  {
    match: /unauthorized|is not authorized|signer is not/i,
    translate: () => ({
      message: 'Not allowed',
      reason: 'This account does not have permission to do that.',
      nextStep: 'Use an account with the right role, or ask an administrator to grant it.',
      retryable: false,
    }),
  },
  {
    match: /not found/i,
    translate: () => ({
      message: 'Not found',
      reason: 'The thing this transaction refers to does not exist.',
      nextStep: 'Check the reference and try again.',
      retryable: false,
    }),
  },
];

/**
 * Translates a raw chain error.
 *
 * Anything unmatched is surfaced verbatim rather than replaced with a vague
 * apology: an unfamiliar error the user can search for and quote to support is
 * more useful than "something went wrong", which destroys the only information
 * they had.
 */
export function translateError(raw: string | undefined | null): TranslatedError {
  const text = (raw ?? '').trim();
  if (!text) {
    return { message: 'Something went wrong', raw: '', retryable: true };
  }

  for (const rule of RULES) {
    const match = rule.match.exec(text);
    if (match) {
      return { ...rule.translate(match, text), raw: text };
    }
  }

  return {
    message: 'The transaction was rejected',
    reason: text,
    nextStep: 'If this keeps happening, quote the details below to support.',
    raw: text,
    retryable: false,
  };
}

/**
 * Describes a transaction result code.
 *
 * Code 0 is the only success. Anything else carries a `raw_log` worth
 * translating.
 */
export function describeTxResult(code: number, rawLog?: string): { ok: boolean; error?: TranslatedError } {
  if (code === 0) return { ok: true };
  return { ok: false, error: translateError(rawLog) };
}
