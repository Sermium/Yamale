package mpc_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/stretchr/testify/require"

	_ "yamale/blockchain/app" // its init() seals the yml bech32 prefix
	"yamale/blockchain/mpc"
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
	genErr error
)

func accountShares(t *testing.T) map[string]mpc.Share {
	t.Helper()
	if testing.Short() {
		t.Skip("distributed key generation takes minutes; run without -short")
	}
	once.Do(func() {
		pre := make(map[string]*keygen.LocalPreParams, len(mpc.Roles))
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
		addr, err := mpc.Address(pub)
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

	pk, err := mpc.CosmosPubKey(pub)
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
