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
	if !strings.Contains(string(raw), "/blockchain/emission/v1") {
		t.Skip("the generated specification no longer describes x/emission")
	}

	filtered := string(specForProfile([]string{"paymsg", "treasury"}))
	if strings.Contains(filtered, "/blockchain/emission/v1/params") {
		t.Error("an emission path survived filtering")
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
