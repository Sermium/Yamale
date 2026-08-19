package keeper

import (
	"context"

	"yamale/blockchain/x/alias/types"
)

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
