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
              "note": "records battery voltages", "technical": {"de": "liest Geräte", "en": "reads devices"}}
}`

func TestParseGood(t *testing.T) {
	m, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "battery" || m.Description["de"] != "Batterien" || m.Runtime.Note["en"] != "records battery voltages" ||
		m.Runtime.Technical["en"] != "reads devices" {
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
		"format":        `{"format": 2, "id": "x", "name": "X"}`,
		"id":            `{"format": 1, "id": "Bad Id", "name": "X"}`,
		"name":          `{"format": 1, "id": "x", "name": " "}`,
		"unknown key":   `{"format": 1, "id": "x", "name": "X", "colour": "red"}`,
		"scope":         `{"format": 1, "id": "x", "name": "X", "runtime": {"api_scopes": ["*"]}}`,
		"scope power":   `{"format": 1, "id": "x", "name": "X", "runtime": {"api_scopes": ["power"]}}`,
		"capability":    `{"format": 1, "id": "x", "name": "X", "runtime": {"capabilities": ["CAP_NET_ADMIN"]}}`,
		"group":         `{"format": 1, "id": "x", "name": "X", "runtime": {"groups": ["occulite"]}}`,
		"data_dir":      `{"format": 1, "id": "x", "name": "X", "runtime": {"data_dirs": ["/usr/local/addons/y"]}}`,
		"data_dir top":  `{"format": 1, "id": "x", "name": "X", "runtime": {"data_dirs": ["/usr/local/"]}}`,
		"needs":         `{"format": 1, "id": "x", "name": "X", "runtime": {"needs": ["rega"]}}`,
		"start":         `{"format": 1, "id": "x", "name": "X", "runtime": {"start": "late"}}`,
		"port":          `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [0]}}`,
		"port_info":     `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": {"162": {"proto": "udp"}}}}`,
		"port_info key": `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": {"snmp": {}}}}`,
		"proto":         `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": {"161": {"proto": "sctp"}}}}`,
		"protocol key":  `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": {"161": {"protocol": "udp"}}}}`,
		"icon":          `{"format": 1, "id": "x", "name": "X", "ui": {"icon": "/etc/passwd"}}`,
		"release":       `{"format": 1, "id": "x", "name": "X", "release": {"github": "nope"}}`,
		"asset":         `{"format": 1, "id": "x", "name": "X", "release": {"github": "a/b", "asset": "x.tar.gz"}}`,
		"changelog":     `{"format": 1, "id": "x", "name": "X", "changelog": "javascript:alert(1)"}`,
		"arch":          `{"format": 1, "id": "x", "name": "X", "requires": {"architectures": ["mips"]}}`,
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

// runtime.port_info as occulited's manifest has it: keyed by a declared port, proto tcp or udp.
func TestPortInfo(t *testing.T) {
	src := `{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161, 8443], "port_info": {
	  "161": {"proto": "udp", "label": {"de": "SNMP-Agent", "en": "SNMP agent"}},
	  "8443": {"proto": "tcp", "tls": true}}}}`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if pi := m.Runtime.PortInfo["161"]; pi.Proto != "udp" || pi.Label["en"] != "SNMP agent" {
		t.Fatalf("%+v", pi)
	}
	if pi := m.Runtime.PortInfo["8443"]; pi.Proto != "tcp" || !pi.TLS {
		t.Fatalf("%+v", pi)
	}
	out, _ := json.Marshal(m)
	if !strings.Contains(string(out), `"161":{"proto":"udp"`) {
		t.Fatalf("proto must survive: %s", out)
	}
	for _, c := range []struct{ src, want string }{
		{`{"162": {"proto": "udp"}}`, `runtime.port_info "162" names no declared port`},
		{`{"snmp": {}}`, `runtime.port_info "snmp" names no declared port`},
		{`{"161": {"proto": "UDP"}}`, `proto must be "tcp" or "udp", got "UDP"`},
	} {
		_, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": ` + c.src + `}}`))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.want)
		}
	}
	// no proto is fine: it is informational
	if _, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X", "runtime": {"ports": [161], "port_info": {"161": {"tls": true}}}}`)); err != nil {
		t.Error(err)
	}
}

// The scope that powers the system off and nothing else (a UPS addon); a reboot stays with power,
// which an addon never gets.
func TestScopePowerHalt(t *testing.T) {
	if _, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X", "runtime": {"daemon": true, "needs": [], "api_scopes": ["power:halt"]}}`)); err != nil {
		t.Error(err)
	}
}

func TestBackupBlock(t *testing.T) {
	ok := map[string]string{
		"hook":         `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/backup-snapshot", "staging": "var/backup", "timeout": 120, "size_hint_mb": 50, "restore": "bin/backup-restore"}}`,
		"hook minimal": `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup"}}`,
		"targets":      `{"format": 1, "id": "sync", "name": "Sync", "backup": {"targets": {"protocol": 1, "label": {"de": "Cloud-Speicher", "en": "Cloud storage"}, "setup": "/addon-settings/sync#accounts"}}}`,
		"both":         `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "targets": {"protocol": 1, "label": "Bucket", "setup": "/addons/x/"}}}`,
		"timeout max":  `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "timeout": 1800}}`,
	}
	for name, src := range ok {
		if _, err := Parse([]byte(src)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string]string{
		"empty":            `{"format": 1, "id": "x", "name": "X", "backup": {}}`,
		"no staging":       `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap"}}`,
		"no hook":          `{"format": 1, "id": "x", "name": "X", "backup": {"staging": "backup"}}`,
		"absolute":         `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "/bin/snap", "staging": "backup"}}`,
		"dotdot":           `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/../snap", "staging": "backup"}}`,
		"dot segment":      `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/./snap", "staging": "backup"}}`,
		"trailing slash":   `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup/"}}`,
		"space":            `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/my snap", "staging": "backup"}}`,
		"hook in staging":  `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "var/backup/snap", "staging": "var/backup"}}`,
		"hook is staging":  `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "var", "staging": "var"}}`,
		"restore in stage": `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "var", "restore": "var/restore"}}`,
		"timeout low":      `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "timeout": 5}}`,
		"timeout high":     `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "timeout": 1801}}`,
		"size negative":    `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "size_hint_mb": -1}}`,
		"unknown key":      `{"format": 1, "id": "x", "name": "X", "backup": {"hook": "bin/snap", "staging": "backup", "paths": ["/x"]}}`,
		"targets+timeout":  `{"format": 1, "id": "x", "name": "X", "backup": {"timeout": 60, "targets": {"protocol": 1, "label": "B", "setup": "/addons/x/"}}}`,
		"protocol":         `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 2, "label": "B", "setup": "/addons/x/"}}}`,
		"label empty":      `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": {"en": " "}, "setup": "/addons/x/"}}}`,
		"label long":       `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "` + strings.Repeat("x", 41) + `", "setup": "/addons/x/"}}}`,
		"label newline":    `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "a\nb", "setup": "/addons/x/"}}}`,
		"setup site":       `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "B", "setup": "https://example.com/"}}}`,
		"setup elsewhere":  `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "B", "setup": "/system/backup"}}}`,
		"setup dotdot":     `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "B", "setup": "/addons/../system"}}}`,
		"setup double":     `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "B", "setup": "/addons//x"}}}`,
		"targets unknown":  `{"format": 1, "id": "x", "name": "X", "backup": {"targets": {"protocol": 1, "label": "B", "setup": "/addons/x/", "url": "x"}}}`,
	}
	for name, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted %s", name, src)
		}
	}
	m, err := Parse([]byte(`{"format": 1, "id": "h", "name": "H", "backup": {"hook": "bin/snap", "staging": "backup", "size_hint_mb": 101}}`))
	if err != nil || !m.Backup.Large() {
		t.Fatalf("101 MB is large: %v", err)
	}
	if (&Backup{SizeHintMB: 100}).Large() || (*Backup)(nil).Large() {
		t.Fatal("100 MB and no block are not large")
	}
}

func TestRuntimeBinAndTimers(t *testing.T) {
	src := `{"format": 1, "id": "sync", "name": "Sync", "runtime": {"needs": [], "bin": "bin",
		"timers": {"exec": "lite-sync", "args": ["run"]}, "read_addon_data": true, "api_scopes": ["system:read", "timers:addon"]}}`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Runtime.Bin != "bin" || m.Runtime.Timers.Exec != "lite-sync" || !m.Runtime.ReadAddonData {
		t.Fatalf("%+v", m.Runtime)
	}
	for _, src := range []string{
		`{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin"}}`,
		`{"format": 1, "id": "x", "name": "X", "runtime": {"read_addon_data": true}}`,
	} {
		m, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if m.Undeclared() {
			t.Errorf("%s is a declaration", src)
		}
	}
	bad := map[string]string{
		"bin absolute":  `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "/usr/bin"}}`,
		"bin dotdot":    `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "../bin"}}`,
		"bin trailing":  `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin/"}}`,
		"timers no bin": `{"format": 1, "id": "x", "name": "X", "runtime": {"timers": {"exec": "run"}}}`,
		"timers slash":  `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin", "timers": {"exec": "sub/run"}}}`,
		"timers dot":    `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin", "timers": {"exec": ".."}}}`,
		"timers args":   `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin", "timers": {"exec": "run", "args": ["a", "b", "c", "d", "e"]}}}`,
		"timers arg":    `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin", "timers": {"exec": "run", "args": ["$HOME"]}}}`,
		"timers space":  `{"format": 1, "id": "x", "name": "X", "runtime": {"bin": "bin", "timers": {"exec": "run", "args": ["a b"]}}}`,
	}
	for name, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted %s", name, src)
		}
	}
}

func TestSummaryAndTechnical(t *testing.T) {
	m, err := Parse([]byte(`{"format": 1, "id": "x", "name": "X",
  "summary": {"de": "Kurz", "en": "Short"},
  "runtime": {"needs": [], "technical": "reads nothing"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Summary["de"] != "Kurz" || m.Runtime.Technical["en"] != "reads nothing" || len(m.Warnings()) != 0 {
		t.Fatalf("%+v %v", m, m.Warnings())
	}
	// technical, like note, is no declaration
	if !m.Undeclared() {
		t.Error("a block of needs and technical is undeclared")
	}
	// too long or more than one line: accepted, with a warning (the system cuts it, never refuses)
	long := strings.Repeat("ä", SummaryMax+1)
	m, err = Parse([]byte(`{"format": 1, "id": "x", "name": "X", "summary": {"de": "` + long + `", "en": "two\nlines"}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := m.Warnings()
	if len(w) != 2 || !strings.Contains(w[0], "summary.de has 81 characters") || !strings.Contains(w[1], "summary.en has more than one line") {
		t.Fatalf("warnings: %q", w)
	}
	// exactly at the limit: no warning
	m, _ = Parse([]byte(`{"format": 1, "id": "x", "name": "X", "summary": "` + strings.Repeat("a", SummaryMax) + `"}`))
	if len(m.Warnings()) != 0 {
		t.Fatalf("warnings at the limit: %q", m.Warnings())
	}
}
