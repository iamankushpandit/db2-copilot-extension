package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
)

// Service handles the GitHub App OAuth pre-authorization flow.
type Service struct {
	cfg *config.Config
}

// NewService creates a new OAuth service.
func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// HandleAuthorization redirects the user to GitHub for OAuth authorization.
// GitHub calls this endpoint before granting the agent access to the user's token.
func (s *Service) HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}

	params := url.Values{
		"client_id":    {s.cfg.ClientID},
		"redirect_uri": {s.cfg.FQDN + "/auth/callback"},
		"state":        {state},
	}

	http.Redirect(w, r, githubAuthorizeURL+"?"+params.Encode(), http.StatusFound)
}

// HandleCallback exchanges the authorization code for an access token.
func (s *Service) HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	token, err := s.exchangeCode(code)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to exchange code: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"access_token": token})
}

func (s *Service) exchangeCode(code string) (string, error) {
	params := url.Values{
		"client_id":     {s.cfg.ClientID},
		"client_secret": {s.cfg.ClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequest(http.MethodPost, githubTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("github oauth error: %s", tokenResp.Error)
	}

	return tokenResp.AccessToken, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
