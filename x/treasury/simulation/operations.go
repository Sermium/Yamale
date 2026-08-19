// Package simulation drives the treasury module's randomized operations.
//
// The point of simulating a treasury is not that any one message succeeds — it
// is that deposits, spends, locks, claims and revocations interleave in orders
// nobody thought to write a test for, while the ledger has to stay consistent
// throughout. The keeper's property test covers that for a single treasury in
// isolation; these operations put it under the whole application, alongside
// every other module competing for the same accounts.
package simulation

import (
	"fmt"
	"math/rand"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/treasury/keeper"
	"yamale/blockchain/x/treasury/types"
)

// SimulateMsgCreateTreasury opens a treasury owned by a random account.
func SimulateMsgCreateTreasury(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)

		msg := &types.MsgCreateTreasury{
			Creator: simAccount.Address.String(),
			Name:    simtypes.RandStringOfLength(r, 10),
		}

		return deliver(r, app, ctx, txGen, ak, bk, simAccount, msg, nil)
	}
}

// SimulateMsgDeposit funds a random treasury from a random account.
func SimulateMsgDeposit(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgDeposit{Depositor: simAccount.Address.String()}

		treasury, found, err := randomTreasury(ctx, k, r)
		if err != nil {
			return noOp(msg, "unable to read treasuries"), nil, err
		}
		if !found {
			return noOp(msg, "no treasuries exist yet"), nil, nil
		}

		spendable := bk.SpendableCoins(ctx, simAccount.Address).AmountOf(sdk.DefaultBondDenom)
		if !spendable.IsPositive() {
			return noOp(msg, "depositor holds nothing to deposit"), nil, nil
		}
		amount, err := simtypes.RandPositiveInt(r, spendable.Quo(math.NewInt(4)))
		if err != nil {
			return noOp(msg, "unable to generate a deposit amount"), nil, nil
		}

		deposit := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, amount))
		msg.TreasuryId = treasury.Id
		msg.Amount = deposit

		return deliver(r, app, ctx, txGen, ak, bk, simAccount, msg, deposit)
	}
}

// SimulateMsgSpend pays out of a treasury the signer actually controls.
func SimulateMsgSpend(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		recipient, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgSpend{}

		treasury, admin, found, err := randomAdministeredTreasury(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read treasuries"), nil, err
		}
		if !found {
			return noOp(msg, "no treasury is administered by a simulation account"), nil, nil
		}
		if treasury.Paused {
			return noOp(msg, "treasury is paused"), nil, nil
		}

		// Bound the spend by everything that could reject it — the available
		// balance, the per-transaction cap and the period allowance — because
		// the simulator treats an undeliverable transaction as a fatal error
		// rather than a no-op.
		capacity, err := k.SpendCapacityAt(ctx, treasury.Id, sdk.DefaultBondDenom, ctx.BlockTime().Unix())
		if err != nil {
			return noOp(msg, "unable to read the treasury spend capacity"), nil, err
		}
		if !capacity.MaxSingleSpend.IsPositive() {
			return noOp(msg, "treasury has no spending capacity right now"), nil, nil
		}

		amount, err := simtypes.RandPositiveInt(r, capacity.MaxSingleSpend)
		if err != nil {
			return noOp(msg, "unable to generate a spend amount"), nil, nil
		}

		msg.Spender = admin.Address.String()
		msg.TreasuryId = treasury.Id
		msg.Recipient = recipient.Address.String()
		msg.Amount = sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, amount))
		msg.Memo = simtypes.RandStringOfLength(r, 16)

		// The spend leaves the treasury, not the signer's own wallet, so no
		// coins need reserving against the fee.
		return deliver(r, app, ctx, txGen, ak, bk, admin, msg, nil)
	}
}

// SimulateMsgCreateLock commits part of a treasury to a beneficiary.
func SimulateMsgCreateLock(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		beneficiary, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgCreateLock{}

		treasury, admin, found, err := randomAdministeredTreasury(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read treasuries"), nil, err
		}
		if !found {
			return noOp(msg, "no treasury is administered by a simulation account"), nil, nil
		}
		if treasury.Paused {
			return noOp(msg, "treasury is paused"), nil, nil
		}

		available, err := k.AvailableBalance(ctx, treasury.Id, sdk.DefaultBondDenom)
		if err != nil {
			return noOp(msg, "unable to read the treasury balance"), nil, err
		}
		if !available.IsPositive() {
			return noOp(msg, "treasury has nothing available to commit"), nil, nil
		}

		amount, err := simtypes.RandPositiveInt(r, available)
		if err != nil {
			return noOp(msg, "unable to generate a lock amount"), nil, nil
		}

		params, err := k.Params.Get(ctx)
		if err != nil {
			return noOp(msg, "unable to read params"), nil, err
		}

		// Anchor the schedule to the chain's clock, and make it long enough to
		// clear the minimum, so most attempts produce a usable lock.
		now := ctx.BlockTime().Unix()
		duration := int64(params.MinLockSeconds) + int64(r.Intn(100_000)+1)
		start := now
		end := start + duration
		cliff := start + int64(r.Intn(int(duration)))

		msg.Admin = admin.Address.String()
		msg.TreasuryId = treasury.Id
		msg.Beneficiary = beneficiary.Address.String()
		msg.Denom = sdk.DefaultBondDenom
		msg.Amount = amount.String()
		msg.LockType = randomLockType(r)
		msg.StartTime = start
		msg.CliffTime = cliff
		msg.EndTime = end
		msg.ReleaseIntervals = uint64(r.Intn(5))
		msg.Revocable = r.Intn(2) == 0

		return deliver(r, app, ctx, txGen, ak, bk, admin, msg, nil)
	}
}

// SimulateMsgClaimLock withdraws whatever has vested to a beneficiary that is
// also a simulation account, so the message can be signed.
func SimulateMsgClaimLock(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgClaimLock{}

		lock, beneficiary, found, err := randomClaimableLock(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read locks"), nil, err
		}
		if !found {
			return noOp(msg, "no lock has anything claimable by a simulation account"), nil, nil
		}

		msg.Beneficiary = beneficiary.Address.String()
		msg.LockId = lock.Id

		return deliver(r, app, ctx, txGen, ak, bk, beneficiary, msg, nil)
	}
}

// SimulateMsgRevokeLock cancels a revocable lock.
func SimulateMsgRevokeLock(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgRevokeLock{}

		lock, found, err := randomActiveRevocableLock(ctx, k, r)
		if err != nil {
			return noOp(msg, "unable to read locks"), nil, err
		}
		if !found {
			return noOp(msg, "no revocable lock is active"), nil, nil
		}

		treasury, err := k.Treasury.Get(ctx, lock.TreasuryId)
		if err != nil {
			return noOp(msg, "unable to read the treasury"), nil, nil
		}
		if treasury.Paused {
			return noOp(msg, "treasury is paused"), nil, nil
		}

		adminAddr, err := sdk.AccAddressFromBech32(treasury.Admin)
		if err != nil {
			return noOp(msg, "treasury admin is not an account address"), nil, nil
		}
		admin, ok := simtypes.FindAccount(accs, adminAddr)
		if !ok {
			return noOp(msg, "treasury admin is not a simulation account"), nil, nil
		}

		msg.Admin = admin.Address.String()
		msg.LockId = lock.Id

		return deliver(r, app, ctx, txGen, ak, bk, admin, msg, nil)
	}
}

// SimulateMsgAssignRole hands a random account a spender or pauser role.
func SimulateMsgAssignRole(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		grantee, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgAssignRole{}

		treasury, admin, found, err := randomAdministeredTreasury(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read treasuries"), nil, err
		}
		if !found {
			return noOp(msg, "no treasury is administered by a simulation account"), nil, nil
		}

		role := types.Role_ROLE_SPENDER
		if r.Intn(3) == 0 {
			role = types.Role_ROLE_PAUSER
		}

		msg.Admin = admin.Address.String()
		msg.TreasuryId = treasury.Id
		msg.Address = grantee.Address.String()
		msg.Role = role

		return deliver(r, app, ctx, txGen, ak, bk, admin, msg, nil)
	}
}

// SimulateMsgSetSpendPolicy constrains a treasury's spending.
func SimulateMsgSetSpendPolicy(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgSetSpendPolicy{}

		treasury, admin, found, err := randomAdministeredTreasury(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read treasuries"), nil, err
		}
		if !found {
			return noOp(msg, "no treasury is administered by a simulation account"), nil, nil
		}

		msg.Admin = admin.Address.String()
		msg.Policy = types.SpendPolicy{
			TreasuryId:          treasury.Id,
			Denom:               sdk.DefaultBondDenom,
			PerTransactionLimit: fmt.Sprintf("%d", r.Intn(1_000_000)+1),
			PeriodLimit:         fmt.Sprintf("%d", r.Intn(10_000_000)+1),
			PeriodSeconds:       uint64(r.Intn(86_400) + 1),
		}

		return deliver(r, app, ctx, txGen, ak, bk, admin, msg, nil)
	}
}

// deliver signs and broadcasts a message, wrapping the boilerplate every
// operation above would otherwise repeat.
func deliver(
	r *rand.Rand,
	app *baseapp.BaseApp,
	ctx sdk.Context,
	txGen client.TxConfig,
	ak types.AuthKeeper,
	bk types.BankKeeper,
	signer simtypes.Account,
	msg sdk.Msg,
	spent sdk.Coins,
) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
	return simulation.GenAndDeliverTxWithRandFees(simulation.OperationInput{
		R:               r,
		App:             app,
		TxGen:           txGen,
		Cdc:             nil,
		Msg:             msg,
		CoinsSpentInMsg: spent,
		Context:         ctx,
		SimAccount:      signer,
		AccountKeeper:   ak,
		Bankkeeper:      bk,
		ModuleName:      types.ModuleName,
	})
}

func noOp(msg sdk.Msg, reason string) simtypes.OperationMsg {
	return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), reason)
}

func randomLockType(r *rand.Rand) types.LockType {
	if r.Intn(2) == 0 {
		return types.LockType_LOCK_TYPE_TIME
	}
	return types.LockType_LOCK_TYPE_VESTING
}

// randomTreasury picks any existing treasury.
func randomTreasury(ctx sdk.Context, k keeper.Keeper, r *rand.Rand) (types.Treasury, bool, error) {
	var all []types.Treasury
	if err := k.Treasury.Walk(ctx, nil, func(_ uint64, v types.Treasury) (bool, error) {
		all = append(all, v)
		return false, nil
	}); err != nil {
		return types.Treasury{}, false, err
	}
	if len(all) == 0 {
		return types.Treasury{}, false, nil
	}
	return all[r.Intn(len(all))], true, nil
}

// randomAdministeredTreasury picks a treasury whose admin is a simulation
// account, so the resulting message can actually be signed.
func randomAdministeredTreasury(
	ctx sdk.Context,
	k keeper.Keeper,
	r *rand.Rand,
	accs []simtypes.Account,
) (types.Treasury, simtypes.Account, bool, error) {
	var all []types.Treasury
	if err := k.Treasury.Walk(ctx, nil, func(_ uint64, v types.Treasury) (bool, error) {
		all = append(all, v)
		return false, nil
	}); err != nil {
		return types.Treasury{}, simtypes.Account{}, false, err
	}
	if len(all) == 0 {
		return types.Treasury{}, simtypes.Account{}, false, nil
	}

	offset := r.Intn(len(all))
	for i := range all {
		candidate := all[(offset+i)%len(all)]
		addr, err := sdk.AccAddressFromBech32(candidate.Admin)
		if err != nil {
			continue
		}
		if acc, ok := simtypes.FindAccount(accs, addr); ok {
			return candidate, acc, true, nil
		}
	}
	return types.Treasury{}, simtypes.Account{}, false, nil
}

// randomClaimableLock finds a lock with something available to a signer we hold.
func randomClaimableLock(
	ctx sdk.Context,
	k keeper.Keeper,
	r *rand.Rand,
	accs []simtypes.Account,
) (types.Lock, simtypes.Account, bool, error) {
	var candidates []types.Lock
	now := ctx.BlockTime().Unix()

	if err := k.Lock.Walk(ctx, nil, func(_ uint64, v types.Lock) (bool, error) {
		if v.Active && types.ClaimableAmount(v, now).IsPositive() {
			candidates = append(candidates, v)
		}
		return false, nil
	}); err != nil {
		return types.Lock{}, simtypes.Account{}, false, err
	}
	if len(candidates) == 0 {
		return types.Lock{}, simtypes.Account{}, false, nil
	}

	offset := r.Intn(len(candidates))
	for i := range candidates {
		lock := candidates[(offset+i)%len(candidates)]
		addr, err := sdk.AccAddressFromBech32(lock.Beneficiary)
		if err != nil {
			continue
		}
		if acc, ok := simtypes.FindAccount(accs, addr); ok {
			return lock, acc, true, nil
		}
	}
	return types.Lock{}, simtypes.Account{}, false, nil
}

// randomActiveRevocableLock finds a lock that may still be cancelled.
func randomActiveRevocableLock(ctx sdk.Context, k keeper.Keeper, r *rand.Rand) (types.Lock, bool, error) {
	var candidates []types.Lock
	if err := k.Lock.Walk(ctx, nil, func(_ uint64, v types.Lock) (bool, error) {
		if v.Active && v.Revocable {
			candidates = append(candidates, v)
		}
		return false, nil
	}); err != nil {
		return types.Lock{}, false, err
	}
	if len(candidates) == 0 {
		return types.Lock{}, false, nil
	}
	return candidates[r.Intn(len(candidates))], true, nil
}
