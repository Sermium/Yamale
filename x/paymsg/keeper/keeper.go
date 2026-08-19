package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"

	"yamale/blockchain/x/paymsg/types"
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

	bankKeeper             types.BankKeeper
	ParticipantApplication collections.Map[string, types.ParticipantApplication]
	ApprovedParticipant    collections.Map[string, types.ApprovedParticipant]
	// PaymentRecord is keyed by (instructing participant, end-to-end id).
	//
	// The id alone was a single global namespace, so one participant could take
	// a reference another intended to use and block it forever — and two
	// participants legitimately using the same invoice number would collide
	// without anybody being malicious. ISO 20022 scopes uniqueness to the
	// instructing party, and so does this.
	PaymentRecord collections.Map[collections.Pair[string, string], types.PaymentRecord]

	// Customer maps an account to the participant that acts for it.
	Customer collections.Map[string, types.Customer]
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

		bankKeeper:             bankKeeper,
		Params:                 collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		ParticipantApplication: collections.NewMap(sb, types.ParticipantApplicationKey, "participantApplication", collections.StringKey, codec.CollValue[types.ParticipantApplication](cdc)), ApprovedParticipant: collections.NewMap(sb, types.ApprovedParticipantKey, "approvedParticipant", collections.StringKey, codec.CollValue[types.ApprovedParticipant](cdc)), PaymentRecord: collections.NewMap(sb, types.PaymentRecordKey, "paymentRecord",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.PaymentRecord](cdc)),
		Customer: collections.NewMap(sb, types.CustomerKey, "customer", collections.StringKey, codec.CollValue[types.Customer](cdc)),
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

// ApprovedParticipantExists reports whether an institution is admitted right
// now.
//
// Exposed for x/alias, which will not let an institution record where an
// account is unless the rail still recognises it. Approval is withdrawable, so
// the question has to be asked at the moment of the act rather than inferred
// from a customer relationship registered when the answer was different.
func (k Keeper) ApprovedParticipantExists(ctx context.Context, participant string) (bool, error) {
	return k.ApprovedParticipant.Has(ctx, participant)
}

// ParticipantOf reports which approved participant acts for an account.
//
// found is false rather than an error for an account that banks nowhere: that
// is an answer, and the caller decides what it means.
func (k Keeper) ParticipantOf(ctx context.Context, account string) (string, bool, error) {
	c, err := k.Customer.Get(ctx, account)
	switch {
	case err == nil:
		return c.Participant, true, nil
	case errors.Is(err, collections.ErrNotFound):
		return "", false, nil
	default:
		return "", false, err
	}
}
