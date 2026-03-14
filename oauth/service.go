package oauth

import (
	"fmt"
	"net/http"
	"net/url"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
)

// Service handles the GitHub OAuth flow for the Copilot Extension.
type Service struct {
	clientID     string
	clientSecret string
	fqdn         string
}

// NewService creates a new OAuth service.
func NewService(clientID, clientSecret, fqdn string) *Service {
	return &Service{
		clientID:     clientID,
		clientSecret: clientSecret,
		fqdn:         fqdn,
	}
}

// PreAuth handles GET /auth/authorization.
// It redirects the user to GitHub's OAuth authorization page.
func (s *Service) PreAuth(w http.ResponseWriter, r *http.Request) {
	redirectURI := s.fqdn + "/auth/callback"

	params := url.Values{}
	params.Set("client_id", s.clientID)
	params.Set("redirect_uri", redirectURI)

	authURL := githubAuthorizeURL + "?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// PostAuth handles GET /auth/callback.
// It exchanges the authorization code for an access token and shows a
// success page to the user.
func (s *Service) PostAuth(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	tokenParams := url.Values{}
	tokenParams.Set("client_id", s.clientID)
	tokenParams.Set("client_secret", s.clientSecret)
	tokenParams.Set("code", code)

	resp, err := http.PostForm(githubTokenURL, tokenParams)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to exchange code for token: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>DB2 Copilot Extension – Authorization Successful</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; background: #f6f8fa; }
    .card { background: #fff; border-radius: 8px; padding: 40px; box-shadow: 0 1px 3px rgba(0,0,0,.12); text-align: center; max-width: 480px; }
    h1 { color: #1a7f37; }
    p { color: #57606a; line-height: 1.6; }
  </style>
</head>
<body>
  <div class="card">
    <h1>&#10003; Authorization Successful</h1>
    <p>You have successfully authorized the <strong>DB2 Copilot Extension</strong>.</p>
    <p>You can now close this tab and return to GitHub Copilot Chat or Microsoft Teams to start querying your DB2 database.</p>
  </div>
</body>
</html>`)
}
