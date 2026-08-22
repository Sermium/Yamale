package keeper_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/enforcement/types"
)

// The four oversight additions, tested against the thing each one is supposed
// to prevent rather than against its own implementation.

// ---------------------------------------------------------------------------
// 1. A seizure must carry an external legal instrument.
// ---------------------------------------------------------------------------

// The validator set producing a document and then citing that document as its
// warrant to take somebody's assets is not oversight; it is the same body
// twice. A seizure has to name something issued outside this chain.
func TestASeizureWithoutALegalInstrumentIsRefused(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener:       validator,
		Target:       scammerStr,
		Action:       types.CASE_ACTION_SEIZE,
		Reason:       "drained a pool",
		EvidenceUri:  "https://example.org/report.pdf",
		EvidenceHash: "9f2c0e1b",
	})
	require.ErrorIs(t, err, types.ErrLegalInstrumentRequired,
		"a seizure with evidence but no legal authority must be refused")

	// The wording is asserted, not just the sentinel, and that is not
	// fussiness. Sending no instrument at all and sending a malformed one are
	// different mistakes with different fixes, and both are refused with the
	// same registered error — so without this the check for "you sent nothing"
	// could be deleted entirely and the field-level validation underneath would
	// keep this test green while telling a validator to fix a field they never
	// filled in. Mutation testing found exactly that.
	require.ErrorContains(t, err, "must name the court order, regulatory direction or warrant")

	// And the account is untouched: a refused case freezes nothing.
	require.False(t, f.keeper.IsFrozen(f.ctx, scammerStr))
}

// The instrument is a different thing from the evidence, and satisfying one
// must not satisfy the other. This is the substitution the separate structure
// exists to prevent.
func TestEvidenceDoesNotSubstituteForLegalAuthority(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	// Evidence waived by governance, instrument still required. The two are
	// governed separately on purpose: a deployment may decide how much evidence
	// to demand, and no deployment may decide it needs no court order.
	f.setParams(t, func(p *types.Params) { p.SeizeRequiresEvidence = false })

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_SEIZE,
		Reason: "drained a pool",
	})
	require.ErrorIs(t, err, types.ErrLegalInstrumentRequired,
		"waiving evidence must not waive the requirement for external legal authority")
	require.ErrorContains(t, err, "must name the court order, regulatory direction or warrant")
}

func TestAMalformedLegalInstrumentIsRefused(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)

	for name, mutate := range map[string]func(*types.LegalInstrument){
		"no issuing authority": func(li *types.LegalInstrument) { li.IssuingAuthority = "  " },
		"no reference": func(li *types.LegalInstrument) {
			// The half that makes the instrument findable by somebody who does
			// not trust this chain. A hash without it pins a document nobody
			// can ask for.
			li.Reference = ""
		},
		"unspecified kind": func(li *types.LegalInstrument) {
			li.Kind = types.LEGAL_INSTRUMENT_KIND_UNSPECIFIED
		},
		"hash is not a digest": func(li *types.LegalInstrument) { li.Hash = "not-a-hash" },
		"hash is not hex": func(li *types.LegalInstrument) {
			li.Hash = strings.Repeat("z", 64)
		},
		"hash is uppercase": func(li *types.LegalInstrument) {
			li.Hash = strings.ToUpper(strings.Repeat("a1b2", 16))
		},
		"no issue date": func(li *types.LegalInstrument) { li.IssuedAt = 0 },
		"dated in the future": func(li *types.LegalInstrument) {
			// An order dated tomorrow has not been issued.
			li.IssuedAt = f.ctx.(sdk.Context).BlockTime().Unix() + 86_400
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, target := f.fundedAddr(t, coins(1_000))
			li := instrument()
			mutate(&li)

			_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
				Opener: validator, Target: target, Action: types.CASE_ACTION_SEIZE,
				Reason: "grounds", EvidenceUri: "https://example.org/x", EvidenceHash: "ab",
				LegalInstrument: li,
			})
			require.ErrorIs(t, err, types.ErrLegalInstrumentRequired)
			require.False(t, f.keeper.IsFrozen(f.ctx, target))
		})
	}
}

// A freeze takes nothing and has to be openable in the minute a theft is
// noticed, which is not a minute in which anybody has a court order.
func TestAFreezeNeedsNoLegalInstrument(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, suspectStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: suspectStr, Action: types.CASE_ACTION_FREEZE,
		Reason: "an exchange reported this deposit as stolen four minutes ago",
	})
	require.NoError(t, err)
	require.True(t, f.keeper.IsFrozen(f.ctx, suspectStr))
}

// The instrument is kept on the case, because a record of what authority a
// seizure rested on is the only thing that makes the authority checkable later.
func TestTheLegalInstrumentIsKeptOnTheCase(t *testing.T) {
	f := initFixture(t)
	_, scammerStr := f.fundedAddr(t, coins(1_000_000))

	id := f.openAndHoldSeizure(t, scammerStr)

	stored, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, instrument(), stored.LegalInstrument)
}

// ---------------------------------------------------------------------------
// 2. Delays scale with the amount.
// ---------------------------------------------------------------------------

// Taking a market trader's float and taking a family's savings should not move
// at the same speed.
func TestALargeSeizureWaitsLongerThanASmallOne(t *testing.T) {
	f := initFixture(t)
	_, smallStr := f.fundedAddr(t, coins(100_000))   // below every tier
	_, largeStr := f.fundedAddr(t, coins(5_000_000)) // over the 1,000,000 tier

	smallID := f.openAndHoldSeizure(t, smallStr)
	largeID := f.openAndHoldSeizure(t, largeStr)

	small, err := f.keeper.Case.Get(f.ctx, smallID)
	require.NoError(t, err)
	large, err := f.keeper.Case.Get(f.ctx, largeID)
	require.NoError(t, err)

	// Both were decided in the same block, so the execute heights are directly
	// comparable and the difference is the schedule and nothing else.
	require.Equal(t, small.ResolvedAtHeight, large.ResolvedAtHeight)
	require.Equal(t, small.ResolvedAtHeight+10, small.ExecuteAtHeight, "the floor applies to everything")
	require.Equal(t, large.ResolvedAtHeight+100, large.ExecuteAtHeight, "the 1,000,000 tier applies to the large one")
	require.Greater(t, large.ExecuteAtHeight, small.ExecuteAtHeight,
		"taking somebody's savings must not move at the speed meant for pocket change")

	// And the wait is real, not just recorded: at the small case's height the
	// large one has still taken nothing.
	f.runTo(t, small.ExecuteAtHeight)
	require.Equal(t, math.NewInt(100_000), f.env.Balance(f.destination, "uyml"))

	stillHeld, err := f.keeper.Case.Get(f.ctx, largeID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, stillHeld.Status)

	f.runTo(t, large.ExecuteAtHeight)
	require.Equal(t, math.NewInt(5_100_000), f.env.Balance(f.destination, "uyml"))
}

// Staked funds count towards the size of a seizure. Without this, anybody
// holding their money in a validator would have the largest seizure on the
// chain treated as the smallest — a few blocks' delay, almost no room taken in
// the window — and the stake collected later anyway.
func TestStakedFundsCountTowardsTheDelay(t *testing.T) {
	f := initFixture(t)

	// A registered validator, so the assessment can convert shares to tokens
	// through it.
	holder := f.addValidator(t, 10)
	holderBz, err := f.env.AddressCodec.StringToBytes(holder)
	require.NoError(t, err)
	operator := sdk.ValAddress(holderBz).String()

	_, richStr := f.fundedAddr(t, coins(1_000))
	f.staking.delegate(richStr, operator, 9_000_000)

	id := f.openAndHoldSeizure(t, richStr)
	held, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)

	require.Equal(t, coins(9_001_000), held.AssessedValue,
		"the assessment must see the stake, not only the balance")
	require.Equal(t, held.ResolvedAtHeight+100, held.ExecuteAtHeight,
		"1,000 liquid over 9,000,000 staked is a large seizure, and must be delayed as one")
}

// A seizure the validators agreed to takes nothing until its delay expires.
// Nothing is unbonded either — a case the ombudsman stops during the hold must
// leave its target exactly as they were.
func TestAHeldSeizureTakesNothingAndUnbondsNothing(t *testing.T) {
	f := initFixture(t)

	holder := f.addValidator(t, 10)
	holderBz, err := f.env.AddressCodec.StringToBytes(holder)
	require.NoError(t, err)
	operator := sdk.ValAddress(holderBz).String()

	scammer, scammerStr := f.fundedAddr(t, coins(500_000))
	f.staking.delegate(scammerStr, operator, 200_000)

	f.openAndHoldSeizure(t, scammerStr)

	require.Equal(t, math.NewInt(500_000), f.env.Balance(scammer, "uyml"),
		"a held seizure has not taken anything")
	require.Equal(t, math.ZeroInt(), f.env.Balance(f.destination, "uyml"))
	require.Empty(t, f.staking.undelegated[scammerStr],
		"nothing is unbonded during the hold, so a vetoed case leaves the target still staked")

	// The account is frozen throughout, and the freeze no longer lapses: the
	// set has decided, so there is nothing left for a lapse to protect against.
	require.True(t, f.keeper.IsFrozen(f.ctx, scammerStr))
	freeze, found, err := f.keeper.FreezeOf(f.ctx, scammerStr)
	require.NoError(t, err)
	require.True(t, found)
	require.Zero(t, freeze.ExpiresAtHeight)
}

// The delay would be worth nothing if a permissionless message could step over
// it. Sweep is that message.
func TestSweepCannotShortCircuitTheDelay(t *testing.T) {
	f := initFixture(t)
	scammer, scammerStr := f.fundedAddr(t, coins(1_000_000))
	_, senderStr := f.fundedAddr(t, coins(1))

	id := f.openAndHoldSeizure(t, scammerStr)

	_, err := f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: senderStr, CaseId: id})
	require.ErrorIs(t, err, types.ErrNotPassed,
		"sweeping a held case would collect the balance before the veto window had opened")
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(scammer, "uyml"))
}

// EventCaseResolved is emitted once, at a case's final status, and HELD is not
// one.
//
// Anything watching case lifecycles keys off that event. Emitting it when a
// seizure is merely agreed would announce it as finished and never correct
// itself, leaving an explorer showing "resolved: held" for a case whose money
// moved a week later — and a vetoed case that emitted nothing would be the one
// kind of ending nobody could follow.
func TestResolvedIsEmittedOnceAndOnlyAtAFinalStatus(t *testing.T) {
	resolvedStatuses := func(f *fixture) []types.CaseStatus {
		out := make([]types.CaseStatus, 0)
		for _, e := range f.env.Ctx.EventManager().Events() {
			if e.Type != "blockchain.enforcement.v1.EventCaseResolved" {
				continue
			}
			for _, a := range e.Attributes {
				if a.Key == "status" {
					out = append(out, types.CaseStatus(types.CaseStatus_value[strings.Trim(a.Value, `"`)]))
				}
			}
		}
		return out
	}

	t.Run("a seizure resolves once, when it executes", func(t *testing.T) {
		f := initFixture(t)
		_, target := f.fundedAddr(t, coins(80_000))

		id := f.openAndHoldSeizure(t, target)
		require.Empty(t, resolvedStatuses(f),
			"an agreed seizure is not resolved; it is waiting, and EventCaseHeld is what says so")

		f.executeHeld(t, id)
		require.Equal(t, []types.CaseStatus{types.CASE_STATUS_PASSED}, resolvedStatuses(f),
			"the seizure resolves exactly once, at the status it ended on")
	})

	t.Run("a veto resolves the case too", func(t *testing.T) {
		f := initFixture(t)
		ombudsman := f.appointOmbudsman(t)
		validator := f.addValidator(t, 10)
		f.addValidator(t, 10)
		_, target := f.fundedAddr(t, coins(80_000))

		opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
			Opener: validator, Target: target, Action: types.CASE_ACTION_FREEZE, Reason: "suspicion",
		})
		require.NoError(t, err)
		_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
			Ombudsman: ombudsman, CaseId: opened.Id, Reason: "the wrong account",
		})
		require.NoError(t, err)

		require.Equal(t, []types.CaseStatus{types.CASE_STATUS_VETOED}, resolvedStatuses(f),
			"a vetoed case must announce its ending like every other kind")
	})
}

// ---------------------------------------------------------------------------
// 3. The ombudsman's veto.
// ---------------------------------------------------------------------------

func TestTheOmbudsmanCanVetoACaseStillBeingVotedOn(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	scammer, scammerStr := f.fundedAddr(t, coins(400_000))

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspected theft",
	})
	require.NoError(t, err)
	require.True(t, f.keeper.IsFrozen(f.ctx, scammerStr))

	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: opened.Id,
		Reason: "the account belongs to a licensed remitter and the transfers are its ordinary business",
	})
	require.NoError(t, err)

	vetoed, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VETOED, vetoed.Status,
		"vetoed is its own status: rejected is the set disagreeing, this is one office outside it refusing")
	require.Contains(t, vetoed.Reason, "Vetoed by the ombudsman")

	// The account can send again, in the same block.
	require.False(t, f.keeper.IsFrozen(f.ctx, scammerStr))
	elsewhere, _ := f.addr(t)
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(400_000)))

	// And the case does not resolve again when its voting period ends.
	f.runTo(t, vetoed.VotingEndsAtHeight+1)
	after, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VETOED, after.Status)
}

// The veto that matters: a seizure the validators have already agreed to, in
// the window before it takes anything.
func TestTheOmbudsmanCanVetoAnAgreedSeizureBeforeItExecutes(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	scammer, scammerStr := f.fundedAddr(t, coins(2_000_000))

	id := f.openAndHoldSeizure(t, scammerStr)
	held, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)

	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: id,
		Reason: "the order relied on names a different account; the reference does not match the register",
	})
	require.NoError(t, err)

	// The account is released and keeps every coin.
	require.False(t, f.keeper.IsFrozen(f.ctx, scammerStr))
	require.Equal(t, math.NewInt(2_000_000), f.env.Balance(scammer, "uyml"))
	require.Equal(t, math.ZeroInt(), f.env.Balance(f.destination, "uyml"))

	// And the end blocker does not carry it out when the delay expires. This is
	// the assertion that matters: a queue entry left behind would seize from an
	// account that had already been given back.
	f.runTo(t, held.ExecuteAtHeight)
	f.runTo(t, held.ExecuteAtHeight+1)

	require.Equal(t, math.NewInt(2_000_000), f.env.Balance(scammer, "uyml"))
	require.Equal(t, math.ZeroInt(), f.env.Balance(f.destination, "uyml"))

	after, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VETOED, after.Status)

	// A vetoed seizure took nothing, so it must not have taken room in the
	// rolling window either.
	window, err := f.qs.SeizureWindow(f.ctx, &types.QuerySeizureWindowRequest{})
	require.NoError(t, err)
	require.Zero(t, window.SeizureCount)
	require.True(t, window.Seized.IsZero())
}

// A veto cannot un-take money, and the refusal says so rather than marking the
// case vetoed and leaving a comforting lie in the record.
func TestTheOmbudsmanCannotVetoASeizureThatHasExecuted(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	_, scammerStr := f.fundedAddr(t, coins(300_000))

	id := f.openAndPassSeizure(t, scammerStr)
	require.Equal(t, math.NewInt(300_000), f.env.Balance(f.destination, "uyml"))

	_, err := f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: id, Reason: "too late",
	})
	require.ErrorIs(t, err, types.ErrCaseClosed)

	unchanged, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_PASSED, unchanged.Status)
}

// An unappointed office means nobody, never anybody. A comparison that let an
// empty message field match an empty parameter would hand the veto to whoever
// noticed first.
func TestWithNoOmbudsmanAppointedThereIsNoVeto(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspicion",
	})
	require.NoError(t, err)

	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: "", CaseId: opened.Id, Reason: "letting myself out",
	})
	require.ErrorIs(t, err, types.ErrNoOmbudsman)

	_, stranger := f.addr(t)
	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: stranger, CaseId: opened.Id, Reason: "nor me",
	})
	require.ErrorIs(t, err, types.ErrNoOmbudsman)

	require.True(t, f.keeper.IsFrozen(f.ctx, scammerStr))
}

func TestOnlyTheAppointedOmbudsmanMayVeto(t *testing.T) {
	f := initFixture(t)
	f.appointOmbudsman(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspicion",
	})
	require.NoError(t, err)

	_, stranger := f.addr(t)
	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: stranger, CaseId: opened.Id, Reason: "not my office",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
	require.True(t, f.keeper.IsFrozen(f.ctx, scammerStr))
}

// ---------------------------------------------------------------------------
// 4. The ombudsman is structurally unable to open or advance a case.
// ---------------------------------------------------------------------------

// The strong version of the claim: enumerate every message in the service and
// show the ombudsman is refused by all of them but the veto.
//
// The list is checked against the interface by reflection, so a message added
// later fails this test until somebody decides, deliberately, which side of the
// line it falls on. That is the part that makes this structural rather than a
// snapshot: the guarantee is maintained by the compiler and the test together,
// not by whoever remembers.
func TestTheOmbudsmanIsRefusedByEveryMessageButTheVeto(t *testing.T) {
	msgServerType := reflect.TypeOf((*types.MsgServer)(nil)).Elem()
	got := make([]string, 0, msgServerType.NumMethod())
	for i := 0; i < msgServerType.NumMethod(); i++ {
		got = append(got, msgServerType.Method(i).Name)
	}
	sort.Strings(got)

	// Every message this module serves, and what the ombudsman may do with it.
	// Only one says "may".
	want := []string{
		"EmergencyFreeze",  // refused: opens a case
		"EmergencyRelease", // refused: emergency authority only, and cannot be the ombudsman
		"OmbudsmanVeto",    // the one message this office may send
		"OpenCase",         // refused: opens a case
		"ReverseCase",      // refused: governance authority only
		"Sweep",            // refused: advances collection
		"UpdateParams",     // refused: governance authority only
		"VoteCase",         // refused: advances a case
		"WithdrawCase",     // refused: only the opener, which the ombudsman can never be
	}
	require.Equal(t, want, got,
		"a message was added to x/enforcement; decide explicitly whether the ombudsman may send it, then update this list")
}

// Every message in the service must be registered on the interface registry,
// or it cannot be decoded from a transaction.
//
// This test exists because writing the veto skipped that registration, and
// nothing caught it: the keeper tests call the message server directly, so they
// pass against a message no wallet, CLI or group proposal could actually
// submit. The office would have been unable to use the only power it has, and
// the first sign of it would have been on a chain.
//
// Derived from the service descriptor rather than from a hand-written list, so
// the next message added is covered the moment it exists.
func TestEveryMessageIsRegisteredForDecoding(t *testing.T) {
	f := initFixture(t)
	registry := f.env.Codec.InterfaceRegistry()

	msgServerType := reflect.TypeOf((*types.MsgServer)(nil)).Elem()
	require.Positive(t, msgServerType.NumMethod(), "a test that enumerates nothing passes vacuously")

	for i := 0; i < msgServerType.NumMethod(); i++ {
		name := msgServerType.Method(i).Name
		typeURL := "/blockchain.enforcement.v1.Msg" + name
		t.Run(name, func(t *testing.T) {
			_, err := registry.Resolve(typeURL)
			require.NoError(t, err,
				"%s is served by the Msg service but is not registered in RegisterInterfaces, "+
					"so it can be handled but never decoded from a transaction", typeURL)
		})
	}
}

// The ombudsman is barred from opening and voting even when it holds a bonded
// validator's key.
//
// That is the case worth testing, because it is the one a signer check alone
// would let through. An ombudsman that was merely "not a validator" would be
// protected by an accident of configuration; this is protected by the handler,
// on every call, whatever the staking module says.
func TestABondedOmbudsmanStillCannotOpenOrVote(t *testing.T) {
	f := initFixture(t)

	// The ombudsman's key is also a bonded validator's — the configuration the
	// office is supposed to be appointed outside of, arrived at anyway.
	ombudsman := f.addValidator(t, 10)
	other := f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.setParams(t, func(p *types.Params) { p.Ombudsman = ombudsman })

	_, targetStr := f.fundedAddr(t, coins(1_000_000))

	_, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: ombudsman, Target: targetStr, Action: types.CASE_ACTION_FREEZE, Reason: "opening one myself",
	})
	require.ErrorIs(t, err, types.ErrOmbudsmanCannotInitiate)
	require.False(t, f.keeper.IsFrozen(f.ctx, targetStr))

	_, err = f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: ombudsman, Target: targetStr, Action: types.CASE_ACTION_SEIZE,
		Reason: "or a seizure", EvidenceUri: "https://example.org/x", EvidenceHash: "ab",
		LegalInstrument: instrument(),
	})
	require.ErrorIs(t, err, types.ErrOmbudsmanCannotInitiate)

	// Someone else opens one; the ombudsman still cannot move it along.
	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: other, Target: targetStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspected theft",
	})
	require.NoError(t, err)

	for _, option := range []types.VoteOption{
		types.VOTE_OPTION_YES, types.VOTE_OPTION_NO, types.VOTE_OPTION_ABSTAIN,
	} {
		_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
			Voter: ombudsman, CaseId: opened.Id, Option: option,
		})
		require.ErrorIs(t, err, types.ErrOmbudsmanCannotInitiate,
			"an office that could vote at all would be inside the process it is appointed to check")
	}

	// Not even as an abstention on the record: no vote was stored.
	votes, err := f.qs.CaseVotes(f.ctx, &types.QueryCaseVotesRequest{CaseId: opened.Id})
	require.NoError(t, err)
	for _, vote := range votes.Votes {
		require.NotEqual(t, sdk.ValAddress(mustBytes(t, f, ombudsman)).String(), vote.Validator)
	}
}

// The ombudsman cannot sweep either. A sweep only collects what an executed
// case already ordered, so this bar is not strictly necessary — it is here
// because the claim the office makes is total, and one exception would turn it
// into a claim with a footnote.
func TestTheOmbudsmanCannotSweep(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	_, scammerStr := f.fundedAddr(t, coins(100_000))

	id := f.openAndPassSeizure(t, scammerStr)
	f.env.Fund(t, mustAddr(t, f, scammerStr), coins(50_000))

	_, err := f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: ombudsman, CaseId: id})
	require.ErrorIs(t, err, types.ErrOmbudsmanCannotInitiate)

	// And the chore is still done by anybody else, so nothing is stranded.
	_, anyone := f.fundedAddr(t, coins(1))
	_, err = f.ms.Sweep(f.ctx, &types.MsgSweep{Sender: anyone, CaseId: id})
	require.NoError(t, err)
}

// The parameters refuse the two configurations that would give the office a way
// in through the front door.
func TestParamsRefuseAnOmbudsmanThatCouldAlsoInitiate(t *testing.T) {
	f := initFixture(t)
	base, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	_, shared := f.addr(t)

	both := base
	both.EmergencyAuthority = shared
	both.Ombudsman = shared
	require.ErrorContains(t, both.Validate(), "emergency_authority",
		"the emergency authority can open a case; the ombudsman must never be able to")

	beneficiary := base
	beneficiary.Ombudsman = base.RecoveryDestination
	require.ErrorContains(t, beneficiary.Validate(), "recovery_destination",
		"the office that can stop a seizure must not be the one that receives what seizures take")
}

// The belt-and-braces bar, tested against the state it is braced for: a
// migration that wrote parameters straight into the store without going through
// Validate, leaving the ombudsman holding the emergency authority's key.
func TestEvenUnvalidatedParamsCannotLetTheOmbudsmanOpenACase(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 10)
	f.addValidator(t, 10)

	_, shared := f.addr(t)
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.EmergencyAuthority = shared
	params.Ombudsman = shared
	require.Error(t, params.Validate(), "this configuration is exactly what Validate refuses")

	// Written anyway, as a bad migration would.
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, targetStr := f.fundedAddr(t, coins(100_000))
	_, err = f.ms.EmergencyFreeze(f.ctx, &types.MsgEmergencyFreeze{
		Authority: shared, Target: targetStr, Reason: "letting myself in through the emergency door",
	})
	require.ErrorIs(t, err, types.ErrOmbudsmanCannotInitiate)
	require.False(t, f.keeper.IsFrozen(f.ctx, targetStr))
}

// ---------------------------------------------------------------------------
// 5. The rolling window.
// ---------------------------------------------------------------------------

// A chain that can seize a bounded amount per period cannot be used for mass
// expropriation in one sitting.
func TestTheRollingWindowRefusesASeizureThatWouldBreachItAndAllowsOneAfterItRolls(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, func(p *types.Params) {
		p.SeizureDelayBlocks = 5
		p.SeizureDelayTiers = []types.SeizureDelayTier{
			{Threshold: sdk.NewCoin("uyml", math.NewInt(500_000)), DelayBlocks: 20},
		}
		p.SeizureWindowBlocks = 200
		p.SeizureWindowCap = sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(1_000_000)))
	})

	firstAcct, first := f.fundedAddr(t, coins(600_000))
	secondAcct, second := f.fundedAddr(t, coins(600_000))

	// The first seizure fits inside the window and is carried out.
	firstID := f.openAndHoldSeizure(t, first)
	f.executeHeld(t, firstID)
	require.Equal(t, math.NewInt(600_000), f.env.Balance(f.destination, "uyml"))
	require.Equal(t, math.ZeroInt(), f.env.Balance(firstAcct, "uyml"))

	window, err := f.qs.SeizureWindow(f.ctx, &types.QuerySeizureWindowRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), window.SeizureCount)
	require.Equal(t, coins(600_000), window.Seized)
	require.Equal(t, coins(400_000), window.Remaining)

	// The second would take the window to 1,200,000 against a cap of 1,000,000,
	// so when its delay expires it is refused — and refused without being lost.
	secondID := f.openAndHoldSeizure(t, second)
	held, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	f.runTo(t, held.ExecuteAtHeight)

	deferred, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, deferred.Status,
		"a seizure the cap refuses stays held; it is not cancelled and not carried out")
	require.Equal(t, math.NewInt(600_000), f.env.Balance(secondAcct, "uyml"),
		"the cap refused before anything moved")
	require.Equal(t, math.NewInt(600_000), f.env.Balance(f.destination, "uyml"))
	require.True(t, f.keeper.IsFrozen(t2ctx(f), second), "the target stays frozen while the case waits")

	// It is still listed as held, so nobody has to guess whether it was
	// forgotten.
	stillHeld, err := f.qs.HeldCases(f.ctx, &types.QueryHeldCasesRequest{})
	require.NoError(t, err)
	require.Len(t, stillHeld.Case, 1)
	require.Equal(t, secondID, stillHeld.Case[0].Id)

	// Waiting a few blocks does not help: the window has not rolled.
	f.runTo(t, held.ExecuteAtHeight+50)
	require.Equal(t, math.NewInt(600_000), f.env.Balance(secondAcct, "uyml"))

	// Once the first seizure falls out of the window, the second goes through.
	// 200 blocks after the first executed, the window no longer contains it.
	firstCase, err := f.keeper.Case.Get(f.ctx, firstID)
	require.NoError(t, err)
	f.runTo(t, firstCase.ResolvedAtHeight+20+200+1)

	executed, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_PASSED, executed.Status,
		"once the window rolls, the seizure the cap deferred is carried out")
	require.Equal(t, math.ZeroInt(), f.env.Balance(secondAcct, "uyml"))
	require.Equal(t, math.NewInt(1_200_000), f.env.Balance(f.destination, "uyml"))
}

// The count cap binds every denomination, including one issued the day after
// the value cap was last set. It is the half of the cap that cannot be walked
// around by choosing a currency nobody thought to price.
func TestTheCountCapRefusesASeizureInAnUncappedDenomination(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, func(p *types.Params) {
		p.SeizureDelayBlocks = 5
		p.SeizureWindowBlocks = 500
		p.MaxSeizuresPerWindow = 1
	})

	_, first := f.fundedAddr(t, coins(10_000))

	// A currency the value cap has never heard of.
	exotic := sdk.NewCoins(sdk.NewCoin("ukes", math.NewInt(9_000_000_000)))
	secondAcct, second := f.addr(t)
	f.env.Fund(t, secondAcct, exotic)

	firstID := f.openAndHoldSeizure(t, first)
	f.executeHeld(t, firstID)

	secondID := f.openAndHoldSeizure(t, second)
	held, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	f.runTo(t, held.ExecuteAtHeight)

	stillHeld, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, stillHeld.Status,
		"the count cap must bind a denomination the value cap does not name")
	require.Equal(t, math.NewInt(9_000_000_000), f.env.Balance(secondAcct, "ukes"))
}

// A deferred seizure must be stoppable, and must stay stopped.
//
// This is the regression test for a real bug found while auditing this change.
// A deferred case is queued at its retry height rather than at the height its
// delay expired, so a stop path that deleted the entry at the *original* height
// deleted a key that was not there — leaving the case queued, and the end
// blocker carrying out a seizure against an account that had already been
// released. The fix is that execute_at_height and the queue entry move
// together, so there is exactly one entry and it is always where the case says.
func TestADeferredSeizureThatIsVetoedNeverExecutes(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	f.setParams(t, func(p *types.Params) {
		p.SeizureDelayBlocks = 5
		p.SeizureWindowBlocks = 200
		p.MaxSeizuresPerWindow = 1
		p.Ombudsman = ombudsman
	})

	_, first := f.fundedAddr(t, coins(10_000))
	secondAcct, second := f.fundedAddr(t, coins(20_000))

	// Fill the window, so the next seizure is deferred rather than carried out.
	firstID := f.openAndHoldSeizure(t, first)
	f.executeHeld(t, firstID)

	secondID := f.openAndHoldSeizure(t, second)
	originallyDueAt, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	f.runTo(t, originallyDueAt.ExecuteAtHeight)

	deferred, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, deferred.Status)
	require.Greater(t, deferred.ExecuteAtHeight, originallyDueAt.ExecuteAtHeight,
		"a deferred case must be re-queued, and must say where")

	// Stopped while deferred.
	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: secondID, Reason: "the instrument does not name this account",
	})
	require.NoError(t, err)
	require.False(t, f.keeper.IsFrozen(f.ctx, second))

	// Run well past both the original height and the retry height. A stale
	// queue entry at either one would seize from a released account.
	f.runTo(t, deferred.ExecuteAtHeight)
	f.runTo(t, deferred.ExecuteAtHeight+1)
	f.runTo(t, deferred.ExecuteAtHeight+500)

	require.Equal(t, math.NewInt(20_000), f.env.Balance(secondAcct, "uyml"),
		"a vetoed seizure must never execute, however many times it was deferred first")

	after, err := f.keeper.Case.Get(f.ctx, secondID)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VETOED, after.Status)

	// And it is not still listed as waiting.
	stillHeld, err := f.qs.HeldCases(f.ctx, &types.QueryHeldCasesRequest{})
	require.NoError(t, err)
	require.Empty(t, stillHeld.Case)
}

// Summing the window must not depend on pruning having run, and pruning must
// not be what decides the answer. The lower bound is computed from the current
// height every time it is asked.
func TestTheWindowForgetsNothingItShouldStillCount(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, func(p *types.Params) {
		p.SeizureDelayBlocks = 5
		p.SeizureWindowBlocks = 1_000
	})

	_, target := f.fundedAddr(t, coins(700_000))
	id := f.openAndHoldSeizure(t, target)
	f.executeHeld(t, id)

	executed, err := f.keeper.Case.Get(f.ctx, id)
	require.NoError(t, err)

	// Just inside the window, with no end blocker having run in between: the
	// seizure still counts.
	f.atHeight(executed.ResolvedAtHeight + 900)
	window, err := f.qs.SeizureWindow(f.ctx, &types.QuerySeizureWindowRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), window.SeizureCount)

	// Just outside it, again with no pruning in between: it does not.
	f.atHeight(executed.ResolvedAtHeight + 1_100)
	window, err = f.qs.SeizureWindow(f.ctx, &types.QuerySeizureWindowRequest{})
	require.NoError(t, err)
	require.Zero(t, window.SeizureCount)
	require.True(t, window.Seized.IsZero())
}

// A zero from genesis in an end blocker is how this chain halts, so the window
// arithmetic is guarded where it is used and not only where it is validated.
func TestAZeroWindowFromBadParamsFailsClosed(t *testing.T) {
	f := initFixture(t)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.SeizureWindowBlocks = 0
	require.Error(t, params.Validate(), "Validate refuses this")

	// Written anyway, as a bad migration would. The end blocker must still run,
	// and the cap must still bind rather than silently forgetting everything.
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	require.NoError(t, f.keeper.EndBlocker(f.ctx), "a zero window must not halt the chain")

	require.Positive(t, params.WindowBlocks(), "a zero window must not schedule a retry zero blocks away")
	require.Less(t, params.WindowStartHeight(1_000_000), int64(1_000_000),
		"a zero window must not put the window's start at the current height, which would forget every seizure")
}

// ---------------------------------------------------------------------------
// 6. Genesis, with cases at every status.
// ---------------------------------------------------------------------------

// Every status a case can hold, exported and imported into a *second*
// environment — not the one that exported it, which would prove only that the
// keeper can read its own store.
func TestGenesisRoundTripsWithCasesAtEveryStatus(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)
	one := f.addValidator(t, 10)
	two := f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.addValidator(t, 10)
	f.setParams(t, func(p *types.Params) {
		p.EmergencyAuthority = mustNewAddr(t, f)
	})
	emergency, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	statuses := make(map[types.CaseStatus]uint64)

	// WITHDRAWN
	_, withdrawnTarget := f.fundedAddr(t, coins(10_000))
	withdrawn, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: withdrawnTarget, Action: types.CASE_ACTION_FREEZE, Reason: "taken back",
	})
	require.NoError(t, err)
	_, err = f.ms.WithdrawCase(f.ctx, &types.MsgWithdrawCase{Opener: one, CaseId: withdrawn.Id})
	require.NoError(t, err)
	statuses[types.CASE_STATUS_WITHDRAWN] = withdrawn.Id

	// REJECTED — enough no power that the threshold is unreachable.
	_, rejectedTarget := f.fundedAddr(t, coins(10_000))
	rejected, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: rejectedTarget, Action: types.CASE_ACTION_FREEZE, Reason: "refused by the set",
	})
	require.NoError(t, err)
	for _, v := range f.staking.validators {
		if _, err := f.keeper.Case.Get(f.ctx, rejected.Id); err == nil {
			c, _ := f.keeper.Case.Get(f.ctx, rejected.Id)
			if c.Status != types.CASE_STATUS_VOTING {
				break
			}
		}
		_, err = f.ms.VoteCase(f.ctx, &types.MsgVoteCase{
			Voter: v.account, CaseId: rejected.Id, Option: types.VOTE_OPTION_NO,
		})
		require.NoError(t, err)
	}
	statuses[types.CASE_STATUS_REJECTED] = rejected.Id

	// EXPIRED — nobody votes and the period runs out.
	_, expiredTarget := f.fundedAddr(t, coins(10_000))
	expired, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: two, Target: expiredTarget, Action: types.CASE_ACTION_FREEZE, Reason: "nobody voted",
	})
	require.NoError(t, err)
	expiredCase, err := f.keeper.Case.Get(f.ctx, expired.Id)
	require.NoError(t, err)
	f.runTo(t, expiredCase.VotingEndsAtHeight)
	statuses[types.CASE_STATUS_EXPIRED] = expired.Id

	// VETOED
	_, vetoedTarget := f.fundedAddr(t, coins(10_000))
	vetoed, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: vetoedTarget, Action: types.CASE_ACTION_FREEZE, Reason: "stopped by the ombudsman",
	})
	require.NoError(t, err)
	_, err = f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: vetoed.Id, Reason: "the account is a licensed remitter's",
	})
	require.NoError(t, err)
	statuses[types.CASE_STATUS_VETOED] = vetoed.Id

	// HELD — agreed and waiting out its delay, with a freeze that must survive.
	_, heldTarget := f.fundedAddr(t, coins(2_000_000))
	heldID := f.openAndHoldSeizure(t, heldTarget)
	statuses[types.CASE_STATUS_HELD] = heldID

	// PASSED — a seizure carried all the way through, which also puts a record
	// in the rolling window's ledger.
	_, passedTarget := f.fundedAddr(t, coins(150_000))
	passedID := f.openAndPassSeizure(t, passedTarget)
	statuses[types.CASE_STATUS_PASSED] = passedID

	// REVERSED — governance overturning the case that passed.
	_, err = f.ms.ReverseCase(f.ctx, &types.MsgReverseCase{
		Authority: f.env.AuthorityString(t), CaseId: passedID, Reason: "wrong account",
	})
	require.NoError(t, err)
	statuses[types.CASE_STATUS_REVERSED] = passedID

	// A second passed seizure so that PASSED is still represented after the
	// reversal above, and so the ledger carries a record that must round-trip.
	_, secondPassedTarget := f.fundedAddr(t, coins(90_000))
	secondPassedID := f.openAndPassSeizure(t, secondPassedTarget)
	statuses[types.CASE_STATUS_PASSED] = secondPassedID

	// VOTING, opened last and on purpose. Every runTo above ends a block, and a
	// case opened earlier would have been resolved by one of them — which is
	// how an earlier draft of this test exported no VOTING case at all and only
	// noticed because it asserts that every status is present.
	_, votingTarget := f.fundedAddr(t, coins(10_000))
	voting, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: one, Target: votingTarget, Action: types.CASE_ACTION_FREEZE, Reason: "still being argued",
	})
	require.NoError(t, err)
	statuses[types.CASE_STATUS_VOTING] = voting.Id

	_ = emergency

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an export the chain would refuse to import is not an export")
	require.NotEmpty(t, exported.Seizures, "the rolling window's ledger must be exported")

	// Every status is actually present, so this test cannot pass by covering
	// fewer of them than it claims.
	present := make(map[types.CaseStatus]bool)
	for _, c := range exported.Cases {
		present[c.Status] = true
	}
	for _, status := range []types.CaseStatus{
		types.CASE_STATUS_VOTING,
		types.CASE_STATUS_PASSED,
		types.CASE_STATUS_REJECTED,
		types.CASE_STATUS_EXPIRED,
		types.CASE_STATUS_WITHDRAWN,
		types.CASE_STATUS_REVERSED,
		types.CASE_STATUS_HELD,
		types.CASE_STATUS_VETOED,
	} {
		require.True(t, present[status], "no case in the export is %s", status)
	}

	// A second environment, started from what the first wrote. Importing back
	// into the one that exported would prove only that the keeper can read its
	// own store.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	reexported, err := g.keeper.ExportGenesis(g.ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reexported,
		"derived state must match what was imported, byte for byte, or every upgrade rewrites the record")

	// The held seizure is not merely present, it still works: it is queued, and
	// it executes on the schedule it was decided on rather than being stranded.
	held, err := g.keeper.Case.Get(g.ctx, statuses[types.CASE_STATUS_HELD])
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_HELD, held.Status)
	require.True(t, g.keeper.IsFrozen(g.ctx, held.Target))

	g.runTo(t, held.ExecuteAtHeight)
	carried, err := g.keeper.Case.Get(g.ctx, held.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_PASSED, carried.Status,
		"a held seizure that lost its queue entry in the export would wait forever with its target frozen")
}

// Derived state must survive a round trip, including the state that is not in
// genesis at all.
//
// cases_passed is rebuilt by InitGenesis from the cases, so anything that
// increments it live and is not visible as CASE_STATUS_PASSED afterwards makes
// the chain disagree with its own export. That is not caught by comparing
// genesis to genesis — the counter is not in it — so it is asserted through the
// query, which is where anybody would actually notice the number was wrong.
func TestCasesPassedMatchesWhatAnImportRebuilds(t *testing.T) {
	f := initFixture(t)
	ombudsman := f.appointOmbudsman(t)

	// One seizure carried out, one still waiting, one vetoed while waiting.
	// Only the first has been carried out, so only the first should count.
	_, executedTarget := f.fundedAddr(t, coins(120_000))
	executedID := f.openAndPassSeizure(t, executedTarget)
	require.NotZero(t, executedID)

	_, waitingTarget := f.fundedAddr(t, coins(130_000))
	f.openAndHoldSeizure(t, waitingTarget)

	_, vetoedTarget := f.fundedAddr(t, coins(140_000))
	vetoedID := f.openAndHoldSeizure(t, vetoedTarget)
	_, err := f.ms.OmbudsmanVeto(f.ctx, &types.MsgOmbudsmanVeto{
		Ombudsman: ombudsman, CaseId: vetoedID, Reason: "the instrument names another account",
	})
	require.NoError(t, err)

	live, err := f.qs.Recovered(f.ctx, &types.QueryRecoveredRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), live.CasesPassed,
		"a seizure the validators agreed to has not been carried out until its delay expires")

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)

	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))

	imported, err := g.qs.Recovered(g.ctx, &types.QueryRecoveredRequest{})
	require.NoError(t, err)
	require.Equal(t, live.CasesPassed, imported.CasesPassed,
		"the count a chain carries must equal the count an import of its own export rebuilds")
	require.Equal(t, live.CasesOpened, imported.CasesOpened)
	require.Equal(t, live.Total, imported.Total)
}

// The window's ledger has to survive an upgrade, because an upgrade is exactly
// when a chain that wanted to seize more than a window allows would do it.
func TestTheRollingWindowSurvivesAnUpgrade(t *testing.T) {
	f := initFixture(t)
	f.setParams(t, func(p *types.Params) {
		p.SeizureDelayBlocks = 5
		p.SeizureWindowBlocks = 10_000
		p.MaxSeizuresPerWindow = 1
	})

	_, target := f.fundedAddr(t, coins(100_000))
	id := f.openAndHoldSeizure(t, target)
	f.executeHeld(t, id)

	exported, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	require.Len(t, exported.Seizures, 1)

	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.ctx, *exported))
	g.atHeight(f.env.Ctx.BlockHeight())

	window, err := g.qs.SeizureWindow(g.ctx, &types.QuerySeizureWindowRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), window.SeizureCount,
		"a window reset by the upgrade would let the cap be stepped over at the one moment nobody is watching")
	require.Equal(t, coins(100_000), window.Seized)
}

// ---------------------------------------------------------------------------
// The freeze lapse, which had never been verified on a running chain because
// the devnet's provisional freeze is twenty-four hours long.
// ---------------------------------------------------------------------------

// The freeze lapse had never been observed on the devnet, and the reason turns
// out not to be that twenty-four hours is a long time to watch.
//
// `Params.Validate` requires provisional_freeze_blocks >= voting_period_blocks,
// so the vote always ends first — and every outcome of a vote except passing
// lifts the freeze on the spot. A provisional freeze therefore cannot reach its
// own expiry height while the voting queue is intact: the case is resolved
// before it gets there, and a case that passed had its freeze made permanent.
//
// That makes expireFreezes a backstop rather than the ordinary path, which is
// worth knowing and worth keeping. It is what stands between an account and a
// permanent freeze if a queue entry is ever lost — a genesis import that
// dropped it, a migration, a bug in the resolution path. So the state it
// defends against is constructed here deliberately, because it cannot be
// reached by using the module correctly.
func TestAProvisionalFreezeLapsesEvenIfItsCaseIsNeverResolved(t *testing.T) {
	f := initFixture(t)
	validator := f.addValidator(t, 10)
	f.addValidator(t, 10)

	// Short enough to actually watch expire, and legal: the provisional freeze
	// is not shorter than the voting period.
	f.setParams(t, func(p *types.Params) {
		p.VotingPeriodBlocks = 30
		p.ProvisionalFreezeBlocks = 30
	})

	scammer, scammerStr := f.fundedAddr(t, coins(50_000))
	opened, err := f.ms.OpenCase(f.ctx, &types.MsgOpenCase{
		Opener: validator, Target: scammerStr, Action: types.CASE_ACTION_FREEZE, Reason: "suspected theft",
	})
	require.NoError(t, err)

	freeze, found, err := f.keeper.FreezeOf(f.ctx, scammerStr)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, f.env.Ctx.BlockHeight()+30, freeze.ExpiresAtHeight,
		"opening a case must set an expiry, not an indefinite freeze")

	// The resolution that would normally come never does: its queue entry is
	// gone. This is the failure the expiry exists for, and the only way to
	// reach it is to cause it.
	stored, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.NoError(t, f.keeper.VotingQueue.Remove(f.ctx,
		collections.Join(stored.VotingEndsAtHeight, stored.Id)))

	// One block before it lapses, the account still cannot send.
	f.runTo(t, freeze.ExpiresAtHeight-1)
	require.True(t, f.keeper.IsFrozen(f.ctx, scammerStr))
	elsewhere, _ := f.addr(t)
	require.ErrorIs(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(1)), types.ErrFrozen)

	// At the expiry height it lapses, with nobody having done anything and the
	// case never having been decided.
	f.runTo(t, freeze.ExpiresAtHeight)
	require.False(t, f.keeper.IsFrozen(f.ctx, scammerStr),
		"a freeze one validator imposed alone must expire by itself even if nothing ever resolves the case")
	require.NoError(t, f.env.BankKeeper.SendCoins(f.ctx, scammer, elsewhere, coins(50_000)))

	// The freeze lapsed; the accusation did not silently resolve itself into a
	// finding. Silence is not a finding, and the record should not claim it was.
	after, err := f.keeper.Case.Get(f.ctx, opened.Id)
	require.NoError(t, err)
	require.Equal(t, types.CASE_STATUS_VOTING, after.Status)
}

// ---------------------------------------------------------------------------

func mustBytes(t *testing.T, f *fixture, addr string) []byte {
	t.Helper()
	bz, err := f.env.AddressCodec.StringToBytes(addr)
	require.NoError(t, err)
	return bz
}

func mustAddr(t *testing.T, f *fixture, addr string) sdk.AccAddress {
	t.Helper()
	return sdk.AccAddress(mustBytes(t, f, addr))
}

// mustNewAddr returns a fresh account that may act: placed, and granted the
// enforcement role. Its one caller uses it as the emergency authority.
func mustNewAddr(t *testing.T, f *fixture) string {
	t.Helper()
	_, s := f.addr(t)
	f.grantEnforcement(t, s)
	return s
}

// t2ctx is the fixture's context, named for the one assertion that needs it
// spelled out rather than taken from the receiver.
func t2ctx(f *fixture) sdk.Context { return f.env.Ctx }
