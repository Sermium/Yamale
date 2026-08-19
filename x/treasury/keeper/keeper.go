package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/treasury/types"
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

	TreasurySeq collections.Sequence
	Treasury    collections.Map[uint64, types.Treasury]

	LockSeq collections.Sequence
	Lock    collections.Map[uint64, types.Lock]

	// Balance is the module's ledger: what each treasury holds and how much of
	// it is committed, keyed by (treasury id, denom).
	Balance collections.Map[collections.Pair[uint64, string], types.TreasuryBalance]

	// Role is the access control list, keyed by (treasury id, address).
	Role collections.Map[collections.Pair[uint64, string], types.RoleAssignment]

	// SpendPolicy and SpendWindow are keyed by (treasury id, denom).
	SpendPolicy collections.Map[collections.Pair[uint64, string], types.SpendPolicy]
	SpendWindow collections.Map[collections.Pair[uint64, string], types.SpendWindow]

	// LockByTreasury and LockByBeneficiary are indexes, not storage: they hold
	// no value, only the key pairs needed to list a treasury's or a
	// beneficiary's locks without scanning every lock on the chain.
	LockByTreasury    collections.KeySet[collections.Pair[uint64, uint64]]
	LockByBeneficiary collections.KeySet[collections.Pair[string, uint64]]

	// ActiveLockCount is how many live locks each treasury holds, maintained
	// incrementally so enforcing the cap stays O(1).
	ActiveLockCount collections.Map[uint64, uint64]
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
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,

		bankKeeper: bankKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		TreasurySeq: collections.NewSequence(sb, types.TreasuryCountKey, "treasurySequence"),
		Treasury:    collections.NewMap(sb, types.TreasuryKey, "treasury", collections.Uint64Key, codec.CollValue[types.Treasury](cdc)),

		LockSeq: collections.NewSequence(sb, types.LockCountKey, "lockSequence"),
		Lock:    collections.NewMap(sb, types.LockKey, "lock", collections.Uint64Key, codec.CollValue[types.Lock](cdc)),

		Balance: collections.NewMap(sb, types.BalanceKey, "balance",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.TreasuryBalance](cdc)),

		Role: collections.NewMap(sb, types.RoleKey, "role",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.RoleAssignment](cdc)),

		SpendPolicy: collections.NewMap(sb, types.SpendPolicyKey, "spendPolicy",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.SpendPolicy](cdc)),

		SpendWindow: collections.NewMap(sb, types.SpendWindowKey, "spendWindow",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.SpendWindow](cdc)),

		LockByTreasury: collections.NewKeySet(sb, types.LockByTreasuryKey, "lockByTreasury",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		LockByBeneficiary: collections.NewKeySet(sb, types.LockByBeneficiaryKey, "lockByBeneficiary",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		ActiveLockCount: collections.NewMap(sb, types.ActiveLockCountKey, "activeLockCount",
			collections.Uint64Key, collections.Uint64Value),
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
