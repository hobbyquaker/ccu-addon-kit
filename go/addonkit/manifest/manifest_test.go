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
		`{"format": 1, "id": "x", "name": "X"}`:                                                  true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"needs": ["rfd"], "start": "early"}}`: true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {}}`:                                   true,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"daemon": true}}`:                     false,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"api_scopes": ["meta:read"]}}`:        false,
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
