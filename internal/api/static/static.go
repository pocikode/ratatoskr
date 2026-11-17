// Package static serves the embedded Svelte SPA (web/dist) with history-style fallback.
//
// The Vite build writes into ./dist (see web/vite.config.ts); a committed .gitkeep keeps the
// embed valid before the first build. This package only serves UI assets: proxy and Replay
// traffic never goes through it (ADR-0003).
package static

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded bundle. Mount it as the catch-all after the API routes.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Unreachable: "dist" is a compile-time embed root.
		panic(err)
	}
	return NewHandler(sub)
}

// NewHandler serves files from fsys. Unknown paths fall back to index.html so client-side
// routes survive a reload, except under /api (never swallowed) and /assets (a missing
// hashed asset must 404, not return HTML).
func NewHandler(fsys fs.FS) http.Handler {
	return &handler{fsys: fsys}
}

type handler struct{ fsys fs.FS }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	p := path.Clean("/" + r.URL.Path)
	if p == "/api" || strings.HasPrefix(p, "/api/") {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(p, "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(h.fsys, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite content-hashes everything under assets/.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, h.fsys, name) //nolint:gosec // T-003: name is path.Clean-ed and fs.FS rejects escaping paths
			return
		}
		if strings.HasPrefix(p, "/assets/") {
			http.NotFound(w, r)
			return
		}
	}

	h.serveIndex(w, r)
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(h.fsys, "index.html")
	if err != nil {
		http.Error(w, "web UI is not built: run `make build`", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}
