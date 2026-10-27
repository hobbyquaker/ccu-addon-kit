# The frame and the tab contract (not built yet)

The settings frame is a Svelte 5 app built on the `occulite-ui` component library; it waits for
that library's first release. Until then `vendor` installs a placeholder page
([`classic/html/frame-placeholder.html`](../classic/html/frame-placeholder.html)) that uses the
same CGIs, the same `sid` handling and the same request header: service status and actions, the
log tail, the version and the release check.

What the frame will be:

- **The frame's own tabs** `kit:service`, `kit:log`, `kit:about`, the header with the status
  pill, the session overlay, theme (light/dark following the browser, a toggle) and language
  (the WebUI user's, a toggle), working in a CCU's new tab and inside openccu-lite's shell.
- **An addon's tabs** are Svelte components registered with `mountFrame(el, {tabs, strings})`;
  each gets `active` and `path` as props and a context (`getKit()`) with the session, the
  service store, `api(cgi, opts)` (adds the `sid` and, for a POST, the request header),
  `t()`, `notify`, `ask`/`askText` (never `window.confirm`), the route and `setDirty`.
- A Vite plugin `kitVite()` builds the frame and the addon's tabs into
  `addon_files/<id>/www/frame/` and writes `frame-manifest.json`, which `vendor` checks
  kit.json's `tabs` against.

This document becomes the contract once the frame exists.
