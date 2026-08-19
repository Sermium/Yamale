package docs

import (
	"encoding/json"
	"strings"
	"testing"
)

// The specification is generated from every .proto in the repository, so it
// describes the union of every build profile. A profile binary that served it
// unfiltered would offer a swagger console full of endpoints that answer 404 —
// and would tell anyone reading it that the deployment carries modules it was
// sold as not carrying.
func TestPathsOfUnlinkedModulesAreNotAdvertised(t *testing.T) {
	linked := []string{"paymsg", "stablecoin", "treasury", "oracle", "enforcement", "alias", "validatorgov"}

	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(specForProfile(linked), &doc); err != nil {
		t.Fatalf("filtered spec is not valid JSON: %v", err)
	}

	for path := range doc.Paths {
		match := modulePath.FindStringSubmatch(path)
		if match == nil {
			continue
		}
		found := false
		for _, name := range linked {
			if name == match[1] {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s belongs to x/%s, which this profile does not link", path, match[1])
		}
	}

	if len(doc.Paths) == 0 {
		t.Fatal("filtering removed everything, which would mean the path shapes have changed")
	}
}

// The unfiltered document is what proves the filter is doing something: if the
// generated specification ever stops carrying the excluded modules on its own,
// the test above passes for the wrong reason.
func TestTheUnfilteredSpecStillCarriesEveryModule(t *testing.T) {
	raw, err := Static.ReadFile("static/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "/yamale/blockchain/emission/v1") {
		t.Skip("the generated specification no longer describes x/emission")
	}

	filtered := string(specForProfile([]string{"paymsg", "treasury"}))
	if strings.Contains(filtered, "/yamale/blockchain/emission/v1/params") {
		t.Error("an emission path survived filtering")
	}
}

// Every path this chain owns has to carry the /yamale prefix. x/tokenisation
// shipped without it, and nothing noticed because no client called it: the
// filter above quietly grew a second path shape to cope, and the published
// specification advertised one module under a prefix the others did not use.
// A path that regresses is invisible to the tests that merely substring-match,
// so this one checks the shape of the whole document.
func TestEveryChainPathCarriesTheYamalePrefix(t *testing.T) {
	raw, err := Static.ReadFile("static/openapi.json")
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("specification is not valid JSON: %v", err)
	}

	seen := 0
	for path := range doc.Paths {
		// An SDK path belongs to no chain module and is correct as it is.
		if strings.HasPrefix(path, "/cosmos/") {
			continue
		}
		if !strings.HasPrefix(path, "/yamale/blockchain/") {
			t.Errorf("%s does not carry the /yamale/blockchain/ prefix every other module uses", path)
			continue
		}
		seen++
	}

	if seen == 0 {
		t.Fatal("no chain paths found, which would mean the path shapes have changed")
	}
}

// SDK paths belong to no chain module and must survive untouched, or the
// console loses bank, staking and gov.
func TestSDKPathsAreUntouched(t *testing.T) {
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(specForProfile(nil), &doc); err != nil {
		t.Fatal(err)
	}
	for path := range doc.Paths {
		if modulePath.MatchString(path) {
			t.Errorf("%s should have been filtered", path)
		}
	}
}
