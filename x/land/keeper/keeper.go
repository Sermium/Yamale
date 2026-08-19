package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/land/types"
)

// Keeper holds the land registry.
//
// Two of these collections carry the guarantees the module exists for, and both
// are indexes rather than scans:
//
//   - ByGeometry makes double-registration impossible. A parcel is refused if
//     its survey hash is already titled, and that refusal is checked in O(1)
//     against an index rather than by walking every parcel — a check that gets
//     slower as the registry grows is a check somebody will eventually turn off.
//   - ByRef is how a citizen finds their land: by the reference printed on the
//     paper in their hand, not by a chain id they have never seen.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	// Address capable of executing governance-gated messages. The x/gov module
	// account: registry offices are admitted by the chain's governance and never
	// by each other, because an office that can admit offices can manufacture
	// the independent attestors a quorum depends on.
	authority []byte

	// Asked once, at admission: is this office a group account? See
	// types.GroupKeeper for why that question is the whole intra-office
	// protection.
	groupKeeper types.GroupKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Parcel   collections.Map[uint64, types.Parcel]
	Transfer collections.Map[uint64, types.Transfer]
	// Registry offices, keyed by address.
	Authority collections.Map[string, types.Authority]

	// Permissions to fractionalise, keyed by parcel id, one per parcel.
	//
	// x/tokenisation refuses to open a vehicle over a parcel that has no live
	// entry here. Withdrawal marks the record rather than deleting it, so the
	// registry cannot quietly erase a permission it once gave.
	FractionalisationAuthority collections.Map[uint64, types.FractionalisationAuthority]

	// geometry hash -> parcel id. The uniqueness constraint.
	ByGeometry collections.Map[string, uint64]
	// cadastral reference -> parcel id. How people actually search.
	ByRef collections.Map[string, uint64]

	// Counters are stored, not derived from the highest id present. A derived
	// counter changes behaviour after an export whenever the highest-numbered
	// record has been removed, and an imported registry that renumbers parcels
	// is a different registry.
	NextParcelID   collections.Sequence
	NextTransferID collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	groupKeeper types.GroupKeeper,
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
		groupKeeper:  groupKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),
		Parcel: collections.NewMap(sb, types.ParcelKey, "parcel",
			collections.Uint64Key, codec.CollValue[types.Parcel](cdc)),
		Transfer: collections.NewMap(sb, types.TransferKey, "transfer",
			collections.Uint64Key, codec.CollValue[types.Transfer](cdc)),
		Authority: collections.NewMap(sb, types.AuthorityKey, "authority",
			collections.StringKey, codec.CollValue[types.Authority](cdc)),
		FractionalisationAuthority: collections.NewMap(sb,
			types.FractionalisationAuthorityKey, "fractionalisation_authority",
			collections.Uint64Key,
			codec.CollValue[types.FractionalisationAuthority](cdc)),
		ByGeometry: collections.NewMap(sb, types.ByGeometryKey, "by_geometry",
			collections.StringKey, collections.Uint64Value),
		ByRef: collections.NewMap(sb, types.ByRefKey, "by_ref",
			collections.StringKey, collections.Uint64Value),
		NextParcelID: collections.NewSequence(sb, types.NextParcelIDKey, "next_parcel_id"),
		NextTransferID: collections.NewSequence(sb, types.NextTransferIDKey,
			"next_transfer_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's governance authority.
func (k Keeper) GetAuthority() []byte {
	return k.authority
}
