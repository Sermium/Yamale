/**
 * @yamale/chain — the abstraction layer over the Yamale chain.
 *
 * Every interface built on this chain consumes this package rather than
 * speaking to the node directly. That is the whole point: the judgement about
 * what a message means, how an amount reads, and what an error tells somebody
 * to do next belongs in one place, not re-derived by each frontend with
 * slightly different answers.
 */

export {
  EMPTY_AMOUNT,
  KNOWN_DENOMS,
  formatAmount,
  formatCoins,
  groupDigits,
  isPoolShare,
  parseCoin,
  poolIdFromDenom,
  resolveDenom,
  toBaseUnits,
  toBaseUnitsOf,
  toDisplayAmount,
  type BaseUnits,
  type Coin,
  type DenomInfo,
  type FormatAmountOptions,
} from './denom.ts';

/**
 * Reading a signing request out of the bytes about to be signed, rather than
 * out of what the requesting application said about them.
 */
export {
  describe as describeRequestMessage,
  describeFee,
  headline,
  summariseSigningRequest,
  transactionHash,
  type RequestFee,
  type RequestMessage,
  type SigningRequestSummary,
} from './signrequest.ts';

/**
 * User IDs. The chain's own algorithm, ported once — see alias.ts for what went
 * wrong when three partial copies of it were scattered across the clients.
 */
export {
  ALPHABET,
  COUNTRY_LENGTH,
  FOUNDATION_COUNTRY,
  assignedCountry,
  formatUserId,
  issuableCountry,
  normaliseUserId,
  userIdCountry,
  validUserId,
} from './alias.ts';

/**
 * Placement: whether an account is inside a country's perimeter, and what it
 * costs while it is not. Every account on this chain is country-gated, and an
 * unplaced one holds no user ID — so nobody can address a payment to it.
 */
export {
  countryName,
  countryProblem,
  placementRequest,
  placementVerdict,
  type Placement,
  type PlacementRequest,
  type PlacementState,
  type PlacementVerdict,
} from './placement.ts';

export {
  addressHue,
  addressLabel,
  formatDecimal,
  formatDuration,
  formatNumber,
  formatPercent,
  formatTimestamp,
  timeAgo,
  timeUntil,
  truncateAddress,
  truncateHash,
} from './format.ts';

export {
  decodeMessage,
  describeProposalAction,
  moduleOf,
  shortTypeUrl,
  summariseTransaction,
  type DecodeContext,
  type DecodedDetail,
  type DecodedMessage,
  type MessageKind,
} from './decode.ts';

export { describeTxResult, translateError, type TranslatedError } from './errors.ts';

export {
  describeThroughput,
  finality,
  headroom,
  measure,
  type BlockSample,
  type Performance,
} from './performance.ts';

export {
  ROLE_LABELS,
  checkSpend,
  committed,
  policyRefusals,
  spendable,
  toRoleAssignment,
  toSpendPolicy,
  toTreasury,
  toTreasuryBalance,
  type RoleAssignment,
  type SpendCapacity,
  type SpendPolicy,
  type Treasury,
  type TreasuryBalance,
} from './treasury.ts';

/**
 * What signing something will do to the account that signs it — as distinct
 * from what is in the bytes, which is `signrequest.ts` above.
 */
export {
  REVERSIBILITY,
  REVERSIBILITY_NOTE,
  arriving,
  consequencesOf,
  leaving,
  locking,
  totalCoins,
  type AuthorityEffect,
  type LockEffect,
  type Movement,
  type Reversibility,
  type SigningConsequences,
} from './consequence.ts';

export {
  describeCase,
  isOpen,
  requiredPower,
  toEnforcementCase,
  type CaseStatus,
  type EnforcementCase,
  type EnforcementVote,
} from './enforcement.ts';

export {
  amountOf,
  minimumReceived,
  quoteSwap,
  toPool,
  toProposal,
  toValidator,
  withTally,
  type Pool,
  type Proposal,
  type ProposalStatus,
  type StakingOverview,
  type Validator,
} from './staking.ts';

export {
  DEFAULT_GOV_PARAMS,
  assessProposal,
  toGovParams,
  type GovParams,
  type ProposalAssessment,
} from './gov.ts';

export {
  DEFAULT_MAX_APPRAISAL_AGE,
  DEFAULT_MAX_RATE_AGE,
  comparePoolToRates,
  describeFreshness,
  freshnessOf,
  toAppraisal,
  toRate,
  valueAt,
  type Appraisal,
  type Freshness,
  type PoolComparison,
  type Rate,
} from './prices.ts';

export {
  ChainClient,
  ChainError,
  type Block,
  type ChainClientOptions,
  type ChainStatus,
  type Commitment,
  type Transaction,
  type TreasuryLock,
  type TxStatus,
} from './client.ts';

export { CHAIN_MESSAGE_TYPES, chainRegistry } from './registry.ts';

export {
  ChainSigner,
  beginUnbonding,
  claimLock,
  claimRewards,
  createTreasury,
  delegate,
  depositToTreasury,
  emergencyFreeze,
  emergencyRelease,
  grantFeeAllowance,
  openCase,
  payment,
  revokeFeeAllowance,
  send,
  appointRegulator,
  grantAuditor,
  registerAlias,
  registerViewingKey,
  revokeViewingKey,
  rotateAlias,
  setJurisdiction,
  setPayloadStore,
  submitRates,
  swap,
  sweepCase,
  treasurySpend,
  vote,
  voteCase,
  withdrawCase,
  type CaseAction,
  type CaseVote,
  type EncodeObject,
  type SignerOptions,
  type SubmitResult,
  type VoteOption,
} from './signing.ts';

export {
  availableWallets,
  connect,
  describeChain,
  type ChainInfo,
  type WalletProvider,
} from './wallet.ts';

/**
 * The address book, and the one rule every interface uses to name an account.
 * Local to the device by design — see docs/guides/identity.md.
 */
export {
  displayName,
  listContacts,
  getContact,
  saveContact,
  removeContact,
  exportBook,
  importBook,
  onAddressBookChange,
  type Contact,
  type DisplayName,
  type UserIdLookup,
} from './addressbook.ts';

/**
 * The ISO 20022 payment detail that no longer goes on the chain. The payload
 * lives off-chain; the chain records only a hash of it, which is what lets a
 * party prove which payload a payment carried.
 */
export {
  METADATA_HASH_BYTES,
  METADATA_SALT_BYTES,
  loadPaymentMetadata,
  metadataHash,
  newPaymentMetadata,
  savePaymentMetadata,
  verifyMetadata,
  type PaymentMetadataPayload,
} from './metadata.ts';

/**
 * The encrypted form of that payload, sealed to the viewing keys of the payer,
 * the payee, the regulator of the declared settlement jurisdiction and any live
 * auditor. The chain never sees it; a participant's payload store holds it.
 */
export {
  ENVELOPE_VERSION,
  VIEWING_KEY_BYTES,
  EnvelopeMalformedError,
  EnvelopeUnreadableError,
  decodeEnvelope,
  encodeEnvelope,
  keyId,
  openPayload,
  openRecipientBlock,
  paymentAad,
  sealPayload,
  sealToViewingKey,
  type EnvelopeRecipient,
} from './envelope.ts';

/**
 * Retrieving that payload, and the vocabulary for when there is none.
 *
 * Every way this can end is a named outcome — erased, never stored, no store
 * registered, not entitled, unreachable — and none of them is an empty payload.
 * A payment whose detail was destroyed must render as detail being unavailable,
 * never as a record with nothing in it.
 */
export {
  PayloadStoreClient,
  describeUnavailable,
  newViewingKey,
  sealForStore,
  viewingKeyPublic,
  type PayloadResult,
  type PayloadStoreOptions,
  type PayloadUnavailableReason,
} from './payloadstore.ts';

/**
 * Naming an account with the chain's tier included. React-only, so it lives in
 * its own module rather than being imported by the plain SDK entry points.
 */
export { useDisplayName, AccountName } from './name.tsx';

export {
  t,
  plural,
  setLocale,
  getLocale,
  resolveLocale,
  register,
  direction,
  formatMoney,
  formatDate,
  LANGUAGES,
} from './i18n.ts';
export type { Locale, LanguageInfo, Catalogue } from './i18n.ts';
export { registerAll, AVAILABLE } from './catalogues.ts';
export { LanguagePicker, useLocale } from './language.tsx';
