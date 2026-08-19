package main

// The currencies this chain issues on testnet.
//
// One table, several outputs. The same list has to appear in the stablecoin
// module's genesis, in the bank module's denom metadata, in the oracle's
// accepted denoms and in the client SDK's display registry — and a list that
// lives in four places is a list that disagrees with itself within a month. The
// first symptom would be a balance rendered in base units because one file
// learned about a currency and another did not.
//
// Coverage is every ISO 4217 currency in use on the African continent, which is
// 42 codes for 54 countries: the CFA franc unions account for fourteen of them
// between XOF and XAF, and Lesotho, Namibia and Eswatini use the rand alongside
// their own pegged currencies.

// Currency is one issuable unit.
type Currency struct {
	// Code is the ISO 4217 alphabetic code, e.g. "NGN".
	Code string
	// Name is what a person calls it.
	Name string
	// Minor is the ISO 4217 minor unit count — 2 for the naira's kobo, 0 for
	// the CFA franc, which has no subunit in practice.
	//
	// This is deliberately *not* the chain's exponent. On-chain every currency
	// is held in millionths so that a swap fee or an interest accrual has room
	// to be exact; Minor is what an interface rounds to when it prints one, so
	// 1,359.844414 NGN is stored exactly and shown as ₦1,359.84.
	Minor int
	// Where names the countries using it, for the docs and the explorer.
	Where string
}

// AllCurrencies is the table. Ordered by code so every generated output is
// stable — a reordering would show up as a diff in four files at once and tell
// nobody anything.
var AllCurrencies = []Currency{
	{"AOA", "Angolan Kwanza", 2, "Angola"},
	{"BIF", "Burundian Franc", 0, "Burundi"},
	{"BWP", "Botswana Pula", 2, "Botswana"},
	{"CDF", "Congolese Franc", 2, "Democratic Republic of the Congo"},
	{"CVE", "Cape Verdean Escudo", 2, "Cabo Verde"},
	{"DJF", "Djiboutian Franc", 0, "Djibouti"},
	{"DZD", "Algerian Dinar", 2, "Algeria"},
	{"EGP", "Egyptian Pound", 2, "Egypt"},
	{"ERN", "Eritrean Nakfa", 2, "Eritrea"},
	{"ETB", "Ethiopian Birr", 2, "Ethiopia"},
	{"GHS", "Ghanaian Cedi", 2, "Ghana"},
	{"GMD", "Gambian Dalasi", 2, "The Gambia"},
	{"GNF", "Guinean Franc", 0, "Guinea"},
	{"KES", "Kenyan Shilling", 2, "Kenya"},
	{"KMF", "Comorian Franc", 0, "Comoros"},
	{"LRD", "Liberian Dollar", 2, "Liberia"},
	{"LSL", "Lesotho Loti", 2, "Lesotho"},
	{"LYD", "Libyan Dinar", 3, "Libya"},
	{"MAD", "Moroccan Dirham", 2, "Morocco, Western Sahara"},
	{"MGA", "Malagasy Ariary", 2, "Madagascar"},
	{"MRU", "Mauritanian Ouguiya", 2, "Mauritania"},
	{"MUR", "Mauritian Rupee", 2, "Mauritius"},
	{"MWK", "Malawian Kwacha", 2, "Malawi"},
	{"MZN", "Mozambican Metical", 2, "Mozambique"},
	{"NAD", "Namibian Dollar", 2, "Namibia"},
	{"NGN", "Nigerian Naira", 2, "Nigeria"},
	{"RWF", "Rwandan Franc", 0, "Rwanda"},
	{"SCR", "Seychellois Rupee", 2, "Seychelles"},
	{"SDG", "Sudanese Pound", 2, "Sudan"},
	{"SLE", "Sierra Leonean Leone", 2, "Sierra Leone"},
	{"SOS", "Somali Shilling", 2, "Somalia"},
	{"SSP", "South Sudanese Pound", 2, "South Sudan"},
	{"STN", "São Tomé and Príncipe Dobra", 2, "São Tomé and Príncipe"},
	{"SZL", "Swazi Lilangeni", 2, "Eswatini"},
	{"TND", "Tunisian Dinar", 3, "Tunisia"},
	{"TZS", "Tanzanian Shilling", 2, "Tanzania"},
	{"UGX", "Ugandan Shilling", 0, "Uganda"},
	{"XAF", "Central African CFA Franc", 0, "Cameroon, Central African Republic, Chad, Republic of the Congo, Equatorial Guinea, Gabon"},
	{"XOF", "West African CFA Franc", 0, "Benin, Burkina Faso, Côte d'Ivoire, Guinea-Bissau, Mali, Niger, Senegal, Togo"},
	{"ZAR", "South African Rand", 2, "South Africa, Lesotho, Namibia, Eswatini"},
	{"ZMW", "Zambian Kwacha", 2, "Zambia"},
	{"ZWG", "Zimbabwe Gold", 2, "Zimbabwe"},
}

// ExistingDenoms are the currencies the chain already carried before the
// African set: the native token and the reference currencies the oracle was
// launched with. They are kept in the oracle's accepted list rather than
// replaced, because the FX rates people actually quote against are dollars and
// euros, and a naira rate with nothing to compare it to prices nothing.
var ExistingDenoms = []string{"uyml", "uusd", "uchf", "ueur", "ugbp", "ujpy"}

// ChainExponent is how many decimal places every currency is held in on chain.
//
// Uniform on purpose, and larger than any of these currencies uses in the real
// world. A pool fee taken from a zero-decimal currency like the CFA franc would
// otherwise have nowhere to go but rounding, and rounding that has nowhere to
// go is rounding that quietly favours somebody. Interfaces print Minor places;
// the ledger keeps six.
const ChainExponent = 6

// Denom is the base denomination on chain: micro-units, lowercase.
func (c Currency) Denom() string { return "u" + lower(c.Code) }

// Description is what the stablecoin module records about the currency, and
// what an explorer shows when somebody asks what a denom actually is.
func (c Currency) Description() string {
	return c.Name + ", the currency of " + c.Where + ". Testnet issuance; not redeemable."
}

func lower(s string) string {
	out := []byte(s)
	for i := range out {
		if out[i] >= 'A' && out[i] <= 'Z' {
			out[i] += 'a' - 'A'
		}
	}
	return string(out)
}
