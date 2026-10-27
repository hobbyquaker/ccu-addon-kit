package addonkit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlatform(t *testing.T) {
	dir := t.TempDir()
	d := Detector{VersionFile: filepath.Join(dir, "VERSION"), Occulited: filepath.Join(dir, "occulited"), CCU3Hook: filepath.Join(dir, "S00InstallAddon")}
	if got := d.Platform(); got != OpenCCU {
		t.Fatalf("nothing: %s", got)
	}
	os.WriteFile(d.CCU3Hook, []byte("#!/bin/sh\n"), 0o755)
	if got := d.Platform(); got != CCU3 {
		t.Fatalf("hook: %s", got)
	}
	os.WriteFile(d.VersionFile, []byte("VERSION=3.89.8\nPRODUCT=x\nVARIANT=lite\n"), 0o644)
	if got := d.Platform(); got != CCU3 {
		t.Fatalf("VARIANT=lite is not the marker: %s", got)
	}
	os.WriteFile(d.VersionFile, []byte("VERSION=3.89.8\nLITE=1.0.0-dev.40\n"), 0o644)
	if got := d.Platform(); got != OpenccuLite {
		t.Fatalf("LITE=: %s", got)
	}
	os.Remove(d.VersionFile)
	os.WriteFile(d.Occulited, []byte("x"), 0o644)
	if got := d.Platform(); got != CCU3 {
		t.Fatalf("a non-executable occulited is nothing: %s", got)
	}
	os.Chmod(d.Occulited, 0o755)
	if got := d.Platform(); got != OpenccuLite {
		t.Fatalf("executable occulited: %s", got)
	}
}

func TestLevels(t *testing.T) {
	cases := []struct {
		level, role string
		want        Level
	}{{"administer", "", LevelAdmin}, {"configure", "", LevelUser}, {"operate", "user", LevelUser}, {"read", "", LevelGuest},
		{"", "admin", LevelAdmin}, {"", "user", LevelUser}, {"", "", LevelUnknown}}
	for _, c := range cases {
		if got := LevelOf(c.level, c.role); got != c.want {
			t.Errorf("LevelOf(%q,%q) = %s", c.level, c.role, got)
		}
	}
	if !Admits(LevelUser, LevelUnknown) || Admits(LevelAdmin, LevelUnknown) {
		t.Error("unknown is admitted for user, refused for admin")
	}
	if Admits(LevelUser, LevelGuest) || !Admits(LevelGuest, LevelGuest) || !Admits(LevelUser, LevelAdmin) {
		t.Error("guest/user/admin ordering")
	}
}

const sid = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func stateServer(t *testing.T, calls *atomic.Int32, answer func(bearer string) (int, any)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/v1/state" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		calls.Add(1)
		code, body := answer(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(body)
	}))
}

func TestSession(t *testing.T) {
	var calls atomic.Int32
	srv := stateServer(t, &calls, func(bearer string) (int, any) {
		switch bearer {
		case sid:
			return 200, map[string]any{"authenticated": true, "sid": sid, "user": "alice", "level": "configure", "role": "user"}
		case "ADMINADMINADMINADMINADMINA":
			return 200, map[string]any{"authenticated": true, "sid": bearer, "user": "root", "level": "administer"}
		case "READREADREADREADREADREADRE":
			return 200, map[string]any{"authenticated": true, "sid": bearer, "user": "guest", "level": "read"}
		case "olt_tokentokentokentokento":
			return 200, map[string]any{"authenticated": true, "user": "token:script", "scopes": []string{"addon:x"}}
		}
		return 401, map[string]any{"authenticated": false}
	})
	defer srv.Close()
	lite := func() Kind { return OpenccuLite }
	seen := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := FromRequest(r)
		w.Write([]byte(s.User + ":" + s.Level.String()))
	})
	h := Session(seen, SessionOptions{Base: srv.URL, Platform: lite})
	get := func(h http.Handler, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/x", nil)
		if id != "" {
			r.Header.Set(HeaderSession, id)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := get(h, sid); w.Code != 200 || w.Body.String() != "alice:user" {
		t.Fatalf("user session: %d %s", w.Code, w.Body)
	}
	if w := get(h, sid); w.Code != 200 || calls.Load() != 1 {
		t.Fatalf("second call should hit the cache: calls %d", calls.Load())
	}
	if w := get(h, ""); w.Code != 401 {
		t.Fatalf("no header: %d", w.Code)
	}
	if w := get(h, "nope"); w.Code != 401 || calls.Load() != 1 {
		t.Fatalf("a wrong shape never reaches /state: %d calls %d", w.Code, calls.Load())
	}
	if w := get(h, "ZZZZZZZZZZZZZZZZZZZZZZZZZZ"); w.Code != 401 {
		t.Fatalf("unknown id: %d", w.Code)
	}
	if w := get(h, "olt_tokentokentokentokento"); w.Code != 401 {
		t.Fatalf("an API token (no sid) must be refused: %d", w.Code)
	}
	if w := get(h, "READREADREADREADREADREADRE"); w.Code != 403 {
		t.Fatalf("read level below user: %d", w.Code)
	}
	admin := Session(Require(LevelAdmin, seen), SessionOptions{Base: srv.URL, Platform: lite})
	if w := get(admin, sid); w.Code != 403 {
		t.Fatalf("Require admin on a user: %d", w.Code)
	}
	if w := get(admin, "ADMINADMINADMINADMINADMINA"); w.Code != 200 || w.Body.String() != "root:admin" {
		t.Fatalf("admin: %d %s", w.Code, w.Body)
	}
	// on a CCU the header is a client's and is never trusted
	ccu := Session(seen, SessionOptions{Base: srv.URL, Platform: func() Kind { return CCU3 }})
	if w := get(ccu, sid); w.Code != 401 {
		t.Fatalf("CCU must refuse the header: %d", w.Code)
	}
}

func TestCSRF(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := CSRF(ok)
	try := func(method string, hdr map[string]string) int {
		r := httptest.NewRequest(method, "http://ccu.local/addons/x/api", nil)
		r.Host = "ccu.local"
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := try("GET", nil); c != 204 {
		t.Errorf("GET passes: %d", c)
	}
	if c := try("OPTIONS", nil); c != 405 {
		t.Errorf("OPTIONS is 405: %d", c)
	}
	if c := try("POST", nil); c != 403 {
		t.Errorf("POST without header: %d", c)
	}
	if c := try("POST", map[string]string{HeaderRequest: "1"}); c != 204 {
		t.Errorf("POST with header: %d", c)
	}
	if c := try("POST", map[string]string{HeaderRequest: "1", "Origin": "http://ccu.local"}); c != 204 {
		t.Errorf("same host origin: %d", c)
	}
	if c := try("POST", map[string]string{HeaderRequest: "1", "Origin": "https://CCU.local"}); c != 204 {
		t.Errorf("scheme not compared, host case-insensitive: %d", c)
	}
	if c := try("POST", map[string]string{HeaderRequest: "1", "Origin": "http://evil.example"}); c != 403 {
		t.Errorf("foreign origin: %d", c)
	}
	if c := try("POST", map[string]string{HeaderRequest: "1", "Origin": "null"}); c != 403 {
		t.Errorf("null origin: %d", c)
	}
	if c := try("DELETE", map[string]string{HeaderRequest: "1", "Sec-Fetch-Site": "cross-site"}); c != 403 {
		t.Errorf("cross-site: %d", c)
	}
	if c := try("PUT", map[string]string{HeaderRequest: "1", "Sec-Fetch-Site": "same-origin"}); c != 204 {
		t.Errorf("same-origin: %d", c)
	}
}

func TestAuthTransport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.api")
	os.WriteFile(path, []byte("old\n"), 0o600)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := r.Header.Get("Authorization")
		seen = append(seen, a)
		if a != "Bearer new" {
			w.WriteHeader(401)
			return
		}
		b, _ := readAll(r)
		w.Write(b)
	}))
	defer srv.Close()
	ft := &FileToken{Path: path}
	c := &http.Client{Transport: &AuthTransport{Source: ft}}
	res, err := c.Post(srv.URL, "text/plain", strings.NewReader("body"))
	if err != nil || res.StatusCode != 401 {
		t.Fatalf("old token, unchanged file: %v %v", err, res)
	}
	os.WriteFile(path, []byte("new"), 0o600)
	res, err = c.Post(srv.URL, "text/plain", strings.NewReader("body"))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("after the file changed: %v %v", err, res)
	}
	b, _ := readAll(res)
	if string(b) != "body" {
		t.Fatalf("the body was resent: %q", b)
	}
	// 1: old → 401, reload finds the same token, no retry; 2: old → 401, reload → new, retried once
	if len(seen) != 3 || seen[1] != "Bearer old" || seen[2] != "Bearer new" {
		t.Fatalf("calls: %v", seen)
	}
	if _, err := Token("no-such-addon-" + time.Now().Format("150405")); err == nil {
		t.Fatal("a missing token file is an error")
	}
}

func readAll(x any) ([]byte, error) {
	var buf bytes.Buffer
	switch v := x.(type) {
	case *http.Request:
		_, err := buf.ReadFrom(v.Body)
		return buf.Bytes(), err
	case *http.Response:
		defer v.Body.Close()
		_, err := buf.ReadFrom(v.Body)
		return buf.Bytes(), err
	}
	return nil, nil
}

func TestPrefsAndStatus(t *testing.T) {
	r := httptest.NewRequest("GET", "/?theme=dark&lang=xx", nil)
	r.AddCookie(&http.Cookie{Name: "ol-lang", Value: "de"})
	th, la := Prefs(r)
	if th != "dark" || la != "de" {
		t.Fatalf("prefs: %q %q", th, la)
	}
	r = httptest.NewRequest("GET", "/", nil)
	if th, la := Prefs(r); th != "" || la != "" {
		t.Fatalf("empty prefs: %q %q", th, la)
	}
	var b bytes.Buffer
	s := StatusInfo{Running: true, State: "running", PID: 42, Version: "1.0"}
	if err := StatusJSON(&b, s); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil || m["schema"] != 1.0 || m["pid"] != 42.0 || m["running"] != true {
		t.Fatalf("status line: %s %v", b.String(), err)
	}
	if ExitCode(s) != 0 || ExitCode(StatusInfo{State: "stopped"}) != 3 || ExitCode(StatusInfo{State: "failed"}) != 1 || ExitCode(StatusInfo{}) != 4 {
		t.Fatal("exit codes")
	}
	l, err := Listen(0, "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if !strings.HasPrefix(l.Addr().String(), "127.0.0.1:") {
		t.Fatalf("loopback only: %s", l.Addr())
	}
}
