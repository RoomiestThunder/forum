package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"time"

	"forum/web"
)

// newSPAHandler serves the embedded React build. Any path that doesn't
// match a real file in dist/ falls back to index.html so client-side
// routes (e.g. /post/42 on a hard refresh) resolve correctly.
func newSPAHandler() (http.Handler, error) {
	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if p == "/" {
			p = "index.html"
		} else {
			p = p[1:] // fs.FS paths are relative, no leading slash
		}
		if info, statErr := fs.Stat(dist, p); statErr != nil || info.IsDir() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
