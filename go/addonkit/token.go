package addonkit

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TokenDir is where openccu-lite writes every addon's own API token at every start:
// <id>.api, mode 0600, the addon's user, with the scopes of the manifest's runtime.api_scopes.
const TokenDir = "/run/occulite/addon-tokens"

// LocalTokenFile is the token every addon may read: scope meta:read only.
const LocalTokenFile = "/usr/local/etc/occulite/local-token"

// Token reads the addon's own token. ErrNoToken when the file is missing or empty (the addon
// declared no api_scopes, or this is not openccu-lite).
func Token(id string) (string, error) { return readToken(filepath.Join(TokenDir, id+".api")) }

// ErrNoToken is answered when there is no token file to read.
var ErrNoToken = errors.New("no addon token")

func readToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoToken, err)
	}
	t := strings.TrimSpace(string(b))
	if t == "" {
		return "", fmt.Errorf("%w: %s is empty", ErrNoToken, path)
	}
	return t, nil
}

// TokenSource hands out a bearer token and re-reads it on demand: the system mints a new token
// at every start, so a 401 after an occulited restart is answered by reading the file again.
type TokenSource interface {
	Token() (string, error)
	Reload() (string, error)
}

// FileToken is a TokenSource over one file.
type FileToken struct {
	Path string
	mu   sync.Mutex
	tok  string
}

// AddonToken is the FileToken of the addon's own token.
func AddonToken(id string) *FileToken { return &FileToken{Path: filepath.Join(TokenDir, id+".api")} }

// LocalToken is the FileToken of the system's meta:read token.
func LocalToken() *FileToken { return &FileToken{Path: LocalTokenFile} }

// Token answers the cached token, reading the file the first time.
func (f *FileToken) Token() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tok != "" {
		return f.tok, nil
	}
	t, err := readToken(f.Path)
	if err != nil {
		return "", err
	}
	f.tok = t
	return t, nil
}

// Reload reads the file again.
func (f *FileToken) Reload() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, err := readToken(f.Path)
	if err != nil {
		return "", err
	}
	f.tok = t
	return t, nil
}

// StaticToken is a TokenSource of one fixed string (a token the user pasted, a test).
type StaticToken string

func (s StaticToken) Token() (string, error)  { return string(s), nil }
func (s StaticToken) Reload() (string, error) { return string(s), nil }

// AuthTransport adds the source's token as Authorization: Bearer and, on a 401, reloads it and
// sends the request once more — the token changed under a running addon when occulited restarted.
type AuthTransport struct {
	Source TokenSource
	Base   http.RoundTripper // http.DefaultTransport when nil
}

// RoundTrip implements http.RoundTripper. A request that already carries an Authorization
// header is sent as it is.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Header.Get("Authorization") != "" || t.Source == nil {
		return base.RoundTrip(req)
	}
	tok, err := t.Source.Token()
	if err != nil {
		return nil, err
	}
	r1 := req.Clone(req.Context())
	r1.Header.Set("Authorization", "Bearer "+tok)
	res, err := base.RoundTrip(r1)
	if err != nil || res.StatusCode != http.StatusUnauthorized || req.Body != nil && req.GetBody == nil {
		return res, err
	}
	fresh, rerr := t.Source.Reload()
	if rerr != nil || fresh == tok {
		return res, nil
	}
	res.Body.Close()
	r2 := req.Clone(req.Context())
	if req.GetBody != nil {
		if r2.Body, err = req.GetBody(); err != nil {
			return nil, err
		}
	}
	r2.Header.Set("Authorization", "Bearer "+fresh)
	return base.RoundTrip(r2)
}

// Client is an http.Client that speaks to occulited with the addon's own token (Token(id)),
// re-read on a 401; a 10 s timeout, which a streaming request overrides with its own context.
func Client(id string) *http.Client {
	return &http.Client{Transport: &AuthTransport{Source: AddonToken(id)}, Timeout: 10 * time.Second}
}
