// Package server is the development server: it rebuilds on change, serves
// the result from memory and tells open browsers to reload.
package server

import (
	"fmt"
	"hash/fnv"
	"html"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hanji/internal/site"
)

type Server struct {
	opt     site.Options
	mu      sync.RWMutex
	res     *site.Result
	err     error
	base    string
	clients map[chan struct{}]bool
}

// Dev builds, serves on addr and watches for changes until the process ends.
func Dev(opt site.Options, addr string) error {
	s := &Server{opt: opt, clients: map[chan struct{}]bool{}, base: "/"}
	s.rebuild()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go s.watch()
	fmt.Printf("\n  Hanji %s\n\n  ➜  http://%s%s\n\n", site.Version, strings.Replace(ln.Addr().String(), "[::]", "localhost", 1), s.base)
	return http.Serve(ln, s)
}

func (s *Server) rebuild() {
	start := time.Now()
	res, err := site.Build(s.opt)
	s.mu.Lock()
	s.err = err
	if err == nil {
		s.res = res
		s.base = res.Base
	}
	for c := range s.clients {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	stamp := time.Now().Format("15:04:05")
	if err != nil {
		fmt.Printf("  %s  ✗ build failed\n%s\n", stamp, indent(err.Error()))
		return
	}
	fmt.Printf("  %s  ✓ %d pages in %s\n", stamp, res.Pages, time.Since(start).Round(time.Millisecond))
}

func indent(s string) string {
	return "      " + strings.ReplaceAll(s, "\n", "\n      ")
}

// fingerprint hashes names, sizes and mtimes of everything under the project.
func (s *Server) fingerprint() uint64 {
	h := fnv.New64a()
	filepath.WalkDir(s.opt.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() && p != s.opt.Dir && (name == "dist" || name == "node_modules" || name == "result" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			fmt.Fprintf(h, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return h.Sum64()
}

func (s *Server) watch() {
	last := s.fingerprint()
	for range time.Tick(300 * time.Millisecond) {
		if fp := s.fingerprint(); fp != last {
			last = fp
			s.rebuild()
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	res, buildErr, base := s.res, s.err, s.base
	s.mu.RUnlock()

	if strings.HasSuffix(r.URL.Path, "/_hanji/reload") {
		s.events(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if buildErr != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, errorPage, html.EscapeString(buildErr.Error()))
		return
	}
	if !strings.HasPrefix(r.URL.Path, base) {
		http.Redirect(w, r, base, http.StatusFound)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, base)
	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	f, ok := res.Files[name]
	status := http.StatusOK
	if !ok {
		if _, dir := res.Files[name+"/index.html"]; dir {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
			return
		}
		status = http.StatusNotFound
		if f, ok = res.Files["404.html"]; !ok {
			http.Error(w, "404 — no page at "+r.URL.Path, status)
			return
		}
		name = "404.html"
	}
	data := f.Data
	if f.Path != "" {
		var err error
		if data, err = os.ReadFile(f.Path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(status)
	w.Write(data)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	c := make(chan struct{}, 1)
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
	}()
	for {
		select {
		case <-c:
			fmt.Fprint(w, "data: reload\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Preview serves an already-built directory.
func Preview(dir, addr string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%s not found — run `hanji build` first", dir)
	}
	fmt.Printf("\n  Hanji preview\n\n  ➜  http://%s/\n\n", strings.Replace(addr, "0.0.0.0", "localhost", 1))
	return http.ListenAndServe(addr, http.FileServer(http.Dir(dir)))
}

const errorPage = `<!doctype html><meta charset="utf-8"><title>Build error · Hanji</title>
<style>body{margin:0;font:15px/1.6 ui-monospace,Menlo,monospace;background:#1b1714;color:#f3ead9;padding:48px}
h1{font:600 20px system-ui;color:#ef7a6d;margin:0 0 20px}pre{white-space:pre-wrap;background:#262019;padding:20px;border-radius:10px;border-left:4px solid #ef7a6d}
p{font-family:system-ui;color:#b5a892}</style>
<h1>Build failed</h1><pre>%s</pre><p>This page reloads by itself once the file is fixed.</p>
<script>new EventSource("/_hanji/reload").onmessage=()=>location.reload()</script>`
