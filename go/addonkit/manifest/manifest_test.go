package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

const good = `{
  "format": 1, "id": "battery", "name": "Battery",
  "description": {"de": "Batterien", "en": "Batteries"},
  "homepage": "https://github.com/hobbyquaker/ccu-addon-battery", "licence": "MIT",
  "changelog": "https://github.com/hobbyquaker/ccu-addon-battery/blob/main/CHANGELOG.md",
  "release": {"github": "hobbyquaker/ccu-addon-battery", "asset": "battery-{arch}-{version}.tar.gz"},
  "requires": {"architectures": ["aarch64", "x86_64"]},
  "ui": {"icon": "battery/www/icon.svg", "session_header": true},
  "runtime": {"daemon": true, "needs": [], "api_scopes": ["rpc:read", "meta:read"], "groups": ["usbstorage"], "paths": ["/media"],
              "note": "records battery voltages"}
}`

func TestParseGood(t *testing.T) {
	m, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "battery" || m.Description["de"] != "Batterien" || m.Runtime.Note["en"] != "records battery voltages" {
		t.Fatalf("%+v", m)
	}
	if m.Changelog != "https://github.com/hobbyquaker/ccu-addon-battery/blob/main/CHANGELOG.md" {
		t.Fatalf("changelog: %q", m.Changelog)
	}
	if m.Undeclared() {
		t.Fatal("daemon: true is a declaration")
	}
	out, _ := json.Marshal(m)
	if !strings.Contains(string(out), `"needs":[]`) {
		t.Fatalf("needs: [] must survive: %s", out)
	}
	if !strings.Contains(string(out), `"note":"records battery voltages"`) {
		t.Fatalf("a one-language text is written as a string: %s", out)
	}
}

func TestValidateRefusals(t *testing.T) {
	cases := map[string]string{
		"format":       `{"format": 2, "id": "x", "name": "X"}`,
		"id":           `{"format": 1, "id": "Bad Id", "name": "X"}`,
		"name":         `{"format": 1, "id": "x", "name": " "}`,
		"unknown key":  `{"format": 1, "id": "x", "name": "X", "colour": "red"}`,
		"scope":        `{"format": 1, "id": "x", "name": "X", "runtime": {"api_scopes": ["*"]}}`,
		"capability":   `{"format": 1, "id": "x", "name": "X", "runtime": {"capabilities": ["CAP_NET_ADMIN"]}}`,
		"group":        `{"format": 1, "id": "x", "name": "X", "runtime": {"groups": ["occulite"]}}`,
		"data_dir":     `{"format": 1, "id": "x", "name": "X", "runtime": {"data_dirs": ["/usr/local/addons/y"]}}`,
		"data_dir top": `{"format": 1, "id": "x", "name": "X", "runtime": {"data_dirs": ["/usr/local/"]}}`,
		"needs":        `{"format": 1, "id": "x", "name": "X", "runtime": {"needs": ["rega"]}}`,
		"start":        `{"format": 1, "id": "x", "name": "X", "runtime": {"start": "late"}}`,
		"port":         `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [0]}}`,
		"icon":         `{"format": 1, "id": "x", "name": "X", "ui": {"icon": "/etc/passwd"}}`,
		"release":      `{"format": 1, "id": "x", "name": "X", "release": {"github": "nope"}}`,
		"asset":        `{"format": 1, "id": "x", "name": "X", "release": {"github": "a/b", "asset": "x.tar.gz"}}`,
		"changelog":    `{"format": 1, "id": "x", "name": "X", "changelog": "javascript:alert(1)"}`,
		"arch":         `{"format": 1, "id": "x", "name": "X", "requires": {"architectures": ["mips"]}}`,
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted %s", name, src)
		}
	}
	// a root addon may declare a root capability
	if _, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X", "runtime": {"root": true, "capabilities": ["CAP_SYS_ADMIN"]}}`)); err != nil {
		t.Error(err)
	}
	// a data dir of the addon's own under /usr/local is fine
	if _, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X", "runtime": {"data_dirs": ["/usr/local/x"]}}`)); err != nil {
		t.Error(err)
	}
}

func TestUndeclared(t *testing.T) {
	for src, want := range map[string]bool{
		`{"format": 1, "id": "x", "name": "X"}`:                                                           true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"needs": ["rfd"], "start": "early"}}`:          true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {}}`:                                            true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"daemon": true}}`:                              false,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"api_scopes": ["meta:read"]}}`:                 false,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"web_ingress": {"ranges": ["10.77.0.0/24"]}}}`: false,
	} {
		m, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if m.Undeclared() != want {
			t.Errorf("%s: undeclared %v", src, !want)
		}
	}
}

// runtime.web_ingress: the ranges an ingress addon may claim, with occulited's rules and messages.
func TestWebIngress(t *testing.T) {
	with := func(ranges string) string {
		return strings.Replace(good, `"needs": [],`, `"needs": [], "web_ingress": {"ranges": `+ranges+`},`, 1)
	}
	m, err := Parse([]byte(with(`["10.77.0.0/24", "fd7a:115c:a1e0::/48"]`)))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := m.Runtime.WebIngress.Prefixes()
	if err != nil || len(ps) != 2 || ps[0].String() != "10.77.0.0/24" || ps[1].String() != "fd7a:115c:a1e0::/48" {
		t.Fatalf("prefixes %v %v", ps, err)
	}
	if ps, err := (*WebIngress)(nil).Prefixes(); ps != nil || err != nil {
		t.Fatal("nil ingress")
	}
	out, _ := json.Marshal(m)
	if !strings.Contains(string(out), `"web_ingress":{"ranges":["10.77.0.0/24","fd7a:115c:a1e0::/48"]}`) {
		t.Fatalf("web_ingress must survive: %s", out)
	}
	for _, c := range []struct{ ranges, want string }{
		{`[]`, "1 to 8"},
		{`null`, "1 to 8"},
		{`["10.77.0.1/24"]`, "not a network address (use 10.77.0.0/24)"},
		{`["10.0.0.0/7"]`, "too wide"},
		{`["0.0.0.0/0"]`, "too wide"},
		{`["::/0"]`, "too wide"},
		{`["fd00::/16"]`, "too wide"},
		{`["127.0.0.0/24"]`, "overlaps 127.0.0.0/8, which an ingress may never claim"},
		{`["127.0.0.1/32"]`, "never claim"},
		{`["::1/128"]`, "never claim"},
		{`["0.0.0.0/8"]`, "never claim"},
		{`["169.254.0.0/16"]`, "never claim"},
		{`["fe80::/64"]`, "never claim"},
		{`["224.0.0.0/8"]`, "never claim"},
		{`["ff02::/32"]`, "never claim"},
		{`["::ffff:10.77.0.0/120"]`, "IPv4 range as IPv4"},
		{`["10.77.0.0"]`, "not a CIDR"},
		{`["x/24"]`, "not a CIDR"},
		{`["10.77.0.0/24", "127.0.0.0/8"]`, "never claim"},
		{`["10.1.0.0/24","10.2.0.0/24","10.3.0.0/24","10.4.0.0/24","10.5.0.0/24","10.6.0.0/24","10.7.0.0/24","10.8.0.0/24","10.9.0.0/24"]`, "1 to 8"},
	} {
		_, err := Parse([]byte(with(c.ranges)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.ranges, err, c.want)
		}
	}
	// 100.64.0.0/10 (Tailscale) and the private ranges a VPN hands out are allowed, 8 ranges too
	for _, ok := range []string{`["100.64.0.0/10"]`, `["192.168.77.0/24"]`, `["172.16.0.0/12"]`, `["fd7a:115c:a1e0::/48"]`,
		`["10.1.0.0/24","10.2.0.0/24","10.3.0.0/24","10.4.0.0/24","10.5.0.0/24","10.6.0.0/24","10.7.0.0/24","10.8.0.0/24"]`} {
		if _, err := Parse([]byte(with(ok))); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	// an unknown key inside the block is refused like anywhere else
	if _, err := Parse([]byte(strings.Replace(good, `"needs": [],`, `"needs": [], "web_ingress": {"ranges": ["10.77.0.0/24"], "port": 1},`, 1))); err == nil {
		t.Error("unknown key in web_ingress accepted")
	}
	// a manifest without it: nil
	if m, _ := Parse([]byte(good)); m.Runtime.WebIngress != nil {
		t.Fatal("web_ingress set without a declaration")
	}
}
