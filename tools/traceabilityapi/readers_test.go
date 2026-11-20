package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// mapFetcher serves fixed bodies; anything else is a 404.
type mapFetcher map[string]string

func (m mapFetcher) get(_ context.Context, u, _ string) ([]byte, http.Header, error) {
	if b, ok := m[u]; ok {
		return []byte(b), http.Header{}, nil
	}
	return nil, nil, &statusError{404}
}

func fixture(t *testing.T, name string) string {
	b, err := os.ReadFile("testdata/catalogue/" + name)
	require.NoError(t, err)
	return string(b)
}

const joa = "https://www.jewelofafrica.com"

func TestJewelOfAfricasFeedIsRead(t *testing.T) {
	f := mapFetcher{
		joa + "/products.json?limit=250&page=1": fixture(t, "shopify-products.json"),
		joa + "/meta.json":                      fixture(t, "shopify-meta.json"),
	}
	ps, shop, err := readShopify(context.Background(), f, joa, maxDesigns)
	require.NoError(t, err)
	require.True(t, shop)
	require.Len(t, ps, 96, "measured 2026-11-16: 96 products, one variant each")
	withSKU := 0
	for _, p := range ps {
		require.NotEmpty(t, p.Title)
		require.LessOrEqual(t, len(p.Images), 12)
		require.Equal(t, "USD", p.Currency)
		if p.SKU != "" {
			withSKU++
		}
	}
	require.Equal(t, 70, withSKU)

	var parrot sourceProduct
	for _, p := range ps {
		if p.SKU == "JOA-071" {
			parrot = p
		}
	}
	require.Equal(t, "Emerald Parrot Pendant", parrot.Title)
	require.Equal(t, "emerald-parrot-pendant-071", parrot.Handle)
	require.Equal(t, "17678.00", parrot.Price)
	require.Equal(t, joa+"/products/emerald-parrot-pendant-071", parrot.URL)
	require.Equal(t, "Pendant", parrot.Type)
	require.Len(t, parrot.Images, 4)
	require.Contains(t, parrot.Images[0], "width=2400", "Shopify's CDN is asked for at most 2400 px")
}

func TestAShopifyFeedIsReadPageByPageOnlyWhileFull(t *testing.T) {
	var page struct {
		Products []json.RawMessage `json:"products"`
	}
	require.NoError(t, json.Unmarshal([]byte(fixture(t, "shopify-products.json")), &page))
	one, _ := json.Marshal(map[string]any{"products": page.Products[:3]})
	f := mapFetcher{joa + "/products.json?limit=250&page=1": string(one)}
	ps, shop, err := readShopify(context.Background(), f, joa, maxDesigns)
	require.NoError(t, err)
	require.True(t, shop)
	require.Len(t, ps, 3, "a page under 250 is the last; page 2 is never asked for")
	require.Equal(t, "", ps[0].Currency, "no meta.json: no currency, so no list price later")
}

func TestASiteWithoutAFeedIsNotShopify(t *testing.T) {
	_, shop, err := readShopify(context.Background(), mapFetcher{}, "https://shop.example", maxDesigns)
	require.NoError(t, err)
	require.False(t, shop)
	html := mapFetcher{"https://shop.example/products.json?limit=250&page=1": "<html>not json</html>"}
	_, shop, err = readShopify(context.Background(), html, "https://shop.example", maxDesigns)
	require.NoError(t, err)
	require.False(t, shop)
}

func TestSchemaOrgPagesAreReadFromTheProductSitemap(t *testing.T) {
	f := mapFetcher{
		"https://shop.example/sitemap.xml":              fixture(t, "sitemap-index.xml"),
		"https://shop.example/product-sitemap.xml":      fixture(t, "sitemap-products.xml"),
		"https://shop.example/product/baobab-ring/":     fixture(t, "woo-product.html"),
		"https://shop.example/product/sahel-cuff/":      fixture(t, "wix-product.html"),
		"https://shop.example/product/broken/":          "<html>no markup</html>",
		"https://elsewhere.example/product-sitemap.xml": "<urlset></urlset>",
		"https://evil.example/product/elsewhere/":       fixture(t, "woo-product.html"),
	}
	ps, err := readSchemaOrg(context.Background(), f, "https://shop.example", maxDesigns)
	require.NoError(t, err)
	require.Len(t, ps, 2, "the page without markup is skipped, and another host is never read")

	ring := ps[0]
	require.Equal(t, "Baobab Ring", ring.Title)
	require.Equal(t, "NGR-12", ring.SKU)
	require.Equal(t, "1250.00", ring.Price)
	require.Equal(t, "EUR", ring.Currency)
	require.Equal(t, []string{"https://shop.example/wp-content/uploads/baobab-1.jpg", "https://shop.example/wp-content/uploads/baobab-2.jpg"}, ring.Images)
	require.Equal(t, "https://shop.example/product/baobab-ring/", ring.URL)
	require.Equal(t, "/product/baobab-ring/", ring.Path)

	cuff := ps[1]
	require.Equal(t, "Sahel Cuff", cuff.Title)
	require.Equal(t, "", cuff.SKU)
	require.Equal(t, "480", cuff.Price)
	require.Equal(t, "USD", cuff.Currency, "currency codes are upper-cased")
	require.Equal(t, []string{"https://static.wixstatic.com/media/sahel-cuff.jpg"}, cuff.Images)
}

func TestAProductAtTheTopLevelIsFound(t *testing.T) {
	page := `<script type="application/ld+json">{"@type":"Product","name":"Plain","description":"Gold.","image":{"url":"https://shop.example/a.jpg"},"offers":{"@type":"AggregateOffer","lowPrice":"99.5","priceCurrency":"XOF"}}</script>`
	p, ok := productFromPage([]byte(page), "https://shop.example/product/plain")
	require.True(t, ok)
	require.Equal(t, "Plain", p.Title)
	require.Equal(t, "99.5", p.Price)
	require.Equal(t, "XOF", p.Currency)
	require.Equal(t, []string{"https://shop.example/a.jpg"}, p.Images)
}

func TestRateLimitsAndRobotsStopTheReaders(t *testing.T) {
	for _, stop := range []error{errRateLimited, errRobots} {
		f := failingFetcher{err: stop}
		_, _, err := readShopify(context.Background(), f, "https://shop.example", maxDesigns)
		require.ErrorIs(t, err, stop)
		_, err = readSchemaOrg(context.Background(), f, "https://shop.example", maxDesigns)
		require.ErrorIs(t, err, stop)
	}
}

type failingFetcher struct{ err error }

func (f failingFetcher) get(context.Context, string, string) ([]byte, http.Header, error) {
	return nil, nil, f.err
}

func TestHTMLBecomesReadableText(t *testing.T) {
	got := plainText(`<p>Twelve <b>significant</b> emeralds&nbsp;totaling 32 carats.</p><ul><li>18kt Yellow Gold</li><li>422 diamonds</li></ul><script>alert(1)</script><style>p{}</style>`)
	require.Equal(t, "Twelve significant emeralds totaling 32 carats.\n18kt Yellow Gold\n422 diamonds", got)
}
