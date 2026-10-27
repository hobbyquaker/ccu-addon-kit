# The frame's CGIs, kit.json and the vendor step

The classic part of the kit is Tcl 8.2 and POSIX sh only: no binary per architecture, nothing
shared between addons at runtime. `ccu-addon-kit vendor` copies it into each addon's tree at
build time.

## What lands in the installed addon

```
/usr/local/addons/<id>/
├── lib/kit/                 the kit's Tcl library (copied) + VERSION + MANIFEST
│   ├── addon.tcl            generated from kit.json
│   ├── cgi.tcl session.tcl form.tcl json.tcl release.tcl rc-helpers.sh
│   └── cgi/                 the frame CGIs' bodies
├── bin/kit-run              the timeout runner (POSIX sh)
├── bin/update_addon         the hm_addons.cfg helper (the handbook's)
├── versions                 VERSION_ADDON=... (the addon's build writes it)
├── etc/lighttpd.conf        the proxy rule, with proxied.port (generated)
└── www/                     = /addons/<www>/ (the id unless kit.json's www says otherwise)
    ├── settings.cgi         the Config-Url, generated
    ├── update_check.cgi     the Zusatzsoftware page's update check, generated
    ├── lighttpd-error-503.html   only with proxied.errorPage
    ├── kit/                 session.cgi service.cgi alive.cgi log.cgi version.cgi (generated stubs)
    ├── frame/               the built frame (index.html, assets/, addon.json), copied by
    │                        vendor --frame; without one a placeholder page
    └── api/                 the addon's own tab CGIs
```

Every generated CGI is a stub with the addon's directory baked in, so no CGI derives paths from
its own location (the www symlink makes `[info script]` walks fragile):

```tcl
#!/bin/tclsh
set KIT_HOME /usr/local/addons/kitdemo
if {[info exists env(KIT_HOME)] && [info exists env(KIT_TEST)]} { set KIT_HOME $env(KIT_HOME) }
source $KIT_HOME/lib/kit/cgi/service.tcl
```

`KIT_TEST` is never set by lighttpd: a request header always arrives with the `HTTP_` prefix.

## kit.json

One file in the addon repository; [`schema/kit.schema.json`](../schema/kit.schema.json)
describes it for editors, `vendor` checks it.

```json
{
  "$schema": "../node_modules/ccu-addon-kit/schema/kit.schema.json",
  "schema": 1,
  "id": "battery",
  "name": {"de": "Batterien", "en": "Batteries"},
  "icon": "icon.svg",
  "service": {"commands": ["start", "stop", "restart", "reload"], "pollSeconds": 10},
  "logs": [
    {"id": "main", "label": {"de": "Dienst", "en": "Service"}, "file": "/var/log/battery.log", "rotated": "/var/log/battery.log.0"},
    {"id": "syslog", "label": {"de": "Systemprotokoll", "en": "System log"}, "syslogTag": "battery"}
  ],
  "update": {"github": "owner/ccu-addon-battery"},
  "tabs": ["overview", "settings", "kit:service", "kit:log", "kit:about"],
  "proxied": {"path": "/addons/battery/app/", "errorPage": true}
}
```

| Key | Required | Meaning |
| --- | --- | --- |
| `schema` | yes | `1` |
| `id` | yes | `[a-z0-9][a-z0-9-]{0,31}`; the rc.d name, the addon's directory, `/addons/<id>/` |
| `www` | no | the directory of the frame and its CGIs, `/addons/<www>/` (the `Config-Url` is `/addons/<www>/settings.cgi`); default the id. Another name when the addon's own application is proxied at `/addons/<id>/` (RedMatic: `red` and `redmatic`) |
| `name` | yes | `{de, en}` |
| `icon` | no | a path inside `www/` |
| `addonDir`, `rc`, `versionFile` | no | default `/usr/local/addons/<id>`, `/usr/local/etc/config/rc.d/<id>`, `<addonDir>/versions` |
| `service.commands` | no | default all four; what `service.cgi` accepts and the frame offers |
| `service.pollSeconds` | no | 2–300, default 10 |
| `logs` | no | the Log tab's sources: `file` (+ `rotated`) or `syslogTag`; default one syslog source tagged `<id>` |
| `update.github` | no | `owner/repo` for the release check; without it only the version is shown |
| `permissions` | no | who may do what, see [session.md](session.md#levels); default: any valid session may do everything |
| `tabs` | yes | the order of the tabs; `kit:service`, `kit:log`, `kit:about` are the frame's; an addon tab id must be registered in the built frame |
| `proxied` | no | an addon with its own HTTP server: `path` (below `/addons/<id>/`, ending with `/`, not hiding the frame's `/addons/<www>/`, `kit/` or `frame/`), `port` (127.0.0.1; `vendor` then writes `etc/lighttpd.conf`), `websocket` (default true), `errorPage` (installs the 503 page) |
| `app` | no | `{path, label: {de, en}}`: the addon's own application, which the frame links with a button and the session handed over ([tab-contract.md](tab-contract.md#linking-the-addons-application)) |
| `links` | no | `[{label: {de, en}, url}]` for the About tab |

The **versions file** holds sh-sourceable `KEY=value` lines (optionally `export` and quoted).
`VERSION_ADDON` is required; every other `*_VERSION` or `VERSION_*` key is a component
(`NODE_VERSION` → `node`).

`vendor` generates `lib/kit/addon.tcl` (Tcl 8.2 has no JSON parser; every non-ASCII character is
written as `\uXXXX` because a CCU3 sources files as iso8859-1) and `www/frame/addon.json` (public:
no paths, no secrets).

### An application at `/addons/<id>/`, the frame elsewhere

```json
{"id": "red", "www": "redmatic",
 "proxied": {"path": "/addons/red/", "port": 1880, "errorPage": true},
 "app": {"path": "/addons/red/", "label": {"de": "Node-RED öffnen", "en": "Open Node-RED"}}}
```

`vendor` writes `etc/lighttpd.conf` from `proxied`:

```
$HTTP["url"] == "/addons/red" {
    url.redirect = ("" => "/addons/red/")
}
$HTTP["url"] =~ "^/addons/red/" {
    proxy.server = ("/addons/red/" => (("host" => "127.0.0.1", "port" => 1880)))
    proxy.header = ("upgrade" => "enable")
    server.errorfile-prefix = "/usr/local/addons/red/www/lighttpd-error-"
}
```

(only what openccu-lite's fragment check allows). The reference `update_script` links it as
`/usr/local/etc/config/lighttpd/<id>.conf` and restarts lighttpd on a running OpenCCU when the
link is new (the CCU3 firmware installs during the shutdown, so its boot reads it); the reference
rc.d `uninstall` removes it. The addon's tree is linked as `/usr/local/etc/config/addons/www/<www>`
(`WWW=` in both reference scripts), and `<id>.cfg`'s `CONFIG_URL` is `/addons/<www>/settings.cgi`:
`vendor` refuses a tree whose `<id>.cfg` or `WWW=` disagrees with kit.json.

## Common rules

- Answers are JSON (`Content-Type: application/json; charset=utf-8`, `Cache-Control: no-store`)
  unless a table says text. Success: `{"ok": true, ...}`. Failure: an HTTP status and
  `{"ok": false, "error": {"code": "...", "message": "..."}}`; `message` is English, for logs.

  | `code` | HTTP | When |
  | --- | --- | --- |
  | `session-invalid` | 401 | no session, an expired one, one the platform does not confirm |
  | `forbidden` | 403 | a valid session below the level the request class needs |
  | `csrf` | 403 | a write without the request header, or from a foreign origin |
  | `method` | 405 | a write as GET; any method but GET and POST (`Allow: GET, POST`) |
  | `bad-request` | 400 | an unknown command, a bad parameter |
  | `not-found` | 404 | an unknown log source |
  | `busy` | 409 | another service action of this addon is running |
  | `timeout` | 504 | the rc.d command ran out of its time |
  | `failed` | 500 | the command exited non-zero (its `output` is in the answer) |
  | `internal` | 500 | a Tcl error inside the kit (the first line of `errorInfo`) |

- A Tcl error never reaches lighttpd as a bare 500: `kit_main` catches it and answers `internal`.
- Parameters never become globals and are never `subst`ed.
- External commands run through `bin/kit-run <seconds> <cmd> <args...>`: stdout and stderr
  merged (`-q`: stderr dropped), TERM at the limit to the command's whole process group, KILL two
  seconds later, exit 124 on a timeout.

## kit/session.cgi

`GET kit/session.cgi[?sid=@xxxxxxxxxx@]`, no level needed.

```json
{"ok": true,
 "session": {"valid": true, "level": "admin", "user": "Admin", "via": "sid", "lang": "de"},
 "platform": {"kind": "openccu", "firmware": "3.89.8.20260719", "arch": "x86_64", "tcl": "8.6.13"},
 "kit": {"version": "0.1.0", "requestHeader": "X-CCU-Addon-Request"},
 "permissions": {"read": true, "service": true, "log": true, "write": true}}
```

An invalid session is a `200` with `"session": {"valid": false}`. `level` is `admin`, `user`,
`guest` or `unknown`; `lang` is the WebUI user's language (`de`, `en`, `auto`) where the platform
has one; `platform.kind` is `ccu3`, `openccu` or `openccu-lite`; `permissions` is kit.json's
table evaluated for this session.

## kit/service.cgi

| Request | Class | Answer |
| --- | --- | --- |
| `GET ?cmd=status` (the default) | read | the rc.d script's status line **verbatim** when it is one line starting with `{`, else `{"schema":0,"running":<exit==0>,"raw":"<output>"}`; headers `X-Kit-Exit`, `X-Kit-Mem-Total-Kb`, `X-Kit-Mem-Available-Kb`; 5 s limit, then `504 timeout` |
| `POST cmd=start\|stop\|restart\|reload` | service + CSRF | `{"ok":true,"cmd":"restart","exit":0,"output":"..."}`; non-zero exit: `500 failed` with the same fields; limits 90/90/180/20 s |
| `GET ?cmd=start` (any action as GET) | — | `405 method` |

The verbatim body keeps JSON parsing out of Tcl; the frame validates the line. Actions hold a
lock (`/tmp/kit-<id>-service.lock`, a directory; older than 5 minutes counts as stale) and answer
`409 busy` while another runs. On openccu-lite the rc.d entry is the system's wrapper, so the
actions become the addon's service unit's and nothing kit-specific is needed.

## kit/alive.cgi

`GET kit/alive.cgi` → `{"running": true}`, **no session**: it runs `status` and reports whether it
exited 0, nothing else. The 503 page (`lighttpd-error-503.html`, with `proxied.errorPage`) polls it
every 3 s and reloads once the service runs; point the addon's lighttpd fragment's
`server.errorfile-prefix` at it.

## kit/log.cgi

| Request | Class | Answer |
| --- | --- | --- |
| `GET` | log | `{"ok":true,"sources":[{"id","label","kind","available","size","mtime"}...]}` |
| `GET ?source=<id>&lines=200` | log | `{"ok":true,"source","lines":[...],"truncated","size","mtime"}`: the newest lines (default 200, max 2000) of the last 256 KiB, the first partial line dropped, the rotated file read when the current one has fewer; a `syslog` source filters `/var/log/messages` (and `.0`) by `<tag>[` or `<tag>:` |
| `GET ?source=<id>&download=1` | log | `text/plain` attachment `<id>-<source>-YYYYMMDD-HHMM.log`, the last 4 MiB |
| `GET ?bundle=1` | log | a text debug bundle: versions, `/VERSION`, `uname -a`, `status`, `df -h`, `free`, the tail of every source; an addon adds sections with `lib/kit-debug.tcl` defining `kit_debug_sections` (a list of title / command-list pairs) |

On openccu-lite there is no `/var/log/messages`: a `syslog` source answers
`"available": false, "link": "/system/log?unit=addon-<id>"`.

## kit/version.cgi and update_check.cgi

| Request | Class | Answer |
| --- | --- | --- |
| `GET kit/version.cgi` | read | `{"ok":true,"installed","components","kit","platform"}` |
| `GET kit/version.cgi?cmd=check` | read | `{"ok":true,"installed","latest","newer","url","checkedAt","source":"network"\|"cache"}`; without a network `"latest":null,"error":"offline"`; also `rate-limited`, `no-release`, `http-<status>`, `not-configured` |
| `POST kit/version.cgi` `cmd=check&force=1` | read + CSRF | the same, ignoring a cache older than 60 s |
| `GET update_check.cgi?cmd=check_version` | none | plain text: the newest version, or `n/a` (the handbook's chapter 04 contract) |
| `GET update_check.cgi?cmd=download` | none | a redirect to the release page |

The check asks `https://api.github.com/repos/<owner>/<repo>/releases/latest` (no drafts, no
prereleases) with `curl` through `kit-run`. The cache `/tmp/kit-<id>-release.json` (tmpfs) keeps
a success for 6 hours and an error for 30 minutes, which keeps a CCU far below GitHub's 60
unauthenticated requests per hour. Versions compare semver-aware (`kit_version_compare`: numeric
parts, `+n` as a build number, a prerelease older than its release). Self-update is not part of
the kit.

## settings.cgi

The `Config-Url`. It answers `www/frame/index.html` with `Cache-Control: no-store`,
`Referrer-Policy: no-referrer` and no `X-Frame-Options` (openccu-lite frames it), **without**
checking the session: the page is static and holds no data, its first request
(`kit/session.cgi`) decides, and every data request is checked.

OpenCCU's lighttpd sets its own `Cache-Control: private, no-cache, ...` and
`X-Frame-Options: SAMEORIGIN` on every answer, replacing the CGI's (seen on OpenCCU 3.89.8). Both
are harmless here: the page is not cached either way, and the only frame it lives in,
openccu-lite's shell, is same-origin.

## A tab's own CGIs

Any name under `www/api/`, a stub like the kit's, on top of `lib/kit/cgi.tcl`:

```tcl
#!/bin/tclsh
set KIT_HOME /usr/local/addons/kitdemo
if {[info exists env(KIT_HOME)] && [info exists env(KIT_TEST)]} { set KIT_HOME $env(KIT_HOME) }
source $KIT_HOME/lib/kit/cgi.tcl

kit_main {
    if {[kit_request_is_post]} {
        kit_require write
        kit_form f
        if {![info exists f(greeting)] || [string length $f(greeting)] > 80} {
            kit_fail 400 bad-request "greeting missing or too long"
        }
        set fd [open $KIT_HOME/etc/greeting.txt w]
        puts -nonewline $fd $f(greeting)
        close $fd
        kit_ok [list saved b 1]
    }
    kit_require read
    kit_ok [list greeting s [kit_read_file $KIT_HOME/etc/greeting.txt 1024]]
}
```

| Procedure | Does |
| --- | --- |
| `kit_main {body}` | refuses methods but GET/POST, runs the body, answers `internal` on a Tcl error |
| `kit_query arr` / `kit_form arr` | decoded query / POST body (`x-www-form-urlencoded`, ≤ 1 MiB) into an array |
| `kit_request_is_post` | 1 for a POST |
| `kit_require read\|write\|service\|log` | method, session (`sid` from the query, or openccu-lite's header), CSRF for `write`/`service`, level; answers the error and exits on failure |
| `kit_ok {name type value ...}` | `{"ok":true,...}` and exit; types `s` string, `n` number, `b` bool, `j` raw JSON, `z` string or null when empty |
| `kit_fail status code message ?triples?` | the error answer and exit |
| `kit_json_string`, `kit_json_obj`, `kit_json_list`, `kit_json_strings` | JSON building |
| `kit_run seconds cmd args` / `kit_run_stdout ...` | `{exitcode output}` through `kit-run`, stdin `/dev/null` |
| `kit_read_file path ?maxBytes?` | `""` when missing; the last `maxBytes` when longer |
| `kit_versions` | the versions file as a name/value list |
| `kit_log level text` | `logger -t <id>-kit` |
| `kit_lock name` / `kit_unlock dir` | an atomic lock directory below `/tmp` |
| `kit_version_compare a b` | -1, 0, 1 |

## The vendor step

1. **Pin** the kit in the addon's `package.json` as a devDependency with an exact tag
   (`"ccu-addon-kit": "github:hobbyquaker/ccu-addon-kit#v0.1.0"`).
2. **Build** the settings page with Vite and `kitVite()` into `web/dist/`
   ([tab-contract.md](tab-contract.md#building-a-page)).
3. **Vendor:** `npx ccu-addon-kit vendor --config kit.json --out addon_files/<id> --frame web/dist`
   copies the library and `kit-run`, the built frame into `www/frame/` (removing an earlier
   build's leftovers), generates `addon.tcl`, `addon.json`, the CGI stubs, `etc/lighttpd.conf` and
   the 503 page, writes `lib/kit/VERSION` (`<version> <commit>`) and `lib/kit/MANIFEST` (the hashes
   it wrote). It checks kit.json, the tab ids against the frame's `frame-manifest.json` and the
   `www` directory against `<id>.cfg` and the scripts' `WWW=`, and refuses to overwrite a library
   file somebody edited (its hash differs from `MANIFEST`) unless `--force`.
4. **Package** as before. The vendored files are build output, not committed (see
   [`example/kitdemo/.gitignore`](../example/kitdemo/.gitignore)), so a kit update is one line.
5. **Drift check** in the addon's CI: `npx ccu-addon-kit vendor --check --frame web/dist` fails
   when the tree differs from what the pinned kit produces.

`ccu-addon-kit new <id> --name "<name>"` scaffolds an addon from [`example/kitdemo`](../example/kitdemo).
