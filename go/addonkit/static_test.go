package addonkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// build writes a "build" of the app into dir with every file at the same fixed mtime, as the
// reproducible packages have it.
func build(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func get(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The second build's index.html, same mtime and the same length, answers 200 with its own
// content to a revalidation carrying the first one's validators.
func TestAppFilesUpdateIsNotA304(t *testing.T) {
	dir := t.TempDir()
	build(t, dir, map[string]string{
		"index.html":           `<script src="/assets/index-AAAA.js"></script>`,
		"assets/index-AAAA.js": "a",
		"favicon.svg":          "<svg/>",
	})
	h := AppFiles(dir)

	first := get(h, "GET", "/", nil)
	if first.Code != 200 || !strings.Contains(first.Body.String(), "AAAA") {
		t.Fatalf("first: %d %q", first.Code, first.Body)
	}
	// the validators a browser sends back are the ones the first answer carried
	etag := first.Header().Get("ETag")
	validators := map[string]string{}
	if etag != "" {
		validators["If-None-Match"] = etag
	}
	if lm := first.Header().Get("Last-Modified"); lm != "" {
		validators["If-Modified-Since"] = lm
	}
	if again := get(h, "GET", "/", validators); again.Code != http.StatusNotModified {
		t.Fatalf("unchanged: %d, want 304", again.Code)
	}

	build(t, dir, map[string]string{
		"index.html":           `<script src="/assets/index-BBBB.js"></script>`,
		"assets/index-BBBB.js": "b",
	})
	second := get(h, "GET", "/", validators)
	if second.Code != 200 || !strings.Contains(second.Body.String(), "BBBB") {
		t.Fatalf("after the update: %d %q, want 200 with the new index", second.Code, second.Body)
	}
	if etag == "" || second.Header().Get("ETag") == etag {
		t.Fatalf("ETag %q -> %q: none, or it did not change with the content", etag, second.Header().Get("ETag"))
	}
	if second.Header().Get("Last-Modified") != "" || second.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers: %v", second.Header())
	}
	// a mtime validator alone is never a 304 (there is no Last-Modified to compare against)
	since := map[string]string{"If-Modified-Since": time.Now().UTC().Format(http.TimeFormat)}
	if r := get(h, "GET", "/index.html", since); r.Code != 200 {
		t.Fatalf("If-Modified-Since alone: %d", r.Code)
	}
}

func TestAppFiles(t *testing.T) {
	dir := t.TempDir()
	build(t, dir, map[string]string{
		"index.html":           "INDEX",
		"assets/index-AAAA.js": "JS",
		"favicon.svg":          "<svg/>",
		".secret":              "no",
		"assets/.hidden":       "no",
	})
	h := AppFiles(dir)
	for _, c := range []struct {
		method, target string
		code           int
		body, cache    string
	}{
		{"GET", "/", 200, "INDEX", "no-cache"},
		{"GET", "/devices/42", 200, "INDEX", "no-cache"},
		{"GET", "/assets/index-AAAA.js", 200, "JS", "public, max-age=31536000, immutable"},
		{"GET", "/favicon.svg", 200, "<svg/>", "no-cache"},
		{"HEAD", "/favicon.svg", 200, "", "no-cache"},
		{"GET", "/assets", 200, "INDEX", "no-cache"},
		{"GET", "/.secret", 404, "", ""},
		{"GET", "/assets/.hidden", 404, "", ""},
		{"GET", "/../../etc/passwd", 200, "INDEX", "no-cache"},
		{"POST", "/", 405, "", "no-store"},
	} {
		w := get(h, c.method, c.target, nil)
		if w.Code != c.code || (c.body != "" && w.Body.String() != c.body) || w.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s %s: %d %q cache %q", c.method, c.target, w.Code, w.Body, w.Header().Get("Cache-Control"))
		}
	}
	if ct := get(h, "GET", "/", nil).Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("index content type %q", ct)
	}
	// a missing index.html: 404, not a panic
	if w := get(AppFiles(t.TempDir()), "GET", "/", nil); w.Code != 404 {
		t.Errorf("no index: %d", w.Code)
	}
}
