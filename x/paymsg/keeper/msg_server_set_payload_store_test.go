package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// The payee is the party that has to find the payload, and the only thing it is
// guaranteed to have is the payment record — which names the instructing
// participant and nothing else. So the store address is a directory entry on
// the chain rather than something every client configures separately.
func TestSetPayloadStoreRecordsAndWithdrawsTheURL(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	_, participant := newParticipant(t, f, ms, "00000001", "Bank One", true)

	_, err := ms.SetPayloadStore(f.ctx, &types.MsgSetPayloadStore{
		Participant: participant, Url: "https://payloads.bankone.example",
	})
	require.NoError(t, err)

	rec, err := f.keeper.ApprovedParticipant.Get(f.ctx, participant)
	require.NoError(t, err)
	require.Equal(t, "https://payloads.bankone.example", rec.PayloadStoreUrl)

	// The rest of the record survives. Writing a fresh ApprovedParticipant to
	// set one field would blank the code and the name, which are what a
	// statement export prints.
	require.Equal(t, "00000001", rec.Code)
	require.Equal(t, "Bank One", rec.Name)

	// Withdrawing is a supported act, not an error. A participant winding down
	// its service should be able to say so, and a client that then reports the
	// detail as unavailable is telling the truth.
	_, err = ms.SetPayloadStore(f.ctx, &types.MsgSetPayloadStore{Participant: participant, Url: ""})
	require.NoError(t, err)

	rec, err = f.keeper.ApprovedParticipant.Get(f.ctx, participant)
	require.NoError(t, err)
	require.Empty(t, rec.PayloadStoreUrl)
	require.Equal(t, "Bank One", rec.Name)
}

// A value that is not a URL is not a store, and the failure lands on the payee
// rather than on whoever registered it — so it is refused where it is written.
func TestSetPayloadStoreRefusesSomethingThatIsNotAStore(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	_, participant := newParticipant(t, f, ms, "00000001", "Bank One", true)

	for name, url := range map[string]string{
		"no scheme":      "payloads.bankone.example",
		"wrong scheme":   "ftp://payloads.bankone.example",
		"no host":        "https://",
		"query string":   "https://payloads.bankone.example?tenant=1",
		"fragment":       "https://payloads.bankone.example#here",
		"path traversal": "https://payloads.bankone.example/../other",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ms.SetPayloadStore(f.ctx, &types.MsgSetPayloadStore{
				Participant: participant, Url: url,
			})
			require.ErrorIs(t, err, types.ErrInvalidPayloadStore)
		})
	}
}

// Whoever can rewrite this field decides which host the payee's client asks for
// the detail of a payment. A third party able to set it could point every
// retrieval at a server it controls and learn which payments are being read and
// by whom, even though it can decrypt none of them.
func TestOnlyAnApprovedParticipantMayRecordItsOwnStore(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	_, pending := newParticipant(t, f, ms, "00000002", "Bank Two", false)

	_, err := ms.SetPayloadStore(f.ctx, &types.MsgSetPayloadStore{
		Participant: pending, Url: "https://payloads.banktwo.example",
	})
	require.ErrorIs(t, err, types.ErrNotApprovedParticipant)
}

// One function builds the retrieval URL, so the payer's wallet, the payee's
// client, the regulator's tooling and the store itself all derive the same one.
// Two implementations that disagree by a slash produce a 404 — and a 404 here
// renders as detail that has been erased, which is a materially different
// statement from a path that was built wrong.
func TestPayloadStoreEndpointIsBuiltInOnePlace(t *testing.T) {
	require.Equal(t,
		"https://store.example/payloads/yml1one/E2E-1",
		types.PayloadStoreEndpoint("https://store.example", "yml1one", "E2E-1"))

	// A trailing slash on the registered base must not produce a double slash.
	require.Equal(t,
		"https://store.example/payloads/yml1one/E2E-1",
		types.PayloadStoreEndpoint("https://store.example/", "yml1one", "E2E-1"))

	// An end-to-end id is ISO 20022 free text and may carry a slash, which
	// would otherwise open a path segment the store never meant to serve.
	require.Equal(t,
		"https://store.example/payloads/yml1one/E2E%2F..%2Fadmin",
		types.PayloadStoreEndpoint("https://store.example", "yml1one", "E2E/../admin"))
}
