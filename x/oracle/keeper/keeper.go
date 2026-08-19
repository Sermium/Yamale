package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/oracle/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	// Address capable of executing a MsgUpdateParams message.
	// Typically, this should be the x/gov module account.
	authority []byte

	stakingKeeper types.StakingKeeper
	nftKeeper     types.NFTKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	// ExchangeRate is the agreed price per denom.
	ExchangeRate collections.Map[string, types.ExchangeRate]

	// Vote holds the current round's reports, keyed by (validator, denom).
	// Cleared at the end of every round.
	Vote collections.Map[collections.Pair[string, string], types.ExchangeRateVote]

	// Feeder maps a validator to the hot key allowed to vote for it.
	Feeder collections.Map[string, string]

	// MissCounter records reporting reliability per validator.
	MissCounter collections.Map[string, types.MissCounter]

	// Appraiser holds valuers and applicants, keyed by address.
	Appraiser collections.Map[string, types.Appraiser]

	// Appraisal is the current valuation, keyed by (class id, nft id).
	Appraisal collections.Map[collections.Pair[string, string], types.Appraisal]

	// AppraisalHistory retains superseded valuations, keyed by
	// (class id, nft id, sequence), so the record of what an asset was said to
	// be worth survives being revalued.
	AppraisalHistory collections.Map[collections.Triple[string, string, uint64], types.Appraisal]
	AppraisalSeq     collections.Map[collections.Pair[string, string], uint64]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	stakingKeeper types.StakingKeeper,
	nftKeeper types.NFTKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	pairKey := collections.PairKeyCodec(collections.StringKey, collections.StringKey)

	k := Keeper{
		storeService:  storeService,
		cdc:           cdc,
		addressCodec:  addressCodec,
		authority:     authority,
		stakingKeeper: stakingKeeper,
		nftKeeper:     nftKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		ExchangeRate: collections.NewMap(sb, types.ExchangeRateKey, "exchangeRate",
			collections.StringKey, codec.CollValue[types.ExchangeRate](cdc)),

		Vote: collections.NewMap(sb, types.VoteKey, "vote",
			pairKey, codec.CollValue[types.ExchangeRateVote](cdc)),

		Feeder: collections.NewMap(sb, types.FeederKey, "feeder",
			collections.StringKey, collections.StringValue),

		MissCounter: collections.NewMap(sb, types.MissCounterKey, "missCounter",
			collections.StringKey, codec.CollValue[types.MissCounter](cdc)),

		Appraiser: collections.NewMap(sb, types.AppraiserKey, "appraiser",
			collections.StringKey, codec.CollValue[types.Appraiser](cdc)),

		Appraisal: collections.NewMap(sb, types.AppraisalKey, "appraisal",
			pairKey, codec.CollValue[types.Appraisal](cdc)),

		AppraisalHistory: collections.NewMap(sb, types.AppraisalHistoryKey, "appraisalHistory",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.Uint64Key),
			codec.CollValue[types.Appraisal](cdc)),

		AppraisalSeq: collections.NewMap(sb, types.AppraisalSeqKey, "appraisalSeq",
			pairKey, collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() []byte {
	return k.authority
}

// StakingKeeper exposes the validator set the module votes with.
//
// Needed by the simulation, which has to pick a validator whose feeder key it
// actually holds before it can generate a signable vote.
func (k Keeper) StakingKeeper() types.StakingKeeper {
	return k.stakingKeeper
}
