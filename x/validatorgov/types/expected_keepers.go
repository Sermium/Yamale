package types

import (
	"context"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	authz "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	aliastypes "yamale/blockchain/x/alias/types"
	constitutiontypes "yamale/blockchain/x/constitution/types"
)

// AliasKeeper is the recorded side of the jurisdiction question: what has the
// chain been told about this account, and by whom.
//
// This module already collects a jurisdiction — the validator declares one when
// it applies, signs for it, and the concentration ceilings group by it. x/alias
// holds the other one, written by the approved participant that onboarded the
// account and did the know-your-customer work, or corrected by a foundation
// administrator, and never by the account itself. The two are kept apart and
// reconciled rather than merged; see the JurisdictionReconciliation query for
// why neither is the other's source.
//
// It returns the raw registry record and not x/alias's CountryOf, and that
// choice is the whole of the reconciliation. CountryOf answers a different
// question — "which country does this account belong to", the one the perimeter
// checks ask — and to answer it always, it falls back to the foundation's
// reserved code for a named administrator nobody has placed. That fallback is
// derived from a parameter list: no recorder stands behind it and no height
// dates it. Reporting it as the recorded side would have this module publish a
// provenance that does not exist, which is precisely the failure a
// reconciliation is built to expose rather than to commit.
//
// So found is false when there is no record, reported as a value rather than as
// an error, because "nobody has placed this account" is an answer the question
// has. It is never folded into agreement.
//
// One method and it only reads. An interface wide enough to write through would
// let the module that collects self-declared jurisdictions go and confirm them
// itself, which would turn two registries with different provenance into one
// with none. The direction is acyclic: x/alias knows nothing about this module.
type AliasKeeper interface {
	JurisdictionOf(ctx context.Context, address string) (record aliastypes.Jurisdiction, found bool, err error)
}

// ConstitutionKeeper is where the concentration ceilings come from.
//
// One method, and it only reads. The ceilings are held in another module
// precisely so that this one cannot change them: an interface wide enough to
// write through would put the values a validator set is bounded by inside the
// module that set is admitted through.
//
// The direction also keeps the wiring acyclic. x/constitution depends on
// x/staking and nothing else on this chain, so both this module and
// x/enforcement can consult it without depinject having a cycle to resolve.
type ConstitutionKeeper interface {
	GetInvariants(ctx context.Context) (constitutiontypes.Invariants, error)
}

// StakingKeeper defines the expected interface for the Staking module.
type StakingKeeper interface {
	ConsensusAddressCodec() address.Codec
	ValidatorByConsAddr(context.Context, sdk.ConsAddress) (stakingtypes.ValidatorI, error)

	// GetValidator resolves the validator record behind an operator address.
	// A rotation may name an operator that was approved but never created a
	// validator, so the absence of a record is an ordinary outcome here rather
	// than an error.
	GetValidator(context.Context, sdk.ValAddress) (stakingtypes.Validator, error)

	// Jail and Unjail are how a contested validator is paused: removed from the
	// active set without being slashed and without its delegations unbonding.
	// Both panic in x/staking if the validator is already in the state being
	// asked for, so the module checks before it calls.
	Jail(context.Context, sdk.ConsAddress) error
	Unjail(context.Context, sdk.ConsAddress) error

	// GetBondedValidatorsByPower is the active set the concentration ceilings
	// are measured over. Taken from x/staking rather than from this module's
	// own approval list on purpose: the check has to see the power a validator
	// actually carries, however it arrived — by growth, by merger, or by a
	// governance proposal that set it — and the approval list would only show
	// what this module last agreed to.
	GetBondedValidatorsByPower(context.Context) ([]stakingtypes.Validator, error)

	// BondDenom, Delegate and the two unbonding calls are how a governance
	// decision about a validator's power becomes consensus power.
	//
	// There is no other way. Cosmos derives power from bonded tokens and lets
	// exactly one module report validator updates, so a seat has to be a
	// quantity of stake rather than a number this module publishes. Seats are
	// therefore moved between the module's own reserve and the validator, which
	// changes power without minting anything and without writing into
	// x/staking's accounting behind its back.
	BondDenom(context.Context) (string, error)
	Delegate(ctx context.Context, delAddr sdk.AccAddress, bondAmt math.Int, tokenSrc stakingtypes.BondStatus, validator stakingtypes.Validator, subtractAccount bool) (math.LegacyDec, error)
	ValidateUnbondAmount(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, amt math.Int) (math.LegacyDec, error)
	Undelegate(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, sharesAmount math.LegacyDec) (time.Time, math.Int, error)
}

// AuthzKeeper defines the expected interface for the Authz module.
//
// A completed rotation grants the incoming operator authorisation over the
// messages that operate the validator. There is no other way: x/staking keys a
// validator record — and every delegation pointing at it — by the operator
// address it was created with, and the SDK has no operation that re-keys one.
// Granting is what moves who signs while leaving the stake exactly where the
// guide says it stays.
type AuthzKeeper interface {
	SaveGrant(ctx context.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
}

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI // only used for simulation

	// GetModuleAddress resolves the module's own seat reserve — the account
	// that holds the undelegated seats and is the only delegator this module
	// ever acts as.
	GetModuleAddress(name string) sdk.AccAddress
}

// BankKeeper defines the expected interface for the Bank module.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins

	// GetBalance answers what the seat reserve can still fund. Asked before a
	// delegation rather than after it fails, so that a governance proposal to
	// raise a validator's power is refused with the reason rather than with an
	// insufficient-funds error from three modules away.
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
