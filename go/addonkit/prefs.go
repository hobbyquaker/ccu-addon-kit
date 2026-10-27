package addonkit

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
)

// Prefs is what the shell hands an addon page about its look: theme=system|light|dark and
// lang=de|en on the frame URL, the ol-theme and ol-lang cookies for a page rendered on the
// server. Empty when neither says anything. The Svelte side uses occulite-ui's helpers; this is
// for server-rendered bits and for a redirect that must keep them.
func Prefs(r *http.Request) (theme, lang string) {
	theme = pick(r.URL.Query().Get("theme"), "system", "light", "dark")
	lang = pick(r.URL.Query().Get("lang"), "de", "en")
	if theme == "" {
		if c, err := r.Cookie("ol-theme"); err == nil {
			theme = pick(c.Value, "light", "dark")
		}
	}
	if lang == "" {
		if c, err := r.Cookie("ol-lang"); err == nil {
			lang = pick(c.Value, "de", "en")
		}
	}
	return theme, lang
}

func pick(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return ""
}

// Listen opens the addon's own server on the loopback only (D-29: nothing but lighttpd faces the
// LAN; the lighttpd fragment proxies /addons/<id>/… here). port 0 takes a free one; the address
// of the listener says which. A unix socket in the addon's tree is the alternative: pass a path
// starting with / as socket.
func Listen(port int, socket string) (net.Listener, error) {
	if socket != "" {
		_ = os.Remove(socket)
		l, err := net.Listen("unix", socket)
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(socket, 0o660); err != nil {
			l.Close()
			return nil, err
		}
		return l, nil
	}
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// StatusInfo is the service convention's status object (docs/service-convention.md): one JSON
// line, schema 1; running or not, with pid, RSS and start time while running, the version, and
// the last error when there is one.
type StatusInfo struct {
	Running   bool   `json:"running"`
	State     string `json:"state"` // running | stopped | failed | unknown
	PID       int    `json:"pid,omitempty"`
	RSSKB     int64  `json:"rss_kb,omitempty"`
	Since     int64  `json:"since,omitempty"` // Unix seconds
	Version   string `json:"version,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// StatusJSON writes the status line for a Go daemon that answers `bin/<id> status` itself, so
// the rc.d script can delegate `status` to it. The exit code the convention wants is ExitCode.
func StatusJSON(w interface{ Write([]byte) (int, error) }, s StatusInfo) error {
	type line struct {
		Schema int `json:"schema"`
		StatusInfo
	}
	b, err := json.Marshal(line{1, s})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

// ExitCode is the convention's exit code for a state: 0 running, 3 stopped, 1 failed, 4 unknown.
func ExitCode(s StatusInfo) int {
	switch s.State {
	case "running":
		return 0
	case "stopped":
		return 3
	case "failed":
		return 1
	}
	return 4
}
