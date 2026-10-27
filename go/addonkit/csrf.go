package addonkit

import (
	"net/http"
	"net/url"
	"strings"
)

// HeaderRequest is the header every writing request must carry (docs/session.md, CSRF): an HTML
// form on another site cannot set it, and a cross-origin fetch with it needs a preflight that
// OPTIONS refuses. The same name on both platforms, so one frontend helper serves both.
const HeaderRequest = "X-CCU-Addon-Request"

// CSRFReason says why CSRF refused a request, "" when it passed. The reasons are the Tcl side's:
// method, header, origin, fetch-site.
func CSRFReason(r *http.Request) string {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return ""
	case http.MethodOptions:
		return "method"
	}
	if r.Header.Get(HeaderRequest) == "" {
		return "header"
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || o == "null" || !strings.EqualFold(u.Host, r.Host) {
			return "origin"
		}
	}
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" && s != "none" {
		return "fetch-site"
	}
	return ""
}

// CSRF applies the kit's four checks to every request that is not a GET or HEAD: the request
// header, an Origin that names this host (port included, scheme not compared; `null` refused),
// a Sec-Fetch-Site of same-origin or none. OPTIONS answers 405 with no Access-Control-* header,
// so a cross-origin preflight never succeeds. A failure is 403 {"error":"csrf","message":reason}
// (405 method for OPTIONS). The level check is Session's.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch reason := CSRFReason(r); reason {
		case "":
			next.ServeHTTP(w, r)
		case "method":
			WriteError(w, http.StatusMethodNotAllowed, "method", "OPTIONS is not answered")
		default:
			WriteError(w, http.StatusForbidden, "csrf", reason)
		}
	})
}
