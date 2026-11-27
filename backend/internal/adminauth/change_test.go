package adminauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func changeRequest(address string) *http.Request {
	r := httptest.NewRequest(http.MethodPut, "/api/admin/security/totp", nil)
	r.RemoteAddr = address + ":4000"
	return r
}

func TestAStoredSecretTurnsTheCodeOnAndTheEnvironmentsWins(t *testing.T) {
	auth, _ := testAuth(t, "production", "synthetic-admin-password")
	if auth.RequiresCode() || auth.EnvironmentCode() {
		t.Fatal("no secret asks for a code")
	}
	if err := auth.SetStoredSecret([]byte("short"), 0); err != ErrTOTPSecret {
		t.Fatalf("a short secret = %v", err)
	}
	if err := auth.SetStoredSecret(rfcKey, 0); err != nil || !auth.RequiresCode() {
		t.Fatalf("stored err=%v asked=%v", err, auth.RequiresCode())
	}
	if err := auth.SetStoredSecret(nil, 0); err != nil || auth.RequiresCode() {
		t.Fatal("clearing the stored secret kept the code")
	}
	env, _ := codeAuth(t)
	other := []byte("abcdefghijklmnopqrst")
	_ = env.SetStoredSecret(other, 0)
	if !env.EnvironmentCode() || string(env.secretLocked()) != string(rfcKey) {
		t.Fatal("the environment's secret must win over a stored one")
	}
}

func TestVerifyChangeChecksBothFactorsAndSpendsTheCode(t *testing.T) {
	auth, now := codeAuth(t)
	current := totpCode(rfcKey, totpStep(*now))
	if err := auth.VerifyChange(changeRequest("192.0.2.1"), "wrong-password", current, true); err != ErrInvalidCredentials {
		t.Fatalf("wrong password = %v", err)
	}
	if err := auth.VerifyChange(changeRequest("192.0.2.1"), "synthetic-admin-password", "", true); err != ErrInvalidCredentials {
		t.Fatalf("missing code = %v", err)
	}
	// Without the code asked for, the password alone passes.
	if err := auth.VerifyChange(changeRequest("192.0.2.1"), "synthetic-admin-password", "", false); err != nil {
		t.Fatalf("password only = %v", err)
	}
	if err := auth.VerifyChange(changeRequest("192.0.2.1"), "synthetic-admin-password", current, true); err != nil {
		t.Fatalf("both factors = %v", err)
	}
	if err := auth.VerifyChange(changeRequest("192.0.2.1"), "synthetic-admin-password", current, true); err != ErrInvalidCredentials {
		t.Fatal("a spent code passed again")
	}
	for range 5 {
		_ = auth.VerifyChange(changeRequest("192.0.2.9"), "wrong-password", "", false)
	}
	if err := auth.VerifyChange(changeRequest("192.0.2.9"), "synthetic-admin-password", "", false); err != ErrTooManyAttempts {
		t.Fatalf("an address past its limit = %v", err)
	}
	dev, _ := testAuth(t, "development", "")
	if err := dev.VerifyChange(changeRequest("192.0.2.1"), "", "", true); err != nil {
		t.Fatalf("development = %v", err)
	}
}

func TestEndOtherSessionsKeepsTheRequestsSession(t *testing.T) {
	auth, _ := testAuth(t, "production", "synthetic-admin-password")
	first := codeLogin(auth, "192.0.2.1", `{"password":"synthetic-admin-password"}`).Result().Cookies()[0]
	codeLogin(auth, "192.0.2.2", `{"password":"synthetic-admin-password"}`)
	r := changeRequest("192.0.2.1")
	r.AddCookie(first)
	auth.EndOtherSessions(r)
	if len(auth.sessions) != 1 || !auth.authenticated(r) {
		t.Fatalf("sessions=%d", len(auth.sessions))
	}
}
