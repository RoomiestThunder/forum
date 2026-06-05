// Package templates embeds the HTML templates so they are available
// regardless of the working directory at runtime or test time.
package templates

import "embed"

//go:embed *.html
var FS embed.FS
