package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/validatorgov/types"
)

// JurisdictionReconciliation sets each approved validator's declared country
// beside the country the chain recorded for its operator account.
//
// Two registries hold that fact and neither is the other's source. The
// declaration is signed by the operator and is what the jurisdiction ceiling
// groups by; the record in x/alias was written by the participant that onboarded
// the account and did the know-your-customer work, or corrected by a foundation
// administrator, and never by the account itself. See the RPC's comment in
// query.proto for why they are reconciled rather than merged.
//
// Every approved validator is returned, agreements included. A supervisory query
// that answered with an empty list when all was well would be indistinguishable
// from one that was broken or pointed at an empty registry, and this is exactly
// the query whose silence would be believed.
//
// It reads both registries and compares two strings. There is no arithmetic here
// to get wrong and no second implementation of anything: the declaration comes
// out of the same ApprovedValidator record the epoch check reads, and the record
// out of x/alias's own collection.
func (q queryServer) JurisdictionReconciliation(ctx context.Context, req *types.QueryJurisdictionReconciliationRequest) (*types.QueryJurisdictionReconciliationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}

	// Fails closed, and this is the whole reason the check is here rather than
	// left to a nil dereference. With no registry to consult, every validator
	// would come back UNRECORDED — a plausible-looking answer, forty rows long,
	// that says nothing about the chain and everything about the wiring. A
	// comparison that cannot be made is refused, so that a wiring mistake costs
	// the operator an error rather than a false clean bill of health.
	if q.k.aliasKeeper == nil {
		return nil, status.Error(codes.FailedPrecondition,
			"the jurisdiction registry is not wired into this module, so no reconciliation can be made")
	}

	records := make([]types.JurisdictionReconciliation, 0)
	var agree, disagree, unrecorded uint32

	// Walked in key order, which is the operator address, so two nodes answering
	// the same query return the same list in the same order.
	err := q.k.ApprovedValidator.Walk(ctx, nil, func(candidate string, approved types.ApprovedValidator) (bool, error) {
		record, found, err := q.k.aliasKeeper.JurisdictionOf(ctx, candidate)
		if err != nil {
			return true, err
		}

		row := types.JurisdictionReconciliation{
			Candidate:            candidate,
			DeclaredJurisdiction: approved.Declaration.Jurisdiction,
			// UNRECORDED until a record is found. The default has to be the
			// state that claims the least, because it is the one a missing
			// lookup produces.
			Agreement: types.JURISDICTION_AGREEMENT_UNRECORDED,
		}

		switch {
		case !found:
			unrecorded++

		// Both sides are normalised before they are compared, and reported as
		// they are stored. Each module uppercases on write, so this changes
		// nothing on a chain that has only ever been written to through its
		// messages; it is here so that a value carried in by an older import
		// cannot be reported as a disagreement between a country and itself.
		// Normalising cannot manufacture agreement between two different
		// countries, which is the only direction that would matter.
		case aliastypes.NormaliseCountry(record.Country) == aliastypes.NormaliseCountry(approved.Declaration.Jurisdiction):
			row.Agreement = types.JURISDICTION_AGREEMENT_AGREE
			agree++

		default:
			row.Agreement = types.JURISDICTION_AGREEMENT_DISAGREE
			disagree++
		}

		if found {
			// Reported for the disagreements and the agreements alike. "Who says
			// so" is what makes the record evidence rather than an opinion, and
			// an agreement nobody is named for is worth less than one that is.
			row.RecordedJurisdiction = record.Country
			row.RecordedBy = record.RecordedBy
			row.RecordedAtHeight = record.RecordedAtHeight
		}

		records = append(records, row)
		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryJurisdictionReconciliationResponse{
		Records:         records,
		AgreeCount:      agree,
		DisagreeCount:   disagree,
		UnrecordedCount: unrecorded,
	}, nil
}
