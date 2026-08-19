package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/constitution/types"
)

// Keeper holds the values this chain fixed at genesis, and the only path by
// which they may change.
//
// It depends on nothing except x/staking. That direction is the whole reason
// the invariants live in a module of their own rather than in a shared store or
// in whichever module happens to read a value first: x/validatorgov and
// x/enforcement both consult it, so anything it consulted back would be a cycle
// depinject cannot wire and a review cannot follow.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	// Address capable of proposing an amendment. Typically the x/gov module
	// account. There is no MsgUpdateParams here for it to hold, by design.
	authority []byte

	Schema collections.Schema

	stakingKeeper types.StakingKeeper

	// Invariants is the settlement in force. An Item rather than a Map: there
	// is one constitution, and a module able to hold two at once would need a
	// rule for which one x/enforcement is checked against.
	Invariants collections.Item[types.Invariants]

	AmendmentSeq collections.Sequence
	Amendment    collections.Map[uint64, types.Amendment]

	// Ratification is keyed by (amendment id, validator operator address). The
	// tally is kept as a running total, so a second ratification from the same
	// validator would not be caught by recomputing it — this index is what
	// catches it.
	Ratification collections.Map[collections.Pair[uint64, string], types.Ratification]

	// AmendmentQueue indexes pending amendments by (effective height, id), so
	// the end blocker pays for what falls due now rather than for every
	// amendment there has ever been.
	AmendmentQueue collections.KeySet[collections.Pair[int64, uint64]]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	stakingKeeper types.StakingKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,

		stakingKeeper: stakingKeeper,

		Invariants: collections.NewItem(sb, types.InvariantsKey, "invariants",
			codec.CollValue[types.Invariants](cdc)),
		AmendmentSeq: collections.NewSequence(sb, types.AmendmentSeqKey, "amendmentSequence"),
		Amendment: collections.NewMap(sb, types.AmendmentKey, "amendment",
			collections.Uint64Key, codec.CollValue[types.Amendment](cdc)),
		Ratification: collections.NewMap(sb, types.RatificationKey, "ratification",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Ratification](cdc)),
		AmendmentQueue: collections.NewKeySet(sb, types.AmendmentQueueKey, "amendmentQueue",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
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

// GetInvariants returns the settlement in force.
//
// This is the whole interface other modules use. It returns an error rather
// than a zero value when nothing has been written, because a chain with no
// constitution must not be one where every ceiling reads as zero and every
// comparison against it silently passes.
func (k Keeper) GetInvariants(ctx context.Context) (types.Invariants, error) {
	inv, err := k.Invariants.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Invariants{}, types.ErrNoInvariants
		}
		return types.Invariants{}, err
	}
	return inv, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, collections.ErrNotFound)
}
