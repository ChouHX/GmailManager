package main

import (
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"gmailmanager/internal/api"
	"gmailmanager/internal/config"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
)

//go:embed all:web/dist
var embedded embed.FS

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[gmailmanager] ")

	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	box, err := secret.New(cfg.Secret, cfg.DataDir)
	if err != nil {
		log.Fatalf("init secret box: %v", err)
	}

	assets, err := newAssets()
	if err != nil {
		log.Fatalf("load frontend assets: %v", err)
	}

	srv := api.New(cfg, st, box, assets)
	srv.LogStartup()

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-stop
	log.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

// newAssets serves the embedded SPA build with history fallback. When the
// frontend has not been built yet it serves a short instruction page instead of
// refusing to start, so `go run .` still works for API-only development.
func newAssets() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "web/dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		log.Printf("WARNING: web/dist/index.html missing, serving placeholder page")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, placeholderPage)
		}), nil
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "/" {
			serveIndex(w, index)
			return
		}
		if f, err := sub.Open(strings.TrimPrefix(clean, "/")); err == nil {
			f.Close()
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, index)
	}), nil
}

const placeholderPage = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<title>Gmail Manager</title><style>
body{font:13px/1.7 "Segoe UI","Microsoft YaHei",sans-serif;color:#1c1f23;background:#f4f5f6;margin:0;
display:grid;place-items:center;height:100vh}
main{background:#fff;border:1px solid #e2e5e8;padding:22px 26px;max-width:520px}
h1{font-size:14px;margin:0 0 8px}
code{background:#fafbfb;border:1px solid #e8eaec;padding:1px 4px;font-size:12px}
</style></head><body><main>
<h1>前端尚未构建</h1>
<p>API 已就绪。构建前端后刷新此页面：</p>
<p><code>cd web &amp;&amp; npm install &amp;&amp; npm run build</code></p>
<p>或使用 Docker：<code>docker compose up -d --build</code></p>
</main></body></html>`

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}
