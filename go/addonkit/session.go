package addonkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level is the kit's user level, the same three the Tcl side maps every platform onto
// (docs/session.md): openccu-lite's `administer` is admin, `configure` and `operate` are user,
// `read` is guest; a session whose level is not known is unknown.
type Level int

const (
	LevelUnknown Level = iota
	LevelGuest
	LevelUser
	LevelAdmin
)

func (l Level) String() string {
	switch l {
	case LevelAdmin:
		return "admin"
	case LevelUser:
		return "user"
	case LevelGuest:
		return "guest"
	}
	return "unknown"
}

// LevelOf maps openccu-lite's level (else its role, for older systems) onto the kit's.
func LevelOf(level, role string) Level {
	switch level {
	case "administer":
		return LevelAdmin
	case "configure", "operate":
		return LevelUser
	case "read":
		return LevelGuest
	}
	switch role {
	case "admin":
		return LevelAdmin
	case "user":
		return LevelUser
	}
	return LevelUnknown
}

// Admits says whether a session of level have may do what needs need: `unknown` is admitted where
// `user` is needed and refused where `admin` is needed (reading keeps working on a system whose
// answer does not parse, writing does not open up); `guest` is admitted for guest only.
func Admits(need, have Level) bool {
	if have == LevelUnknown {
		return need <= LevelUser
	}
	return have >= need
}

// SessionInfo is what the gate's header told and /state confirmed.
type SessionInfo struct {
	// ID is the session id; empty for an API token (its secret is never handed on).
	ID   string
	User string
	// Token is the API token's name when the request came with one, "" for a session.
	Token string
	// Scopes are the token's scopes as /state answered them; nil for a session.
	Scopes []string
	Level  Level
}

// SessionOptions configure Session.
type SessionOptions struct {
	// Base is occulited's URL, http://127.0.0.1 by default.
	Base string
	// Need is the level every request through this handler needs; LevelUser by default. A route
	// that needs more wraps itself in Require.
	Need Level
	// Client makes the /state calls; http.DefaultClient with a 5 s timeout by default.
	Client *http.Client
	// Platform decides whether the header may be trusted at all: a CCU forwards a client's
	// header untouched. Platform() by default; a test sets it.
	Platform func() Kind
	// CacheTTL is how long a confirmed id is remembered, 10 s by default; 0 keeps the default,
	// a negative value disables the cache.
	CacheTTL time.Duration
	// OnRefused is called with the reason when a request is refused; nil logs nothing.
	OnRefused func(r *http.Request, reason string)
	// AddonID is the addon's id. On openccu-lite the gate also lets a program in with an API
	// token of the scope addon:<id> (or Full access, *) and hands the token on in the session
	// header; the checker admits it only when this is set and the token holds addon:<AddonID>
	// or *. "" admits no token, as before.
	AddonID string
}

// HeaderSession is the header lighttpd's gate adds to every request under /addons/ on openccu-lite.
const HeaderSession = "X-Occulite-Session"

// sessionIDShape is the two shapes openccu-lite ever sends: the 26-character base32 id, and the
// 10-character legacy alias. Nothing else is ever sent to /state.
var sessionIDShape = regexp.MustCompile(`^(?:[A-Z2-7]{26}|[A-Za-z0-9]{10})$`)

// tokenShape is an API token as occulited mints it and the gate accepts it: olt_ and 32 hex.
var tokenShape = regexp.MustCompile(`^olt_[0-9a-fA-F]{32}$`)

// HeaderAuth is the header the gate adds beside the session header: what it accepted, "session"
// or "token" (older systems send none).
const HeaderAuth = "X-Occulite-Auth"

type ctxKey struct{}

// FromRequest answers the session Session attached, nil outside it.
func FromRequest(r *http.Request) *SessionInfo {
	s, _ := r.Context().Value(ctxKey{}).(*SessionInfo)
	return s
}

// WithSession attaches a session to a request; for tests of handlers behind Session.
func WithSession(r *http.Request, s *SessionInfo) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, s))
}

// Checker confirms session ids against occulited and caches the answers.
type Checker struct {
	opts  SessionOptions
	mu    sync.Mutex
	cache map[string]cacheEntry
	order []string
}

type cacheEntry struct {
	info  SessionInfo
	until time.Time
}

const cacheSize = 64

// NewChecker makes a Checker; Session uses one per handler, an addon that checks sessions
// elsewhere (a WebSocket upgrade) shares one.
func NewChecker(opts SessionOptions) *Checker {
	if opts.Base == "" {
		opts.Base = "http://127.0.0.1"
	}
	if opts.Need == LevelUnknown {
		opts.Need = LevelUser
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if opts.Platform == nil {
		opts.Platform = Platform
	}
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 10 * time.Second
	}
	return &Checker{opts: opts, cache: map[string]cacheEntry{}}
}

// ErrNoSession is answered when the request carries no header the gate would have set.
var ErrNoSession = errors.New("no session header")

// ErrRefused is answered when occulited did not confirm the id or the token.
var ErrRefused = errors.New("session not confirmed")

// Check confirms the header's credential against GET /api/auth/v1/state. It trusts the header
// only on openccu-lite and sends only the shapes the gate hands on to /state: a session id (the
// answer must carry the same sid), or an API token when AddonID is set (the answer must carry no
// sid and the scope addon:<AddonID> or *; see TokenLevel for the level).
func (c *Checker) Check(ctx context.Context, r *http.Request) (*SessionInfo, error) {
	if c.opts.Platform() != OpenccuLite {
		return nil, fmt.Errorf("%w: not openccu-lite", ErrNoSession)
	}
	id := r.Header.Get(HeaderSession)
	if id == "" {
		return nil, ErrNoSession
	}
	auth := r.Header.Get(HeaderAuth)
	token := false
	switch {
	case sessionIDShape.MatchString(id) && auth != "token":
	case tokenShape.MatchString(id) && auth != "session":
		if c.opts.AddonID == "" {
			return nil, fmt.Errorf("%w: an API token, and this addon admits none", ErrRefused)
		}
		token = true
	default:
		return nil, fmt.Errorf("%w: id shape", ErrRefused)
	}
	if info, ok := c.cached(id); ok {
		return &info, nil
	}
	var info *SessionInfo
	var err error
	if token {
		info, err = c.confirmToken(ctx, id)
	} else {
		info, err = c.confirm(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	c.remember(id, *info)
	return info, nil
}

// TokenLevel is the kit's level of an API token at addon id: admin for Full access (*), user
// for the scope addon:<id> - it opens the addon's pages as a user's session would, never its
// admin routes - and unknown when it holds neither (the checker refuses those).
func TokenLevel(id string, scopes []string) Level {
	l := LevelUnknown
	for _, s := range scopes {
		switch s {
		case "*":
			return LevelAdmin
		case "addon:" + id:
			l = LevelUser
		}
	}
	return l
}

type stateAnswer struct {
	Authenticated bool     `json:"authenticated"`
	SID           string   `json:"sid"`
	User          string   `json:"user"`
	Level         string   `json:"level"`
	Role          string   `json:"role"`
	Scopes        []string `json:"scopes"`
}

func (c *Checker) state(ctx context.Context, cred string) (*stateAnswer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.Base+"/api/auth/v1/state", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred)
	res, err := c.opts.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: state answered %d", ErrRefused, res.StatusCode)
	}
	var a stateAnswer
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&a); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	return &a, nil
}

// confirmToken admits an API token: occulited knows it (authenticated, no sid - a session's
// answer is not a token's), and it holds this addon's scope or Full access.
func (c *Checker) confirmToken(ctx context.Context, tok string) (*SessionInfo, error) {
	a, err := c.state(ctx, tok)
	if err != nil {
		return nil, err
	}
	if !a.Authenticated || a.SID != "" {
		return nil, ErrRefused
	}
	level := TokenLevel(c.opts.AddonID, a.Scopes)
	if level == LevelUnknown {
		return nil, fmt.Errorf("%w: the token lacks the scope addon:%s", ErrRefused, c.opts.AddonID)
	}
	return &SessionInfo{User: a.User, Token: strings.TrimPrefix(a.User, "token:"), Scopes: a.Scopes, Level: level}, nil
}

func (c *Checker) confirm(ctx context.Context, id string) (*SessionInfo, error) {
	a, err := c.state(ctx, id)
	if err != nil {
		return nil, err
	}
	// the same sid, with or without the CCU's @ wrapping; a token answers no sid at all
	if !a.Authenticated || a.SID == "" || (a.SID != id && a.SID != "@"+id+"@") {
		return nil, ErrRefused
	}
	return &SessionInfo{ID: id, User: a.User, Level: LevelOf(a.Level, a.Role)}, nil
}

func (c *Checker) cached(id string) (SessionInfo, bool) {
	if c.opts.CacheTTL < 0 {
		return SessionInfo{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[id]
	if !ok || time.Now().After(e.until) {
		return SessionInfo{}, false
	}
	return e.info, true
}

func (c *Checker) remember(id string, info SessionInfo) {
	if c.opts.CacheTTL < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.cache[id]; !ok {
		c.order = append(c.order, id)
		for len(c.order) > cacheSize {
			delete(c.cache, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.cache[id] = cacheEntry{info: info, until: time.Now().Add(c.opts.CacheTTL)}
}

// Forget drops an id from the cache (after a logout the addon learned of).
func (c *Checker) Forget(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, id)
}

// Session wraps next so that every request carries a confirmed openccu-lite session of at least
// opts.Need. A request without one is answered 401 {"error":"session-invalid"} (the kit's frame
// shows its expiry overlay on that code), one below the level 403 {"error":"forbidden"}.
func Session(next http.Handler, opts SessionOptions) http.Handler {
	return NewChecker(opts).Handler(next)
}

// Handler is Session with a shared Checker.
func (c *Checker) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, err := c.Check(r.Context(), r)
		if err != nil {
			c.refuse(w, r, http.StatusUnauthorized, "session-invalid", err.Error())
			return
		}
		if !Admits(c.opts.Need, info.Level) {
			c.refuse(w, r, http.StatusForbidden, "forbidden", "level "+info.Level.String())
			return
		}
		next.ServeHTTP(w, WithSession(r, info))
	})
}

func (c *Checker) refuse(w http.ResponseWriter, r *http.Request, status int, code, reason string) {
	if c.opts.OnRefused != nil {
		c.opts.OnRefused(r, code+": "+reason)
	}
	WriteError(w, status, code, reason)
}

// Require refuses, with 403 forbidden, a request whose session (attached by Session) is below need.
func Require(need Level, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := FromRequest(r)
		if s == nil || !Admits(need, s.Level) {
			WriteError(w, http.StatusForbidden, "forbidden", "the level of this session does not allow it")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WriteError answers one JSON error the way the kit's CGIs do: {"error": code, "message": msg}.
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}
