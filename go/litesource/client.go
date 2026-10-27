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

// Probe tells openccu-lite (a Version), a CCU (ErrNotOpenccuLite) and an unreachable system
// (any other error: still booting, retry) apart.
func (c *Client) Probe(ctx context.Context) (*Version, error) {
	var v Version
	err := c.do(ctx, http.MethodGet, "/api/meta/v1/version", nil, &v)
	var he *HTTPError
	if errors.As(err, &he) {
		return nil, ErrNotOpenccuLite
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
