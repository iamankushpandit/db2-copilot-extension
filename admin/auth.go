package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

const (
	sessionCookieName  = "db_copilot_admin"
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserAPIURL   = "https://api.github.com/user"
)

// sessionPayload is the data stored in the signed session cookie.
type sessionPayload struct {
	Username string `json:"u"`
	Expires  int64  `json:"e"`
}

// AuthService manages admin authentication via GitHub OAuth and signed cookies.
type AuthService struct {
	clientID     string
	clientSecret string
	fqdn         string
	secret       []byte
	cfgMgr       *config.Manager
}

// NewAuthService creates an AuthService.
// sessionSecret is used to sign cookies; if empty a random secret is generated
// (sessions will not survive a server restart).
func NewAuthService(clientID, clientSecret, fqdn, sessionSecret string, cfgMgr *config.Manager) *AuthService {
	var secret []byte
	if sessionSecret != "" {
		secret = []byte(sessionSecret)
	} else {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			// Fall back to a deterministic but weak secret when rand fails.
			secret = []byte("db-copilot-admin-default-secret")
		}
	}
	return &AuthService{
		clientID:     clientID,
		clientSecret: clientSecret,
		fqdn:         fqdn,
		secret:       secret,
		cfgMgr:       cfgMgr,
	}
}

// RequireAdmin is middleware that redirects unauthenticated users to the login
// page and denies access if the authenticated user is not in the allowed list.
func (a *AuthService) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, ok := a.sessionUsername(r)
		if !ok {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}

		adminCfg := a.cfgMgr.Admin()
		if !adminCfg.IsAdminUser(username) {
			http.Error(w, "Access denied: you are not in the admin user list.", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

// HandleLogin initiates the GitHub OAuth flow for admin access.
func (a *AuthService) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	setStateCookie(w, r, state)

	params := url.Values{}
	params.Set("client_id", a.clientID)
	params.Set("redirect_uri", a.fqdn+"/admin/callback")
	params.Set("scope", "read:user")
	params.Set("state", state)

	http.Redirect(w, r, githubAuthorizeURL+"?"+params.Encode(), http.StatusFound)
}

// HandleCallback processes the GitHub OAuth callback for admin.
func (a *AuthService) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate CSRF state.
	state := r.URL.Query().Get("state")
	if !validateStateCookie(r, state) {
		http.Error(w, "Invalid or missing state parameter (possible CSRF)", http.StatusBadRequest)
		return
	}
	clearStateCookie(w, r)

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for token.
	token, err := a.exchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange authorization code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch GitHub username.
	username, err := a.fetchUsername(r.Context(), token)
	if err != nil {
		http.Error(w, "Failed to fetch GitHub user info: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check allowlist.
	adminCfg := a.cfgMgr.Admin()
	if !adminCfg.IsAdminUser(username) {
		// Show login page with error.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `<!DOCTYPE html><html><body>
<p>Access denied: GitHub user <strong>%s</strong> is not in the admin user list.</p>
<p>Ask an existing admin to add your username to <code>admin_config.json → admin_ui.allowed_github_users</code>.</p>
<a href="/admin/login">← Try again</a>
</body></html>`, username)
		return
	}

	// Set session cookie.
	if err := a.setSession(w, r, username, adminCfg.AdminUI.SessionTimeoutHours); err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}

// HandleLogout clears the session cookie and redirects to the login page.
func (a *AuthService) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		Secure:   r.TLS != nil,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// SessionUsername returns the authenticated username from the session cookie.
// It is exported for use by handlers that need the logged-in user's name.
func (a *AuthService) SessionUsername(r *http.Request) (string, bool) {
	return a.sessionUsername(r)
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────────────────────

func (a *AuthService) sessionUsername(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	payload, err := a.verifyToken(cookie.Value)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() > payload.Expires {
		return "", false
	}
	return payload.Username, true
}

func (a *AuthService) setSession(w http.ResponseWriter, r *http.Request, username string, timeoutHours int) error {
	if timeoutHours <= 0 {
		timeoutHours = 24
	}
	payload := sessionPayload{
		Username: username,
		Expires:  time.Now().Add(time.Duration(timeoutHours) * time.Hour).Unix(),
	}
	token, err := a.signToken(payload)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/admin",
		MaxAge:   timeoutHours * 3600,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// signToken serialises the payload to JSON, appends an HMAC-SHA256 signature,
// and returns the result as a base64url-encoded string.
func (a *AuthService) signToken(p sessionPayload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(data)
	sig := mac.Sum(nil)

	// Concatenate payload + sig, then base64url-encode.
	combined := append(data, sig...)
	return base64.RawURLEncoding.EncodeToString(combined), nil
}

// verifyToken decodes and verifies a signed token produced by signToken.
func (a *AuthService) verifyToken(raw string) (*sessionPayload, error) {
	combined, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(combined) < sha256.Size {
		return nil, fmt.Errorf("token too short")
	}

	data := combined[:len(combined)-sha256.Size]
	sig := combined[len(combined)-sha256.Size:]

	mac := hmac.New(sha256.New, a.secret)
	mac.Write(data)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return nil, fmt.Errorf("invalid signature")
	}

	var p sessionPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &p, nil
}

// exchangeCode trades an OAuth code for an access token.
func (a *AuthService) exchangeCode(ctx context.Context, code string) (string, error) {
	params := url.Values{}
	params.Set("client_id", a.clientID)
	params.Set("client_secret", a.clientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", a.fqdn+"/admin/callback")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("GitHub OAuth error: %s", result.Error)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}
	return result.AccessToken, nil
}

// fetchUsername calls the GitHub /user API to get the authenticated user's login.
func (a *AuthService) fetchUsername(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("decoding user response: %w", err)
	}
	if user.Login == "" {
		return "", fmt.Errorf("empty login in GitHub user response")
	}
	return user.Login, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// CSRF state cookie helpers
// ──────────────────────────────────────────────────────────────────────────────

const stateCookieName = "db_copilot_admin_state"

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func setStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/admin",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func validateStateCookie(r *http.Request, state string) bool {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(cookie.Value), []byte(state))
}

func clearStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		Secure:   r.TLS != nil,
		HttpOnly: true,
	})
}
