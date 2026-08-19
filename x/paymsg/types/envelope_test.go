package types_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/paymsg/types"
)

// keypair returns one party's X25519 halves.
func keypair(t *testing.T) (*ecdh.PrivateKey, []byte) {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv, priv.PublicKey().Bytes()
}

// The three parties the confidentiality design names all read the same payload,
// and a fourth does not. This is the whole feature in one test.
func TestEveryEntitledPartyOpensAndAStrangerDoesNot(t *testing.T) {
	payerKey, payerPub := keypair(t)
	payeeKey, payeePub := keypair(t)
	regulatorKey, regulatorPub := keypair(t)
	strangerKey, _ := keypair(t)

	payload, err := types.NewPaymentMetadata("SALA", "March salary, employee 4417")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-1")

	env, err := types.SealPayload(payload, []types.Recipient{
		{PublicKey: payerPub}, {PublicKey: payeePub}, {PublicKey: regulatorPub},
	}, aad)
	require.NoError(t, err)

	for name, key := range map[string]*ecdh.PrivateKey{
		"payer": payerKey, "payee": payeeKey, "regulator": regulatorKey,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := types.OpenPayload(env, key, aad)
			require.NoError(t, err)
			require.Equal(t, payload.PurposeCode, got.PurposeCode)
			require.Equal(t, payload.RemittanceInformation, got.RemittanceInformation)
			require.Equal(t, payload.Salt, got.Salt)
		})
	}

	// The fourth party holds a well-formed key and is simply not on the
	// envelope. It must fail as "not addressed to you" rather than as a
	// malformed envelope, because a client that reported the latter would send
	// the payee chasing the store for a file that is perfectly intact.
	_, err = types.OpenPayload(env, strangerKey, aad)
	require.ErrorIs(t, err, types.ErrEnvelopeUnreadable)
}

// The envelope carries the payload the chain recorded, so a party who decrypts
// can prove which payload the payment carried. Without this the whole
// arrangement is a claim by whoever held the ciphertext.
func TestDecryptedPayloadStillVerifiesAgainstTheOnChainHash(t *testing.T) {
	key, pub := keypair(t)

	payload, err := types.NewPaymentMetadata("SUPP", "invoice 88213")
	require.NoError(t, err)
	recorded, err := payload.Hash()
	require.NoError(t, err)

	aad := types.PaymentAAD("yml1bankone", "E2E-2")
	env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)

	got, err := types.OpenPayload(env, key, aad)
	require.NoError(t, err)
	require.NoError(t, types.VerifyMetadata(got, recorded))
}

// A store that edited the remittance line has to be caught by the cipher, not
// only by the on-chain hash. The hash is the backstop; the AEAD tag is the
// check that fires whether or not the reader remembered to look the payment up.
func TestATamperedCiphertextDoesNotOpen(t *testing.T) {
	key, pub := keypair(t)
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-3")

	env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)

	env.Ciphertext[len(env.Ciphertext)/2] ^= 0x01
	_, err = types.OpenPayload(env, key, aad)
	require.ErrorIs(t, err, types.ErrEnvelopeMalformed)
}

// The associated data binds an envelope to one payment. A store able to serve
// payment A's envelope in answer to a request for payment B would otherwise
// hand back something that decrypts cleanly and only disagrees with the block
// if somebody checks.
func TestAnEnvelopeDoesNotOpenUnderAnotherPaymentsIdentity(t *testing.T) {
	key, pub := keypair(t)
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)

	right := types.PaymentAAD("yml1bankone", "E2E-A")
	env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, right)
	require.NoError(t, err)

	// The positive case first. Without it this test passes when the binding is
	// removed entirely — every open fails, including the wrong one, and a test
	// that only asserts failure cannot tell "correctly refused" from "nothing
	// works".
	opened, err := types.OpenPayload(env, key, right)
	require.NoError(t, err)
	require.Equal(t, "March salary", opened.RemittanceInformation)

	_, err = types.OpenPayload(env, key, types.PaymentAAD("yml1bankone", "E2E-B"))
	require.ErrorIs(t, err, types.ErrEnvelopeMalformed)

	// And the same reference under a different instructing participant, which
	// is the collision the payment record's composite key exists for.
	_, err = types.OpenPayload(env, key, types.PaymentAAD("yml1banktwo", "E2E-A"))
	require.ErrorIs(t, err, types.ErrEnvelopeMalformed)
}

// Rotation is forward-only: a key that has been rotated away from still opens
// the envelopes that were sealed to it, and the new key does not.
//
// This is the behaviour the design chose over re-wrapping history, and it is
// worth a test rather than a comment, because the alternative failure is
// invisible — an operator rotates, and payment detail from before the rotation
// silently stops opening.
func TestARotatedKeyStillOpensTheEnvelopesSealedToIt(t *testing.T) {
	oldKey, oldPub := keypair(t)
	newKey, newPub := keypair(t)

	payload, err := types.NewPaymentMetadata("SALA", "before the rotation")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-ROT")

	env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: oldPub}}, aad)
	require.NoError(t, err)

	got, err := types.OpenPayload(env, oldKey, aad)
	require.NoError(t, err)
	require.Equal(t, "before the rotation", got.RemittanceInformation)

	// The new key was not a recipient, so it does not open the old envelope.
	// Nothing re-wraps it, and nothing pretends it did.
	_, err = types.OpenPayload(env, newKey, aad)
	require.ErrorIs(t, err, types.ErrEnvelopeUnreadable)

	// Everything sealed after the rotation goes to the new key, and the old one
	// is now the party that cannot read.
	after, err := types.NewPaymentMetadata("SALA", "after the rotation")
	require.NoError(t, err)
	env2, err := types.SealPayload(after, []types.Recipient{{PublicKey: newPub}}, aad)
	require.NoError(t, err)

	got2, err := types.OpenPayload(env2, newKey, aad)
	require.NoError(t, err)
	require.Equal(t, "after the rotation", got2.RemittanceInformation)
	_, err = types.OpenPayload(env2, oldKey, aad)
	require.ErrorIs(t, err, types.ErrEnvelopeUnreadable)
}

// An envelope with no recipients encrypts, stores and serves perfectly and is
// readable by nobody. It has to be refused where it is built, because there is
// no later moment at which anything can tell it apart from a good one.
func TestSealRefusesAnEnvelopeNobodyCanRead(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)

	_, err = types.SealPayload(payload, nil, types.PaymentAAD("yml1bankone", "E2E-4"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "readable by nobody")
}

// A viewing key that is not a viewing key is refused at the moment of sealing.
// The alternative is an envelope that looks addressed to somebody and opens for
// nobody, discovered by the party who needed to read it.
func TestSealRefusesAMalformedRecipientKey(t *testing.T) {
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-5")

	_, err = types.SealPayload(payload, []types.Recipient{{PublicKey: make([]byte, 31)}}, aad)
	require.Error(t, err)

	// Thirty-two zero bytes is the case that matters most: it is what an
	// uninitialised buffer looks like, it is a low-order point, and Go's ECDH
	// refuses the agreement rather than returning a shared secret every holder
	// of the same value could reproduce.
	_, err = types.SealPayload(payload, []types.Recipient{{PublicKey: make([]byte, 32)}}, aad)
	require.Error(t, err)
}

// The ciphertext length must not track the length of the remittance line.
// Without padding, anyone who can see a response size can tell a four-character
// reference from a name and address, which is a meaningful part of what moving
// the payload off-chain was for.
func TestCiphertextLengthDoesNotLeakTheRemittanceLength(t *testing.T) {
	_, pub := keypair(t)
	aad := types.PaymentAAD("yml1bankone", "E2E-6")

	lengths := map[int]struct{}{}
	for _, remittance := range []string{
		"", "x", "invoice 1", "Mrs Adaeze Okonkwo, 14 Marina Road, Lagos, invoice 88213",
	} {
		payload, err := types.NewPaymentMetadata("SALA", remittance)
		require.NoError(t, err)
		env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
		require.NoError(t, err)
		lengths[len(env.Ciphertext)] = struct{}{}
	}
	require.Len(t, lengths, 1,
		"four payloads of very different lengths produced ciphertexts of different sizes, so the padding is not doing its job")
}

// Two sealings of one payload must not produce the same bytes, or an observer
// learns that two payments carry identical detail without ever reading it.
func TestSealingTwiceProducesDifferentBytes(t *testing.T) {
	_, pub := keypair(t)
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-7")

	first, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)
	second, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)

	require.NotEqual(t, first.Ciphertext, second.Ciphertext)
	require.NotEqual(t, first.Nonce, second.Nonce)
	require.NotEqual(t, first.Recipients[0].EphemeralPublicKey, second.Recipients[0].EphemeralPublicKey)
}

// The content key must be fresh per envelope, and differing ciphertexts do not
// prove that: a fixed content key with a fresh nonce still produces different
// bytes every time. What a shared content key would actually mean is that one
// recovered key opens every envelope ever sealed, so that is what is tested —
// a recipient block lifted from one envelope must not open another's body.
func TestABlockFromOneEnvelopeDoesNotOpenAnother(t *testing.T) {
	key, pub := keypair(t)
	aad := types.PaymentAAD("yml1bankone", "E2E-SHARED")

	first, err := types.NewPaymentMetadata("SALA", "first payment")
	require.NoError(t, err)
	second, err := types.NewPaymentMetadata("SALA", "second payment")
	require.NoError(t, err)

	envA, err := types.SealPayload(first, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)
	envB, err := types.SealPayload(second, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)

	// B's body under A's key. If the content key were shared this would open,
	// and one compromised key would read every payment on the rail.
	spliced := types.PayloadEnvelope{
		Version:    envB.Version,
		Recipients: envA.Recipients,
		Nonce:      envB.Nonce,
		Ciphertext: envB.Ciphertext,
	}
	_, err = types.OpenPayload(spliced, key, aad)
	require.ErrorIs(t, err, types.ErrEnvelopeMalformed,
		"a block from one envelope opened another's body, so the content key is not fresh per envelope")
}

// key_id is a hint and eight bytes is not a commitment. A reader whose
// fingerprint matches no block must still try the rest, or a collision would
// make an envelope it can open look like one addressed to somebody else.
func TestAReaderOpensABlockWhoseHintIsWrong(t *testing.T) {
	key, pub := keypair(t)
	payload, err := types.NewPaymentMetadata("SALA", "March salary")
	require.NoError(t, err)
	aad := types.PaymentAAD("yml1bankone", "E2E-8")

	env, err := types.SealPayload(payload, []types.Recipient{{PublicKey: pub}}, aad)
	require.NoError(t, err)

	env.Recipients[0].KeyId = []byte{9, 9, 9, 9, 9, 9, 9, 9}
	got, err := types.OpenPayload(env, key, aad)
	require.NoError(t, err)
	require.Equal(t, "March salary", got.RemittanceInformation)
}

// The envelope this repository writes must be the envelope the client SDK
// reads. Both suites open the same recorded bytes with the same recorded keys,
// so a change to the HKDF label, the info ordering, the associated data framing
// or the padding shows up here rather than as a payment nobody can read.
func TestEnvelopeVectorsOpenForEveryRecordedReader(t *testing.T) {
	for _, v := range loadVectors(t).Envelopes {
		t.Run(v.Name, func(t *testing.T) {
			var env types.PayloadEnvelope
			require.NoError(t, env.Unmarshal(mustHex(t, v.EnvelopeHex)))

			// The associated data is rebuilt from the payment's identity rather
			// than read from the file, so the vector pins PaymentAAD's framing
			// too. A build that ordered those fields differently would produce
			// a different aad and fail below.
			aad := types.PaymentAAD(v.InstructingParticipant, v.EndToEndID)
			require.Equal(t, v.AADHex, hex.EncodeToString(aad))

			require.NotEmpty(t, v.Readers)
			for _, r := range v.Readers {
				priv, err := ecdh.X25519().NewPrivateKey(mustHex(t, r.PrivateKeyHex))
				require.NoError(t, err)
				require.Equal(t, r.PublicKeyHex, hex.EncodeToString(priv.PublicKey().Bytes()))
				require.Equal(t, r.KeyIDHex, hex.EncodeToString(types.KeyID(priv.PublicKey().Bytes())))

				got, err := types.OpenPayload(env, priv, aad)
				require.NoError(t, err, "the %s could not open the recorded envelope", r.Role)
				require.Equal(t, v.PurposeCode, got.PurposeCode)
				require.Equal(t, v.RemittanceInformation, got.RemittanceInformation)
				require.Equal(t, v.SaltHex, hex.EncodeToString(got.Salt))

				// And what came out is what the chain recorded.
				require.NoError(t, types.VerifyMetadata(got, mustHex(t, v.HashHex)))
			}

			require.NotEmpty(t, v.NonReaders)
			for _, r := range v.NonReaders {
				priv, err := ecdh.X25519().NewPrivateKey(mustHex(t, r.PrivateKeyHex))
				require.NoError(t, err)
				_, err = types.OpenPayload(env, priv, aad)
				require.ErrorIs(t, err, types.ErrEnvelopeUnreadable,
					"the %s must not be able to open this envelope", r.Role)
			}
		})
	}
}
