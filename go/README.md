# go/ - the openccu-lite side

The Go module `github.com/hobbyquaker/ccu-addon-kit/go` for an addon written in Go. On openccu-lite
the system's own Addons page already does service control, logs and memory, so this side is the
plumbing for an addon's own server, not a settings frame. The first consumer is
[ccu-addon-battery](https://github.com/hobbyquaker/ccu-addon-battery).

```sh
go get github.com/hobbyquaker/ccu-addon-kit/go@main
```

## `addonkit`

| | |
| --- | --- |
| `Platform()` | the handbook's one rule, evaluated per call: `OpenccuLite` (a `LITE=` line in `/VERSION` or an executable `/usr/bin/occulited`), `CCU3` (`/etc/init.d/S00InstallAddon`), else `OpenCCU` |
| `Session(next, SessionOptions)`, `FromRequest(r)`, `Require(level, next)` | the session check: reads the gate's `X-Occulite-Session` (trusted on openccu-lite only), confirms it with `GET /api/auth/v1/state` as Bearer (the same `sid` must come back; API tokens have none and are refused), caches a confirmed id for 10 s (64 at most), maps `level` onto the kit's `admin`/`user`/`guest`/`unknown` as `docs/session.md` says. 401 `session-invalid`, 403 `forbidden` |
| `CSRF(next)` | the same four checks as the Tcl side, with the same header `X-CCU-Addon-Request`: POST only (OPTIONS is 405 without CORS headers), the header, an `Origin` naming this host, `Sec-Fetch-Site` same-origin or none. 403 `csrf` with the reason |
| `Token(id)`, `AddonToken(id)`, `LocalToken()`, `AuthTransport`, `Client(id)` | the addon's own token from `/run/occulite/addon-tokens/<id>.api` (the scopes of the manifest's `api_scopes`), re-read on a 401 (the system mints a new one at every start) |
| `AppFiles(dir)` | the addon's built app: a path without a file is `index.html` (the app's routes), dotfiles 404; `/assets/` immutable for a year, every other file `no-cache` with an ETag from its content and no `Last-Modified` - the packages are built reproducibly (one mtime for every file of every version), so a mtime validator would keep a browser on the old `index.html` after an update |
| `Prefs(r)` | theme and language from `?theme=&lang=` and the `ol-theme`/`ol-lang` cookies, for server-rendered bits |
| `Listen(port, socket)` | the loopback only, or a unix socket in the addon's tree (nothing but lighttpd faces the LAN) |
| `StatusJSON`, `ExitCode` | the service convention's status line for a daemon that answers `status` itself |
| `addonkit/manifest` | the `openccu-lite.json` subset a Go addon declares, with the validator the addon's tests run (the install-time rules: scopes never granted, root capabilities and groups refused for a confined addon, `data_dirs` guard rails) and `Undeclared()` |
| `addonkit/fragment` | renders `etc/lighttpd.conf`: the redirect of the bare path and the proxy of `/addons/<id>/` (or a sub-path) to the loopback port or socket, the 503 page - the same text as the CLI's `vendor` from kit.json's `proxied` (shared fixtures in `testdata/`); the frame and the CGIs live in their own `/addons/<www>/`, within the directives occulited accepts |

## `litesource`

The Go port of occulite-client's protocol handling, as far as a recording addon needs it:

| | |
| --- | --- |
| `New(base, token)` | a client of `http://127.0.0.1` with a `TokenSource` |
| `Probe(ctx)` | openccu-lite, a CCU (`ErrNotOpenccuLite`: a 404, an HTML page or JSON that is not the metadata API), a system that does not answer its API yet (`*NotReadyError`: a 502/503/504 or the `{"error":"starting"}` openccu-lite's lighttpd answers at boot until occulited is up, with its `Retry-After`) or unreachable (any other error) |
| `Await(ctx, lite)` | probes until the system answers: a `NotReadyError` or an unreachable system is retried after its `Retry-After`, else `ProbeRetry` (5 s); `ErrNotOpenccuLite` ends the wait unless `lite` says the platform rule already recognised openccu-lite (`addonkit.IsOpenccuLite()`) - a daemon's start loop, which must never take a boot-time 503 for a CCU |
| `Events(ctx, Filter, lastEventID)` | `GET /api/rpc/v1/events` as a channel of typed messages: reconnects with backoff, resumes with `Last-Event-ID`, takes 45 s of silence as a dead connection, and delivers one `resync` per reconnection whenever values may have been missed (the server's own, a hello of another boot, a reconnect without an id) - after which the consumer reads `State` again |
| `State(ctx, Filter)` | the state store, every page, with the `event_id` to resume from |
| `History`, `Paramsets`, `ParamsetsFor`, `ListDevices`, `GetParamset`, `Call` | the rest of lite-rpc over JSON-RPC (`Double` sends a `double`) |
| `Meta().Snapshot`, `Meta().Events(ctx, since)` | the metadata API; a gap in the revisions or a reconnect is a `resync` event |
| `Meta().PatchObject(ctx, iface, address, ObjectPatch)` | `PATCH /api/meta/v1/objects/{ref}`: the name, the enums, `meta` merged per namespace (a nil value removes one); answers the revision and whether anything changed - the store's 304 for a write that changes nothing is not an error |
| `Source` | the seam an addon records against (`Seed`, `Subscribe`, `Devices`, `Read`, `Descriptions`, `Names`, `NameEvents`); `*Client` implements it, `ccusource.Source` is the classic CCU's, over XML-RPC and ReGa |
| `fakelite` | a fake openccu-lite for tests: an `httptest` server with the state store, the stream (ids, hello, resume, resync, reboot), JSON-RPC, the paramset cache, the metadata snapshot, stream and object writes (`MetaWrites`), `/api/auth/v1/state`, and the boot-time 503 `starting` answers (`SetStarting(n)`) |

## `ccusource`

The classic CCU (CCU3, OpenCCU) behind the same `litesource.Source` seam: the interface processes
from `InterfacesList.xml` over XML-RPC with an `init` callback for the events, the names and rooms
from ReGa. `Call(ctx, iface, method, params...)` is the classic side of `litesource.Client.Call`:
the answer as JSON, a `litesource.Double` at any depth sent as a double, a fault as `*Fault`.

What a read costs on air is the caller's concern: HmIP `VALUES` come from hmipserver's cache, a
BidCos `VALUES` read of a value channel is a radio frame; channel 0 and `MASTER` are free on both.

## Tests

```sh
cd go && go vet ./... && go test -race ./...
```
