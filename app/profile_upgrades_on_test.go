//go:build !settlement

package app

import (
	"testing"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	herdtypes "yamale/blockchain/x/herd/types"
	selfcustodytypes "yamale/blockchain/x/selfcustody/types"
	syndicatetypes "yamale/blockchain/x/syndicate/types"
)

// The herd-syndicate-selfcustody upgrade, driven the way a real one is.
//
// `priorVersions(app, <the new modules>)` and not `GetVersionMap()`: the version
// map a real upgrade receives is the one from BEFORE the new modules existed, so
// removing them is what makes RunMigrations see something to do and call their
// InitGenesis. Handed the full map, these tests would pass against a handler
// that never ran the modules at all.

func TestTheThreeModulesAddTheirStores(t *testing.T) {
	u := findUpgrade(t, "herd-syndicate-selfcustody")
	require.NotNil(t, u.Handler, "the assertions in this upgrade live in its handler")

	// All three are new modules. A store missing here is not a cosmetic
	// omission: the module's InitGenesis writes into a store the app never
	// mounted, and the failure surfaces at the halt height on every validator
	// at once.
	for _, key := range []string{
		herdtypes.StoreKey, syndicatetypes.StoreKey, selfcustodytypes.StoreKey,
	} {
		require.Contains(t, u.StoreUpgrades.Added, key)
	}
}

// The assertion that is not ceremonial.
//
// Params.AcknowledgementHash is what every RequestDisclosure and
// ConfirmDisclosure is checked against. A chain publishing a hash this binary's
// embedded text does not produce refuses every holder — nobody can take their
// own key — which is exactly the failure x/selfcustody exists to prevent.
func TestTheDoorOutActuallyOpensAfterTheUpgrade(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "herd-syndicate-selfcustody")
	_, err := u.Handler(ctx, app, priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName))
	require.NoError(t, err)

	params, err := app.SelfCustodyKeeper.Params.Get(ctx)
	require.NoError(t, err, "the upgrade must leave the disclosure parameters readable")
	require.Equal(t, selfcustodytypes.AcknowledgementHash(), params.AcknowledgementHash,
		"the published hash must be the one this binary's embedded acknowledgement produces")

	// And the published terms must actually admit an acknowledgement, which is
	// the property a holder depends on rather than the hash itself.
	require.NoError(t,
		params.MatchesPublished(params.AcknowledgementVersion, params.AcknowledgementHash))
}

// And it must refuse, loudly, when the hash does not match.
//
// Without this the test above only checks DefaultParams, which is the thing most
// likely to drift under it. Here the handler is given a chain whose published
// hash is wrong and has to stop the upgrade rather than install a pathway no
// holder can use.
func TestTheUpgradeRefusesAnUnusableDisclosurePathway(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "herd-syndicate-selfcustody")
	vm := priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName)
	_, err := u.Handler(ctx, app, vm)
	require.NoError(t, err)

	// Break it the way a bad governance proposal or an edited document would.
	params, err := app.SelfCustodyKeeper.Params.Get(ctx)
	require.NoError(t, err)
	params.AcknowledgementHash =
		"0000000000000000000000000000000000000000000000000000000000000000"
	require.NoError(t, app.SelfCustodyKeeper.Params.Set(ctx, params))

	// The full version map this time, so RunMigrations does NOT re-run
	// InitGenesis and overwrite what was just written. That is also the
	// realistic shape of the hazard: the handler checks whatever the parameters
	// are once migrations have finished, whether a migration wrote them or they
	// were already there.
	_, err = u.Handler(ctx, app, app.ModuleManager.GetVersionMap())
	require.Error(t, err, "an acknowledgement hash nobody can match must halt the upgrade")
	require.Contains(t, err.Error(), "no holder could take")
}

// Neither commercial module may arrive with anybody already admitted.
//
// Admitting a platform that takes other people's money, or an office with
// authority over somebody's livestock, is a governance decision. An upgrade that
// seeded either would be installing authority nobody voted for.
func TestNeitherCommercialModuleArrivesWithAnybodyAdmitted(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "herd-syndicate-selfcustody")
	_, err := u.Handler(ctx, app, priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName))
	require.NoError(t, err)

	operators := 0
	require.NoError(t, app.SyndicateKeeper.Operator.Walk(ctx, nil,
		func(string, syndicatetypes.Operator) (bool, error) { operators++; return false, nil }))
	require.Zero(t, operators, "a syndication platform is admitted by a vote, not by a migration")

	authorities := 0
	require.NoError(t, app.HerdKeeper.Authority.Walk(ctx, nil,
		func(string, herdtypes.Authority) (bool, error) { authorities++; return false, nil }))
	require.Zero(t, authorities, "a registry office is admitted by a vote, not by a migration")
}

// And it must refuse if one somehow arrives admitted, for the same reason the
// netting test drives its handler against a chain that is already running.
func TestTheUpgradeRefusesAPreAdmittedOperator(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "herd-syndicate-selfcustody")
	vm := priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName)
	_, err := u.Handler(ctx, app, vm)
	require.NoError(t, err)

	require.NoError(t, app.SyndicateKeeper.Operator.Set(ctx, "yml1whoever", syndicatetypes.Operator{
		Address: "yml1whoever", Name: "Admitted By Nobody", Jurisdiction: "SN", Active: true,
	}))

	// The full version map this time, so RunMigrations does NOT re-run
	// InitGenesis and overwrite what was just written. That is also the
	// realistic shape of the hazard: the handler checks whatever the parameters
	// are once migrations have finished, whether a migration wrote them or they
	// were already there.
	_, err = u.Handler(ctx, app, app.ModuleManager.GetVersionMap())
	require.Error(t, err, "an operator nobody voted for must halt the upgrade")
	require.Contains(t, err.Error(), "admitted by a vote")
}
