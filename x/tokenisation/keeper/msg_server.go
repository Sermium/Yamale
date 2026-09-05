package keeper

import (
	"bytes"
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

type msgServer struct{ Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

func (m msgServer) assertGov(addr string) error {
	b, err := m.addressCodec.StringToBytes(addr)
	if err != nil {
		return err
	}
	if !bytes.Equal(b, m.authority) {
		return types.ErrInvalidSigner
	}
	return nil
}

func (m msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.assertGov(msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, m.Params.Set(ctx, msg.Params)
}

// CreateCollection is governance only. There is no permissionless counterpart:
// a registry of deeds is not something a chain grants on request.
func (m msgServer) CreateCollection(ctx context.Context, msg *types.MsgCreateCollection) (*types.MsgCreateCollectionResponse, error) {
	if err := m.assertGov(msg.Authority); err != nil {
		return nil, err
	}

	c := msg.Collection
	if c.Verification == types.VERIFY_ATTESTORS && c.AttestationThreshold < 2 {
		return nil, types.ErrInvalidThreshold
	}
	// Checked here rather than left to the first attestation, because a
	// collection whose threshold exceeds its register is one where no honest
	// sale can ever finalise — and the way that presents is every vehicle in it
	// silently becoming a one-way door months later, with the money already in.
	if err := validAttestorRegister(m.addressCodec, c.Verification, c.AttestationThreshold, c.Attestors); err != nil {
		return nil, err
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if c.ChallengeWindowSeconds < params.MinChallengeWindowSeconds ||
		c.ChallengeWindowSeconds > params.MaxChallengeWindowSeconds {
		return nil, types.ErrInvalidParams
	}
	if uint32(c.DisputeBondBps) > params.MaxDisputeBondBps {
		return nil, types.ErrInvalidParams
	}

	if ok, err := m.Collections.Has(ctx, c.Id); err != nil {
		return nil, err
	} else if ok {
		return nil, types.ErrCollectionExists
	}

	return &types.MsgCreateCollectionResponse{}, m.Collections.Set(ctx, c.Id, c)
}

// SetCollectionAuthority appoints or revokes. Revocation stops future mints and
// touches nothing already issued — deciding an existing asset was wrongly
// issued is a seizure, and seizures go through x/enforcement.
func (m msgServer) SetCollectionAuthority(ctx context.Context, msg *types.MsgSetCollectionAuthority) (*types.MsgSetCollectionAuthorityResponse, error) {
	if err := m.assertGov(msg.Authority); err != nil {
		return nil, err
	}
	c, err := m.Collections.Get(ctx, msg.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}
	c.Authority = msg.NewAuthority
	return &types.MsgSetCollectionAuthorityResponse{}, m.Collections.Set(ctx, msg.CollectionId, c)
}

// MintAsset records title. Only the collection's appointed authority, and the
// asset is attributed to its owner at creation — there is no self-mint-then-
// transfer path, because that is where an authority laundering assets to itself
// stops being distinguishable from one doing its job.
func (m msgServer) MintAsset(ctx context.Context, msg *types.MsgMintAsset) (*types.MsgMintAssetResponse, error) {
	c, err := m.Collections.Get(ctx, msg.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}
	if c.Authority == "" {
		return nil, types.ErrNoAuthority
	}
	if c.Authority != msg.Minter {
		return nil, types.ErrNotAuthority
	}
	if _, err := m.addressCodec.StringToBytes(msg.Owner); err != nil {
		return nil, err
	}
	// Checked here as well as at fractionalisation because it costs one read.
	// A vehicle over a parcel that does not exist, or over one held by somebody
	// other than the named owner, is refused before anybody has been sold a
	// share in it — the alternative is discovering it at issuance, with a buyer
	// on the other side.
	if msg.ParcelId != 0 {
		if _, err := m.parcelHeldBy(ctx, msg.ParcelId, msg.Owner); err != nil {
			return nil, err
		}
	}

	id, err := m.NextAssetID.Next(ctx)
	if err != nil {
		return nil, err
	}
	// Ids start at 1. Zero is indistinguishable from an unset proto field.
	id++

	asset := types.Asset{
		Id:           id,
		CollectionId: msg.CollectionId,
		Owner:        msg.Owner,
		Uri:          msg.Uri,
		ParcelId:     msg.ParcelId,
		Status:       types.STATUS_HELD,
	}
	if err := m.Assets.Set(ctx, id, asset); err != nil {
		return nil, err
	}
	return &types.MsgMintAssetResponse{AssetId: id}, nil
}

// Fractionalise mints the shareholding. Supply is fixed here forever.
func (m msgServer) Fractionalise(ctx context.Context, msg *types.MsgFractionalise) (*types.MsgFractionaliseResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Owner != msg.Owner {
		return nil, types.ErrNotOwner
	}
	if asset.FractionDenom != "" {
		return nil, types.ErrAlreadyFractionalised
	}
	if msg.HolderShareBps == 0 || msg.HolderShareBps > 10_000 {
		return nil, types.ErrInvalidShare
	}
	if !msg.Supply.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	// The supervised bridge from x/land. An asset that names no parcel is not
	// land and is untouched by any of it.
	if asset.ParcelId != 0 {
		if err := m.assertLandPermitsIssue(
			ctx, asset.ParcelId, msg.Owner, msg.HolderShareBps,
		); err != nil {
			return nil, err
		}
	}

	owner, err := m.addressCodec.StringToBytes(msg.Owner)
	if err != nil {
		return nil, err
	}

	denom := types.FractionDenom(msg.AssetId, msg.Symbol)
	coins := sdk.NewCoins(sdk.NewCoin(denom, msg.Supply))
	if err := m.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, owner, coins); err != nil {
		return nil, err
	}

	asset.FractionDenom = denom
	asset.HolderShareBps = msg.HolderShareBps
	asset.Status = types.STATUS_ACTIVE
	if err := m.Assets.Set(ctx, msg.AssetId, asset); err != nil {
		return nil, err
	}
	if asset.ParcelId != 0 {
		if err := m.recordIssuedShare(ctx, asset.ParcelId, msg.HolderShareBps); err != nil {
			return nil, err
		}
	}
	if err := m.ByDenom.Set(ctx, denom, msg.AssetId); err != nil {
		return nil, err
	}
	if err := m.Vaults.Set(ctx, msg.AssetId, types.Vault{
		AssetId:            msg.AssetId,
		CumulativePerToken: math.LegacyZeroDec(),
		Denom:              msg.IncomeDenom,
	}); err != nil {
		return nil, err
	}

	// Seed the first holder's position, at the index the vault has just been
	// created with - zero.
	//
	// Without this they have no position at all, and Settle's rule for an
	// account it has never seen is to start it at the CURRENT index, because
	// crediting a new buyer the asset's entire history would pay them for a
	// period they did not hold. Right for a buyer, wrong for this account: it
	// has held every share since issuance. The effect was that a holder who
	// never transferred anything had their position created on the way out, at
	// an index that had already moved, and was paid nothing for the whole life
	// of the vehicle.
	//
	// It stayed hidden because any transfer settles both sides, so the ordinary
	// path - issue, then distribute - creates the position early and by
	// accident. What it hits is the case with no transfer at all.
	if err := m.Settle(ctx, msg.AssetId, owner, msg.Supply); err != nil {
		return nil, err
	}

	return &types.MsgFractionaliseResponse{FractionDenom: denom}, nil
}

// TransferAsset moves title. The shareholding is untouched, and the obligation
// to fund the vault moves with title rather than staying behind.
func (m msgServer) TransferAsset(ctx context.Context, msg *types.MsgTransferAsset) (*types.MsgTransferAssetResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Owner != msg.Owner {
		return nil, types.ErrNotOwner
	}
	if _, err := m.addressCodec.StringToBytes(msg.Recipient); err != nil {
		return nil, err
	}
	asset.Owner = msg.Recipient
	return &types.MsgTransferAssetResponse{}, m.Assets.Set(ctx, msg.AssetId, asset)
}

// FundVault pays income in and raises the index.
func (m msgServer) FundVault(ctx context.Context, msg *types.MsgFundVault) (*types.MsgFundVaultResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_ACTIVE && asset.Status != types.STATUS_REALISED {
		return nil, types.ErrWrongStatus
	}
	if !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	vault, err := m.Vaults.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, err
	}
	// The index is denominated in the vault's income denom, and AccrueIncome
	// credits it by amount alone. Without this check anyone funds a worthless
	// token, the index rises as though real income arrived, and the next
	// claimant withdraws the vault in the denom that actually has value.
	if msg.Amount.Denom != vault.Denom {
		return nil, types.ErrWrongDenom
	}

	funder, err := m.addressCodec.StringToBytes(msg.Funder)
	if err != nil {
		return nil, err
	}

	// Only the holders' share is collected. The rest is the sponsor's share of
	// their own asset, and the module used to take it into an account with no
	// message that pays it back — on a vehicle with holder_share_bps of 2,500,
	// three quarters of every rent payment was stranded. Leaving it with the
	// funder is what the message has always said happens.
	collected := sdk.NewCoin(msg.Amount.Denom, holderCut(asset, msg.Amount.Amount))
	if !collected.Amount.IsPositive() {
		return nil, types.ErrAmountTooSmall
	}
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, funder, types.ModuleName, sdk.NewCoins(collected)); err != nil {
		return nil, err
	}
	if err := m.hold(ctx, msg.AssetId, collected); err != nil {
		return nil, err
	}
	if err := m.AccrueIncome(ctx, msg.AssetId, msg.Amount.Amount); err != nil {
		return nil, err
	}
	return &types.MsgFundVaultResponse{Collected: collected}, nil
}

// Claim withdraws accrued income without giving up the shareholding — a coupon
// should not force an exit.
func (m msgServer) Claim(ctx context.Context, msg *types.MsgClaim) (*types.MsgClaimResponse, error) {
	holder, err := m.addressCodec.StringToBytes(msg.Holder)
	if err != nil {
		return nil, err
	}
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	vault, err := m.Vaults.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, err
	}

	balance := m.bankKeeper.GetBalance(sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
	if err := m.Settle(ctx, msg.AssetId, holder, balance); err != nil {
		return nil, err
	}

	key := collections.Join(msg.AssetId, msg.Holder)
	pos, err := m.Positions.Get(ctx, key)
	if err != nil || !pos.Accrued.IsPositive() {
		return nil, types.ErrNothingOwed
	}

	paid := sdk.NewCoin(vault.Denom, pos.Accrued)
	pos.Accrued = math.ZeroInt()
	if err := m.Positions.Set(ctx, key, pos); err != nil {
		return nil, err
	}
	// Off this vehicle's ledger before the bank keeper is asked, so a payout
	// this vehicle cannot cover fails here rather than succeeding out of a
	// different vehicle's rent.
	if err := m.release(ctx, msg.AssetId, paid); err != nil {
		return nil, err
	}
	payout := sdk.NewCoins(paid)
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, holder, payout); err != nil {
		return nil, err
	}
	return &types.MsgClaimResponse{Paid: payout}, nil
}

// Redeem burns tokens and pays their share in one step.
//
// The burn *is* the claim. Burning first and expecting a later claim would
// strand the money of everyone slow, asleep or dead, and leave the module
// holding funds it can no longer attribute.
func (m msgServer) Redeem(ctx context.Context, msg *types.MsgRedeem) (*types.MsgRedeemResponse, error) {
	holder, err := m.addressCodec.StringToBytes(msg.Holder)
	if err != nil {
		return nil, err
	}
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_REALISED {
		return nil, types.ErrWrongStatus
	}
	if !msg.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	vault, err := m.Vaults.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, err
	}

	balance := m.bankKeeper.GetBalance(sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
	if err := m.Settle(ctx, msg.AssetId, holder, balance); err != nil {
		return nil, err
	}

	key := collections.Join(msg.AssetId, msg.Holder)
	pos, err := m.Positions.Get(ctx, key)
	if err != nil {
		return nil, types.ErrNothingOwed
	}

	// A position exists for anyone who has ever held the denom, including
	// somebody who has since transferred it all away. Dividing by their balance
	// would panic on zero -- and a panic in a permissionless handler is not a
	// failed transaction, it is a halted chain. Acquire one token, send it
	// away, call this: every validator stops.
	if !balance.IsPositive() {
		return nil, types.ErrNothingOwed
	}
	// Redeeming more than you hold would compute a share larger than you are
	// owed and drive the position negative. The burn below would fail and
	// revert, but only by accident of ordering; refuse it here where the
	// reason is legible.
	if msg.Amount.GT(balance) {
		return nil, types.ErrInvalidAmount
	}

	// The redeemer's share of what they are owed, in proportion to the tokens
	// they are surrendering. Truncated toward the vault: a fraction of a unit
	// that cannot be divided stays behind rather than being conjured, the same
	// rule that governs x/amm.
	share := pos.Accrued.Mul(msg.Amount).Quo(balance)

	burn := sdk.NewCoins(sdk.NewCoin(asset.FractionDenom, msg.Amount))
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, holder, types.ModuleName, burn); err != nil {
		return nil, err
	}
	if err := m.bankKeeper.BurnCoins(ctx, types.ModuleName, burn); err != nil {
		return nil, err
	}

	pos.Accrued = pos.Accrued.Sub(share)
	if err := m.Positions.Set(ctx, key, pos); err != nil {
		return nil, err
	}

	payout := sdk.NewCoins(sdk.NewCoin(vault.Denom, share))
	if share.IsPositive() {
		if err := m.release(ctx, msg.AssetId, sdk.NewCoin(vault.Denom, share)); err != nil {
			return nil, err
		}
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, holder, payout); err != nil {
			return nil, err
		}
	}

	// Title burns when the last share is surrendered.
	if m.bankKeeper.GetSupply(sdk.UnwrapSDKContext(ctx), asset.FractionDenom).Amount.IsZero() {
		asset.Status = types.STATUS_CLOSED
		if err := m.Assets.Set(ctx, msg.AssetId, asset); err != nil {
			return nil, err
		}
	}

	return &types.MsgRedeemResponse{Paid: payout}, nil
}

// validAttestorRegister holds a collection's register to the two things that
// can be checked from the register alone: that it can reach its own threshold,
// and that it is not padded with the same account twice.
//
// A duplicate would be the cheapest possible forgery of a quorum — one signer
// listed three times reads as three attestors to anybody skimming, and
// AttestSale's own duplicate check would then refuse the second and third
// signature from the one account that could actually meet the threshold.
func validAttestorRegister(codec address.Codec, mode types.VerificationMode, threshold uint32, attestors []string) error {
	if mode != types.VERIFY_ATTESTORS {
		// A register on a collection that verifies some other way is not
		// wrong, only unused, and refusing it would be the chain having an
		// opinion about a field it does not read.
		return nil
	}
	if uint32(len(attestors)) < threshold {
		return types.ErrInvalidThreshold.Wrapf(
			"%d appointed attestors cannot meet a threshold of %d, so no sale in this collection could ever finalise",
			len(attestors), threshold)
	}
	seen := make(map[string]struct{}, len(attestors))
	for _, a := range attestors {
		if _, err := codec.StringToBytes(a); err != nil {
			return types.ErrNotAttestor.Wrapf("%q is not an address", a)
		}
		if _, dup := seen[a]; dup {
			return types.ErrNotAttestor.Wrapf(
				"%s is listed twice; a register padded with one account is not a quorum", a)
		}
		seen[a] = struct{}{}
	}
	return nil
}

// SetCollectionAttestors replaces the register of accounts that may attest a
// sale reported under this collection.
//
// Governance only, like CreateCollection and SetCollectionAuthority beside it.
// If the seller could appoint the accounts that check the seller, this register
// would restate the problem rather than fix it.
func (m msgServer) SetCollectionAttestors(
	ctx context.Context, msg *types.MsgSetCollectionAttestors,
) (*types.MsgSetCollectionAttestorsResponse, error) {
	if err := m.assertGov(msg.Authority); err != nil {
		return nil, err
	}
	c, err := m.Collections.Get(ctx, msg.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}
	if err := validAttestorRegister(m.addressCodec, c.Verification, c.AttestationThreshold, msg.Attestors); err != nil {
		return nil, err
	}

	// Attestations already recorded are not revisited. They were made by an
	// appointed attestor at the moment they were made, and rewriting that to
	// match a later appointment would let governance manufacture or destroy a
	// quorum after the fact — a worse power than the one being constrained here.
	c.Attestors = msg.Attestors
	return &types.MsgSetCollectionAttestorsResponse{}, m.Collections.Set(ctx, msg.CollectionId, c)
}

// FinaliseSale opens redemption on an asset whose reported price has cleared
// verification and its challenge window.
//
// The crank existed as a keeper method from the beginning and nothing ever
// called it — no message, no EndBlocker, not even a test. So no asset could
// reach STATUS_REALISED, and Redeem, which requires that status, could never
// succeed: every fractionalised vehicle was a one-way door. This is the caller.
//
// Anybody may send it. Every condition is fixed by the report and the clock, so
// the sender decides nothing and contributes only the gas. If only the sponsor
// could finalise, shareholders would be unable to exit until the party holding
// their money chose to allow it.
func (m msgServer) FinaliseSale(
	ctx context.Context, msg *types.MsgFinaliseSale,
) (*types.MsgFinaliseSaleResponse, error) {
	if _, err := m.addressCodec.StringToBytes(msg.Caller); err != nil {
		return nil, err
	}
	return &types.MsgFinaliseSaleResponse{}, m.Keeper.FinaliseSale(ctx, msg.AssetId)
}
