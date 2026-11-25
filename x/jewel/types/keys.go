package types

import (
	"fmt"
	"strconv"
	"strings"

	"cosmossdk.io/collections"
)

const (
	// ModuleName defines the module name.
	ModuleName = "jewel"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency.
	GovModuleName = "gov"

	// NFTSendTypeURL is x/nft's own send. It skips the offer and acceptance,
	// the stolen flag and the owner count, so x/circuit must disable it, and
	// this module refuses to start registering while it is open.
	NFTSendTypeURL = "/cosmos.nft.v1beta1.MsgSend"

	// MaxSearchResults bounds JewelsByInscription. Two jewelers may share a
	// code; two hundred sharing one is not a search, it is a scan.
	MaxSearchResults = 50

	// MaxAmendmentNote and MaxAmendments bound a jewel's corrections. The
	// amendments live inside the jewel's record, which every offer, report
	// and change of owner reads and rewrites, so without a bound a jeweler's
	// staff could make a sold jewel too expensive for its owner to move.
	MaxAmendmentNote = 500
	MaxAmendments    = 50

	// The kinds of change of owner EventJewelTransferred reports.
	TransferAccept   = "accept"   // an offer to an account, accepted
	TransferClaim    = "claim"    // an offer to a claim key, claimed
	TransferCard     = "card"     // a claim filed with the ownership card, approved by the shop
	TransferReassign = "reassign" // the office's reassignment, completed
	TransferSale     = "sale"     // bought on the marketplace
)

// Prefixes. "jewel" is a byte-prefix of "jeweler", and collections refuses
// overlapping prefixes, so the records are "maker" and "piece".
var (
	ParamsKey       = collections.NewPrefix("p_jewel")
	JewelerKey      = collections.NewPrefix("maker")
	JewelKey        = collections.NewPrefix("piece")
	OfferKey        = collections.NewPrefix("offer")
	BenefitKey      = collections.NewPrefix("benefit")
	ReassignmentKey = collections.NewPrefix("reassign")
	UsedNonceKey    = collections.NewPrefix("used_nonce")
	CardKey         = collections.NewPrefix("card")
	ClaimKey        = collections.NewPrefix("claim")
	ListingKey      = collections.NewPrefix("listing")

	// The indexes, each one there because the alternative is a scan.
	ByBusinessKey      = collections.NewPrefix("idx_business")
	ByStaffKey         = collections.NewPrefix("idx_staff")
	ByInscriptionKey   = collections.NewPrefix("idx_inscription")
	SearchKey          = collections.NewPrefix("idx_search")
	BenefitsByJewelKey = collections.NewPrefix("idx_benefits_by_piece")
	// (jeweler id, jewel id): the jeweler's pending claims, for the Atelier.
	// It changes only together with Claim.
	ClaimsByJewelerKey = collections.NewPrefix("idx_claims_by_maker")

	NextJewelerIDKey = collections.NewPrefix("next_maker_id")
	NextJewelIDKey   = collections.NewPrefix("next_piece_id")
	NextBenefitIDKey = collections.NewPrefix("next_benefit_id")
)

// ClassID is the x/nft class holding one jeweler's jewels.
func ClassID(jewelerID uint64) string { return fmt.Sprintf("jewel-%d", jewelerID) }

// TokenID is the x/nft id of one jewel.
func TokenID(jewelID uint64) string { return fmt.Sprintf("jewel-%d", jewelID) }

// JewelIDOf reads the jewel id back out of an x/nft token id.
func JewelIDOf(tokenID string) (uint64, error) {
	rest, ok := strings.CutPrefix(tokenID, "jewel-")
	if !ok {
		return 0, fmt.Errorf("%q is not a jewel token id", tokenID)
	}
	return strconv.ParseUint(rest, 10, 64)
}
