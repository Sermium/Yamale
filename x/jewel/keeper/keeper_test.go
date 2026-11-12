package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// The keeper builds its schema without overlapping prefixes and reads back
// the params the fixture wrote.
func TestTheKeeperBuildsAndHoldsItsParams(t *testing.T) {
	f := setup(t)
	res, err := f.qs.Params(f.env.Ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, f.office, res.Params.Office)
}
