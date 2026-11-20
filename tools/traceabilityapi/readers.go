package main

// Reading a jeweler's website: Shopify's public feed first, then schema.org
// Product markup on the pages a sitemap lists. Everything goes through a
// pageFetcher, so robots.txt and the fetch rules apply to every request.

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const maxDesigns = 500

var errUnreadable = errors.New("this site doesn't publish its products in a form we can read")

type sourceProduct struct {
	SKU, Handle, Path, Title, DescriptionHTML, Type, URL, Price, Currency string
	Tags, Images                                                          []string
}

type shopifyPage struct {
	Products []struct {
		Title       string          `json:"title"`
		Handle      string          `json:"handle"`
		BodyHTML    string          `json:"body_html"`
		ProductType string          `json:"product_type"`
		Tags        json.RawMessage `json:"tags"`
		Variants    []struct {
			SKU   string `json:"sku"`
			Price string `json:"price"`
		} `json:"variants"`
		Images []struct {
			Src string `json:"src"`
		} `json:"images"`
	} `json:"products"`
}

// shopifyImage asks Shopify's CDN for at most 2400 px, the size the app
// re-encodes to anyway, so an original does not trip the 5 MB limit.
func shopifyImage(src string) string {
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	if strings.Contains(src, "?") {
		return src + "&width=2400"
	}
	return src + "?width=2400"
}

func shopifyTags(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var joined string
	if json.Unmarshal(raw, &joined) == nil && joined != "" {
		for _, t := range strings.Split(joined, ",") {
			list = append(list, strings.TrimSpace(t))
		}
	}
	return list
}

// readShopify reads <origin>/products.json page by page. shop is false when
// the site has no such feed, so the caller tries schema.org next.
func readShopify(ctx context.Context, f pageFetcher, origin string, max int) ([]sourceProduct, bool, error) {
	var out []sourceProduct
	for page := 1; len(out) < max; page++ {
		body, _, err := f.get(ctx, fmt.Sprintf("%s/products.json?limit=250&page=%d", origin, page), acceptPages)
		var se *statusError
		if page == 1 && errors.As(err, &se) && se.code >= 400 && se.code < 500 {
			return nil, false, nil
		}
		if err != nil {
			return nil, page > 1, err
		}
		var p shopifyPage
		if json.Unmarshal(body, &p) != nil || p.Products == nil {
			if page == 1 {
				return nil, false, nil
			}
			return nil, true, errUnreadable
		}
		for _, sp := range p.Products {
			prod := sourceProduct{
				Handle: sp.Handle, Path: "/products/" + sp.Handle, URL: origin + "/products/" + sp.Handle,
				Title: sp.Title, DescriptionHTML: sp.BodyHTML, Type: sp.ProductType, Tags: shopifyTags(sp.Tags),
			}
			if len(sp.Variants) > 0 {
				prod.SKU, prod.Price = strings.TrimSpace(sp.Variants[0].SKU), sp.Variants[0].Price
			}
			for _, im := range sp.Images {
				if len(prod.Images) < 12 && im.Src != "" {
					prod.Images = append(prod.Images, shopifyImage(im.Src))
				}
			}
			out = append(out, prod)
		}
		if len(p.Products) < 250 {
			break
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	currency := ""
	if body, _, err := f.get(ctx, origin+"/meta.json", acceptPages); err == nil {
		var meta struct {
			Currency string `json:"currency"`
		}
		if json.Unmarshal(body, &meta) == nil {
			currency = strings.ToUpper(meta.Currency)
		}
	} else if !absent(err) {
		return nil, true, err
	}
	for i := range out {
		out[i].Currency = currency
	}
	return out, true, nil
}

type sitemapXML struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

func sameHost(raw, origin string) bool {
	u, err := url.Parse(raw)
	o, err2 := url.Parse(origin)
	return err == nil && err2 == nil && strings.EqualFold(u.Host, o.Host) && u.Scheme == o.Scheme
}

// absent: the site answered, and the answer was "not here" (any 4xx other
// than a rate limit, which the fetcher reports as errRateLimited). Every other
// failure is a read that did not finish, and must stop the read: a shorter
// list would be taken for the whole catalogue.
func absent(err error) bool {
	var se *statusError
	return errors.As(err, &se) && se.code >= 400 && se.code < 500
}

// productPages lists the product pages of a site from its sitemap.
func productPages(ctx context.Context, f pageFetcher, origin string, max int) ([]string, error) {
	body, _, err := f.get(ctx, origin+"/sitemap.xml", acceptPages)
	if absent(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sm sitemapXML
	if xml.Unmarshal(body, &sm) != nil {
		return nil, nil
	}
	var candidates []string
	if len(sm.Sitemaps) == 0 {
		for _, u := range sm.URLs {
			if strings.Contains(u.Loc, "/product") {
				candidates = append(candidates, u.Loc)
			}
		}
	} else {
		var named, others []string
		for _, s := range sm.Sitemaps {
			loc := strings.TrimSpace(s.Loc)
			if !sameHost(loc, origin) {
				continue
			}
			if strings.Contains(strings.ToLower(loc), "product") {
				named = append(named, loc)
			} else {
				others = append(others, loc)
			}
		}
		children, filter := named, false
		if len(children) == 0 {
			children, filter = others, true
		}
		if len(children) > 10 {
			children = children[:10]
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			b, _, err := f.get(ctx, child, acceptPages)
			if absent(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var c sitemapXML
			if xml.Unmarshal(b, &c) != nil {
				continue
			}
			for _, u := range c.URLs {
				if !filter || strings.Contains(u.Loc, "/product") {
					candidates = append(candidates, u.Loc)
				}
			}
		}
	}
	seen := map[string]bool{}
	var pages []string
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if !sameHost(c, origin) || seen[c] || len(pages) >= max {
			continue
		}
		seen[c] = true
		pages = append(pages, c)
	}
	return pages, nil
}

// readSchemaOrg reads schema.org Product markup from the product pages a
// site's sitemap lists. A page without markup is skipped.
func readSchemaOrg(ctx context.Context, f pageFetcher, origin string, max int) ([]sourceProduct, error) {
	pages, err := productPages(ctx, f, origin, max)
	if err != nil {
		return nil, err
	}
	var out []sourceProduct
	for _, u := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, _, err := f.get(ctx, u, acceptPages)
		if absent(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if p, ok := productFromPage(body, u); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// ldJSONBlocks returns the text of each <script type="application/ld+json">.
func ldJSONBlocks(page []byte) [][]byte {
	z := html.NewTokenizer(bytes.NewReader(page))
	var out [][]byte
	inLD := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken:
			name, more := z.TagName()
			inLD = false
			if string(name) == "script" {
				for more {
					var k, v []byte
					k, v, more = z.TagAttr()
					if string(k) == "type" && strings.EqualFold(strings.TrimSpace(string(v)), "application/ld+json") {
						inLD = true
					}
				}
			}
		case html.TextToken:
			if inLD {
				out = append(out, append([]byte(nil), z.Text()...))
				inLD = false
			}
		case html.EndTagToken:
			inLD = false
		}
	}
}

func isType(t any, want string) bool {
	switch v := t.(type) {
	case string:
		return v == want
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func findProduct(v any) map[string]any {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if p := findProduct(e); p != nil {
				return p
			}
		}
	case map[string]any:
		if isType(x["@type"], "Product") {
			return x
		}
		if g, ok := x["@graph"]; ok {
			return findProduct(g)
		}
	}
	return nil
}

func ldString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func ldImages(v any) []string {
	var out []string
	add := func(e any) {
		switch x := e.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			if u := ldString(x["url"]); u != "" {
				out = append(out, u)
			}
		}
	}
	if list, ok := v.([]any); ok {
		for _, e := range list {
			add(e)
		}
	} else {
		add(v)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func productFromPage(page []byte, pageURL string) (sourceProduct, bool) {
	for _, block := range ldJSONBlocks(page) {
		var v any
		if json.Unmarshal(block, &v) != nil {
			continue
		}
		obj := findProduct(v)
		if obj == nil {
			continue
		}
		p := sourceProduct{
			Title: ldString(obj["name"]), DescriptionHTML: ldString(obj["description"]), SKU: ldString(obj["sku"]),
			URL: pageURL, Images: ldImages(obj["image"]),
		}
		if u, err := url.Parse(pageURL); err == nil {
			p.Path = u.Path
		}
		offer := obj["offers"]
		if list, ok := offer.([]any); ok && len(list) > 0 {
			offer = list[0]
		}
		if o, ok := offer.(map[string]any); ok {
			p.Price = ldString(o["price"])
			if p.Price == "" {
				p.Price = ldString(o["lowPrice"])
			}
			p.Currency = strings.ToUpper(ldString(o["priceCurrency"]))
		}
		return p, p.Title != ""
	}
	return sourceProduct{}, false
}

var blockTags = map[string]bool{"p": true, "br": true, "div": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}

// plainText is the readable text of an HTML fragment: tags dropped, entities
// decoded, one line per block, script and style left out.
func plainText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			var lines []string
			for _, l := range strings.Split(b.String(), "\n") {
				if l = strings.Join(strings.Fields(l), " "); l != "" {
					lines = append(lines, l)
				}
			}
			return strings.Join(lines, "\n")
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			if n := string(name); (n == "script" || n == "style") && tt == html.StartTagToken {
				skip++
			} else if blockTags[n] {
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if n := string(name); n == "script" || n == "style" {
				if skip > 0 {
					skip--
				}
			} else if blockTags[n] {
				b.WriteByte('\n')
			}
		}
	}
}
