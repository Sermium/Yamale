package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/selfcustody/types"
)

// Keeper holds the key-disclosure pathway.
//
// Deliberately small. It holds no key material, talks to no custodian service,
// and cannot cause a key to be released — it records a decision the holder made
// and the terms they made it under. The actual reconstruction happens on the
// holder's own device, from their own share plus the one the custodian releases
// against this record, and keeping those two things apart is the point: a
// module that could both authorise and perform a disclosure would be a single
// component whose compromise hands over every account.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	authority []byte

	Schema collections.Schema
	Params collections.Item[types.Params]

	// Permanent, never pruned.
	Disclosure collections.Map[string, types.Disclosure]
	// In flight only; removed on confirm or cancel.
	Request collections.Map[string, types.Request]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
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

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),
		Disclosure: collections.NewMap(sb, types.DisclosureKey, "disclosure",
			collections.StringKey, codec.CollValue[types.Disclosure](cdc)),
		Request: collections.NewMap(sb, types.RequestKey, "request",
			collections.StringKey, codec.CollValue[types.Request](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's governance authority.
func (k Keeper) GetAuthority() []byte { return k.authority }

// IsDisclosed reports whether an account's key has been handed over.
//
// This is the question every other part of the system should ask before
// offering a custody guarantee about an account. It is exported so that the
// custodian service, the wallet and anything else can consult one answer rather
// than each forming its own opinion.
func (k Keeper) IsDisclosed(ctx context.Context, account string) (bool, error) {
	has, err := k.Disclosure.Has(ctx, account)
	if err != nil {
		return false, err
	}
	return has, nil
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, collections.ErrNotFound)
}
