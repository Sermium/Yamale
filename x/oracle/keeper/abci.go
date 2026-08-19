package keeper

import (
	"context"
	"sort"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/oracle/types"
)

// EndBlocker closes a voting round.
//
// Rates are agreed on a period boundary rather than on every block for two
// reasons. Aggregating once means every validator's report for the round is
// visible at the same time, so no one can see the others' prices before
// choosing their own. And it bounds the work: a tally is O(validators × denoms)
// and running it every block, at every height, to move a price by a fraction of
// a percent is cost without benefit.
func (k Keeper) EndBlocker(ctx context.Context) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	// A zero vote period stops rounds; it must not stop the chain.
	//
	// Params.Validate() rejects zero, so neither MsgUpdateParams nor a genesis
	// that was actually validated can produce it. What can is a genesis file
	// edited after `validate-genesis` — which is what every launch ceremony
	// does, including this chain's own scripts. The modulo below is an integer
	// division, and dividing by zero is a panic rather than an error: the chain
	// would fail to produce its first block, and no transaction could correct it
	// because there would be no chain to send one to.
	if params.VotePeriod == 0 {
		return nil
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()
	if height < 1 || uint64(height)%params.VotePeriod != 0 {
		return nil
	}

	powers, totalPower, err := k.bondedPowers(ctx)
	if err != nil {
		return err
	}

	// Votes are read once and grouped by denom in memory. Iterating the whole
	// collection per denom would re-read every vote for every accepted currency.
	votesByDenom, reported, err := k.collectVotes(ctx, powers)
	if err != nil {
		return err
	}

	now := sdkCtx.BlockTime().Unix()

	// Iterating params.AcceptedDenoms — a stored, ordered slice — rather than
	// the map keeps the write order identical on every node. A map's iteration
	// order is deliberately random in Go, and a state machine whose writes vary
	// between validators does not reach consensus.
	for _, denom := range params.AcceptedDenoms {
		result := types.AggregateRate(votesByDenom[denom], totalPower, params.VoteThresholdBps)
		if !result.Accepted {
			// Too little stake reported. The previous rate stands and keeps
			// ageing, which is what makes a feed that stops reporting eventually
			// unusable rather than frozen at its last value.
			continue
		}

		if err := k.ExchangeRate.Set(ctx, denom, types.ExchangeRate{
			Denom:          denom,
			Rate:           result.Rate.String(),
			UpdatedAt:      now,
			UpdatedHeight:  height,
			VotingPowerBps: result.PowerBps,
		}); err != nil {
			return err
		}
	}

	if err := k.recordReliability(ctx, powers, reported); err != nil {
		return err
	}

	// Votes describe one round. Clearing them means the next round is decided
	// on reports somebody still stands behind, and that an absent validator is
	// counted as absent rather than as repeating its last price forever.
	return k.clearVotes(ctx)
}

// bondedPowers returns the consensus power of every bonded validator and the
// total, both as of the last completed block.
func (k Keeper) bondedPowers(ctx context.Context) (map[string]int64, int64, error) {
	powers := make(map[string]int64)
	var total int64

	err := k.stakingKeeper.IterateBondedValidatorsByPower(ctx, func(_ int64, validator stakingtypes.ValidatorI) bool {
		power := validator.GetConsensusPower(sdk.DefaultPowerReduction)
		if power <= 0 {
			return false
		}
		powers[validator.GetOperator()] = power
		total += power
		return false
	})
	if err != nil {
		return nil, 0, err
	}

	return powers, total, nil
}

// collectVotes reads the round's votes once, grouping them by denom and noting
// which validators reported at all.
//
// A vote from an address that is not currently bonded is discarded rather than
// counted at zero weight: a validator that was jailed or unbonded between
// voting and the tally has no stake behind its opinion any more.
func (k Keeper) collectVotes(ctx context.Context, powers map[string]int64) (map[string][]types.WeightedVote, map[string]bool, error) {
	byDenom := make(map[string][]types.WeightedVote)
	reported := make(map[string]bool)

	iter, err := k.Vote.Iterate(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		vote, err := iter.Value()
		if err != nil {
			return nil, nil, err
		}

		power, bonded := powers[vote.Validator]
		if !bonded {
			continue
		}

		rate, err := math.LegacyNewDecFromStr(vote.Rate)
		if err != nil {
			// Handlers reject unparseable rates, so this is unreachable from a
			// transaction. Skipping rather than failing keeps a corrupted entry
			// from halting every future round.
			continue
		}

		reported[vote.Validator] = true
		byDenom[vote.Denom] = append(byDenom[vote.Denom], types.WeightedVote{
			Validator: vote.Validator,
			Rate:      rate,
			Power:     power,
		})
	}

	return byDenom, reported, nil
}

// recordReliability counts, per bonded validator, how many rounds it was asked
// to report in and how many it missed.
//
// Nothing is slashed. On a small permissioned network an automatic penalty
// mostly punishes the operator whose machine rebooted, and a known, accountable
// validator set handles absence better than an incentive that fires during an
// outage. Governance can add a penalty later using exactly this record; it
// cannot easily undo one that fired wrongly.
func (k Keeper) recordReliability(ctx context.Context, powers map[string]int64, reported map[string]bool) error {
	// The map is not iterated directly: writes must land in the same order on
	// every node, and Go randomises map order.
	validators := make([]string, 0, len(powers))
	for validator := range powers {
		validators = append(validators, validator)
	}
	sort.Strings(validators)

	for _, validator := range validators {
		counter, err := k.MissCounter.Get(ctx, validator)
		if err != nil {
			counter = types.MissCounter{Validator: validator}
		}

		counter.Windows++
		if !reported[validator] {
			counter.Misses++
		}

		if err := k.MissCounter.Set(ctx, validator, counter); err != nil {
			return err
		}
	}

	return nil
}

// clearVotes empties the round.
func (k Keeper) clearVotes(ctx context.Context) error {
	return k.Vote.Clear(ctx, nil)
}
