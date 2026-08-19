package keeper

import (
	"context"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/treasury/types"
)

// Conditional locks — escrow.
//
// A buyer commits money, the seller ships knowing it exists, and the funds move
// when the buyer confirms. What makes this worth building on a chain rather
// than in an application is where the money sits in between: the module
// account. Not the platform's account, not the seller's, not an administrator's
// — an account nobody can spend from except through the four handlers below.
//
// That is the whole guarantee, and it is the one an application cannot give.

// OpenEscrow funds a conditional lock from the depositor's own balance.
//
// Note this does *not* draw on a treasury. Every other lock moves a treasury's
// available balance into its locked balance; escrow money belongs to a buyer
// who administers nothing. Mixing the two would mean a treasury admin could
// eventually be argued into reaching an escrow, and the point of the design is
// that nobody can.
func (m msgServer) OpenEscrow(ctx context.Context, msg *types.MsgOpenEscrow) (*types.MsgOpenEscrowResponse, error) {
	depositor, err := m.addressCodec.StringToBytes(msg.Depositor)
	if err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Beneficiary); err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Moderator); err != nil {
		return nil, err
	}
	if !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	// Paying yourself through an escrow is not a deal, and a moderator who is
	// one of the parties is not a moderator.
	if msg.Beneficiary == msg.Depositor {
		return nil, types.ErrSelfEscrow
	}
	if msg.Moderator == msg.Depositor || msg.Moderator == msg.Beneficiary {
		return nil, types.ErrModeratorIsParty
	}

	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, depositor, types.ModuleName, sdk.NewCoins(msg.Amount)); err != nil {
		return nil, err
	}

	id, err := m.LockSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	id++ // ids start at 1; zero is indistinguishable from an unset proto field

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	lock := types.Lock{
		Id:              id,
		Beneficiary:     msg.Beneficiary,
		Denom:           msg.Amount.Denom,
		TotalAmount:     msg.Amount.Amount.String(),
		ReleasedAmount:  "0",
		LockType:        types.LockType_LOCK_TYPE_CONDITIONAL,
		Active:          true,
		CreatedAtHeight: uint64(sdkCtx.BlockHeight()),
		Depositor:       msg.Depositor,
		Moderator:       msg.Moderator,
		Dispute:         types.DisputeState_DISPUTE_STATE_NONE,
	}
	if err := m.Lock.Set(ctx, id, lock); err != nil {
		return nil, err
	}
	return &types.MsgOpenEscrowResponse{LockId: id}, nil
}

// ReleaseEscrow pays the beneficiary. Only the depositor may.
//
// The buyer confirming *is* the condition. Nobody confirms on their behalf —
// not the seller, not the moderator, not a treasury admin — because a release
// anyone else can trigger is not a condition, it is a formality.
func (m msgServer) ReleaseEscrow(ctx context.Context, msg *types.MsgReleaseEscrow) (*types.MsgReleaseEscrowResponse, error) {
	lock, err := m.conditional(ctx, msg.LockId)
	if err != nil {
		return nil, err
	}
	if lock.Depositor != msg.Depositor {
		return nil, types.ErrNotDepositor
	}
	// A frozen lock is the moderator's to decide. Letting the depositor release
	// during a case would let a buyer defuse the seller's complaint by paying,
	// which sounds harmless until the case was about a partial delivery.
	if lock.Dispute == types.DisputeState_DISPUTE_STATE_OPEN {
		return nil, types.ErrEscrowDisputed
	}
	return &types.MsgReleaseEscrowResponse{}, m.settle(ctx, lock, true)
}

// DisputeEscrow freezes a lock and refers it to its named moderator.
//
// Either party may. That symmetry is what removes the need for a deadline: a
// seller facing a buyer who has gone quiet escalates rather than waiting
// forever, and an automatic release would instead have rewarded whichever
// seller shipped nothing and waited.
func (m msgServer) DisputeEscrow(ctx context.Context, msg *types.MsgDisputeEscrow) (*types.MsgDisputeEscrowResponse, error) {
	lock, err := m.conditional(ctx, msg.LockId)
	if err != nil {
		return nil, err
	}
	if msg.Party != lock.Depositor && msg.Party != lock.Beneficiary {
		return nil, types.ErrNotParty
	}
	if lock.Dispute == types.DisputeState_DISPUTE_STATE_OPEN {
		return nil, types.ErrAlreadyDisputed
	}
	if msg.Reason == "" {
		// A moderator judging between two strangers has nothing else to go on,
		// and a case with no statement gets decided on whoever complains
		// loudest afterwards.
		return nil, types.ErrNoReason
	}

	lock.Dispute = types.DisputeState_DISPUTE_STATE_OPEN
	lock.DisputeReason = msg.Reason
	lock.DisputeOpenedBy = msg.Party
	return &types.MsgDisputeEscrowResponse{}, m.Lock.Set(ctx, lock.Id, lock)
}

// ResolveEscrow is the moderator deciding an open case.
//
// Their only power, and only on a lock somebody actually disputed. A moderator
// who could act on a quiet lock would be a custodian under another name, which
// is exactly what this design removes.
func (m msgServer) ResolveEscrow(ctx context.Context, msg *types.MsgResolveEscrow) (*types.MsgResolveEscrowResponse, error) {
	lock, err := m.conditional(ctx, msg.LockId)
	if err != nil {
		return nil, err
	}
	if lock.Moderator != msg.Moderator {
		return nil, types.ErrNotModerator
	}
	if lock.Dispute != types.DisputeState_DISPUTE_STATE_OPEN {
		return nil, types.ErrNoOpenCase
	}

	lock.Dispute = types.DisputeState_DISPUTE_STATE_RESOLVED
	return &types.MsgResolveEscrowResponse{}, m.settle(ctx, lock, msg.PayBeneficiary)
}

// settle pays out and closes the lock, in that order.
//
// The transfer happens before the record is written, so a failed send takes the
// whole message down with it rather than leaving a closed lock with the money
// still in the module account.
func (m msgServer) settle(ctx context.Context, lock types.Lock, toBeneficiary bool) error {
	recipient := lock.Depositor
	if toBeneficiary {
		recipient = lock.Beneficiary
	}
	to, err := m.addressCodec.StringToBytes(recipient)
	if err != nil {
		return err
	}

	amount, ok := math.NewIntFromString(lock.TotalAmount)
	if !ok {
		return types.ErrInvalidAmount
	}
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, to, sdk.NewCoins(sdk.NewCoin(lock.Denom, amount))); err != nil {
		return err
	}

	lock.ReleasedAmount = lock.TotalAmount
	lock.Active = false
	return m.Lock.Set(ctx, lock.Id, lock)
}

// conditional fetches a live conditional lock, refusing anything else.
//
// Reusing the lock store for two very different instruments means every handler
// has to prove it is looking at the right one. A time lock reaching an escrow
// handler would release on rules nobody agreed to.
func (m msgServer) conditional(ctx context.Context, id uint64) (types.Lock, error) {
	lock, err := m.Lock.Get(ctx, id)
	if err != nil {
		return types.Lock{}, types.ErrLockNotFound
	}
	if lock.LockType != types.LockType_LOCK_TYPE_CONDITIONAL {
		return types.Lock{}, types.ErrNotEscrow
	}
	if !lock.Active {
		return types.Lock{}, types.ErrLockClosed
	}
	return lock, nil
}
