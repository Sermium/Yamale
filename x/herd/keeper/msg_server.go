package keeper

import (
	"bytes"
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/herd/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns the write side of the registry.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// UpdateParams sets the module parameters. Governance only.
func (m msgServer) UpdateParams(
	ctx context.Context, msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	if err := m.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// RegisterAuthority admits a registry office.
func (m msgServer) RegisterAuthority(
	ctx context.Context, msg *types.MsgRegisterAuthority,
) (*types.MsgRegisterAuthorityResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Office); err != nil {
		return nil, types.ErrInvalidKeeper.Wrapf("office %q", msg.Office)
	}

	// An office must be a group account, so that every decision it later makes
	// — enrolling a herd, certifying parentage, validating a transfer, striking
	// an animal off — already requires its own M-of-N inside the office. A
	// plain key here makes each of those a single bribe, and no amount of
	// cross-office quorum on transfers repairs registration or certification.
	if err := m.assertGroupAccount(ctx, msg.Office); err != nil {
		return nil, err
	}

	// The jurisdiction has to be a country the chain recognises. Free text
	// would admit an office to a perimeter no grant can ever cover: it would
	// look admitted and be unable to enrol a single herd, and the failure would
	// surface to an agent in a field rather than to whoever wrote the proposal.
	jurisdiction := aliastypes.NormaliseCountry(msg.Jurisdiction)
	if !aliastypes.AssignedCountry(jurisdiction) {
		return nil, types.ErrInvalidJurisdiction.Wrapf("%q", msg.Jurisdiction)
	}

	if err := m.Authority.Set(ctx, msg.Office, types.Authority{
		Address:      msg.Office,
		Name:         msg.Name,
		Jurisdiction: jurisdiction,
		Active:       msg.Active,
	}); err != nil {
		return nil, err
	}
	return &types.MsgRegisterAuthorityResponse{}, nil
}

// assertGovernance refuses anybody but the gov module account.
func (m msgServer) assertGovernance(signer string) error {
	want, err := m.addressCodec.BytesToString(m.GetAuthority())
	if err != nil {
		return err
	}
	got, err := m.addressCodec.StringToBytes(signer)
	if err != nil {
		return types.ErrNotAuthority.Wrapf("%q is not an address", signer)
	}
	if !bytes.Equal(got, m.GetAuthority()) {
		return types.ErrNotAuthority.Wrapf(
			"this message is governance-only: expected %s, got %s", want, signer)
	}
	return nil
}

// assertGroupAccount refuses an office that is a plain key.
//
// Checked rather than trusted to a human review step at proposal time. The
// alternative puts the whole intra-office protection in a step that will
// eventually be rushed, and the cost of checking is one lookup.
func (m msgServer) assertGroupAccount(ctx context.Context, office string) error {
	_, err := m.groupKeeper.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{
		Address: office,
	})
	if err != nil {
		return types.ErrOfficeNotGroup.Wrapf("%s", office)
	}
	return nil
}

// blockHeight is the height the current message is being executed at.
func blockHeight(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockHeight()
}

// blockTime is used for the challenge window rather than a height, because the
// window is a period a person has to notice something in, and a person
// experiences days rather than blocks.
func blockTime(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}
