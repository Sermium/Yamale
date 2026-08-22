package types

import sdkerrors "cosmossdk.io/errors"

// Errors are numbered from 2; code 1 is reserved for the SDK's internal error.
var (
	ErrAlreadyRegistered = sdkerrors.Register(ModuleName, 2,
		"this account already holds an identifier")
	ErrNotRegistered = sdkerrors.Register(ModuleName, 3,
		"this account holds no identifier")
	ErrNotFound = sdkerrors.Register(ModuleName, 4,
		"no account holds that identifier")
	ErrMalformedID = sdkerrors.Register(ModuleName, 5,
		"that is not a well-formed identifier")
	ErrInvalidParams = sdkerrors.Register(ModuleName, 6,
		"invalid parameters")
	// Exhausted cannot happen at 32^8 with a nonce that keeps incrementing, and
	// the loop that would spin forever if it did must still have a way out.
	ErrExhausted = sdkerrors.Register(ModuleName, 7,
		"could not derive an unused identifier")
	ErrInvalidSigner = sdkerrors.Register(ModuleName, 8,
		"invalid authority for this message")
	// The refusal the perimeter is built on. Not a permissive default and not a
	// placeholder country: an account whose jurisdiction nobody has recorded
	// gets no identifier, so there is no account on the rail whose perimeter is
	// unknown and no state for anyone to reason about or exploit.
	ErrNoJurisdiction = sdkerrors.Register(ModuleName, 9,
		"this account has no recorded jurisdiction")
	ErrInvalidCountry = sdkerrors.Register(ModuleName, 10,
		"that is not an assigned ISO 3166-1 alpha-2 country code")
	// Raised when a participant that did not onboard the account, or that is no
	// longer approved, tries to record where it is. The country is evidence
	// gathered by whoever performed the KYC; anyone else asserting it is a
	// guess wearing the same clothes.
	ErrNotTheRecorder = sdkerrors.Register(ModuleName, 11,
		"only the account's approved participant or a foundation administrator may record its jurisdiction")
	// A participant may record a country once. Changing one already recorded is
	// an authority's act, because a participant able to rewrite it could move a
	// customer out from under the authority investigating them.
	ErrJurisdictionSet = sdkerrors.Register(ModuleName, 12,
		"this account's jurisdiction is already recorded; only a foundation administrator may correct it")
	// Raised before a key that is not an X25519 public key can be published.
	// The check has to be here because nothing downstream can make it: an
	// envelope sealed to a malformed key is well-formed, stores cleanly and
	// opens for nobody.
	ErrInvalidViewingKey = sdkerrors.Register(ModuleName, 13,
		"that is not a 32-byte X25519 public key")
	// Distinguishes a version that was never published from one that was. A
	// reader told only "not found" cannot tell a key it should ask an older
	// registry for from one that never existed.
	ErrViewingKeyNotFound = sdkerrors.Register(ModuleName, 14,
		"this account has published no viewing key at that version")
	ErrNoRegulator = sdkerrors.Register(ModuleName, 15,
		"no regulator is appointed for that country")
	ErrInvalidAuditorGrant = sdkerrors.Register(ModuleName, 16,
		"an auditor grant must expire at a future height, and only so many may be live at once")
	// Raised when a grant names no role, or a number that is not one. The zero
	// value is reserved as unspecified, so this is what a message with the role
	// field left unset gets — never a default.
	ErrInvalidRole = sdkerrors.Register(ModuleName, 17,
		"that is not a role that can be held, and an unset role is never a default")
	// Raised when a grant's *where* is neither an assigned country code nor the
	// chain-wide marker. The foundation's reserved code lands here too: it marks
	// the absence of a perimeter, and a grant over nowhere that reads like a
	// grant over everywhere is the one confusion this module cannot allow.
	ErrInvalidScope = sdkerrors.Register(ModuleName, 18,
		"a role's jurisdiction must be an assigned country code or the chain-wide marker")
	// The refusal the whole of piece three exists to produce: the actor holds no
	// grant of that role reaching that perimeter. Raised for an actor with no
	// grants at all as readily as for one whose grant names another country —
	// there is no state in which the absence of a grant permits an action.
	ErrOutOfScope = sdkerrors.Register(ModuleName, 19,
		"this account holds no grant of that role covering that jurisdiction")
	// Raised when a role would be granted to a plain key. A role is only worth
	// the office that holds it, and an office that is one key is one bribe.
	ErrHolderNotGroup = sdkerrors.Register(ModuleName, 20,
		"a role holder must be an x/group account, so that acting on it is M-of-N")
	// Distinguishes revoking a grant that was never made from revoking one that
	// was. An operator told only "done" cannot tell that they revoked the
	// jurisdiction they meant from that they revoked nothing at all.
	ErrGrantNotFound = sdkerrors.Register(ModuleName, 21,
		"no such grant: this account does not hold that role in that jurisdiction")
	// Raised when the perimeter check cannot be made because the module that
	// would answer it was not wired in. It fails closed on purpose: a check that
	// is skipped when its dependency is missing is a check that a wiring mistake
	// silently removes, which is worse than one that is absent on purpose.
	ErrNoScopeKeeper = sdkerrors.Register(ModuleName, 22,
		"the jurisdictional perimeter cannot be checked because the registry is not wired in")
)
