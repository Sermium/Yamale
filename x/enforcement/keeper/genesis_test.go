package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/enforcement/types"
)

// testRecoveryDestination is a valid address that is nobody's account. The
// value does not matter to these tests; that there *is* one does, because a
// genesis without a recovery destination is now refused outright and every
// test below would otherwise fail on that before reaching what it is about.
var testRecoveryDestination = sdk.AccAddress([]byte("foundation-test-addr")).String()

// otherDestination is a second valid address, for the tests that check a
// proposal cannot repoint where seized assets go.
var otherDestination = sdk.AccAddress([]byte("somebody-elses-addr")).String()

// startableGenesis is DefaultGenesis with the fields that have no default
// filled in — the same thing every setup script in scripts/ does.
//
// There are three of them now, and they are all the same kind of thing: a value
// this binary cannot guess for somebody else's network. The destination names
// an institution, and the delay tiers and the window cap name a currency. A
// default for any of them would be a live setting that looked configured and
// was not, which is worse than an absent one because it satisfies the check.
func startableGenesis() *types.GenesisState {
	genesis := types.DefaultGenesis()
	genesis.Params.RecoveryDestination = testRecoveryDestination
	genesis.Params.SeizureDelayTiers = []types.SeizureDelayTier{
		{Threshold: sdk.NewCoin("uyml", math.NewInt(1_000_000)), DelayBlocks: 100},
	}
	genesis.Params.SeizureWindowCap = sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(100_000_000)))
	return genesis
}

// A freeze that did not survive an export would hand every frozen account back
// at the next upgrade — quietly, at the one moment nobody is watching balances.
// This is the test that says it does.
func TestGenesisRoundTripsWithFreezesIntact(t *testing.T) {
	f := initFixture(t)
	one := f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.addValidator(t, 10)

	scammer, scammerStr := f.env.NewFundedAddr(t, coins(500_000))
	suspect, suspectStr := f.env.NewFundedAddr(t, coins(100_000))

	// One case still being voted on, one already passed.
	open, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: suspectStr, Action: types.CASE_ACTION_FREEZE, Reason: "under investigation",
	})
	require.NoError(t, err)
	_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{Voter: one, CaseId: open.Id, Option: types.VOTE_OPTION_YES})
	require.NoError(t, err)

	passed := f.openAndPassSeizure(t, scammerStr)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an export the chain would refuse to import is not an export")

	// A fresh chain, started from what the old one wrote.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reexported,
		"derived state must match what was imported, or every upgrade rewrites the record")

	// The freezes are the part that has to still work, not merely still exist.
	require.True(t, g.keeper.IsFrozen(g.ctx, scammerStr))
	require.True(t, g.keeper.IsFrozen(g.ctx, suspectStr))

	elsewhere, _ := g.env.Addr(t)
	g.env.Fund(t, scammer, coins(10))
	g.env.Fund(t, suspect, coins(10))
	require.ErrorIs(t, g.env.BankKeeper.SendCoins(g.ctx, scammer, elsewhere, coins(10)), types.ErrFrozen)
	require.ErrorIs(t, g.env.BankKeeper.SendCoins(g.ctx, suspect, elsewhere, coins(10)), types.ErrFrozen)

	// And the case that was still open still resolves on the schedule it was
	// opened with, rather than being restarted or abandoned.
	imported, err := g.keeper.Case.Get(g.ctx, open.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VOTING, imported.Status)

	g.atHeight(imported.VotingEndsAtHeight + 1)
	require.NoError(t, g.keeper.EndBlocker(g.ctx))

	resolved, err := g.keeper.Case.Get(g.ctx, open.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_EXPIRED, resolved.Status)
	require.False(t, g.keeper.IsFrozen(g.ctx, suspectStr), "expiring the case must release the account")

	// The passed seizure is untouched by that.
	require.True(t, g.keeper.IsFrozen(g.ctx, scammerStr))

	// Ids carry on from where they stopped rather than colliding with a case
	// that already exists.
	next, err := g.ms.OpenCase(g.ctx, &types.MsgOpenCase{
		Opener: g.addValidator(t, 10), Target: suspectStr, Action: types.CASE_ACTION_FREEZE, Reason: "again",
	})
	require.NoError(t, err)
	require.Greater(t, next.Id, passed)
}

func TestDefaultGenesisNumbersCasesFromOne(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, target := f.env.NewFundedAddr(t, coins(1))

	resp, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: target, Action: types.CASE_ACTION_FREEZE, Reason: "the first case",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), resp.Id,
		"a case id of zero is indistinguishable from an unset field, and nobody could look it up")
}

// Genesis validation is the last line before a chain starts with state that
// contradicts itself.
func TestGenesisRefusesAFreezeWithNoCase(t *testing.T) {
	genesis := startableGenesis()
	genesis.CaseCount = 2
	genesis.Freezes = []types.Freeze{{Address: "yml1nobody", CaseId: 1}}

	require.ErrorContains(t, genesis.Validate(), "does not exist")
}

func TestGenesisRefusesAFreezeFromAClosedCase(t *testing.T) {
	genesis := startableGenesis()
	genesis.CaseCount = 2
	genesis.Cases = []types.Case{{
		Id:     1,
		Target: "yml1target",
		Action: types.CASE_ACTION_FREEZE,
		Status: types.CASE_STATUS_REJECTED,
	}}
	genesis.Freezes = []types.Freeze{{Address: "yml1target", CaseId: 1}}

	require.ErrorContains(t, genesis.Validate(), "CASE_STATUS_REJECTED")
}

func TestGenesisRefusesACaseIdThatWouldCollide(t *testing.T) {
	genesis := startableGenesis()
	genesis.CaseCount = 1
	genesis.Cases = []types.Case{{
		Id:     1,
		Target: "yml1target",
		Action: types.CASE_ACTION_FREEZE,
		Status: types.CASE_STATUS_EXPIRED,
	}}

	require.ErrorContains(t, genesis.Validate(), "collide")
}

// The parameter that was found empty on a running chain.
//
// A seizure needs somewhere to send what it takes. With no destination, two
// thirds of the validator set could pass a case that then cannot be carried
// out — and the record would say assets were recovered. That state was reached
// once, on the devnet, and went unnoticed until a console happened to print
// the parameter. These tests are what makes reaching it again impossible.
func TestGenesisRefusesAnEmptyRecoveryDestination(t *testing.T) {
	genesis := types.DefaultGenesis()
	require.Equal(t, "", genesis.Params.RecoveryDestination,
		"the default has no destination, because no address compiled into the binary is anybody's foundation")

	require.ErrorContains(t, genesis.Validate(), "recovery_destination",
		"a genesis with nowhere to send seized funds must not validate")

	// And the gate that matters: not `genesis validate`, which an operator may
	// never run, but InitChain, on the bytes the chain is really starting from.
	f := initFixture(t)
	err := f.keeper.InitGenesis(f.ctx, *genesis)
	require.Error(t, err, "a chain must not start with no recovery destination")
	require.ErrorContains(t, err, "recovery_destination")
}

// Whitespace is not an address, and neither is a typo. Both would satisfy a
// non-empty check and neither can receive anything.
func TestGenesisRefusesAMalformedRecoveryDestination(t *testing.T) {
	// Valid bech32, correct checksum, and addressed to a different chain. Built
	// rather than pasted so it stays wrong whatever this chain's prefix is.
	elsewhere, err := bech32.ConvertAndEncode("elsewhere", []byte("foundation-test-addr"))
	require.NoError(t, err)

	for name, destination := range map[string]string{
		"blank":       "   ",
		"not bech32":  "the-foundation",
		"wrong chain": elsewhere,
		"truncated":   testRecoveryDestination[:len(testRecoveryDestination)-1],
	} {
		t.Run(name, func(t *testing.T) {
			genesis := types.DefaultGenesis()
			genesis.Params.RecoveryDestination = destination

			require.ErrorContains(t, genesis.Validate(), "recovery_destination")

			f := initFixture(t)
			require.Error(t, f.keeper.InitGenesis(f.ctx, *genesis),
				"a chain must not start with an unusable recovery destination")
		})
	}
}

// The other half: a genesis that names one starts, keeps it, and exports it
// unchanged. A check that only ever refuses is indistinguishable from one that
// refuses everything.
func TestGenesisWithARecoveryDestinationRoundTrips(t *testing.T) {
	genesis := startableGenesis()
	require.NoError(t, genesis.Validate())

	f := initFixture(t)
	require.NoError(t, f.keeper.InitGenesis(f.ctx, *genesis))

	stored, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, testRecoveryDestination, stored.RecoveryDestination)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.Equal(t, testRecoveryDestination, exported.Params.RecoveryDestination)
	require.NoError(t, exported.Validate(),
		"an export the chain would refuse to import is not an export")

	// Imported into a fresh chain, byte for byte.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))
	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reexported)
}
