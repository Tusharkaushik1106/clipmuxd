package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed all:web
var webFS embed.FS

type Config struct {
	Addr  string
	Inbox string
	Token string
}

type Server struct {
	cfg Config
	hub *hub
	mu  sync.Mutex
}

func New(cfg Config) *Server {
	return &Server{cfg: cfg, hub: newHub()}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux.Handle("/", s.authPage(http.FileServer(http.FS(sub))))

	mux.HandleFunc("/api/send", s.auth(s.handleSend))
	mux.HandleFunc("/api/inbox", s.auth(s.handleInboxList))
	mux.HandleFunc("/api/inbox/", s.auth(s.handleInboxGet))
	mux.HandleFunc("/api/raw/", s.auth(s.handleInboxRaw))
	mux.HandleFunc("/api/delete/", s.auth(s.handleDelete))
	mux.HandleFunc("/api/ws", s.auth(s.hub.serve))

	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	log.Printf("listening on %s", s.cfg.Addr)
	return srv.ListenAndServe()
}

func (s *Server) checkToken(r *http.Request) bool {
	k := r.URL.Query().Get("k")
	if k == "" {
		k = r.Header.Get("X-EC-Token")
	}
	if k == "" {
		if c, err := r.Cookie("ec_token"); err == nil {
			k = c.Value
		}
	}
	return k != "" && k == s.cfg.Token
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkToken(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// authPage: allow loading the shell but set a cookie from ?k= so later
// requests from the page (which may not pass ?k=) succeed.
func (s *Server) authPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if k := r.URL.Query().Get("k"); k == s.cfg.Token && k != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "ec_token",
				Value:    k,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   60 * 60 * 24 * 365,
			})
		}
		// service worker + manifest are fine to serve without auth
		if r.URL.Path == "/sw.js" || r.URL.Path == "/manifest.webmanifest" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.checkToken(r) {
			http.Error(w, "unauthorized — open the URL printed on your PC (with ?k=...)", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // text|file
	Category string `json:"category"` // text|link|image|video|audio|doc|archive|other
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Preview  string `json:"preview,omitempty"`
	URL      string `json:"url,omitempty"` // for link category
}

var (
	extImage   = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".svg": true, ".heic": true, ".heif": true, ".tiff": true, ".avif": true}
	extVideo   = map[string]bool{".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true, ".wmv": true, ".flv": true, ".m4v": true, ".3gp": true}
	extAudio   = map[string]bool{".mp3": true, ".wav": true, ".flac": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".wma": true}
	extDoc     = map[string]bool{".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true, ".rtf": true, ".md": true, ".csv": true, ".epub": true}
	extArchive = map[string]bool{".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true}
)

func categorizeFile(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case extImage[ext]:
		return "image"
	case extVideo[ext]:
		return "video"
	case extAudio[ext]:
		return "audio"
	case extDoc[ext]:
		return "doc"
	case extArchive[ext]:
		return "archive"
	default:
		return "other"
	}
}

func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \n\t") {
		// allow leading/trailing whitespace only
		return false
	}
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// 5 GB max
	r.Body = http.MaxBytesReader(w, r.Body, 5<<30)

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		s.saveMultipart(w, r)
		return
	}
	// plain text
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		http.Error(w, "empty", http.StatusBadRequest)
		return
	}
	s.saveText(w, text)
}

func (s *Server) saveText(w http.ResponseWriter, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := fmt.Sprintf("%s.txt", time.Now().Format("20060102-150405"))
	path := filepath.Join(s.cfg.Inbox, name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	copyToClipboard(text)
	log.Printf("text received (%d bytes) -> clipboard", len(text))
	s.hub.broadcast("new")
	writeJSON(w, map[string]any{"ok": true, "name": name, "clipboard": true})
}

func (s *Server) saveMultipart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// text field takes priority if present
	if t := strings.TrimSpace(r.FormValue("text")); t != "" {
		s.saveText(w, t)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file or text", http.StatusBadRequest)
		return
	}
	defer file.Close()

	safeName := sanitizeFilename(hdr.Filename)
	stamp := time.Now().Format("20060102-150405")
	final := fmt.Sprintf("%s_%s", stamp, safeName)
	// O_EXCL prevents two concurrent uploads with the same timestamp + name from
	// silently overwriting each other. Retry with a counter suffix on collision.
	var out *os.File
	for i := 0; i < 50; i++ {
		tryName := final
		if i > 0 {
			tryName = fmt.Sprintf("%s_%d_%s", stamp, i, safeName)
		}
		out, err = os.OpenFile(filepath.Join(s.cfg.Inbox, tryName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			final = tryName
			break
		}
		if !errors.Is(err, os.ErrExist) {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if out == nil {
		http.Error(w, "could not allocate filename", 500)
		return
	}
	n, err := io.Copy(out, file)
	cerr := out.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if cerr != nil {
		http.Error(w, cerr.Error(), 500)
		return
	}
	log.Printf("file received: %s (%d bytes)", final, n)
	clip := false
	if categorizeFile(final) == "image" {
		go copyImageToClipboard(filepath.Join(s.cfg.Inbox, final))
		clip = true
	}
	s.hub.broadcast("new")
	writeJSON(w, map[string]any{"ok": true, "name": final, "size": n, "clipboard": clip})
}

func (s *Server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.cfg.Inbox)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		it := item{
			ID:       e.Name(),
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".txt") && info.Size() < 64*1024 {
			it.Kind = "text"
			it.Category = "text"
			if b, err := os.ReadFile(filepath.Join(s.cfg.Inbox, e.Name())); err == nil {
				body := strings.TrimSpace(string(b))
				if looksLikeURL(body) {
					it.Category = "link"
					it.URL = body
				}
				prev := body
				if len(prev) > 500 {
					prev = prev[:500] + "…"
				}
				it.Preview = prev
			}
		} else {
			it.Kind = "file"
			it.Category = categorizeFile(e.Name())
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Modified > items[j].Modified })
	writeJSON(w, items)
}

func (s *Server) handleInboxGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/inbox/")
	id = filepath.Base(id)
	if id == "" || id == "." || strings.Contains(id, "..") {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.cfg.Inbox, id)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, id))
	http.ServeContent(w, r, id, info.ModTime(), f)
}

func (s *Server) handleInboxRaw(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/raw/")
	id = filepath.Base(id)
	if id == "" || id == "." || strings.Contains(id, "..") {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.cfg.Inbox, id)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// no Content-Disposition -> browser renders inline
	http.ServeContent(w, r, id, info.ModTime(), f)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "POST/DELETE only", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/delete/")
	id = filepath.Base(id)
	if id == "" || id == "." || strings.Contains(id, "..") {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := os.Remove(filepath.Join(s.cfg.Inbox, id)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.hub.broadcast("new")
	writeJSON(w, map[string]any{"ok": true})
}

func sanitizeFilename(n string) string {
	n = filepath.Base(n)
	n = strings.ReplaceAll(n, "\\", "_")
	n = strings.ReplaceAll(n, "/", "_")
	if n == "" || n == "." || n == ".." {
		n = "file"
	}
	return n
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
