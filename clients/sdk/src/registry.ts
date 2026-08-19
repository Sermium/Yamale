/**
 * This chain's own messages, registered so they can be signed.
 *
 * CosmJS can only encode message types it has been given an encoder for. Its
 * default registry covers the standard Cosmos modules, which is why transfers,
 * staking and votes worked from the start and payments, treasury spends and
 * swaps did not — signing one is not a matter of knowing its type URL, it is a
 * matter of turning it into protobuf bytes.
 *
 * The encoders come from `./generated`, which is produced by `make proto-ts`
 * from the same .proto files the chain is built from. That is the whole point:
 * a client that hand-wrote its encoders would drift from the chain the first
 * time a field was added, and drift silently.
 */

import { defaultRegistryTypes } from '@cosmjs/stargate';
import { Registry, type GeneratedType } from '@cosmjs/proto-signing';

import * as amm from './generated/blockchain/amm/v1/tx.ts';
import * as builderfee from './generated/blockchain/builderfee/v1/tx.ts';
import * as enforcement from './generated/blockchain/enforcement/v1/tx.ts';
import * as alias from './generated/blockchain/alias/v1/tx.ts';
import * as feegrant from './generated/cosmos/feegrant/v1beta1/tx.ts';
import * as oracle from './generated/blockchain/oracle/v1/tx.ts';
import * as paymsg from './generated/blockchain/paymsg/v1/tx.ts';
import * as stablecoin from './generated/blockchain/stablecoin/v1/tx.ts';
import * as treasury from './generated/blockchain/treasury/v1/tx.ts';
import * as validatorgov from './generated/blockchain/validatorgov/v1/tx.ts';

/**
 * Every message this chain adds, paired with its type URL.
 *
 * Authority-gated messages — the `Approve*` family, `UpdateParams` — are
 * included deliberately. They can only be executed by the governance module
 * account, so registering them does not grant anyone anything; what it does is
 * let an interface *build* one to put inside a proposal, which is otherwise a
 * CLI-only step in every one of the approval flows.
 */
export const CHAIN_MESSAGE_TYPES: ReadonlyArray<[string, GeneratedType]> = [
  // Fee grants. Not this chain's own messages, but not in CosmJS's default
  // registry either — and they are how an institution pays the network fee for
  // a customer who holds only their own currency. Without them the answer to
  // "must my customers hold YML?" is yes, from a browser at least.
  ['/cosmos.feegrant.v1beta1.MsgGrantAllowance', feegrant.MsgGrantAllowance],
  ['/cosmos.feegrant.v1beta1.MsgRevokeAllowance', feegrant.MsgRevokeAllowance],

  // Payments
  ['/blockchain.paymsg.v1.MsgSendPayment', paymsg.MsgSendPayment],
  ['/blockchain.paymsg.v1.MsgApplyParticipant', paymsg.MsgApplyParticipant],
  ['/blockchain.paymsg.v1.MsgApproveParticipant', paymsg.MsgApproveParticipant],
  ['/blockchain.paymsg.v1.MsgRegisterCustomer', paymsg.MsgRegisterCustomer],

  // Treasuries
  ['/blockchain.treasury.v1.MsgCreateTreasury', treasury.MsgCreateTreasury],
  ['/blockchain.treasury.v1.MsgDeposit', treasury.MsgDeposit],
  ['/blockchain.treasury.v1.MsgSpend', treasury.MsgSpend],
  ['/blockchain.treasury.v1.MsgCreateLock', treasury.MsgCreateLock],
  // Conditional locks — escrow. The chain holds the money between the deal and
  // its end, which is the one guarantee an application cannot give itself.
  ['/blockchain.treasury.v1.MsgOpenEscrow', treasury.MsgOpenEscrow],
  ['/blockchain.treasury.v1.MsgReleaseEscrow', treasury.MsgReleaseEscrow],
  ['/blockchain.treasury.v1.MsgDisputeEscrow', treasury.MsgDisputeEscrow],
  ['/blockchain.treasury.v1.MsgResolveEscrow', treasury.MsgResolveEscrow],
  ['/blockchain.treasury.v1.MsgClaimLock', treasury.MsgClaimLock],
  ['/blockchain.treasury.v1.MsgRevokeLock', treasury.MsgRevokeLock],
  ['/blockchain.treasury.v1.MsgAssignRole', treasury.MsgAssignRole],
  ['/blockchain.treasury.v1.MsgRevokeRole', treasury.MsgRevokeRole],
  ['/blockchain.treasury.v1.MsgSetSpendPolicy', treasury.MsgSetSpendPolicy],
  ['/blockchain.treasury.v1.MsgSetPaused', treasury.MsgSetPaused],
  ['/blockchain.treasury.v1.MsgSetAdmin', treasury.MsgSetAdmin],

  // Trading
  ['/blockchain.amm.v1.MsgCreatePool', amm.MsgCreatePool],
  ['/blockchain.amm.v1.MsgJoinPool', amm.MsgJoinPool],
  ['/blockchain.amm.v1.MsgExitPool', amm.MsgExitPool],
  ['/blockchain.amm.v1.MsgSwap', amm.MsgSwap],

  // Currencies
  ['/blockchain.stablecoin.v1.MsgRegisterCurrency', stablecoin.MsgRegisterCurrency],
  ['/blockchain.stablecoin.v1.MsgMintCoin', stablecoin.MsgMintCoin],
  ['/blockchain.stablecoin.v1.MsgBurnCoin', stablecoin.MsgBurnCoin],
  ['/blockchain.stablecoin.v1.MsgApproveIssuer', stablecoin.MsgApproveIssuer],

  // Prices and valuations
  ['/blockchain.oracle.v1.MsgSubmitExchangeRates', oracle.MsgSubmitExchangeRates],
  ['/blockchain.oracle.v1.MsgDelegateFeeder', oracle.MsgDelegateFeeder],
  ['/blockchain.oracle.v1.MsgApplyAppraiser', oracle.MsgApplyAppraiser],
  ['/blockchain.oracle.v1.MsgApproveAppraiser', oracle.MsgApproveAppraiser],
  ['/blockchain.oracle.v1.MsgRevokeAppraiser', oracle.MsgRevokeAppraiser],
  ['/blockchain.oracle.v1.MsgSubmitAppraisal', oracle.MsgSubmitAppraisal],

  // Freezing and recovering stolen assets
  ['/blockchain.alias.v1.MsgRegisterAlias', alias.MsgRegisterAlias],
  ['/blockchain.alias.v1.MsgRotateAlias', alias.MsgRotateAlias],
  ['/blockchain.alias.v1.MsgSetJurisdiction', alias.MsgSetJurisdiction],
  ['/blockchain.enforcement.v1.MsgOpenCase', enforcement.MsgOpenCase],
  ['/blockchain.enforcement.v1.MsgVoteCase', enforcement.MsgVoteCase],
  ['/blockchain.enforcement.v1.MsgWithdrawCase', enforcement.MsgWithdrawCase],
  ['/blockchain.enforcement.v1.MsgSweep', enforcement.MsgSweep],
  ['/blockchain.enforcement.v1.MsgReverseCase', enforcement.MsgReverseCase],
  ['/blockchain.enforcement.v1.MsgEmergencyFreeze', enforcement.MsgEmergencyFreeze],
  ['/blockchain.enforcement.v1.MsgEmergencyRelease', enforcement.MsgEmergencyRelease],

  // Validator onboarding
  ['/blockchain.validatorgov.v1.MsgApplyValidator', validatorgov.MsgApplyValidator],
  ['/blockchain.validatorgov.v1.MsgApproveValidator', validatorgov.MsgApproveValidator],
  // Operator key rotation. The recovery path is the one that most needs to be
  // buildable from an interface: whoever notices a lost key is rarely the
  // person who lost it, and telling them to install the CLI first is how a
  // validator stays unrecoverable.
  ['/blockchain.validatorgov.v1.MsgRotateOperator', validatorgov.MsgRotateOperator],
  ['/blockchain.validatorgov.v1.MsgProposeOperatorRecovery', validatorgov.MsgProposeOperatorRecovery],
  ['/blockchain.validatorgov.v1.MsgApproveOperatorRecovery', validatorgov.MsgApproveOperatorRecovery],
  ['/blockchain.validatorgov.v1.MsgCancelOperatorRotation', validatorgov.MsgCancelOperatorRotation],

  // Builder fee share
  ['/blockchain.builderfee.v1.MsgRegisterBuilder', builderfee.MsgRegisterBuilder],
  ['/blockchain.builderfee.v1.MsgApproveBuilder', builderfee.MsgApproveBuilder],
];

/**
 * A registry covering the standard Cosmos messages and this chain's own.
 *
 * Built once and shared: constructing one per signer would re-register every
 * type on each connection for no benefit.
 */
export function chainRegistry(): Registry {
  return new Registry([...defaultRegistryTypes, ...CHAIN_MESSAGE_TYPES]);
}
