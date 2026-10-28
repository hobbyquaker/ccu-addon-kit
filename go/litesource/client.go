// Package litesource is the Go port of occulite-client's protocol handling, as far as an addon's
// backend needs it: lite-rpc's event stream with resume and resync, the state store, the paramset
// cache, JSON-RPC calls to the interface processes, and the metadata API's snapshot and change
// stream — openccu-lite's APIs (occulited's docs/system-api.md and docs/meta-api.md) over the
// loopback with the addon's own token. The Source interface on top is the seam a classic-CCU
// implementation (XML-RPC callbacks, ReGa names) fills later.
package litesource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
)

// Client speaks to one openccu-lite system.
type Client struct {
	base string
	http *http.Client
	// Log receives one line per noteworthy event (a stream that ended, a resync); nil is silent.
	Log func(format string, args ...any)
	// Reconnect is the stream's backoff: 1 s doubling to 30 s by default.
	Reconnect struct{ Min, Max time.Duration }
	// Silence is how long a stream may say nothing before it is taken as dead; 45 s by default
	// (occulited pings every 15 s).
	Silence time.Duration
}

// New makes a client of base (http://127.0.0.1 by default) with the token source; the client
// re-reads the token on a 401 (addonkit.AuthTransport). A nil source sends no credential.
func New(base string, token addonkit.TokenSource) *Client {
	if base == "" {
		base = "http://127.0.0.1"
	}
	c := &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Transport: &addonkit.AuthTransport{Source: token}}}
	c.Reconnect.Min, c.Reconnect.Max = time.Second, 30*time.Second
	c.Silence = 45 * time.Second
	return c
}

// Base is the system's URL.
func (c *Client) Base() string { return c.base }

// HTTPError is an answer that was not 2xx.
type HTTPError struct {
	Status int
	Code   string
	Body   string
	// RetryAfter is the answer's Retry-After in seconds, 0 when it named none (openccu-lite's
	// lighttpd says 5 while occulited is not answering).
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("http %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("http %d", e.Status)
}

func (c *Client) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(format, args...)
	}
}

// do sends one request and decodes a JSON answer into out (nil skips the body).
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		he := &HTTPError{Status: res.StatusCode, Body: string(data)}
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil {
			he.Code = e.Error
		}
		if n, err := strconv.Atoi(strings.TrimSpace(res.Header.Get("Retry-After"))); err == nil && n > 0 {
			he.RetryAfter = time.Duration(n) * time.Second
		}
		return he
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Version is the open probe GET /api/meta/v1/version: the answer of an openccu-lite system.
type Version struct {
	API            string          `json:"api"`
	Version        int             `json:"version"`
	Revision       int64           `json:"revision"`
	Implementation string          `json:"implementation"`
	Commit         string          `json:"commit"`
	Capabilities   json.RawMessage `json:"capabilities"`
}

// ErrNotOpenccuLite is answered by Probe when the system answered something that is not the
// metadata API (a CCU: 404 or HTML).
var ErrNotOpenccuLite = errors.New("not openccu-lite")

// NotReadyError is Probe's answer when the system is there but does not answer the API yet: a
// 502, 503 or 504, or openccu-lite's {"error":"starting"} - lighttpd's answer while occulited is
// not up (at boot, during a restart). Retry, after RetryAfter when the answer named one.
type NotReadyError struct {
	Status     int
	Code       string
	RetryAfter time.Duration
}

func (e *NotReadyError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("the system is not ready yet (http %d %s)", e.Status, e.Code)
	}
	return fmt.Sprintf("the system is not ready yet (http %d)", e.Status)
}

// Probe tells openccu-lite (a Version), a CCU (ErrNotOpenccuLite) and a system that does not
// answer yet (a *NotReadyError for a 502/503/504 or {"error":"starting"}, any other error when it
// is unreachable: still booting, retry) apart. Only a 404, an HTML answer or a JSON answer that is
// not the metadata API say "not openccu-lite" - at boot, openccu-lite's lighttpd answers 503
// {"error":"starting"} until occulited is up, and that is not a CCU.
func (c *Client) Probe(ctx context.Context) (*Version, error) {
	var v Version
	err := c.do(ctx, http.MethodGet, "/api/meta/v1/version", nil, &v)
	var he *HTTPError
	if errors.As(err, &he) {
		switch {
		case he.Status == 502 || he.Status == 503 || he.Status == 504 || he.Code == "starting":
			return nil, &NotReadyError{Status: he.Status, Code: he.Code, RetryAfter: he.RetryAfter}
		case he.Status == 404 || looksHTML(he.Body):
			return nil, ErrNotOpenccuLite
		}
		return nil, he
	}
	if err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) {
			return nil, ErrNotOpenccuLite
		}
		return nil, err
	}
	if v.API != "meta" {
		return nil, ErrNotOpenccuLite
	}
	return &v, nil
}

// looksHTML says whether a body is an HTML document (a CCU's error page) rather than JSON.
func looksHTML(body string) bool {
	t := strings.ToLower(strings.TrimSpace(body))
	return strings.HasPrefix(t, "<!doctype html") || strings.HasPrefix(t, "<html")
}

// ProbeRetry is how long Await waits between probes when the system named no Retry-After; 5 s by
// default (a test shortens it).
var ProbeRetry = 5 * time.Second

// Await probes until the system answers the metadata API and returns its Version. A system that
// says it is not ready (NotReadyError) or does not answer at all is probed again after its
// Retry-After, else after ProbeRetry. ErrNotOpenccuLite ends the wait - unless lite is true: on a
// system the platform rule already recognised as openccu-lite (addonkit.IsOpenccuLite) the answer
// is a hiccup of its web server, never a CCU, so the wait goes on. The context's end returns
// ctx.Err(). Every reason is logged through Log when it first appears and then at most once a
// minute while it persists, so a long boot does not flood the journal.
func (c *Client) Await(ctx context.Context, lite bool) (*Version, error) {
	var lastMsg string
	var lastLog time.Time
	for {
		v, err := c.Probe(ctx)
		if err == nil {
			return v, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrNotOpenccuLite) && !lite {
			return nil, err
		}
		wait := ProbeRetry
		var nr *NotReadyError
		if errors.As(err, &nr) && nr.RetryAfter > 0 {
			wait = nr.RetryAfter
		}
		msg := err.Error()
		if errors.Is(err, ErrNotOpenccuLite) {
			msg = "the system does not answer its API yet (not openccu-lite?)"
		}
		if msg != lastMsg || time.Since(lastLog) >= time.Minute {
			c.logf("waiting for the system: %s", msg)
			lastMsg, lastLog = msg, time.Now()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Interface is one entry of GET /api/rpc/v1/interfaces.
type Interface struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	URLPath  string `json:"url_path"`
	Running  bool   `json:"running"`
}

// Interfaces lists the interface processes this system runs (needs rpc:read).
func (c *Client) Interfaces(ctx context.Context) ([]Interface, error) {
	var out []Interface
	err := c.do(ctx, http.MethodGet, "/api/rpc/v1/interfaces", nil, &out)
	return out, err
}
