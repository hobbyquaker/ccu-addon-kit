#!/usr/bin/env node
// The size of the settings page an addon ships (ccu-addon-kit, MIT): kitdemo's frame built the
// way an addon builds it (kitVite, minified, one Svelte runtime, occulite-ui), JS and CSS
// gzipped. Fails when it is more than 10 % over test/frame/size-budget.json; --update writes
// the measured size as the new budget.
import {mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {gzipSync} from 'node:zlib';
import {build} from 'vite';

const ROOT = resolve(import.meta.dirname, '../..');
const BUDGET = join(ROOT, 'test/frame/size-budget.json');
const TOLERANCE = 0.1;

const out = mkdtempSync(join(tmpdir(), 'kit-size-'));
try {
    await build({configFile: join(ROOT, 'example/kitdemo/web/vite.config.mjs'), logLevel: 'error', build: {outDir: out}});
    const sizes = {js: 0, css: 0};
    const walk = (d) => readdirSync(d).flatMap((f) => (statSync(join(d, f)).isDirectory() ? walk(join(d, f)) : [join(d, f)]));
    for (const f of walk(out)) {
        if (f.endsWith('.js')) sizes.js += gzipSync(readFileSync(f)).length;
        if (f.endsWith('.css')) sizes.css += gzipSync(readFileSync(f)).length;
    }
    const total = sizes.js + sizes.css;
    const budget = JSON.parse(readFileSync(BUDGET, 'utf8')).kitdemo;
    const kb = (b) => `${(b / 1024).toFixed(1)} KB`;
    const over = total > budget * (1 + TOLERANCE);
    const report = `kitdemo's settings page (gzip): JS ${kb(sizes.js)}, CSS ${kb(sizes.css)}, total ${kb(total)}; budget ${kb(budget)} (+${TOLERANCE * 100} %)${over ? ' - OVER' : ''}`;
    console.log(report);
    if (process.env.GITHUB_STEP_SUMMARY) writeFileSync(process.env.GITHUB_STEP_SUMMARY, `### Size\n\n${report}\n`, {flag: 'a'});
    if (process.argv.includes('--update')) {
        writeFileSync(BUDGET, `${JSON.stringify({kitdemo: total}, null, 2)}\n`);
        console.log('size-budget.json updated');
    } else if (over) {
        process.exitCode = 1;
    }
} finally {
    rmSync(out, {recursive: true, force: true});
}
