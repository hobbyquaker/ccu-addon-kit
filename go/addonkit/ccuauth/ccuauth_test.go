package ccuauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
)

// a stand-in rega.exe: one live admin session, one user session, anything else unknown
func rega(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		b, _ := io.ReadAll(r.Body)
		s := string(b)
		if !strings.HasPrefix(s, "Write(system.GetSessionVarStr('") || !strings.HasSuffix(s, "'));") || len(s) != len("Write(system.GetSessionVarStr('')); ")+9 {
			t.Errorf("unexpected script %q", s)
		}
		out := ""
		switch {
		case strings.Contains(s, "'ADMIN00001'"):
			out = "1004;8;Admin;;;"
		case strings.Contains(s, "'USER000001'"):
			out = "1005;2;Hans;Hans;Meier;"
		}
		io.WriteString(w, out+"<xml><exec>/rega.exe</exec><sessionId></sessionId></xml>")
	}))
}

func TestConfirmAndHandler(t *testing.T) {
	var calls int32
	srv := rega(t, &calls)
	defer srv.Close()
	c := New("battery", Options{Rega: srv.URL, CacheTTL: time.Minute})
	ctx := context.Background()
	info, err := c.Confirm(ctx, "@ADMIN00001@")
	if err != nil || info.User != "Admin" || info.Level != addonkit.LevelAdmin || info.ID != "ADMIN00001" {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := c.Confirm(ctx, "@ADMIN00001@"); err != nil || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("the cache: %d calls", calls)
	}
	if _, err := c.Confirm(ctx, "NOPE000000"); err == nil {
		t.Fatal("unknown sid")
	}
	if _, err := c.Confirm(ctx, "'); Write(1); //"); err == nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatal("a bad shape never reaches ReGa")
	}
	if Bare("@abcdefghij@") != "abcdefghij" || Bare("abcdefghij") != "abcdefghij" || Bare("short") != "" {
		t.Fatal("Bare")
	}

	h := c.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := addonkit.FromRequest(r)
		io.WriteString(w, s.User+" "+s.Level.String())
	}))
	call := func(path, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "battery_sid", Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("/addons/battery/api/v1/devices", ""); w.Code != 401 || !strings.Contains(w.Body.String(), "session-invalid") {
		t.Fatalf("no session: %d %s", w.Code, w.Body)
	}
	w := call("/addons/battery/?sid=@USER000001@", "")
	if w.Code != 200 || w.Body.String() != "Hans user" {
		t.Fatalf("query sid: %d %s", w.Code, w.Body)
	}
	ck := w.Result().Cookies()
	if len(ck) != 1 || ck[0].Name != "battery_sid" || ck[0].Value != "USER000001" || ck[0].Path != "/addons/battery/" || !ck[0].HttpOnly {
		t.Fatalf("cookie: %+v", ck)
	}
	if w := call("/addons/battery/api/v1/devices", "USER000001"); w.Code != 200 || w.Body.String() != "Hans user" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("cookie sid: %d %s", w.Code, w.Body)
	}
	if w := call("/addons/battery/api/v1/devices", "NOPE000000"); w.Code != 401 {
		t.Fatalf("bad cookie: %d", w.Code)
	}
	// a stricter need
	adm := New("battery", Options{Rega: srv.URL, Need: addonkit.LevelAdmin}).Handler(http.NotFoundHandler())
	r := httptest.NewRequest("GET", "/x?sid=@USER000001@", nil)
	rec := httptest.NewRecorder()
	adm.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("user below admin: %d", rec.Code)
	}
	if info, ok := ParseSessionVar("1004;8;Admin;;;", "x"); !ok || info.Level != addonkit.LevelAdmin {
		t.Fatal("parse")
	}
	if _, ok := ParseSessionVar("", "x"); ok {
		t.Fatal("empty is no session")
	}
	if Output("abc<xml><exec>/rega.exe</exec></xml>") != "abc" {
		t.Fatal("Output")
	}
}
