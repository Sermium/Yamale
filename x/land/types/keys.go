package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	ModuleName = "land"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency.
	GovModuleName = "gov"
)

var (
	// ParamsKey is the prefix to retrieve all Params.
	ParamsKey = collections.NewPrefix("p_land")

	// ParcelKey stores parcels by id.
	ParcelKey = collections.NewPrefix("parcel")
	// TransferKey stores transfers by id.
	TransferKey = collections.NewPrefix("transfer")
	// AuthorityKey stores registry offices by address.
	AuthorityKey = collections.NewPrefix("authority")

	// FractionalisationAuthorityKey stores permissions to fractionalise, by
	// parcel id. Persisted rather than left as a past message, because
	// x/tokenisation checks it at every issuance and a permission nothing can
	// read is a ceiling nothing can enforce.
	FractionalisationAuthorityKey = collections.NewPrefix("frac_auth")

	// ByGeometryKey indexes survey hash -> parcel id. This index *is* the
	// single-ownership guarantee: registration consults it and refuses a second
	// title over the same ground.
	ByGeometryKey = collections.NewPrefix("by_geometry")
	// ByRefKey indexes cadastral reference -> parcel id.
	ByRefKey = collections.NewPrefix("by_ref")

	// NextParcelIDKey and NextTransferIDKey hold the counters. Exported in
	// genesis rather than derived, so an imported registry keeps its numbering.
	NextParcelIDKey   = collections.NewPrefix("next_parcel_id")
	NextTransferIDKey = collections.NewPrefix("next_transfer_id")
)

// Restriction kinds the keeper itself acts on. The rest are free-form, because
// land law differs by country and a chain that hard-codes one country's rules
// is a chain only that country can use.
const (
	RestrictionNoFractionalisation = "no_fractionalisation"
)
