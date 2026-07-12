// Package web embeds the built single-page app so the Go binary can serve
// it without depending on the filesystem layout at runtime.
//
// The dist/ directory is produced by `npm run build` inside web/ and is not
// checked in (see web/.gitignore) - only dist/.gitkeep is, so `go build`
// still works against a fresh checkout that hasn't run the frontend build
// yet (it just embeds an empty shell instead of the real app).
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
