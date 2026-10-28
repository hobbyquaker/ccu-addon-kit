# Session, levels, CSRF

`lib/kit/session.tcl` is the one session check for classic addons. It is standalone (it sources
nothing of the kit), so an existing addon can take it before it takes the frame. Tcl 8.2.

```tcl
source $KIT_HOME/lib/kit/session.tcl

kit_platform              ;# -> ccu3 | openccu | openccu-lite
kit_is_openccu_lite       ;# -> 1/0, evaluated per request, never cached
kit_session_check $sid    ;# -> 1/0; fills the global array KIT_SESSION:
                          ;#    valid 0/1, level admin|user|guest|unknown, user, via header|sid, lang de|en|auto|""
kit_level_ok $needed      ;# -> 1/0 for any|user|admin
kit_csrf_ok               ;# -> "" when the request may write, else method|header|origin|fetch-site
```

## Where the session comes from

| | CCU3 (eQ-3 firmware) and OpenCCU | openccu-lite |
| --- | --- | --- |
| The page gets it | the WebUI opens `<Config-Url>?sid=@xxxxxxxxxx@` in a new tab (the *Einstellungen* button and the Zusatzsoftware page) | the gate lets the request in by its cookie; lighttpd adds `X-Occulite-Session` to every request under `/addons/` and strips a client's copy; `?sid=@...@` is a legacy alias |
| A CGI checks it | `load tclrega.so; rega_script "Write(system.GetSessionVarStr('<10 chars>'));"`: the second list element is non-empty for a live session | `GET http://127.0.0.1/api/auth/v1/state` with `Authorization: Bearer <header>` must answer `"authenticated":true` and the same `"sid"`; API tokens have no `sid` and are refused (the Go checker admits one of the addon's scope, see *API tokens*) |
| The level | the answer `uid;upl;username;firstname;lastname;`: UPL `8` admin, `2` user, `1` guest | `/state`'s `level` (`administer`, `configure`, `operate`, `read`), else `role` |
| The language | `/etc/config/userprofiles/<user>.lang`: `0` auto, `1` de, `2` en | the shell's `?lang=` and its cookie (the frame's business) |

Rules the check follows:

- Only the ten characters of `@xxxxxxxxxx@` (letters and digits) ever go into the ReGa script.
- The header is read **on openccu-lite only** (a `LITE=` line in `/VERSION`, or
  `/usr/bin/occulited`): a CCU passes a client's header through untouched.
- Only the two id shapes the system hands out (26 characters of base32, or ten letters and
  digits) are sent to `/state`; a token (`olt_...`) or anything else is refused without asking.
- A header the system confirms decides alone: a `?sid=` cannot lift a session to another
  account's level. When the header is missing or not confirmed, `?sid=` is asked at `/state`
  (older images put the session itself there), then through the `tclrega.so` shim, which knows
  the user name only: level `unknown`.

## API tokens (Go, openccu-lite)

openccu-lite's gate also lets a **program** in: an API token with the scope `addon:<id>` of the
addon behind the path (or Full access, `*`) sent as `Authorization: Bearer olt_...`. The gate
hands the token itself on in `X-Occulite-Session` and says so in `X-Occulite-Auth: token` (a
session has `session`); a client's copies of both are removed first.

The Go checker (`addonkit.Session`, `Checker`) admits such a request when the addon sets
`SessionOptions.AddonID`:

- only the token shape (`olt_` and 32 hex) goes to `/state`, as Bearer like a session id, and
  the answer must say `"authenticated":true`, carry **no** `sid`, and hold `addon:<AddonID>` or
  `*` in `scopes`; an unknown or expired token (401), another addon's token, or one with API
  scopes only is refused with `401 session-invalid`, as is a credential whose shape contradicts
  `X-Occulite-Auth`;
- the level: `*` is `admin`, `addon:<id>` is `user` (the addon's pages and the user-level
  writes, as an `operate`/`configure` session; never `admin` routes). Other scopes add nothing;
- `FromRequest` answers `User: "token:<name>"`, `Token: "<name>"`, the `Scopes`, and no `ID`
  (the secret is not handed on); the answer is cached for the TTL like a session's;
- without `AddonID` no token is admitted (the behaviour before), and on a CCU the header is
  never trusted at all. The Tcl check (`kit_session_check`) still refuses tokens.

CSRF is unchanged for tokens: a writing request carries `X-CCU-Addon-Request: 1` whatever the
credential, which any program can send. An addon that serves a read-only API to tools that
cannot set a header (a Grafana datasource's `POST /search`) mounts those routes outside `CSRF`.

## Levels

| Kit level | CCU3 / OpenCCU (UPL) | openccu-lite (`level`, else `role`) |
| --- | --- | --- |
| `admin` | `8` | `administer` (`role: admin`) |
| `user` | `2` | `configure`, `operate` (`role: user`) |
| `guest` | `1` | `read` |
| `unknown` | the answer does not parse | a session accepted through the shim |

Every request belongs to a class: `read` (status, the tabs' GETs), `service` (start/stop/...),
`log`, `write` (the tabs' POSTs). kit.json's `permissions` says which level each class needs:

| Value | Admits |
| --- | --- |
| `any` | every valid session, whatever its level |
| `user` | `user`, `admin`, and `unknown` (reading keeps working where the level cannot be confirmed) |
| `admin` | `admin` only (`unknown` is refused: writing never opens up on an unconfirmed level) |

**The default is `any` for every class**: any valid WebUI session may do everything, as the
existing addons allow today. Levels are an **opt-in per addon**:

```json
"permissions": "levels"
```

is the table `read: user`, `service`, `log`, `write: admin`; or set the classes one by one, e.g.
`"permissions": {"write": "admin"}`. On a CCU the Zusatzsoftware page itself needs an admin, but
the `hm_addons.cfg` button is shown to every user, which is where `user` matters.

## CSRF

A writing request (`service`, `write`, and the forced update check) must

1. be a `POST` (`405 method` otherwise; OPTIONS gets a 405 without any `Access-Control-*`
   header, so a cross-origin preflight never succeeds);
2. carry **`X-CCU-Addon-Request: 1`**: an HTML form on another site cannot set a header, and a
   cross-origin `fetch` with one needs the preflight that step 1 refuses;
3. when it has `Origin`, name this host (`Origin`'s host and port equal `Host`; the scheme is not
   compared, the CCU serves the same page on 80 and 443); `Origin: null` is refused;
4. when it has `Sec-Fetch-Site`, say `same-origin` or `none`;
5. carry a session of the class's level.

There is no stored token: Tcl 8.2 has no random source to mint one without state, and the custom
header plus the Origin/Fetch-Metadata check is the standard defence for an API called only by
its own scripts. On a CCU the `sid` is a second secret a foreign site does not know; on
openccu-lite the gate adds its own Fetch-Metadata check in front. A refusal answers `403 csrf`
and logs `logger -t <id>-kit "csrf refused: <reason> from <REMOTE_ADDR>"`.

## The sid in the browser (the frame's side)

- The frame reads `sid` from the address once, keeps it in `sessionStorage` (`kit:<id>:sid`) and
  removes it from the address bar with `history.replaceState`; `Referrer-Policy: no-referrer`
  keeps it out of `Referer`s. A reload reads it back.
- Every request sends it as `?sid=@...@`. On openccu-lite with `ui.session_header` there is no
  `sid`; the header does it.
- Any `401 session-invalid` shows an overlay: open the page again from the WebUI.
