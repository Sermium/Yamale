package keeper_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gateway "github.com/cosmos/gogogateway"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/land/types"
)

// A real cadastral reference. Registry references are structured, and the
// structure is written with slashes — country or region, office, year, serial.
// This is the shape of the string a citizen is holding when they walk up to a
// counter, and the shape the module calls its primary lookup.
const slashedRef = "ACC/GA/2019/00412"

// Survey hashes are commonly hex, but the keeper accepts any non-empty string
// and deliberately imposes no encoding — a registry recording base64 digests is
// within the rules, and the base64 alphabet contains '/'.
const slashedGeometry = "sha256:Zm9vL2Jhcg/Nq8vT+1Ig="

// restGateway serves the module's Query service exactly as the node does: the
// same grpc-gateway mux, the same gogo marshaler the SDK's API server installs
// (the default one cannot marshal the non-nullable Parcel field), and the same
// generated routing table.
//
// It registers the query server in-process rather than dialing gRPC, so the
// test exercises the routing and the request binding — which is where the
// defect was — without needing a node.
//
// The sdk.Context is attached to the inbound request rather than carried from
// the client, because that is where it comes from in the node: baseapp's query
// router puts it on the context the handler receives. Attaching it client-side
// attaches nothing at all — a request context does not cross the socket.
func (f *fixture) restGateway(t *testing.T) *httptest.Server {
	t.Helper()

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &gateway.JSONPb{
			EmitDefaults: true, OrigName: true,
		}),
		runtime.WithProtoErrorHandler(runtime.DefaultHTTPProtoErrorHandler),
	)
	require.NoError(t, types.RegisterQueryHandlerServer(
		t.Context(), mux, keeper.NewQueryServerImpl(f.k)))

	ctx := f.env.Ctx
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			mux.ServeHTTP(w, r.WithContext(ctx))
		}))
}

// getREST issues a GET and returns the status and the decoded body.
//
// The URL is assembled as a string and not through url.URL, because what is
// under test is precisely how the path and query arrive on the wire: a helper
// that re-encoded them would be testing itself.
func getREST(t *testing.T, srv *httptest.Server, path string) (int, map[string]any) {
	t.Helper()

	res, err := srv.Client().Get(srv.URL + path)
	require.NoError(t, err)
	defer res.Body.Close()

	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

// The defect this test exists for: ParcelByRef is the module's stated primary
// lookup and it could not be performed over REST at all.
//
// The reference was bound as a path segment, and a path template matches one
// segment. Every real reference contains slashes, so the route matched nothing
// — percent-encoded or raw — and the node answered 501 for the one query a
// person at a counter can actually make. The reference now travels in the query
// string, where a slash is an ordinary character.
func TestParcelByRefOverRESTCarriesSlashes(t *testing.T) {
	f := setup(t)
	id := f.register(t, "survey-A", slashedRef)

	srv := f.restGateway(t)
	defer srv.Close()

	// Percent-encoded, which is what a browser's URLSearchParams produces.
	code, body := getREST(t, srv,
		"/yamale/blockchain/land/v1/parcel_by_ref?cadastral_ref="+url.QueryEscape(slashedRef))
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	parcel, ok := body["parcel"].(map[string]any)
	require.True(t, ok, "body: %v", body)
	require.Equal(t, slashedRef, parcel["cadastral_ref"])
	require.Equal(t, "1", parcel["id"])
	require.Equal(t, id, uint64(1))

	// Raw slashes in the query string, which is legal in a query component and
	// is what a hand-typed URL or curl produces. Both forms must reach the same
	// parcel, or the fix only works for callers who happen to encode.
	code, body = getREST(t, srv,
		"/yamale/blockchain/land/v1/parcel_by_ref?cadastral_ref="+slashedRef)
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	parcel, ok = body["parcel"].(map[string]any)
	require.True(t, ok, "body: %v", body)
	require.Equal(t, slashedRef, parcel["cadastral_ref"])
}

// The old shape must be gone rather than merely superseded. A route left
// behind at /parcel_by_ref/{cadastral_ref} would keep answering for the
// slash-free references used in tests and demos, and go on failing for the
// real ones — which is exactly how this survived to begin with.
func TestParcelByRefPathFormIsNotRouted(t *testing.T) {
	f := setup(t)
	f.register(t, "survey-A", slashedRef)

	srv := f.restGateway(t)
	defer srv.Close()

	code, _ := getREST(t, srv, "/yamale/blockchain/land/v1/parcel_by_ref/"+slashedRef)
	require.NotEqual(t, http.StatusOK, code)

	// Not even for a reference that would have fitted in one segment.
	code, _ = getREST(t, srv, "/yamale/blockchain/land/v1/parcel_by_ref/REF-001")
	require.NotEqual(t, http.StatusOK, code)
}

// A reference nobody holds must be a clean not-found rather than a routing
// failure. The two were indistinguishable before — both were "the API did not
// answer" — which is how a page could report "no such title" for a title that
// existed.
func TestParcelByRefOverRESTNotFound(t *testing.T) {
	f := setup(t)
	f.register(t, "survey-A", slashedRef)

	srv := f.restGateway(t)
	defer srv.Close()

	code, _ := getREST(t, srv,
		"/yamale/blockchain/land/v1/parcel_by_ref?cadastral_ref="+url.QueryEscape("ACC/GA/2019/99999"))
	require.Equal(t, http.StatusNotFound, code)
}

// ParcelByGeometry carried the same defect latently: the keeper imposes no
// encoding on a survey hash, so a base64 digest is a lawful one and contains
// slashes.
func TestParcelByGeometryOverRESTCarriesSlashes(t *testing.T) {
	f := setup(t)
	f.register(t, slashedGeometry, "REF-001")

	srv := f.restGateway(t)
	defer srv.Close()

	code, body := getREST(t, srv,
		"/yamale/blockchain/land/v1/parcel_by_geometry?geometry_hash="+url.QueryEscape(slashedGeometry))
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	parcel, ok := body["parcel"].(map[string]any)
	require.True(t, ok, "body: %v", body)
	require.Equal(t, slashedGeometry, parcel["geometry_hash"])
}

// The freeze grounds have to be readable by the same public, unauthenticated
// surface as everything else. A reason held only in a transaction is a reason
// a citizen has to know how to search a block explorer to find.
func TestFreezeReasonIsServedOverREST(t *testing.T) {
	f := setup(t)
	id := f.register(t, "survey-A", slashedRef)

	const order = "court order 2026/114, succession disputed"
	_, err := f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Reason: order,
	})
	require.NoError(t, err)

	srv := f.restGateway(t)
	defer srv.Close()

	code, body := getREST(t, srv,
		"/yamale/blockchain/land/v1/parcel_by_ref?cadastral_ref="+url.QueryEscape(slashedRef))
	require.Equal(t, http.StatusOK, code, "body: %v", body)

	parcel := body["parcel"].(map[string]any)
	require.Equal(t, "STATUS_FROZEN", parcel["status"])
	freezes, ok := parcel["freezes"].([]any)
	require.True(t, ok, "body: %v", body)
	require.Len(t, freezes, 1)

	freeze := freezes[0].(map[string]any)
	require.Equal(t, order, freeze["reason"])
	require.Equal(t, f.office, freeze["imposed_by"])
	require.Equal(t, false, freeze["lifted"])
}
