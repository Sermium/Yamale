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
	"time"

	"cosmossdk.io/core/address"
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
}

func NewStakingKeeper() *StakingKeeper {
	return &StakingKeeper{
		byOperator: map[string]*stakingtypes.Validator{},
		byCons:     map[string]*stakingtypes.Validator{},
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
