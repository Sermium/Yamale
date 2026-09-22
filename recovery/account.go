package recovery

import (
	"bytes"
	"errors"
	"fmt"
)

// The whole construction, as the account services actually use it.
//
// Two calls. Protect turns a freshly generated key into three holdings, one for
// each party. Restore turns any two of them plus the password back into the
// key. Everything between is in shamir.go and seal.go.
//
// SEALING AT ALL is the design, and forgetting it is the failure that matters:
// split the raw key and the custodian plus the recovery service — both run by
// the platform — reconstruct it between them with nothing else needed.
// TestBothServicesTogetherStillCannotOpenIt fails at that exact line, which was
// checked by making the change rather than assumed.
//
// The ORDER is a smaller and subtler question, worth stating accurately because
// the obvious claim about it is wrong. Sealing each share separately instead
// would ALSO need the password, so it does not hand anybody the key. What it
// gives away is an ORACLE: each service would then hold a complete
// authenticated ciphertext of its own, so ONE breach is enough to test password
// guesses offline, for free, forever. Sealed first, a single share carries no
// tag and nothing to test a guess against — it is indistinguishable from
// random, which is what TestASingleShareIsIndependentOfEveryByteOfTheSecret
// measures. So one breach leaks nothing at all, and it takes two before the
// password is load-bearing.

// The three parties, and the evaluation point each one's share sits at.
//
// Fixed rather than assigned at runtime so that a holding's role says which
// share it is. Two holdings that claimed the same role would interpolate
// through one point twice, which Combine refuses.
const (
	RoleDevice    = "device"
	RoleCustodian = "custodian"
	RoleRecovery  = "recovery"
)

var roleX = map[string]byte{
	RoleDevice:    1,
	RoleCustodian: 2,
	RoleRecovery:  3,
}

// Threshold is how many holdings a recovery needs.
//
// Two of three. One is not enough or a single breached service would be the
// end of it; three would mean that losing the phone — the ordinary reason
// anybody recovers anything — made recovery impossible.
const Threshold = 2

var (
	ErrNotEnoughHoldings = errors.New("a recovery needs at least two holdings")
	ErrMixedHoldings     = errors.New("these holdings are not from the same sealed key")
	ErrUnknownRole       = errors.New("that is not one of the three roles")
)

// Holding is what one party stores.
//
// The header — salt, nonce, Argon2 parameters — is NOT secret and is repeated
// in every holding. It has to be: a recovery is assembled from whichever two
// parties are reachable, and if the header lived with only one of them, that
// party's loss would be unrecoverable while its share was still theoretically
// unnecessary.
type Holding struct {
	Role string `json:"role"`

	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`

	Share Share `json:"share"`
}

// Protect seals a secret under a password and splits the result three ways.
//
// The secret is a secp256k1 private key in the intended use, but nothing here
// depends on that — it is bytes, and keeping it that way means this package can
// be tested without a curve and reused for anything else worth recovering.
func Protect(secret []byte, password string) ([]Holding, error) {
	sealed, err := Seal(secret, password)
	if err != nil {
		return nil, err
	}

	// The CIPHERTEXT is what gets split. See the note at the top of this file.
	shares, err := Split(sealed.Ciphertext, 3, Threshold)
	if err != nil {
		return nil, err
	}

	out := make([]Holding, 0, 3)
	for _, role := range []string{RoleDevice, RoleCustodian, RoleRecovery} {
		x := roleX[role]
		var share Share
		for _, s := range shares {
			if s.X == x {
				share = s
				break
			}
		}
		if share.X == 0 {
			return nil, fmt.Errorf("no share at x=%d for the %s role", x, role)
		}
		out = append(out, Holding{
			Role:    role,
			Salt:    sealed.Salt,
			Nonce:   sealed.Nonce,
			Time:    sealed.Time,
			Memory:  sealed.Memory,
			Threads: sealed.Threads,
			Share:   share,
		})
	}
	return out, nil
}

// Restore rebuilds the secret from any two holdings and the password.
//
// A wrong password and a wrong pair of holdings both come back as ErrWrongSeal,
// because the authentication tag is the only thing that can tell either of them
// apart from success, and it cannot say which happened. That is also the
// property that makes splitting the ciphertext safe: an attacker with two
// shares and a guess learns whether the guess was right, but they had to spend
// an Argon2id derivation to find out.
func Restore(holdings []Holding, password string) ([]byte, error) {
	if len(holdings) < Threshold {
		return nil, ErrNotEnoughHoldings
	}

	head := holdings[0]
	if _, ok := roleX[head.Role]; !ok {
		return nil, ErrUnknownRole
	}
	shares := make([]Share, 0, len(holdings))
	for _, h := range holdings {
		if _, ok := roleX[h.Role]; !ok {
			return nil, ErrUnknownRole
		}
		// Checked rather than trusted. Two holdings from different enrolments
		// would combine into a ciphertext that fails its tag, so the account
		// would be reported unrecoverable when in fact the wrong pair was
		// fetched — a diagnosis nobody would reach from ErrWrongSeal alone.
		if !bytes.Equal(h.Salt, head.Salt) || !bytes.Equal(h.Nonce, head.Nonce) ||
			h.Time != head.Time || h.Memory != head.Memory || h.Threads != head.Threads {
			return nil, ErrMixedHoldings
		}
		shares = append(shares, h.Share)
	}

	ciphertext, err := Combine(shares)
	if err != nil {
		return nil, err
	}

	sealed := &Sealed{
		Salt:       head.Salt,
		Nonce:      head.Nonce,
		Ciphertext: ciphertext,
		Time:       head.Time,
		Memory:     head.Memory,
		Threads:    head.Threads,
	}
	return sealed.Open(password)
}
