package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// makeTestAuthService creates a minimal AuthService for testing.
// It uses an in-memory secret and does not depend on config files.
func makeTestAuthService() *AuthService {
	return &AuthService{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		fqdn:         "https://example.com",
		secret:       []byte("test-secret-key-for-unit-tests-!"),
		cfgMgr:       nil, // intentionally nil — tests that need cfgMgr will set it
	}
}

func TestSignAndVerifyToken(t *testing.T) {
	a := makeTestAuthService()

	payload := sessionPayload{
		Username: "testuser",
		Expires:  time.Now().Add(time.Hour).Unix(),
	}

	token, err := a.signToken(payload)
	if err != nil {
		t.Fatalf("signToken error: %v", err)
	}
	if token == "" {
		t.Fatal("signToken returned empty token")
	}

	got, err := a.verifyToken(token)
	if err != nil {
		t.Fatalf("verifyToken error: %v", err)
	}
	if got.Username != payload.Username {
		t.Errorf("Username = %q, want %q", got.Username, payload.Username)
	}
	if got.Expires != payload.Expires {
		t.Errorf("Expires = %d, want %d", got.Expires, payload.Expires)
	}
}

func TestVerifyToken_TamperedSignature(t *testing.T) {
	a := makeTestAuthService()

	payload := sessionPayload{
		Username: "testuser",
		Expires:  time.Now().Add(time.Hour).Unix(),
	}

	token, err := a.signToken(payload)
	if err != nil {
		t.Fatalf("signToken error: %v", err)
	}

	// Flip the last character to tamper with the signature.
	tampered := token[:len(token)-1] + "X"
	if tampered == token {
		tampered = token[:len(token)-1] + "Y"
	}

	_, err = a.verifyToken(tampered)
	if err == nil {
		t.Error("verifyToken should fail for tampered token")
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	a1 := makeTestAuthService()
	a2 := &AuthService{secret: []byte("different-secret-key-for-testing")}

	payload := sessionPayload{Username: "user", Expires: time.Now().Add(time.Hour).Unix()}
	token, _ := a1.signToken(payload)

	_, err := a2.verifyToken(token)
	if err == nil {
		t.Error("verifyToken should fail with a different secret")
	}
}

func TestVerifyToken_EmptyToken(t *testing.T) {
	a := makeTestAuthService()
	_, err := a.verifyToken("")
	if err == nil {
		t.Error("verifyToken should fail for empty token")
	}
}

func TestVerifyToken_InvalidBase64(t *testing.T) {
	a := makeTestAuthService()
	_, err := a.verifyToken("not-valid-base64!!!")
	if err == nil {
		t.Error("verifyToken should fail for invalid base64")
	}
}

func TestSessionUsername_NoCookie(t *testing.T) {
	a := makeTestAuthService()
	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	_, ok := a.sessionUsername(r)
	if ok {
		t.Error("sessionUsername should return false when no cookie is set")
	}
}

func TestSessionUsername_ValidCookie(t *testing.T) {
	a := makeTestAuthService()

	payload := sessionPayload{
		Username: "alice",
		Expires:  time.Now().Add(time.Hour).Unix(),
	}
	token, err := a.signToken(payload)
	if err != nil {
		t.Fatalf("signToken: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})

	username, ok := a.sessionUsername(r)
	if !ok {
		t.Error("sessionUsername returned false for valid cookie")
	}
	if username != "alice" {
		t.Errorf("username = %q, want %q", username, "alice")
	}
}

func TestSessionUsername_ExpiredCookie(t *testing.T) {
	a := makeTestAuthService()

	payload := sessionPayload{
		Username: "alice",
		Expires:  time.Now().Add(-time.Hour).Unix(), // already expired
	}
	token, _ := a.signToken(payload)

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})

	_, ok := a.sessionUsername(r)
	if ok {
		t.Error("sessionUsername should return false for expired session")
	}
}

func TestStateCookieRoundTrip(t *testing.T) {
	state := randomState()
	if len(state) == 0 {
		t.Fatal("randomState returned empty string")
	}

	w := httptest.NewRecorder()
	setStateCookie(w, state)

	// Simulate reading the cookie back.
	resp := w.Result()
	r := httptest.NewRequest(http.MethodGet, "/admin/callback?state="+state, nil)
	for _, c := range resp.Cookies() {
		r.AddCookie(c)
	}

	if !validateStateCookie(r, state) {
		t.Error("validateStateCookie should return true for matching state")
	}
	if validateStateCookie(r, "different-state") {
		t.Error("validateStateCookie should return false for mismatched state")
	}
}

func TestRandomState_Uniqueness(t *testing.T) {
	states := make(map[string]bool)
	for i := 0; i < 10; i++ {
		s := randomState()
		if states[s] {
			t.Errorf("randomState returned duplicate value: %s", s)
		}
		states[s] = true
	}
}
