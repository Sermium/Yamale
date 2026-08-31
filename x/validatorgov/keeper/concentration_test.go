package keeper_test

import (
	"fmt"
	"math/rand"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/types"
)

// The concentration ceilings, tested against the states they exist for.
//
// Every one of these arranges a validator set and then runs the epoch check,
// because the claim being tested is never "the arithmetic is right" — it is
// "the chain corrected a concentration nobody proposed". An admission-time test
// would pass against a module that did nothing at all after admission, which is
// exactly the hole the epoch check was written to close.

const (
	// testEpoch is short so a test can reach an epoch boundary in one line.
	testEpoch = 10

	// noCeiling is what a test passes for the ceilings it is not about. A cap
	// of ten thousand basis points allows the whole set, which is the only
	// value that genuinely disables one — anything less is a real ceiling at
	// the small set sizes these tests use, and it was quietly demoting
	// validators the tests were not looking at.
	noCeiling = 10_000
)

// caps installs a settlement with the ceilings a test cares about, written
// straight into the constitution's store.
//
// Direct rather than through an amendment: an amendment takes three weeks of
// blocks by construction, and what is being tested here is the enforcement, not
// the path a change takes to arrive.
func (f *fixture) caps(t *testing.T, entityBps, ownerBps, jurisdictionBps uint64, minActive uint32) {
	t.Helper()

	inv := f.invariants
	inv.MaxEntityPowerBps = entityBps
	inv.MaxBeneficialOwnerPowerBps = ownerBps
	inv.MaxJurisdictionPowerBps = jurisdictionBps
	inv.MinActiveValidators = minActive
	inv.ConcentrationEpochBlocks = testEpoch

	require.NoError(t, f.constitution.Invariants.Set(f.env.Ctx, inv))
}

// admit runs the whole admission path — apply, govern, create the validator —
// and returns the operator address in the account form every record here is
// keyed by.
func (f *fixture) admit(t *testing.T, entity, owner, jurisdiction string, seats int64) (sdk.AccAddress, string) {
	t.Helper()

	ms := keeperMsgServer(f)
	account, accountStr := f.env.Addr(t)

	_, err := ms.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{
		Creator:           accountStr,
		Moniker:           entity,
		Description:       "admitted in a test",
		LegalEntityId:     entity,
		BeneficialOwnerId: owner,
		Jurisdiction:      jurisdiction,
	})
	require.NoError(t, err)

	_, err = ms.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: accountStr, Approve: true,
	})
	require.NoError(t, err)

	f.staking.AddValidatorWithSeats(account, seats)
	return account, accountStr
}

// epoch runs the check at a height that is an epoch boundary.
func (f *fixture) epoch(t *testing.T, n int64) {
	t.Helper()
	require.NoError(t, f.keeper.ConcentrationEndBlocker(f.env.Ctx.WithBlockHeight(n*testEpoch)))
}

func (f *fixture) demoted(t *testing.T, operator string) bool {
	t.Helper()
	has, err := f.keeper.Demotion.Has(f.env.Ctx, operator)
	require.NoError(t, err)
	return has
}

// A set that is inside its ceilings is left completely alone. Without this the
// other tests would all pass against a check that demoted everybody.
func TestValidatorAdmittedWithinCapKeepsItsSeats(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, 2_500, 3_400, 1)

	countries := []string{"CH", "ZA", "SG", "GB", "SN"}
	operators := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		_, operator := f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), countries[i], 1)
		operators = append(operators, operator)
	}

	f.epoch(t, 1)

	for _, operator := range operators {
		require.False(t, f.demoted(t, operator), "a set inside every ceiling must not be touched")
		addr, err := f.env.AddressCodec.StringToBytes(operator)
		require.NoError(t, err)
		require.False(t, f.staking.IsJailed(addr))
	}
}

// The case the whole design turns on. Nothing was proposed, nobody voted, and
// the validator was inside its ceiling when it was admitted — it simply grew.
// An admission-time check sees none of this.
func TestValidatorThatGrowsAfterAdmissionIsDemotedAtTheNextEpoch(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, 2_500, noCeiling, 1)

	grower, growerStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 1)
	for i := 0; i < 4; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	f.epoch(t, 1)
	require.False(t, f.demoted(t, growerStr), "one seat in five is exactly the ceiling, not over it")

	// The acquisition: the same entity now carries three seats. Nothing about
	// this arrives as a message the chain could have refused.
	f.staking.SetSeats(grower, 3)

	f.epoch(t, 2)

	require.True(t, f.demoted(t, growerStr), "a breach that arrived by growth must still be corrected")
	require.True(t, f.staking.IsJailed(grower))

	demotion, err := f.keeper.Demotion.Get(f.env.Ctx, growerStr)
	require.NoError(t, err)
	require.Equal(t, types.CONCENTRATION_CAP_ENTITY, demotion.Cap)
	require.Equal(t, "ENTITY-BIG", demotion.Group)
	require.Equal(t, uint64(2_000), demotion.CapBps)
	require.True(t, demotion.JailedValidator)
}

// Two legal entities, one owner. Each is inside the entity ceiling; together
// they are over the owner ceiling, which is the ceiling that matters — an owner
// admitted twice votes twice.
func TestTwoEntitiesUnderOneBeneficialOwnerBreachJointly(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, 2_500, noCeiling, 1)

	_, firstStr := f.admit(t, "SUBSIDIARY-A", "STATE-BANK", "CH", 1)
	_, secondStr := f.admit(t, "SUBSIDIARY-B", "STATE-BANK", "CH", 1)
	for i := 0; i < 3; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "ZA", 1)
	}

	// Two of five is 4000 basis points against an owner ceiling of 2500.
	f.epoch(t, 1)

	demotedCount := 0
	for _, operator := range []string{firstStr, secondStr} {
		if f.demoted(t, operator) {
			demotedCount++
			demotion, err := f.keeper.Demotion.Get(f.env.Ctx, operator)
			require.NoError(t, err)
			require.Equal(t, types.CONCENTRATION_CAP_BENEFICIAL_OWNER, demotion.Cap)
			require.Equal(t, "STATE-BANK", demotion.Group)
		}
	}
	require.Equal(t, 1, demotedCount,
		"one demotion brings the owner from two of five to one of five, which is inside the ceiling")
}

// A jurisdiction ceiling is not an entity ceiling with a different name: it
// binds across entities that have nothing to do with each other except the
// authority they answer to.
func TestJurisdictionBreachAcrossThreeSeparateEntities(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, 3_400, 1)

	local := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		_, operator := f.admit(t, fmt.Sprintf("BANK-%d", i), fmt.Sprintf("OWNER-%d", i), "ZA", 1)
		local = append(local, operator)
	}
	for i := 0; i < 3; i++ {
		f.admit(t, fmt.Sprintf("FOREIGN-%d", i), fmt.Sprintf("FOREIGN-OWNER-%d", i), "CH", 1)
	}

	// Three of six is 5000 basis points against a ceiling of 3400, which allows
	// 2040 — two seats.
	f.epoch(t, 1)

	demoted := 0
	for _, operator := range local {
		if f.demoted(t, operator) {
			demoted++
			demotion, err := f.keeper.Demotion.Get(f.env.Ctx, operator)
			require.NoError(t, err)
			require.Equal(t, types.CONCENTRATION_CAP_JURISDICTION, demotion.Cap)
			require.Equal(t, "ZA", demotion.Group)
		}
	}
	require.Equal(t, 1, demoted, "two of six is 3333 basis points, inside a 3400 ceiling")
}

// Restoration is automatic. Requiring a vote to come back would make a
// concentration ceiling an expulsion, decided by the validators who gained from
// it.
func TestDemotionRestoresWhenTheBreachClears(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, noCeiling, noCeiling, 1)

	grower, growerStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 3)
	for i := 0; i < 4; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	f.epoch(t, 1)
	require.True(t, f.demoted(t, growerStr))
	require.True(t, f.staking.IsJailed(grower))

	// The entity restructures down to one seat. Nothing else changes.
	f.staking.SetSeats(grower, 1)

	f.epoch(t, 2)

	require.False(t, f.demoted(t, growerStr), "the record must go when the breach does")
	require.False(t, f.staking.IsJailed(grower), "the seats must come back without anybody voting")
}

// A validator already jailed for something else must stay jailed when the
// breach clears, or a concentration ceiling becomes a way of clearing somebody
// else's downtime.
//
// The state is arranged directly rather than by jailing and then demoting,
// because x/staking takes a jailed validator out of the power index at the
// moment it jails it — so the epoch check never sees one. Where the combination
// does arrive is an import: a genesis carrying a demotion whose validator has
// since been jailed for downtime, which is exactly what this writes.
func TestRestorationDoesNotUnjailWhatItDidNotJail(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, noCeiling, noCeiling, 1)

	held, heldStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 1)
	for i := 0; i < 4; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	require.NoError(t, f.keeper.Demotion.Set(f.env.Ctx, heldStr, types.Demotion{
		Operator:        heldStr,
		Cap:             types.CONCENTRATION_CAP_ENTITY,
		Group:           "ENTITY-BIG",
		CapBps:          2_000,
		DemotedAtHeight: 1,
		JailedValidator: false,
	}))
	require.NoError(t, f.staking.Jail(f.env.Ctx, f.consAddrOf(t, held)))

	f.epoch(t, 1)

	require.False(t, f.demoted(t, heldStr), "the breach has cleared, so the record goes")
	require.True(t, f.staking.IsJailed(held), "the downtime jail is not this module's to lift")
}

// A ceiling that cannot be satisfied at the current set size is published and
// left alone. Enforcement must never be the thing that stops block production.
func TestBreachIsReportedNotCorrectedAtTheFloor(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, noCeiling, noCeiling, 3)

	operators := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		_, operator := f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
		operators = append(operators, operator)
	}

	// One of three is 3333 basis points, over a 2000 ceiling — for all three of
	// them, and there is no set of demotions that fixes it.
	f.epoch(t, 1)

	for _, operator := range operators {
		require.False(t, f.demoted(t, operator),
			"a chain must keep producing blocks even when its ceilings cannot be met")
	}

	found := false
	for _, event := range f.env.Ctx.EventManager().Events() {
		if event.Type == "blockchain.validatorgov.v1.EventConcentrationUncorrected" {
			found = true
		}
	}
	require.True(t, found, "an uncorrectable breach has to be visible, since nothing else will show it")
}

// The check runs at an epoch and not on every block, and a height that is not a
// boundary must leave the state completely alone.
func TestCheckDoesNotRunBetweenEpochs(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, noCeiling, noCeiling, 1)

	_, growerStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 3)
	for i := 0; i < 4; i++ {
		f.admit(t, fmt.Sprintf("ENTITY-%d", i), fmt.Sprintf("OWNER-%d", i), "CH", 1)
	}

	require.NoError(t, f.keeper.ConcentrationEndBlocker(f.env.Ctx.WithBlockHeight(testEpoch-1)))
	require.False(t, f.demoted(t, growerStr))

	f.epoch(t, 1)
	require.True(t, f.demoted(t, growerStr))
}

// A zero epoch length is a modulus of zero inside an end blocker, which halts
// the chain in the first block. Validate refuses one, but Validate has not been
// sufficient protection on this chain before, so the divisor is guarded where it
// is used.
func TestZeroEpochLengthDoesNotHaltTheChain(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, noCeiling, noCeiling, 1)

	inv := f.invariants
	inv.ConcentrationEpochBlocks = 0
	require.NoError(t, f.constitution.Invariants.Set(f.env.Ctx, inv))

	for height := int64(1); height <= 3; height++ {
		require.NoError(t, f.keeper.ConcentrationEndBlocker(f.env.Ctx.WithBlockHeight(height)),
			"a zero epoch length must disable the check, not stop the chain")
	}
}

// A validator with no approval record — a genesis validator from the gentx
// ceremony — is counted in the denominator and belongs to no group. Leaving it
// out of the total would inflate everybody else's share and demote validators
// for power a third party holds.
func TestUndeclaredValidatorCountsInTheTotalAndInNoGroup(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 3_400, noCeiling, noCeiling, 1)

	_, declaredStr := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)
	f.admit(t, "ENTITY-B", "OWNER-B", "CH", 1)

	// A third validator nobody admitted through this module.
	stranger, _ := f.env.Addr(t)
	f.staking.AddValidatorWithSeats(stranger, 1)

	// One of three is 3333, inside a 3400 ceiling — but only because the
	// undeclared validator is in the denominator.
	f.epoch(t, 1)
	require.False(t, f.demoted(t, declaredStr))
}

// Determinism is not negotiable: every node has to compute the same demotions
// from the same set. The ordering here comes from an explicit sort and not from
// any map, and this is what says so.
func TestPlanIsDeterministicUnderShuffledInput(t *testing.T) {
	holders := make([]types.SeatHolder, 0, 12)
	for i := 0; i < 12; i++ {
		holders = append(holders, types.SeatHolder{
			Operator: fmt.Sprintf("operator-%02d", i),
			Power:    int64(1 + i%3),
			Declaration: types.Declaration{
				LegalEntityId:     fmt.Sprintf("ENTITY-%d", i%4),
				BeneficialOwnerId: fmt.Sprintf("OWNER-%d", i%2),
				Jurisdiction:      []string{"CH", "ZA", "SG"}[i%3],
			},
		})
	}
	caps := types.CapSet{Entity: 1_500, BeneficialOwner: 3_000, Jurisdiction: 4_000, MinActive: 1}

	want, wantBreaches := types.Plan(holders, caps)
	require.NotEmpty(t, want, "a plan that demotes nothing would prove nothing about ordering")

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		shuffled := append([]types.SeatHolder(nil), holders...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })

		got, gotBreaches := types.Plan(shuffled, caps)
		require.Equal(t, want, got, "the demotion plan must not depend on the order the set arrived in")
		require.Equal(t, wantBreaches, gotBreaches)
	}
}

// The groups come back in a fixed order — every ceiling in the order the
// ceilings are applied, and alphabetically within each.
//
// This is separate from the shuffle test above and it is not redundant with it.
// Shuffling the input proves the *result* does not depend on the caller's
// order; it does not pin the order the groups themselves come out in, and that
// order decides which group's breach is corrected first when two of them
// breach at once. A demotion plan that was deterministic but arbitrary would
// still be a plan nobody could predict from the register.
func TestHoldingsAreReturnedInAFixedOrder(t *testing.T) {
	holders := []types.SeatHolder{
		{Operator: "op-a", Power: 1, Declaration: types.Declaration{
			LegalEntityId: "ZEBRA", BeneficialOwnerId: "OWNER-Z", Jurisdiction: "ZA"}},
		{Operator: "op-b", Power: 2, Declaration: types.Declaration{
			LegalEntityId: "ALPHA", BeneficialOwnerId: "OWNER-A", Jurisdiction: "CH"}},
		{Operator: "op-c", Power: 1, Declaration: types.Declaration{
			LegalEntityId: "MIKE", BeneficialOwnerId: "OWNER-A", Jurisdiction: "CH"}},
	}

	got := make([][2]string, 0)
	for _, holding := range types.Holdings(holders) {
		got = append(got, [2]string{holding.Cap.String(), holding.Group})
	}

	require.Equal(t, [][2]string{
		{"CONCENTRATION_CAP_ENTITY", "ALPHA"},
		{"CONCENTRATION_CAP_ENTITY", "MIKE"},
		{"CONCENTRATION_CAP_ENTITY", "ZEBRA"},
		{"CONCENTRATION_CAP_BENEFICIAL_OWNER", "OWNER-A"},
		{"CONCENTRATION_CAP_BENEFICIAL_OWNER", "OWNER-Z"},
		{"CONCENTRATION_CAP_JURISDICTION", "CH"},
		{"CONCENTRATION_CAP_JURISDICTION", "ZA"},
	}, got)
}

// The share arithmetic divides by the total power, which is zero on a chain
// whose validators are all jailed or not yet bonded.
func TestShareArithmeticGuardsAZeroTotal(t *testing.T) {
	require.Equal(t, uint64(0), constitutiontypes.PowerBps(5, 0))
	require.Equal(t, int64(0), constitutiontypes.AllowedPower(0, 2_000))
	require.Equal(t, int64(0), constitutiontypes.AllowedPower(100, 0))

	plans, breaches := types.Plan(nil, types.CapSet{Entity: 2_000, MinActive: 1})
	require.Empty(t, plans)
	require.Empty(t, breaches)
}

// The end blocker survives a params store that has never been written.
//
// Audit finding 3.7 (Low). ConcentrationEndBlocker tolerates absent params and
// proceeds with a zero-valued Params, relying on AttestationInterval() to read
// zero as "use the default". Not returning an error is right — the alternative
// halts the chain over a missing store entry — but the reliance on an accessor
// behaving well was inferred from reading it rather than asserted anywhere.
//
// The state is reachable: a module added to a running chain by an upgrade whose
// migration did not write params starts exactly here, which is not a
// hypothetical on a chain that has taken four upgrades.
func TestConcentrationEndBlockerSurvivesAnEmptyParamsStore(t *testing.T) {
	f := newFixture(t, false)

	// Emptied deliberately: even the no-genesis fixture writes DefaultParams,
	// so without this the test would pass while exercising the ordinary path
	// and prove nothing about the one it names.
	require.NoError(t, f.keeper.Params.Remove(f.env.Ctx))
	_, err := f.keeper.Params.Get(f.env.Ctx)
	require.Error(t, err, "the params store is not empty, so this test is not testing what it says")

	// Every height, not just a boundary: the divisor guard and the interval
	// accessor are on different paths and only one of them runs off-epoch.
	for _, height := range []int64{1, testEpoch - 1, testEpoch, testEpoch + 1, testEpoch * 3} {
		require.NoError(t,
			f.keeper.ConcentrationEndBlocker(f.env.Ctx.WithBlockHeight(height)),
			"an empty params store halted the chain at height %d", height)
	}
}

// And the accessor it relies on reads zero as the default rather than as an
// interval of zero, which would make every declaration stale in the same block.
func TestAZeroAttestationIntervalMeansTheDefaultNotZero(t *testing.T) {
	var empty types.Params
	require.Equal(t, uint64(types.DefaultAttestationIntervalBlocks), empty.AttestationInterval(),
		"a zero interval read as zero would mark every declaration stale immediately")
	require.Positive(t, empty.AttestationInterval(),
		"an interval of zero is also a modulus of zero for anything that divides by it")
}
