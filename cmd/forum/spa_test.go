package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const indexHTML = "<!doctype html><title>app</title>"

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte(indexHTML)},
		"favicon.ico":         {Data: []byte("icon")},
		"assets/app-3f9a.js":  {Data: []byte("console.log('app')")},
		"assets/style-77.css": {Data: []byte("body{}")},
	}
}

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	return res, string(body)
}

func TestServesTheIndexAtTheRoot(t *testing.T) {
	h, built := spaHandler(builtFS())
	if !built {
		t.Fatal("a build with an index.html is a built frontend")
	}
	res, body := get(t, h, "/")
	if res.StatusCode != http.StatusOK || body != indexHTML {
		t.Fatalf("GET / = %d %q", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestClientSideRoutesFallBackToTheIndex(t *testing.T) {
	h, _ := spaHandler(builtFS())
	for _, path := range []string{"/post/42", "/u/alice", "/settings", "/some/deep/unknown/route", "/assets/", "/assets"} {
		res, body := get(t, h, path)
		if res.StatusCode != http.StatusOK || body != indexHTML {
			t.Errorf("GET %s = %d %q, want the index (a hard refresh on a client route must work)", path, res.StatusCode, body)
		}
	}
}

func TestServesRealFiles(t *testing.T) {
	h, _ := spaHandler(builtFS())
	for path, want := range map[string]string{
		"/favicon.ico":         "icon",
		"/assets/app-3f9a.js":  "console.log('app')",
		"/assets/style-77.css": "body{}",
	} {
		if res, body := get(t, h, path); res.StatusCode != http.StatusOK || body != want {
			t.Errorf("GET %s = %d %q, want %q", path, res.StatusCode, body, want)
		}
	}
}

func TestPathTraversalCannotEscapeTheBuild(t *testing.T) {
	h, _ := spaHandler(builtFS())
	for _, path := range []string{"/../../etc/passwd", "/..%2f..%2fetc/passwd", "/assets/../../../etc/passwd"} {
		res, body := get(t, h, path)
		if strings.Contains(body, "root:") {
			t.Fatalf("GET %s leaked a file outside the build", path)
		}
		if res.StatusCode >= 500 {
			t.Errorf("GET %s = %d, want a clean fallback", path, res.StatusCode)
		}
	}
}

func TestCachePolicy(t *testing.T) {
	h, _ := spaHandler(builtFS())

	// Hashed assets never change meaning, so they can be cached for good.
	res, _ := get(t, h, "/assets/app-3f9a.js")
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("hashed assets should be cached immutably, Cache-Control = %q", cc)
	}

	// The index names the current bundle; a cached one would point at stale files.
	for _, path := range []string{"/", "/post/1"} {
		res, _ := get(t, h, path)
		if cc := res.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("GET %s: index must not be cached, Cache-Control = %q", path, cc)
		}
	}

	// Files outside assets/ aren't content-hashed, so no long cache.
	res, _ = get(t, h, "/favicon.ico")
	if cc := res.Header.Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("an unhashed file must not be cached forever, Cache-Control = %q", cc)
	}
}

// A fresh checkout has only dist/.gitkeep. That used to stop the server from
// starting at all, which broke the documented "run the API, serve the frontend
// with Vite" workflow.
func TestMissingBuildIsNotFatal(t *testing.T) {
	h, built := spaHandler(fstest.MapFS{".gitkeep": {Data: nil}})

	if built {
		t.Fatal("a dist/ without index.html is not a built frontend")
	}
	res, body := get(t, h, "/")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", res.StatusCode)
	}
	if !strings.Contains(body, "npm run build") || !strings.Contains(body, "/api/v1") {
		t.Errorf("the message should say how to build and where the API is, got %q", body)
	}
}

func TestTheEmbeddedBuildLoads(t *testing.T) {
	// Whatever is in the embedded dist/ - a real build or just the placeholder -
	// the constructor must succeed.
	if _, _, err := newSPAHandler(); err != nil {
		t.Fatalf("newSPAHandler: %v", err)
	}
}
