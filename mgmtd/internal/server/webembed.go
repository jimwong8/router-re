package server

import (
	"embed"
	"net/http"
	"path"
	"strings"
)

//go:embed ../../web/dist/*
var webDist embed.FS

func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		http.NotFound(w, r)
		return
	}
	f := p
	if f == "/" || f == "" {
		f = "/index.html"
	}
	// embed names include the relative prefix
	name := "../../web/dist" + f
	data, err := webDist.ReadFile(name)
	if err != nil {
		if !strings.HasPrefix(f, "/assets/") {
			name = "../../web/dist/index.html"
			data, err = webDist.ReadFile(name)
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if f == "/index.html" || f == "/" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(f, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
