package service

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/dist/*
var staticDist embed.FS

func (s *APIServer) handleInfoPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	dist, err := fs.Sub(staticDist, "static/dist")
	if err != nil {
		http.Error(w, "static assets are not available", http.StatusInternalServerError)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(dist, path); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, dist, path)
}
