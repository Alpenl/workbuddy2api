// Package web serves the frontend bundled into the Go binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler returns an SPA file handler. A source-only Go build returns a clear
// error at the root; production builds populate dist before compiling Go.
func Handler() http.Handler {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		return unavailableHandler()
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return unavailableHandler()
	}
	return &spaHandler{files: dist, server: http.FileServer(http.FS(dist))}
}

func unavailableHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "web assets were not included in this build", http.StatusServiceUnavailable)
	})
}

type spaHandler struct {
	files  fs.FS
	server http.Handler
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if filePath == "." {
		filePath = ""
	}
	if filePath != "" {
		if info, err := fs.Stat(h.files, filePath); err == nil && !info.IsDir() {
			if strings.HasPrefix(filePath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			h.server.ServeHTTP(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	request := r.Clone(r.Context())
	request.URL.Path = "/"
	h.server.ServeHTTP(w, request)
}
