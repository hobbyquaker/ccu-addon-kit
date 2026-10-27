// Package fragment renders the lighttpd fragment an addon with its own HTTP server ships as
// etc/lighttpd.conf in its tree (handbook ch. 11, "Your own HTTP server"): the redirect of the
// bare /addons/<id> to /addons/<id>/, and the proxy of /addons/<id>/… to the addon's loopback
// port, with the CGIs the addon keeps for lighttpd excluded. Only directives occulited's
// validator accepts are written.
package fragment

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Fragment describes one addon's rule.
type Fragment struct {
	// ID is the addon id; the rule covers /addons/<id>/.
	ID string
	// Port is the loopback port the daemon listens on (addonkit.Listen).
	Port int
	// Socket is a unix socket in the addon's tree instead of a port.
	Socket string
	// Exclude names files under /addons/<id>/ lighttpd keeps serving itself (settings.cgi,
	// service.cgi); a negative lookahead leaves them out of the proxy.
	Exclude []string
	// WebSocket enables the upgrade header, which the CCU's lighttpd and openccu-lite's pass.
	WebSocket bool
	// ErrorfilePrefix is server.errorfile-prefix inside the addon's tree, for the 503 page
	// while the daemon is not up; empty writes none.
	ErrorfilePrefix string
	// Comment is written at the top, one # per line; empty writes the default.
	Comment string
}

var idShape = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Render writes the fragment.
func Render(f Fragment) (string, error) {
	if !idShape.MatchString(f.ID) {
		return "", fmt.Errorf("fragment: id %q is not an addon id", f.ID)
	}
	if f.Socket == "" && (f.Port < 1 || f.Port > 65535) {
		return "", fmt.Errorf("fragment: port %d out of range", f.Port)
	}
	if f.Socket != "" && !strings.HasPrefix(f.Socket, "/usr/local/addons/"+f.ID+"/") {
		return "", fmt.Errorf("fragment: socket %q is not in the addon's tree", f.Socket)
	}
	for _, e := range f.Exclude {
		if strings.ContainsAny(e, "/ \t\"") || e == "" {
			return "", fmt.Errorf("fragment: exclude %q is not a file name", e)
		}
	}
	if f.ErrorfilePrefix != "" && !strings.HasPrefix(f.ErrorfilePrefix, "/usr/local/addons/"+f.ID+"/") {
		return "", fmt.Errorf("fragment: errorfile prefix %q is not in the addon's tree", f.ErrorfilePrefix)
	}
	var b strings.Builder
	comment := f.Comment
	if comment == "" {
		comment = fmt.Sprintf("The lighttpd rule of the %s addon: /addons/%s/ is served by the addon's own server on the loopback.\n"+
			"On openccu-lite the system makes its own validated copy of this file; on a CCU update_script copies it to\n"+
			"/usr/local/etc/config/lighttpd/%s.conf.", f.ID, f.ID, f.ID)
	}
	for _, line := range strings.Split(strings.TrimRight(comment, "\n"), "\n") {
		b.WriteString("# " + line + "\n")
	}
	fmt.Fprintf(&b, "\n$HTTP[\"url\"] == \"/addons/%s\" {\n    url.redirect = (\"^/addons/%s$\" => \"/addons/%s/\")\n}\n\n", f.ID, f.ID, f.ID)
	pattern := fmt.Sprintf("^/addons/%s/", f.ID)
	if len(f.Exclude) > 0 {
		ex := append([]string(nil), f.Exclude...)
		sort.Strings(ex)
		for i, e := range ex {
			ex[i] = regexp.QuoteMeta(e)
		}
		pattern += "(?!" + strings.Join(ex, "|") + ")"
	}
	fmt.Fprintf(&b, "$HTTP[\"url\"] =~ \"%s\" {\n", pattern)
	if f.Socket != "" {
		fmt.Fprintf(&b, "    proxy.server = (\"\" => ((\"socket\" => \"%s\")))\n", f.Socket)
	} else {
		fmt.Fprintf(&b, "    proxy.server = (\"\" => ((\"host\" => \"127.0.0.1\", \"port\" => %d)))\n", f.Port)
	}
	if f.WebSocket {
		b.WriteString("    proxy.header = (\"upgrade\" => \"enable\")\n")
	}
	if f.ErrorfilePrefix != "" {
		fmt.Fprintf(&b, "    server.errorfile-prefix = \"%s\"\n", f.ErrorfilePrefix)
	}
	b.WriteString("}\n")
	return b.String(), nil
}
