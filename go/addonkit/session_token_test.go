package addonkit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"
)

// openccu-lite's gate lets a program in with an API token of the scope addon:<id>
// and hands the token on in X-Occulite-Session (X-Occulite-Auth: token); the checker confirms it
// at /state and admits it for its own addon only.
func TestSessionAPIToken(t *testing.T) {
	const (
		good    = "olt_0123456789abcdef0123456789abcdef"
		full    = "olt_ffffffffffffffffffffffffffffffff"
		other   = "olt_00000000000000000000000000000000"
		expired = "olt_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		meta    = "olt_11111111111111111111111111111111"
		unknown = "olt_22222222222222222222222222222222"
		sid     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	)
	fl := fakelite.New()
	defer fl.Close()
	fl.Sessions[sid] = fakelite.Session{User: "alice", Level: "configure"}
	fl.APITokens[good] = fakelite.APIToken{Name: "grafana", Scopes: []string{"addon:history"}}
	fl.APITokens[full] = fakelite.APIToken{Name: "root", Scopes: []string{"*"}}
	fl.APITokens[other] = fakelite.APIToken{Name: "battery-only", Scopes: []string{"addon:battery", "rpc:read"}}
	fl.APITokens[expired] = fakelite.APIToken{Name: "old", Scopes: []string{"addon:history"}, Expires: time.Now().Add(-time.Minute)}
	fl.APITokens[meta] = fakelite.APIToken{Name: "meta", Scopes: []string{"meta:read", "system:read"}}

	lite := func() addonkit.Kind { return addonkit.OpenccuLite }
	seen := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := addonkit.FromRequest(r)
		w.Write([]byte(s.User + ":" + s.Level.String() + ":" + s.Token + ":" + s.ID + ":" + strings.Join(s.Scopes, ",")))
	})
	var refused []string
	opts := addonkit.SessionOptions{Base: fl.URL, Platform: lite, AddonID: "history", CacheTTL: -1,
		OnRefused: func(r *http.Request, reason string) { refused = append(refused, reason) }}
	h := addonkit.Session(seen, opts)
	admin := addonkit.Session(addonkit.Require(addonkit.LevelAdmin, seen), opts)
	get := func(h http.Handler, cred, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/grafana/", nil)
		r.Header.Set(addonkit.HeaderSession, cred)
		if auth != "" {
			r.Header.Set(addonkit.HeaderAuth, auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if w := get(h, good, "token"); w.Code != 200 || w.Body.String() != "token:grafana:user:grafana::addon:history" {
		t.Fatalf("an addon:history token: %d %s", w.Code, w.Body)
	}
	if w := get(h, good, ""); w.Code != 200 {
		t.Fatalf("a token without X-Occulite-Auth (the shape decides): %d %s", w.Code, w.Body)
	}
	if w := get(admin, good, "token"); w.Code != 403 {
		t.Fatalf("addon:<id> is user level, never admin: %d", w.Code)
	}
	if w := get(admin, full, "token"); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "token:root:admin:root:") {
		t.Fatalf("Full access is admin: %d %s", w.Code, w.Body)
	}
	for _, c := range []struct{ name, cred, auth string }{
		{"another addon's token", other, "token"},
		{"an expired token", expired, "token"},
		{"a token with only API scopes", meta, "token"},
		{"a token occulited does not know", unknown, "token"},
		{"a token the gate called a session", good, "session"},
		{"a session the gate called a token", sid, "token"},
		{"a token of the wrong shape", "olt_0123", "token"},
	} {
		if w := get(h, c.cred, c.auth); w.Code != 401 || !strings.Contains(w.Body.String(), "session-invalid") {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
	if !strings.Contains(strings.Join(refused, "\n"), "lacks the scope addon:history") {
		t.Errorf("the refusal names the missing scope: %q", refused)
	}
	// sessions work as before beside tokens
	if w := get(h, sid, "session"); w.Code != 200 || w.Body.String() != "alice:user::"+sid+":" {
		t.Fatalf("a session: %d %s", w.Code, w.Body)
	}
	// without AddonID no token is admitted (the kit's behaviour before)
	noID := addonkit.Session(seen, addonkit.SessionOptions{Base: fl.URL, Platform: lite})
	if w := get(noID, full, "token"); w.Code != 401 {
		t.Fatalf("no AddonID admits no token: %d", w.Code)
	}
	// on a classic CCU the header is a client's and is never trusted, a token's neither
	ccu := addonkit.Session(seen, addonkit.SessionOptions{Base: fl.URL, AddonID: "history", Platform: func() addonkit.Kind { return addonkit.CCU3 }})
	if w := get(ccu, good, "token"); w.Code != 401 {
		t.Fatalf("CCU must refuse the header: %d", w.Code)
	}
}

func TestSessionAPITokenCache(t *testing.T) {
	const tok = "olt_0123456789abcdef0123456789abcdef"
	fl := fakelite.New()
	defer fl.Close()
	fl.APITokens[tok] = fakelite.APIToken{Name: "grafana", Scopes: []string{"addon:history"}}
	c := addonkit.NewChecker(addonkit.SessionOptions{Base: fl.URL, AddonID: "history", Platform: func() addonkit.Kind { return addonkit.OpenccuLite }})
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(addonkit.HeaderSession, tok)
	r.Header.Set(addonkit.HeaderAuth, "token")
	if _, err := c.Check(r.Context(), r); err != nil {
		t.Fatal(err)
	}
	delete(fl.APITokens, tok)
	if _, err := c.Check(r.Context(), r); err != nil {
		t.Fatalf("a confirmed token is cached like a session: %v", err)
	}
	c.Forget(tok)
	if _, err := c.Check(r.Context(), r); err == nil {
		t.Fatal("after Forget the deleted token is asked again and refused")
	}
}

func TestTokenLevel(t *testing.T) {
	for _, c := range []struct {
		scopes []string
		want   addonkit.Level
	}{
		{nil, addonkit.LevelUnknown},
		{[]string{"rpc:read", "meta:read"}, addonkit.LevelUnknown},
		{[]string{"addon:battery"}, addonkit.LevelUnknown},
		{[]string{"addon:history"}, addonkit.LevelUser},
		{[]string{"addon:history", "system:write"}, addonkit.LevelUser},
		{[]string{"addon:history", "*"}, addonkit.LevelAdmin},
		{[]string{"*"}, addonkit.LevelAdmin},
	} {
		if got := addonkit.TokenLevel("history", c.scopes); got != c.want {
			t.Errorf("%v: %s, want %s", c.scopes, got, c.want)
		}
	}
}
