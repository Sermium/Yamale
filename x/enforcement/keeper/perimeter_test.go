package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/enforcement/types"
)

// The jurisdictional perimeter, at the place it matters most on this chain: the
// two messages that stop somebody's money.
//
// Every fixture validator in the other files is granted the fixture's country,
// so those tests exercise the rules they are about. Here a second country is
// introduced deliberately, and the assertions are that the same signature that
// works at home is refused abroad — and that the refusal actually leaves the
// money spendable, which is the only version of "refused" that counts.

// abroad returns a funded account recorded in a country the fixture's validators
// hold no grant for.
func (f *fixture) abroad(t *testing.T, amount sdk.Coins) (sdk.AccAddress, string) {
	t.Helper()
	addr, s := f.env.Addr(t)
	f.perimeter.Place(t, s, "NG")
	f.env.Fund(t, addr, amount)
	return addr, s
}

// A validator granted one country cannot open a case against an account in
// another, and can against one at home.
func TestAValidatorCannotOpenACaseAcrossTheBorder(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)

	foreign, foreignStr := f.abroad(t, coins(1_000_000))
	_, homeStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: foreignStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "somebody else's jurisdiction",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.ErrorContains(t, err, "ROLE_ENFORCEMENT_AUTHORITY")
	require.ErrorContains(t, err, "NG")

	// Refused means the money still moves. A perimeter that produced an error and
	// froze the account anyway would be worse than none, because the record would
	// say the case was refused.
	require.False(t, f.keeper.IsFrozen(f.ctx, foreignStr))
	_, elsewhere := f.addr(t)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, foreign,
		sdk.AccAddress(mustBytes(t, f, elsewhere)), coins(1_000)))

	// And nothing was half-written: no case exists, so the next one to be opened
	// is still the first.
	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: homeStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "at home",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), opened.Id)
	require.True(t, f.keeper.IsFrozen(f.ctx, homeStr))
}

// Granting the country afterwards is what makes the refusal a perimeter rather
// than a bug: the same message, the same signer, the same target, and it lands.
func TestGrantingTheCountryLetsTheSameCaseThrough(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, foreignStr := f.abroad(t, coins(1_000_000))

	open := &types.MsgOpenCase{
		Opener: validator, Target: foreignStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "mule account",
	}
	_, err := f.ms.OpenCase(f.ctx, open)
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)

	f.perimeter.Grant(t, validator, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, "NG")

	_, err = f.ms.OpenCase(f.ctx, open)
	require.NoError(t, err)
	require.True(t, f.keeper.IsFrozen(f.ctx, foreignStr))
}

// Revoking it takes the power away again, on the next message rather than at some
// later re-examination: a perimeter checked once at admission protects only the
// moment it ran.
func TestRevokingTheGrantStopsTheNextCase(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, first := f.fundedAddr(t, coins(10_000))
	_, second := f.fundedAddr(t, coins(10_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: first,
		Action: types.CASE_ACTION_FREEZE, Reason: "one",
	})
	require.NoError(t, err)

	f.perimeter.Revoke(t, validator, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, country)

	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: second,
		Action: types.CASE_ACTION_FREEZE, Reason: "two",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.False(t, f.keeper.IsFrozen(f.ctx, second))
}

// An account the chain cannot place cannot be stopped by anybody.
//
// This is the refusal that has to hold hardest, because the tempting
// implementation of "look up the target's country" returns an empty string and no
// error — and an empty string matches nothing, which reads as a refusal, or
// everything, which reads as a pass. Here it is neither: it is an error naming
// the account nobody has placed.
func TestAnAccountWithNoJurisdictionCannotBeFrozen(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)

	// Deliberately not through f.addr, which places what it hands out.
	nowhere, nowhereStr := f.env.Addr(t)
	f.env.Fund(t, nowhere, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: nowhereStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "unplaced",
	})
	require.ErrorIs(t, err, aliastypes.ErrNoJurisdiction)
	require.False(t, f.keeper.IsFrozen(f.ctx, nowhereStr))

	// Not even a chain-wide grant reaches it. The check runs before any grant is
	// consulted, so holding every perimeter is not holding one the chain cannot
	// name.
	f.perimeter.Grant(t, validator, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, aliastypes.ChainWide)
	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: nowhereStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "unplaced",
	})
	require.ErrorIs(t, err, aliastypes.ErrNoJurisdiction)

	// Place it, and the same chain-wide grant reaches it.
	f.place(t, nowhereStr)
	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: nowhereStr,
		Action: types.CASE_ACTION_FREEZE, Reason: "placed",
	})
	require.NoError(t, err)
	require.True(t, f.keeper.IsFrozen(f.ctx, nowhereStr))
}

// The emergency path is not an exception to the border.
//
// It is the one path that acts on a single signature, which makes it the one path
// where an unbounded territorial reach would matter most.
func TestTheEmergencyFreezeStopsAtTheBorderToo(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	founders := f.withEmergencyAuthority(t)

	_, foreignStr := f.abroad(t, coins(1_000_000))
	_, homeStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: foreignStr, Reason: "not their jurisdiction",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.False(t, f.keeper.IsFrozen(f.ctx, foreignStr))

	_, err = f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: founders, Target: homeStr, Reason: "theft in progress",
	})
	require.NoError(t, err)
	require.True(t, f.keeper.IsFrozen(f.ctx, homeStr))
}

// A seizure is a freeze plus a taking, so it is behind the same border.
func TestASeizureCannotBeOpenedAcrossTheBorder(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	_, foreignStr := f.abroad(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: foreignStr, Action: types.CASE_ACTION_SEIZE,
		Reason: "recovery", EvidenceUri: "ipfs://evidence", EvidenceHash: "abcd",
		LegalInstrument: instrument(),
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
}
