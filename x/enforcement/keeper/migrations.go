package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
	"yamale/blockchain/x/internal/legacyparams"
)

// legacyEmergencyAuthorityField is the number Params.emergency_authority carried
// before it was reserved. Named rather than written as an 8 at the call site,
// because a bare number in a wire-format read is the one place a typo produces a
// plausible answer instead of an error.
const legacyEmergencyAuthorityField = 8

// Migrator runs the module's state migrations.
type Migrator struct{ keeper Keeper }

// NewMigrator returns a Migrator for the keeper.
func NewMigrator(k Keeper) Migrator { return Migrator{keeper: k} }

// Migrate1to2 drops the retired emergency_authority from the stored parameters.
//
// It rewrites the parameters through the current type, which discards the
// reserved field — exactly the decoder behaviour that makes
// LegacyEmergencyAuthority necessary, used here on purpose. Without it the dead
// bytes would sit in the store until the next MsgUpdateParams, and an operator
// comparing the raw value against an export would get two different answers to
// what this module's parameters are.
//
// # What it deliberately does not do
//
// It does not grant the retired authority its replacement role. That grant lives
// in x/alias, and this module has no edge into x/alias — deliberately, so that
// the perimeter cannot be widened from inside the module it constrains. The
// upgrade handler in app/upgrades.go holds both keepers and does it there,
// reading the address through LegacyEmergencyAuthority BEFORE this migration
// runs, because afterwards it is gone.
//
// That split means this migration is safe to run twice and safe to run on a
// chain that never had an emergency authority, which is the property a migration
// should have. It also means the grant is the upgrade's act rather than the
// module's, which is the truthful attribution: no message in this module has
// ever been able to write a role grant.
func (m Migrator) Migrate1to2(ctx context.Context) error {
	authority, found, err := m.keeper.LegacyEmergencyAuthority(ctx)
	if err != nil {
		return err
	}

	params, err := m.keeper.Params.Get(ctx)
	if err != nil {
		return fmt.Errorf("reading the enforcement parameters to rewrite: %w", err)
	}
	if err := m.keeper.Params.Set(ctx, params); err != nil {
		return err
	}

	if found {
		sdk.UnwrapSDKContext(ctx).Logger().With("module", "x/"+types.ModuleName).Info("the emergency authority is a role grant now",
			"was", authority,
			"consequence", "MsgEmergencyFreeze and MsgEmergencyRelease are authorised by "+
				"ROLE_ENFORCEMENT_AUTHORITY covering the target's country, not by a parameter",
		)
	}
	return nil
}

// LegacyEmergencyAuthority reads the retired emergency_authority parameter out of
// the raw stored bytes.
//
// Exported because the caller that needs it is not in this module: the upgrade
// handler has to read it before the module migrations run and then write the
// replacement grant into x/alias, which this module cannot reach.
//
// The boolean is not decoration. An empty emergency authority meant "nobody" —
// the parameter's whole careful property was that an unset value never meant
// "anybody" — so a caller has to be able to tell an address that was never set
// from one that was set to the empty string, and grant nothing in either case
// rather than granting to "".
func (k Keeper) LegacyEmergencyAuthority(ctx context.Context) (string, bool, error) {
	raw, err := k.storeService.OpenKVStore(ctx).Get(types.ParamsKey.Bytes())
	if err != nil {
		return "", false, fmt.Errorf("reading the enforcement parameters: %w", err)
	}
	authority, found, err := legacyparams.Last(raw, legacyEmergencyAuthorityField)
	if err != nil {
		return "", false, fmt.Errorf("reading emergency_authority out of the stored enforcement parameters: %w", err)
	}
	if authority == "" {
		return "", false, nil
	}
	return authority, found, nil
}
