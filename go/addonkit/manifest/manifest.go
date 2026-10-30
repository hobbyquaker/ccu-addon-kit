// Package manifest is the subset of openccu-lite's addon manifest (openccu-lite.json, occulited's
// docs/manifest-format.md) a Go addon declares, with a validator for the addon's own tests: the
// rules the system applies at install, so a package that would be refused or shown as
// "undeclared" fails the test instead.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path"
	"regexp"
	"strings"
)

// Text is a translated text: {"de": …, "en": …} or one string.
type Text map[string]string

// UnmarshalJSON accepts a string as {"en": s}.
func (t *Text) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = Text{"en": s}
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*t = m
	return nil
}

// MarshalJSON writes a one-language text as a string.
func (t Text) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		if s, ok := t["en"]; ok {
			return json.Marshal(s)
		}
	}
	return json.Marshal(map[string]string(t))
}

// Manifest is openccu-lite.json.
type Manifest struct {
	Format      int    `json:"format"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description Text   `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	// Changelog is an http(s) URL of the release notes (a CHANGELOG.md): the Addons page links it
	// beside an offered update instead of the release's page.
	Changelog string    `json:"changelog,omitempty"`
	Licence   string    `json:"licence,omitempty"`
	Version   string    `json:"version,omitempty"`
	Release   *Release  `json:"release,omitempty"`
	Requires  *Requires `json:"requires,omitempty"`
	UI        *UI       `json:"ui,omitempty"`
	Runtime   *Runtime  `json:"runtime,omitempty"`
}

// Release says where the releases are.
type Release struct {
	GitHub        string            `json:"github"`
	Asset         string            `json:"asset,omitempty"`
	Assets        map[string]string `json:"assets,omitempty"`
	FallbackAsset string            `json:"fallback_asset,omitempty"`
}

// Requires is what the addon needs of the system.
type Requires struct {
	Architectures []string `json:"architectures,omitempty"`
	Lite          string   `json:"lite,omitempty"`
	Forms         []string `json:"forms,omitempty"`
	Rega          bool     `json:"rega,omitempty"`
}

// UI is the web integration block.
type UI struct {
	Icon          string `json:"icon,omitempty"`
	IconDark      string `json:"icon_dark,omitempty"`
	Logo          string `json:"logo,omitempty"`
	LogoDark      string `json:"logo_dark,omitempty"`
	SettingsURL   string `json:"settings_url,omitempty"`
	SessionHeader bool   `json:"session_header,omitempty"`
	OwnUpdater    bool   `json:"own_updater,omitempty"`
}

// Runtime is what the unit is built from.
type Runtime struct {
	Root         bool                `json:"root,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty"`
	Groups       []string            `json:"groups,omitempty"`
	DataDirs     []string            `json:"data_dirs,omitempty"`
	Paths        []string            `json:"paths,omitempty"`
	Ports        []int               `json:"ports,omitempty"`
	PortInfo     map[string]PortInfo `json:"port_info,omitempty"`
	Needs        []string            `json:"needs"`
	Start        string              `json:"start,omitempty"`
	Daemon       bool                `json:"daemon,omitempty"`
	APIScopes    []string            `json:"api_scopes,omitempty"`
	Note         Text                `json:"note,omitempty"`
	// WebIngress: the addon brings connections from outside into the web server (a VPN or tunnel
	// addon that ends the tunnel in its own process). The system gives it lighttpd's PROXY-protocol
	// Unix socket /run/lighttpd-addons/<id>/proxy.sock (www-data:addon-<id>, 0750) and accepts the
	// client address the addon claims only inside Ranges.
	WebIngress *WebIngress `json:"web_ingress,omitempty"`
}

// WebIngress is an addon's declaration of a web ingress.
type WebIngress struct {
	// Ranges are the address ranges (CIDR, IPv4 or IPv6) the addon's clients have: what it may
	// claim as a request's address. Never the loopback, never everything.
	Ranges []string `json:"ranges"`
}

// Prefixes parses the ingress ranges with the system's rules: 1 to 8 ranges, each a canonical
// CIDR range (the network address), no wider than /8 (IPv4) or /32 (IPv6), an IPv4 range written
// as IPv4, and none that holds the loopback, the unspecified address, link-local or multicast
// addresses. A nil ingress has no prefixes and no error.
func (w *WebIngress) Prefixes() ([]netip.Prefix, error) {
	if w == nil {
		return nil, nil
	}
	if len(w.Ranges) == 0 || len(w.Ranges) > 8 {
		return nil, errors.New("runtime.web_ingress.ranges: 1 to 8 ranges")
	}
	out := make([]netip.Prefix, 0, len(w.Ranges))
	for _, s := range w.Ranges {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("runtime.web_ingress.ranges %q is not a CIDR range", s)
		}
		if p.Masked() != p {
			return nil, fmt.Errorf("runtime.web_ingress.ranges %q is not a network address (use %s)", s, p.Masked())
		}
		a := p.Addr()
		if a.Is4() && p.Bits() < 8 || a.Is6() && p.Bits() < 32 {
			return nil, fmt.Errorf("runtime.web_ingress.ranges %q is too wide", s)
		}
		if a.Is4In6() {
			return nil, fmt.Errorf("runtime.web_ingress.ranges %q: write an IPv4 range as IPv4", s)
		}
		for _, bad := range deniedIngress {
			if bad.Overlaps(p) {
				return nil, fmt.Errorf("runtime.web_ingress.ranges %q overlaps %s, which an ingress may never claim", s, bad)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// deniedIngress are the ranges no web ingress may hand out as a client's address: the loopback
// (the system trusts it), the unspecified address, link-local and multicast, and IPv4-mapped IPv6.
var deniedIngress = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("::ffff:0:0/96"),
}

// PortInfo describes one declared port.
type PortInfo struct {
	Protocol string `json:"protocol,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	Label    Text   `json:"label,omitempty"`
}

// MarshalJSON keeps `needs: []` in the output (an absent key means "after rfd and HMIPServer",
// an empty list "right after the network").
func (r Runtime) MarshalJSON() ([]byte, error) {
	type plain Runtime
	p := plain(r)
	if p.Needs == nil {
		p.Needs = []string{}
	}
	return json.Marshal(p)
}

// Load reads and validates a manifest file.
func Load(file string) (*Manifest, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes and validates a manifest.
func Parse(b []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

var idShape = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// urlShape is the system's rule for the manifest's links.
var urlShape = regexp.MustCompile(`^https?://[^\s"'<>]+$`)

// Scopes an addon may declare for its own token; the system drops anything else at the mint.
var Scopes = []string{"meta:read", "meta:write", "system:read", "logs:read", "system:write", "addons:write", "led",
	"rpc:read", "rpc:operate", "rpc:configure", "rpc:admin"}

// RootCapabilities are refused for a confined addon (ch. 11): root in effect.
var RootCapabilities = []string{"CAP_SYS_ADMIN", "CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_PTRACE", "CAP_SYS_CHROOT",
	"CAP_SYS_BOOT", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH", "CAP_FOWNER", "CAP_CHOWN", "CAP_SETUID", "CAP_SETGID",
	"CAP_SETPCAP", "CAP_MKNOD", "CAP_BPF", "CAP_MAC_ADMIN", "CAP_MAC_OVERRIDE", "CAP_NET_ADMIN"}

// sharedTrees are the parts of /usr/local a data_dirs entry may never name.
var sharedTrees = []string{"addons", "etc", "tmp", "backup", "crontabs", "lost+found", "var", "sdcard", "usb"}

// Validate applies the format's rules: format 1, an id of the rc.d shape, a name; release,
// requires, ui and runtime as the system accepts them. A manifest that passes may still be shown
// as "undeclared" (Undeclared says so).
func (m *Manifest) Validate() error {
	var errs []error
	fail := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	if m.Format != 1 {
		fail("format must be 1, got %d", m.Format)
	}
	if !idShape.MatchString(m.ID) {
		fail("id %q is not an rc.d name (lower-case letters, digits, _ and -)", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		fail("name is empty")
	}
	if m.Changelog != "" && !urlShape.MatchString(m.Changelog) {
		fail("changelog %q is not an http(s) URL", m.Changelog)
	}
	if m.Release != nil {
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(m.Release.GitHub) {
			fail("release.github %q is not owner/repo", m.Release.GitHub)
		}
		if m.Release.Asset == "" && len(m.Release.Assets) == 0 && m.Release.FallbackAsset == "" {
			fail("release names no asset")
		}
		if m.Release.Asset != "" && !strings.Contains(m.Release.Asset, "{version}") {
			fail("release.asset %q has no {version}", m.Release.Asset)
		}
	}
	if m.Requires != nil {
		for _, a := range m.Requires.Architectures {
			if a != "armv7l" && a != "aarch64" && a != "x86_64" {
				fail("requires.architectures: unknown %q", a)
			}
		}
		for _, f := range m.Requires.Forms {
			if f != "sd" && f != "ova" {
				fail("requires.forms: unknown %q", f)
			}
		}
	}
	if m.UI != nil {
		for k, p := range map[string]string{"icon": m.UI.Icon, "icon_dark": m.UI.IconDark, "logo": m.UI.Logo, "logo_dark": m.UI.LogoDark} {
			if p == "" {
				continue
			}
			if path.IsAbs(p) || strings.Contains(p, "..") || path.Clean(p) != p {
				fail("ui.%s %q must be a clean relative path inside the package", k, p)
			}
		}
	}
	if r := m.Runtime; r != nil {
		for _, n := range r.Needs {
			if n != "rfd" && n != "hmipserver" && n != "hs485d" {
				fail("runtime.needs: unknown %q", n)
			}
		}
		if r.Start != "" && r.Start != "early" {
			fail("runtime.start must be \"early\" or absent, got %q", r.Start)
		}
		for _, p := range r.Ports {
			if p < 1 || p > 65535 {
				fail("runtime.ports: %d out of range", p)
			}
		}
		for _, s := range r.APIScopes {
			if !contains(Scopes, s) {
				fail("runtime.api_scopes: %q is never granted", s)
			}
		}
		if !r.Root {
			for _, c := range r.Capabilities {
				if contains(RootCapabilities, c) {
					fail("runtime.capabilities: %s is refused for a confined addon", c)
				}
			}
			for _, g := range r.Groups {
				if g == "occulite" || g == "root" {
					fail("runtime.groups: %s is refused for a confined addon", g)
				}
			}
		}
		for _, d := range r.DataDirs {
			if err := checkDataDir(d, m.ID); err != nil {
				fail("runtime.data_dirs: %v", err)
			}
		}
		for _, p := range r.Paths {
			if !path.IsAbs(p) || path.Clean(p) != p {
				fail("runtime.paths: %q is not a clean absolute path", p)
			}
		}
		if _, err := r.WebIngress.Prefixes(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func checkDataDir(d, id string) error {
	if !path.IsAbs(d) || path.Clean(d) != d {
		return fmt.Errorf("%q is not a clean absolute path", d)
	}
	rel, ok := strings.CutPrefix(d, "/usr/local/")
	if !ok || rel == "" {
		return fmt.Errorf("%q is not under /usr/local/", d)
	}
	first := strings.SplitN(rel, "/", 2)[0]
	if strings.HasPrefix(first, ".") || contains(sharedTrees, first) {
		return fmt.Errorf("%q names a shared tree of /usr/local", d)
	}
	return nil
}

// Undeclared says whether the system would show the addon as "undeclared": no runtime block, or
// one that says only the start order (needs, start, note). A web ingress is a declaration.
func (m *Manifest) Undeclared() bool {
	r := m.Runtime
	if r == nil {
		return true
	}
	return !r.Root && len(r.Capabilities) == 0 && len(r.Groups) == 0 && len(r.DataDirs) == 0 && len(r.Paths) == 0 &&
		len(r.Ports) == 0 && !r.Daemon && len(r.APIScopes) == 0 && r.WebIngress == nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
