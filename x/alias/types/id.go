package types

import (
	"crypto/sha256"
	"strings"
)

// The user ID: a country prefix, eight payload characters and one check
// character, written in groups for the eye — NG-K3M9-7QRT-B.
//
// The payload is Crockford Base32, which drops I, L, O and U from the 36
// alphanumerics. The first three go because I/l/1 and O/0 are the transcription
// errors people actually make; U goes so a random draw cannot spell something
// obscene. Crockford also decodes case-insensitively and folds I and L to 1 and
// O to 0 on input, so the most common typo corrects itself instead of failing.
//
// 32^8 = 1,099,511,627,776 — one-point-one trillion from eight characters,
// inside each country.
//
// The prefix is **not** Crockford and is never folded. Folding it would map CI
// and CL onto C1, and SI and SL onto S1: Côte d'Ivoire indistinguishable from
// Chile, Slovenia from Sierra Leone. A perimeter that cannot tell two countries
// apart is not a perimeter, so the prefix keeps all 26 letters and the folding
// starts after it.
const (
	// 0-9 then A-Z without I, L, O, U. Exactly 32 symbols, in Crockford's order.
	Alphabet       = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	PayloadLength  = 8
	MinPayloadLen  = 8
	MaxPayloadLen  = 16
	groupSeparator = "-"
)

// alphabet is indexed by value; index returns value. Built once rather than
// scanned per character.
var indexOf = func() map[byte]int {
	m := make(map[byte]int, 32)
	for i := 0; i < len(Alphabet); i++ {
		m[Alphabet[i]] = i
	}
	// Crockford's input folding. These never appear in output; they exist so a
	// person who wrote down an I or an O gets the right account rather than an
	// error they cannot act on.
	m['I'] = m['1']
	m['L'] = m['1']
	m['O'] = m['0']
	return m
}()

// checkCharacter computes the trailing check symbol with the Luhn mod N
// algorithm over base 32, across the country prefix and the payload together.
//
// Luhn rather than a plain weighted modulus because it catches **every**
// single-character substitution and every adjacent transposition of two
// characters that differ by other than half the base. A plain sum-mod-31 misses
// the swap of the first and last symbols in the alphabet; this does not.
//
// The prefix is covered because a typo in the country is exactly the error a
// check character exists to catch, and it is the one typo with no other
// defence. NG and NE are neighbours in the list and neighbours on the map; ML
// and MZ differ by one keystroke. Left out of the sum, a wrong country would
// still pass validation, reach the chain and resolve to nothing — and the
// client would have to report "no such account", which reads as "that person
// does not exist" rather than "you typed the country wrong". Covered, the
// mistake is caught in the input box, before a transaction is built.
//
// The two alphabets meet here. Prefix characters take their value from A=0..Z=25
// and payload characters from the Crockford table; both land inside 0..31, which
// is all Luhn mod 32 requires, and the mapping stays injective so every single
// substitution is still detected.
//
// Note what it does not promise: it is a typo check, not a signature. It stops
// somebody paying the wrong account because they misread a character. It stops
// nothing deliberate.
//
// An empty country reproduces the pre-jurisdiction computation exactly, which is
// what lets the identifiers issued before this existed still be recognised as
// the tombstones they became.
func checkCharacter(country, payload string) byte {
	const base = 32
	sum, factor := 0, 2

	fold := func(value int) {
		addend := factor * value
		factor = 3 - factor // alternate 2,1,2,1…
		addend = (addend / base) + (addend % base)
		sum += addend
	}

	for i := len(payload) - 1; i >= 0; i-- {
		fold(indexOf[payload[i]])
	}
	for i := len(country) - 1; i >= 0; i-- {
		fold(int(country[i] - 'A'))
	}

	remainder := sum % base
	return Alphabet[(base-remainder)%base]
}

// Normalise strips formatting, uppercases, and folds the ambiguous characters
// of the payload, so "ng-k3m9-7qrt-b" and "NGK3M97QRTB" are the same identifier.
//
// The first two characters are left unfolded because they are the country, and
// the fold is not injective over the 26 letters — see checkCharacter.
func Normalise(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c == '-' || c == ' ' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if b.Len() >= CountryLength {
			switch c {
			case 'I', 'L':
				c = '1'
			case 'O':
				c = '0'
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// normaliseLegacy folds every position, which is what the module did before
// identifiers carried a country. Used only to recognise the tombstones the
// migration left behind.
func normaliseLegacy(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c == '-' || c == ' ' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		switch c {
		case 'I', 'L':
			c = '1'
		case 'O':
			c = '0'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// Country returns the identifier's prefix, or "" if it is too short to have
// one. It reads the identifier rather than a stored field because the prefix is
// the only copy of that fact — two copies can disagree, and this is the one
// people read.
func Country(id string) string {
	n := Normalise(id)
	if len(n) < CountryLength {
		return ""
	}
	return n[:CountryLength]
}

// Format inserts the display grouping: country, four, four, then the check
// character — NG-K3M9-7QRT-B. Purely presentational; every stored and compared
// form is normalised.
func Format(id string) string {
	n := Normalise(id)
	if len(n) < CountryLength+2 {
		return n
	}
	country := n[:CountryLength]
	payload, check := n[CountryLength:len(n)-1], n[len(n)-1:]

	parts := []string{country}
	for i := 0; i < len(payload); i += 4 {
		end := i + 4
		if end > len(payload) {
			end = len(payload)
		}
		parts = append(parts, payload[i:end])
	}
	parts = append(parts, check)
	return strings.Join(parts, groupSeparator)
}

// Valid reports whether an identifier is well formed, carries a prefix this
// chain will issue, and agrees with its own check character. Clients call this
// before building a transaction so a mistyped ID never reaches the chain.
//
// It says nothing about whether the prefix is true of the account — that is
// state, checked at issuance, and the reason a lying one is never handed out in
// the first place.
func Valid(id string) bool {
	n := Normalise(id)
	if len(n) < CountryLength+MinPayloadLen+1 || len(n) > CountryLength+MaxPayloadLen+1 {
		return false
	}
	country := n[:CountryLength]
	if !IssuableCountry(country) {
		return false
	}
	payload, check := n[CountryLength:len(n)-1], n[len(n)-1]
	for i := 0; i < len(payload); i++ {
		if _, ok := indexOf[payload[i]]; !ok {
			return false
		}
	}
	return checkCharacter(country, payload) == check
}

// ValidLegacy reports whether an identifier is one of the prefixless ones the
// module issued before jurisdictions existed.
//
// They exist only as tombstones now: the v1-to-v2 migration retired every one
// of them, and nothing of this shape is ever issued again. It is kept so that
// genesis can still validate a retired list carried across the migration —
// dropping those tombstones to make the file validate would let a stranger be
// issued a handle somebody memorised.
func ValidLegacy(id string) bool {
	n := normaliseLegacy(id)
	if len(n) < MinPayloadLen+1 || len(n) > MaxPayloadLen+1 {
		return false
	}
	for i := 0; i < len(n); i++ {
		if _, ok := indexOf[n[i]]; !ok {
			return false
		}
	}
	payload, check := n[:len(n)-1], n[len(n)-1]
	return checkCharacter("", payload) == check
}

// Derive produces the identifier for an address in a country, deterministically.
//
// The chain assigns identifiers; nobody chooses one. A chosen handle brings
// squatting, a resale market and — the one that costs money — impersonation:
// YAMALE-PAY against YAMALE-PAY1 in a confirmation dialog is where every
// phishing attack on a name-based payment system starts.
//
// Derived from a hash rather than a counter so the registry does not leak how
// many accounts exist or in what order they joined. The nonce exists for
// collisions: the keeper increments it and derives again, which at 1.1 trillion
// values is not expected to happen and must still terminate when it does.
//
// The country goes into the pre-image, not just the prefix. Otherwise an account
// whose jurisdiction is corrected would be issued its own old payload back under
// a new prefix — NG-K3M9-7QRT-B replaced by GH-K3M9-7QRT-C — and the retired
// handle and the live one would be one glance apart, which is the confusion the
// module refuses to create anywhere else.
func Derive(country, address string, nonce uint64, payloadLength int) string {
	if payloadLength < MinPayloadLen {
		payloadLength = MinPayloadLen
	}
	if payloadLength > MaxPayloadLen {
		payloadLength = MaxPayloadLen
	}

	h := sha256.New()
	h.Write([]byte(country))
	h.Write([]byte(address))
	// Nonce folded in as bytes rather than as text so the pre-image cannot be
	// confused with a longer address ending in digits.
	var n [8]byte
	for i := 0; i < 8; i++ {
		n[i] = byte(nonce >> (8 * i))
	}
	h.Write(n[:])
	digest := h.Sum(nil)

	var b strings.Builder
	b.Grow(payloadLength + 1)
	for i := 0; i < payloadLength; i++ {
		b.WriteByte(Alphabet[int(digest[i])%32])
	}
	payload := b.String()
	return country + payload + string(checkCharacter(country, payload))
}
