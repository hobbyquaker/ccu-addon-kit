# ccu-addon-kit

The shared parts of a Homematic CCU addon's settings page, for the classic firmwares (eQ-3's
CCU3 firmware and OpenCCU) and, later, the Go side for openccu-lite:

- **a service convention** for the addon's rc.d script: `status` prints one JSON line and its exit
  code says running or not; `start`, `stop`, `restart`, `reload` behave the same in every addon
  ([docs/service-convention.md](docs/service-convention.md)), with a reference script and a
  conformance test, `ccu-addon-kit check-rc`;
- **a Tcl CGI library** for the CCU's lighttpd: one session check for the WebUI's `sid` and
  openccu-lite's session header, optional user levels, CSRF protection for writing requests,
  JSON output, form parsing, a timeout runner ([docs/session.md](docs/session.md),
  [docs/cgi-api.md](docs/cgi-api.md)). Tcl 8.2 (the CCU3 firmware's) and POSIX sh only: no
  binary per architecture;
- **the frame's CGIs**: session, service status and actions, a log tail with download and debug
  bundle, version and release check (cached, offline-tolerant), the WebUI's `update_check.cgi`,
  the 503 page for an addon with its own server, the `settings.cgi` entry point;
- **a settings frame** in Svelte 5 on [occulite-ui](https://github.com/hobbyquaker/occulite-ui):
  the header with the service state, the Service, Log and About tabs, the addon's own tabs, a
  button to the addon's own application, light and dark, German and English, inside the CCU's
  WebUI and openccu-lite's shell ([docs/tab-contract.md](docs/tab-contract.md)); `kitVite()` for
  the addon's Vite build;
- **the vendor step**: the kit and the built frame are copied into each addon at build time from
  one `kit.json` (plus the lighttpd proxy rule of an addon with its own server); nothing is shared
  between addons at runtime, so every addon carries its own pinned copy;
- **`go/`**: the openccu-lite side (`addonkit`, `litesource`), started with the first addon that
  needs it.

Status: in development, no release yet.

## Using it in an addon

```sh
npx ccu-addon-kit new myaddon --name "My Addon"      # scaffold from example/kitdemo
cd myaddon
npm install
# write kit.json, the daemon, the rc.d variables and the tabs in web/, then:
npx vite build --config web/vite.config.mjs
npx ccu-addon-kit vendor --config kit.json --out addon_files/myaddon --frame web/dist
npx ccu-addon-kit vendor --config kit.json --out addon_files/myaddon --frame web/dist --check   # in CI
npx ccu-addon-kit check-rc --exec "ssh root@<test-ccu>" --rc /usr/local/etc/config/rc.d/myaddon --log /var/log/myaddon.log
```

[`example/kitdemo`](example/kitdemo) is a complete noarch addon: a small sh daemon, the reference
rc.d script, the handbook's `update_script`, the `hm_addons.cfg` entry, an openccu-lite manifest,
the settings page with one tab of its own (`web/`) and its CGI, and `build.sh` producing the
`.tar.gz`.

The CCU addon handbook, [ccu-addon-howto](https://github.com/homematic-community/ccu-addon-howto),
explains the platform underneath: packaging, the installers, the rc.d script, the WebUI.

## Repository

| Path | Holds |
| --- | --- |
| `classic/tcl/lib/kit/` | the Tcl library and the frame CGIs' bodies |
| `classic/cgi/stub.cgi.in` | the template of the generated CGI stubs |
| `classic/sh/` | `kit-run`, `rc.d-reference`, `rc-helpers.sh`, `update_script-reference`, `update_addon` |
| `classic/html/` | the 503 page and the placeholder frame page |
| `frame/` | the settings frame (Svelte 5, `ccu-addon-kit/frame`) and its unit tests |
| `vite/` | `kitVite()` (`ccu-addon-kit/vite`) |
| `cli/` | `vendor`, `check-rc`, `new` |
| `schema/` | `kit.schema.json`, `service-status.schema.json` |
| `example/kitdemo/` | the example addon |
| `test/` | the frame's Playwright tests against a stub server with the real CGIs (`test/frame`), the CGI simulator with a stub `tclrega.so` and a stub `/state` (`test/tcl`), the Tcl 8.2 lint, the shell checks, the CLI tests, the end-to-end container (`test/e2e`) |
| `go/` | the openccu-lite side (later) |

## Tests

```sh
npm ci                   # the frame's build and test dependencies
npm test                 # the CLI tests (node only)
npm run test:frame       # the frame's unit tests
npm run typecheck        # svelte-check
npm run lint:tcl         # the Tcl 8.2 lint
sh test/gate.sh          # everything CI runs, in docker: shellcheck and busybox, the Tcl tests
                         # under Tcl 8.6 and under Tcl 8.2.3 built from source, the end-to-end
                         # install of kitdemo with lighttpd and the conformance test, the
                         # frame's checks, its size budget and its Playwright tests
```

## Licence

MIT, so the kit can be copied into addons under any licence.
