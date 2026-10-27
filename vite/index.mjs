// kitVite() (MIT): the Vite setup of an addon's settings page (docs/tab-contract.md).
//
//   // web/vite.config.mjs
//   import {defineConfig} from 'vite';
//   import {kitVite} from 'ccu-addon-kit/vite';
//   export default defineConfig({plugins: [kitVite()]});
//
// - adds the Svelte plugin (`svelte: false` when the config brings its own, or its options), one Svelte runtime (dedupe), and resolves
//   `ccu-addon-kit/frame` to the kit's source;
// - relative paths (`base: './'`); the page is served as `/addons/<id>/settings.cgi` and its files
//   lie in `/addons/<id>/frame/`, so index.html's own references get the `frame/` prefix;
// - builds into `dist/` next to the config (`outDir` to change), which `vendor --frame` copies
//   into the addon's `www/frame/`;
// - writes `frame-manifest.json` with the tab ids registered through `defineTabs([...])` (string
//   literals of `id:` inside the call), which `vendor` checks kit.json's `tabs` against.

import {readFileSync} from 'node:fs';
import {dirname, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const KIT_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const FRAME_ENTRY = resolve(KIT_ROOT, 'frame/src/index.ts');
const TAB_ID = /^[a-z0-9][a-z0-9-]*$/;

function kitVersion() {
    try {
        return JSON.parse(readFileSync(resolve(KIT_ROOT, 'package.json'), 'utf8')).version;
    } catch {
        return 'unknown';
    }
}

/** The `id: '...'` literals inside every `defineTabs(...)` call of a module's source. */
export function scanTabIds(code) {
    const ids = [];
    let at = 0;
    for (;;) {
        const start = code.indexOf('defineTabs(', at);
        if (start < 0) break;
        // the call's argument: up to the matching parenthesis, strings and comments skipped
        let depth = 0;
        let i = start + 'defineTabs'.length;
        let end = -1;
        for (; i < code.length; i++) {
            const c = code[i];
            if (c === '"' || c === "'" || c === '`') {
                const q = c;
                for (i++; i < code.length && code[i] !== q; i++) if (code[i] === '\\') i++;
                continue;
            }
            if (c === '/' && code[i + 1] === '/') { i = code.indexOf('\n', i); if (i < 0) break; continue; }
            if (c === '/' && code[i + 1] === '*') { i = code.indexOf('*/', i + 2) + 1; if (i <= 0) break; continue; }
            if (c === '(') depth++;
            if (c === ')' && --depth === 0) { end = i; break; }
        }
        if (end < 0) break;
        const arg = code.slice(start, end);
        for (const m of arg.matchAll(/(?:^|[{,\s])["']?id["']?\s*:\s*(["'])([^"'\\]*)\1/g)) {
            if (TAB_ID.test(m[2]) && !ids.includes(m[2])) ids.push(m[2]);
        }
        at = end;
    }
    return ids;
}

/**
 * @param {{outDir?: string, svelte?: boolean | object}} [options]
 * @returns {import('vite').PluginOption[]}
 */
export function kitVite(options = {}) {
    const tabs = new Set();
    const plugin = {
        name: 'ccu-addon-kit',
        enforce: 'pre',
        config(user) {
            return {
                base: './',
                resolve: {
                    alias: [{find: /^ccu-addon-kit\/frame$/, replacement: FRAME_ENTRY}],
                    dedupe: ['svelte', 'occulite-ui'],
                },
                build: {
                    outDir: user.build?.outDir ?? options.outDir ?? 'dist',
                    emptyOutDir: true,
                    target: 'es2020',
                    assetsDir: 'assets',
                    // one script and one stylesheet: a CCU serves files slowly
                    cssCodeSplit: false,
                    modulePreload: {polyfill: false},
                },
            };
        },
        // the page is served as settings.cgi one directory up: its own references get the frame/
        // prefix; everything else (chunks, CSS urls) is relative to the files themselves
        transformIndexHtml: {
            order: 'post',
            handler: (html) => html.replace(/(\s(?:src|href)=")\.\/(?!frame\/)/g, '$1./frame/'),
        },
        transform(code, id) {
            if (id.includes('/node_modules/') || !/\.(m?[jt]s|svelte)(\?|$)/.test(id)) return null;
            if (code.includes('defineTabs(')) for (const t of scanTabIds(code)) tabs.add(t);
            return null;
        },
        generateBundle() {
            this.emitFile({
                type: 'asset',
                fileName: 'frame-manifest.json',
                source: `${JSON.stringify({kit: kitVersion(), tabs: [...tabs].sort()}, null, 2)}\n`,
            });
        },
    };
    const plugins = [plugin];
    if (options.svelte !== false) {
        // the addon's own @sveltejs/vite-plugin-svelte; `svelte: false` when the config adds it itself
        const svelteOptions = typeof options.svelte === 'object' ? options.svelte : {};
        plugins.push(import('@sveltejs/vite-plugin-svelte').then((m) => m.svelte(svelteOptions)));
    }
    return plugins;
}

export default kitVite;
