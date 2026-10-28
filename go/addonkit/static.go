package addonkit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// AppFiles serves an addon's built app (a Vite build: index.html, its hashed assets under
// /assets/, a few files like the favicon) from dir. A path without a file answers index.html, so
// the app's own routes load it; a dotfile anywhere in the path answers 404, as does a method
// other than GET and HEAD (405).
//
// The packages are built reproducibly, so every file of every version carries the same mtime.
// A validator from the mtime would answer an updated index.html's revalidation with 304 and keep
// the browser on the old one, which names the old assets. So:
//
//   - /assets/ (content-hashed names) are immutable for a year;
//   - every other file is `no-cache` with an ETag from its content and no Last-Modified, so a
//     revalidation is 304 only while the content is the same.
func AppFiles(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			WriteError(w, http.StatusMethodNotAllowed, "method", r.Method)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		if strings.Contains(p, "/.") {
			http.NotFound(w, r)
			return
		}
		file := filepath.Join(dir, filepath.FromSlash(p))
		if st, err := os.Stat(file); err != nil || st.IsDir() {
			p, file = "/index.html", filepath.Join(dir, "index.html")
		}
		if strings.HasPrefix(p, "/assets/") {
			f, err := os.Open(file)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			st, err := f.Stat()
			if err != nil || !st.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, st.Name(), st.ModTime(), f)
			return
		}
		b, err := os.ReadFile(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(b)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
		// a zero time: no Last-Modified, and If-Modified-Since is not looked at
		http.ServeContent(w, r, path.Base(p), time.Time{}, bytes.NewReader(b))
	})
}
