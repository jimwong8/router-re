package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		http.NotFound(w, r)
		return
	}
	base := "../web/dist"
	f := p
	if f == "/" || f == "" {
		f = "/index.html"
	}
	f = strings.TrimPrefix(f, "/")
	full := filepath.Join(base, f)
	_, err := os.Stat(full)
	if err != nil {
		if !strings.HasPrefix(p, "/assets/") {
			f = "index.html"
			full = filepath.Join(base, f)
		} else {
			http.NotFound(w, r)
			return
		}
	}
	if p == "/" || p == "/index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(p, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFile(w, r, full)
}
