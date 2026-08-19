package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/validatorgov/types"
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

	stakingKeeper        types.StakingKeeper
	authzKeeper          types.AuthzKeeper
	ValidatorApplication collections.Map[string, types.ValidatorApplication]
	ApprovedValidator    collections.Map[string, types.ApprovedValidator]

	RotationSeq collections.Sequence
	Rotation    collections.Map[uint64, types.OperatorRotation]

	// PendingRotation maps an operator address to the one rotation open against
	// it. Read for every signer of every transaction the chain processes, which
	// is why it is an index and not a scan.
	PendingRotation collections.Map[string, uint64]

	// RotationQueue indexes the rotations that are counting down by (completion
	// height, id), so the end blocker pays for what falls due now rather than
	// for every rotation there has ever been.
	RotationQueue collections.KeySet[collections.Pair[int64, uint64]]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	stakingKeeper types.StakingKeeper,
	authzKeeper types.AuthzKeeper,
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

		stakingKeeper:        stakingKeeper,
		authzKeeper:          authzKeeper,
		Params:               collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		ValidatorApplication: collections.NewMap(sb, types.ValidatorApplicationKey, "validatorApplication", collections.StringKey, codec.CollValue[types.ValidatorApplication](cdc)), ApprovedValidator: collections.NewMap(sb, types.ApprovedValidatorKey, "approvedValidator", collections.StringKey, codec.CollValue[types.ApprovedValidator](cdc)),

		RotationSeq: collections.NewSequence(sb, types.RotationSeqKey, "rotationSequence"),
		Rotation: collections.NewMap(sb, types.RotationKey, "rotation",
			collections.Uint64Key, codec.CollValue[types.OperatorRotation](cdc)),
		PendingRotation: collections.NewMap(sb, types.PendingRotationKey, "pendingRotation",
			collections.StringKey, collections.Uint64Value),
		RotationQueue: collections.NewKeySet(sb, types.RotationQueueKey, "rotationQueue",
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

// AddressCodec returns the codec the module encodes account addresses with.
// The ante decorators need it to render the raw signer bytes a transaction
// carries into the bech32 form this module's state is keyed by.
func (k Keeper) AddressCodec() address.Codec {
	return k.addressCodec
}
