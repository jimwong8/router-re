package server

import (
	"net/http"
	"strings"
)

func handleWebEmbed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte("<html><body>embed</body></html>"))
	}
}
