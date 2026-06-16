// Package frontend embeds the prebuilt web/dist static assets into the binary
// so the backend can serve the SPA without depending on the runtime filesystem
// layout. This mirrors the new-api packaging flow: the frontend is built in the
// Docker frontend stage, copied into web/dist, and compiled in via go:embed, so
// the served paths always match the build output.
package frontend

import "embed"

// DistFS holds the compiled frontend assets rooted at web/dist.
//
// In production this is populated by the Docker build (frontend stage -> go:embed).
// Locally, run `npm run build` in web/ first; a placeholder web/dist/index.html is
// committed so `go build`/`go test` work without a full frontend build.
//
//go:embed web/dist
var DistFS embed.FS

// IndexHTML is the SPA entry document, embedded as bytes so it can be served
// directly with c.Data. Serving it via http.FileServer would trigger its
// built-in "/index.html -> ./" redirect and loop on "/".
//
//go:embed web/dist/index.html
var IndexHTML []byte
