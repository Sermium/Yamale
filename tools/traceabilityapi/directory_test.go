package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func putProfile(t *testing.T, s *server, body string) int {
	biz := testKey("biz")
	b := []byte(body)
	return serve(s, sign(t, request("PUT", "/v1/jewelers/1/profile", b), biz, "profile", "1."+hashHex(b), testNow.Unix())).Code
}

func TestAShopsContactIsPartOfItsProfile(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("biz")))
	body := `{"logo":"","colour":"#12253f","accent":"#a87b3c","contact":{"about":"Bijoutier depuis 1984.\nOr et argent.",` +
		`"address":"12 rue Mohamed V","city":"Dakar","country":"SN","phone":"+221 33 821 00 00","whatsapp":"+221 77 000 00 00",` +
		`"email":"contact@maison-diop.sn","website":"https://maison-diop.sn","hours":"Lun–Sam 9h–19h"}}`
	require.Equal(t, http.StatusOK, putProfile(t, s, body))
	got := serve(s, request("GET", "/v1/jewelers/1/profile", nil))
	require.JSONEq(t, body, got.Body.String())
}

func TestAContactIsChecked(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("biz")))
	for _, c := range []string{
		`{"country":"Senegal"}`,
		`{"phone":"call me"}`,
		`{"email":"nobody"}`,
		`{"website":"http://maison-diop.sn"}`,
		`{"website":"https://maison diop.sn"}`,
		`{"city":"Dakar\u0007"}`,
		`{"city":"Dakar\nPlateau"}`,
		`{"twitter":"@diop"}`,
	} {
		body := `{"logo":"","colour":"#12253f","accent":"#a87b3c","contact":` + c + `}`
		require.Equal(t, http.StatusBadRequest, putProfile(t, s, body), c)
	}
}

func TestThePortfolioListsConfirmedDesignsOnly(t *testing.T) {
	s := testServer(t, activeJeweler("1", testKey("biz")))
	c := catalogue{Designs: []design{
		{ID: "a", Title: "draft title", Status: statusDrafted},
		{ID: "b", Title: "imported title", Status: statusChanged, Description: "Ring in gold", Type: "Ring",
			Confirmed: &confirmation{Title: "Zebra ring", Fields: fields{Metal: "18k gold"}, Photos: []string{"h1"}}},
		{ID: "c", Status: statusGone, Confirmed: &confirmation{Title: "Gone"}},
		{ID: "d", Status: statusConfirmed, Confirmed: &confirmation{Title: "Amber bangle", Photos: []string{"h2"}}},
	}}
	require.NoError(t, s.saveCatalogue("1", c))
	w := serve(s, request("GET", "/v1/jewelers/1/portfolio", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct{ Designs []portfolioDesign }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Designs, 2)
	require.Equal(t, "Amber bangle", got.Designs[0].Title)
	require.Equal(t, "Zebra ring", got.Designs[1].Title)
	require.Equal(t, "18k gold", got.Designs[1].Fields.Metal)
	require.Equal(t, []string{"h1"}, got.Designs[1].Photos)
}

func TestASuspendedShopHasNoPortfolio(t *testing.T) {
	j := activeJeweler("1", testKey("biz"))
	j.Status = "JEWELER_SUSPENDED"
	s := testServer(t, j)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/jewelers/1/portfolio", nil)).Code)
	require.Equal(t, http.StatusNotFound, serve(s, request("GET", "/v1/jewelers/9/portfolio", nil)).Code)
}
