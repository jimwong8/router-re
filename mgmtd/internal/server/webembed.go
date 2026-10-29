package server

import (
	"embed"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed webdist/*
var webDist embed.FS

func (s *Server) handleWebEmbed(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		http.NotFound(w, r)
		return
	}
	f := p
	if f == "/" || f == "" {
		f = "webdist/index.html"
	} else {
		f = "webdist" + p
	}
	data, err := webDist.ReadFile(strings.TrimPrefix(f, "/"))
	if err != nil {
		if !strings.HasPrefix(p, "/assets/") {
			f = "webdist/index.html"
			data, err = webDist.ReadFile(strings.TrimPrefix(f, "/"))
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Del("Cache-Control")
	if p == "/" || p == "/index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if p == "/" || p == "/index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if strings.HasPrefix(p, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Write(data)
}
