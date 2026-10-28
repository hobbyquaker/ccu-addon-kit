package addonkit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"
)

// openccu-lite's public mode (the Control app's kiosk): the gate lets an anonymous GET/HEAD
// under an addon view's path through with X-Occulite-Auth: public and no session header. The
// checker admits it as the read-only viewer on the routes the addon marks public, and nowhere
// else; nothing is asked of occulited.
func TestSessionPublic(t *testing.T) {
	const sid = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	fl := fakelite.New()
	defer fl.Close()
	fl.Sessions[sid] = fakelite.Session{User: "alice", Level: "configure"}

	lite := func() addonkit.Kind { return addonkit.OpenccuLite }
	ccu := func() addonkit.Kind { return addonkit.CCU3 }
	seen := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := addonkit.FromRequest(r)
		pub := "session"
		if s.Public {
			pub = "public"
		}
		w.Write([]byte(pub + ":" + s.User + ":" + s.Level.String() + ":" + s.ID))
	})
	marked := func(r *http.Request) bool {
		p := r.URL.Path
		return strings.HasPrefix(p, "/addons/x/embed/") || p == "/addons/x/api/v1/dashboards"
	}
	var refused []string
	opts := addonkit.SessionOptions{Base: fl.URL, Platform: lite, Need: addonkit.LevelUser, Public: marked, CacheTTL: -1,
		OnRefused: func(r *http.Request, reason string) { refused = append(refused, reason) }}
	h := addonkit.Session(seen, opts)
	user := addonkit.Session(addonkit.Require(addonkit.LevelUser, seen), opts)
	do := func(h http.Handler, method, path, cred, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if cred != "" {
			r.Header.Set(addonkit.HeaderSession, cred)
		}
		if auth != "" {
			r.Header.Set(addonkit.HeaderAuth, auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	// a marked route: the viewer, a guest, below the handler's Need of user
	for _, m := range []string{"GET", "HEAD"} {
		if w := do(h, m, "/addons/x/embed/dashboard/d1", "", "public"); w.Code != 200 || (m == "GET" && w.Body.String() != "public::guest:") {
			t.Fatalf("%s on a marked route: %d %s", m, w.Code, w.Body)
		}
	}
	if w := do(h, "GET", "/addons/x/api/v1/dashboards", "", "public"); w.Code != 200 {
		t.Fatalf("the marked read: %d %s", w.Code, w.Body)
	}
	// a session on the same route is still the session
	if w := do(h, "GET", "/addons/x/api/v1/dashboards", sid, "session"); w.Code != 200 || w.Body.String() != "session:alice:user:"+sid {
		t.Fatalf("a session on a marked route: %d %s", w.Code, w.Body)
	}
	// a route that needs more than the guest inside a marked one: Require still refuses it
	if w := do(user, "GET", "/addons/x/embed/dashboard/d1", "", "public"); w.Code != 403 {
		t.Fatalf("Require(user) for the viewer: %d", w.Code)
	}
	refused = nil
	for _, c := range []struct{ name, method, path string }{
		{"an unmarked route", "GET", "/addons/x/api/v1/settings"},
		{"an unmarked export", "GET", "/addons/x/api/v1/export"},
		{"a write on a marked route", "PUT", "/addons/x/api/v1/dashboards"},
		{"a POST on a marked route", "POST", "/addons/x/embed/dashboard/d1"},
		{"a DELETE on a marked route", "DELETE", "/addons/x/api/v1/dashboards"},
	} {
		if w := do(h, c.method, c.path, "", "public"); w.Code != 401 || !strings.Contains(w.Body.String(), "session-invalid") {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
	if len(refused) != 5 || !strings.Contains(refused[0], "not a public route") || !strings.Contains(refused[2], "PUT by the public mode's viewer") {
		t.Fatalf("the refusals: %q", refused)
	}
	// without the public header no session is no session, also on a marked route
	if w := do(h, "GET", "/addons/x/embed/dashboard/d1", "", ""); w.Code != 401 {
		t.Fatalf("no header on a marked route: %d", w.Code)
	}
	// another auth word without a session header is no session
	if w := do(h, "GET", "/addons/x/embed/dashboard/d1", "", "session"); w.Code != 401 {
		t.Fatalf("auth session without a session header: %d", w.Code)
	}
	// an addon that marks nothing admits no viewer (the behaviour before)
	none := addonkit.Session(seen, addonkit.SessionOptions{Base: fl.URL, Platform: lite, CacheTTL: -1})
	if w := do(none, "GET", "/addons/x/embed/dashboard/d1", "", "public"); w.Code != 401 {
		t.Fatalf("no Public option: %d", w.Code)
	}
	// on a CCU the header is never trusted
	onCCU := addonkit.Session(seen, addonkit.SessionOptions{Base: fl.URL, Platform: ccu, Public: marked, CacheTTL: -1})
	if w := do(onCCU, "GET", "/addons/x/embed/dashboard/d1", "", "public"); w.Code != 401 {
		t.Fatalf("public on a CCU: %d", w.Code)
	}
	// nothing goes to occulited for the viewer
	calls := 0
	count := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer count.Close()
	quiet := addonkit.Session(seen, addonkit.SessionOptions{Base: count.URL, Platform: lite, Public: marked, CacheTTL: -1})
	if w := do(quiet, "GET", "/addons/x/embed/dashboard/d1", "", "public"); w.Code != 200 || calls != 0 {
		t.Fatalf("the viewer asked occulited: %d, %d calls", w.Code, calls)
	}
}
