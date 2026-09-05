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

	"yamale/blockchain/x/netting/types"
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

	authKeeper   types.AuthKeeper
	bankKeeper   types.BankKeeper
	participants types.ParticipantKeeper

	// CurrentCycle names the open window, and CycleSeq numbers the next one.
	CurrentCycle collections.Item[uint64]
	CycleSeq     collections.Sequence
	Cycle        collections.Map[uint64, types.Cycle]

	// Obligation is keyed by (cycle id, obligation id), and
	// ObligationByParticipant indexes both sides of each one.
	ObligationSeq           collections.Sequence
	Obligation              collections.Map[collections.Pair[uint64, uint64], types.Obligation]
	ObligationByParticipant collections.KeySet[collections.Triple[string, uint64, uint64]]

	// Position is keyed by (cycle id, denom, participant), and that order is
	// load-bearing. The end blocker walks it in store order, so currencies
	// arrive grouped and participants inside a currency arrive in the same
	// sequence on every validator. Accumulating into a Go map instead would
	// settle in a different order per node and produce a different app hash.
	Position collections.Map[collections.Triple[uint64, string, string], math.Int]

	// Reserve is what a participant has prefunded, keyed by (participant,
	// denom); Locked is the part of it already committed to positions in
	// windows that have not settled.
	//
	// The pair between them is the whole safety argument: an obligation is
	// refused unless Locked would stay within Reserve, so by the time a window
	// closes every debit in it is already covered by money this module holds.
	Reserve collections.Map[collections.Pair[string, string], math.Int]
	Locked  collections.Map[collections.Pair[string, string], math.Int]

	// HeldSlice is the retry queue: the (cycle, denom) slices that refused to
	// settle. Empty on a healthy chain, and walked every cycle boundary.
	HeldSlice collections.KeySet[collections.Pair[uint64, string]]

	// HeldSince is the cycle at which each held slice was first held, so a hold
	// has an age. A retry queue with no clock cannot tell anybody that
	// somebody's collateral has been locked since April.
	HeldSince collections.Map[collections.Pair[uint64, string], uint64]

	// RetryCursor and EscalateCursor are where the bounded sweeps below pick
	// up. Both walks used to run the whole collection every cycle boundary with
	// nothing capping the work, on a chain whose consensus max_gas is -1 — and
	// an error returned from an end blocker halts the chain rather than failing
	// a message. Bounding them without a cursor would have been worse than not
	// bounding them: the same first slices retried forever, everything behind
	// them never.
	RetryCursor    collections.Item[string]
	EscalateCursor collections.Item[string]

	// NettedTotal is what each participant has put into the window as a debtor,
	// per (cycle, denom). Read by the aggregate gross threshold and by nothing
	// else; settlement does not consult it, because the money that settles is
	// still decided by Position.
	NettedTotal collections.Map[collections.Triple[uint64, string, string], math.Int]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,

	authKeeper types.AuthKeeper,
	bankKeeper types.BankKeeper,
	participants types.ParticipantKeeper,
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

		authKeeper:   authKeeper,
		bankKeeper:   bankKeeper,
		participants: participants,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		CurrentCycle: collections.NewItem(sb, types.CurrentCycleKey, "currentCycle", collections.Uint64Value),
		CycleSeq:     collections.NewSequence(sb, types.CycleSeqKey, "cycleSequence"),
		Cycle:        collections.NewMap(sb, types.CycleKey, "cycle", collections.Uint64Key, codec.CollValue[types.Cycle](cdc)),

		ObligationSeq: collections.NewSequence(sb, types.ObligationSeqKey, "obligationSequence"),
		Obligation: collections.NewMap(sb, types.ObligationKey, "obligation",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
			codec.CollValue[types.Obligation](cdc)),
		ObligationByParticipant: collections.NewKeySet(sb, types.ObligationByParticipantKey, "obligationByParticipant",
			collections.TripleKeyCodec(collections.StringKey, collections.Uint64Key, collections.Uint64Key)),

		Position: collections.NewMap(sb, types.PositionKey, "position",
			collections.TripleKeyCodec(collections.Uint64Key, collections.StringKey, collections.StringKey),
			sdk.IntValue),

		Reserve: collections.NewMap(sb, types.ReserveKey, "reserve",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey), sdk.IntValue),
		Locked: collections.NewMap(sb, types.LockedKey, "locked",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey), sdk.IntValue),

		HeldSince: collections.NewMap(sb, types.HeldSinceKey, "heldSince",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),

		RetryCursor: collections.NewItem(sb, types.RetryCursorKey, "retryCursor",
			collections.StringValue),
		EscalateCursor: collections.NewItem(sb, types.EscalateCursorKey, "escalateCursor",
			collections.StringValue),

		NettedTotal: collections.NewMap(sb, types.NettedTotalKey, "nettedTotal",
			collections.TripleKeyCodec(collections.Uint64Key, collections.StringKey, collections.StringKey),
			sdk.IntValue),

		HeldSlice: collections.NewKeySet(sb, types.HeldSliceKey, "heldSlice",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey)),
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

// ModuleAddress is where every participant's reserve is custodied.
//
// The reserve lives in a module account rather than staying in the
// participant's own account with a lien recorded against it, for the reason
// x/treasury holds deposits the same way: a balance that is only notionally
// committed can be spent by an ordinary bank send, and the module would find
// out at settlement. Once the coins are here, the commitment is a fact about
// where the money is rather than a promise about what someone will not do.
func (k Keeper) ModuleAddress() sdk.AccAddress {
	return k.authKeeper.GetModuleAddress(types.ModuleName)
}

// GetReserve returns what a participant has prefunded in one currency.
func (k Keeper) GetReserve(ctx context.Context, participant, denom string) (math.Int, error) {
	return k.readInt(ctx, k.Reserve, collections.Join(participant, denom))
}

// GetLocked returns the part of that reserve already committed to positions in
// windows that have not settled.
func (k Keeper) GetLocked(ctx context.Context, participant, denom string) (math.Int, error) {
	return k.readInt(ctx, k.Locked, collections.Join(participant, denom))
}

// Available is what a participant may withdraw right now.
//
// Clamped at zero rather than allowed to go negative. A negative available
// figure would be a bug elsewhere — locked can never legitimately exceed the
// reserve, because that is what every submission checks — and reporting it as a
// negative withdrawable amount would let a subtraction somewhere downstream
// turn it back into a positive one.
func (k Keeper) Available(ctx context.Context, participant, denom string) (math.Int, error) {
	reserve, err := k.GetReserve(ctx, participant, denom)
	if err != nil {
		return math.Int{}, err
	}
	locked, err := k.GetLocked(ctx, participant, denom)
	if err != nil {
		return math.Int{}, err
	}
	if locked.GTE(reserve) {
		return math.ZeroInt(), nil
	}
	return reserve.Sub(locked), nil
}

// GetPosition returns a participant's signed net position in one cycle and
// currency. An absent position is zero, which is what "this participant has
// neither owed nor been owed here" means.
func (k Keeper) GetPosition(ctx context.Context, cycleID uint64, denom, participant string) (math.Int, error) {
	amount, err := k.Position.Get(ctx, collections.Join3(cycleID, denom, participant))
	switch {
	case err == nil:
		return amount, nil
	case errors.Is(err, collections.ErrNotFound):
		return math.ZeroInt(), nil
	default:
		return math.Int{}, err
	}
}

// setPosition writes a position, removing it when it reaches zero.
//
// Removing rather than storing a zero is not tidiness. State rebuilt by
// replaying obligations and state restored from a genesis file have to be the
// same bytes, and an export that omits zeros while the running chain stores
// them makes the two disagree the moment a position nets out exactly — which,
// in a netting module, is the single most likely thing to happen.
func (k Keeper) setPosition(ctx context.Context, cycleID uint64, denom, participant string, amount math.Int) error {
	key := collections.Join3(cycleID, denom, participant)
	if amount.IsZero() {
		if err := k.Position.Remove(ctx, key); err != nil {
			return err
		}
		return nil
	}
	return k.Position.Set(ctx, key, amount)
}

// setReserve writes a reserve balance, removing it when it reaches zero, for
// the same import/export reason as setPosition.
func (k Keeper) setReserve(ctx context.Context, participant, denom string, amount math.Int) error {
	key := collections.Join(participant, denom)
	if amount.IsZero() {
		return k.Reserve.Remove(ctx, key)
	}
	return k.Reserve.Set(ctx, key, amount)
}

// setLocked writes a locked balance, removing it when it reaches zero.
func (k Keeper) setLocked(ctx context.Context, participant, denom string, amount math.Int) error {
	key := collections.Join(participant, denom)
	if amount.IsZero() {
		return k.Locked.Remove(ctx, key)
	}
	return k.Locked.Set(ctx, key, amount)
}

// adjustLocked moves the committed figure by the change in a participant's net
// debit, given its position before and after an obligation.
//
// Only the negative part of a position is collateral. A participant that is
// net owed has committed nothing, and a participant whose position improves
// from -100 to -30 gets 70 back immediately — which is exactly the liquidity
// saving that makes netting worth doing, and it has to be reflected the moment
// the offsetting obligation arrives rather than at close.
func (k Keeper) adjustLocked(ctx context.Context, participant, denom string, before, after math.Int) error {
	delta := debitOf(after).Sub(debitOf(before))
	if delta.IsZero() {
		return nil
	}
	locked, err := k.GetLocked(ctx, participant, denom)
	if err != nil {
		return err
	}
	updated := locked.Add(delta)
	if updated.IsNegative() {
		// Unreachable through the message handlers, which is why it is checked
		// rather than assumed: a locked figure that has gone negative means
		// this module has released collateral it never held, and continuing
		// from there is how one participant's mistake becomes another's loss.
		return fmt.Errorf("locked for %s in %s would go negative (%s)", participant, denom, updated)
	}
	return k.setLocked(ctx, participant, denom, updated)
}

// debitOf is the collateralised part of a position: what it owes, or zero.
func debitOf(position math.Int) math.Int {
	if position.IsNegative() {
		return position.Neg()
	}
	return math.ZeroInt()
}

// readInt returns a stored amount, treating absence as zero.
func (k Keeper) readInt(ctx context.Context, m collections.Map[collections.Pair[string, string], math.Int], key collections.Pair[string, string]) (math.Int, error) {
	amount, err := m.Get(ctx, key)
	switch {
	case err == nil:
		return amount, nil
	case errors.Is(err, collections.ErrNotFound):
		return math.ZeroInt(), nil
	default:
		return math.Int{}, err
	}
}
