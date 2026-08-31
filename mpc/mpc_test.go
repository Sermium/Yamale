package mpc_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/stretchr/testify/require"

	_ "yamale/blockchain/app" // its init() seals the yml bech32 prefix
	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// Key generation is genuinely expensive — safe primes, three parties — so it
// runs once and every test below shares the result. That is a compromise: the
// tests are not independent of one another's ordering in the way a fast suite
// would be. The alternative is four minutes per test, which in practice means
// the suite stops being run at all, and a threshold implementation nobody
// exercises is how the two dead functions in x/tokenisation happened.
var (
	once   sync.Once
	shares map[string]mpc.Share
	pre    map[string]*keygen.LocalPreParams
	genErr error
)

// preParams returns the material generated once for this suite. Reusing it for
// the reshare is sound here and would not be in production, where the incoming
// committee must not reuse the outgoing one's Paillier keys — that is what a
// pool is for.
func preParams() map[string]*keygen.LocalPreParams { return pre }

func accountShares(t *testing.T) map[string]mpc.Share {
	t.Helper()
	if testing.Short() {
		t.Skip("distributed key generation takes minutes; run without -short")
	}
	once.Do(func() {
		pre = make(map[string]*keygen.LocalPreParams, len(mpc.Roles))
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, role := range mpc.Roles {
			wg.Add(1)
			go func(role string) {
				defer wg.Done()
				p, err := mpc.GeneratePreParams(mpc.KeygenTimeout)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					genErr = err
					return
				}
				pre[role] = p
			}(role)
		}
		wg.Wait()
		if genErr != nil {
			return
		}
		shares, genErr = mpc.Keygen(pre)
	})
	require.NoError(t, genErr)
	require.Len(t, shares, 3)
	return shares
}

func hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func verify(t *testing.T, pub *ecdsa.PublicKey, digest, sig []byte) bool {
	t.Helper()
	require.Len(t, sig, 64)
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(pub, digest, r, s)
}

// The property the whole design exists for: the operator cannot move a
// customer's funds, and that is a statement about mathematics rather than about
// policy.
func TestTheCustodianAloneCannotSign(t *testing.T) {
	s := accountShares(t)

	_, _, err := mpc.Sign(hash("pay 500 to somebody else"), map[string]mpc.Share{
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot sign")

	// Nor can the device on its own, which is the same guarantee read from the
	// other end: a stolen phone is not a stolen account.
	_, _, err = mpc.Sign(hash("pay 500 to somebody else"), map[string]mpc.Share{
		mpc.RoleDevice: s[mpc.RoleDevice],
	})
	require.Error(t, err)
}

// The ordinary path. A person's device and the operator's service, together.
func TestDeviceAndCustodianSign(t *testing.T) {
	s := accountShares(t)
	digest := hash("MsgSendPayment 12.50 XOF")

	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:    s[mpc.RoleDevice],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	})
	require.NoError(t, err)
	require.True(t, verify(t, pub, digest, sig), "the signature does not verify")

	// Low-S, because Cosmos rejects the high form and a signature with two
	// valid encodings is a transaction with two hashes.
	half := new(big.Int).Rsh(ecdsaOrder(), 1)
	require.LessOrEqual(t, new(big.Int).SetBytes(sig[32:]).Cmp(half), 0,
		"S is in the upper half of the order and the chain will refuse it")
}

// The recovery path. The foundation's offline share and the custodian, which is
// what a person who has lost their password and their phone is left with.
func TestRecoveryAndCustodianSign(t *testing.T) {
	s := accountShares(t)
	digest := hash("recovered account, first payment")

	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleRecovery:  s[mpc.RoleRecovery],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	})
	require.NoError(t, err)
	require.True(t, verify(t, pub, digest, sig))
}

// Device and recovery, without the operator at all.
//
// Worth asserting on its own: it is the answer to "what happens to my money if
// the company running this disappears". Any two shares sign, and the operator
// is not privileged among them.
func TestDeviceAndRecoverySignWithoutTheOperator(t *testing.T) {
	s := accountShares(t)
	digest := hash("the operator is gone")

	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:   s[mpc.RoleDevice],
		mpc.RoleRecovery: s[mpc.RoleRecovery],
	})
	require.NoError(t, err)
	require.True(t, verify(t, pub, digest, sig))
}

// Every share agrees on one public key, and therefore on one address — which no
// share can produce a signature for by itself.
func TestEveryShareAgreesOnTheAddress(t *testing.T) {
	s := accountShares(t)

	var first string
	for _, role := range mpc.Roles {
		pub, err := s[role].PublicKey()
		require.NoError(t, err)
		addr, err := mpccosmos.Address(pub)
		require.NoError(t, err)
		require.NotEmpty(t, addr)
		if first == "" {
			first = addr
			continue
		}
		require.Equal(t, first, addr, "%s derives a different address", role)
	}
	// The chain's own prefix, sealed by app's init, rather than the SDK default:
	// an address that reads cosmos1... is one no Yamale interface would accept.
	require.True(t, strings.HasPrefix(first, "yml1"), "not a Yamale address: %s", first)
	t.Logf("threshold account address: %s", first)
}

// An ordinary secp256k1 public key comes out, so the chain's ante handler
// verifies it exactly as it verifies a self-custodied account's.
func TestTheKeyIsAnOrdinaryCosmosKey(t *testing.T) {
	s := accountShares(t)
	pub, err := s[mpc.RoleDevice].PublicKey()
	require.NoError(t, err)

	pk, err := mpccosmos.CosmosPubKey(pub)
	require.NoError(t, err)
	require.Len(t, pk.Bytes(), 33, "not a compressed secp256k1 key")

	// The SDK hashes what it is handed, so it takes the MESSAGE; the protocol
	// signs a digest. Getting this backwards produces a signature over
	// sha256(sha256(msg)) that verifies against nothing, which is exactly what
	// the first run of this test did.
	msg := []byte("verified by the chain's own code path")
	sig, _, err := mpc.Sign(hash(string(msg)), map[string]mpc.Share{
		mpc.RoleDevice:    s[mpc.RoleDevice],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	})
	require.NoError(t, err)

	// The SDK's verifier, not ours. This is the check that actually matters:
	// the chain's ante handler runs exactly this.
	require.True(t, pk.VerifySignature(msg, sig),
		"the cosmos-sdk verifier rejects a signature the shares produced")
}

// Two different messages produce two different signatures, and neither
// verifies for the other. Trivially true of a working implementation and worth
// having, because the failure it excludes — a protocol that ignores the message
// and returns something deterministic — verifies against nothing and is easy to
// miss when every other test signs the same string.
func TestASignatureIsBoundToItsMessage(t *testing.T) {
	s := accountShares(t)
	signers := map[string]mpc.Share{
		mpc.RoleDevice:    s[mpc.RoleDevice],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	}

	a, pub, err := mpc.Sign(hash("pay 10"), signers)
	require.NoError(t, err)
	b, _, err := mpc.Sign(hash("pay 10000"), signers)
	require.NoError(t, err)

	require.NotEqual(t, a, b)
	require.False(t, verify(t, pub, hash("pay 10000"), a),
		"a signature over one amount verifies over another")
}

func ecdsaOrder() *big.Int {
	// secp256k1 group order, written out rather than imported so the constant
	// this test compares against is visible in the test.
	n, ok := new(big.Int).SetString(
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	if !ok {
		panic("bad order")
	}
	return n
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }

// The password reset, which is the reason there are three shares rather than
// two — and the reason this is threshold ECDSA rather than a Cosmos multisig.
//
// A person forgets their password. The device share is wrapped by it, so it is
// gone. The other two — recovery and custodian — reshare, and what comes back
// is a fresh set under the SAME public key: same address, same x/alias
// identifier, and nobody who has ever paid this person needs to be told a
// thing.
func TestAPasswordResetKeepsTheAddress(t *testing.T) {
	s := accountShares(t)

	before, err := s[mpc.RoleDevice].PublicKey()
	require.NoError(t, err)
	addrBefore, err := mpccosmos.Address(before)
	require.NoError(t, err)

	// Only the two shares a person who lost their password still has.
	// Pre-params reused from key generation. In production these come from the
	// custodian's pool; the point of passing them is that without them each
	// incoming party spends four minutes hunting safe primes inside the
	// protocol, which turns a password reset into a quarter of an hour.
	fresh, err := mpc.Reshare(map[string]mpc.Share{
		mpc.RoleRecovery:  s[mpc.RoleRecovery],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	}, preParams())
	require.NoError(t, err)
	require.Len(t, fresh, 3, "a reshare must produce a share for every role")

	after, err := fresh[mpc.RoleDevice].PublicKey()
	require.NoError(t, err)
	addrAfter, err := mpccosmos.Address(after)
	require.NoError(t, err)
	require.Equal(t, addrBefore, addrAfter, "the reset moved the account")

	// And the new shares sign for it.
	digest := hash("first payment after the reset")
	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:    fresh[mpc.RoleDevice],
		mpc.RoleCustodian: fresh[mpc.RoleCustodian],
	})
	require.NoError(t, err)
	require.True(t, verify(t, pub, digest, sig))

	// The share ids moved, which is what makes the old ones useless.
	require.NotEqual(t,
		s[mpc.RoleDevice].Data.ShareID.String(),
		fresh[mpc.RoleDevice].Data.ShareID.String(),
		"the reshare reissued the same share id, so nothing was actually rotated")
}

// One share alone cannot reshare, for the same reason one share alone cannot
// sign. Otherwise an operator holding the custodian share could quietly move an
// account onto a committee of its own choosing, which is custody with extra
// steps.
func TestOneShareCannotReshare(t *testing.T) {
	s := accountShares(t)

	_, err := mpc.Reshare(map[string]mpc.Share{mpc.RoleCustodian: s[mpc.RoleCustodian]}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resharing needs")
}

// Two parties, two processes' worth of separation, and nothing but messages
// between them.
//
// This is the arrangement production actually runs, and the one that makes the
// security claim true. mpc.Sign puts every share in one address space, which is
// fine for a test and would be a whole key in all but name inside a browser —
// so this drives two SigningParty values that each hold exactly one share and
// exchange only Outbound envelopes, the way the device and the custodian will
// over HTTPS.
func TestTwoSeparatePartiesSignWithoutSharingAShare(t *testing.T) {
	s := accountShares(t)
	digest := hash("MsgSendPayment, signed across a network")
	committee := []string{mpc.RoleDevice, mpc.RoleCustodian}

	device, err := mpc.NewSigningParty(mpc.RoleDevice, digest, s[mpc.RoleDevice], committee)
	require.NoError(t, err)
	custodian, err := mpc.NewSigningParty(mpc.RoleCustodian, digest, s[mpc.RoleCustodian], committee)
	require.NoError(t, err)

	parties := map[string]*mpc.SigningParty{
		mpc.RoleDevice:    device,
		mpc.RoleCustodian: custodian,
	}

	// Pump the two until both hold a signature. The loop is the transport, and
	// it is deliberately dumb: no ordering, no retries, nothing tss-lib could
	// lean on that a real network would not provide.
	deadline := time.Now().Add(3 * time.Minute)
	for {
		moved := false
		for role, p := range parties {
			out, err := p.Outbound()
			require.NoError(t, err)
			for _, msg := range out {
				moved = true
				for other, dst := range parties {
					if other == role {
						continue
					}
					if !msg.Broadcast && !addressed(msg, other) {
						continue
					}
					require.NoError(t, dst.Handle(msg))
				}
			}
		}
		_, deviceDone := device.Signature()
		_, custodianDone := custodian.Signature()
		if deviceDone && custodianDone {
			break
		}
		require.False(t, time.Now().After(deadline), "the two parties never finished")
		if !moved {
			time.Sleep(20 * time.Millisecond)
		}
	}

	a, ok := device.Signature()
	require.True(t, ok)
	b, ok := custodian.Signature()
	require.True(t, ok)

	// Both compute the identical bytes, which is what lets the device check
	// what it is about to broadcast against its own copy rather than trusting
	// the custodian to hand back the signature it helped make.
	require.Equal(t, a, b, "the two parties disagree about the signature")

	pub, err := device.PublicKey()
	require.NoError(t, err)
	require.True(t, verify(t, pub, digest, a))

	pk, err := mpccosmos.CosmosPubKey(pub)
	require.NoError(t, err)
	require.True(t, pk.VerifySignature([]byte("MsgSendPayment, signed across a network"), a),
		"the chain would reject a signature produced across the wire")
}

func addressed(msg mpc.Outbound, role string) bool {
	for _, to := range msg.To {
		if to == role {
			return true
		}
	}
	return false
}

// A share presented under the wrong role is refused before a single message is
// sent. Otherwise a custodian could start a session claiming its share is the
// device's, and the failure would surface as a hang rather than a refusal.
func TestAShareCannotImpersonateAnotherRole(t *testing.T) {
	s := accountShares(t)

	_, err := mpc.NewSigningParty(
		mpc.RoleDevice, hash("x"), s[mpc.RoleCustodian],
		[]string{mpc.RoleDevice, mpc.RoleCustodian})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not carry that role")
}

// A reshare must not destroy the shares it was given.
//
// tss-lib retires an outgoing party by zeroing its Xi in place, and
// LocalPartySaveData is pointers all the way down — so passing the caller's
// share straight in gutted it. This was invisible when the reshare test ran
// alone and only appeared as a LATER, unrelated test failing, which is the
// worst way to find anything.
//
// It matters far more in production than in a suite: a reshare that dies
// halfway would leave the custodian holding a dead share, and an account whose
// surviving shares can no longer reach the threshold is one nobody can ever
// spend from again.
func TestAReshareLeavesTheOriginalSharesUsable(t *testing.T) {
	s := accountShares(t)

	_, err := mpc.Reshare(map[string]mpc.Share{
		mpc.RoleRecovery:  s[mpc.RoleRecovery],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	}, preParams())
	require.NoError(t, err)

	// The originals still sign. Until the caller stores the new set and retires
	// these deliberately, they have to keep working — that decision is the
	// account service's, not the protocol's.
	digest := hash("the old sharing still works until it is retired")
	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:    s[mpc.RoleDevice],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	})
	require.NoError(t, err, "the reshare consumed the shares it was handed")
	require.True(t, verify(t, pub, digest, sig))
}

// A share retired by a reshare signs nothing, and says so instead of panicking.
//
// Found by running tools/mpc rather than reading it. Signing with an OLD device
// share and a NEW custodian share crashed inside tss-lib's party construction:
//
//	panic: BuildLocalSaveDataSubset: unable to find a signer party in the local
//	save data
//
// The refusal was right; the delivery was not. A panic in a signing path is a
// crashed process rather than a rejected request, and in the custodian it would
// be a denial of service reachable by anybody holding a stale share.
//
// It is not an exotic pairing either. A phone that lost its network halfway
// through a password reset holds exactly the retired share while the custodian
// holds the new one, and the next payment that phone attempts is this
// combination.
func TestAShareRetiredByAReshareRefusesRatherThanPanics(t *testing.T) {
	s := accountShares(t)

	fresh, err := mpc.Reshare(map[string]mpc.Share{
		mpc.RoleRecovery:  s[mpc.RoleRecovery],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	}, preParams())
	require.NoError(t, err)

	// Same account — the reshare kept the public key, which is exactly why
	// these two look compatible and are not.
	oldPub, err := s[mpc.RoleDevice].PublicKey()
	require.NoError(t, err)
	newPub, err := fresh[mpc.RoleCustodian].PublicKey()
	require.NoError(t, err)
	require.True(t, oldPub.Equal(newPub), "the reshare moved the account, which is a different bug")

	_, _, err = mpc.Sign(hash("a payment from a phone that missed the reset"), map[string]mpc.Share{
		mpc.RoleDevice:    s[mpc.RoleDevice],        // retired
		mpc.RoleCustodian: fresh[mpc.RoleCustodian], // current
	})
	require.Error(t, err, "a retired share signed alongside a current one")
	require.Contains(t, err.Error(), "different sharings")

	// And the pairing that should work still does.
	_, _, err = mpc.Sign(hash("a payment after the reset completed"), map[string]mpc.Share{
		mpc.RoleDevice:    fresh[mpc.RoleDevice],
		mpc.RoleCustodian: fresh[mpc.RoleCustodian],
	})
	require.NoError(t, err)
}

// The same mismatch across a transport, which is what production actually runs
// and what the reviewer flagged as untested.
//
// NewSigningParty builds its committee from its OWN share's key list, so
// neither side can detect the mismatch locally — each one is internally
// consistent and believes the other is on its sharing. The failure therefore
// has to surface when a message arrives, and the two outcomes worth excluding
// are a hang (the custodian holding a session open forever) and, far worse, a
// signature that verifies against nothing.
//
// This asserts which of those it is, so that a change to tss-lib or to the
// party cannot quietly turn a clean error into either.
func TestPartiesOnDifferentSharingsFailFastAndProduceNoSignature(t *testing.T) {
	s := accountShares(t)
	fresh, err := mpc.Reshare(map[string]mpc.Share{
		mpc.RoleRecovery:  s[mpc.RoleRecovery],
		mpc.RoleCustodian: s[mpc.RoleCustodian],
	}, preParams())
	require.NoError(t, err)

	digest := hash("across a transport, with a stale share")
	committee := []string{mpc.RoleDevice, mpc.RoleCustodian}

	stale, err := mpc.NewSigningParty(mpc.RoleDevice, digest, s[mpc.RoleDevice], committee)
	require.NoError(t, err, "a party cannot tell locally that its peer has moved on")
	current, err := mpc.NewSigningParty(mpc.RoleCustodian, digest, fresh[mpc.RoleCustodian], committee)
	require.NoError(t, err)

	parties := map[string]*mpc.SigningParty{mpc.RoleDevice: stale, mpc.RoleCustodian: current}

	var failed bool
	deadline := time.Now().Add(90 * time.Second)
	for !failed && time.Now().Before(deadline) {
		moved := false
		for role, p := range parties {
			out, err := p.Outbound()
			if err != nil {
				failed = true
				break
			}
			for _, msg := range out {
				moved = true
				other := mpc.RoleCustodian
				if role == mpc.RoleCustodian {
					other = mpc.RoleDevice
				}
				if !msg.Broadcast && !addressed(msg, other) {
					continue
				}
				if err := parties[other].Handle(msg); err != nil {
					failed = true
					break
				}
			}
			if failed {
				break
			}
		}
		if !moved && !failed {
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Whatever the protocol does about it, the outcome that matters is that
	// nobody ends up holding a signature.
	for role, p := range parties {
		if sig, done := p.Signature(); done {
			t.Fatalf("%s produced a signature from mismatched sharings: %x", role, sig)
		}
	}
	require.True(t, failed, "mismatched sharings neither errored nor finished; this is the hang case")
}
