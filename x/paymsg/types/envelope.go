package types

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// The envelope format, and the four constants that define it.
//
// Nothing here is novel, and that is deliberate. The construction is the
// standard multi-recipient KEM/DEM composition — a fresh content key encrypts
// the payload once, and one X25519 key agreement per recipient wraps a copy of
// that content key. It is what age does and what RFC 9180 formalises as
// DHKEM(X25519, HKDF-SHA256) with ChaCha20-Poly1305. The payload is the ISO
// 20022 detail of somebody's payment; it is not the place to find out whether a
// new scheme holds.
const (
	// EnvelopeVersion is the format this package writes.
	//
	// Present from the first byte because the alternative is discovering, on
	// the day the format has to change, that every stored envelope is a bare
	// blob whose framing can only be guessed at from its length.
	EnvelopeVersion = 1

	// ContentKeyLength is the ChaCha20-Poly1305 key size.
	ContentKeyLength = chacha20poly1305.KeySize

	// KeyIDLength is how much of the recipient's key fingerprint travels.
	//
	// Truncated so the block does not simply publish the recipient's public key
	// to whoever fetches the envelope. Eight bytes is a lookup hint and must
	// never be treated as a commitment: two keys can collide, so a reader that
	// fails to open a matching block has to try the rest.
	KeyIDLength = 8

	// PaddingBlock is what the plaintext is rounded up to before encryption.
	//
	// Without it the ciphertext length says how long the remittance line is,
	// and anyone who can see a response size can tell a four-character
	// reference from a full name and address. That is a meaningful part of what
	// moving the payload off-chain was supposed to protect, and it costs at
	// most 256 bytes to close.
	PaddingBlock = 256

	// MaxEnvelopePlaintext bounds what open will decompress into memory.
	//
	// A store serves envelopes to anyone who can authenticate, and an envelope
	// is bytes somebody else wrote. Without a bound, a malicious or corrupted
	// length prefix inside the padding framing would have the reader allocate
	// whatever it claimed.
	MaxEnvelopePlaintext = 64 * 1024
)

// envelopeDomain separates this construction's key derivation from every other
// use of X25519 on this chain.
//
// A shared secret is just 32 bytes; what stops a wrapping key being usable
// somewhere else is that the label went into the derivation. Without it, a
// future feature that agrees with the same viewing keys would derive the same
// key and silently cross the two.
const envelopeDomain = "yamale/paymsg/payload-envelope/v1"

var (
	// ErrEnvelopeUnreadable is returned when no recipient block opens.
	//
	// Distinct from a malformed envelope, because they mean opposite things to
	// the person holding it: one says "this is not addressed to you", the other
	// says "this is not an envelope". A reader that conflated them would tell a
	// payee their key was wrong when the store had handed them a truncated file.
	ErrEnvelopeUnreadable = errors.New("no recipient block in this envelope opens with the key provided")

	// ErrEnvelopeMalformed is returned for bytes that are not a well-formed
	// envelope of a version this package implements.
	ErrEnvelopeMalformed = errors.New("this is not a payload envelope this build can read")
)

// Recipient is one party entitled to read a payload.
type Recipient struct {
	// PublicKey is 32 bytes of X25519, as published in x/alias.
	PublicKey []byte
}

// KeyID returns the first KeyIDLength bytes of SHA-256 over a public key.
//
// Computed rather than stored, in one place, because a reader matches on it and
// a writer emits it: two implementations of this that disagree produce
// envelopes whose blocks nobody can find, which degrades to trying every block
// and therefore fails silently rather than loudly.
func KeyID(publicKey []byte) []byte {
	sum := sha256.Sum256(publicKey)
	return sum[:KeyIDLength]
}

// SealPayload encrypts a payment payload to every entitled recipient.
//
// aad binds the ciphertext to one payment. It is not decoration: without it a
// store could serve the payload of one payment under the key of another, and
// that substitution would decrypt cleanly and fail only against the on-chain
// hash — if the reader remembered to check it. With it, the substitution fails
// at the cipher, before the reader has to remember anything.
//
// The payload's own salt is left where it is and never leaves the plaintext.
// The salt is what makes the on-chain hash uninvertible, so it is also what
// makes deleting the payload an erasure rather than a gesture: a four-character
// purpose code hashed without one is a lookup table, and destroying a payload
// whose preimage anybody can enumerate erases nothing at all.
func SealPayload(payload PaymentMetadata, recipients []Recipient, aad []byte) (PayloadEnvelope, error) {
	if len(recipients) == 0 {
		// Refused rather than producing an envelope with no recipient blocks.
		// That envelope encrypts, stores and serves perfectly, and is readable
		// by nobody — a payment whose detail is gone from the moment it was
		// sent, discovered whenever somebody first tries to reconcile it.
		return PayloadEnvelope{}, errors.New("an envelope with no recipients is readable by nobody")
	}
	if len(payload.Salt) != MetadataSaltLength {
		return PayloadEnvelope{}, fmt.Errorf("payload salt must be %d bytes, got %d",
			MetadataSaltLength, len(payload.Salt))
	}
	if err := ValidateMetadataFields(payload.PurposeCode, payload.RemittanceInformation); err != nil {
		return PayloadEnvelope{}, err
	}

	plaintext, err := payload.Marshal()
	if err != nil {
		return PayloadEnvelope{}, err
	}
	padded, err := pad(plaintext)
	if err != nil {
		return PayloadEnvelope{}, err
	}

	contentKey := make([]byte, ContentKeyLength)
	if _, err := rand.Read(contentKey); err != nil {
		return PayloadEnvelope{}, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return PayloadEnvelope{}, err
	}

	aead, err := chacha20poly1305.New(contentKey)
	if err != nil {
		return PayloadEnvelope{}, err
	}
	ciphertext := aead.Seal(nil, nonce, padded, aad)

	blocks := make([]*RecipientBlock, 0, len(recipients))
	for _, r := range recipients {
		block, err := wrap(contentKey, r.PublicKey)
		if err != nil {
			return PayloadEnvelope{}, err
		}
		blocks = append(blocks, block)
	}

	return PayloadEnvelope{
		Version:    EnvelopeVersion,
		Recipients: blocks,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// OpenPayload decrypts an envelope with one recipient's private viewing key.
//
// aad must be the same payment identity the envelope was sealed under, so a
// caller that fetched the wrong payment's envelope is told so here rather than
// discovering it against the on-chain hash later — or not at all.
func OpenPayload(env PayloadEnvelope, privateKey *ecdh.PrivateKey, aad []byte) (PaymentMetadata, error) {
	if env.Version != EnvelopeVersion {
		return PaymentMetadata{}, fmt.Errorf("%w: version %d", ErrEnvelopeMalformed, env.Version)
	}
	if len(env.Nonce) != chacha20poly1305.NonceSize {
		return PaymentMetadata{}, fmt.Errorf("%w: nonce is %d bytes", ErrEnvelopeMalformed, len(env.Nonce))
	}
	if privateKey == nil {
		return PaymentMetadata{}, ErrEnvelopeUnreadable
	}

	want := KeyID(privateKey.PublicKey().Bytes())
	contentKey, found := unwrapMatching(env.Recipients, privateKey, want, true)
	if !found {
		// Falls back to every block. key_id is a hint and eight bytes is not a
		// commitment, so a reader that stopped at "no block matches my
		// fingerprint" would refuse an envelope it can actually open whenever
		// two keys collided — a failure that would be blamed on the store.
		contentKey, found = unwrapMatching(env.Recipients, privateKey, want, false)
	}
	if !found {
		return PaymentMetadata{}, ErrEnvelopeUnreadable
	}

	aead, err := chacha20poly1305.New(contentKey)
	if err != nil {
		return PaymentMetadata{}, err
	}
	padded, err := aead.Open(nil, env.Nonce, env.Ciphertext, aad)
	if err != nil {
		// The content key opened a block, so the reader is entitled; the body
		// still failed. That is a tampered ciphertext or the wrong payment's
		// associated data, never a key problem, and saying so is the difference
		// between an afternoon spent on a store bug and one spent on a key.
		return PaymentMetadata{}, fmt.Errorf("%w: the content key was recovered but the body did not authenticate, so the ciphertext or the payment it names has been altered", ErrEnvelopeMalformed)
	}

	plaintext, err := unpad(padded)
	if err != nil {
		return PaymentMetadata{}, err
	}
	var payload PaymentMetadata
	if err := payload.Unmarshal(plaintext); err != nil {
		return PaymentMetadata{}, fmt.Errorf("%w: %s", ErrEnvelopeMalformed, err)
	}
	return payload, nil
}

// PaymentAAD is the associated data every envelope for a payment is bound to.
//
// The domain string is in it so an envelope cannot be replayed into some other
// feature that happens to use the same participant and reference. Built here,
// once, because the payer seals with it and three different readers open with
// it: a fifth implementation that ordered the fields differently would produce
// envelopes that authenticate for nobody.
func PaymentAAD(instructingParticipant, endToEndID string) []byte {
	var b bytes.Buffer
	b.WriteString(envelopeDomain)
	b.WriteByte(0)
	b.WriteString(instructingParticipant)
	b.WriteByte(0)
	b.WriteString(endToEndID)
	return b.Bytes()
}

// WrapTo seals a 32-byte secret to one viewing key.
//
// Exported so the payload store's challenge-response uses this exact
// construction rather than a second one written beside it. The challenge is
// "decrypt this and tell me what it says", and proving possession of the
// viewing key is precisely the credential that entitles the caller to the
// payload — so the challenge grants nothing the key did not already grant, and
// a separate mechanism would be a second thing to get wrong for no gain.
func WrapTo(secret, recipientPublic []byte) (*RecipientBlock, error) {
	if len(secret) != ContentKeyLength {
		return nil, fmt.Errorf("secret must be %d bytes, got %d", ContentKeyLength, len(secret))
	}
	return wrap(secret, recipientPublic)
}

// UnwrapFrom recovers a secret sealed by WrapTo.
func UnwrapFrom(block *RecipientBlock, priv *ecdh.PrivateKey) ([]byte, error) {
	if block == nil || priv == nil {
		return nil, ErrEnvelopeUnreadable
	}
	secret, err := unwrap(block, priv, priv.PublicKey().Bytes())
	if err != nil {
		return nil, ErrEnvelopeUnreadable
	}
	return secret, nil
}

// wrap seals a copy of the content key to one recipient.
//
// The ephemeral key is fresh per block rather than shared across the envelope.
// It costs 32 bytes each and buys two things: the blocks are unlinkable to each
// other, and a party who later holds the content key can add a recipient — an
// auditor appointed after the fact, a rotated regulator key — without the
// original ephemeral secret, which nobody kept.
func wrap(contentKey, recipientPublic []byte) (*RecipientBlock, error) {
	pub, err := ecdh.X25519().NewPublicKey(recipientPublic)
	if err != nil {
		return nil, fmt.Errorf("recipient viewing key is not a valid X25519 public key: %w", err)
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := ephemeral.ECDH(pub)
	if err != nil {
		// X25519 agreement with a low-order point yields an all-zero secret,
		// and Go's ECDH refuses it rather than returning one. Surfaced rather
		// than ignored: a key that produces this is one every holder of the
		// same degenerate point can also "agree" with.
		return nil, fmt.Errorf("key agreement with the recipient's viewing key failed: %w", err)
	}

	keyID := KeyID(recipientPublic)
	wrappingKey, err := deriveWrappingKey(shared, ephemeral.PublicKey().Bytes(), recipientPublic)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(wrappingKey)
	if err != nil {
		return nil, err
	}

	// A zero nonce is correct here and only here: the wrapping key is derived
	// from a freshly generated ephemeral secret, so it is used for exactly one
	// encryption and can never repeat. Deriving a random nonce as well would
	// add 12 bytes per block and protect against nothing.
	nonce := make([]byte, chacha20poly1305.NonceSize)
	// The recipient's key_id is the associated data, so a block cannot be moved
	// between envelopes or dropped into a different recipient's slot without
	// the tag failing.
	return &RecipientBlock{
		KeyId:              keyID,
		EphemeralPublicKey: ephemeral.PublicKey().Bytes(),
		WrappedKey:         aead.Seal(nil, nonce, contentKey, keyID),
	}, nil
}

// unwrapMatching tries the blocks, either only those whose hint matches or all
// of them.
func unwrapMatching(blocks []*RecipientBlock, priv *ecdh.PrivateKey, want []byte, onlyMatching bool) ([]byte, bool) {
	self := priv.PublicKey().Bytes()
	for _, block := range blocks {
		if block == nil {
			continue
		}
		if onlyMatching && subtle.ConstantTimeCompare(block.KeyId, want) != 1 {
			continue
		}
		key, err := unwrap(block, priv, self)
		if err != nil {
			continue
		}
		return key, true
	}
	return nil, false
}

// unwrap recovers the content key from one block, or fails.
func unwrap(block *RecipientBlock, priv *ecdh.PrivateKey, self []byte) ([]byte, error) {
	ephemeral, err := ecdh.X25519().NewPublicKey(block.EphemeralPublicKey)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(ephemeral)
	if err != nil {
		return nil, err
	}
	// Derived against this reader's own public key, not against block.KeyId.
	// Taking the identity from the block would let a writer name somebody
	// else's fingerprint in a block only this reader can open, and the tag
	// would still verify — so the block's claim about who it is for could
	// disagree with who can actually read it.
	wrappingKey, err := deriveWrappingKey(shared, block.EphemeralPublicKey, self)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(wrappingKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	key, err := aead.Open(nil, nonce, block.WrappedKey, KeyID(self))
	if err != nil {
		return nil, err
	}
	if len(key) != ContentKeyLength {
		return nil, errors.New("wrapped key is not a content key")
	}
	return key, nil
}

// deriveWrappingKey turns a raw X25519 secret into a key for one block.
//
// Both public keys go into the derivation, in a fixed order, alongside the
// domain label. Hashing the shared secret alone would derive the same key for
// two different pairings that happened to agree on it, and binding the
// transcript is what makes each block's key specific to the ephemeral that
// produced it and the recipient it was meant for.
func deriveWrappingKey(shared, ephemeralPublic, recipientPublic []byte) ([]byte, error) {
	info := make([]byte, 0, len(ephemeralPublic)+len(recipientPublic))
	info = append(info, ephemeralPublic...)
	info = append(info, recipientPublic...)
	// No salt: the shared secret is already high-entropy and the transcript is
	// carried in info, which is what HKDF's info parameter is for. A random
	// salt would have to travel in the block for the reader to reproduce it.
	return hkdf.Key(sha256.New, shared, nil, envelopeDomain+"\x00"+string(info), ContentKeyLength)
}

// pad rounds the plaintext up to a multiple of PaddingBlock.
//
// Length-prefixed rather than trailing-marker padding, because the payload is
// protobuf and protobuf has no self-delimiting end: a reader that stripped
// trailing zeroes would also strip the last byte of any field that legitimately
// ended in one, corrupting the payload only for some inputs.
func pad(plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxEnvelopePlaintext {
		return nil, fmt.Errorf("payload is %d bytes, above the %d-byte limit", len(plaintext), MaxEnvelopePlaintext)
	}
	total := len(plaintext) + 4
	if r := total % PaddingBlock; r != 0 {
		total += PaddingBlock - r
	}
	padded := make([]byte, total)
	padded[0] = byte(len(plaintext) >> 24)
	padded[1] = byte(len(plaintext) >> 16)
	padded[2] = byte(len(plaintext) >> 8)
	padded[3] = byte(len(plaintext))
	copy(padded[4:], plaintext)
	return padded, nil
}

// unpad recovers the plaintext, refusing a length the buffer cannot hold.
func unpad(padded []byte) ([]byte, error) {
	if len(padded) < 4 {
		return nil, fmt.Errorf("%w: padded plaintext is %d bytes", ErrEnvelopeMalformed, len(padded))
	}
	n := int(padded[0])<<24 | int(padded[1])<<16 | int(padded[2])<<8 | int(padded[3])
	// Checked against the buffer and against the limit before it is used as a
	// bound. The AEAD tag already proves these bytes are the ones sealed, but a
	// reader that trusted the prefix on that basis would panic on an envelope
	// its own sealer produced wrongly, which is a worse way to find out.
	if n < 0 || n > MaxEnvelopePlaintext || 4+n > len(padded) {
		return nil, fmt.Errorf("%w: padded plaintext claims a %d-byte payload", ErrEnvelopeMalformed, n)
	}
	return padded[4 : 4+n], nil
}
