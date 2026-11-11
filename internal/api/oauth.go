package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const githubStateCookie = "gh_oauth_state"

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
	if err := getJSON(client, "https://api.github.com/user", &ghUser); err != nil {
		jsonError(w, "failed to fetch github profile", http.StatusBadGateway)
		return
	}

	email := ghUser.Email
	if email == "" {
		var emails []githubEmail
		if err := getJSON(client, "https://api.github.com/user/emails", &emails); err == nil {
			for _, e := range emails {
				if e.Primary && e.Verified {
					email = e.Email
					break
				}
			}
		}
	}

	providerUserID := strconv.Itoa(ghUser.ID)
	user, err := s.store.FindOrCreateOAuthUser("github", providerUserID, email, ghUser.Login, ghUser.AvatarURL)
	if err != nil {
		s.storeError(w, r, err, "create account")
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

func getJSON(client *http.Client, url string, out interface{}) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
