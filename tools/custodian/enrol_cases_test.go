package main

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// preFixture generates the safe primes once for this file: one set per role.
//
// Separate sets, not one reused three times. Two parties sharing a Paillier key
// is a real cryptographic error, and a fixture that made it would hide it.
var (
	preFixtureOnce sync.Once
	preFixture     map[string]*keygen.LocalPreParams
	preFixtureErr  error
)

func preFor(t *testing.T, role string) *keygen.LocalPreParams {
	t.Helper()
	if testing.Short() {
		t.Skip("distributed key generation takes minutes; run without -short")
	}
	preFixtureOnce.Do(func() {
		preFixture = map[string]*keygen.LocalPreParams{}
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, r := range mpc.Roles {
			wg.Add(1)
			go func(r string) {
				defer wg.Done()
				p, err := mpc.GeneratePreParams(mpc.KeygenTimeout)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					preFixtureErr = err
					return
				}
				preFixture[r] = p
			}(r)
		}
		wg.Wait()
	})
	if preFixtureErr != nil {
		t.Fatalf("generating pre-parameters: %v", preFixtureErr)
	}
	return preFixture[role]
}

// TestEnrolmentProducesAnAccountThatSigns is the whole point: a person with a
// browser ends up with an account, and neither service can move its money
// without that person's device.
func TestEnrolmentProducesAnAccountThatSigns(t *testing.T) {
	const email = "amara@example.org"
	custodian := newEnrolService(t, mpc.RoleCustodian, preFor(t, mpc.RoleCustodian))
	recovery := newEnrolService(t, mpc.RoleRecovery, preFor(t, mpc.RoleRecovery))

	device, err := mpc.NewKeygenParty(mpc.RoleDevice, preFor(t, mpc.RoleDevice))
	if err != nil {
		t.Fatalf("device party: %v", err)
	}

	deviceShare, address := enrol(t, custodian, recovery, device, email, "correct horse battery staple")
	if !strings.HasPrefix(address, "yml1") {
		t.Fatalf("expected a yml address, got %q", address)
	}

	// Both services stored the account under the same address, and each holds
	// its OWN share — which the store re-checks on every read.
	for _, svc := range []*enrolService{custodian, recovery} {
		acct, err := svc.server.store.Get(svc.server.index.Of(email))
		if err != nil {
			t.Fatalf("%s did not store the account: %v", svc.role, err)
		}
		if acct.Address != address {
			t.Fatalf("%s stored %s, the device computed %s", svc.role, acct.Address, address)
		}
		share, err := svc.server.store.Share(acct)
		if err != nil {
			t.Fatalf("%s cannot open its share: %v", svc.role, err)
		}
		if share.Role != svc.role {
			t.Fatalf("%s holds a %s share", svc.role, share.Role)
		}
	}

	// The account signs with device + custodian, which is the pair a payment
	// uses. An enrolment producing a key nobody can sign with would pass every
	// check above.
	custAcct, _ := custodian.server.store.Get(custodian.server.index.Of(email))
	custShare, err := custodian.server.store.Share(custAcct)
	if err != nil {
		t.Fatalf("custodian share: %v", err)
	}
	digest := make([]byte, 32)
	for i := range digest {
		digest[i] = byte(i * 3)
	}
	sig, pub, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:    deviceShare,
		mpc.RoleCustodian: custShare,
	})
	if err != nil {
		t.Fatalf("signing with the enrolled account: %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("expected 64 signature bytes, got %d", len(sig))
	}
	got, err := mpccosmos.Address(pub)
	if err != nil {
		t.Fatalf("address of the signing key: %v", err)
	}
	if got != address {
		t.Fatalf("the signature is for %s, the account is %s", got, address)
	}

	// And device + recovery signs too, which is what makes losing a phone
	// survivable rather than final.
	recAcct, _ := recovery.server.store.Get(recovery.server.index.Of(email))
	recShare, err := recovery.server.store.Share(recAcct)
	if err != nil {
		t.Fatalf("recovery share: %v", err)
	}
	if _, _, err := mpc.Sign(digest, map[string]mpc.Share{
		mpc.RoleDevice:   deviceShare,
		mpc.RoleRecovery: recShare,
	}); err != nil {
		t.Fatalf("signing with device + recovery: %v", err)
	}
}

// TestNeitherStoreWillHoldTheOthersShare is the structural half of "the
// operator cannot move your money".
//
// Two of three does mathematically sign — that is what 2-of-3 means, and
// pretending otherwise would be a lie. What must hold is that no single
// operator ends up with both, which is why these are two deployments with two
// sealing keys and two directories. So what is asserted is the thing that is
// actually protective: neither store will open the other's share, and neither
// will accept it if something tries to write it there.
func TestNeitherStoreWillHoldTheOthersShare(t *testing.T) {
	const email = "bakary@example.org"
	custodian := newEnrolService(t, mpc.RoleCustodian, preFor(t, mpc.RoleCustodian))
	recovery := newEnrolService(t, mpc.RoleRecovery, preFor(t, mpc.RoleRecovery))
	device, err := mpc.NewKeygenParty(mpc.RoleDevice, preFor(t, mpc.RoleDevice))
	if err != nil {
		t.Fatalf("device party: %v", err)
	}
	enrol(t, custodian, recovery, device, email, "a long enough passphrase")

	custAcct, _ := custodian.server.store.Get(custodian.server.index.Of(email))
	recAcct, _ := recovery.server.store.Get(recovery.server.index.Of(email))
	custShare, err := custodian.server.store.Share(custAcct)
	if err != nil {
		t.Fatalf("custodian share: %v", err)
	}
	recShare, err := recovery.server.store.Share(recAcct)
	if err != nil {
		t.Fatalf("recovery share: %v", err)
	}

	if _, err := custodian.server.store.Share(recAcct); err == nil {
		t.Fatal("the custodian store opened an account sealed by the recovery service")
	}
	if _, err := recovery.server.store.Share(custAcct); err == nil {
		t.Fatal("the recovery store opened an account sealed by the custodian")
	}
	if err := custodian.server.store.Put(custAcct, recShare); err == nil {
		t.Fatal("the custodian store accepted a recovery share")
	}
	if err := recovery.server.store.Put(recAcct, custShare); err == nil {
		t.Fatal("the recovery store accepted a custodian share")
	}
}

// TestEnrolmentRefusesAnExistingEmail is the check that makes this endpoint
// safe to expose. Without it anybody could enrol over somebody else's account
// and replace the share that moves their money.
func TestEnrolmentRefusesAnExistingEmail(t *testing.T) {
	const email = "chidi@example.org"
	custodian := newEnrolService(t, mpc.RoleCustodian, preFor(t, mpc.RoleCustodian))
	recovery := newEnrolService(t, mpc.RoleRecovery, preFor(t, mpc.RoleRecovery))
	device, err := mpc.NewKeygenParty(mpc.RoleDevice, preFor(t, mpc.RoleDevice))
	if err != nil {
		t.Fatalf("device party: %v", err)
	}
	enrol(t, custodian, recovery, device, email, "another long passphrase")

	if code := custodian.post(t, "/v1/enrol/start",
		enrolStartRequest{Email: email, Password: "yet another passphrase"}, nil,
	); code != http.StatusConflict {
		t.Fatalf("expected 409 for an email already enrolled, got %d", code)
	}

	// Case and surrounding space must not open a second account for one person
	// — that would not overwrite anything, which is worse: it would silently
	// give them a second account and half their money would be in it.
	if code := custodian.post(t, "/v1/enrol/start",
		enrolStartRequest{Email: "  Chidi@Example.org  ", Password: "yet another passphrase"}, nil,
	); code != http.StatusConflict {
		t.Fatalf("expected 409 for the same email differently cased, got %d", code)
	}
}

// TestEnrolmentCommitsNothingUntilItFinishes: a generation that is abandoned,
// or finished with a claim that does not match, must leave no account behind.
func TestEnrolmentCommitsNothingUntilItFinishes(t *testing.T) {
	const email = "eshe@example.org"
	custodian := newEnrolService(t, mpc.RoleCustodian, preFor(t, mpc.RoleCustodian))

	var start enrolStartResponse
	if code := custodian.post(t, "/v1/enrol/start",
		enrolStartRequest{Email: email, Password: "a sufficiently long one"}, &start,
	); code != http.StatusOK {
		t.Fatalf("start: %d", code)
	}
	if start.Role != mpc.RoleCustodian {
		t.Fatalf("the service reported role %q", start.Role)
	}

	// Finishing a generation that never ran must fail, and must write nothing.
	// Checked explicitly so a future change that makes finish lenient about
	// completion cannot quietly start committing half-built accounts.
	if code := custodian.post(t, "/v1/enrol/finish",
		enrolFinishRequest{Session: start.Session, Address: "yml1whatever"}, nil,
	); code == http.StatusOK {
		t.Fatal("an unfinished enrolment was committed")
	}
	if _, err := custodian.server.store.Get(custodian.server.index.Of(email)); err == nil {
		t.Fatal("an account was written for an enrolment that never finished")
	}
}

// TestEnrolmentRefusesAShortPassword. The password is not one factor among
// several here: a thief holding the phone already has the device share.
func TestEnrolmentRefusesAShortPassword(t *testing.T) {
	custodian := newEnrolService(t, mpc.RoleCustodian, preFor(t, mpc.RoleCustodian))
	if code := custodian.post(t, "/v1/enrol/start",
		enrolStartRequest{Email: "fatou@example.org", Password: "short"}, nil,
	); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a short password, got %d", code)
	}
	if n := custodian.server.enrolments.Open(); n != 0 {
		t.Fatalf("a refused enrolment left %d sessions open", n)
	}
}

// TestPreParamPoolRefusesWhenEmpty guards the change somebody will be tempted
// to make: blocking until parameters are ready, which turns a clean refusal
// into an unbounded queue whose length an attacker sets.
func TestPreParamPoolRefusesWhenEmpty(t *testing.T) {
	blocked := make(chan struct{})
	var closeOnce sync.Once
	pool := NewPreParamPool(1, func() (*keygen.LocalPreParams, error) {
		<-blocked // never produces
		return nil, nil
	})
	t.Cleanup(func() {
		closeOnce.Do(func() { close(blocked) })
		pool.Close()
	})

	done := make(chan error, 1)
	go func() {
		_, err := pool.Take()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an empty pool handed out parameters")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Take blocked on an empty pool instead of refusing")
	}
}

// TestStoreRefusesTheDeviceRole. A service holding the device's share is a
// service that can sign alone, so it is refused at construction.
func TestStoreRefusesTheDeviceRole(t *testing.T) {
	if _, err := NewStore(t.TempDir(), mpc.RoleDevice, []byte(strings.Repeat("k", 32))); err == nil {
		t.Fatal("a store was created for the device's share")
	}
}
