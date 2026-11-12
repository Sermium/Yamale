package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"

	"yamale/blockchain/x/jewel/types"
)

// Keeper holds the jewel register. x/nft holds the tokens; this keeps the
// rules about who may mint, move and use them.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	// The x/gov module account. It appoints the office; the office admits
	// jewelers.
	authority []byte

	groupKeeper types.GroupKeeper
	nft         types.NFTKeeper
	circuit     types.CircuitKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Jeweler      collections.Map[uint64, types.Jeweler]
	Jewel        collections.Map[uint64, types.Jewel]
	Offer        collections.Map[uint64, types.Offer] // by jewel id; one open offer per jewel
	Benefit      collections.Map[uint64, types.Benefit]
	Reassignment collections.Map[uint64, types.Reassignment] // by jewel id
	// (benefit id, nonce): vouchers already spent.
	UsedNonce collections.KeySet[collections.Pair[uint64, string]]

	// business address -> jeweler id. One business, one jeweler.
	ByBusiness collections.Map[string, uint64]
	// staff address -> jeweler id. A person works for one jeweler at a time.
	ByStaff collections.Map[string, uint64]
	// (jeweler id, normalised inscription) -> jewel id. The uniqueness rule.
	ByInscription collections.Map[collections.Pair[uint64, string], uint64]
	// (normalised inscription, jewel id): the public search across jewelers.
	Search collections.KeySet[collections.Pair[string, uint64]]
	// (jewel id, benefit id): one jewel's benefits as a prefix walk.
	BenefitsByJewel collections.KeySet[collections.Pair[uint64, uint64]]

	NextJewelerID collections.Sequence
	NextJewelID   collections.Sequence
	NextBenefitID collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	groupKeeper types.GroupKeeper,
	nftKeeper types.NFTKeeper,
	circuit types.CircuitKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)
	pairU64Str := collections.PairKeyCodec(collections.Uint64Key, collections.StringKey)

	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		groupKeeper:  groupKeeper,
		nft:          nftKeeper,
		circuit:      circuit,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		Jeweler: collections.NewMap(sb, types.JewelerKey, "jeweler",
			collections.Uint64Key, codec.CollValue[types.Jeweler](cdc)),
		Jewel: collections.NewMap(sb, types.JewelKey, "jewel",
			collections.Uint64Key, codec.CollValue[types.Jewel](cdc)),
		Offer: collections.NewMap(sb, types.OfferKey, "offer",
			collections.Uint64Key, codec.CollValue[types.Offer](cdc)),
		Benefit: collections.NewMap(sb, types.BenefitKey, "benefit",
			collections.Uint64Key, codec.CollValue[types.Benefit](cdc)),
		Reassignment: collections.NewMap(sb, types.ReassignmentKey, "reassignment",
			collections.Uint64Key, codec.CollValue[types.Reassignment](cdc)),
		UsedNonce: collections.NewKeySet(sb, types.UsedNonceKey, "used_nonce", pairU64Str),

		ByBusiness: collections.NewMap(sb, types.ByBusinessKey, "by_business",
			collections.StringKey, collections.Uint64Value),
		ByStaff: collections.NewMap(sb, types.ByStaffKey, "by_staff",
			collections.StringKey, collections.Uint64Value),
		ByInscription: collections.NewMap(sb, types.ByInscriptionKey, "by_inscription",
			pairU64Str, collections.Uint64Value),
		Search: collections.NewKeySet(sb, types.SearchKey, "search",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		BenefitsByJewel: collections.NewKeySet(sb, types.BenefitsByJewelKey, "benefits_by_jewel",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NextJewelerID: collections.NewSequence(sb, types.NextJewelerIDKey, "next_jeweler_id"),
		NextJewelID:   collections.NewSequence(sb, types.NextJewelIDKey, "next_jewel_id"),
		NextBenefitID: collections.NewSequence(sb, types.NextBenefitIDKey, "next_benefit_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

// GetAuthority returns the governance authority.
func (k Keeper) GetAuthority() []byte { return k.authority }

// nextID hands out an id, skipping zero: in proto3 an id of 0 is
// indistinguishable from an unset field.
func nextID(ctx context.Context, seq collections.Sequence) (uint64, error) {
	id, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return seq.Next(ctx)
	}
	return id, nil
}

func isNotFound(err error) bool { return errors.Is(err, collections.ErrNotFound) }

func (k Keeper) now(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}

// emit announces a change, after the state that records it has been written.
func (k Keeper) emit(ctx context.Context, ev proto.Message) error {
	return sdk.UnwrapSDKContext(ctx).EventManager().EmitTypedEvent(ev)
}

func (k Keeper) chainID(ctx context.Context) string {
	return sdk.UnwrapSDKContext(ctx).ChainID()
}

// canonical returns the one spelling of an account. Bech32 accepts an
// all-upper-case string for the same account, so every address is brought
// to this form before it is compared, used as a key or stored.
func (k Keeper) canonical(addr string) (string, error) {
	b, err := k.addressCodec.StringToBytes(addr)
	if err != nil {
		return "", types.ErrInvalidAddress.Wrapf("%s", addr)
	}
	return k.addressCodec.BytesToString(b)
}

// requireOffice refuses anyone but the appointed office.
func (k Keeper) requireOffice(ctx context.Context, signer string) error {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	if p.Office == "" {
		return types.ErrNoOffice
	}
	signer, err = k.canonical(signer)
	if err != nil {
		return err
	}
	if signer != p.Office {
		return types.ErrNotOffice.Wrapf("%s", signer)
	}
	return nil
}

// requireBareSendClosed refuses to admit or mint while x/nft's own send can
// still move a jewel past every rule here.
func (k Keeper) requireBareSendClosed(ctx context.Context) error {
	allowed, err := k.circuit.IsAllowed(ctx, types.NFTSendTypeURL)
	if err != nil {
		return err
	}
	if allowed {
		return types.ErrBareSendOpen
	}
	return nil
}

func (k Keeper) getJeweler(ctx context.Context, id uint64) (types.Jeweler, error) {
	j, err := k.Jeweler.Get(ctx, id)
	if isNotFound(err) {
		return types.Jeweler{}, types.ErrNoJeweler.Wrapf("%d", id)
	}
	return j, err
}

func (k Keeper) activeJeweler(ctx context.Context, id uint64) (types.Jeweler, error) {
	j, err := k.getJeweler(ctx, id)
	if err != nil {
		return j, err
	}
	if j.Status != types.JEWELER_ACTIVE {
		return j, types.ErrJewelerSuspended.Wrapf("%s", j.Name)
	}
	return j, nil
}

// requireStaff returns the jeweler, active, that signer works for, and
// refuses if that is not jewelerID.
func (k Keeper) requireStaff(ctx context.Context, signer string, jewelerID uint64) (types.Jeweler, error) {
	got, err := k.ByStaff.Get(ctx, signer)
	if isNotFound(err) || (err == nil && got != jewelerID) {
		return types.Jeweler{}, types.ErrNotStaff.Wrapf("%s", signer)
	}
	if err != nil {
		return types.Jeweler{}, err
	}
	return k.activeJeweler(ctx, jewelerID)
}

func (k Keeper) getJewel(ctx context.Context, id uint64) (types.Jewel, error) {
	j, err := k.Jewel.Get(ctx, id)
	if isNotFound(err) {
		return types.Jewel{}, types.ErrNoJewel.Wrapf("%d", id)
	}
	return j, err
}

// Owner reads the jewel's owner from x/nft, the only record of it.
func (k Keeper) Owner(ctx context.Context, j types.Jewel) (string, error) {
	addr := k.nft.GetOwner(ctx, types.ClassID(j.JewelerId), types.TokenID(j.Id))
	if addr.Empty() {
		return "", types.ErrNoJewel.Wrapf("%d has no owner in x/nft", j.Id)
	}
	return k.addressCodec.BytesToString(addr)
}

// actsForOwner accepts the owner, or, while the jewel is still the jeweler's
// stock, an active member of that jeweler's staff. It returns the owner.
func (k Keeper) actsForOwner(ctx context.Context, signer string, j types.Jewel) (string, error) {
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return "", err
	}
	signer, err = k.canonical(signer)
	if err != nil {
		return "", err
	}
	if signer == owner {
		return owner, nil
	}
	jr, err := k.getJeweler(ctx, j.JewelerId)
	if err != nil {
		return "", err
	}
	if owner == jr.Business && jr.Status == types.JEWELER_ACTIVE {
		if got, err := k.ByStaff.Get(ctx, signer); err == nil && got == jr.Id {
			return owner, nil
		}
	}
	return "", types.ErrNotOwner.Wrapf("%s", signer)
}

// transfer moves the token, counts and dates the new owner, and closes any
// offer and any pending reassignment: a change of owner voids both. kind is
// how it moved, one of types.TransferAccept, TransferClaim, TransferReassign.
func (k Keeper) transfer(ctx context.Context, j *types.Jewel, to, kind string) error {
	addr, err := k.addressCodec.StringToBytes(to)
	if err != nil {
		return types.ErrInvalidAddress.Wrapf("%s", to)
	}
	from, err := k.Owner(ctx, *j)
	if err != nil {
		return err
	}
	if err := k.nft.Transfer(ctx, types.ClassID(j.JewelerId), types.TokenID(j.Id), addr); err != nil {
		return err
	}
	j.OwnerCount++
	j.OwnerSince = append(j.OwnerSince, k.now(ctx))
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return err
	}
	// Remove on a missing key is not an error in collections.
	if err := k.Reassignment.Remove(ctx, j.Id); err != nil {
		return err
	}
	if err := k.Jewel.Set(ctx, j.Id, *j); err != nil {
		return err
	}
	return k.emit(ctx, &types.EventJewelTransferred{JewelId: j.Id, From: from, To: to, Kind: kind})
}
