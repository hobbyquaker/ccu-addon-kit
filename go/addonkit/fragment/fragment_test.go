package fragment

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	out, err := Render(Fragment{ID: "battery", Port: 8290, Exclude: []string{"settings.cgi", "service.cgi"}, WebSocket: true,
		ErrorfilePrefix: "/usr/local/addons/battery/www/lighttpd-error-"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`$HTTP["url"] == "/addons/battery" {`,
		`url.redirect = ("^/addons/battery$" => "/addons/battery/")`,
		`$HTTP["url"] =~ "^/addons/battery/(?!service\.cgi|settings\.cgi)" {`,
		`proxy.server = ("" => (("host" => "127.0.0.1", "port" => 8290)))`,
		`proxy.header = ("upgrade" => "enable")`,
		`server.errorfile-prefix = "/usr/local/addons/battery/www/lighttpd-error-"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	plain, err := Render(Fragment{ID: "x", Port: 1234})
	if err != nil || !strings.Contains(plain, `$HTTP["url"] =~ "^/addons/x/" {`) || strings.Contains(plain, "upgrade") {
		t.Fatalf("plain: %v\n%s", err, plain)
	}
	sock, err := Render(Fragment{ID: "x", Socket: "/usr/local/addons/x/var/x.sock"})
	if err != nil || !strings.Contains(sock, `("socket" => "/usr/local/addons/x/var/x.sock")`) {
		t.Fatalf("socket: %v\n%s", err, sock)
	}
	for _, bad := range []Fragment{{ID: "Bad", Port: 1}, {ID: "x", Port: 0}, {ID: "x", Socket: "/tmp/x.sock"},
		{ID: "x", Port: 1, Exclude: []string{"a/b"}}, {ID: "x", Port: 1, ErrorfilePrefix: "/usr/local/addons/y/e-"}} {
		if _, err := Render(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
