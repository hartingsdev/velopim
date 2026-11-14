// Package web serves the built React app. The build result in dist/ is
// compiled into the binary, so production needs no Node.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const notBuilt = `<!doctype html><meta charset="utf-8"><title>velopim</title>
<p>Die Oberfläche ist noch nicht gebaut. Im Ordner <code>web/</code>: <code>npm install &amp;&amp; npm run build</code>,
danach das Binary neu bauen.</p>`

// Handler serves the app. Unknown paths get index.html so that client-side
// routes survive a reload.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // the directory is embedded at compile time
	}
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(dist, path); err == nil && !info.IsDir() {
				if strings.HasPrefix(path, "assets/") {
					// Vite puts a content hash into these names.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notBuilt))
			return
		}
		_, _ = w.Write(index)
	})
}
