package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/amm/types"
)

type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	// Address capable of executing a MsgUpdateParams message.
	// Typically, this should be the x/gov module account.
	authority []byte

	Schema collections.Schema
	Params collections.Item[types.Params]

	bankKeeper types.BankKeeper

	// restrictedDenom is consulted by CreatePool. Behind a pointer and assigned
	// after the dependency graph is built — the same shape x/enforcement uses
	// for its concentration check and for the same reason: an edge from x/amm
	// to x/tokenisation during wiring is one depinject would have to resolve
	// against a module that may not be linked at all, and the AMM must build in
	// a profile that has no tokenisation module. Nil reads as "nothing is
	// restricted", which is correct when there are no fraction denoms.
	restrictedDenom *types.RestrictedDenomKeeper

	PoolSeq collections.Sequence
	Pool    collections.Map[uint64, types.Pool]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	bankKeeper types.BankKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		// Allocated so app.go's assignment after the graph is built reaches the
		// copy the message server was constructed from, exactly as
		// x/enforcement allocates its concentration pointer.
		restrictedDenom: new(types.RestrictedDenomKeeper),

		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,

		bankKeeper: bankKeeper,
		Params:     collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Pool:       collections.NewMap(sb, types.PoolKey, "pool", collections.Uint64Key, codec.CollValue[types.Pool](cdc)),
		PoolSeq:    collections.NewSequence(sb, types.PoolCountKey, "poolSequence"),
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

// SetRestrictedDenomKeeper hands the AMM the live denom classifier, after the
// dependency graph is built.
//
// Forgetting this call does not open a hole in the direction that matters: with
// no classifier, restrictedDenom stays nil and denomIsRestricted reports false,
// so a chain assembled without the tokenisation module treats every denom as
// poolable — which is correct, because that chain has no fraction denoms. What
// it must never do is the reverse, silently permit a fraction denom on a chain
// that HAS them; that is what the app.go wiring is for, and the regression test
// pins it.
func (k Keeper) SetRestrictedDenomKeeper(rk types.RestrictedDenomKeeper) {
	*k.restrictedDenom = rk
}

// denomIsRestricted reports whether a denom must be kept out of a pool.
//
// An error from the classifier is a refusal: a denom the AMM could not vet is
// one it should not pool. Absence of a classifier is not an error — it is a
// chain with nothing to restrict.
func (k Keeper) denomIsRestricted(ctx context.Context, denom string) (bool, error) {
	if k.restrictedDenom == nil || *k.restrictedDenom == nil {
		return false, nil
	}
	return (*k.restrictedDenom).IsRestrictedDenom(ctx, denom)
}
