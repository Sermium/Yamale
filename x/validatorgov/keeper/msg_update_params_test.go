package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

func TestMsgUpdateParams(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)

	// default params
	testCases := []struct {
		name      string
		input     *types.MsgUpdateParams
		expErr    bool
		expErrMsg string
	}{
		{
			name: "invalid authority",
			input: &types.MsgUpdateParams{
				Authority: "invalid",
				Params:    params,
			},
			expErr:    true,
			expErrMsg: "invalid authority",
		},
		{
			// An empty Params is refused rather than accepted as "all defaults".
			// Both fields are delays that gate a validator's operator key
			// changing hands, and a zero for either would take effect in the
			// block the rotation was submitted in.
			name: "empty params",
			input: &types.MsgUpdateParams{
				Authority: authorityStr,
				Params:    types.Params{},
			},
			expErr:    true,
			expErrMsg: "planned_rotation_delay_blocks must be positive",
		},
		{
			// A challenge window shorter than the planned delay would make
			// claiming a lost key the quick way to rotate one.
			name: "challenge window shorter than the planned delay",
			input: &types.MsgUpdateParams{
				Authority: authorityStr,
				Params: types.NewParams(
					types.DefaultPlannedRotationDelayBlocks,
					types.DefaultPlannedRotationDelayBlocks-1,
					types.DefaultAttestationIntervalBlocks,
					types.DefaultSeatBondAmount(),
				),
			},
			expErr:    true,
			expErrMsg: "must be at least planned_rotation_delay_blocks",
		},
		{
			name: "all good",
			input: &types.MsgUpdateParams{
				Authority: authorityStr,
				Params:    params,
			},
			expErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.UpdateParams(f.ctx, tc.input)

			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
