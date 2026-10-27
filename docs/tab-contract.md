# The frame and the tab contract

The settings frame is a Svelte 5 app on [occulite-ui](https://github.com/hobbyquaker/occulite-ui).
It is the page behind the addon's `Config-Url` (`/addons/<www>/settings.cgi`): a header with the
addon's name, version and service state, the frame's own tabs **Service**, **Log** and **About**,
and the addon's tabs. It runs in a tab of its own on a CCU (the WebUI opens it with
`window.open('…/settings.cgi?sid=@…@')`) and inside openccu-lite's shell (a same-origin frame).

An addon with an application of its own (Node-RED, a dashboard) serves that application from its
own server; the frame holds service, log, version and the addon's settings, and links the
application with a button (kit.json `app`, see [Linking the addon's application](#linking-the-addons-application)).

## Building a page

The addon has a small Vite project, `web/`, as [`example/kitdemo/web`](../example/kitdemo/web):

```ts
// web/src/main.ts
import 'occulite-ui/app.css';
import {defineTabs, mountFrame} from 'ccu-addon-kit/frame';

import strings from './i18n';                  // {de: {...}, en: {...}}
import Overview from './tabs/Overview.svelte';

const tabs = defineTabs([
    {id: 'overview', label: {de: 'Übersicht', en: 'Overview'}, component: Overview},
    {id: 'settings', label: {de: 'Einstellungen', en: 'Settings'},
        component: () => import('./tabs/Settings.svelte'), level: 'admin'},
]);

void mountFrame(document.getElementById('app')!, {tabs, strings});
```

```js
// web/vite.config.mjs
import {defineConfig} from 'vite';
import {kitVite} from 'ccu-addon-kit/vite';

export default defineConfig({plugins: [kitVite()]});
```

`web/index.html` is a plain page with `<div id="app"></div>` and the module script.

Build and vendor:

```sh
npx vite build --config web/vite.config.mjs        # -> web/dist/ (index.html, assets/, frame-manifest.json)
npx ccu-addon-kit vendor --config kit.json --out addon_files/<id> --frame web/dist
```

`kitVite()` adds the Svelte plugin (pass `svelte: false` when the config adds its own), keeps one
Svelte runtime, resolves `ccu-addon-kit/frame`, builds relative to `www/frame/` (the page itself is
served one directory up, as `settings.cgi`) and writes **`frame-manifest.json`** with the tab ids
it found in `defineTabs([...])` (the `id` string literals inside the call). `vendor --frame`
copies the build into `www/frame/`, removes what an earlier build left there, records every file
in `lib/kit/MANIFEST` (so `vendor --check` covers the frame too) and **refuses a kit.json that
lists a tab the frame does not register**. Without a built frame `vendor` installs a placeholder
page with the same CGIs.

Dependencies of the addon: `ccu-addon-kit`, `occulite-ui` (^0.1), `svelte` (^5.40), `vite` (^7),
`@sveltejs/vite-plugin-svelte` (^6) - `ccu-addon-kit new <id>` writes a `package.json` with them.

## `KitTab`

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | `[a-z0-9][a-z0-9-]*`, not starting with `kit` | the route segment (`#/<id>[/<path>]`); must be listed in kit.json's `tabs` |
| `label` | `{de, en}` | the tab title |
| `icon` | `IconData` (optional) | icon data from `occulite-ui/icons` or the addon's own |
| `component` | a Svelte component, or `() => import('./Tab.svelte')` | a lazy tab is loaded when it is first opened, with the frame's loading state |
| `level` | `'user' \| 'admin'` (optional) | hidden below that level (the CGIs still check); `unknown` counts as `user` |
| `badge` | `() => number \| string \| null` (optional) | a reactive badge after the label |

kit.json's `tabs` orders everything; the frame's own tabs are `kit:service`, `kit:log`,
`kit:about` (routes `#/kit-service`, `#/kit-log`, `#/kit-about`). Inside openccu-lite's shell the
frame shows kit.json's `liteTabs` instead when the addon gives that list (a subset of `tabs`: there
the shell has its own service and log pages). An unknown route opens the first
tab. A tab is mounted when it is first opened and **stays mounted** while the user switches, so
its unsaved input survives a switch and the session overlay.

## What a tab gets

Props: `active: boolean` (true while shown: start and stop polling with it) and `path: string` (the
sub-route after the tab id, `#/devices/JEQ0123456` → `JEQ0123456`).

Everything else through `const kit = getKit()` (during the component's setup); every field is
reactive:

| Member | Meaning |
| --- | --- |
| `kit.addon` | `{id, name, version, kitVersion}`; `name` in the page's language, `version` from the status line |
| `kit.config` | kit.json's frame view (`www/frame/addon.json`) |
| `kit.platform` | `{kind: 'ccu3' \| 'openccu' \| 'openccu-lite', firmware, arch, tcl}` |
| `kit.session` | `valid`, `expired`, `level` (`admin`, `user`, `guest`, `unknown`), `user`, `can('read' \| 'write' \| 'service' \| 'log')` (kit.json's `permissions` for this session) |
| `kit.service` | `status` (the status line, plus `exit`, `memTotalKb`, `memAvailableKb` from the headers), `view` (`running`, `stopped`, `failed`, `busy`, `unknown`), `pending`, `last`, `commands`, `refresh()`, `run(cmd)` (asks first for stop and restart) |
| `kit.api(cgi, opts)` | a request below `/addons/<www>/` (or an absolute path): adds `sid=@…@` on a CCU, the header `X-CCU-Addon-Request: 1` and a form body (`x-www-form-urlencoded`) on a POST, a timeout (30 s); resolves the JSON, or `{status, headers, json, text}` with `as: 'raw'`, or text with `as: 'text'` |
| `kit.base` | `/addons/<www>/` |
| `kit.t(key, params)` | the addon's strings first, then the frame's (`kit.*`); a missing key shows the key |
| `kit.text({de, en})` | a two-language text in the page's language |
| `kit.lang`, `kit.theme`, `kit.embedded` | `de`/`en`, `light`/`dark`, inside openccu-lite's shell |
| `kit.notify(kind, text, {timeoutMs})` | a toast (`info`, `success`, `warning`, `error`); errors stay until closed |
| `kit.ask({title, message, confirm, cancel, danger})` | a modal question → `true`/`false`; `kit.askText({title, message, value, confirm})` → the text or `null`. Never `window.confirm`, `prompt` or `alert` |
| `kit.route` | `tab`, `path`, `go(tab, path)` |
| `kit.setDirty(dirty)` | the tab has unsaved input: the frame asks before the tab or the page is left |
| `kit.app` | `{url, label}` of the addon's application (kit.json `app`) or `null` |

`api()` rejects with a **`KitRequestError`** (`code`, `status`, `message`, `body`; `isKitError(e)`
tells): the CGIs' error envelope (`session-invalid`, `forbidden`, `csrf`, `method`, `bad-request`,
`not-found`, `busy`, `timeout`, `failed`, `internal`), `http-<status>` for a non-JSON error,
`network`, `timeout`. `session-invalid` also lays the session overlay over the page; `csrf` and
`internal` are logged to the console.

Components for tabs: `<KitLoading />` (a skeleton of fixed height), `<KitError error={e}
retry={load} />` (the code translated, Retry, the details in a disclosure), `<KitEmpty text=… />`.
Every tab sits in an error boundary: a tab that throws shows "This section could not be loaded"
with Retry, and the rest of the page keeps working. The CSS classes `kit-section`, `kit-dl`,
`kit-notice`, `kit-muted`, `kit-primary`, `kit-danger` and occulite-ui's `hmm-button`,
`hmm-input`, `hmm-select` are there for tabs to use.

A tab's own CGIs are described in [cgi-api.md](cgi-api.md#a-tabs-own-cgis); the example is
kitdemo's [`Hello.svelte`](../example/kitdemo/web/src/tabs/Hello.svelte) with
[`hello.cgi`](../example/kitdemo/addon_files/kitdemo/www/api/hello.cgi).

`kit.ask()` is the frame's own dialog on occulite-ui's `Dialog` for now; it moves onto
occulite-ui's question dialog once that exists, without a change for tabs.

## Session, theme, language

- **The sid** (CCU): read once from `?sid=`, kept in `sessionStorage` (`kit:<id>:sid`, per browser
  tab), removed from the address with `history.replaceState`; a reload reads it back. Every request
  carries it as `sid=@…@`. On openccu-lite there is none: the system's gate and session header do
  it.
- **Keep-alive** (CCU only): while the page is visible, `Session.renew` on `/api/homematic.cgi`
  every 4 minutes. Never on openccu-lite.
- **Expiry:** any `session-invalid` answer stops the polling and lays an overlay over every tab:
  *Retry* asks `session.cgi` again; *To the WebUI* (CCU) goes to `/`, *Sign in again*
  (openccu-lite) reloads the page, which the gate sends to the login. The tabs stay mounted
  underneath.
- **Theme:** inside openccu-lite's shell the shell's (`?theme=` on the frame URL, then its
  message); anywhere else the browser's `prefers-color-scheme`, with a header toggle (system →
  light → dark) kept in `localStorage` (`kit:theme`).
- **Language:** inside the shell the shell's; on a CCU the header's choice (`kit:lang`), else the
  WebUI user's language from `session.cgi`, else the browser's (`de*` → German, else English).
- **Strings:** the frame's are `kit.*` (German and English, in `frame/src/messages.ts`); an addon
  passes its own to `mountFrame` as `{de: {...}, en: {...}}` (or a catalogue `{key: {de, en}}`) and
  never uses the `kit.` prefix.
- **Inside openccu-lite's shell** the header is one compact line without the icon and without the
  theme and language toggles; the Log tab links the system's log (`/system/log?unit=addon-<id>`)
  for a `syslog` source.

## The frame's own tabs

| Tab | Content |
| --- | --- |
| `kit:service` | the state (a pill of fixed width in every state), pid(s), memory and its share of the system's, uptime, version, the last error with a link to the log, the buttons of kit.json's `service.commands` (enabled by state and level; Stop and Restart ask first), the output of the last action. Polls every `pollSeconds` while the page is visible, every 2 s for 30 s after an action |
| `kit:log` | a source selector, the newest 200 / 500 / 2000 lines, a filter in the browser, auto-refresh every 5 s (off by default), *Download*, *Debug bundle* |
| `kit:about` | installed version, components, the kit's version, the platform, the user; the release check (a CCU checks once when the tab opens, from the cache when fresh; openccu-lite only on *Check for updates*), kit.json's `links` |

## Linking the addon's application

An addon whose real interface is an application of its own - served by its own server, e.g.
Node-RED behind a lighttpd proxy rule - keeps that application at `/addons/<id>/` and gives the
frame its own directory (RedMatic's layout: the application at `/addons/red/`, the settings at
`/addons/redmatic/settings.cgi`):

```json
{
  "id": "red",
  "www": "redmatic",
  "proxied": {"path": "/addons/red/", "port": 1880, "errorPage": true},
  "app": {"path": "/addons/red/", "label": {"de": "Node-RED öffnen", "en": "Open Node-RED"}}
}
```

The frame shows `app` as a button in the header and on the Service tab; it opens the application
in a new browser tab with the session handed over the way the WebUI hands it to the settings
page (`?sid=@…@` on a CCU; nothing on openccu-lite, where the gate does it). The application's
server checks that sid itself. `www`, `proxied` and the generated lighttpd rule are in
[cgi-api.md](cgi-api.md#kitjson).

## Size

The settings page of kitdemo - the frame, one tab, the Svelte runtime and occulite-ui - is about
43 KB gzip (JS and CSS); CI fails when it grows more than 10 % over
[`test/frame/size-budget.json`](../test/frame/size-budget.json) (`node test/frame/size.mjs`).

## Tests

- `npx vitest run --config frame/vitest.config.ts`: `api()`, the status view, routes, the
  session and service stores, the keep-alive, i18n, the sid, the error boundary, lazy tabs, the
  manifest scanner.
- `npx playwright test --config test/frame/playwright.config.mjs`: kitdemo's built frame in
  Chromium against a stub server that runs the **real Tcl CGIs** (in the kit's Tcl 8.6 test
  image): the sid handling, the example tab and its CSRF header, service actions with the
  question and the pill's fixed size, a failed action, session expiry and Retry, the dirty-tab
  question, the log, the About tab, the app button, light and dark, German and English, the
  keep-alive, and the page inside a stand-in of openccu-lite's shell.
