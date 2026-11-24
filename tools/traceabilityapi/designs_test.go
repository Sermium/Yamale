package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func product(sku, handle, title string) sourceProduct {
	return sourceProduct{SKU: sku, Handle: handle, Path: "/products/" + handle, URL: "https://shop.example/products/" + handle,
		Title: title, DescriptionHTML: "<p>18kt gold</p>", Price: "100.00", Currency: "USD", Images: []string{"https://cdn.example/a.jpg"}}
}

func TestAKeyIsTheSKUOrTheHandle(t *testing.T) {
	ds := designsFrom([]sourceProduct{product("JOA-071", "parrot", "Parrot"), product("", "emerald-ring-1", "Ring"), product("JOA-071", "copy", "Copy")})
	require.Equal(t, "JOA-071", ds[0].Key)
	require.Equal(t, "handle:emerald-ring-1", ds[1].Key)
	require.Equal(t, "handle:copy", ds[2].Key, "a repeated SKU falls back to the handle")
	require.Equal(t, designID("JOA-071"), ds[0].ID)
	require.Len(t, ds[0].ID, 16)
	require.Equal(t, "18kt gold", ds[0].Description)
	require.Equal(t, &money{Amount: "100.00", Currency: "USD"}, ds[0].ListPrice)
	require.NotNil(t, ds[0].Fields.Stones, "stones are [] in JSON, never null")
}

func TestAPriceNeedsANumberAndACurrency(t *testing.T) {
	p := product("A1", "a", "A")
	p.Currency = ""
	require.Nil(t, designsFrom([]sourceProduct{p})[0].ListPrice)
	p = product("A1", "a", "A")
	p.Price = "on request"
	require.Nil(t, designsFrom([]sourceProduct{p})[0].ListPrice)
}

func TestLongTextIsClipped(t *testing.T) {
	p := product("A1", "a", strings.Repeat("t", 200))
	p.DescriptionHTML = strings.Repeat("d", 6000)
	d := designsFrom([]sourceProduct{p})[0]
	require.Len(t, []rune(d.Title), 120)
	require.Len(t, []rune(d.Description), 5000)
}

func TestTheSourceHashFollowsWhatTheSiteSays(t *testing.T) {
	a := designsFrom([]sourceProduct{product("A1", "a", "A")})[0]
	b := product("A1", "a", "A")
	b.Price = "120.00"
	require.NotEqual(t, a.SourceHash, designsFrom([]sourceProduct{b})[0].SourceHash)
	require.Equal(t, a.SourceHash, designsFrom([]sourceProduct{product("A1", "a", "A")})[0].SourceHash)
}

func TestRefreshMovesDesignsBetweenStatuses(t *testing.T) {
	base := designsFrom([]sourceProduct{product("KEEP", "k", "Keep"), product("EDIT", "e", "Edit"), product("DROP", "d", "Drop"), product("RAW", "r", "Raw")})
	old := mergeDesigns(nil, base)
	for _, d := range old {
		require.Equal(t, statusImported, d.Status)
	}
	confirm := func(d *design) {
		d.Status = statusConfirmed
		d.Fields = fields{Metal: "Gold", WeightGrams: "4", Stones: []stone{}}
		d.Confirmed = &confirmation{Title: d.Title, Fields: d.Fields, Photos: []string{strings.Repeat("a", 64)}, SourceHash: d.SourceHash}
	}
	confirm(&old[0])
	confirm(&old[1])
	confirm(&old[2])

	edited := product("EDIT", "e", "Edit")
	edited.Price = "999.00"
	fresh := designsFrom([]sourceProduct{product("KEEP", "k", "Keep"), edited, product("RAW", "r", "Raw"), product("NEW", "n", "New")})
	got := map[string]design{}
	for _, d := range mergeDesigns(old, fresh) {
		got[d.Key] = d
	}
	require.Equal(t, statusConfirmed, got["KEEP"].Status)
	require.Equal(t, statusChanged, got["EDIT"].Status)
	require.Equal(t, "Gold", got["EDIT"].Fields.Metal, "a changed design keeps its last confirmed fields")
	require.NotNil(t, got["EDIT"].Confirmed)
	require.Equal(t, statusGone, got["DROP"].Status)
	require.Equal(t, statusConfirmed, got["DROP"].GoneFrom)
	require.Equal(t, statusImported, got["RAW"].Status)
	require.Equal(t, statusImported, got["NEW"].Status)

	back := mergeDesigns(mergeDesigns(old, fresh), designsFrom([]sourceProduct{product("DROP", "d", "Drop")}))
	for _, d := range back {
		if d.Key == "DROP" {
			require.Equal(t, statusConfirmed, d.Status, "a design that comes back unchanged is what it was")
		}
	}
}

func TestOnlyUndraftedTextIsDrafted(t *testing.T) {
	d := designsFrom([]sourceProduct{product("A1", "a", "A")})[0]
	d.Status = statusImported
	require.True(t, needsDraft(d))
	d.DraftFor = d.SourceHash
	require.False(t, needsDraft(d))
	d.Status, d.DraftFor = statusChanged, "older"
	require.True(t, needsDraft(d))
	d.Status = statusConfirmed
	require.False(t, needsDraft(d))
}

func TestFieldRulesMirrorTheApp(t *testing.T) {
	ok := fields{Metal: "18kt Yellow Gold", WeightGrams: "", Stones: []stone{{Kind: "Emerald", Carat: "32", Count: 12}}}
	require.Equal(t, "", fieldsProblem("Parrot", ok, true), "the weight may be empty: rows can carry it")
	require.Equal(t, "", fieldsProblem("", fields{Stones: []stone{{Kind: "Diamond", Count: 0}}}, false), "a draft may be incomplete")
	require.NotEqual(t, "", fieldsProblem("", ok, true))
	// The website often does not say: a confirmed design may leave carats and count not stated.
	require.Equal(t, "", fieldsProblem("P", fields{Metal: "Gold", Stones: []stone{{Kind: "Diamond", Carat: "", Count: 422}}}, true))
	require.Equal(t, "", fieldsProblem("P", fields{Metal: "Gold", Stones: []stone{{Kind: "Emerald", Carat: "5.2", Count: 0}}}, true))
	require.Equal(t, "", fieldsProblem("P", fields{Metal: "Gold", Stones: []stone{{Kind: "Tsavorite", Carat: "", Count: 0}}}, true))
	require.NotEqual(t, "", fieldsProblem("P", fields{Metal: "Gold", Stones: []stone{{Kind: " ", Carat: "1", Count: 1}}}, true), "a stone is named")
	require.NotEqual(t, "", fieldsProblem("P", fields{Metal: "Gold", WeightGrams: "4,2"}, false))
	require.NotEqual(t, "", fieldsProblem(strings.Repeat("x", 121), ok, false))
	require.NotEqual(t, "", fieldsProblem("P", fields{Metal: "Gold", Stones: []stone{{Kind: "Ruby", Carat: "1", Count: 10000}}}, false))
}

func TestAFallbackProductWithoutAHandleKeysByItsProductPath(t *testing.T) {
	p := sourceProduct{Path: "/products/emerald-ring-1", Title: "Ring"}
	require.Equal(t, "handle:emerald-ring-1", designKey(p))
	require.Equal(t, "handle:emerald-ring-1", designKey(sourceProduct{Path: "/products/emerald-ring-1/"}))
	require.Equal(t, "url:/pages/about", designKey(sourceProduct{Path: "/pages/about"}))
}
