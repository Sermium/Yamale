/**
 * Turning chain messages into sentences.
 *
 * This is the piece that decides whether an explorer is usable. A transaction
 * is a list of protobuf messages; rendering them as JSON is not an
 * interpretation, it is a refusal to make one. Every message type this chain can
 * carry gets a verb, a sentence, and a judgement about whether an ordinary
 * person needs to see it at all.
 *
 * The `everyday` flag drives the simple view. It marks the handful of message
 * types that correspond to something a non-technical person would recognise as
 * an event in their financial life — money arriving, money leaving, a payment
 * settling. Everything else is real and visible in the expert view, but showing
 * a `MsgUpdateParams` to somebody checking whether their rent arrived is noise.
 */

import { formatAmount, formatCoins, parseCoin, type Coin, type DenomInfo } from './denom.ts';
import { truncateAddress } from './format.ts';

export type MessageKind =
  | 'transfer'
  | 'payment'
  | 'staking'
  | 'governance'
  | 'trade'
  | 'treasury'
  | 'issuance'
  | 'admin'
  | 'other';

export interface DecodedDetail {
  label: string;
  value: string;
  /** Marks a value as an address, so the UI can link and truncate it. */
  address?: boolean;
}

export interface DecodedMessage {
  typeUrl: string;
  kind: MessageKind;
  /** Short verb for a badge or list row: "Sent", "Staked", "Swapped". */
  title: string;
  /** One sentence describing what happened. */
  summary: string;
  /**
   * Whether the simple view should show this message. False for governance
   * plumbing, parameter changes, and administrative messages that only mean
   * something if you already know the chain's internals.
   */
  everyday: boolean;
  /** The account that initiated it, when identifiable. */
  actor?: string;
  /** The account on the other side, when there is one. */
  counterparty?: string;
  /** Value moved by this message, when any. */
  coins?: Coin[];
  /** Labelled specifics, shown when a row is expanded. */
  details?: DecodedDetail[];
  /** The original message, always reachable. */
  raw: Record<string, unknown>;
}

export interface DecodeContext {
  /** Denom metadata resolved from the chain, layered over the built-in set. */
  registry?: Record<string, DenomInfo>;
  /** Known address → name, e.g. validator monikers or a local address book. */
  names?: Record<string, string>;
}

/** Renders an address as a name when one is known, otherwise truncated. */
function who(address: string | undefined, ctx: DecodeContext): string {
  if (!address) return 'someone';
  return ctx.names?.[address] ?? truncateAddress(address);
}

function coinsOf(value: unknown): Coin[] {
  if (!Array.isArray(value)) return [];
  return value.filter((c): c is Coin => !!c && typeof c === 'object' && 'denom' in c && 'amount' in c);
}

function money(coins: Coin[], ctx: DecodeContext): string {
  return formatCoins(coins, { registry: ctx.registry });
}

function amountOf(amount: unknown, denom: unknown, ctx: DecodeContext): string {
  if (typeof amount !== 'string' || typeof denom !== 'string') return 'an amount';
  return formatAmount(amount, denom, { registry: ctx.registry });
}

type Decoder = (msg: Record<string, any>, ctx: DecodeContext) => Omit<DecodedMessage, 'typeUrl' | 'raw'>;

/**
 * Per-message decoders, keyed by proto type URL.
 *
 * Adding a message type to the chain means adding a line here. A missing entry
 * degrades gracefully — see `fallbackDecode` — but it degrades to something
 * nobody wants to read, so it is worth keeping current.
 */
const DECODERS: Record<string, Decoder> = {
  // ---- bank -------------------------------------------------------------
  '/cosmos.bank.v1beta1.MsgSend': (m, ctx) => {
    const coins = coinsOf(m.amount);
    return {
      kind: 'transfer',
      title: 'Transfer',
      summary: `${who(m.from_address, ctx)} sent ${money(coins, ctx)} to ${who(m.to_address, ctx)}`,
      everyday: true,
      actor: m.from_address,
      counterparty: m.to_address,
      coins,
    };
  },
  '/cosmos.bank.v1beta1.MsgMultiSend': (m, ctx) => {
    const outputs = Array.isArray(m.outputs) ? m.outputs : [];
    return {
      kind: 'transfer',
      title: 'Batch transfer',
      summary: `A batch payment to ${outputs.length} recipient${outputs.length === 1 ? '' : 's'}`,
      everyday: true,
      actor: Array.isArray(m.inputs) && m.inputs[0] ? m.inputs[0].address : undefined,
      coins: Array.isArray(m.inputs) && m.inputs[0] ? coinsOf(m.inputs[0].coins) : [],
    };
  },

  // ---- payments ---------------------------------------------------------
  '/blockchain.paymsg.v1.MsgSendPayment': (m, ctx) => ({
    kind: 'payment',
    title: 'Payment',
    summary: `${who(m.debtor, ctx)} paid ${amountOf(m.amount, m.denom, ctx)} to ${who(m.creditor, ctx)}${
      m.remittance_information ? ` for "${m.remittance_information}"` : ''
    }`,
    everyday: true,
    actor: m.debtor,
    counterparty: m.creditor,
    coins: typeof m.amount === 'string' && typeof m.denom === 'string' ? [{ denom: m.denom, amount: m.amount }] : [],
    details: [
      { label: 'Reference', value: m.end_to_end_id ?? '—' },
      // A payment whose detail is held off-chain must say so. Showing the
      // usual "—" would read as "no purpose was given", and somebody
      // reconciling would go looking for a field that is not missing but
      // deliberately elsewhere.
      hasMetadataHash(m)
        ? { label: 'Purpose', value: 'Held off-chain' }
        : { label: 'Purpose', value: purposeCode(m.purpose_code) },
      { label: 'Sending institution', value: m.instructing_participant ?? '—', address: true },
      { label: 'Receiving institution', value: m.instructed_participant ?? '—', address: true },
      ...(m.settlement_jurisdiction
        ? [{ label: 'Settles in', value: String(m.settlement_jurisdiction) }]
        : []),
    ],
  }),
  '/blockchain.paymsg.v1.MsgApplyParticipant': (m, ctx) => ({
    kind: 'admin',
    title: 'Institution application',
    summary: `${who(m.creator, ctx)} applied to become a payment institution${m.name ? ` as "${m.name}"` : ''}`,
    everyday: false,
    actor: m.creator,
  }),
  '/blockchain.paymsg.v1.MsgRegisterCustomer': (m, ctx) => ({
    kind: 'admin',
    title: m.registered ? 'Customer registered' : 'Customer removed',
    summary: m.registered
      ? `${who(m.participant, ctx)} recorded ${who(m.customer, ctx)} as one of its customers, which lets their payments name it as the instructing institution`
      : `${who(m.participant, ctx)} ended its relationship with ${who(m.customer, ctx)}`,
    everyday: false,
    actor: m.participant,
    counterparty: m.customer,
  }),
  '/blockchain.paymsg.v1.MsgApproveParticipant': (m, ctx) => ({
    kind: 'governance',
    title: 'Institution decision',
    summary: `Governance ${m.approve ? 'approved' : 'rejected'} ${who(m.participant, ctx)} as a payment institution`,
    everyday: false,
    counterparty: m.participant,
  }),

  // ---- staking ----------------------------------------------------------
  '/cosmos.staking.v1beta1.MsgDelegate': (m, ctx) => {
    const coins = m.amount ? [m.amount as Coin] : [];
    return {
      kind: 'staking',
      title: 'Stake',
      summary: `${who(m.delegator_address, ctx)} staked ${money(coins, ctx)} with ${who(m.validator_address, ctx)}`,
      everyday: true,
      actor: m.delegator_address,
      counterparty: m.validator_address,
      coins,
    };
  },
  '/cosmos.staking.v1beta1.MsgUndelegate': (m, ctx) => {
    const coins = m.amount ? [m.amount as Coin] : [];
    return {
      kind: 'staking',
      title: 'Unstake',
      summary: `${who(m.delegator_address, ctx)} began unstaking ${money(coins, ctx)} from ${who(m.validator_address, ctx)}`,
      everyday: true,
      actor: m.delegator_address,
      counterparty: m.validator_address,
      coins,
      details: [{ label: 'Note', value: 'Unstaked funds are released after the unbonding period.' }],
    };
  },
  '/cosmos.staking.v1beta1.MsgBeginRedelegate': (m, ctx) => {
    const coins = m.amount ? [m.amount as Coin] : [];
    return {
      kind: 'staking',
      title: 'Move stake',
      summary: `${who(m.delegator_address, ctx)} moved ${money(coins, ctx)} from ${who(
        m.validator_src_address,
        ctx,
      )} to ${who(m.validator_dst_address, ctx)}`,
      everyday: true,
      actor: m.delegator_address,
      counterparty: m.validator_dst_address,
      coins,
    };
  },
  '/cosmos.staking.v1beta1.MsgCreateValidator': (m, ctx) => ({
    kind: 'staking',
    title: 'New validator',
    summary: `${m.description?.moniker ? `"${m.description.moniker}"` : who(m.validator_address, ctx)} joined as a validator`,
    everyday: false,
    actor: m.validator_address,
    coins: m.value ? [m.value as Coin] : [],
  }),
  '/cosmos.staking.v1beta1.MsgEditValidator': (m, ctx) => ({
    kind: 'staking',
    title: 'Validator update',
    summary: `${who(m.validator_address, ctx)} updated their validator details`,
    everyday: false,
    actor: m.validator_address,
  }),
  '/cosmos.staking.v1beta1.MsgCancelUnbondingDelegation': (m, ctx) => ({
    kind: 'staking',
    title: 'Cancel unstake',
    summary: `${who(m.delegator_address, ctx)} cancelled an unstaking request`,
    everyday: true,
    actor: m.delegator_address,
    coins: m.amount ? [m.amount as Coin] : [],
  }),

  // ---- rewards ----------------------------------------------------------
  '/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward': (m, ctx) => ({
    kind: 'staking',
    title: 'Collect rewards',
    summary: `${who(m.delegator_address, ctx)} collected staking rewards from ${who(m.validator_address, ctx)}`,
    everyday: true,
    actor: m.delegator_address,
    counterparty: m.validator_address,
  }),
  '/cosmos.distribution.v1beta1.MsgWithdrawValidatorCommission': (m, ctx) => ({
    kind: 'staking',
    title: 'Collect commission',
    summary: `${who(m.validator_address, ctx)} collected their validator commission`,
    everyday: false,
    actor: m.validator_address,
  }),
  '/cosmos.distribution.v1beta1.MsgSetWithdrawAddress': (m, ctx) => ({
    kind: 'admin',
    title: 'Reward address',
    summary: `${who(m.delegator_address, ctx)} set rewards to be paid to ${who(m.withdraw_address, ctx)}`,
    everyday: false,
    actor: m.delegator_address,
  }),
  '/cosmos.distribution.v1beta1.MsgFundCommunityPool': (m, ctx) => ({
    kind: 'governance',
    title: 'Community funding',
    summary: `${who(m.depositor, ctx)} contributed ${money(coinsOf(m.amount), ctx)} to the community pool`,
    everyday: false,
    actor: m.depositor,
    coins: coinsOf(m.amount),
  }),

  // ---- governance -------------------------------------------------------
  '/cosmos.gov.v1.MsgSubmitProposal': (m, ctx) => ({
    kind: 'governance',
    title: 'New proposal',
    summary: `${who(m.proposer, ctx)} submitted a governance proposal${m.title ? `: "${m.title}"` : ''}`,
    everyday: false,
    actor: m.proposer,
    coins: coinsOf(m.initial_deposit),
  }),
  '/cosmos.gov.v1.MsgVote': (m, ctx) => ({
    kind: 'governance',
    title: 'Vote',
    summary: `${who(m.voter, ctx)} voted ${voteOption(m.option)} on proposal ${m.proposal_id}`,
    everyday: false,
    actor: m.voter,
  }),
  '/cosmos.gov.v1.MsgVoteWeighted': (m, ctx) => ({
    kind: 'governance',
    title: 'Split vote',
    summary: `${who(m.voter, ctx)} cast a split vote on proposal ${m.proposal_id}`,
    everyday: false,
    actor: m.voter,
  }),
  '/cosmos.gov.v1.MsgDeposit': (m, ctx) => ({
    kind: 'governance',
    title: 'Proposal deposit',
    summary: `${who(m.depositor, ctx)} backed proposal ${m.proposal_id} with ${money(coinsOf(m.amount), ctx)}`,
    everyday: false,
    actor: m.depositor,
    coins: coinsOf(m.amount),
  }),

  // ---- trading ----------------------------------------------------------
  '/blockchain.amm.v1.MsgSwap': (m, ctx) => ({
    kind: 'trade',
    title: 'Swap',
    summary: `${who(m.sender, ctx)} swapped ${amountOf(m.token_in_amount, m.token_in_denom, ctx)} for ${
      resolveSymbol(m.token_out_denom, ctx)
    }`,
    everyday: true,
    actor: m.sender,
    coins: typeof m.token_in_amount === 'string' ? [{ denom: m.token_in_denom, amount: m.token_in_amount }] : [],
    details: [
      { label: 'Pool', value: `#${m.pool_id}` },
      { label: 'Minimum accepted', value: amountOf(m.min_amount_out, m.token_out_denom, ctx) },
    ],
  }),
  '/blockchain.amm.v1.MsgCreatePool': (m, ctx) => ({
    kind: 'trade',
    title: 'New pool',
    summary: `${who(m.creator, ctx)} opened a ${resolveSymbol(m.denom_a, ctx)}/${resolveSymbol(
      m.denom_b,
      ctx,
    )} trading pool`,
    everyday: false,
    actor: m.creator,
    details: [
      { label: 'Initial liquidity', value: `${amountOf(m.amount_a, m.denom_a, ctx)} and ${amountOf(m.amount_b, m.denom_b, ctx)}` },
      { label: 'Trading fee', value: `${(Number(m.swap_fee_bps ?? 0) / 100).toFixed(2)}%` },
    ],
  }),
  '/blockchain.amm.v1.MsgJoinPool': (m, ctx) => ({
    kind: 'trade',
    title: 'Add liquidity',
    summary: `${who(m.sender, ctx)} added liquidity to pool #${m.pool_id}`,
    everyday: false,
    actor: m.sender,
  }),
  '/blockchain.amm.v1.MsgExitPool': (m, ctx) => ({
    kind: 'trade',
    title: 'Remove liquidity',
    summary: `${who(m.sender, ctx)} withdrew liquidity from pool #${m.pool_id}`,
    everyday: false,
    actor: m.sender,
  }),

  // ---- stablecoin issuance ---------------------------------------------
  '/blockchain.stablecoin.v1.MsgMintCoin': (m, ctx) => ({
    kind: 'issuance',
    title: 'Issued',
    summary: `${who(m.issuer, ctx)} issued ${amountOf(m.amount, m.denom, ctx)} to ${who(m.recipient, ctx)}`,
    everyday: true,
    actor: m.issuer,
    counterparty: m.recipient,
    coins: typeof m.amount === 'string' ? [{ denom: m.denom, amount: m.amount }] : [],
  }),
  '/blockchain.stablecoin.v1.MsgBurnCoin': (m, ctx) => ({
    kind: 'issuance',
    title: 'Redeemed',
    summary: `${who(m.issuer, ctx)} redeemed ${amountOf(m.amount, m.denom, ctx)} out of circulation`,
    everyday: true,
    actor: m.issuer,
    coins: typeof m.amount === 'string' ? [{ denom: m.denom, amount: m.amount }] : [],
  }),
  '/blockchain.stablecoin.v1.MsgRegisterCurrency': (m, ctx) => ({
    kind: 'admin',
    title: 'Currency application',
    summary: `${who(m.creator, ctx)} applied to issue ${m.symbol || m.denom}`,
    everyday: false,
    actor: m.creator,
  }),
  '/blockchain.stablecoin.v1.MsgApproveIssuer': (m) => ({
    kind: 'governance',
    title: 'Issuer decision',
    summary: `Governance ${m.approve ? 'approved' : 'rejected'} an issuer for ${m.denom}`,
    everyday: false,
  }),

  // ---- treasury ---------------------------------------------------------
  '/blockchain.treasury.v1.MsgCreateTreasury': (m, ctx) => ({
    kind: 'treasury',
    title: 'New treasury',
    summary: `${who(m.creator, ctx)} opened the treasury "${m.name || 'untitled'}"`,
    everyday: false,
    actor: m.creator,
  }),
  '/blockchain.treasury.v1.MsgDeposit': (m, ctx) => ({
    kind: 'treasury',
    title: 'Treasury deposit',
    summary: `${who(m.depositor, ctx)} paid ${money(coinsOf(m.amount), ctx)} into treasury #${m.treasury_id}`,
    everyday: true,
    actor: m.depositor,
    coins: coinsOf(m.amount),
  }),
  '/blockchain.treasury.v1.MsgSpend': (m, ctx) => ({
    kind: 'treasury',
    title: 'Treasury payment',
    summary: `Treasury #${m.treasury_id} paid ${money(coinsOf(m.amount), ctx)} to ${who(m.recipient, ctx)}${
      m.memo ? ` for "${m.memo}"` : ''
    }`,
    everyday: true,
    actor: m.spender,
    counterparty: m.recipient,
    coins: coinsOf(m.amount),
  }),
  '/blockchain.treasury.v1.MsgCreateLock': (m, ctx) => ({
    kind: 'treasury',
    title: 'Funds committed',
    summary: `Treasury #${m.treasury_id} committed ${amountOf(m.amount, m.denom, ctx)} to ${who(
      m.beneficiary,
      ctx,
    )}${m.lock_type === 'LOCK_TYPE_VESTING' ? ', vesting over time' : ', released in full later'}`,
    everyday: true,
    actor: m.admin,
    counterparty: m.beneficiary,
    coins: typeof m.amount === 'string' ? [{ denom: m.denom, amount: m.amount }] : [],
    details: [
      { label: 'Type', value: m.lock_type === 'LOCK_TYPE_VESTING' ? 'Vesting schedule' : 'Time lock' },
      { label: 'Can be cancelled', value: m.revocable ? 'Yes' : 'No' },
    ],
  }),
  '/blockchain.treasury.v1.MsgClaimLock': (m, ctx) => ({
    kind: 'treasury',
    title: 'Claimed',
    summary: `${who(m.beneficiary, ctx)} claimed funds that had become available to them`,
    everyday: true,
    actor: m.beneficiary,
  }),
  '/blockchain.treasury.v1.MsgRevokeLock': (m, ctx) => ({
    kind: 'treasury',
    title: 'Commitment cancelled',
    summary: `A treasury cancelled commitment #${m.lock_id}; anything already earned still goes to the beneficiary`,
    everyday: true,
    actor: m.admin,
  }),
  '/blockchain.treasury.v1.MsgAssignRole': (m, ctx) => ({
    kind: 'admin',
    title: 'Role granted',
    summary: `Treasury #${m.treasury_id} granted ${who(m.address, ctx)} the ${roleName(m.role)} role`,
    everyday: false,
    actor: m.admin,
    counterparty: m.address,
  }),
  '/blockchain.treasury.v1.MsgRevokeRole': (m, ctx) => ({
    kind: 'admin',
    title: 'Role removed',
    summary: `Treasury #${m.treasury_id} removed ${who(m.address, ctx)}'s role`,
    everyday: false,
    actor: m.admin,
  }),
  '/blockchain.treasury.v1.MsgSetSpendPolicy': (m, ctx) => ({
    kind: 'admin',
    title: 'Spending rules',
    summary: `Treasury #${m.policy?.treasury_id} updated its spending limits`,
    everyday: false,
    actor: m.admin,
  }),
  '/blockchain.treasury.v1.MsgSetPaused': (m, ctx) => ({
    kind: 'admin',
    title: m.paused ? 'Treasury frozen' : 'Treasury resumed',
    summary: `${who(m.sender, ctx)} ${m.paused ? 'froze' : 'resumed'} treasury #${m.treasury_id}`,
    everyday: true,
    actor: m.sender,
  }),
  '/blockchain.treasury.v1.MsgSetAdmin': (m, ctx) => ({
    kind: 'admin',
    title: 'Control transferred',
    summary: `Treasury #${m.treasury_id} handed control to ${who(m.new_admin, ctx)}`,
    everyday: false,
    actor: m.admin,
    counterparty: m.new_admin,
  }),

  // ---- validator onboarding --------------------------------------------
  '/blockchain.validatorgov.v1.MsgApplyValidator': (m, ctx) => ({
    kind: 'admin',
    title: 'Validator application',
    summary: `${who(m.creator, ctx)} applied to run a validator${m.moniker ? ` as "${m.moniker}"` : ''}`,
    everyday: false,
    actor: m.creator,
  }),
  '/blockchain.validatorgov.v1.MsgApproveValidator': (m, ctx) => ({
    kind: 'governance',
    title: 'Validator decision',
    summary: `Governance ${m.approve ? 'approved' : 'rejected'} ${who(m.candidate, ctx)} as a validator`,
    everyday: false,
    counterparty: m.candidate,
  }),

  // ---- builder fee share ------------------------------------------------
  '/blockchain.builderfee.v1.MsgRegisterBuilder': (m, ctx) => ({
    kind: 'admin',
    title: 'Builder registration',
    summary: `${who(m.creator, ctx)} registered for a fee share on ${shortTypeUrl(m.msg_type_url)}`,
    everyday: false,
    actor: m.creator,
  }),
  '/blockchain.builderfee.v1.MsgApproveBuilder': (m) => ({
    kind: 'governance',
    title: 'Builder decision',
    summary: `Governance ${m.approve ? 'approved' : 'rejected'} a fee share for ${shortTypeUrl(m.msg_type_url)}`,
    everyday: false,
  }),

  // ---- prices and valuations --------------------------------------------
  //
  // Rate submissions are marked as not everyday deliberately. Every validator
  // sends one every minute, so leaving them in a plain-language feed would bury
  // the payment somebody is actually looking for under thousands of rows that
  // never concern them.
  '/blockchain.oracle.v1.MsgSubmitExchangeRates': (m, ctx) => ({
    kind: 'admin',
    title: 'Price report',
    summary: `${who(m.feeder, ctx)} reported ${(m.rates ?? []).length} price${
      (m.rates ?? []).length === 1 ? '' : 's'
    } for ${truncateAddress(m.validator ?? '')}`,
    everyday: false,
    actor: m.feeder,
    details: (m.rates ?? []).map((r: any) => ({
      label: r.denom ?? '',
      value: r.rate ?? '',
    })),
  }),
  '/blockchain.oracle.v1.MsgDelegateFeeder': (m, ctx) => ({
    kind: 'admin',
    title: 'Price reporting delegated',
    summary: `${truncateAddress(m.validator ?? '')} authorised ${who(m.feeder, ctx)} to report prices on its behalf`,
    everyday: false,
    actor: m.operator,
    counterparty: m.feeder,
  }),
  '/blockchain.oracle.v1.MsgApplyAppraiser': (m, ctx) => ({
    kind: 'admin',
    title: 'Valuer application',
    summary: `${who(m.creator, ctx)} applied to value real-world assets${m.name ? ` as "${m.name}"` : ''}`,
    everyday: false,
    actor: m.creator,
    details: [
      { label: 'Credentials', value: m.credentials ?? '' },
      { label: 'Asset types', value: (m.class_ids ?? []).join(', ') || 'all' },
    ],
  }),
  '/blockchain.oracle.v1.MsgApproveAppraiser': (m, ctx) => ({
    kind: 'governance',
    title: 'Valuer decision',
    summary: `Governance ${m.approve ? 'approved' : 'rejected'} ${who(m.appraiser, ctx)} as an independent valuer`,
    everyday: false,
    counterparty: m.appraiser,
  }),
  '/blockchain.oracle.v1.MsgRevokeAppraiser': (m, ctx) => ({
    kind: 'governance',
    title: 'Valuer authority withdrawn',
    summary: `Governance withdrew ${who(m.appraiser, ctx)}'s authority to value assets${
      m.reason ? `: ${m.reason}` : ''
    }. Their existing valuations remain on record.`,
    everyday: false,
    counterparty: m.appraiser,
  }),
  '/blockchain.oracle.v1.MsgSubmitAppraisal': (m, ctx) => ({
    kind: 'admin',
    title: 'Asset valued',
    summary: `${who(m.appraiser, ctx)} valued ${m.class_id}/${m.nft_id} at ${amountOf(
      m.value,
      m.value_denom,
      ctx,
    )}`,
    everyday: true,
    actor: m.appraiser,
    details: [
      { label: 'Valuation date', value: unixDate(m.valued_at) },
      { label: 'Method', value: m.method ?? '' },
      ...(m.report_uri ? [{ label: 'Report', value: m.report_uri }] : []),
    ],
  }),

  // ---- permissions ------------------------------------------------------
  '/cosmos.authz.v1beta1.MsgGrant': (m, ctx) => ({
    kind: 'admin',
    title: 'Permission granted',
    summary: `${who(m.granter, ctx)} authorised ${who(m.grantee, ctx)} to act on their behalf`,
    everyday: false,
    actor: m.granter,
    counterparty: m.grantee,
  }),
  '/cosmos.authz.v1beta1.MsgRevoke': (m, ctx) => ({
    kind: 'admin',
    title: 'Permission revoked',
    summary: `${who(m.granter, ctx)} withdrew ${who(m.grantee, ctx)}'s authorisation`,
    everyday: false,
    actor: m.granter,
    counterparty: m.grantee,
  }),
  '/cosmos.authz.v1beta1.MsgExec': (m, ctx) => ({
    kind: 'admin',
    title: 'Acted on behalf',
    summary: `${who(m.grantee, ctx)} acted using an authorisation granted to them`,
    everyday: false,
    actor: m.grantee,
  }),
  '/cosmos.feegrant.v1beta1.MsgGrantAllowance': (m, ctx) => ({
    kind: 'admin',
    title: 'Fee sponsorship',
    summary: `${who(m.granter, ctx)} agreed to cover ${who(m.grantee, ctx)}'s transaction fees`,
    everyday: false,
    actor: m.granter,
    counterparty: m.grantee,
  }),
  '/cosmos.feegrant.v1beta1.MsgRevokeAllowance': (m, ctx) => ({
    kind: 'admin',
    title: 'Sponsorship ended',
    summary: `${who(m.granter, ctx)} stopped covering ${who(m.grantee, ctx)}'s fees`,
    everyday: false,
    actor: m.granter,
  }),

  // ---- shared multisig (x/group) ---------------------------------------
  '/cosmos.group.v1.MsgCreateGroupWithPolicy': (m, ctx) => ({
    kind: 'admin',
    title: 'Shared account created',
    summary: `${who(m.admin, ctx)} created a shared account requiring multiple approvals`,
    everyday: false,
    actor: m.admin,
  }),
  '/cosmos.group.v1.MsgSubmitProposal': (m, ctx) => ({
    kind: 'governance',
    title: 'Approval requested',
    summary: `${who(Array.isArray(m.proposers) ? m.proposers[0] : undefined, ctx)} requested approval for an action on a shared account`,
    everyday: true,
  }),
  '/cosmos.group.v1.MsgVote': (m, ctx) => ({
    kind: 'governance',
    title: 'Approval given',
    summary: `${who(m.voter, ctx)} voted ${voteOption(m.option)} on shared-account request ${m.proposal_id}`,
    everyday: true,
    actor: m.voter,
  }),
  '/cosmos.group.v1.MsgExec': (m, ctx) => ({
    kind: 'governance',
    title: 'Approved action carried out',
    summary: `Request ${m.proposal_id} reached enough approvals and was carried out`,
    everyday: true,
    actor: m.executor,
  }),

  // ---- cross-chain ------------------------------------------------------
  '/ibc.applications.transfer.v1.MsgTransfer': (m, ctx) => ({
    kind: 'transfer',
    title: 'Sent to another chain',
    summary: `${who(m.sender, ctx)} sent ${m.token ? money([m.token as Coin], ctx) : 'funds'} to another chain`,
    everyday: true,
    actor: m.sender,
    counterparty: m.receiver,
    coins: m.token ? [m.token as Coin] : [],
  }),
};

/** Governance and module parameter updates all read the same way. */
function isParamUpdate(typeUrl: string): boolean {
  return typeUrl.endsWith('.MsgUpdateParams');
}

/**
 * Decodes a message into something readable.
 *
 * An unrecognised type still produces a usable row rather than an empty one:
 * the module and action are derivable from the type URL alone, which is enough
 * to tell somebody what area of the chain was touched while making it obvious
 * the decoder needs extending.
 */
export function decodeMessage(message: Record<string, any>, ctx: DecodeContext = {}): DecodedMessage {
  const typeUrl: string = message['@type'] ?? message.typeUrl ?? 'unknown';
  const decoder = DECODERS[typeUrl];

  if (decoder) {
    return { typeUrl, raw: message, ...decoder(message, ctx) };
  }

  if (isParamUpdate(typeUrl)) {
    return {
      typeUrl,
      raw: message,
      kind: 'governance',
      title: 'Settings change',
      summary: `Governance updated the ${moduleOf(typeUrl)} module's settings`,
      everyday: false,
    };
  }

  return { typeUrl, raw: message, ...fallbackDecode(typeUrl) };
}

/**
 * What a proposal would do, phrased as a thing that has not happened yet.
 *
 * The ordinary decoders describe messages that already executed, so they are
 * written in the past tense — correct on a transaction page, wrong under "if
 * this passes", where "Governance approved X" reads as though the vote were
 * over. Rather than bend every decoder, the handful of messages governance can
 * actually execute get a forward-looking phrasing here, and anything else falls
 * back to the decoded summary: an awkward tense is much better than a proposal
 * whose effect is not described at all.
 */
export function describeProposalAction(message: Record<string, any>, ctx: DecodeContext = {}): string {
  const typeUrl: string = message['@type'] ?? message.typeUrl ?? 'unknown';
  const m = message;

  switch (typeUrl) {
    case '/blockchain.oracle.v1.MsgApproveAppraiser':
      return m.approve
        ? `Admit ${who(m.appraiser, ctx)} as an independent valuer${
            m.class_ids?.length ? `, limited to ${m.class_ids.join(', ')}` : ''
          }`
        : `Refuse ${who(m.appraiser, ctx)}'s application to value assets`;

    case '/blockchain.oracle.v1.MsgRevokeAppraiser':
      return `Withdraw ${who(m.appraiser, ctx)}'s authority to value assets${m.reason ? ` — ${m.reason}` : ''}`;

    case '/blockchain.validatorgov.v1.MsgApproveValidator':
      return m.approve
        ? `Allow ${who(m.candidate, ctx)} to run a validator`
        : `Refuse ${who(m.candidate, ctx)}'s application to validate`;

    case '/blockchain.stablecoin.v1.MsgApproveIssuer':
      return m.approve
        ? `Allow ${who(m.issuer, ctx)} to issue ${m.denom ?? 'a currency'}`
        : `Refuse ${who(m.issuer, ctx)}'s application to issue currency`;

    case '/blockchain.paymsg.v1.MsgApproveParticipant':
      return m.approve
        ? `Allow ${who(m.participant, ctx)} to send and receive payments`
        : `Refuse ${who(m.participant, ctx)}'s application to route payments`;

    case '/blockchain.builderfee.v1.MsgApproveBuilder':
      return m.approve
        ? `Grant a share of the fees on ${shortTypeUrl(m.msg_type_url)}`
        : `Refuse a fee share on ${shortTypeUrl(m.msg_type_url)}`;

    default:
      if (isParamUpdate(typeUrl)) {
        return `Change the ${moduleOf(typeUrl)} module's settings`;
      }
      return decodeMessage(message, ctx).summary;
  }
}

function fallbackDecode(typeUrl: string): Omit<DecodedMessage, 'typeUrl' | 'raw'> {
  const action = shortTypeUrl(typeUrl).replace(/^Msg/, '');
  const spaced = action.replace(/([a-z])([A-Z])/g, '$1 $2');
  return {
    kind: 'other',
    title: spaced || 'Unknown',
    summary: `${spaced || 'An action'} on the ${moduleOf(typeUrl)} module`,
    everyday: false,
  };
}

/** `/blockchain.amm.v1.MsgSwap` → `MsgSwap`. */
export function shortTypeUrl(typeUrl: string | undefined): string {
  if (!typeUrl) return 'unknown';
  const parts = typeUrl.split('.');
  return parts[parts.length - 1] ?? typeUrl;
}

/** `/blockchain.amm.v1.MsgSwap` → `amm`. */
export function moduleOf(typeUrl: string | undefined): string {
  if (!typeUrl) return 'chain';
  const match = /^\/(?:[a-z0-9]+)\.([a-z0-9]+)\./.exec(typeUrl);
  return match?.[1] ?? 'chain';
}

function resolveSymbol(denom: unknown, ctx: DecodeContext): string {
  if (typeof denom !== 'string') return 'another asset';
  return formatAmount('0', denom, { registry: ctx.registry }).split(' ').slice(1).join(' ') || denom;
}

function voteOption(option: unknown): string {
  const map: Record<string, string> = {
    VOTE_OPTION_YES: 'yes',
    VOTE_OPTION_NO: 'no',
    VOTE_OPTION_ABSTAIN: 'abstain',
    VOTE_OPTION_NO_WITH_VETO: 'no with veto',
    VOTE_OPTION_ONE: 'yes',
    VOTE_OPTION_THREE: 'no',
  };
  return map[String(option)] ?? String(option ?? 'unknown').toLowerCase();
}

/**
 * A Unix-seconds field as a date.
 *
 * Only the date, not the time: these are valuation dates — the day an inspection
 * or a NAV describes — and showing a spurious 00:00:00 would imply a precision
 * the number does not have.
 */
function unixDate(seconds: unknown): string {
  const value = Number(seconds ?? 0);
  if (!Number.isFinite(value) || value <= 0) return '—';
  return new Date(value * 1000).toISOString().slice(0, 10);
}

function roleName(role: unknown): string {
  const map: Record<string, string> = {
    ROLE_ADMIN: 'administrator',
    ROLE_SPENDER: 'spender',
    ROLE_PAUSER: 'emergency freeze',
  };
  return map[String(role)] ?? String(role ?? 'unknown').toLowerCase();
}

/**
 * Whether a payment kept its ISO 20022 detail off-chain.
 *
 * The field arrives as base64 from the REST gateway and as bytes from a direct
 * protobuf decode, and both reach this file. Checking only one shape would make
 * the same payment read as private in the explorer and as blank in the wallet.
 */
function hasMetadataHash(m: Record<string, unknown>): boolean {
  const hash = m.metadata_hash ?? m.metadataHash;
  if (typeof hash === 'string') return hash.length > 0;
  if (hash instanceof Uint8Array) return hash.length > 0;
  return false;
}

/** ISO 20022 external purpose codes, as words. */
function purposeCode(code: unknown): string {
  const map: Record<string, string> = {
    SALA: 'Salary',
    SUPP: 'Supplier payment',
    TRAD: 'Trade',
    INTC: 'Intra-company transfer',
    TAXS: 'Tax',
    PENS: 'Pension',
  };
  const key = String(code ?? '');
  return map[key] ?? (key || '—');
}

/**
 * Summarises a whole transaction in one line.
 *
 * A transaction with a single message reads as that message. Several messages
 * of the same kind read as a batch. Anything else is described by count, since
 * inventing a narrative across unrelated actions would misrepresent it.
 */
export function summariseTransaction(messages: DecodedMessage[]): string {
  if (messages.length === 0) return 'An empty transaction';
  if (messages.length === 1) return messages[0].summary;

  const kinds = new Set(messages.map((m) => m.title));
  if (kinds.size === 1) {
    return `${messages.length} × ${messages[0].title.toLowerCase()}`;
  }
  return `${messages.length} actions in one transaction`;
}
