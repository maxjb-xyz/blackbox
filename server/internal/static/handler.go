package static

import (
	"io/fs"
	"net/http"
	"strings"
)

func Handler(staticFS fs.FS) http.Handler {
	sub, err := fs.Sub(staticFS, "web/dist")
	if err != nil {
		panic("static: failed to sub FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		if _, err := fs.Stat(sub, name); err != nil {
			data, readErr := fs.ReadFile(sub, "index.html")
			if readErr != nil {
				http.Error(w, "index.html not found", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
