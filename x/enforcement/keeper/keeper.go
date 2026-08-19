package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
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

	authKeeper    types.AuthKeeper
	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper

	// constitutionKeeper holds the four parameters governance is not allowed to
	// move. Read-only, and read on both write paths into Params.
	constitutionKeeper types.ConstitutionKeeper

	CaseSeq collections.Sequence
	Case    collections.Map[uint64, types.Case]

	// Vote is keyed by (case id, validator operator address).
	Vote collections.Map[collections.Pair[uint64, string], types.Vote]

	// Freeze is keyed by address, and is read on every transfer the chain
	// processes. Everything else here can afford to be a scan; this cannot.
	Freeze collections.Map[string, types.Freeze]

	// VotingQueue indexes open cases by (voting end height, case id), and
	// FreezeExpiryQueue provisional freezes by (expiry height, address). Both
	// exist so the end blocker's cost depends on what is happening now rather
	// than on how much has ever happened.
	VotingQueue       collections.KeySet[collections.Pair[int64, uint64]]
	FreezeExpiryQueue collections.KeySet[collections.Pair[int64, string]]

	// Recovered is the running total of everything seized, keyed by denom, and
	// CasesPassed the count of cases that got there. Kept as state rather than
	// computed on demand because the honest answer to "how often has this been
	// used" should not require replaying the chain.
	Recovered   collections.Map[string, math.Int]
	CasesPassed collections.Item[uint64]

	// ExecutionQueue indexes the seizures the set has agreed to and that are
	// waiting out the delay their size earned, keyed by (execute height, case
	// id). Walked by the end blocker for the same reason the other queues are.
	ExecutionQueue collections.KeySet[collections.Pair[int64, uint64]]

	// SeizureLedger is the rolling window, and all of it. One record per
	// executed seizure, keyed by (height, case id), summed by range scan from
	// the window's start height.
	//
	// There is no running total beside it on purpose. A total kept next to the
	// records it is derived from is a second copy of the same fact, and the day
	// they disagree the module will believe the cheap one. The number of
	// records inside a window is bounded by the cap itself, so summing them is
	// bounded too — the cap pays for its own enforcement.
	SeizureLedger collections.Map[collections.Pair[int64, uint64], types.SeizureRecord]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	authKeeper types.AuthKeeper,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
	constitutionKeeper types.ConstitutionKeeper,
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

		constitutionKeeper: constitutionKeeper,

		authKeeper:    authKeeper,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		CaseSeq: collections.NewSequence(sb, types.CaseSeqKey, "caseSequence"),
		Case:    collections.NewMap(sb, types.CaseKey, "case", collections.Uint64Key, codec.CollValue[types.Case](cdc)),

		Vote: collections.NewMap(sb, types.VoteKey, "vote",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Vote](cdc)),

		Freeze: collections.NewMap(sb, types.FreezeKey, "freeze",
			collections.StringKey, codec.CollValue[types.Freeze](cdc)),

		VotingQueue: collections.NewKeySet(sb, types.VotingQueueKey, "votingQueue",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),

		FreezeExpiryQueue: collections.NewKeySet(sb, types.FreezeExpiryQueueKey, "freezeExpiryQueue",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey)),

		Recovered:   collections.NewMap(sb, types.RecoveredKey, "recovered", collections.StringKey, sdk.IntValue),
		CasesPassed: collections.NewItem(sb, types.CasesPassedKey, "casesPassed", collections.Uint64Value),

		ExecutionQueue: collections.NewKeySet(sb, types.ExecutionQueueKey, "executionQueue",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),

		SeizureLedger: collections.NewMap(sb, types.SeizureLedgerKey, "seizureLedger",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key),
			codec.CollValue[types.SeizureRecord](cdc)),
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

// IsFrozen reports whether an address may not send.
//
// This is the question the bank asks on every transfer, so it answers in one
// store read and never returns an error: a store failure here would have to
// either block every transfer on the chain or let a frozen account through, and
// the second is the one that loses somebody's money. It fails closed.
func (k Keeper) IsFrozen(ctx context.Context, addr string) bool {
	has, err := k.Freeze.Has(ctx, addr)
	if err != nil {
		return true
	}
	return has
}

// FreezeOf returns the freeze on an address, if any.
func (k Keeper) FreezeOf(ctx context.Context, addr string) (types.Freeze, bool, error) {
	freeze, err := k.Freeze.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Freeze{}, false, nil
		}
		return types.Freeze{}, false, err
	}
	return freeze, true, nil
}

// freeze records a freeze and, when it is provisional, queues its expiry.
func (k Keeper) freeze(ctx context.Context, addr string, caseID uint64, expiresAtHeight int64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if err := k.Freeze.Set(ctx, addr, types.Freeze{
		Address:         addr,
		CaseId:          caseID,
		ExpiresAtHeight: expiresAtHeight,
		FrozenAtHeight:  sdkCtx.BlockHeight(),
	}); err != nil {
		return err
	}
	if expiresAtHeight > 0 {
		return k.FreezeExpiryQueue.Set(ctx, collections.Join(expiresAtHeight, addr))
	}
	return nil
}

// unfreeze lifts a freeze and removes it from the expiry queue.
func (k Keeper) unfreeze(ctx context.Context, addr string) error {
	freeze, found, err := k.FreezeOf(ctx, addr)
	if err != nil || !found {
		return err
	}
	if freeze.ExpiresAtHeight > 0 {
		if err := k.FreezeExpiryQueue.Remove(ctx, collections.Join(freeze.ExpiresAtHeight, addr)); err != nil {
			return err
		}
	}
	return k.Freeze.Remove(ctx, addr)
}

// makePermanent converts the provisional freeze that came with opening a case
// into one that does not lapse. Called when a case passes: at that point the
// freeze is no longer one validator's suspicion, it is the set's decision.
func (k Keeper) makePermanent(ctx context.Context, addr string) error {
	freeze, found, err := k.FreezeOf(ctx, addr)
	if err != nil || !found {
		return err
	}
	if freeze.ExpiresAtHeight > 0 {
		if err := k.FreezeExpiryQueue.Remove(ctx, collections.Join(freeze.ExpiresAtHeight, addr)); err != nil {
			return err
		}
	}
	freeze.ExpiresAtHeight = 0
	return k.Freeze.Set(ctx, addr, freeze)
}
