import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync, mkdirSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {vendor} from '../../cli/vendor.mjs';
import {scaffold} from '../../cli/new.mjs';
import {KIT_ROOT} from '../../cli/lib/kitroot.mjs';
import {lint} from '../tcl/lint-tcl82.mjs';

const quiet = {log: () => {}, warn: () => {}};

describe('vendor', () => {
    let dir;
    let config;
    before(() => {
        dir = mkdtempSync(join(tmpdir(), 'kit-vendor-'));
        config = join(dir, 'kit.json');
        writeFileSync(config, JSON.stringify({schema: 1, id: 'demo', name: {de: 'Demo', en: 'Demo'}, tabs: ['hello', 'kit:service'],
            proxied: {path: '/addons/demo/app/', errorPage: true}}));
    });
    after(() => rmSync(dir, {recursive: true, force: true}));

    it('writes the library, the stubs and the generated files', () => {
        const out = join(dir, 'a');
        const warnings = [];
        const r = vendor({config, out, log: () => {}, warn: (m) => warnings.push(m)});
        assert.ok(r.written.length > 20);
        for (const f of ['lib/kit/cgi.tcl', 'lib/kit/session.tcl', 'lib/kit/cgi/service.tcl', 'lib/kit/addon.tcl', 'lib/kit/VERSION', 'lib/kit/MANIFEST',
            'bin/kit-run', 'bin/update_addon', 'www/settings.cgi', 'www/update_check.cgi', 'www/kit/session.cgi', 'www/kit/service.cgi',
            'www/kit/alive.cgi', 'www/kit/log.cgi', 'www/kit/version.cgi', 'www/frame/addon.json', 'www/frame/index.html', 'www/lighttpd-error-503.html']) {
            assert.ok(existsSync(join(out, f)), f);
        }
        assert.equal(statSync(join(out, 'www/kit/service.cgi')).mode & 0o777, 0o755);
        assert.equal(statSync(join(out, 'bin/kit-run')).mode & 0o777, 0o755);
        const stub = readFileSync(join(out, 'www/kit/service.cgi'), 'utf8');
        assert.match(stub, /^#!\/bin\/tclsh\n/);
        assert.match(stub, /^set KIT_HOME \/usr\/local\/addons\/demo$/m);
        assert.match(stub, /^source \$KIT_HOME\/lib\/kit\/cgi\/service\.tcl$/m);
        assert.match(readFileSync(join(out, 'www/lighttpd-error-503.html'), 'utf8'), /"\/addons\/demo" \+ '\/kit\/alive\.cgi'/);
        assert.deepEqual(warnings, ['no built frame (www/frame/frame-manifest.json): tabs hello not checked']);
        assert.deepEqual(lint([join(out, 'lib/kit/addon.tcl'), join(out, 'www/kit/log.cgi')]), []);
    });

    it('passes --check right after, and fails it on drift', () => {
        const out = join(dir, 'b');
        vendor({config, out, ...quiet});
        vendor({config, out, check: true, ...quiet});
        writeFileSync(join(out, 'www/kit/log.cgi'), 'changed');
        assert.throws(() => vendor({config, out, check: true, ...quiet}), /www\/kit\/log\.cgi/);
    });

    it('refuses to overwrite an edited library file without --force', () => {
        const out = join(dir, 'c');
        vendor({config, out, ...quiet});
        writeFileSync(join(out, 'lib/kit/json.tcl'), '# edited\n');
        assert.throws(() => vendor({config, out, ...quiet}), /edited kit files[\s\S]*lib\/kit\/json\.tcl/);
        vendor({config, out, force: true, ...quiet});
        assert.notEqual(readFileSync(join(out, 'lib/kit/json.tcl'), 'utf8'), '# edited\n');
    });

    it('keeps a built frame and checks its tabs', () => {
        const out = join(dir, 'd');
        mkdirSync(join(out, 'www/frame'), {recursive: true});
        writeFileSync(join(out, 'www/frame/index.html'), '<!doctype html><title>built</title>');
        writeFileSync(join(out, 'www/frame/frame-manifest.json'), JSON.stringify({tabs: ['other']}));
        assert.throws(() => vendor({config, out, ...quiet}), /tab "hello" is in kit.json but not registered/);
        writeFileSync(join(out, 'www/frame/frame-manifest.json'), JSON.stringify({tabs: ['hello']}));
        vendor({config, out, ...quiet});
        assert.equal(readFileSync(join(out, 'www/frame/index.html'), 'utf8'), '<!doctype html><title>built</title>');
    });
});

describe('the example and the scaffold', () => {
    it('kitdemo uses the reference rc.d and update_script unchanged', () => {
        const ref = readFileSync(join(KIT_ROOT, 'classic/sh/rc.d-reference'), 'utf8');
        assert.equal(readFileSync(join(KIT_ROOT, 'example/kitdemo/addon_files/kitdemo/rc.d'), 'utf8'), ref);
        assert.match(ref, /^ADDON=kitdemo$/m);
        assert.equal(readFileSync(join(KIT_ROOT, 'example/kitdemo/addon_files/update_script'), 'utf8'),
            readFileSync(join(KIT_ROOT, 'classic/sh/update_script-reference'), 'utf8'));
    });

    it('new scaffolds an addon with the id replaced', () => {
        const dir = mkdtempSync(join(tmpdir(), 'kit-new-'));
        try {
            const r = scaffold({id: 'battery', name: 'Batterien', dir});
            assert.ok(r.written.includes(join('battery', 'addon_files/battery/rc.d')));
            assert.ok(r.written.includes(join('battery', 'addon_files/battery.cfg')));
            const rc = readFileSync(join(dir, 'battery/addon_files/battery/rc.d'), 'utf8');
            assert.match(rc, /^ADDON=battery$/m);
            assert.equal(rc.includes('kitdemo'), false);
            assert.equal(statSync(join(dir, 'battery/addon_files/battery/rc.d')).mode & 0o777, 0o755);
            const kit = JSON.parse(readFileSync(join(dir, 'battery/kit.json'), 'utf8'));
            assert.equal(kit.id, 'battery');
            assert.equal(r.written.some((p) => p.includes('/lib/kit/') || p.includes('/www/kit/')), false);
            assert.throws(() => scaffold({id: 'battery', dir}), /exists already/);
            assert.throws(() => scaffold({id: 'Bad', dir}), /must match/);
        } finally {
            rmSync(dir, {recursive: true, force: true});
        }
    });
});
