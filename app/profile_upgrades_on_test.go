//go:build !settlement

package app

import (
	"context"
	"testing"

	"cosmossdk.io/collections"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	herdtypes "yamale/blockchain/x/herd/types"
	jeweltypes "yamale/blockchain/x/jewel/types"
	markettypes "yamale/blockchain/x/market/types"
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

/* ========================================================================== */
/*  syndicate-documents                                                       */
/* ========================================================================== */

// The deal-papers upgrade adds NO store, and that is the assertion.
//
// The documents live in collections under x/syndicate's existing store key, so
// mounting a second one would be mounting a store nothing writes to — and an
// entry in StoreUpgrades.Added for a key that is already mounted is not
// harmless. What makes this a coordinated halt at all is the message: a
// validator on the old binary cannot decode MsgAttachDocument.
func TestTheDocumentsUpgradeAddsNoStore(t *testing.T) {
	u := findUpgrade(t, "syndicate-documents")
	require.NotNil(t, u.Handler)
	require.Empty(t, u.StoreUpgrades.Added,
		"documents live under x/syndicate's existing store key; nothing new is mounted")
	require.Empty(t, u.StoreUpgrades.Renamed)
	require.Empty(t, u.StoreUpgrades.Deleted)
}

// After the upgrade the new collections must ANSWER, and answer empty.
//
// Answering proves they were built into the keeper's schema rather than
// silently absent — an unregistered collection reads as a zero value and fails
// at the first write, in a block, rather than here. Empty proves the handler
// invented no attestation: a document's worth is the accountability of whoever
// attested to it, and nobody attested to anything during a halt.
func TestDocumentsArriveEmptyAndTheirCounterStartsAtZero(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	first := findUpgrade(t, "herd-syndicate-selfcustody")
	_, err := first.Handler(ctx, app, priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName))
	require.NoError(t, err)

	u := findUpgrade(t, "syndicate-documents")
	_, err = u.Handler(ctx, app, app.ModuleManager.GetVersionMap())
	require.NoError(t, err)

	documents := 0
	require.NoError(t, app.SyndicateKeeper.Document.Walk(ctx, nil,
		func(uint64, syndicatetypes.Document) (bool, error) { documents++; return false, nil }))
	require.Zero(t, documents)

	next, err := app.SyndicateKeeper.NextDocumentID.Peek(ctx)
	require.NoError(t, err)
	require.Zero(t, next)
}

// And it must refuse a document that arrived without anybody attesting to it.
//
// Without this the test above only checks that DefaultGenesis is empty, which
// is the thing most likely to drift under it.
func TestTheDocumentsUpgradeRefusesAPaperNobodyAttestedTo(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	first := findUpgrade(t, "herd-syndicate-selfcustody")
	_, err := first.Handler(ctx, app, priorVersions(app,
		herdtypes.ModuleName, syndicatetypes.ModuleName, selfcustodytypes.ModuleName))
	require.NoError(t, err)

	require.NoError(t, app.SyndicateKeeper.Document.Set(ctx, 1, syndicatetypes.Document{
		Id: 1, DealId: 1, Kind: syndicatetypes.DOCUMENT_KIND_TERM_SHEET,
		Title: "Attested By Nobody", Sha256: make([]byte, 32),
	}))

	u := findUpgrade(t, "syndicate-documents")
	_, err = u.Handler(ctx, app, app.ModuleManager.GetVersionMap())
	require.Error(t, err, "a document written during a halt must stop the upgrade")
	require.Contains(t, err.Error(), "not written during a halt")
}

// The market upgrade, which is the one that mounts a store and then depends on
// something that is NOT in its handler.

// A new module means a new store, and a store missing here is not a cosmetic
// omission: the module’s InitGenesis writes into a store the app never mounted,
// and the failure surfaces at the halt height on every validator at once.
func TestTheMarketAddsItsStore(t *testing.T) {
	u := findUpgrade(t, "market")
	require.Contains(t, u.StoreUpgrades.Added, markettypes.StoreKey)
}

// The market arrives able to match, and having traded nothing.
//
// max_fills_per_order is the assertion that is not ceremonial. At zero no
// incoming order could ever cross another: every order would rest, the book
// would fill with orders overlapping in price that never trade, and the market
// would LOOK like it worked — orders accepted, book deep — while being a machine
// for escrowing property and giving it straight back. A market that refuses
// orders is visibly broken; one that matches none is not.
func TestTheMarketArrivesAbleToMatchAndHavingTradedNothing(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	u := findUpgrade(t, "market")
	_, err := u.Handler(ctx, app, priorVersions(app, markettypes.ModuleName))
	require.NoError(t, err)

	params, err := app.MarketKeeper.Params.Get(ctx)
	require.NoError(t, err)
	require.NotZero(t, params.MaxFillsPerOrder,
		"a book that can never cross accepts every order and matches none")

	// Ids start at 1, so the sequences start at 0. A counter arriving part-used
	// would mean the handler wrote an order during the halt — an offer nobody
	// made, escrowed with property nobody committed.
	for _, peek := range []func(ctx2 context.Context) (uint64, error){
		app.MarketKeeper.NextListingID.Peek,
		app.MarketKeeper.NextAuctionID.Peek,
		app.MarketKeeper.NextOrderID.Peek,
	} {
		next, err := peek(ctx)
		require.NoError(t, err)
		require.Zero(t, next)
	}
}

// The escrow is declared to x/tokenisation by the BINARY, not by the upgrade.
//
// This is the property worth a test of its own, because its failure is silent.
// x/tokenisation carries a sender’s cost basis along with the shares on every
// transfer; across a marketplace escrow that is wrong twice over, and the
// symptom is not a crash but a chain that trades correctly and reports every
// holder’s cost wrong. Nothing in the upgrade handler can check it, so it is
// checked here against a fully wired app.
func TestTheMarketEscrowIsRegisteredWithTheRegister(t *testing.T) {
	app := newUpgradeTestApp(t)

	require.True(t,
		app.TokenisationKeeper.IsEscrow(app.MarketKeeper.ModuleAddress()),
		"app.registerMarketEscrow must have run; without it a holder who withdraws "+
			"an offer nobody took comes back with their cost basis spent")
}

func TestTheJewelRegisterAddsItsStore(t *testing.T) {
	u := findUpgrade(t, "jewel-register")
	require.Contains(t, u.StoreUpgrades.Added, jeweltypes.StoreKey)
}

// The upgrade closes x/nft's own send in the same block the module arrives.
// Left open for even one block, a jewel token could move outside x/jewel; and
// x/jewel refuses to admit any jeweler while it is open, so without this no
// jeweler could ever be admitted.
func TestTheJewelRegisterClosesTheBareNFTSendAndAppointsNobody(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})
	if err := app.CircuitBreakerKeeper.DisableList.Remove(ctx, jeweltypes.NFTSendTypeURL); err != nil {
		require.ErrorIs(t, err, collections.ErrNotFound)
	}

	u := findUpgrade(t, "jewel-register")
	_, err := u.Handler(ctx, app, priorVersions(app, jeweltypes.ModuleName))
	require.NoError(t, err)

	closed, err := app.CircuitBreakerKeeper.DisableList.Has(ctx, jeweltypes.NFTSendTypeURL)
	require.NoError(t, err)
	require.True(t, closed)

	params, err := app.JewelKeeper.Params.Get(ctx)
	require.NoError(t, err)
	require.Empty(t, params.Office, "the office is appointed by a vote, never by a migration")
	want := jeweltypes.DefaultParams()
	require.Equal(t, want, params, "InitGenesis must have run: fromVM must not pre-set the module")
}

// An upgrade that found an office already appointed would be installing an
// authority nobody voted for; it refuses rather than carry on.
func TestTheJewelRegisterRefusesAnOfficeItDidNotAppoint(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})
	p := jeweltypes.DefaultParams()
	p.Office = "yml1dlszg2sst9r69my4f84l3mj66zxcf3umcgujys30t84srg95dgvsrmuayr"
	require.NoError(t, app.JewelKeeper.Params.Set(ctx, p))

	u := findUpgrade(t, "jewel-register")
	_, err := u.Handler(ctx, app, priorVersions(app)) // jewel already at its version: no InitGenesis
	require.ErrorContains(t, err, "office")
}
