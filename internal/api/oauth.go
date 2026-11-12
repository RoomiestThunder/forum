package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
)

const (
	githubStateCookie = "gh_oauth_state"
	defaultGitHubAPI  = "https://api.github.com"
)

// authProviders tells the SPA which "sign in with X" buttons to render -
// there's no point showing a GitHub button that 503s because the server
// doesn't have OAuth credentials configured.
func (s *Server) authProviders(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]bool{"github": s.githubOAuth != nil}, http.StatusOK)
}

func (s *Server) githubLogin(w http.ResponseWriter, r *http.Request) {
	if s.githubOAuth == nil {
		jsonError(w, "GitHub sign-in is not configured", http.StatusServiceUnavailable)
		return
	}
	stateBuf := make([]byte, 16)
	if _, err := rand.Read(stateBuf); err != nil {
		s.internalError(w, r, err, "start GitHub sign-in")
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBuf)

	http.SetCookie(w, &http.Cookie{
		Name:     githubStateCookie,
		Value:    state,
		Path:     "/api/v1/auth/github",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.githubOAuth.AuthCodeURL(state), http.StatusFound)
}

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	if s.githubOAuth == nil {
		jsonError(w, "GitHub sign-in is not configured", http.StatusServiceUnavailable)
		return
	}

	stateCookie, err := r.Cookie(githubStateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		jsonError(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: githubStateCookie, Value: "", Path: "/api/v1/auth/github", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		jsonError(w, "missing code", http.StatusBadRequest)
		return
	}

	token, err := s.githubOAuth.Exchange(r.Context(), code)
	if err != nil {
		jsonError(w, "oauth exchange failed", http.StatusBadGateway)
		return
	}
	client := s.githubOAuth.Client(r.Context(), token)

	var ghUser githubUser
	if err := getJSON(client, s.githubAPI+"/user", &ghUser); err != nil || ghUser.ID == 0 {
		if err != nil {
			s.logger.Warn("github profile fetch failed", slog.Any("err", err))
		}
		jsonError(w, "failed to fetch github profile", http.StatusBadGateway)
		return
	}

	// Only an address GitHub has verified is used. It decides which existing
	// account this sign-in is attached to, so an unverified one - which anyone can
	// type into their GitHub profile - would let them take over that account.
	// The public email on the profile is deliberately ignored for this reason.
	email := s.verifiedGitHubEmail(client)

	providerUserID := strconv.Itoa(ghUser.ID)
	user, err := s.store.FindOrCreateOAuthUser("github", providerUserID, email, ghUser.Login, ghUser.AvatarURL)
	if err != nil {
		s.storeError(w, r, err, "create account")
		return
	}
	if user.Banned {
		jsonError(w, "account suspended", http.StatusForbidden)
		return
	}

	tokens, err := s.issueTokens(user.ID, user.Username)
	if err != nil {
		s.storeError(w, r, err, "issue tokens")
		return
	}

	// The SPA can't read a JSON body from a full-page redirect, so the
	// tokens go in the URL fragment - browsers never send fragments back to
	// the server, so they don't end up in access logs or the Referer header.
	frag := url.Values{
		"access_token":  {tokens.AccessToken},
		"refresh_token": {tokens.RefreshToken},
		"user_id":       {strconv.Itoa(user.ID)},
		"username":      {user.Username},
	}
	http.Redirect(w, r, s.baseURL+"/oauth/callback#"+frag.Encode(), http.StatusFound)
}

// verifiedGitHubEmail returns the user's primary verified address, else any
// verified one, else "" - including when the lookup fails, in which case the
// sign-in simply can't be linked to an existing account.
func (s *Server) verifiedGitHubEmail(client *http.Client) string {
	var emails []githubEmail
	if err := getJSON(client, s.githubAPI+"/user/emails", &emails); err != nil {
		s.logger.Warn("github email lookup failed", slog.Any("err", err))
		return ""
	}
	fallback := ""
	for _, e := range emails {
		if !e.Verified || e.Email == "" {
			continue
		}
		if e.Primary {
			return e.Email
		}
		if fallback == "" {
			fallback = e.Email
		}
	}
	return fallback
}

// getJSON GETs url and decodes the JSON body into out. A non-200 response is an
// error: GitHub answers a bad or revoked token with a JSON error object, which
// would otherwise decode into an empty struct and look like a successful
// lookup of a user with id 0.
func getJSON(client *http.Client, url string, out interface{}) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
