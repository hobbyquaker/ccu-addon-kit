// ccu-addon-kit vendor: copies the kit's classic part into an addon's tree and generates the
// files that depend on its kit.json (docs/vendoring.md).
//
//   ccu-addon-kit vendor --config kit.json --out addon_files/<id> [--frame web/dist] [--force] [--check]
//
// --frame copies a frame built with kitVite() into www/frame/ (and removes what an earlier
// build left there); without it a frame built straight into www/frame/ is kept, and the
// placeholder page is installed when there is none.
// --check writes nothing and fails when the tree differs from what this kit would produce
// (the drift check for an addon's CI).

import {createHash} from 'node:crypto';
import {existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync, chmodSync} from 'node:fs';
import {dirname, join, relative} from 'node:path';
import {execFileSync} from 'node:child_process';
import {KIT_TABS, addonJson, addonTcl, lighttpdConf, normalizeKit} from './lib/kitconfig.mjs';
import {KIT_ROOT, kitVersion} from './lib/kitroot.mjs';

const CGIS = {
    'www/settings.cgi': 'settings',
    'www/update_check.cgi': 'update_check',
    'www/kit/session.cgi': 'session',
    'www/kit/service.cgi': 'service',
    'www/kit/alive.cgi': 'alive',
    'www/kit/log.cgi': 'log',
    'www/kit/version.cgi': 'version',
};
const PLACEHOLDER_MARK = 'ccu-addon-kit (MIT): the placeholder page';
const MANIFEST = 'lib/kit/MANIFEST';

function sha(data) { return createHash('sha256').update(data).digest('hex'); }

function walk(dir) {
    const out = [];
    for (const name of readdirSync(dir).sort()) {
        const p = join(dir, name);
        if (statSync(p).isDirectory()) { out.push(...walk(p)); } else { out.push(p); }
    }
    return out;
}

function kitCommit() {
    try {
        return execFileSync('git', ['-C', KIT_ROOT, 'rev-parse', '--short=12', 'HEAD'], {stdio: ['ignore', 'pipe', 'ignore']}).toString().trim();
    } catch {
        return 'unknown';
    }
}

function htmlEscape(s) { return s.replace(/[&<>"']/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'})[c]); }
// a JS string literal that is also safe inside <script>
function jsString(s) {
    return JSON.stringify(s).replace(/</g, '\\u003c').split(String.fromCharCode(0x2028)).join('\\u2028')
        .split(String.fromCharCode(0x2029)).join('\\u2029');
}

function fillPage(template, kit) {
    const base = `/addons/${kit.www}`;
    return template
        .replaceAll('@NAME_DE_HTML@', htmlEscape(kit.name.de))
        .replaceAll('@NAME_EN_HTML@', htmlEscape(kit.name.en))
        .replaceAll('@NAME_DE_JS@', jsString(kit.name.de))
        .replaceAll('@NAME_EN_JS@', jsString(kit.name.en))
        .replaceAll('@ID_JS@', jsString(kit.id))
        .replaceAll('@BASE_JS@', jsString(base));
}

// every file vendor owns, as {path: {data, mode}}, relative to the addon's tree
export function plan(kit, {out, frame, version = kitVersion(), commit = kitCommit()} = {}) {
    const files = {};
    const add = (path, data, mode = 0o644) => { files[path] = {data: Buffer.isBuffer(data) ? data : Buffer.from(data), mode}; };
    const tclRoot = join(KIT_ROOT, 'classic/tcl/lib/kit');
    for (const p of walk(tclRoot)) { add(join('lib/kit', relative(tclRoot, p)), readFileSync(p)); }
    add('lib/kit/rc-helpers.sh', readFileSync(join(KIT_ROOT, 'classic/sh/rc-helpers.sh')));
    add('lib/kit/addon.tcl', addonTcl(kit, version));
    add('lib/kit/VERSION', `${version} ${commit}\n`);
    add('bin/kit-run', readFileSync(join(KIT_ROOT, 'classic/sh/kit-run')), 0o755);
    add('bin/update_addon', readFileSync(join(KIT_ROOT, 'classic/sh/update_addon')), 0o755);
    const stub = readFileSync(join(KIT_ROOT, 'classic/cgi/stub.cgi.in'), 'utf8');
    for (const [path, cgi] of Object.entries(CGIS)) {
        add(path, stub.replaceAll('@NAME@', path.replace(/^www\//, '')).replaceAll('@KIT_VERSION@', version)
            .replaceAll('@KIT_HOME@', kit.addonDir).replaceAll('@CGI@', cgi), 0o755);
    }
    add('www/frame/addon.json', addonJson(kit, version));
    const fragment = lighttpdConf(kit, version);
    if (fragment) { add('etc/lighttpd.conf', fragment); }
    if (kit.proxied && kit.proxied.errorPage) {
        add('www/lighttpd-error-503.html', fillPage(readFileSync(join(KIT_ROOT, 'classic/html/lighttpd-error-503.html'), 'utf8'), kit));
    }
    if (frame) {
        if (!existsSync(join(frame, 'index.html')) || !existsSync(join(frame, 'frame-manifest.json'))) {
            throw new Error(`--frame ${frame}: no frame built with kitVite() there (index.html, frame-manifest.json)`);
        }
        for (const p of walk(frame)) {
            const rel = relative(frame, p);
            if (rel !== 'addon.json') { add(join('www/frame', rel), readFileSync(p)); }
        }
    }
    // the placeholder until the addon builds the frame; a built frame is never touched
    const index = out ? join(out, 'www/frame/index.html') : null;
    if (!frame && (!index || !existsSync(index) || readFileSync(index, 'utf8').includes(PLACEHOLDER_MARK))) {
        add('www/frame/index.html', fillPage(readFileSync(join(KIT_ROOT, 'classic/html/frame-placeholder.html'), 'utf8'), kit));
    }
    const manifest = Object.keys(files).sort().map((p) => `${sha(files[p].data)}  ${p}`).join('\n');
    add(MANIFEST, `${manifest}\n`);
    return files;
}

// the frame's registered tab ids (kitVite writes frame-manifest.json); warnings, not errors,
// while no frame is built
function checkTabs(kit, out, frame, warn) {
    const addonTabs = kit.tabs.filter((t) => !KIT_TABS.includes(t));
    const mf = join(frame ?? join(out, 'www/frame'), 'frame-manifest.json');
    if (!existsSync(mf)) {
        if (addonTabs.length) { warn(`no built frame (www/frame/frame-manifest.json): tabs ${addonTabs.join(', ')} not checked`); }
        return [];
    }
    const registered = JSON.parse(readFileSync(mf, 'utf8')).tabs || [];
    return addonTabs.filter((t) => !registered.includes(t)).map((t) => `tab "${t}" is in kit.json but not registered in the frame`);
}

// the addon's own files must name the same www directory: the Einstellungen button's
// CONFIG_URL (<id>.cfg next to the tree) and WWW= of the rc.d script
function checkWww(kit, out) {
    const errors = [];
    const url = `/addons/${kit.www}/settings.cgi`;
    const cfg = join(out, '..', `${kit.id}.cfg`);
    if (existsSync(cfg)) {
        const m = /CONFIG_URL\s+(\S+)/.exec(readFileSync(cfg, 'utf8'));
        if (m && m[1] !== url) { errors.push(`${cfg}: CONFIG_URL ${m[1]}, kit.json's www says ${url}`); }
    }
    for (const script of [join(out, 'rc.d'), join(out, '..', 'update_script')]) {
        if (!existsSync(script)) { continue; }
        const m = /^WWW=(\S+)/m.exec(readFileSync(script, 'utf8'));
        if (m && m[1] !== kit.www && !(m[1] === '$ADDON' && kit.www === kit.id)) {
            errors.push(`${script}: WWW=${m[1]}, kit.json's www is ${kit.www}`);
        }
    }
    return errors;
}

export function vendor({config, out, frame, force = false, check = false, log = console.log, warn = (m) => console.error(`warning: ${m}`)}) {
    const raw = JSON.parse(readFileSync(config, 'utf8'));
    const kit = normalizeKit(raw);
    const errors = [...checkTabs(kit, out, frame, warn), ...checkWww(kit, out)];
    if (errors.length) { throw new Error(errors.join('\n')); }
    const files = plan(kit, {out, frame});
    // a copied frame owns www/frame/: what an earlier build left there goes
    const stale = [];
    if (frame && existsSync(join(out, 'www/frame'))) {
        for (const p of walk(join(out, 'www/frame'))) {
            const rel = relative(out, p);
            if (!files[rel]) { stale.push(rel); }
        }
    }
    const differs = [];
    for (const [path, {data, mode}] of Object.entries(files)) {
        const p = join(out, path);
        const same = existsSync(p) && readFileSync(p).equals(data) && (statSync(p).mode & 0o777) === mode;
        if (!same) { differs.push(path); }
    }
    if (check) {
        differs.push(...stale);
        if (differs.length) {
            throw new Error(`the vendored files differ from ccu-addon-kit ${kitVersion()}:\n  ${differs.join('\n  ')}\nre-run: ccu-addon-kit vendor --config ${config} --out ${out}`);
        }
        log(`vendor --check: ${Object.keys(files).length} files match`);
        return {kit, files, written: []};
    }
    // a copy somebody edited is refused: its hash differs from the one vendor recorded
    const mfPath = join(out, MANIFEST);
    if (!force && existsSync(mfPath)) {
        const recorded = new Map(readFileSync(mfPath, 'utf8').trim().split('\n').filter(Boolean).map((l) => {
            const [h, ...rest] = l.split('  ');
            return [rest.join('  '), h];
        }));
        const edited = [];
        for (const path of differs) {
            const p = join(out, path);
            if (path !== MANIFEST && path.startsWith('lib/kit/') && existsSync(p) && recorded.has(path) && recorded.get(path) !== sha(readFileSync(p))) {
                edited.push(path);
            }
        }
        if (edited.length) {
            throw new Error(`refusing to overwrite edited kit files (use --force):\n  ${edited.join('\n  ')}`);
        }
    }
    for (const path of stale) { rmSync(join(out, path)); }
    for (const path of differs) {
        const p = join(out, path);
        mkdirSync(dirname(p), {recursive: true});
        writeFileSync(p, files[path].data);
        chmodSync(p, files[path].mode);
    }
    log(`vendor: ${differs.length} of ${Object.keys(files).length} files written into ${out}${stale.length ? `, ${stale.length} stale removed` : ''}`);
    return {kit, files, written: differs};
}
