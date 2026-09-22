package recovery

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Sealing the key before it is split.
//
// # What this defends, exactly
//
// Splitting alone gives "no ONE service can reconstruct the key". Sealing gives
// "not even ALL of them together can", which is the property that survives both
// services being run by the same operator — and that operator is the platform
// itself, so the weaker statement was never worth making.
//
// Two colluding services reconstruct a ciphertext. Opening it needs the
// password, which no service holds in any form that can produce it.
//
// # What it does NOT defend, said plainly
//
// The password becomes the last line, so a breach of BOTH services exposes the
// account to offline guessing at Argon2id speed. A threshold signature scheme
// offered no such avenue. That is a real reduction and it is the trade decision
// A already made everywhere else: with the device signing alone, nothing
// co-signs and nothing can freeze, so a stolen device and password is a stolen
// account regardless of what this file does.
//
// The honest summary is therefore: this protects against one service being
// breached (completely), and against both being breached (only as well as the
// password is chosen). The length floor in checkPassword is what makes the
// second clause mean anything.
//
// # The trap this file exists to avoid
//
// The custodian STORES argon2.IDKey(password, salt, ...) as a password
// verifier — see Verifier in tools/custodian/auth.go. If the seal derived its
// key the same way from the same password, the stored verifier WOULD BE the
// decryption key: a store dump would open every account with no guessing at
// all, and the whole construction would be decoration.
//
// So the seal derives through its own domain separator and its own salt, and
// TestTheSealKeyIsNeverThePasswordVerifier holds that line.

// Argon2id parameters for the seal.
//
// Deliberately heavier than the login verifier's. A verifier is computed on
// every sign-in and its cost is paid by a waiting human; a seal key is derived
// twice in the life of an account — once at enrolment, once at a recovery — so
// there is no reason to be gentle. This is the number an attacker with the
// ciphertext has to pay per guess.
const (
	sealTime    = 4
	sealMemory  = 256 * 1024 // KiB
	sealThreads = 4
	sealKeyLen  = chacha20poly1305.KeySize
	sealSaltLen = 16
)

// sealDomain separates this derivation from every other use of the same
// password. Mixed into the salt rather than the password so that it cannot be
// confused with a password a user actually typed.
const sealDomain = "yamale/recovery/seal/v1"

var (
	ErrNoPassword     = errors.New("a seal needs a password")
	ErrWrongSeal      = errors.New("the password is wrong, or the sealed key was not reconstructed correctly")
	ErrSealMalformed  = errors.New("the sealed key is not in a form this can open")
	ErrSealWeakParams = errors.New("the sealed key names Argon2 parameters weaker than this build accepts")
)

// Sealed is a key encrypted under a password.
//
// The parameters travel with it. Not for the attacker's benefit — they can
// count the memory a derivation uses — but so that raising sealMemory later
// does not strand every account enrolled before the change. Without them, the
// only way to open an old blob would be to guess which build wrote it.
type Sealed struct {
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`

	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
}

// Seal encrypts a secret under a password.
func Seal(secret []byte, password string) (*Sealed, error) {
	if len(secret) == 0 {
		return nil, ErrEmptySecret
	}
	if password == "" {
		return nil, ErrNoPassword
	}

	salt := make([]byte, sealSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("reading randomness for a salt: %w", err)
	}

	out := &Sealed{
		Salt:    salt,
		Time:    sealTime,
		Memory:  sealMemory,
		Threads: sealThreads,
	}

	aead, err := out.aead(password)
	if err != nil {
		return nil, err
	}

	// XChaCha20's nonce is 24 bytes, which is wide enough that a random one
	// will not repeat. A 12-byte nonce would need a counter, and a counter
	// needs somewhere to persist across restarts that nothing here has.
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("reading randomness for a nonce: %w", err)
	}
	out.Nonce = nonce
	out.Ciphertext = aead.Seal(nil, nonce, secret, out.associatedData())
	return out, nil
}

// Open decrypts a sealed secret.
//
// A wrong password and a badly reconstructed ciphertext produce the same error,
// and that is on purpose: telling them apart would tell somebody holding two
// shares of a three-share split that their password was right, which is exactly
// the oracle the construction is trying not to give them.
func (s *Sealed) Open(password string) ([]byte, error) {
	if s == nil || len(s.Salt) == 0 || len(s.Nonce) == 0 || len(s.Ciphertext) == 0 {
		return nil, ErrSealMalformed
	}
	if password == "" {
		return nil, ErrNoPassword
	}
	// A floor, so that a blob written by a future build with deliberately weak
	// parameters — or one edited by somebody who got write access to the
	// store — cannot talk this build into a cheap derivation. It does not stop
	// an attacker with the ciphertext from choosing their own cost offline;
	// nothing can. It stops THIS service from doing weak work on request.
	if s.Time < 1 || s.Memory < 64*1024 || s.Threads < 1 {
		return nil, ErrSealWeakParams
	}

	aead, err := s.aead(password)
	if err != nil {
		return nil, err
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, ErrSealMalformed
	}
	out, err := aead.Open(nil, s.Nonce, s.Ciphertext, s.associatedData())
	if err != nil {
		return nil, ErrWrongSeal
	}
	return out, nil
}

// aead derives the key and builds the cipher.
func (s *Sealed) aead(password string) (interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
	NonceSize() int
}, error) {
	// The domain separator goes into the SALT. Appending it to the password
	// would mean a password of "x" + domain and a domain of "" produced the
	// same key, which is the sort of thing nobody notices until it matters.
	salt := append([]byte(sealDomain), s.Salt...)

	key := argon2.IDKey([]byte(password), salt, s.Time, s.Memory, s.Threads, sealKeyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("building the cipher: %w", err)
	}
	return aead, nil
}

// associatedData binds the parameters to the ciphertext.
//
// They already affect the key, so changing one breaks decryption anyway. This
// makes that failure happen in the authentication tag, where it is unambiguous,
// rather than in a derivation that silently produces the wrong key.
func (s *Sealed) associatedData() []byte {
	var b bytes.Buffer
	b.WriteString(sealDomain)
	_ = binary.Write(&b, binary.BigEndian, s.Time)
	_ = binary.Write(&b, binary.BigEndian, s.Memory)
	_ = binary.Write(&b, binary.BigEndian, s.Threads)
	return b.Bytes()
}
