package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	ModuleName = "herd"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency.
	GovModuleName = "gov"
)

var (
	// ParamsKey is the prefix to retrieve all Params.
	ParamsKey = collections.NewPrefix("p_herd")

	// HerdKey stores herds by id, AnimalKey animals, and so on.
	HerdKey        = collections.NewPrefix("herd")
	AnimalKey      = collections.NewPrefix("animal")
	HealthEventKey = collections.NewPrefix("health")
	TransferKey    = collections.NewPrefix("transfer")
	// AuthorityKey stores registry offices by address.
	AuthorityKey = collections.NewPrefix("authority")
	// AgentKey holds the accredited people. Separate from AuthorityKey because
	// an agent is a person with a plain key and an office is an x/group
	// account: same registry, different kind of thing entirely.
	AgentKey = collections.NewPrefix("agent")

	// The indexes. Every one exists because the alternative is a scan, and a
	// check whose cost grows with the registry is a check somebody eventually
	// turns off — which on a national herd means the second year.
	//
	// All of them carry an "idx_" prefix, and that is not decoration. A
	// collections prefix must not be a byte-prefix of another: "transfer" sits
	// inside "transfers_by_animal", so the index's keys would land inside the
	// range the transfer map walks, and iterating transfers would return index
	// entries decoded as records. The schema builder refuses it outright —
	// "schema has overlapping prefixes" — which is the right place to find out,
	// and where this was found.
	//
	// ByHerdRef is how an officer holding a paper file finds the record.
	ByHerdRefKey = collections.NewPrefix("idx_by_herd_ref")
	// ByNationalID resolves the identifier printed on a certificate. This is
	// the uniqueness constraint on animal records.
	ByNationalIDKey = collections.NewPrefix("idx_by_national_id")
	// ByTag resolves a transponder code read in the field, which is the query
	// an agent with a reader actually makes.
	ByTagKey = collections.NewPrefix("idx_by_tag")

	// AnimalsByHerdKey and HealthByHerdKey are (herd id, record id) pairs, so
	// listing one herd's animals is a prefix walk rather than a scan of every
	// animal in the country.
	AnimalsByHerdKey = collections.NewPrefix("idx_animals_by_herd")
	HealthByHerdKey  = collections.NewPrefix("idx_health_by_herd")
	// TransfersByAnimalKey is (animal id, transfer id): one animal's
	// provenance, including the transfers that were objected to.
	TransfersByAnimalKey = collections.NewPrefix("idx_transfers_by_animal")

	// The counters. Exported in genesis rather than derived from the highest
	// id present, so an imported registry keeps its numbering — and a retired
	// animal at the end of the range does not silently reset it.
	NextHerdIDKey        = collections.NewPrefix("next_herd_id")
	NextAnimalIDKey      = collections.NewPrefix("next_animal_id")
	NextHealthEventIDKey = collections.NewPrefix("next_health_id")
	NextTransferIDKey    = collections.NewPrefix("next_transfer_id")
)

// MaxPedigreeDepth bounds a pedigree walk regardless of what the caller asks
// for.
//
// A pedigree is a graph populated by whoever declared it, and an unbounded walk
// over somebody else's graph is a denial of service with a polite name. Ten
// generations is far beyond any pedigree a live sector maintains — the Ladoum
// certificates carry two — and it is still a fixed cost.
const MaxPedigreeDepth = 10

// BirthTypeTwin and its neighbours are the values the birth-interval check
// treats as one birth rather than two.
//
// Free text otherwise, because breeders write what they write and a registry
// that refused an unrecognised word would refuse the record rather than the
// error. These are matched case-insensitively.
const (
	BirthTypeSingle  = "single"
	BirthTypeTwin    = "twin"
	BirthTypeTriplet = "triplet"
)
