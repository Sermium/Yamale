package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/internal/legacyparams"
)

// legacyFoundationAdministratorsField is the number Params.foundation_administrators
// carried before it was reserved. Named rather than written as a 2 at the call
// site, because a bare number in a wire-format read is the one place a typo
// produces a plausible answer instead of an error.
const legacyFoundationAdministratorsField = 2

// Migrator runs the module's state migrations.
type Migrator struct{ keeper Keeper }

// NewMigrator returns a Migrator for the keeper.
func NewMigrator(k Keeper) Migrator { return Migrator{keeper: k} }

// Migrate1to2 retires every identifier issued before identifiers carried a
// country.
//
// The devnet has aliases with no prefix and no jurisdiction behind them. There
// are three things to do with them and only one that is honest.
//
// Leaving them is out: a prefixless identifier is an account inside no
// perimeter, holding a handle that says nothing about where it is, and every
// authority query would have to carry an "and these" clause forever. That is
// the migration limbo the design refuses — a perimeter that is advisory for an
// unbounded set of accounts is not a perimeter.
//
// Guessing a country for them is worse. There is no source of truth to guess
// from: the jurisdiction registry is what this migration introduces, so any
// prefix written here would be invented, and an invented prefix is exactly the
// lie the feature exists to make impossible. It would also be indistinguishable
// afterwards from one a participant actually attested to.
//
// So they are tombstoned. Every old identifier stops resolving at the upgrade
// height, and the account re-registers once its participant has recorded where
// it is. Tombstoned rather than deleted, because somebody has an old handle
// written down: "this was given up" is a truthful answer to them and "never
// existed" is not, and the Retired query is there precisely to tell those two
// apart before somebody sends money.
//
// The jurisdiction registry is left empty on purpose. Populating it is the
// participants' act, not the migration's — see the module's ParticipantKeeper
// for why the party that did the KYC is the only one that may write it.
func (m Migrator) Migrate1to2(ctx context.Context) error {
	var stale []types.Alias
	if err := m.keeper.Aliases.Walk(ctx, nil, func(_ string, a types.Alias) (bool, error) {
		stale = append(stale, a)
		return false, nil
	}); err != nil {
		return err
	}

	for _, a := range stale {
		// Everything issued under v1 is prefixless, and nothing prefixless can
		// ever be issued again — every identifier now begins with a country. The
		// tombstone is still written, because it is what turns a resolution
		// failure into an answer.
		if err := m.keeper.retire(ctx, a.Id, a.Address); err != nil {
			return err
		}
		m.keeper.Logger().Info("retired a pre-jurisdiction identifier",
			"id", a.Id, "address", a.Address)
	}
	return nil
}

// Migrate2to3 carries the foundation administrators out of the parameters and
// into the grant registry.
//
// Every address in the retired foundation_administrators parameter becomes a
// chain-wide grant of ROLE_FOUNDATION_ADMINISTRATOR, and the parameters are
// rewritten without the field.
//
// # Why this cannot be skipped
//
// The live chain has an administrator. It is the only account that may correct
// the country recorded against another account, appoint a country's regulator or
// grant the auditor role — and after the upgrade those three paths read a grant
// instead of a list. An upgrade that ran the code change without this would
// leave a chain whose parameters still named somebody and whose handlers no
// longer looked there: the appointment would read as done in every query and
// confer nothing, which is the exact failure mode this repository has hit
// before. The visible symptom would be a foundation that cannot fix a mistyped
// country, discovered at the moment somebody needs it fixed.
//
// # Why the value has to be read as bytes
//
// Field 2 is reserved in v3, and a reserved field is one the generated Go type
// no longer has. This build's gogoproto does not keep unknown fields either, so
// unmarshalling the stored parameters with the current type drops the addresses
// silently — with no error, because dropping an unknown field is what a protobuf
// decoder is supposed to do. So the raw bytes are read and field 2 is scanned
// out of them by x/internal/legacyparams. That is more code than a struct field
// would have been, and it is the honest amount of code for the situation: the
// alternative is a second generated message that exists only to be read once and
// then sits in the reference documentation forever.
//
// # What is deliberately not checked
//
// The holders are not required to be x/group accounts, and no required_shape is
// recorded. Both would be true of a grant made today and neither is true of the
// administrators this chain already has, which were bare addresses in a
// parameter list. A migration that applied the new rule would not be carrying an
// authority across, it would be deleting one and calling the deletion a
// standard — and the account it deleted is the one that can correct a country.
// Tightening them is a foundation proposal per grant, afterwards, deliberately.
//
// The cap is not enforced here either, for the same reason: the parameter's own
// Validate refused a ninth entry, so a chain cannot be carrying more than eight,
// and if one somehow were, halting every node on an upgrade is a worse answer
// than carrying the state that exists and letting the next grant be refused.
func (m Migrator) Migrate2to3(ctx context.Context) error {
	raw, err := m.keeper.storeService.OpenKVStore(ctx).Get(types.ParamsKey.Bytes())
	if err != nil {
		return fmt.Errorf("reading the alias parameters to migrate: %w", err)
	}

	administrators, err := legacyparams.Strings(raw, legacyFoundationAdministratorsField)
	if err != nil {
		return fmt.Errorf("reading foundation_administrators out of the stored alias parameters: %w", err)
	}

	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	authority := m.keeper.GetAuthority()
	for _, administrator := range administrators {
		grant := types.RoleGrant{
			Holder: administrator,
			Role:   types.ROLE_FOUNDATION_ADMINISTRATOR,
			// Chain-wide, because that is what the parameter was: an exemption
			// from belonging to any national perimeter. A country scope would be
			// refused by RoleGrant.Validate and would also be a narrowing this
			// migration has no standing to decide.
			Jurisdiction: types.ChainWide,
			// Attributed to governance, because governance is what appointed
			// them: the parameter was set by MsgUpdateParams and by nothing else.
			// Attributing it to the upgrade would lose the one fact granted_by
			// exists to record.
			GrantedBy:       authority,
			GrantedAtHeight: height,
		}
		if err := grant.Validate(); err != nil {
			return fmt.Errorf("the foundation administrator %q cannot be carried across as a grant: %w",
				administrator, err)
		}
		if err := m.keeper.grant(ctx, grant); err != nil {
			return err
		}
		m.keeper.Logger().Info("carried a foundation administrator into the grant registry",
			"holder", administrator, "role", types.RoleName(grant.Role), "jurisdiction", grant.Jurisdiction)
	}

	// Rewrite the parameters so the store stops carrying the retired field.
	//
	// Read through the current type and written back, which drops field 2 —
	// exactly the behaviour that made the raw read above necessary, used here on
	// purpose. Without it the dead bytes would sit in the store until the next
	// MsgUpdateParams, and an operator comparing the raw value against an export
	// would find two different answers to what the parameters are.
	params, err := m.keeper.Params.Get(ctx)
	if err != nil {
		return fmt.Errorf("reading the alias parameters to rewrite: %w", err)
	}
	if err := m.keeper.Params.Set(ctx, params); err != nil {
		return err
	}

	m.keeper.Logger().Info("the foundation administrators are role grants now",
		"carried", len(administrators),
		"consequence", "appointing one is MsgGrantRole with ROLE_FOUNDATION_ADMINISTRATOR and the chain-wide scope, "+
			"which is governance and nobody else, and the holder must be an x/group account",
	)
	return nil
}

// GrantForUpgrade writes a role grant with no signer and no authority check.
//
// It exists for one caller — an upgrade handler in app/upgrades.go — and the
// name says so rather than reading like an ordinary keeper method, because a
// method that writes a grant without asking who is granting it is precisely the
// thing the rest of this module exists to prevent.
//
// The reason it has to exist at all is a module boundary. x/enforcement retired
// its emergency_authority parameter in favour of ROLE_ENFORCEMENT_AUTHORITY, and
// the address it held must survive the upgrade or the chain loses its emergency
// path — but grants live here and x/enforcement has no edge into this module,
// deliberately, so that the perimeter cannot be widened from inside the module
// it constrains. The upgrade handler is the one place that holds both keepers,
// and it is a place the chain reaches once, at a height governance voted for.
//
// It validates the grant and it writes both directions, so it cannot produce a
// record the ordinary paths would have refused or a reverse index that disagrees
// with the registry. What it does not do is check a signer, a group account, or
// a shape — the same three omissions Migrate2to3 makes, for the same reason: a
// migration that applied today's rules to yesterday's state would be deleting an
// authority rather than carrying it across.
func (k Keeper) GrantForUpgrade(ctx context.Context, g types.RoleGrant) error {
	if err := g.Validate(); err != nil {
		return fmt.Errorf("an upgrade tried to write a grant this module would refuse: %w", err)
	}
	return k.grant(ctx, g)
}
