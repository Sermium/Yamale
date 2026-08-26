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
    // Not a chain error at all: the request never reached a node. It arrives
    // here anyway because every interface funnels its failures through one
    // translator, and `fetch` rejecting with the word "Failed" is the single
    // most common thing a reader of these screens will ever see. Left
    // untranslated it reads as the chain refusing them, which is the opposite
    // of what happened — and it matters, because a treasury that cannot be read
    // is not a treasury that is empty.
    match: /failed to fetch|networkerror|network request failed|load failed|err_(?:connection|network|name_not_resolved)/i,
    translate: () => ({
      message: 'The node could not be reached',
      reason:
        'The request never arrived. That is a fault between this device and the node — nothing on the chain has changed, and nothing shown as missing is known to be missing.',
      nextStep: 'Check the connection and try again.',
      retryable: true,
    }),
  },
  {
    // The client throws `Request failed with 503` out of its own `get`. The
    // number is meaningless to a reader and the distinction between "the node
    // is not answering" and "the node answered, and said no" is the whole of
    // what they need.
    match: /request failed with (5\d\d)|\b(?:502|503|504) (?:bad gateway|service unavailable|gateway time-?out)/i,
    translate: (m) => ({
      message: 'The node is not answering',
      reason: `It replied ${m[1] ?? 'with a server error'} rather than with data. The chain itself may be perfectly healthy; this particular node is not serving.`,
      nextStep: 'Try again shortly, or point this interface at another node.',
      retryable: true,
    }),
  },
  {
    // Deny-by-default on /api/rest returns 401 with an auth challenge, which a
    // browser turns into a login box. See the note in the ops docs: the fix is
    // never a password, it is a path the gateway was not told to allow.
    match: /request failed with 40[13]|unauthori[sz]ed request|forbidden/i,
    translate: () => ({
      message: 'This node refuses that query',
      reason:
        'The node answered, but its gateway does not expose this endpoint to the public. It is a configuration decision, not a fault in the data.',
      nextStep: 'Ask whoever runs the node to allow this query, or use a node that serves it.',
      retryable: false,
    }),
  },
  {
    // The node answered and said there is no such thing. Distinct from every
    // other failure above: this one is an *answer*, and a screen may safely
    // render "there is none" rather than "unknown".
    match: /request failed with 404/i,
    translate: () => ({
      message: 'There is no such record',
      reason: 'The node answered, and it holds nothing under that identifier.',
      nextStep: 'Check the number or the address.',
      retryable: false,
    }),
  },
  {
    match: /aborted|timeout|timed out/i,
    translate: () => ({
      message: 'The node took too long',
      reason: 'The request was given up on before an answer came back.',
      nextStep: 'Try again.',
      retryable: true,
    }),
  },
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
    // The state a brand-new account is in before anything has ever been sent to
    // it. The chain has no record of it, so it has no sequence number, so
    // nothing can be signed from it — and CosmJS reports that by *throwing*
    // rather than returning a result, which means it reaches an interface
    // through the catch path where the raw text is all there is. Left
    // untranslated it reads as a fault in the app: "Account 'yml1…' does not
    // exist on chain" tells somebody their account is missing, when what is
    // missing is the first coin.
    match: /does not exist on chain|account .* not found: key not found/i,
    translate: () => ({
      message: 'This account has never received anything',
      reason:
        'The chain has no record of it yet. An account comes into existence when money first arrives, so until then it cannot send.',
      nextStep: 'Have somebody send it any amount, or claim test funds, and then try again.',
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
