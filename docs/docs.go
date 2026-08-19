package docs

import (
	"embed"
	"encoding/json"
	httptemplate "html/template"
	"net/http"
	"regexp"
	"slices"

	"github.com/gorilla/mux"
)

const (
	apiFile   = "/static/openapi.json"
	indexFile = "template/index.tpl"
)

//go:embed static
var Static embed.FS

//go:embed template
var template embed.FS

// RegisterOpenAPIService serves the REST specification and the console that
// renders it.
//
// linkedModules is the set of modules this binary actually carries. The
// specification is generated from every .proto in the repository, so it
// describes the union of all build profiles; served unfiltered by a profile
// binary it offers an operator endpoints that answer 404, and — worse for a
// document a buyer reads — advertises modules the deployment was specifically
// sold as not having.
func RegisterOpenAPIService(appName string, rtr *mux.Router, linkedModules []string) {
	spec := specForProfile(linkedModules)

	rtr.HandleFunc(apiFile, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(spec)
	})
	rtr.HandleFunc("/", handler(appName))
}

// modulePath matches this chain's generated REST paths, capturing the module
// name.
//
// This used to accept an optional /yamale prefix, because x/tokenisation was
// generated without one. Now that every module carries it, matching only the
// one shape is what makes the filter fail loudly: a path that regresses to the
// bare /blockchain/ form stops matching, and an unmatched path is kept — so a
// profile binary would advertise a module it does not link, which is the exact
// thing this filter exists to prevent.
var modulePath = regexp.MustCompile(`^/yamale/blockchain/([a-z]+)/`)

// specForProfile drops every path belonging to a chain module this binary does
// not link.
//
// Only paths are filtered. The definitions they referenced are left in place:
// an unused schema in a specification is inert, whereas walking the reference
// graph to prune them is a way to remove one that something else still needs
// and break the console for every module.
//
// Any failure falls back to the unfiltered document. A specification that
// over-advertises is a documentation defect; an endpoint that returns nothing
// because a filter panicked is an outage.
func specForProfile(linkedModules []string) []byte {
	raw, err := Static.ReadFile("static/openapi.json")
	if err != nil {
		return nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}

	var paths map[string]json.RawMessage
	if err := json.Unmarshal(doc["paths"], &paths); err != nil {
		return raw
	}

	for path := range paths {
		match := modulePath.FindStringSubmatch(path)
		if match == nil {
			continue // an SDK path, or one this chain does not own
		}
		if !slices.Contains(linkedModules, match[1]) {
			delete(paths, path)
		}
	}

	filtered, err := json.Marshal(paths)
	if err != nil {
		return raw
	}
	doc["paths"] = filtered

	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// handler returns an http handler that servers OpenAPI console for an OpenAPI spec at specURL.
func handler(title string) http.HandlerFunc {
	t, _ := httptemplate.ParseFS(template, indexFile)

	return func(w http.ResponseWriter, req *http.Request) {
		_ = t.Execute(w, struct {
			Title string
			URL   string
		}{
			title,
			apiFile,
		})
	}
}
