package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/codec"

	aliastypes "yamale/blockchain/x/alias/types"
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

	bankKeeper types.BankKeeper

	// scope is the jurisdictional perimeter, held behind a pointer.
	//
	// The indirection is not decoration. x/alias consults this module, so this
	// module cannot receive x/alias through depinject — that is a cycle — and it
	// is handed the perimeter after construction instead. The keeper is copied by
	// value into the AppModule that builds the message server, so a plain field
	// assigned later would be set on one copy and read on another; a pointer
	// allocated once in NewKeeper is shared by every copy, which is what makes
	// SetScopeKeeper reach the handler that actually runs.
	scope *types.ScopeKeeper

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
		scope:                  new(types.ScopeKeeper),
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

// SetScopeKeeper hands this module the jurisdictional perimeter, after
// construction.
//
// Called once, from app.go, for the reason spelled out on the field: x/alias
// consults this module, so the perimeter cannot come through the dependency
// graph without making a cycle of it. Until it is called the perimeter check
// refuses rather than passes, so forgetting this line removes the ability of a
// national payments authority to admit anybody — it does not quietly let
// everybody in.
func (k Keeper) SetScopeKeeper(scope types.ScopeKeeper) {
	*k.scope = scope
}

// assertScope refuses a signer admitting a participant outside its perimeter.
//
// Fails closed on an unwired registry, and note that this is the branch a wiring
// mistake actually takes: an unset pointer target is a nil interface, which is
// exactly the zero value that must never read as an authorisation.
func (k Keeper) assertScope(ctx context.Context, actor, target string) error {
	if k.scope == nil || *k.scope == nil {
		return aliastypes.ErrNoScopeKeeper
	}
	scope := *k.scope
	// "Not an authority at all" before "not this perimeter", so that a random
	// account sending this message is told it may not send it rather than told
	// something about the applicant. This branch permits nothing — the assertion
	// below is the gate, and it runs whatever this returns.
	holds, err := scope.HoldsRole(ctx, actor, aliastypes.ROLE_PAYMENTS_AUTHORITY)
	if err != nil {
		return err
	}
	if !holds {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner,
			"invalid authority; expected %s or a payments authority, got %s", expected, actor)
	}
	return scope.AssertScope(ctx, actor, aliastypes.ROLE_PAYMENTS_AUTHORITY, target)
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
