package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"forum/internal/database"
)

// fakeGitHub answers the three calls the callback makes: the token exchange,
// /user and /user/emails. Each authorization code stands for one GitHub user.
type fakeGitHub struct {
	*httptest.Server

	mu       sync.Mutex
	profiles map[string]map[string]any // code -> /user body
	emails   map[string][]map[string]any
	// failEmails makes /user/emails answer 500; failProfile makes /user answer 401.
	failEmails, failProfile bool
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{profiles: map[string]map[string]any{}, emails: map[string][]map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-" + r.Form.Get("code"), "token_type": "bearer"})
	})
	code := func(r *http.Request) string { return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok-") }
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		profile, ok := g.profiles[code(r)]
		if g.failProfile || !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(profile)
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.failEmails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(g.emails[code(r)])
	})
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	return g
}

// user registers a GitHub user reachable through the given authorization code.
func (g *fakeGitHub) user(code string, id int, login, publicEmail string, emails ...map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.profiles[code] = map[string]any{"id": id, "login": login, "email": publicEmail, "avatar_url": ""}
	g.emails[code] = emails
}

func email(address string, primary, verified bool) map[string]any {
	return map[string]any{"email": address, "primary": primary, "verified": verified}
}

func newOAuthServer(t *testing.T, gh *fakeGitHub) (*httptest.Server, database.Store) {
	t.Helper()
	return newTestServerFull(t, Options{BaseURL: "http://app.example", GitHubClientID: "id", GitHubClientSecret: "secret"}, nil,
		func(s *Server) {
			s.githubOAuth.Endpoint = oauth2.Endpoint{
				AuthURL:  gh.URL + "/login/oauth/authorize",
				TokenURL: gh.URL + "/login/oauth/access_token",
			}
			s.githubAPI = gh.URL
		})
}

var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// githubSignIn performs the whole redirect dance with the given authorization
// code and returns the callback's response and, when it succeeded, the values
// from the URL fragment it redirected to.
func githubSignIn(t *testing.T, base, code string) (*http.Response, url.Values) {
	t.Helper()
	start, err := noFollow.Get(base + "/api/v1/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	_ = start.Body.Close()
	authorize, err := url.Parse(start.Header.Get("Location"))
	if err != nil || start.StatusCode != http.StatusFound {
		t.Fatalf("login should redirect to GitHub: %d %q", start.StatusCode, start.Header.Get("Location"))
	}
	state := authorize.Query().Get("state")

	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/auth/github/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil)
	for _, c := range start.Cookies() {
		req.AddCookie(c)
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusFound {
		return resp, nil
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(location.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	return resp, fragment
}

func mustSignIn(t *testing.T, base, code string) url.Values {
	t.Helper()
	resp, fragment := githubSignIn(t, base, code)
	if fragment == nil {
		t.Fatalf("GitHub sign-in with %q: want a redirect carrying tokens, got %d", code, resp.StatusCode)
	}
	return fragment
}

func meOf(t *testing.T, base, accessToken string) map[string]any {
	t.Helper()
	return decode[map[string]any](t, doJSON(t, http.MethodGet, base+"/api/v1/me", accessToken, nil))
}

func TestGitHubSignInCreatesAVerifiedAccount(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 1001, "octocat", "", email("noise@example.com", false, true), email("octo@example.com", true, true))
	srv, _ := newOAuthServer(t, gh)

	fragment := mustSignIn(t, srv.URL, "c1")

	me := meOf(t, srv.URL, fragment.Get("access_token"))
	if me["username"] != "octocat" || me["email"] != "octo@example.com" || me["email_verified"] != true {
		t.Fatalf("unexpected account: %v", me)
	}
	if fragment.Get("refresh_token") == "" {
		t.Error("the refresh token must be handed over too")
	}
}

func TestGitHubPrefersThePrimaryVerifiedEmail(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 1, "dev", "",
		email("unverified-primary@example.com", true, false),
		email("secondary@example.com", false, true),
		email("primary@example.com", true, true))
	srv, _ := newOAuthServer(t, gh)

	me := meOf(t, srv.URL, mustSignIn(t, srv.URL, "c1").Get("access_token"))

	if me["email"] != "primary@example.com" {
		t.Fatalf("email = %v, want the primary verified one", me["email"])
	}
}

// The core of the account-takeover bug: anyone can type someone else's address
// into their GitHub profile, so only an address GitHub has verified may link.
func TestAnUnverifiedGitHubEmailNeverLinksToAnExistingAccount(t *testing.T) {
	gh := newFakeGitHub(t)
	// The attacker's GitHub profile *claims* victim@example.com both as the public
	// email and in the list, but GitHub hasn't verified it.
	gh.user("attacker", 666, "mallory", "victim@example.com", email("victim@example.com", true, false))
	srv, _ := newOAuthServer(t, gh)
	victim := registerPair(t, srv.URL, "victim")
	victimID := userIDOf(t, srv.URL, victim.Access)

	attacker := meOf(t, srv.URL, mustSignIn(t, srv.URL, "attacker").Get("access_token"))

	if int(attacker["id"].(float64)) == victimID {
		t.Fatal("the attacker was signed in to the victim's account")
	}
	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		map[string]string{"login": "victim", "password": "password123"}); got != http.StatusOK {
		t.Errorf("the victim's own login must be untouched, got %d", got)
	}
}

func TestThePublicProfileEmailAloneIsNotTrusted(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 5, "someone", "victim@example.com") // public email, and no verified emails at all
	srv, _ := newOAuthServer(t, gh)
	victim := registerPair(t, srv.URL, "victim")

	got := meOf(t, srv.URL, mustSignIn(t, srv.URL, "c1").Get("access_token"))

	if int(got["id"].(float64)) == userIDOf(t, srv.URL, victim.Access) {
		t.Fatal("the unverified public email linked to the victim's account")
	}
}

func TestSignInWithoutAReachableEmailListStillWorksButNeverLinks(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 5, "someone", "victim@example.com", email("victim@example.com", true, true))
	gh.failEmails = true
	srv, _ := newOAuthServer(t, gh)
	victim := registerPair(t, srv.URL, "victim")

	got := meOf(t, srv.URL, mustSignIn(t, srv.URL, "c1").Get("access_token"))

	if int(got["id"].(float64)) == userIDOf(t, srv.URL, victim.Access) {
		t.Fatal("with the email list unavailable nothing may be linked")
	}
}

// Registration doesn't verify email, so someone can create an account on an
// address that isn't theirs, with a password they know. When the real owner
// signs in through GitHub they are attached to that account - and the
// squatter's access must not survive it.
func TestLinkingEvictsWhoeverSquattedTheUnverifiedAccount(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("owner", 77, "real-owner", "", email("victim@example.com", true, true))
	srv, _ := newOAuthServer(t, gh)
	squatter := registerPair(t, srv.URL, "victim") // registered on victim@example.com, password123, unverified

	fragment := mustSignIn(t, srv.URL, "owner")

	me := meOf(t, srv.URL, fragment.Get("access_token"))
	if me["email_verified"] != true || int(me["id"].(float64)) != userIDOf(t, srv.URL, squatter.Access) {
		t.Fatalf("the owner should now hold the existing account, verified: %v", me)
	}
	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		map[string]string{"login": "victim@example.com", "password": "password123"}); got != http.StatusUnauthorized {
		t.Errorf("the squatter's password must stop working: got %d", got)
	}
	if code, _ := refresh(t, srv.URL, squatter.Refresh); code != http.StatusUnauthorized {
		t.Errorf("the squatter's session must be revoked: got %d", code)
	}
}

func TestLinkingToAVerifiedAccountKeepsItsPassword(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("owner", 77, "bob-gh", "", email("bob@example.com", true, true))
	srv, store := newOAuthServer(t, gh)
	bob := registerPair(t, srv.URL, "bob")
	if err := store.SetEmailVerified(userIDOf(t, srv.URL, bob.Access)); err != nil {
		t.Fatal(err)
	}

	mustSignIn(t, srv.URL, "owner")

	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		map[string]string{"login": "bob", "password": "password123"}); got != http.StatusOK {
		t.Errorf("a verified account keeps working with its password: got %d", got)
	}
}

func TestABannedUserCannotSignInThroughGitHub(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("m", 9, "mallory", "", email("mallory@example.com", true, true))
	srv, _ := newOAuthServer(t, gh)
	admin := registerUser(t, srv.URL, "admin")

	first := mustSignIn(t, srv.URL, "m")
	mallory := meOf(t, srv.URL, first.Get("access_token"))
	banned := status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, int(mallory["id"].(float64))),
		admin, map[string]bool{"banned": true})
	if banned != http.StatusOK {
		t.Fatalf("ban: got %d", banned)
	}

	resp, fragment := githubSignIn(t, srv.URL, "m")

	if resp.StatusCode != http.StatusForbidden || fragment != nil {
		t.Fatalf("a banned account must not receive tokens: status %d", resp.StatusCode)
	}
}

// GitHub answers a revoked or bogus token with a JSON error. That used to
// decode into an empty profile, and an account for GitHub user id 0 was created.
func TestAGitHubErrorDoesNotCreateAnAccount(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.failProfile = true
	srv, store := newOAuthServer(t, gh)

	resp, fragment := githubSignIn(t, srv.URL, "anything")

	if resp.StatusCode != http.StatusBadGateway || fragment != nil {
		t.Fatalf("want 502 and no tokens, got %d", resp.StatusCode)
	}
	if n, _ := store.CountUsers(); n != 0 {
		t.Fatalf("no account may be created from a failed lookup, got %d users", n)
	}
}

func TestGitHubUsernamesAreMadeToFitTheRules(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 1, "octo-cat", "")
	gh.user("c2", 2, "octo-cat", "")
	srv, _ := newOAuthServer(t, gh)

	first := meOf(t, srv.URL, mustSignIn(t, srv.URL, "c1").Get("access_token"))
	second := meOf(t, srv.URL, mustSignIn(t, srv.URL, "c2").Get("access_token"))

	if first["username"] != "octo_cat" || second["username"] != "octo_cat1" {
		t.Fatalf("usernames = %v, %v; want octo_cat and octo_cat1", first["username"], second["username"])
	}
}

func TestCallbackRejectsAMissingOrWrongState(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.user("c1", 1, "octocat", "")
	srv, store := newOAuthServer(t, gh)

	for name, target := range map[string]string{
		"no cookie, no state":  "/api/v1/auth/github/callback?code=c1",
		"state without cookie": "/api/v1/auth/github/callback?code=c1&state=forged",
	} {
		resp, err := noFollow.Get(srv.URL + target)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", name, resp.StatusCode)
		}
	}

	// A cookie from one flow with a different state from another.
	start, _ := noFollow.Get(srv.URL + "/api/v1/auth/github")
	_ = start.Body.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/github/callback?code=c1&state=not-the-cookie-value", nil)
	for _, c := range start.Cookies() {
		req.AddCookie(c)
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a state that doesn't match the cookie: want 400, got %d", resp.StatusCode)
	}
	if n, _ := store.CountUsers(); n != 0 {
		t.Errorf("no account may be created without a valid state, got %d", n)
	}
}
