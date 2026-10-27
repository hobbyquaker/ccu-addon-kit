// Package ccuauth confirms the WebUI session of a classic CCU (CCU3, OpenCCU) for a Go addon's
// own HTTP server, the way the kit's Tcl session.tcl does it: the WebUI hands the settings page
// (and, through the frame's app button, the application) a `?sid=@xxxxxxxxxx@`; ReGaHSS confirms
// the ten characters through `Write(system.GetSessionVarStr('<id>'))` over rega.exe on the
// loopback, whose answer is `uid;upl;user;first;last;` for a live session and "" otherwise
// (UPL 8 admin, 2 user, 1 guest). A page's later requests carry no query, so the handler keeps
// a confirmed sid in a cookie of the addon's own (HttpOnly, SameSite=Strict, the addon's path)
// and confirms it again every CacheTTL.
package ccuauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
)

// DefaultRega is ReGaHSS's script endpoint on every CCU.
const DefaultRega = "http://127.0.0.1:8181/rega.exe"

// Options configure the checker.
type Options struct {
	// Rega is rega.exe's URL; DefaultRega by default.
	Rega string
	// Need is the level every request needs; LevelUser by default (any WebUI session, the
	// kit's default on a CCU: a user-level session counts as user, a guest as guest).
	Need addonkit.Level
	// Cookie is the cookie's name, "<id>_sid"; CookiePath its path, "/addons/<id>/".
	Cookie     string
	CookiePath string
	// CacheTTL is how long a confirmed sid counts without asking ReGa again; 10 s by default.
	CacheTTL time.Duration
	Client   *http.Client
	// OnRefused is called with the reason when a request is refused; nil logs nothing.
	OnRefused func(r *http.Request, reason string)
}

// Checker confirms sids.
type Checker struct {
	opts  Options
	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	info  addonkit.SessionInfo
	until time.Time
}

// sidShape is what the WebUI hands over: @ + ten characters + @, or the bare ten.
var sidShape = regexp.MustCompile(`^@?([A-Za-z0-9]{10})@?$`)

// ErrNoSession: the request carries neither a sid nor the addon's cookie.
var ErrNoSession = errors.New("no session")

// ErrRefused: ReGa did not confirm the sid.
var ErrRefused = errors.New("session not confirmed")

// New makes a checker for the addon id.
func New(id string, o Options) *Checker {
	if o.Rega == "" {
		o.Rega = DefaultRega
	}
	if o.Need == addonkit.LevelUnknown {
		o.Need = addonkit.LevelUser
	}
	if o.Cookie == "" {
		o.Cookie = id + "_sid"
	}
	if o.CookiePath == "" {
		o.CookiePath = "/addons/" + id + "/"
	}
	if o.CacheTTL == 0 {
		o.CacheTTL = 10 * time.Second
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Checker{opts: o, cache: map[string]entry{}}
}

// Bare answers the ten characters of a sid in either shape, "" for anything else.
func Bare(sid string) string {
	m := sidShape.FindStringSubmatch(strings.TrimSpace(sid))
	if m == nil {
		return ""
	}
	return m[1]
}

// Confirm asks ReGa about a sid (cached for CacheTTL).
func (c *Checker) Confirm(ctx context.Context, sid string) (*addonkit.SessionInfo, error) {
	id := Bare(sid)
	if id == "" {
		return nil, fmt.Errorf("%w: id shape", ErrRefused)
	}
	c.mu.Lock()
	if e, ok := c.cache[id]; ok && time.Now().Before(e.until) {
		info := e.info
		c.mu.Unlock()
		return &info, nil
	}
	c.mu.Unlock()
	info, err := c.ask(ctx, id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.cache) > 64 {
		c.cache = map[string]entry{}
	}
	c.cache[id] = entry{info: *info, until: time.Now().Add(c.opts.CacheTTL)}
	c.mu.Unlock()
	return info, nil
}

// Forget drops a sid from the cache (a logout).
func (c *Checker) Forget(sid string) {
	c.mu.Lock()
	delete(c.cache, Bare(sid))
	c.mu.Unlock()
}

func (c *Checker) ask(ctx context.Context, id string) (*addonkit.SessionInfo, error) {
	// the ten checked characters are the only thing that ever goes into the script
	script := "Write(system.GetSessionVarStr('" + id + "'));"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.Rega, strings.NewReader(script))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	res, err := c.opts.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rega: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("rega: %s", res.Status)
	}
	info, ok := ParseSessionVar(Output(string(b)), id)
	if !ok {
		return nil, ErrRefused
	}
	return info, nil
}

// Output is what the script wrote: rega.exe appends an <xml> element with the variables.
func Output(body string) string {
	if i := strings.Index(body, "<xml>"); i >= 0 {
		return body[:i]
	}
	return body
}

// ParseSessionVar reads `uid;upl;user;first;last;`; ok false for an empty answer (no session).
func ParseSessionVar(s, id string) (*addonkit.SessionInfo, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ";")
	if len(parts) < 3 {
		return nil, false
	}
	level := addonkit.LevelUnknown
	switch strings.TrimSpace(parts[1]) {
	case "8":
		level = addonkit.LevelAdmin
	case "2":
		level = addonkit.LevelUser
	case "1":
		level = addonkit.LevelGuest
	}
	return &addonkit.SessionInfo{ID: id, User: strings.TrimSpace(parts[2]), Level: level}, true
}

// Check finds the request's sid - `?sid=` first, then the addon's cookie - and confirms it.
// It answers the sid as well, so the handler can set the cookie.
func (c *Checker) Check(ctx context.Context, r *http.Request) (*addonkit.SessionInfo, string, error) {
	sid := r.URL.Query().Get("sid")
	fromQuery := sid != ""
	if sid == "" {
		if ck, err := r.Cookie(c.opts.Cookie); err == nil {
			sid = ck.Value
		}
	}
	if sid == "" {
		return nil, "", ErrNoSession
	}
	info, err := c.Confirm(ctx, sid)
	if err != nil {
		return nil, "", err
	}
	if fromQuery {
		return info, Bare(sid), nil
	}
	return info, "", nil
}

// Handler refuses a request without a confirmed session (401 session-invalid, the frame shows
// its overlay on that code) or below Need (403 forbidden), attaches the session (addonkit.
// FromRequest) and sets the addon's cookie when the sid came with the query.
func (c *Checker) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, fresh, err := c.Check(r.Context(), r)
		if err != nil {
			c.refuse(w, r, http.StatusUnauthorized, "session-invalid", err.Error())
			return
		}
		if !addonkit.Admits(c.opts.Need, info.Level) {
			c.refuse(w, r, http.StatusForbidden, "forbidden", "level "+info.Level.String())
			return
		}
		if fresh != "" {
			http.SetCookie(w, &http.Cookie{Name: c.opts.Cookie, Value: fresh, Path: c.opts.CookiePath, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		next.ServeHTTP(w, addonkit.WithSession(r, info))
	})
}

func (c *Checker) refuse(w http.ResponseWriter, r *http.Request, status int, code, reason string) {
	if c.opts.OnRefused != nil {
		c.opts.OnRefused(r, code+": "+reason)
	}
	addonkit.WriteError(w, status, code, reason)
}
