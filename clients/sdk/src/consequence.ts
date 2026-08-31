/**
 * What signing this will actually do to the account that signs it.
 *
 * `signrequest.ts` answers *what is in these bytes*. That is a different
 * question from *what happens to me if I approve*, and only the second one is
 * what a person is deciding. A screen can list "Transfer — yml1a…4f sent
 * 250 YML to yml1x…9q" perfectly accurately and still leave somebody unable to
 * answer the two questions they actually have: **is that my account, and can I
 * get it back?**
 *
 * So this module reduces a decoded request to a ledger written from the
 * signer's point of view — what leaves, what arrives, what becomes locked and
 * until when — plus the one judgement that decides how loud a screen should be:
 * whether the act can be undone.
 *
 * Three rules.
 *
 * **The signer's address decides the side.** A bank send where the signer is
 * `to_address` is money arriving, and rendering it under "leaves this account"
 * because the message type is a transfer would be a lie with a plausible
 * shape. Where neither side is the signer — a treasury spend, a proposal the
 * signer merely submits — the movement is reported as happening elsewhere, and
 * named as such.
 *
 * **An unrecognised message makes the ledger incomplete, never empty.** A
 * transaction carrying one known transfer and one message this build cannot
 * read must not render as "250 YML leaves, nothing else". `incomplete` is the
 * flag that stops the screen claiming a total.
 *
 * **Reversibility is taken at its worst.** A transaction that submits a
 * proposal *and* sends 250 YML is irreversible, because the 250 is gone
 * whatever the group decides.
 */

import type { Coin } from './denom.ts';
import type { RequestMessage, SigningRequestSummary } from './signrequest.ts';

/**
 * How far a signature can be walked back.
 *
 * Ordered by how much it should worry a reader, worst first — `worst()` below
 * relies on the order, so a new value goes in at its severity rather than at
 * the end.
 */
export const REVERSIBILITY = [
  'irreversible',
  'delayed',
  'revocable',
  'proposal',
  'none',
  'unknown',
] as const;

export type Reversibility = (typeof REVERSIBILITY)[number];

/** One movement of value, from the signer's point of view. */
export interface Movement {
  coins: Coin[];
  /** Where it comes from: an address, or a label like `treasury 1`. */
  from?: string;
  /** Where it goes: an address, or a label. */
  to?: string;
  /**
   * `out` — leaves the signing account.
   * `in` — arrives in the signing account.
   * `elsewhere` — moves between two other parties on this signature's authority.
   */
  side: 'out' | 'in' | 'elsewhere';
}

/** Value that stops being spendable, and the terms on which it comes back. */
export interface LockEffect {
  coins: Coin[];
  /** Who it is committed to, when the message names somebody. */
  beneficiary?: string;
  /** Unix seconds at which it is fully released, when the message states one. */
  releasesAt?: number;
  /** True when whoever created it may still cancel the unreleased part. */
  revocable?: boolean;
  /**
   * Why it is locked, in one clause. Rendered after "becomes locked", so it
   * reads as a sentence and carries no full stop.
   */
  note: string;
  /** True when the release date is not in the message and cannot be stated. */
  releaseUnknown?: boolean;
}

/**
 * A power handed to another account.
 *
 * Distinct from a movement because nothing moves when it is signed and
 * everything can move afterwards, which is exactly the case a signer is worst
 * at judging from a message name.
 */
export interface AuthorityEffect {
  /** Who gains the power. */
  grantee?: string;
  /** What they may then do, in plain words. */
  power: string;
}

export interface SigningConsequences {
  movements: Movement[];
  locks: LockEffect[];
  authority: AuthorityEffect[];
  reversibility: Reversibility;
  /** True when at least one message could not be classified. */
  incomplete: boolean;
}

/** What a `Reversibility` means, as one sentence a signer can act on. */
export const REVERSIBILITY_NOTE: Record<Reversibility, string> = {
  irreversible:
    'Irreversible. Once this is in a block nobody can undo it — not the recipient, not an administrator, not the validators.',
  delayed:
    'Takes effect on a schedule you cannot shorten. Signing starts a clock; the funds are not available again until it runs out.',
  revocable: 'You can undo this yourself later, with a second transaction.',
  proposal:
    'Nothing moves when you sign. This asks other people to decide, and it is their approval that would move anything.',
  none: 'Nothing leaves this account, and nothing becomes locked.',
  unknown:
    'This build cannot say whether it can be undone, because it does not recognise every message in it.',
};

/**
 * A message's effect, keyed by proto type URL.
 *
 * A `null` entry means "recognised, and moves nothing" — which is a different
 * answer from an absent entry, and the difference is the whole point. An absent
 * entry makes the ledger incomplete and the reversibility unknown; a null one
 * lets a screen say plainly that nothing leaves.
 */
type Classifier = (
  msg: Record<string, any>,
  signer: string,
) => { movements?: Movement[]; locks?: LockEffect[]; authority?: AuthorityEffect[]; reversibility: Reversibility };

const coinList = (value: unknown): Coin[] =>
  Array.isArray(value)
    ? value.filter((c): c is Coin => !!c && typeof c === 'object' && 'denom' in c && 'amount' in c)
    : [];

const oneCoin = (value: unknown): Coin[] =>
  value && typeof value === 'object' && 'denom' in (value as Coin) ? [value as Coin] : [];

/**
 * A coin the chain carries as two fields rather than one.
 *
 * Half this chain's own messages hold `amount` and `denom` side by side instead
 * of a `cosmos.base.v1beta1.Coin`, and reading them as a Coin yields
 * `{denom: undefined}` — which renders as a bare number with no currency on the
 * screen where the currency is half the decision.
 */
const splitCoin = (amount: unknown, denom: unknown): Coin[] =>
  typeof amount === 'string' && amount !== '' && typeof denom === 'string' && denom !== ''
    ? [{ denom, amount }]
    : [];

/** Which side of a movement the signer is on. */
function sideOf(from: string | undefined, to: string | undefined, signer: string): Movement['side'] {
  if (signer && from === signer) return 'out';
  if (signer && to === signer) return 'in';
  return 'elsewhere';
}

/** Unix seconds out of a proto int64, which arrives as a string. */
function seconds(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value;
  if (typeof value === 'string' && /^\d+$/.test(value) && value !== '0') return Number(value);
  return undefined;
}

const CLASSIFIERS: Record<string, Classifier> = {
  // ---- money leaving outright -------------------------------------------
  '/cosmos.bank.v1beta1.MsgSend': (m, signer) => ({
    movements: [
      {
        coins: coinList(m.amount),
        from: m.from_address,
        to: m.to_address,
        side: sideOf(m.from_address, m.to_address, signer),
      },
    ],
    reversibility: 'irreversible',
  }),
  '/blockchain.paymsg.v1.MsgSendPayment': (m, signer) => ({
    movements: [
      {
        coins: splitCoin(m.amount, m.denom),
        from: m.debtor,
        to: m.creditor,
        side: sideOf(m.debtor, m.creditor, signer),
      },
    ],
    reversibility: 'irreversible',
  }),
  '/ibc.applications.transfer.v1.MsgTransfer': (m, signer) => ({
    movements: [
      { coins: oneCoin(m.token), from: m.sender, to: m.receiver, side: sideOf(m.sender, m.receiver, signer) },
    ],
    reversibility: 'irreversible',
  }),
  '/blockchain.amm.v1.MsgSwap': (m, signer) => ({
    movements: [
      {
        coins: splitCoin(m.token_in_amount, m.token_in_denom),
        from: m.sender,
        to: `pool ${m.pool_id ?? '?'}`,
        side: sideOf(m.sender, undefined, signer),
      },
    ],
    reversibility: 'irreversible',
  }),
  '/blockchain.stablecoin.v1.MsgBurnCoin': (m, signer) => ({
    movements: [
      { coins: splitCoin(m.amount, m.denom), from: m.issuer, side: sideOf(m.issuer, undefined, signer) },
    ],
    reversibility: 'irreversible',
  }),
  '/cosmos.distribution.v1beta1.MsgFundCommunityPool': (m, signer) => ({
    movements: [
      { coins: coinList(m.amount), from: m.depositor, to: 'the community pool', side: sideOf(m.depositor, undefined, signer) },
    ],
    reversibility: 'irreversible',
  }),

  // ---- money moving out of a shared account ------------------------------
  //
  // The signer is the spender, not the source. Reporting `treasury 3 pays
  // 250 YML` as "leaves this account" would be wrong in the direction that
  // matters: it would make a treasurer think their own balance was at stake.
  '/blockchain.treasury.v1.MsgSpend': (m, signer) => ({
    movements: [
      {
        coins: coinList(m.amount),
        from: `treasury ${m.treasury_id ?? '?'}`,
        to: m.recipient,
        side: sideOf(undefined, m.recipient, signer),
      },
    ],
    reversibility: 'irreversible',
  }),
  '/blockchain.treasury.v1.MsgDeposit': (m, signer) => ({
    movements: [
      {
        coins: coinList(m.amount),
        from: m.depositor,
        to: `treasury ${m.treasury_id ?? '?'}`,
        side: sideOf(m.depositor, undefined, signer),
      },
    ],
    reversibility: 'irreversible',
  }),

  // ---- money that stops being spendable ----------------------------------
  '/blockchain.treasury.v1.MsgCreateLock': (m) => {
    const coins: Coin[] =
      typeof m.amount === 'string' && typeof m.denom === 'string'
        ? [{ denom: m.denom, amount: m.amount }]
        : [];
    const releasesAt = seconds(m.end_time);
    return {
      locks: [
        {
          coins,
          beneficiary: m.beneficiary,
          releasesAt,
          releaseUnknown: releasesAt === undefined,
          revocable: Boolean(m.revocable),
          note:
            m.lock_type === 'LOCK_TYPE_VESTING'
              ? 'committed to a beneficiary and released to them gradually'
              : m.lock_type === 'LOCK_TYPE_CONDITIONAL'
                ? 'held until whoever committed it confirms, or a named moderator decides'
                : 'committed to a beneficiary and released to them in full at the end',
        },
      ],
      // Revocable means the treasury can cancel the *unreleased* part, which is
      // a real way back. Irrevocable is not: the money has left the treasury's
      // reach for good, which is the whole reason to commit it.
      reversibility: m.revocable ? 'revocable' : 'irreversible',
    };
  },
  '/cosmos.staking.v1beta1.MsgDelegate': (m) => ({
    locks: [
      {
        coins: oneCoin(m.amount),
        releaseUnknown: true,
        note: 'bonded to a validator, and released only after you unstake and the unbonding period runs out',
      },
    ],
    reversibility: 'delayed',
  }),
  '/cosmos.staking.v1beta1.MsgUndelegate': (m) => ({
    locks: [
      {
        coins: oneCoin(m.amount),
        releaseUnknown: true,
        note: 'unbonding, and unavailable until the chain’s unbonding period runs out',
      },
    ],
    reversibility: 'delayed',
  }),
  '/cosmos.staking.v1beta1.MsgBeginRedelegate': () => ({ reversibility: 'delayed' }),
  '/cosmos.staking.v1beta1.MsgCancelUnbondingDelegation': () => ({ reversibility: 'delayed' }),
  '/cosmos.gov.v1.MsgDeposit': (m, signer) => ({
    movements: [
      {
        coins: coinList(m.amount),
        from: m.depositor,
        to: `proposal ${m.proposal_id ?? '?'}`,
        side: sideOf(m.depositor, undefined, signer),
      },
    ],
    // A deposit comes back if the proposal is not vetoed, and is burned if it
    // is. "Delayed" rather than "irreversible" because the ordinary outcome is
    // that it returns.
    reversibility: 'delayed',
  }),

  // ---- money arriving -----------------------------------------------------
  '/blockchain.treasury.v1.MsgClaimLock': () => ({ reversibility: 'none' }),
  '/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward': () => ({ reversibility: 'none' }),
  '/cosmos.distribution.v1beta1.MsgWithdrawValidatorCommission': () => ({ reversibility: 'none' }),
  '/blockchain.stablecoin.v1.MsgMintCoin': (m, signer) => ({
    movements: [
      {
        coins: splitCoin(m.amount, m.denom),
        to: m.recipient ?? m.issuer,
        side: sideOf(undefined, m.recipient ?? m.issuer, signer),
      },
    ],
    reversibility: 'irreversible',
  }),

  // ---- powers handed to somebody else ------------------------------------
  '/cosmos.authz.v1beta1.MsgGrant': (m) => ({
    authority: [
      {
        grantee: m.grantee,
        power:
          'send transactions from this account, within the grant’s limits, without asking again',
      },
    ],
    reversibility: 'revocable',
  }),
  '/cosmos.feegrant.v1beta1.MsgGrantAllowance': (m) => ({
    authority: [{ grantee: m.grantee, power: 'have this account pay their network fees' }],
    reversibility: 'revocable',
  }),
  '/blockchain.treasury.v1.MsgAssignRole': (m) => ({
    authority: [
      { grantee: m.address, power: `hold the ${String(m.role ?? 'role').replace(/^ROLE_/, '').toLowerCase()} role in treasury ${m.treasury_id ?? '?'}` },
    ],
    reversibility: 'revocable',
  }),
  '/blockchain.treasury.v1.MsgSetAdmin': (m) => ({
    authority: [{ grantee: m.new_admin, power: `administer treasury ${m.treasury_id ?? '?'}` }],
    // Handing the admin role away is only revocable by the new admin, which is
    // to say not by the signer.
    reversibility: 'irreversible',
  }),
  '/cosmos.authz.v1beta1.MsgRevoke': () => ({ reversibility: 'none' }),
  '/cosmos.feegrant.v1beta1.MsgRevokeAllowance': () => ({ reversibility: 'none' }),
  '/blockchain.treasury.v1.MsgRevokeRole': () => ({ reversibility: 'none' }),
  '/cosmos.distribution.v1beta1.MsgSetWithdrawAddress': (m) => ({
    authority: [{ grantee: m.withdraw_address, power: 'receive this account’s staking rewards' }],
    reversibility: 'revocable',
  }),

  // ---- asking, rather than doing -----------------------------------------
  '/cosmos.gov.v1.MsgSubmitProposal': () => ({ reversibility: 'proposal' }),
  '/cosmos.group.v1.MsgSubmitProposal': () => ({ reversibility: 'proposal' }),
  '/cosmos.gov.v1.MsgVote': () => ({ reversibility: 'proposal' }),
  '/cosmos.gov.v1.MsgVoteWeighted': () => ({ reversibility: 'proposal' }),
  '/cosmos.group.v1.MsgVote': () => ({ reversibility: 'proposal' }),
  '/blockchain.enforcement.v1.MsgVoteCase': () => ({ reversibility: 'proposal' }),
  '/blockchain.enforcement.v1.MsgOpenCase': () => ({ reversibility: 'proposal' }),
  '/blockchain.validatorgov.v1.MsgApplyValidator': () => ({ reversibility: 'proposal' }),
  '/blockchain.oracle.v1.MsgApplyAppraiser': () => ({ reversibility: 'proposal' }),
  '/blockchain.paymsg.v1.MsgApplyParticipant': () => ({ reversibility: 'proposal' }),

  // ---- record-keeping that moves nothing ---------------------------------
  '/blockchain.treasury.v1.MsgCreateTreasury': () => ({ reversibility: 'none' }),
  '/blockchain.treasury.v1.MsgSetSpendPolicy': () => ({ reversibility: 'revocable' }),
  '/blockchain.treasury.v1.MsgSetPaused': () => ({ reversibility: 'revocable' }),
  '/blockchain.treasury.v1.MsgRevokeLock': () => ({ reversibility: 'irreversible' }),
  '/blockchain.paymsg.v1.MsgRegisterCustomer': () => ({ reversibility: 'revocable' }),
  '/cosmos.staking.v1beta1.MsgEditValidator': () => ({ reversibility: 'revocable' }),
  '/blockchain.oracle.v1.MsgSubmitExchangeRates': () => ({ reversibility: 'none' }),
  '/blockchain.oracle.v1.MsgDelegateFeeder': () => ({ reversibility: 'revocable' }),
};

/**
 * The worst reversibility in a set.
 *
 * "Worst" is first in `REVERSIBILITY`, so an irreversible message inside a
 * transaction full of proposals still makes the whole thing irreversible. The
 * alternative — reporting the most common one, or the outer message's — is how
 * a screen ends up calling a transfer wrapped in a proposal "nothing moves when
 * you sign".
 */
function worst(values: Reversibility[]): Reversibility {
  let best = REVERSIBILITY.length - 1;
  for (const value of values) {
    const rank = REVERSIBILITY.indexOf(value);
    if (rank >= 0 && rank < best) best = rank;
  }
  return REVERSIBILITY[best]!;
}

/**
 * Walks one message and everything nested inside it.
 *
 * A group or governance proposal's payload is followed, because "requested
 * approval for an action" says nothing about whether the action pays somebody.
 * The *reversibility* of a nested message is deliberately not inherited: the
 * signer of a proposal is not the party who will move the money, and calling a
 * proposal irreversible because it contains a transfer would put the loudest
 * warning in the product on the safest act in it.
 */
function walk(
  message: RequestMessage,
  signer: string,
  out: { movements: Movement[]; locks: LockEffect[]; authority: AuthorityEffect[]; ranks: Reversibility[] },
  nested: boolean,
) {
  if (message.problem) {
    out.ranks.push('unknown');
    return;
  }

  // Only the outermost layer's effects are the signer's own. Inside a proposal,
  // a transfer is something a group would do later on its own authority, so it
  // belongs in the description of what is being proposed and not in a ledger
  // headed "leaves this account". A nested type with no classifier is therefore
  // not a gap either — nothing was going to be read off it. A nested message
  // that could not be *decoded* is still a gap, which is why the check above
  // this one runs at every depth.
  if (!nested) {
    const classify = CLASSIFIERS[message.typeUrl];
    if (!classify) {
      out.ranks.push('unknown');
    } else {
      const effect = classify(message.decoded?.raw ?? {}, signer);
      out.movements.push(...(effect.movements ?? []));
      out.locks.push(...(effect.locks ?? []));
      out.authority.push(...(effect.authority ?? []));
      out.ranks.push(effect.reversibility);
    }
  }

  for (const inner of message.contains) walk(inner, signer, out, true);
}

/**
 * The ledger for a whole signing request.
 *
 * `signer` decides which side of each movement is "this account". Passing an
 * empty string is legitimate — the caller may not know yet — and produces a
 * ledger where every movement reads as happening elsewhere, which is the honest
 * answer rather than a guessed one.
 */
export function consequencesOf(
  summary: SigningRequestSummary,
  signer: string,
): SigningConsequences {
  if (summary.undecodable) {
    return { movements: [], locks: [], authority: [], reversibility: 'unknown', incomplete: true };
  }

  const out = { movements: [] as Movement[], locks: [] as LockEffect[], authority: [] as AuthorityEffect[], ranks: [] as Reversibility[] };
  for (const message of summary.messages) walk(message, signer, out, false);

  return {
    movements: out.movements.filter((m) => m.coins.length > 0),
    locks: out.locks,
    authority: out.authority,
    reversibility: worst(out.ranks),
    incomplete: summary.incomplete || out.ranks.includes('unknown'),
  };
}

/**
 * Adds coins of the same denom together.
 *
 * BigInt throughout, and by denom rather than across them: 250 YML and 40 USD
 * do not have a sum, and a screen that printed one would be inventing an
 * exchange rate at the moment somebody is deciding whether to part with money.
 */
export function totalCoins(coins: Coin[]): Coin[] {
  const byDenom = new Map<string, bigint>();
  for (const coin of coins) {
    let value: bigint;
    try {
      value = BigInt(coin.amount);
    } catch {
      continue;
    }
    byDenom.set(coin.denom, (byDenom.get(coin.denom) ?? 0n) + value);
  }
  return [...byDenom.entries()]
    .filter(([, amount]) => amount !== 0n)
    .map(([denom, amount]) => ({ denom, amount: amount.toString() }));
}

/** Everything leaving the signing account, summed by denom. */
export function leaving(c: SigningConsequences): Coin[] {
  return totalCoins(c.movements.filter((m) => m.side === 'out').flatMap((m) => m.coins));
}

/** Everything arriving in the signing account, summed by denom. */
export function arriving(c: SigningConsequences): Coin[] {
  return totalCoins(c.movements.filter((m) => m.side === 'in').flatMap((m) => m.coins));
}

/** Everything becoming locked, summed by denom. */
export function locking(c: SigningConsequences): Coin[] {
  return totalCoins(c.locks.flatMap((l) => l.coins));
}
