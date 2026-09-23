package main

import (
	"encoding/base32"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The second factor, through the endpoints.
//
// factors_test.go covers the ladder's arithmetic. This covers the thing that
// was missing until now: that any of it is reachable, and that a recovery
// actually demands it. A well-tested function nothing calls is not a control.

func decodeBase32(t *testing.T, s string) []byte {
	t.Helper()
	out, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	require.NoError(t, err)
	return out
}

func factorFixture(t *testing.T) (*recoveryFixture, string, string) {
	t.Helper()
	const email = "amara@example.org"
	const password = "correct horse battery staple"
	f := newRecoveryFixture(t)
	f.enrolAccount(t, email, password)
	return f, email, password
}

func TestEnrollingAnAuthenticatorTakesTwoSteps(t *testing.T) {
	f, email, password := factorFixture(t)

	// Nothing enrolled: the floor.
	var state factorState
	require.Equal(t, http.StatusOK,
		f.post(t, "/v1/factors", factorRequest{Email: email, Password: password}, &state))
	require.Equal(t, string(FactorEmail), state.Strongest)
	require.False(t, state.TOTP)

	var begun totpBegin
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/begin",
		factorRequest{Email: email, Password: password}, &begun))
	require.NotEmpty(t, begun.Secret)
	require.Contains(t, begun.URI, "otpauth://totp/Yamale:"+email)

	// THE POINT. A secret has been issued and the account is unchanged, because
	// there is no way to know the QR was ever scanned. An enrolment abandoned
	// here must not arm a factor its owner cannot produce — they would discover
	// that at a recovery, which is the worst possible moment.
	require.Equal(t, http.StatusOK,
		f.post(t, "/v1/factors", factorRequest{Email: email, Password: password}, &state))
	require.Equal(t, string(FactorEmail), state.Strongest, "an unconfirmed secret armed the factor")
	require.False(t, state.TOTP)

	// A wrong code does not confirm, and does not discard the enrolment either:
	// a mistyped code is the common case.
	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/factors/totp/confirm",
		factorRequest{Email: email, Password: password, Code: "000000"}, nil))

	secret := decodeBase32(t, begun.Secret)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/factors/totp/confirm",
		factorRequest{Email: email, Password: password, Code: totpNow(secret, *f.clock)}, &state))
	require.Equal(t, string(FactorTOTP), state.Strongest)
	require.True(t, state.TOTP)
}

func TestAPasswordAloneCannotReplaceAConfirmedAuthenticator(t *testing.T) {
	// The password is one of the two things a thief has. If it were enough to
	// swap the authenticator, the factor would defend against nothing: the
	// thief would simply enrol their own and walk through the recovery.
	f, email, password := factorFixture(t)
	f.armTOTP(t, email)

	code, reason := f.postWithBody(t, "/v1/factors/totp/begin",
		factorRequest{Email: email, Password: password})
	require.Equal(t, http.StatusConflict, code)
	require.Contains(t, reason, "already has an authenticator")
}

func TestFactorEndpointsNeedThePassword(t *testing.T) {
	f, email, _ := factorFixture(t)
	for _, path := range []string{"/v1/factors", "/v1/factors/totp/begin", "/v1/factors/totp/confirm"} {
		require.Equal(t, http.StatusUnauthorized,
			f.post(t, path, factorRequest{Email: email, Password: "wrong"}, nil), path)
	}
}

/* ------------------------------------------- the factor gates the claim */

// completedRecovery drives a recovery to the point where a holding is claimable.
func completedRecovery(t *testing.T, f *recoveryFixture, email string) string {
	t.Helper()
	var rec recoveryView
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/initiate", recoveryInitiateRequest{
		Email: email, Operator: "amina", Team: "support", Proof: "in person",
	}, &rec))
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "boubacar", Team: "risk"}, nil))
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/approve",
		recoveryApproveRequest{ID: rec.ID, Operator: "chinwe", Team: "compliance"}, nil))
	*f.clock = f.clock.Add(RecoveryDelay + time.Minute)
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/complete",
		recoveryIDRequest{ID: rec.ID, Email: email}, nil))
	*f.clock = f.clock.Add(PostRecoveryFreeze + time.Minute)
	return rec.ID
}

func TestAClaimWithoutTheFactorIsRefused(t *testing.T) {
	f, email, password := factorFixture(t)
	secret := f.armTOTP(t, email)
	id := completedRecovery(t, f, email)

	// Everything else is right: the recovery completed, the waiting period
	// passed, the password is correct. This is the account holder's own half
	// and nobody else can pass it.
	require.Equal(t, http.StatusBadRequest, f.post(t, "/v1/recovery/claim",
		claimRequest{ID: id, Email: email, Password: password}, nil),
		"a holding was released with no second factor at all")

	require.Equal(t, http.StatusUnauthorized, f.post(t, "/v1/recovery/claim",
		claimRequest{ID: id, Email: email, Password: password,
			Factor: string(FactorTOTP), Answer: "000000"}, nil),
		"a holding was released against a wrong code")

	var out claimResponse
	require.Equal(t, http.StatusOK, f.post(t, "/v1/recovery/claim",
		claimRequest{ID: id, Email: email, Password: password,
			Factor: string(FactorTOTP), Answer: totpNow(secret, *f.clock)}, &out))
	require.NotEmpty(t, out.Holding.Share.Y)
}

func TestAWeakerFactorIsRefusedEvenWhenItWouldVerify(t *testing.T) {
	// The downgrade. An account with an authenticator must not be claimable by
	// answering with the rung below it, or enrolling a factor would buy nothing
	// — an attacker would simply offer the weakest thing the service accepts.
	f, email, password := factorFixture(t)
	f.armTOTP(t, email)
	id := completedRecovery(t, f, email)

	code, reason := f.postWithBody(t, "/v1/recovery/claim", claimRequest{
		ID: id, Email: email, Password: password,
		Factor: string(FactorEmail), Answer: "123456",
	})
	require.Equal(t, http.StatusBadRequest, code)
	require.Contains(t, reason, string(FactorTOTP),
		"the refusal should name the factor this account actually has")
}

func TestAnAccountWithNoAuthenticatorCannotClaimHereAndIsToldWhy(t *testing.T) {
	// The honest state of this deployment: the emailed code is the floor, there
	// is no mailer, so an account that enrolled no authenticator cannot recover.
	//
	// It answers 503 rather than 401 because the caller did nothing wrong and no
	// answer would help. Telling them authentication failed would send them to
	// retype a password that is already correct.
	f, email, password := factorFixture(t)
	id := completedRecovery(t, f, email)

	code, reason := f.postWithBody(t, "/v1/recovery/claim", claimRequest{
		ID: id, Email: email, Password: password,
		Factor: string(FactorEmail), Answer: "123456",
	})
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Contains(t, reason, "no way to send one")
	require.Contains(t, reason, "/v1/factors/totp/begin")
}

// The notifier and the notify program, together.
//
// Every other test here uses a fake notifier. This one runs the real
// commandNotifier against the real tools/notify binary, because the contract
// between them is a JSON document on a pipe and an exit code — neither of which
// a fake exercises, and both of which are what a deployment depends on.
//
// It is also the test that found the bug: the notifier does not pass its
// environment to the child, so a notify program configured by an environment
// variable could never have been configured at all.
func TestTheRealNotifierDrivesTheRealNotifyProgram(t *testing.T) {
	bin := buildNotify(t)
	spool := t.TempDir()

	// Configured by FILE, which is the only channel that survives the notifier
	// deliberately stripping the environment.
	conf := filepath.Join(t.TempDir(), "notify.conf")
	require.NoError(t, os.WriteFile(conf, []byte("# the spool\n"+spool+"\n"), 0o600))
	t.Setenv("YAMALE_NOTIFY_CONF", conf)

	n := newCommandNotifier(bin)
	account := Account{Index: "idx-1", Address: "yml1abcdefghijklmn"}

	require.NoError(t, n.RecoveryStarted(account, "amara@example.org", RecoveryDelay),
		"the real notifier could not drive the real notify program")

	entries, err := os.ReadDir(spool)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no notice was written")

	body, err := os.ReadFile(filepath.Join(spool, entries[0].Name()))
	require.NoError(t, err)
	require.Contains(t, string(body), "amara@example.org")
	require.Contains(t, string(body), "72 hours")
	// The address is in the notice and not in the filename.
	require.NotContains(t, entries[0].Name(), "amara")

	// And a failure propagates: an unwritable spool must abort a recovery
	// rather than being logged and forgotten.
	//
	// The unwritable path is a directory UNDER A REGULAR FILE, which fails on
	// every platform. An absolute path like /does/not/exist is not unwritable
	// on Windows — it resolves against the current drive and MkdirAll creates
	// it happily, so the first version of this assertion passed on Linux and
	// quietly proved nothing here.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(conf, []byte(filepath.Join(blocker, "spool")+"\n"), 0o600))
	require.Error(t, n.RecoveryStarted(account, "amara@example.org", RecoveryDelay),
		"a notice that could not be written was reported as sent")
}

func buildNotify(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "notify.exe")
	cmd := exec.Command("go", "build", "-o", out, "yamale/blockchain/tools/notify")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the notify program here: %v: %s", err, combined)
	}
	return out
}
