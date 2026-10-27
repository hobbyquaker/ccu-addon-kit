// ccu-addon-kit new <id> [--name "Name"] [--dir <parent>]: scaffolds an addon from
// example/kitdemo - the kit.json, update_script, rc.d script (the convention's reference),
// the hm_addons.cfg entry, the openccu-lite manifest, the example daemon and tab CGI, the
// settings page (web/: the frame and the example tab), the build script - with "kitdemo" and
// "Kit Demo" replaced - and a package.json with the kit and the frame's build dependencies.

import {existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync, chmodSync} from 'node:fs';
import {join, relative} from 'node:path';
import {KIT_ROOT, kitVersion} from './lib/kitroot.mjs';

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
    // kitdemo lives inside the kit and uses its dependencies; a scaffold brings its own
    const kit = JSON.parse(readFileSync(join(KIT_ROOT, 'package.json'), 'utf8'));
    const pin = (name) => kit.devDependencies[name];
    const pkg = {
        name: id,
        version: '0.0.0',
        private: true,
        type: 'module',
        scripts: {build: 'sh build.sh', vendor: `ccu-addon-kit vendor --config kit.json --out addon_files/${id} --frame web/dist`,
            check: `ccu-addon-kit vendor --config kit.json --out addon_files/${id} --frame web/dist --check`},
        devDependencies: {
            '@sveltejs/vite-plugin-svelte': pin('@sveltejs/vite-plugin-svelte'),
            'ccu-addon-kit': `github:hobbyquaker/ccu-addon-kit#v${kitVersion().replace(/-dev$/, '')}`,
            'occulite-ui': pin('occulite-ui'),
            svelte: pin('svelte'),
            vite: pin('vite'),
        },
    };
    writeFileSync(join(dst, 'package.json'), `${JSON.stringify(pkg, null, 2)}\n`);
    written.push(relative(dir, join(dst, 'package.json')));
    return {dir: dst, written};
}
