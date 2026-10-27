// ccu-addon-kit new <id> [--name "Name"] [--dir <parent>]: scaffolds an addon from
// example/kitdemo - the kit.json, update_script, rc.d script (the convention's reference),
// the hm_addons.cfg entry, the openccu-lite manifest, the example daemon and tab CGI, the
// build script - with "kitdemo" and "Kit Demo" replaced.

import {existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync, chmodSync} from 'node:fs';
import {join, relative} from 'node:path';
import {KIT_ROOT} from './lib/kitroot.mjs';

const SKIP = new Set(['dist', 'node_modules', '.gitignore-vendored']);
// what vendor and the build write; never part of a scaffold
const GENERATED = [/^addon_files\/[^/]+\/lib\/kit(\/|$)/, /^addon_files\/[^/]+\/www\/kit(\/|$)/, /^addon_files\/[^/]+\/www\/frame(\/|$)/,
    /^addon_files\/[^/]+\/bin\/(kit-run|update_addon)$/, /^addon_files\/[^/]+\/www\/(settings|update_check)\.cgi$/,
    /^addon_files\/[^/]+\/versions$/, /^addon_files\/[^/]+\/www\/lighttpd-error-503\.html$/];

function walk(dir, root = dir) {
    const out = [];
    for (const name of readdirSync(dir).sort()) {
        if (SKIP.has(name)) { continue; }
        const p = join(dir, name);
        const rel = relative(root, p);
        if (GENERATED.some((re) => re.test(rel))) { continue; }
        if (statSync(p).isDirectory()) { out.push(...walk(p, root)); } else { out.push(rel); }
    }
    return out;
}

export function scaffold({id, name, dir = '.'}) {
    if (!/^[a-z0-9][a-z0-9-]{0,31}$/.test(id)) { throw new Error('the id must match [a-z0-9][a-z0-9-]{0,31}'); }
    if (id === 'kitdemo') { throw new Error('pick an id of your own'); }
    name = name || id;
    const src = join(KIT_ROOT, 'example/kitdemo');
    const dst = join(dir, id);
    if (existsSync(dst)) { throw new Error(`${dst} exists already`); }
    const written = [];
    for (const rel of walk(src)) {
        const from = join(src, rel);
        const to = join(dst, rel.replaceAll('kitdemo', id));
        mkdirSync(join(to, '..'), {recursive: true});
        const text = readFileSync(from, 'utf8')
            .replaceAll('Kit Demo', name)
            .replaceAll('kitdemo', id)
            .replaceAll('KITDEMO', id.toUpperCase().replace(/-/g, '_'));
        writeFileSync(to, text);
        chmodSync(to, statSync(from).mode & 0o777);
        written.push(relative(dir, to));
    }
    return {dir: dst, written};
}
