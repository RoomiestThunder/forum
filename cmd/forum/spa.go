package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"forum/web"
)

// newSPAHandler serves the embedded React build. Any path that doesn't
// match a real file in dist/ falls back to index.html so client-side
// routes (e.g. /post/42 on a hard refresh) resolve correctly.
//
// built reports whether a frontend build is actually embedded. A fresh
// checkout has only a placeholder in dist/, and the server is still useful
// then - the API works, and during development the frontend is served by
// Vite - so a missing build is not an error: the handler answers with a
// message saying how to build it.
func newSPAHandler() (handler http.Handler, built bool, err error) {
	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return nil, false, err
	}
	handler, built = spaHandler(dist)
	return handler, built, nil
}

func spaHandler(dist fs.FS) (http.Handler, bool) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return http.HandlerFunc(frontendNotBuilt), false
	}
	fileServer := http.FileServer(http.FS(dist))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// index.html names the current hashed bundles, so a cached copy would
		// keep pointing at files from a previous deploy.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path) // rooted, so ".." can't climb out of dist
		if p == "/" || p == "/index.html" {
			serveIndex(w, r)
			return
		}
		name := p[1:] // fs.FS paths are relative, no leading slash
		if info, statErr := fs.Stat(dist, name); statErr != nil || info.IsDir() {
			serveIndex(w, r)
			return
		}
		// Vite names everything under assets/ after a hash of its contents, so
		// a given URL never changes meaning and can be cached indefinitely.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	}), true
}

func frontendNotBuilt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("The web interface has not been built.\n\n" +
		"Run `npm run build` in web/ and rebuild the server, or use `npm run dev` in web/ (http://localhost:5173).\n" +
		"The API is available at /api/v1.\n"))
}
