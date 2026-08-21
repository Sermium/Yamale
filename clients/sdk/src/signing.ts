/**
 * Signing and broadcasting.
 *
 * Everything else in this package reads the chain. This is the half that
 * changes it, and it is deliberately built around an injected signer rather
 * than around a browser wallet: the same code path is used by a browser
 * extension, by a key held in a script, and by the test suite — so the
 * behaviour that matters can be exercised against a real chain without a
 * browser in the loop.
 *
 * What it does not do is decide anything. Amounts arrive in base units, the
 * caller has already been shown what will happen, and this module's whole job
 * is to turn that into a signed transaction and report honestly what the chain
 * did with it.
 */

import { SigningStargateClient, type DeliverTxResponse, type StdFee } from '@cosmjs/stargate';
import type { OfflineSigner } from '@cosmjs/proto-signing';

import { chainRegistry } from './registry.ts';
import { BasicAllowance } from './generated/cosmos/feegrant/v1beta1/feegrant.ts';

import { describeTxResult, type TranslatedError } from './errors.ts';
import type { Coin } from './denom.ts';

export interface SignerOptions {
  /** The node's CometBFT RPC endpoint, e.g. http://localhost:26657 */
  rpcUrl: string;
  chainId: string;
  /** Fee denom; defaults to the chain's native token. */
  feeDenom?: string;
  /**
   * Gas price in base units per unit of gas. The chain's own devnet runs at
   * zero, but a network with a non-zero minimum will reject anything below it.
   */
  gasPrice?: number;
}

/** What happened, in terms a caller can act on. */
export interface SubmitResult {
  hash: string;
  height: number;
  /** 0 means the transaction did what it said. */
  code: number;
  succeeded: boolean;
  gasUsed: number;
  /** Present when the transaction failed, already translated. */
  error?: TranslatedError;
  raw: DeliverTxResponse;
}

/**
 * A signer bound to one chain.
 *
 * Constructed from any OfflineSigner: `window.keplr.getOfflineSigner(chainId)`
 * in a browser, `DirectSecp256k1HdWallet.fromMnemonic(...)` in a script or a
 * test. Nothing below knows which it was given.
 */
export class ChainSigner {
  private readonly options: SignerOptions;
  private readonly signer: OfflineSigner;
  private client: SigningStargateClient | null = null;

  constructor(signer: OfflineSigner, options: SignerOptions) {
    this.signer = signer;
    this.options = options;
  }

  /** The address that will sign, from the first account the signer offers. */
  async address(): Promise<string> {
    const accounts = await this.signer.getAccounts();
    if (accounts.length === 0) {
      throw new Error('the signer holds no accounts');
    }
    return accounts[0]!.address;
  }

  private async connect(): Promise<SigningStargateClient> {
    if (!this.client) {
      // With the chain's own registry, not the default one: without it every
      // message this chain adds fails to encode, which is what made payments,
      // treasury spends and swaps CLI-only.
      this.client = await SigningStargateClient.connectWithSigner(this.options.rpcUrl, this.signer, {
        registry: chainRegistry(),
      });
    }
    return this.client;
  }

  /**
   * Signs, broadcasts, and waits for the transaction to be included.
   *
   * Waits deliberately. The result of broadcasting is only that the node
   * accepted the transaction into its mempool — a swap below its slippage
   * floor, a payment from an unapproved participant and a spend over a
   * treasury's limit all broadcast cleanly and then fail in the block. An
   * interface that reported the broadcast would tell somebody their payment
   * succeeded when the chain had rejected it.
   */
  async submit(messages: readonly EncodeObject[], memo = '', gas = 200_000): Promise<SubmitResult> {
    const client = await this.connect();
    const sender = await this.address();

    let result: DeliverTxResponse;
    try {
      result = await client.signAndBroadcast(sender, [...messages], this.fee(gas), memo);
    } catch (caught) {
      // A transaction rejected at CheckTx — an unaffordable fee, a bad
      // sequence, a mempool refusal — never reaches a block, and CosmJS
      // *throws* for those rather than returning a result. Left unhandled that
      // breaks the promise this method makes: callers are told to read
      // `succeeded`, and would instead get an exception for the single most
      // common failure a real user hits. Found by pointing this at a live
      // chain; the unit tests never saw it because they never broadcast.
      const rejection = asBroadcastFailure(caught);
      if (!rejection) throw caught;

      const described = describeTxResult(rejection.code, rejection.log);
      return {
        hash: '',
        height: 0,
        code: rejection.code,
        succeeded: false,
        gasUsed: 0,
        error: described.error,
        raw: { code: rejection.code, rawLog: rejection.log } as DeliverTxResponse,
      };
    }

    const described = describeTxResult(result.code, result.rawLog ?? '');

    return {
      hash: result.transactionHash,
      height: result.height,
      code: result.code,
      succeeded: described.ok,
      gasUsed: Number(result.gasUsed ?? 0),
      error: described.error,
      raw: result,
    };
  }

  private fee(gas: number): StdFee {
    const denom = this.options.feeDenom ?? 'uyml';
    const price = this.options.gasPrice ?? 0;
    // Rounded up: a fee a fraction below the node's minimum is rejected
    // outright, and the difference is a rounding error in cost terms.
    const amount = Math.ceil(gas * price).toString();

    return { amount: [{ denom, amount }], gas: gas.toString() };
  }

  disconnect(): void {
    this.client?.disconnect();
    this.client = null;
  }
}

/**
 * Recognises a rejection at broadcast, as opposed to a genuine fault.
 *
 * Matched on shape rather than on the error's class, because the class is not
 * exported by CosmJS and an `instanceof` against a package internal is a
 * dependency upgrade away from silently never matching — which would put the
 * exception back without any test noticing.
 */
function asBroadcastFailure(caught: unknown): { code: number; log: string } | null {
  const error = caught as { code?: unknown; log?: unknown; message?: unknown };
  if (typeof error?.code !== 'number' || error.code === 0) return null;
  const log = typeof error.log === 'string' ? error.log : String(error.message ?? '');
  return { code: error.code, log };
}

/** The shape CosmJS uses for an encoded message. */
export interface EncodeObject {
  typeUrl: string;
  value: unknown;
}

// ---------------------------------------------------------------- messages
//
// Constructors for the actions a person actually takes. They exist so an
// interface never assembles a typeUrl by hand — a typo there produces a
// transaction the chain rejects for reasons that read as unrelated.
//
// These cover the standard Cosmos messages, which CosmJS can encode out of the
// box. This chain's own follow below, encoded through the generated registry in
// ./registry.ts.

/** Move tokens from the signer to another account. */
export function send(from: string, to: string, amount: Coin[]): EncodeObject {
  return {
    typeUrl: '/cosmos.bank.v1beta1.MsgSend',
    value: { fromAddress: from, toAddress: to, amount },
  };
}

/** Stake with a validator. */
export function delegate(delegator: string, validator: string, amount: Coin): EncodeObject {
  return {
    typeUrl: '/cosmos.staking.v1beta1.MsgDelegate',
    value: { delegatorAddress: delegator, validatorAddress: validator, amount },
  };
}

/**
 * Begin unstaking.
 *
 * Named for what it does rather than for its message: the tokens are not
 * returned for the whole unbonding period, and an interface that calls this
 * "withdraw" invites somebody to expect their balance to change now.
 */
export function beginUnbonding(delegator: string, validator: string, amount: Coin): EncodeObject {
  return {
    typeUrl: '/cosmos.staking.v1beta1.MsgUndelegate',
    value: { delegatorAddress: delegator, validatorAddress: validator, amount },
  };
}

export type VoteOption = 'yes' | 'abstain' | 'no' | 'veto';

const VOTE_OPTIONS: Record<VoteOption, number> = {
  yes: 1,
  abstain: 2,
  no: 3,
  veto: 4,
};

/** Vote on a governance proposal. */
export function vote(voter: string, proposalId: string, option: VoteOption): EncodeObject {
  return {
    typeUrl: '/cosmos.gov.v1.MsgVote',
    value: { proposalId: BigInt(proposalId), voter, option: VOTE_OPTIONS[option] },
  };
}

/** Claim staking rewards from one validator. */
export function claimRewards(delegator: string, validator: string): EncodeObject {
  return {
    typeUrl: '/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward',
    value: { delegatorAddress: delegator, validatorAddress: validator },
  };
}

// ------------------------------------------------- this chain's own messages
//
// Same reasoning as the standard ones above: an interface should never assemble
// a type URL by hand. These are the actions the guides describe, now available
// to anything holding a signer.

/**
 * Send an ISO 20022 credit transfer.
 *
 * Prefer `metadataHash` over `purposeCode` and `remittanceInformation`. The
 * plaintext pair goes onto a public ledger that cannot forget it, and it is
 * where a customer's name ends up in practice; the hash pins a payload kept
 * off-chain (see metadata.ts) and reveals nothing. Passing both is refused
 * here rather than by the chain, so the caller finds out before paying a fee
 * for a transaction that cannot succeed.
 */
export function payment(fields: {
  debtor: string;
  endToEndId: string;
  instructingParticipant: string;
  instructedParticipant: string;
  creditor: string;
  denom: string;
  amount: string;
  purposeCode?: string;
  remittanceInformation?: string;
  metadataHash?: Uint8Array;
  /** ISO 3166-1 alpha-2. Names the authority that may act on this payment. */
  settlementJurisdiction?: string;
}): EncodeObject {
  const purposeCode = fields.purposeCode ?? '';
  const remittanceInformation = fields.remittanceInformation ?? '';
  const metadataHash = fields.metadataHash ?? new Uint8Array(0);

  if (metadataHash.length > 0 && (purposeCode !== '' || remittanceInformation !== '')) {
    throw new Error(
      'a payment carries its ISO 20022 detail as metadataHash or as plaintext, never both: sending both records the detail the hash exists to keep off the ledger',
    );
  }

  const jurisdiction = fields.settlementJurisdiction ?? '';
  if (jurisdiction !== '' && !/^[A-Z]{2}$/.test(jurisdiction)) {
    throw new Error(
      `settlementJurisdiction must be a two-letter uppercase ISO 3166-1 alpha-2 code, got "${jurisdiction}"`,
    );
  }

  return {
    typeUrl: '/blockchain.paymsg.v1.MsgSendPayment',
    value: {
      debtor: fields.debtor,
      endToEndId: fields.endToEndId,
      instructingParticipant: fields.instructingParticipant,
      instructedParticipant: fields.instructedParticipant,
      creditor: fields.creditor,
      denom: fields.denom,
      amount: fields.amount,
      purposeCode,
      remittanceInformation,
      metadataHash,
      settlementJurisdiction: jurisdiction,
    },
  };
}

/** Swap against a liquidity pool. `minAmountOut` is the floor, not a hint. */
export function swap(fields: {
  sender: string;
  poolId: string;
  tokenInDenom: string;
  tokenInAmount: string;
  tokenOutDenom: string;
  minAmountOut: string;
}): EncodeObject {
  return { typeUrl: '/blockchain.amm.v1.MsgSwap', value: { ...fields } };
}

/** Open a shared treasury. The admin defaults to the creator when empty. */
export function createTreasury(creator: string, name: string, admin = ''): EncodeObject {
  return { typeUrl: '/blockchain.treasury.v1.MsgCreateTreasury', value: { creator, name, admin } };
}

/** Fund a treasury. Permissionless, the way paying an invoice is. */
export function depositToTreasury(depositor: string, treasuryId: string, amount: Coin[]): EncodeObject {
  return {
    typeUrl: '/blockchain.treasury.v1.MsgDeposit',
    value: { depositor, treasuryId, amount },
  };
}

/** Pay out of a treasury, within its spending policy. */
export function treasurySpend(fields: {
  spender: string;
  treasuryId: string;
  recipient: string;
  amount: Coin[];
  memo?: string;
}): EncodeObject {
  return {
    typeUrl: '/blockchain.treasury.v1.MsgSpend',
    value: { ...fields, memo: fields.memo ?? '' },
  };
}

/** Claim whatever a treasury lock has released to you. */
export function claimLock(beneficiary: string, lockId: string): EncodeObject {
  return { typeUrl: '/blockchain.treasury.v1.MsgClaimLock', value: { beneficiary, lockId } };
}

/** Report observed prices for the current voting round. */
export function submitRates(
  feeder: string,
  validator: string,
  rates: Array<{ denom: string; rate: string }>,
): EncodeObject {
  return {
    typeUrl: '/blockchain.oracle.v1.MsgSubmitExchangeRates',
    value: { feeder, validator, rates },
  };
}

/** Case actions, named for what they do rather than for their enum values. */
export type CaseAction = 'freeze' | 'seize';

const CASE_ACTIONS: Record<CaseAction, number> = { freeze: 1, seize: 2 };

/**
 * Accuse an address and freeze it while the validators decide.
 *
 * The freeze takes effect in the block this lands in, not when the vote ends —
 * which is the only way it is any use against somebody actively moving funds.
 * It expires by itself if the case is never resolved.
 */
export function openCase(fields: {
  /** The validator's own account address — the key it signs with. */
  opener: string;
  target: string;
  action: CaseAction;
  reason: string;
  evidenceUri?: string;
  evidenceHash?: string;
}): EncodeObject {
  return {
    typeUrl: '/blockchain.enforcement.v1.MsgOpenCase',
    value: {
      opener: fields.opener,
      target: fields.target,
      action: CASE_ACTIONS[fields.action],
      reason: fields.reason,
      evidenceUri: fields.evidenceUri ?? '',
      evidenceHash: fields.evidenceHash ?? '',
    },
  };
}

export type CaseVote = 'yes' | 'no' | 'abstain';

const CASE_VOTES: Record<CaseVote, number> = { yes: 1, no: 2, abstain: 3 };

/**
 * Vote on an enforcement case, as a validator.
 *
 * `voter` is the validator's account address, not its operator address: the
 * chain reads the operator and the voting power from the staking module.
 */
export function voteCase(voter: string, caseId: string, option: CaseVote): EncodeObject {
  return {
    typeUrl: '/blockchain.enforcement.v1.MsgVoteCase',
    value: { voter, caseId, option: CASE_VOTES[option] },
  };
}

/** Take back a case you opened, releasing the account. */
export function withdrawCase(opener: string, caseId: string): EncodeObject {
  return { typeUrl: '/blockchain.enforcement.v1.MsgWithdrawCase', value: { opener, caseId } };
}

/**
 * Collect what a passed seizure can now reach.
 *
 * Anyone may send this and nobody gains by it: the destination is fixed by the
 * module's parameters. It exists because a seizure against staked funds is not
 * finished on the day it passes.
 */
export function sweepCase(sender: string, caseId: string): EncodeObject {
  return { typeUrl: '/blockchain.enforcement.v1.MsgSweep', value: { sender, caseId } };
}

/**
 * Freeze an account on the founders' authority, without waiting for a validator
 * to open a case.
 *
 * `authority` is the founders' group policy address, so this is built to go
 * inside an x/group proposal rather than signed directly. There is deliberately
 * no emergency seizure to pair with it: this path can stop money and let it go
 * again, and can never take any.
 */
export function emergencyFreeze(fields: {
  authority: string;
  target: string;
  reason: string;
  evidenceUri?: string;
  evidenceHash?: string;
}): EncodeObject {
  return {
    typeUrl: '/blockchain.enforcement.v1.MsgEmergencyFreeze',
    value: {
      authority: fields.authority,
      target: fields.target,
      reason: fields.reason,
      evidenceUri: fields.evidenceUri ?? '',
      evidenceHash: fields.evidenceHash ?? '',
    },
  };
}

/** Lift a freeze immediately, whoever imposed it. */
export function emergencyRelease(authority: string, caseId: string, reason = ''): EncodeObject {
  return {
    typeUrl: '/blockchain.enforcement.v1.MsgEmergencyRelease',
    value: { authority, caseId, reason },
  };
}

// ---------------------------------------------------------------- fee grants
//
// The answer to "must my customers hold YML to move their own money?" — no, if
// their institution sponsors them. Fees are payable in YML, so an account
// holding only naira is stuck behind a currency it never asked for; a grant
// lets the bank pay instead, which is how ISO 20022 already works. The
// institution bears the cost of the rails.

/**
 * Sponsor another account's network fees.
 *
 * `spendLimit` is a budget, not a blank cheque: it is consumed as fees are
 * paid, and when it runs out the grantee is refused again. Leave it empty for
 * an unlimited allowance — which is rarely what anyone means, and is why it has
 * to be said explicitly rather than being the default.
 *
 * `expiresAt` ends it on a date whatever is left. An allowance with neither a
 * limit nor an expiry is a standing authority to spend the granter's balance on
 * fees, so this function makes you pass both fields even when they are empty.
 */
export function grantFeeAllowance(fields: {
  granter: string;
  grantee: string;
  spendLimit: Coin[];
  expiresAt?: Date;
}): EncodeObject {
  const allowance = BasicAllowance.fromPartial({
    spendLimit: fields.spendLimit,
    expiration: fields.expiresAt,
  });

  return {
    typeUrl: '/cosmos.feegrant.v1beta1.MsgGrantAllowance',
    value: {
      granter: fields.granter,
      grantee: fields.grantee,
      // Packed by hand because the field is an Any: the chain accepts several
      // kinds of allowance and the type URL is what tells them apart. A basic
      // allowance is the one an institution wants — a capped, optionally dated
      // budget — and the periodic kind is a per-period refill that is easy to
      // reach for and easy to leave running.
      allowance: {
        typeUrl: '/cosmos.feegrant.v1beta1.BasicAllowance',
        value: BasicAllowance.encode(allowance).finish(),
      },
    },
  };
}

/**
 * Withdraw a sponsorship. Takes effect immediately: the next transaction the
 * grantee sends naming this granter is refused.
 */
export function revokeFeeAllowance(granter: string, grantee: string): EncodeObject {
  return {
    typeUrl: '/cosmos.feegrant.v1beta1.MsgRevokeAllowance',
    value: { granter, grantee },
  };
}

/**
 * Claim a user ID for an account.
 *
 * Carries no identifier and no country: the chain derives one, and takes the
 * country prefix from the jurisdiction already recorded against the account.
 * There is nothing for the sender to choose, which is what removes squatting
 * and lookalikes — and it means this constructor cannot get the ID wrong,
 * because it never names one. An account with no recorded jurisdiction is
 * refused rather than given a default.
 *
 * The assigned ID comes back in the response and in the `alias_registered`
 * event; a client learns it by querying afterwards rather than by predicting it.
 */
export function registerAlias(account: string): EncodeObject {
  return {
    typeUrl: '/blockchain.alias.v1.MsgRegisterAlias',
    value: { account },
  };
}

/**
 * Retire the account's user ID and take a new one.
 *
 * For a key that has been compromised. One message rather than release then
 * register, because that pair has to be atomic — an account between the two
 * would resolve to nothing while still existing. The retired ID is tombstoned
 * and never issued again.
 */
export function rotateAlias(account: string): EncodeObject {
  return {
    typeUrl: '/blockchain.alias.v1.MsgRotateAlias',
    value: { account },
  };
}

/**
 * Record, or correct, the country an account belongs to.
 *
 * Signed by the approved participant that onboarded the account — the party
 * that performed the KYC and therefore the only one that knows the answer — or
 * by a foundation administrator. Never by the account itself: a perimeter
 * somebody may choose is the perimeter with no authority watching it.
 *
 * Correcting a country the account already had retires its user ID and issues a
 * replacement carrying the new one, in this same message. That is what stops
 * the prefix ageing into a lie, and it is why calling this on somebody who
 * already holds an ID is not a small act.
 */
export function setJurisdiction(recorder: string, account: string, country: string): EncodeObject {
  return {
    typeUrl: '/blockchain.alias.v1.MsgSetJurisdiction',
    value: { recorder, account, country: country.toUpperCase() },
  };
}

/**
 * Publish the X25519 public key your payment payloads are sealed to.
 *
 * Only ever the public half. A private key on an append-only ledger is a
 * private key published to everyone forever, and there is no erasure path that
 * takes it back — so this refuses anything that is not 32 bytes rather than
 * letting a caller pass whichever half was to hand.
 *
 * Sending it again rotates. The previous version stays queryable, so payloads
 * already sealed to it remain openable by whoever holds that private half;
 * everything sealed afterwards goes to the new one. Nothing re-wraps history.
 */
export function registerViewingKey(account: string, publicKey: Uint8Array): EncodeObject {
  if (publicKey.length !== 32) {
    throw new Error(
      `a viewing key is 32 bytes of X25519, got ${publicKey.length}; a shorter value seals envelopes that open for nobody`,
    );
  }
  return { typeUrl: '/blockchain.alias.v1.MsgRegisterViewingKey', value: { account, publicKey } };
}

/**
 * Mark one of your viewing key versions compromised.
 *
 * It stops senders sealing to it. It does not make the payloads already sealed
 * to it unreadable — ciphertext that has been distributed cannot be recalled —
 * and any interface built on this must say so, because an operator who believes
 * a transaction closed the exposure will not go and destroy the payloads.
 *
 * The version is named rather than defaulted to the newest, because the key an
 * operator wants to revoke is usually the old one they have just rotated away
 * from.
 */
export function revokeViewingKey(account: string, version: string): EncodeObject {
  return { typeUrl: '/blockchain.alias.v1.MsgRevokeViewingKey', value: { account, version } };
}

/**
 * Name the authority that holds the third viewing key over payments settling in
 * one country. Governance or a foundation administrator.
 */
export function appointRegulator(authority: string, country: string, address: string): EncodeObject {
  return {
    typeUrl: '/blockchain.alias.v1.MsgAppointRegulator',
    value: { authority, country: country.toUpperCase(), address },
  };
}

/**
 * Grant the time-boxed role that reads payment detail across accounts.
 *
 * It expires by itself at the height given. There is no unbounded form and no
 * zero-means-forever: a role that can become permanent by leaving a field unset
 * is time-boxed only by convention, and the convention is what fails when
 * nobody is looking.
 */
export function grantAuditor(authority: string, address: string, expiresAtHeight: string): EncodeObject {
  return {
    typeUrl: '/blockchain.alias.v1.MsgGrantAuditor',
    value: { authority, address, expiresAtHeight },
  };
}

/**
 * Record where you serve the encrypted payloads of the payments you instruct.
 *
 * A directory entry, not key material. The payee is the party that has to find
 * it, and the only thing the payee is guaranteed to have is the payment record
 * — which names the instructing participant and nothing else.
 *
 * The empty string withdraws the store, and that is a supported act: a client
 * that then reports the detail as unavailable is telling the truth, where one
 * still calling a dead host reports a network fault and invites a retry that
 * will never work.
 */
export function setPayloadStore(participant: string, url: string): EncodeObject {
  return { typeUrl: '/blockchain.paymsg.v1.MsgSetPayloadStore', value: { participant, url } };
}
