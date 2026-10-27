// The frame's CGIs: service, alive, log, version, update_check, settings; the example tab CGI.
import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {copyFileSync, mkdirSync, utimesSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {CSRF, SID, regaSessions, setup} from './harness.mjs';
import {KIT_ROOT} from '../../cli/lib/kitroot.mjs';

const A = {sid: SID.admin};

describe('kit/service.cgi', () => {
    let t;
    before(async () => { t = await setup(); regaSessions(t); });
    after(() => t.close());

    it('passes the status line through verbatim, with the exit and memory headers', async () => {
        const line = '{"schema":1,"running":true,"state":"running","pid":2817,"rss_kb":48320,"since":1761562800,"version":"0.4.0","x_port":8091}';
        t.write('rc-status-out', `${line}\n`);
        t.write('rc-status-code', '0');
        const r = await t.cgi('www/kit/service.cgi', {query: {...A, cmd: 'status'}});
        assert.equal(r.status, 200);
        assert.equal(r.text, `${line}\n`);
        assert.equal(r.headers['x-kit-exit'], '0');
        assert.equal(r.headers['x-kit-mem-total-kb'], '1012340');
        assert.equal(r.headers['x-kit-mem-available-kb'], '612340');
        const d = await t.cgi('www/kit/service.cgi', {query: A});
        assert.equal(d.text, `${line}\n`, 'status is the default command');
    });

    it('wraps output that is not one JSON line', async () => {
        for (const [out, code] of [['hmm running, pid 12', '0'], ['{"a":1}\n{"b":2}', '0'], ['', '3']]) {
            t.write('rc-status-out', out);
            t.write('rc-status-code', code);
            const r = await t.cgi('www/kit/service.cgi', {query: {...A, cmd: 'status'}});
            assert.deepEqual(r.json, {schema: 0, running: code === '0', raw: out});
            assert.equal(r.headers['x-kit-exit'], code);
        }
    });

    it('runs an action with its output merged and records it', async () => {
        t.write('rc-calls', '');
        t.write('rc-action-code', '0');
        const r = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'restart'}});
        assert.equal(r.status, 200);
        assert.equal(r.json.ok, true);
        assert.equal(r.json.cmd, 'restart');
        assert.equal(r.json.exit, 0);
        assert.match(r.json.output, /restart on stdout/);
        assert.match(r.json.output, /restart on stderr/);
        assert.equal(t.read('rc-calls'), 'restart\n');
    });

    it('answers a failed action with 500 failed and the output', async () => {
        t.write('rc-action-code', '1');
        const r = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'start'}});
        assert.equal(r.status, 500);
        assert.equal(r.json.error.code, 'failed');
        assert.equal(r.json.exit, 1);
        assert.match(r.json.output, /start on stdout/);
        t.write('rc-action-code', '0');
    });

    it('refuses unknown commands, and commands kit.json leaves out', async () => {
        for (const cmd of ['uninstall', 'status', '', 'start;reboot']) {
            const r = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd}});
            assert.equal(r.status, 400, cmd);
            assert.equal(r.json.error.code, 'bad-request');
        }
        const g = await t.cgi('www/kit/service.cgi', {query: {...A, cmd: 'nonsense'}});
        assert.equal(g.status, 400);
        const nr = await setup({service: {commands: ['start', 'stop', 'restart']}});
        regaSessions(nr);
        const r = await nr.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'reload'}});
        assert.equal(r.status, 400);
        await nr.close();
    });

    it('answers 409 while another action holds the lock, and clears a stale one', async () => {
        const lock = join(t.env.KIT_TMP, 'kit-kitdemo-service.lock');
        mkdirSync(lock);
        const r = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'stop'}});
        assert.equal(r.status, 409);
        assert.equal(r.json.error.code, 'busy');
        const old = Date.now() / 1000 - 600;
        utimesSync(lock, old, old);
        const s = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'stop'}});
        assert.equal(s.status, 200);
        const again = await t.cgi('www/kit/service.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'stop'}});
        assert.equal(again.status, 200, 'the lock is released after the action');
    });

    it('times a hanging status out with 504 after 5 s', {timeout: 20000}, async () => {
        t.write('rc-status-sleep', '8');
        const started = Date.now();
        const r = await t.cgi('www/kit/service.cgi', {query: {...A, cmd: 'status'}});
        const took = Date.now() - started;
        t.write('rc-status-sleep', '0');
        assert.equal(r.status, 504);
        assert.equal(r.json.error.code, 'timeout');
        assert.ok(took >= 4500 && took < 8000, `took ${took} ms`);
    });
});

describe('kit/alive.cgi', () => {
    let t;
    before(async () => { t = await setup(); });
    after(() => t.close());

    it('answers running from the status exit code, without a session', async () => {
        t.write('rc-status-code', '0');
        assert.deepEqual((await t.cgi('www/kit/alive.cgi')).json, {running: true});
        t.write('rc-status-code', '3');
        assert.deepEqual((await t.cgi('www/kit/alive.cgi')).json, {running: false});
    });
});

describe('kit/log.cgi', () => {
    let t;
    before(async () => {
        t = await setup();
        regaSessions(t);
        const lines = [];
        for (let i = 1; i <= 300; i++) { lines.push(`line ${i}`); }
        lines.push('quote " backslash \\ tab \t bell \x07 umlaut ä €');
        t.write('kitdemo.log', `${lines.join('\n')}\n`);
        t.write('kitdemo.log.0', 'older 1\nolder 2\n');
        t.write('messages.0', 'Oct 27 09:00:00 ccu user.info kitdemo[12]: from the rotated file\n');
        t.write('messages', [
            'Oct 27 10:00:00 ccu user.info kitdemo[12]: one',
            'Oct 27 10:00:01 ccu user.info other[13]: not ours',
            'Oct 27 10:00:02 ccu user.info kitdemo: two',
            'Oct 27 10:00:03 ccu user.info kitdemox[14]: not ours either',
        ].join('\n') + '\n');
        mkdirSync(join(t.home, 'lib'), {recursive: true});
    });
    after(() => t.close());

    it('lists the sources', async () => {
        const r = await t.cgi('www/kit/log.cgi', {query: A});
        assert.equal(r.json.ok, true);
        const [main, sys] = r.json.sources;
        assert.equal(main.id, 'main');
        assert.equal(main.kind, 'file');
        assert.equal(main.available, true);
        assert.deepEqual(main.label, {de: 'Dienst', en: 'Service'});
        assert.ok(main.size > 1000 && Number.isInteger(main.mtime));
        assert.deepEqual(sys, {id: 'syslog', label: {de: 'Systemprotokoll', en: 'System log'}, kind: 'syslog', available: true});
    });

    it('tails a file as valid JSON lines', async () => {
        const r = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'main', lines: 3}});
        assert.deepEqual(r.json.lines, ['line 299', 'line 300', 'quote " backslash \\ tab \t bell \x07 umlaut ä €']);
        assert.equal(r.json.truncated, true);
        const all = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'main', lines: 2000}});
        assert.equal(all.json.lines.length, 301 + 2, 'the rotated file fills up');
        assert.equal(all.json.lines[0], 'older 1');
        const d = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'main'}});
        assert.equal(d.json.lines.length, 200);
    });

    it('reads at most the last 256 KiB and drops the partial first line', async () => {
        t.write('big.log', `${'x'.repeat(300000)}\npartial-free 1\nlast\n`);
        const big = await setup({logs: [{id: 'big', file: join(t.dir, 'big.log')}]});
        regaSessions(big);
        const r = await big.cgi('www/kit/log.cgi', {query: {...A, source: 'big', lines: 10}});
        assert.deepEqual(r.json.lines, ['partial-free 1', 'last']);
        assert.equal(r.json.truncated, true);
        await big.close();
    });

    it('filters the syslog by tag and reads the rotated file when short', async () => {
        const r = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'syslog', lines: 10}});
        assert.deepEqual(r.json.lines, [
            'Oct 27 09:00:00 ccu user.info kitdemo[12]: from the rotated file',
            'Oct 27 10:00:00 ccu user.info kitdemo[12]: one',
            'Oct 27 10:00:02 ccu user.info kitdemo: two',
        ]);
    });

    it('refuses an unknown source and a bad line count', async () => {
        const r = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'nope'}});
        assert.equal(r.status, 404);
        assert.equal(r.json.error.code, 'not-found');
        for (const lines of ['abc', '0', '-1']) {
            const b = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'main', lines}});
            assert.equal(b.status, 400, lines);
        }
    });

    it('offers a download and a debug bundle with the addon sections', async () => {
        const d = await t.cgi('www/kit/log.cgi', {query: {...A, source: 'main', download: 1}});
        assert.match(d.headers['content-type'], /^text\/plain/);
        assert.match(d.headers['content-disposition'], /^attachment; filename="kitdemo-main-\d{8}-\d{4}\.log"$/);
        assert.ok(d.text.startsWith('line 1\n'));
        writeFileSync(join(t.home, 'lib/kit-debug.tcl'), 'proc kit_debug_sections {} { return [list "addon extra" [list echo extra-section-output]] }\n');
        const b = await t.cgi('www/kit/log.cgi', {query: {...A, bundle: 1}});
        assert.match(b.headers['content-disposition'], /kitdemo-debug-/);
        for (const s of ['===== versions =====', 'VERSION_ADDON=0.4.0', '===== status =====', '===== addon extra =====', 'extra-section-output', '===== log main (file) =====', '===== log syslog (syslog) =====']) {
            assert.ok(b.text.includes(s), s);
        }
    });
});

const RELEASE = JSON.stringify({tag_name: 'v0.5.0', html_url: 'https://github.com/owner/repo/releases/tag/v0.5.0', draft: false});

describe('kit/version.cgi and update_check.cgi', () => {
    let t;
    before(async () => { t = await setup(); regaSessions(t); });
    after(() => t.close());
    const cache = () => join(t.env.KIT_TMP, 'kit-kitdemo-release.json');
    const calls = () => t.read('curl-count').split('\n').filter(Boolean).length;

    it('reports the installed version and the components', async () => {
        const r = await t.cgi('www/kit/version.cgi', {query: A});
        assert.equal(r.json.installed, '0.4.0');
        assert.deepEqual(r.json.components, {node: '22.20.0', mosquitto: '2.1.2'});
        assert.equal(r.json.kit, '0.1.0-test');
        assert.equal(r.json.platform.kind, 'openccu');
    });

    it('checks GitHub once and serves the cache afterwards', async () => {
        t.write('curl-body', RELEASE);
        t.write('curl-code', '200');
        t.write('curl-calls', '');
        t.write('curl-count', '');
        const r = await t.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
        assert.equal(r.json.latest, '0.5.0');
        assert.equal(r.json.newer, true);
        assert.equal(r.json.url, 'https://github.com/owner/repo/releases/tag/v0.5.0');
        assert.equal(r.json.source, 'network');
        assert.match(t.read('curl-calls'), /https:\/\/api\.github\.com\/repos\/owner\/repo\/releases\/latest/);
        const c = await t.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
        assert.equal(c.json.source, 'cache');
        assert.equal(c.json.latest, '0.5.0');
        assert.equal(calls(), 1);
        const u = await t.cgi('www/update_check.cgi', {query: {cmd: 'check_version', version: '0.4.0'}});
        assert.equal(u.text, '0.5.0');
        assert.match(u.headers['content-type'], /^text\/plain/);
        assert.equal(calls(), 1, 'update_check shares the cache');
        const dl = await t.cgi('www/update_check.cgi', {query: {cmd: 'download'}});
        assert.equal(dl.status, 302);
        assert.equal(dl.headers.location, 'https://github.com/owner/repo/releases/tag/v0.5.0');
    });

    it('forces a check only by a CSRF-checked POST, and not within 60 s', async () => {
        const no = await t.cgi('www/kit/version.cgi', {method: 'POST', query: A, body: {cmd: 'check', force: 1}});
        assert.equal(no.status, 403);
        assert.equal(no.json.error.code, 'csrf');
        const n = calls();
        const fresh = await t.cgi('www/kit/version.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'check', force: 1}});
        assert.equal(fresh.json.source, 'cache', 'a cache younger than 60 s stands');
        const old = Math.floor(Date.now() / 1000) - 120;
        writeFileSync(cache(), JSON.stringify({latest: '0.5.0', url: null, checkedAt: old, error: null}));
        const f = await t.cgi('www/kit/version.cgi', {method: 'POST', query: A, headers: CSRF, body: {cmd: 'check', force: 1}});
        assert.equal(f.json.source, 'network');
        assert.equal(calls(), n + 1);
    });

    it('says offline, rate-limited or the HTTP status, and caches errors briefly', async () => {
        const cases = [
            [() => { t.write('curl-exit', '6'); }, 'offline'],
            [() => { t.write('curl-body', '{"message":"API rate limit exceeded for 192.0.2.1."}'); t.write('curl-code', '403'); }, 'rate-limited'],
            [() => { t.write('curl-body', '<html>'); t.write('curl-code', '503'); }, 'http-503'],
            [() => { t.write('curl-body', '{"message":"Not Found"}'); t.write('curl-code', '404'); }, 'no-release'],
        ];
        for (const [prepare, error] of cases) {
            writeFileSync(cache(), '');
            prepare();
            const r = await t.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
            assert.equal(r.json.ok, true);
            assert.equal(r.json.error, error);
            assert.equal(r.json.latest, null);
            assert.equal(r.json.newer, false);
            const u = await t.cgi('www/update_check.cgi', {query: {cmd: 'check_version'}});
            assert.equal(u.text, 'n/a');
            try { (await import('node:fs')).rmSync(t.file('curl-exit')); } catch { /* not there */ }
        }
        const n = calls();
        const again = await t.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
        assert.equal(again.json.source, 'cache');
        assert.equal(calls(), n);
        const stale = Math.floor(Date.now() / 1000) - 1801;
        writeFileSync(cache(), JSON.stringify({latest: null, url: null, checkedAt: stale, error: 'offline'}));
        t.write('curl-body', RELEASE);
        t.write('curl-code', '200');
        const after30 = await t.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
        assert.equal(after30.json.source, 'network', 'an error is cached for 30 min only');
    });

    it('answers not-configured without update.github', async () => {
        const n = await setup({update: undefined});
        regaSessions(n);
        const r = await n.cgi('www/kit/version.cgi', {query: {...A, cmd: 'check'}});
        assert.equal(r.json.error, 'not-configured');
        assert.equal((await n.cgi('www/update_check.cgi')).text, 'n/a');
        assert.equal((await n.cgi('www/update_check.cgi', {query: {cmd: 'download'}})).status, 404);
        await n.close();
    });
});

describe('settings.cgi', () => {
    let t;
    before(async () => { t = await setup(); });
    after(() => t.close());

    it('serves the frame page without a session, never cached, no referrer', async () => {
        const r = await t.cgi('www/settings.cgi', {query: {sid: '@whatever00@'}});
        assert.equal(r.status, 200);
        assert.match(r.headers['content-type'], /^text\/html/);
        assert.equal(r.headers['cache-control'], 'no-store');
        assert.equal(r.headers['referrer-policy'], 'no-referrer');
        assert.equal(r.headers['x-frame-options'], undefined);
        assert.match(r.text, /ccu-addon-kit \(MIT\): the placeholder page/);
        assert.match(r.text, /<title>Kit Demo „Ä“<\/title>/);
        assert.match(r.text, /var ID = "kitdemo";/);
    });
});

describe('the example tab CGI (kitdemo www/api/hello.cgi)', () => {
    let t;
    before(async () => {
        t = await setup();
        regaSessions(t);
        mkdirSync(join(t.home, 'www/api'), {recursive: true});
        copyFileSync(join(KIT_ROOT, 'example/kitdemo/addon_files/kitdemo/www/api/hello.cgi'), join(t.home, 'www/api/hello.cgi'));
        t.env.KIT_HOME = t.home;
    });
    after(() => t.close());

    it('reads and writes the greeting', async () => {
        let r = await t.cgi('www/api/hello.cgi', {query: A});
        assert.deepEqual(r.json, {ok: true, greeting: ''});
        r = await t.cgi('www/api/hello.cgi', {method: 'POST', query: A, headers: CSRF, body: {greeting: 'Grüß dich [exec id] $x'}});
        assert.deepEqual(r.json, {ok: true, saved: true});
        r = await t.cgi('www/api/hello.cgi', {query: A});
        assert.equal(r.json.greeting, 'Grüß dich [exec id] $x');
        r = await t.cgi('www/api/hello.cgi', {method: 'POST', query: A, headers: CSRF, body: {greeting: 'x'.repeat(81)}});
        assert.equal(r.status, 400);
        r = await t.cgi('www/api/hello.cgi', {method: 'POST', query: A, body: {greeting: 'no header'}});
        assert.equal(r.status, 403);
    });

    it('turns a Tcl error into a 500 internal answer', async () => {
        writeFileSync(join(t.home, 'www/api/broken.cgi'), `set KIT_HOME ${t.home}\nsource $KIT_HOME/lib/kit/cgi.tcl\nkit_main {\n    kit_require read\n    no_such_command 1 2\n}\n`);
        const r = await t.cgi('www/api/broken.cgi', {query: A});
        assert.equal(r.status, 500);
        assert.equal(r.json.error.code, 'internal');
        assert.match(r.json.error.message, /no_such_command/);
    });
});
