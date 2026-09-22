package recovery

import (
	"crypto/rand"
	"crypto/sha256"
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
// Heavier than the login verifier's, because a verifier is computed on every
// sign-in with a human waiting, while a seal key is derived twice in the life
// of an account — once at enrolment, once at a recovery.
//
// # These are constrained by the BROWSER, which is where a seal is created
//
// The device generates the key and seals it, so whatever this file does must be
// reproducible by JavaScript. The only Argon2id available there without a
// secure context is libsodium's, through @cosmjs/crypto — and crypto.subtle is
// genuinely absent on the devnet, which is served over plain HTTP; see the note
// at the top of clients/wallet/src/vault.ts, written after that exact failure.
//
// libsodium's crypto_pwhash imposes two limits that are not negotiable:
//
//   - ALG_ARGON2ID13 runs with PARALLELISM 1 and exposes no way to change it.
//     An earlier version of this file used 4, which no browser could reproduce,
//     so a blob sealed here could never have been opened there.
//   - the salt must be EXACTLY 16 bytes. The domain separator is therefore
//     hashed into the salt rather than prepended to it; see argonSalt.
//
// Memory and time are measured rather than guessed, with the same WASM the
// browser runs:
//
//	t=24, m=12 MiB   0.49s   what clients/wallet already ships and works
//	t=3,  m=128 MiB  0.65s   this
//	t=2,  m=256 MiB  0.88s
//	t=4,  m=256 MiB  1.80s   the earlier choice
//
// A phone is three to six times slower than the machine those came from, so
// this lands around two to four seconds there. 256 MiB was rejected on MEMORY
// rather than time: a quarter-gigabyte WASM allocation on a low-end Android
// browser is a failure cliff, not a slow path, and the difference between 128
// and 256 MiB is about one bit of an attacker's cost — nothing beside the
// entropy of the password itself.
const (
	sealTime   = 3
	sealMemory = 128 * 1024 // KiB
	// Fixed at 1, and not a preference. See above: libsodium cannot do anything
	// else, and a seal the device cannot open is not a seal, it is a loss.
	sealThreads = 1
	sealKeyLen  = chacha20poly1305.KeySize
	sealSaltLen = 16
)

// sealDomain separates this derivation from every other use of the same
// password. Mixed into the salt rather than the password so that it cannot be
// confused with a password a user actually typed.
const sealDomain = "yamale/recovery/seal/v1"

// argonSalt is the 16 bytes handed to Argon2id.
//
// The domain separator cannot simply be prepended: libsodium requires exactly
// 16 bytes and throws otherwise, so the browser could not reproduce a longer
// one. Hashing both down to 16 preserves the separation and fits.
//
// Length-prefixed inside the hash for the same reason DepositDigest is. Both
// inputs happen to be fixed-length today, so nothing is ambiguous yet — but the
// day somebody makes the domain a variable is the day a prefix-free encoding
// stops being decoration.
func argonSalt(salt []byte) []byte {
	h := sha256.New()
	var n [4]byte
	for _, part := range [][]byte{[]byte(sealDomain), salt} {
		binary.BigEndian.PutUint32(n[:], uint32(len(part)))
		h.Write(n[:])
		h.Write(part)
	}
	return h.Sum(nil)[:16]
}

var (
	ErrNoPassword      = errors.New("a seal needs a password")
	ErrWrongSeal       = errors.New("the password is wrong, or the sealed key was not reconstructed correctly")
	ErrSealMalformed   = errors.New("the sealed key is not in a form this can open")
	ErrSealWeakParams  = errors.New("the sealed key names Argon2 parameters weaker than this build accepts")
	ErrSealNotPortable = errors.New(
		"the sealed key names an Argon2 parallelism other than 1, which no browser can reproduce")
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
	out.Ciphertext = aead.Seal(nil, nonce, secret, nil)
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
	if s.Time < 1 || s.Memory < 64*1024 {
		return nil, ErrSealWeakParams
	}
	// Anything other than 1 is unopenable in a browser, and the browser is
	// where a recovery is completed. Refused here rather than discovered by
	// somebody on a new phone with the right password and no account.
	if s.Threads != 1 {
		return nil, ErrSealNotPortable
	}

	aead, err := s.aead(password)
	if err != nil {
		return nil, err
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, ErrSealMalformed
	}
	out, err := aead.Open(nil, s.Nonce, s.Ciphertext, nil)
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
	key := argon2.IDKey([]byte(password), argonSalt(s.Salt), s.Time, s.Memory, s.Threads, sealKeyLen)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("building the cipher: %w", err)
	}
	return aead, nil
}

// There is no associated data, and the parameters are bound anyway.
//
// An earlier version passed the domain and the Argon2 parameters as AEAD
// associated data. That was removed for one reason: the browser has to open
// these, and the XChaCha20-Poly1305 that @cosmjs/crypto exposes takes no
// associated data at all. A field the device cannot supply is a field that
// makes every blob unopenable where it matters most.
//
// Nothing is actually unbound by the removal, which is why it was an acceptable
// trade rather than a concession:
//
//   - the domain is mixed into the Argon2 salt, see argonSalt;
//   - Time, Memory and Threads are INPUTS to the derivation, so altering any of
//     them produces a different key.
//
// So a tampered header still fails, and still fails as ErrWrongSeal. What
// changes is only where: the authentication tag rejects a key derived from the
// altered parameters, rather than rejecting the parameters directly. The
// observable behaviour is identical and the caller cannot tell the difference —
// which is the point, since Open deliberately does not distinguish a wrong
// password from a bad reconstruction either.
