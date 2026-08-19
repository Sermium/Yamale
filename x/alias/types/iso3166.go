package types

// ISO 3166-1 alpha-2, the assigned list.
//
// A shape check — two letters — is not enough. NX, QK and ZX are all two
// letters and none of them is a country, so a mistyped code would be recorded
// as a perimeter that no authority holds and no authority can act on, and the
// account would then be issued an identifier advertising it. The point of the
// prefix is that it cannot lie; a code that names nowhere is one of the ways it
// could.
//
// Kept as a literal table rather than derived from a library so that the set of
// perimeters this chain will accept is a reviewable diff. ISO changes it every
// few years; adding a code is a one-line change here.

// FoundationCountry marks an identifier held by an account with no national
// perimeter — the foundation administrators, and nobody else.
//
// ZZ is in ISO 3166-1's permanently user-assigned range, which the standard
// guarantees will never be given to a country. That guarantee is the whole
// reason to use it: an operator reading ZZ can be certain it is not a country
// they have not heard of. It is refused as an input to MsgSetJurisdiction, so
// it can never be recorded as an ordinary account's jurisdiction.
const FoundationCountry = "ZZ"

// CountryLength is how many characters of an identifier the prefix occupies.
const CountryLength = 2

// assignedCountries is every officially assigned ISO 3166-1 alpha-2 code.
var assignedCountries = map[string]struct{}{}

func init() {
	const codes = "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ " +
		"BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ " +
		"CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ " +
		"DE DJ DK DM DO DZ EC EE EG EH ER ES ET " +
		"FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY " +
		"HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT " +
		"JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ " +
		"LA LB LC LI LK LR LS LT LU LV LY " +
		"MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ " +
		"NA NC NE NF NG NI NL NO NP NR NU NZ OM " +
		"PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA " +
		"RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ " +
		"TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ " +
		"UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW"

	for i := 0; i+CountryLength <= len(codes); i += CountryLength + 1 {
		assignedCountries[codes[i:i+CountryLength]] = struct{}{}
	}
}

// AssignedCountry reports whether a code is an ISO 3166-1 alpha-2 code ISO has
// actually assigned. The foundation's reserved code is not one of them: it is
// deliberately excluded so that no ordinary account can be recorded there.
func AssignedCountry(code string) bool {
	_, ok := assignedCountries[code]
	return ok
}

// IssuableCountry reports whether a code may appear as an identifier's prefix.
//
// Wider than AssignedCountry by exactly one code, the foundation's, and that
// widening is the whole exception. It is checked against the administrator list
// wherever it is used; this function only says the prefix is well formed.
func IssuableCountry(code string) bool {
	return code == FoundationCountry || AssignedCountry(code)
}

// NormaliseCountry uppercases a country code without touching anything else.
//
// Deliberately not Normalise: the payload's Crockford folding turns I into 1
// and O into 0, which would collapse CI onto CL and SI onto SL. A perimeter
// that cannot tell Côte d'Ivoire from Chile is not a perimeter.
func NormaliseCountry(code string) string {
	b := []byte(code)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}
