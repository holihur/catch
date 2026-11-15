package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
)

// serveSPA serves the built React app and falls back to index.html for client
// routes. It is only active when STATIC_DIR is configured.
func (a *API) serveSPA(w http.ResponseWriter, r *http.Request) {
	if a.cfg.StaticDir == "" {
		httpx.Error(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	clean := filepath.Clean("/" + r.URL.Path)
	full := filepath.Join(a.cfg.StaticDir, clean)
	root, err := filepath.Abs(a.cfg.StaticDir)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "静态资源目录无效")
		return
	}
	abs, err := filepath.Abs(full)
	if err != nil || (abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator))) {
		httpx.Error(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		if strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, abs)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "前端尚未构建")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}
