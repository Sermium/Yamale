// Package testutil provides in-memory stand-ins for the two keepers
// x/validatorgov calls out to.
//
// They exist because the paths worth testing here are the ones with an effect
// outside the module: a contested validator being jailed and given back, and a
// completed rotation granting the incoming operator authority over the outgoing
// one's validator. A test against a nil keeper would assert that the module
// reached the right line, not that the right thing happened.
package testutil

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authz "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// StakingKeeper is an in-memory validator set.
type StakingKeeper struct {
	byOperator map[string]*stakingtypes.Validator
	byCons     map[string]*stakingtypes.Validator

	// delegated records what each delegator holds of each validator, keyed by
	// "delegator/validator". It exists so that lowering a power can only unbond
	// what the seat reserve actually put in.
	delegated map[string]math.Int
}

func NewStakingKeeper() *StakingKeeper {
	return &StakingKeeper{
		byOperator: map[string]*stakingtypes.Validator{},
		byCons:     map[string]*stakingtypes.Validator{},
		delegated:  map[string]math.Int{},
	}
}

// AddValidator creates a validator for an operator account address, with a
// fresh consensus key, and returns it.
func (s *StakingKeeper) AddValidator(operator sdk.AccAddress) stakingtypes.Validator {
	pubKey := ed25519.GenPrivKey().PubKey()
	validator, err := stakingtypes.NewValidator(sdk.ValAddress(operator).String(), pubKey, stakingtypes.Description{})
	if err != nil {
		panic(err)
	}

	consAddr, err := validator.GetConsAddr()
	if err != nil {
		panic(err)
	}

	s.byOperator[sdk.ValAddress(operator).String()] = &validator
	s.byCons[sdk.ConsAddress(consAddr).String()] = &validator
	return validator
}

func (s *StakingKeeper) GetValidator(_ context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error) {
	validator, ok := s.byOperator[addr.String()]
	if !ok {
		return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
	}
	return *validator, nil
}

// Jail mirrors x/staking, which panics rather than tolerating a double jail.
// Reproducing that is the point: the module's guard against re-jailing an
// already-jailed validator is only tested if getting it wrong is fatal here
// too.
func (s *StakingKeeper) Jail(_ context.Context, consAddr sdk.ConsAddress) error {
	validator, ok := s.byCons[consAddr.String()]
	if !ok {
		panic(fmt.Sprintf("validator record not found for consensus address %s", consAddr))
	}
	if validator.Jailed {
		panic("cannot jail already jailed validator")
	}
	validator.Jailed = true
	// Mirrors x/staking, which drops a jailed validator out of the power index
	// and unbonds it. Without this a demoted validator would still be returned
	// by GetBondedValidatorsByPower and counted in its own group at the next
	// epoch, so a demotion would look as though it had done nothing.
	validator.Status = stakingtypes.Unbonding
	return nil
}

// Unjail mirrors x/staking, which panics on un-jailing a validator that is not
// jailed.
func (s *StakingKeeper) Unjail(_ context.Context, consAddr sdk.ConsAddress) error {
	validator, ok := s.byCons[consAddr.String()]
	if !ok {
		panic(fmt.Sprintf("validator record not found for consensus address %s", consAddr))
	}
	if !validator.Jailed {
		panic("cannot unjail already unjailed validator")
	}
	validator.Jailed = false
	validator.Status = stakingtypes.Bonded
	return nil
}

// IsJailed reports whether the validator behind an operator account address is
// jailed. False when there is no validator at all.
func (s *StakingKeeper) IsJailed(operator sdk.AccAddress) bool {
	validator, ok := s.byOperator[sdk.ValAddress(operator).String()]
	return ok && validator.Jailed
}

func (s *StakingKeeper) ConsensusAddressCodec() address.Codec {
	return addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix())
}

func (s *StakingKeeper) ValidatorByConsAddr(_ context.Context, consAddr sdk.ConsAddress) (stakingtypes.ValidatorI, error) {
	validator, ok := s.byCons[consAddr.String()]
	if !ok {
		return nil, stakingtypes.ErrNoValidatorFound
	}
	return *validator, nil
}

// Grant is one authorisation recorded by AuthzKeeper.
type Grant struct {
	Grantee    string
	Granter    string
	MsgTypeURL string
	Expiration *time.Time
}

// AuthzKeeper records the grants a completed rotation writes.
type AuthzKeeper struct {
	Grants []Grant
}

func NewAuthzKeeper() *AuthzKeeper { return &AuthzKeeper{} }

func (a *AuthzKeeper) SaveGrant(_ context.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error {
	a.Grants = append(a.Grants, Grant{
		Grantee:    grantee.String(),
		Granter:    granter.String(),
		MsgTypeURL: authorization.MsgTypeURL(),
		Expiration: expiration,
	})
	return nil
}

// GrantedTypes returns the message type URLs granted from granter to grantee.
func (a *AuthzKeeper) GrantedTypes(grantee, granter string) []string {
	types := make([]string, 0, len(a.Grants))
	for _, grant := range a.Grants {
		if grant.Grantee == grantee && grant.Granter == granter {
			types = append(types, grant.MsgTypeURL)
		}
	}
	return types
}

// The concentration ceilings need a validator set with power in it, and a
// governance path that moves that power. Both live here rather than against a
// real x/staking because the states worth testing — a validator that grew after
// admission, two operators behind one owner, a set one member above the floor —
// are three lines to arrange here and a bonding ceremony there.

// bondDenom is what this stand-in calls the staking denomination. It only has
// to be consistent with itself; nothing in x/validatorgov reads its name.
const bondDenom = "uyml"

// AddValidatorWithSeats creates a bonded validator holding the given number of
// seats, where one seat is one unit of consensus power.
func (s *StakingKeeper) AddValidatorWithSeats(operator sdk.AccAddress, seats int64) stakingtypes.Validator {
	s.AddValidator(operator)
	s.SetSeats(operator, seats)
	return *s.byOperator[sdk.ValAddress(operator).String()]
}

// SetSeats sets a validator power directly, which is how a test arranges the
// state an acquisition or a governance proposal would have produced.
func (s *StakingKeeper) SetSeats(operator sdk.AccAddress, seats int64) {
	validator, ok := s.byOperator[sdk.ValAddress(operator).String()]
	if !ok {
		panic(fmt.Sprintf("no validator for operator %s", operator))
	}
	validator.Tokens = sdk.DefaultPowerReduction.MulRaw(seats)
	validator.DelegatorShares = math.LegacyNewDecFromInt(validator.Tokens)
	if !validator.Jailed {
		validator.Status = stakingtypes.Bonded
	}
}

// Seats reports a validator power, jailed or not.
func (s *StakingKeeper) Seats(operator sdk.AccAddress) int64 {
	validator, ok := s.byOperator[sdk.ValAddress(operator).String()]
	if !ok {
		return 0
	}
	return validator.PotentialConsensusPower(sdk.DefaultPowerReduction)
}

// GetBondedValidatorsByPower mirrors x/staking: bonded validators only, largest
// first, ties broken by operator address.
func (s *StakingKeeper) GetBondedValidatorsByPower(context.Context) ([]stakingtypes.Validator, error) {
	out := make([]stakingtypes.Validator, 0, len(s.byOperator))
	for _, validator := range s.byOperator {
		if validator.Status == stakingtypes.Bonded {
			out = append(out, *validator)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		pi := out[i].PotentialConsensusPower(sdk.DefaultPowerReduction)
		pj := out[j].PotentialConsensusPower(sdk.DefaultPowerReduction)
		if pi != pj {
			return pi > pj
		}
		return out[i].OperatorAddress < out[j].OperatorAddress
	})
	return out, nil
}

func (s *StakingKeeper) GetLastTotalPower(context.Context) (math.Int, error) {
	total := int64(0)
	for _, validator := range s.byOperator {
		if validator.Status == stakingtypes.Bonded {
			total += validator.PotentialConsensusPower(sdk.DefaultPowerReduction)
		}
	}
	return math.NewInt(total), nil
}

func (s *StakingKeeper) Validator(_ context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error) {
	validator, ok := s.byOperator[addr.String()]
	if !ok {
		return nil, stakingtypes.ErrNoValidatorFound
	}
	return *validator, nil
}

func (s *StakingKeeper) BondDenom(context.Context) (string, error) { return bondDenom, nil }

// Delegate adds tokens to a validator and records what this delegator holds of
// it, which is what x/staking does to the numbers the ceilings are computed
// over. It moves no coins: the reserve balance is checked against the real bank
// keeper before this is reached, and that check is the part worth testing.
func (s *StakingKeeper) Delegate(_ context.Context, delAddr sdk.AccAddress, bondAmt math.Int, _ stakingtypes.BondStatus, validator stakingtypes.Validator, _ bool) (math.LegacyDec, error) {
	stored, ok := s.byOperator[validator.OperatorAddress]
	if !ok {
		return math.LegacyDec{}, stakingtypes.ErrNoValidatorFound
	}
	stored.Tokens = stored.Tokens.Add(bondAmt)
	stored.DelegatorShares = math.LegacyNewDecFromInt(stored.Tokens)
	if !stored.Jailed {
		stored.Status = stakingtypes.Bonded
	}

	key := delAddr.String() + "/" + validator.OperatorAddress
	held, ok := s.delegated[key]
	if !ok {
		held = math.ZeroInt()
	}
	s.delegated[key] = held.Add(bondAmt)
	return math.LegacyNewDecFromInt(bondAmt), nil
}

// ValidateUnbondAmount refuses to unbond more than this delegator put in, which
// is the check that stops a power reduction from eating a validator own
// self-delegation.
func (s *StakingKeeper) ValidateUnbondAmount(_ context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, amt math.Int) (math.LegacyDec, error) {
	held, ok := s.delegated[delAddr.String()+"/"+valAddr.String()]
	if !ok || held.LT(amt) {
		return math.LegacyDec{}, stakingtypes.ErrNotEnoughDelegationShares
	}
	return math.LegacyNewDecFromInt(amt), nil
}

func (s *StakingKeeper) Undelegate(_ context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, shares math.LegacyDec) (time.Time, math.Int, error) {
	validator, ok := s.byOperator[valAddr.String()]
	if !ok {
		return time.Time{}, math.Int{}, stakingtypes.ErrNoValidatorFound
	}
	amount := shares.TruncateInt()
	validator.Tokens = validator.Tokens.Sub(amount)
	validator.DelegatorShares = math.LegacyNewDecFromInt(validator.Tokens)

	key := delAddr.String() + "/" + valAddr.String()
	s.delegated[key] = s.delegated[key].Sub(amount)
	return time.Time{}, amount, nil
}
