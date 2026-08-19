// Command openapi merges the per-proto Swagger files that
// protoc-gen-openapiv2 emits into the single specification published at
// docs/static/openapi.json.
//
// The plugin writes one document per .proto file, which is not something a
// reader or a code generator can consume — the published spec has to be one
// file describing the whole REST surface. Ignite used to do this merge
// invisibly, which is why the committed spec silently stopped being updated
// when the project stopped using Ignite: it was two whole modules out of date,
// describing an API that no longer matched the chain.
//
// Usage:
//
//	openapi --in <generated-dir> --out docs/static/openapi.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type document struct {
	Swagger     string                     `json:"swagger,omitempty"`
	Info        map[string]any             `json:"info,omitempty"`
	Consumes    []string                   `json:"consumes,omitempty"`
	Produces    []string                   `json:"produces,omitempty"`
	Paths       map[string]json.RawMessage `json:"paths,omitempty"`
	Definitions map[string]json.RawMessage `json:"definitions,omitempty"`
	Tags        []json.RawMessage          `json:"tags,omitempty"`
}

// merged is written with an explicit field order so the output is stable, which
// is what makes the drift check meaningful.
type merged struct {
	ID          string                     `json:"id"`
	Consumes    []string                   `json:"consumes"`
	Produces    []string                   `json:"produces"`
	Swagger     string                     `json:"swagger"`
	Info        map[string]any             `json:"info"`
	Paths       map[string]json.RawMessage `json:"paths"`
	Definitions map[string]json.RawMessage `json:"definitions"`
}

func main() {
	in := flag.String("in", "", "directory of generated .swagger.json files")
	out := flag.String("out", "docs/static/openapi.json", "path of the merged specification")
	title := flag.String("title", "HTTP API Console", "title for the merged document")
	chain := flag.String("chain", "yamale/blockchain", "chain name, used in the description")
	flag.Parse()

	if *in == "" {
		fatal(fmt.Errorf("--in is required"))
	}

	result := merged{
		ID:       "openapi",
		Consumes: []string{"application/json"},
		Produces: []string{"application/json"},
		Swagger:  "2.0",
		Info: map[string]any{
			"title":       *title,
			"description": "Chain " + *chain + " REST API",
			"contact":     map[string]any{"name": *chain},
			"version":     "version not set",
		},
		Paths:       map[string]json.RawMessage{},
		Definitions: map[string]json.RawMessage{},
	}

	files, err := collect(*in)
	if err != nil {
		fatal(err)
	}
	if len(files) == 0 {
		fatal(fmt.Errorf("no .swagger.json files under %s — run the generation step first", *in))
	}

	var skipped int
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			fatal(err)
		}

		var doc document
		if err := json.Unmarshal(raw, &doc); err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}

		// A proto with no HTTP annotations still produces a document, with no
		// paths in it. Its definitions are only reachable through a service
		// that does have them, so carrying them would bloat the spec with types
		// nothing references.
		if len(doc.Paths) == 0 {
			skipped++
			continue
		}

		for route, body := range doc.Paths {
			// Two protos claiming one route would mean two services answering
			// the same URL, which is a real conflict rather than something to
			// resolve by ordering.
			if existing, clash := result.Paths[route]; clash && string(existing) != string(body) {
				fatal(fmt.Errorf("route %s is defined differently in more than one proto", route))
			}
			result.Paths[route] = body
		}
		for name, body := range doc.Definitions {
			result.Definitions[name] = body
		}
	}

	if err := write(*out, result); err != nil {
		fatal(err)
	}

	fmt.Printf("wrote %s (%d routes, %d definitions, from %d of %d documents)\n",
		*out, len(result.Paths), len(result.Definitions), len(files)-skipped, len(files))
}

// collect returns every generated Swagger file, sorted, so a rerun with no
// changes produces byte-identical output.
func collect(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".swagger.json") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func write(path string, doc merged) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Marshalled through the standard encoder, which sorts map keys, so the
	// route and definition ordering is deterministic rather than following Go's
	// randomised map iteration.
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "openapi:", err)
	os.Exit(1)
}
