package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/types"
)

// The register is what makes a concentration ceiling computable at all, so the
// tests here are about what the chain refuses to record rather than about what
// it does with a record.

func TestApplicationWithoutADeclarationIsRefused(t *testing.T) {
	f := initFixture(t)
	ms := keeperMsgServer(f)
	_, candidate := f.env.Addr(t)

	cases := map[string]types.MsgApplyValidator{
		"no entity": {
			Creator: candidate, BeneficialOwnerId: "OWNER", Jurisdiction: "CH",
		},
		"no beneficial owner": {
			Creator: candidate, LegalEntityId: "ENTITY", Jurisdiction: "CH",
		},
		"no jurisdiction": {
			Creator: candidate, LegalEntityId: "ENTITY", BeneficialOwnerId: "OWNER",
		},
		// Two letters is not a country. A code naming nowhere would create a
		// jurisdiction ceiling no authority holds and exactly one validator is
		// ever measured against.
		"jurisdiction that is not a country": {
			Creator: candidate, LegalEntityId: "ENTITY", BeneficialOwnerId: "OWNER", Jurisdiction: "QK",
		},
		// The foundation's reserved code is not a national perimeter, and a
		// validator declaring it would be declaring exemption from the ceiling
		// that keeps any one state below a blocking minority.
		"the foundation's reserved code": {
			Creator: candidate, LegalEntityId: "ENTITY", BeneficialOwnerId: "OWNER", Jurisdiction: "ZZ",
		},
	}

	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ms.ApplyValidator(f.env.Ctx, &msg)
			require.ErrorIs(t, err, types.ErrInvalidDeclaration)
		})
	}
}

// A lowercase country code and a padded identifier are the same declaration as
// their tidy forms. The ceilings group by exact string equality, so " ACME" and
// "ACME" would otherwise be two entities holding a fifth each rather than one
// holding two fifths — a cap defeated by a space bar.
func TestDeclarationIsNormalisedBeforeItIsStored(t *testing.T) {
	f := initFixture(t)
	ms := keeperMsgServer(f)
	_, candidate := f.env.Addr(t)

	_, err := ms.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{
		Creator:           candidate,
		LegalEntityId:     "  ACME  ",
		BeneficialOwnerId: " ACME HOLDINGS ",
		Jurisdiction:      "ch",
	})
	require.NoError(t, err)

	application, err := f.keeper.ValidatorApplication.Get(f.env.Ctx, candidate)
	require.NoError(t, err)
	require.Equal(t, "ACME", application.Declaration.LegalEntityId)
	require.Equal(t, "ACME HOLDINGS", application.Declaration.BeneficialOwnerId)
	require.Equal(t, "CH", application.Declaration.Jurisdiction)
}

// Approval copies the declaration onto the record the ceilings actually read.
func TestApprovalCarriesTheDeclarationOntoTheAllowlist(t *testing.T) {
	f := initFixture(t)
	_, operator := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	approved, err := f.keeper.ApprovedValidator.Get(f.env.Ctx, operator)
	require.NoError(t, err)
	require.Equal(t, "ENTITY-A", approved.Declaration.LegalEntityId)
	require.Equal(t, "OWNER-A", approved.Declaration.BeneficialOwnerId)
	require.Equal(t, "CH", approved.Declaration.Jurisdiction)
}

// Re-attestation updates the approval and leaves the application alone. The two
// are meant to diverge: the application is what the set voted for and the
// approval is what is claimed now, and a register holding only one of them
// cannot show that anything changed.
func TestAttestationUpdatesTheApprovalAndNotTheApplication(t *testing.T) {
	f := initFixture(t)
	_, operator := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	ctx := f.env.Ctx.WithBlockHeight(5_000)
	_, err := keeperMsgServer(f).AttestOwnership(ctx, &types.MsgAttestOwnership{
		Creator:           operator,
		LegalEntityId:     "ENTITY-A",
		BeneficialOwnerId: "STATE-BANK",
		Jurisdiction:      "CH",
	})
	require.NoError(t, err)

	approved, err := f.keeper.ApprovedValidator.Get(f.env.Ctx, operator)
	require.NoError(t, err)
	require.Equal(t, "STATE-BANK", approved.Declaration.BeneficialOwnerId)
	require.Equal(t, int64(5_000), approved.Declaration.AttestedAtHeight)

	application, err := f.keeper.ValidatorApplication.Get(f.env.Ctx, operator)
	require.NoError(t, err)
	require.Equal(t, "OWNER-A", application.Declaration.BeneficialOwnerId,
		"the admission record must keep what the set actually voted for")
}

// The declared owner is what the ceilings group by, so a re-attestation that
// moves an operator under an existing owner is an event the epoch check has to
// act on. This is the merger case: nothing about it arrives as a power change.
func TestReattestingIntoAnotherOwnerBreachesAtTheNextEpoch(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, 2_500, noCeiling, 1)

	_, first := f.admit(t, "SUBSIDIARY-A", "STATE-BANK", "CH", 1)
	_, second := f.admit(t, "INDEPENDENT-B", "INDEPENDENT-B", "CH", 1)
	for i := 0; i < 3; i++ {
		f.admit(t, "OTHER-"+string(rune('A'+i)), "OTHER-OWNER-"+string(rune('A'+i)), "ZA", 1)
	}

	f.epoch(t, 1)
	require.False(t, f.demoted(t, first))
	require.False(t, f.demoted(t, second))

	// The acquisition, declared honestly.
	_, err := keeperMsgServer(f).AttestOwnership(f.env.Ctx.WithBlockHeight(15), &types.MsgAttestOwnership{
		Creator:           second,
		LegalEntityId:     "INDEPENDENT-B",
		BeneficialOwnerId: "STATE-BANK",
		Jurisdiction:      "CH",
	})
	require.NoError(t, err)

	f.epoch(t, 2)

	require.True(t, f.demoted(t, first) != f.demoted(t, second),
		"exactly one of the two must lose its seat, bringing the owner back inside the ceiling")
}

func TestOnlyAnApprovedOperatorMayAttest(t *testing.T) {
	f := initFixture(t)
	_, stranger := f.env.Addr(t)

	_, err := keeperMsgServer(f).AttestOwnership(f.env.Ctx, &types.MsgAttestOwnership{
		Creator: stranger, LegalEntityId: "E", BeneficialOwnerId: "O", Jurisdiction: "CH",
	})
	require.ErrorIs(t, err, types.ErrNotApprovedValidator)
}

// A declaration nobody has re-signed for is reported as stale at every epoch.
// Nothing is done to the validator: the chain cannot verify a declaration, so
// publishing the date is the most it can honestly do.
func TestStaleDeclarationIsReportedAndNotEnforced(t *testing.T) {
	f := initFixture(t)
	f.caps(t, noCeiling, noCeiling, noCeiling, 1)

	params, err := f.keeper.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	params.AttestationIntervalBlocks = 5
	require.NoError(t, f.keeper.Params.Set(f.env.Ctx, params))

	_, operator := f.admit(t, "ENTITY-A", "OWNER-A", "CH", 1)

	f.epoch(t, 1)
	require.False(t, f.demoted(t, operator), "a stale declaration is not a demotion")

	stale := 0
	for _, event := range f.env.Ctx.EventManager().Events() {
		if event.Type == "blockchain.validatorgov.v1.EventDeclarationStale" {
			stale++
		}
	}
	require.Positive(t, stale, "a declaration nobody has re-signed for has to be visible as stale")
}

// A zero interval reaching the epoch check from an upgraded store would report
// every validator on the chain as stale in the same block.
func TestZeroAttestationIntervalReportsNothing(t *testing.T) {
	require.False(t, types.Declaration{}.IsStale(1_000_000, 0))
}

// A declaration that has never been attested is stale from the first epoch.
// "Nobody has ever signed for this" is a stronger version of "nobody has signed
// recently", and treating it as fresh would exempt the records with the least
// behind them.
func TestNeverAttestedIsStale(t *testing.T) {
	require.True(t, types.Declaration{}.IsStale(11, 10))
	require.False(t, types.Declaration{}.IsStale(10, 10))
}
