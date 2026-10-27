# go/ - the openccu-lite side (not started)

The Go module `github.com/hobbyquaker/ccu-addon-kit/go` will live here: `addonkit` (platform
detection, the session check against openccu-lite's `/api/auth/v1/state`, the same CSRF rules
as the Tcl side with the same `X-CCU-Addon-Request` header, the addon's API token, loopback
listening, manifest and lighttpd fragment helpers, the status line of the service convention)
and `litesource` (events, state and metadata from openccu-lite's APIs).

On openccu-lite the system's own addon page already does service control, logs and memory, so
this side is plumbing for an addon's own server, not a settings frame. It is written together
with the first addon that needs it; until then this directory holds only this note.
