package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var web embed.FS

func (a *App) manage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	sub, _ := fs.Sub(web, "static")
	if r.URL.Path == "/manage" || r.URL.Path == "/manage/" {
		b, _ := fs.ReadFile(sub, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
		return
	}
	http.StripPrefix("/manage/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
