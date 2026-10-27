// The end-to-end test (the CCU addon handbook, chapter 08 sections 2 and 2b): builds kitdemo,
// replays OpenCCU's install_addon in the CCU-like container, runs the service convention's
// conformance test against the installed rc.d script, calls every kit CGI through lighttpd
// with a stub ReGa session, updates and uninstalls.
//
//   docker build -f test/e2e/Dockerfile -t ccu-addon-kit-e2e test/ && node --test test/e2e/
//
// Needs docker. E2E_IMAGE names the image (default ccu-addon-kit-e2e).

import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync, spawnSync} from 'node:child_process';
import {readdirSync} from 'node:fs';
import {join} from 'node:path';
import {KIT_ROOT} from '../../cli/lib/kitroot.mjs';
import {checkRc} from '../../cli/check-rc.mjs';

const IMAGE = process.env.E2E_IMAGE || 'ccu-addon-kit-e2e';
const NAME = `kit-e2e-${process.pid}`;
const ADDON = '/usr/local/addons/kitdemo';
const RC = '/usr/local/etc/config/rc.d/kitdemo';
const VERSION = '0.1.0-e2e.1';
const SID = '@E2eAdmin01@';

function sh(script, {ok = true} = {}) {
    const r = spawnSync('docker', ['exec', NAME, 'sh', '-c', script], {encoding: 'utf8'});
    if (ok && r.status !== 0) { throw new Error(`exit ${r.status}: ${script}\n${r.stdout}\n${r.stderr}`); }
    return {code: r.status, out: r.stdout, err: r.stderr};
}

// one request through lighttpd: {status, headers, body, json}
function http(path, {method = 'GET', headers = {}, data} = {}) {
    const args = ['-s', '-i', '-X', method];
    for (const [k, v] of Object.entries(headers)) { args.push('-H', `${k}: ${v}`); }
    if (data !== undefined) { args.push('--data', data); }
    args.push(`http://127.0.0.1${path}`);
    const r = spawnSync('docker', ['exec', NAME, 'curl', ...args], {encoding: 'utf8'});
    const raw = r.stdout;
    const sep = raw.indexOf('\r\n\r\n');
    const head = raw.slice(0, sep).split('\r\n');
    const status = Number(head[0].split(' ')[1]);
    const hdrs = {};
    for (const l of head.slice(1)) { const i = l.indexOf(':'); hdrs[l.slice(0, i).toLowerCase()] = l.slice(i + 1).trim(); }
    const body = raw.slice(sep + 4);
    let json;
    try { json = JSON.parse(body); } catch { /* not JSON */ }
    return {status, headers: hdrs, body, json};
}

const POST = {'X-CCU-Addon-Request': '1', Origin: 'http://127.0.0.1', 'Content-Type': 'application/x-www-form-urlencoded'};

function installAddon() {
    return sh(`dir=$(mktemp -d -p /usr/local/tmp) && tar -C "$dir" --no-same-owner --no-same-permissions -xzf /dist/kitdemo-${VERSION}.tar.gz \
        && cd "$dir" && sha256sum -c update_script.sha256 >/dev/null && ./update_script HM-RASPBERRYMATIC >/tmp/update_script.log 2>&1; rc=$?; cd /; rm -rf "$dir"; exit $rc`, {ok: false}).code;
}

describe('kitdemo end to end', {timeout: 600000}, () => {
    before(() => {
        execFileSync('sh', [join(KIT_ROOT, 'example/kitdemo/build.sh')], {
            env: {...process.env, VERSION, KIT: `node ${join(KIT_ROOT, 'cli/index.mjs')}`}, stdio: ['ignore', 'ignore', 'inherit'],
        });
        assert.ok(readdirSync(join(KIT_ROOT, 'example/kitdemo/dist')).includes(`kitdemo-${VERSION}.tar.gz`));
        execFileSync('docker', ['run', '-d', '--init', '--name', NAME, '-v', `${join(KIT_ROOT, 'example/kitdemo/dist')}:/dist:ro`, IMAGE], {stdio: 'ignore'});
        for (let i = 0; i < 50 && sh('test -f /var/log/messages && curl -s -o /dev/null http://127.0.0.1/', {ok: false}).code !== 0; i++) {
            execFileSync('sleep', ['0.2']);
        }
        sh(`echo "${SID.slice(1, -1)} 1001;8;Admin;;;" > /tmp/kit-rega-sids`);
    });
    after(() => { spawnSync('docker', ['rm', '-f', NAME], {stdio: 'ignore'}); });

    it('installs fresh with exit 10, the links and the button', () => {
        assert.equal(installAddon(), 10, sh('cat /tmp/update_script.log', {ok: false}).out);
        assert.equal(sh(`test -x ${RC} && test -L /usr/local/etc/config/addons/www/kitdemo && echo ok`).out.trim(), 'ok');
        const cfg = sh('cat /usr/local/etc/config/hm_addons.cfg').out;
        assert.match(cfg, /kitdemo \{CONFIG_URL \/addons\/kitdemo\/settings\.cgi/);
        assert.match(sh(`${RC} info`).out, /^Name: Kit Demo$/m);
        assert.equal(sh(`cat ${ADDON}/etc/kitdemo.conf ${ADDON}/etc/greeting.txt >/dev/null && echo ok`).out.trim(), 'ok', 'the defaults were copied');
        assert.equal(sh(`test -x ${ADDON}/bin/kit-run && test -f ${ADDON}/lib/kit/addon.tcl && test -f ${ADDON}/www/frame/index.html && echo ok`).out.trim(), 'ok');
    });

    it('conforms to the service convention (check-rc C1-C12)', async () => {
        const report = await checkRc({exec: `docker exec ${NAME}`, rc: RC, log: '/var/log/kitdemo.log'});
        const failed = report.results.filter((r) => r.status !== 'pass');
        assert.ok(report.ok, JSON.stringify(report.results, null, 2));
        assert.deepEqual(failed, [], 'no warnings either');
    });

    it('answers the kit CGIs through lighttpd', () => {
        sh(`${RC} start`);
        const page = http('/addons/kitdemo/settings.cgi?sid=%40E2eAdmin01%40');
        assert.equal(page.status, 200);
        assert.equal(page.headers['referrer-policy'], 'no-referrer');
        assert.match(page.body, /<title>Kit Demo<\/title>/);
        assert.match(http('/addons/kitdemo/frame/addon.json').body, /"id": "kitdemo"/);

        const none = http('/addons/kitdemo/kit/session.cgi');
        assert.deepEqual(none.json.session, {valid: false});
        const s = http(`/addons/kitdemo/kit/session.cgi?sid=${encodeURIComponent(SID)}`);
        assert.deepEqual(s.json.session, {valid: true, level: 'admin', user: 'Admin', via: 'sid'});
        assert.equal(s.json.platform.kind, 'openccu');

        const st = http(`/addons/kitdemo/kit/service.cgi?cmd=status&sid=${encodeURIComponent(SID)}`);
        assert.equal(st.status, 200);
        assert.equal(st.json.schema, 1);
        assert.equal(st.json.running, true);
        assert.equal(st.json.version, VERSION);
        assert.equal(st.headers['x-kit-exit'], '0');
        assert.ok(Number(st.headers['x-kit-mem-total-kb']) > 0);
        assert.equal(http('/addons/kitdemo/kit/service.cgi?cmd=status').status, 401);

        assert.equal(http(`/addons/kitdemo/kit/service.cgi?sid=${encodeURIComponent(SID)}`, {method: 'POST', data: 'cmd=stop',
            headers: {'Content-Type': 'application/x-www-form-urlencoded'}}).json.error.code, 'csrf');
        const stop = http(`/addons/kitdemo/kit/service.cgi?sid=${encodeURIComponent(SID)}`, {method: 'POST', data: 'cmd=stop', headers: POST});
        assert.equal(stop.status, 200, stop.body);
        assert.match(stop.json.output, /Stopping kitdemo: OK/);
        assert.deepEqual(http('/addons/kitdemo/kit/alive.cgi').json, {running: false});
        const start = http(`/addons/kitdemo/kit/service.cgi?sid=${encodeURIComponent(SID)}`, {method: 'POST', data: 'cmd=start', headers: POST});
        assert.equal(start.status, 200, start.body);
        assert.deepEqual(http('/addons/kitdemo/kit/alive.cgi').json, {running: true});
        const reload = http(`/addons/kitdemo/kit/service.cgi?sid=${encodeURIComponent(SID)}`, {method: 'POST', data: 'cmd=reload', headers: POST});
        assert.equal(reload.status, 200, reload.body);

        const logs = http(`/addons/kitdemo/kit/log.cgi?sid=${encodeURIComponent(SID)}`);
        assert.deepEqual(logs.json.sources.map((x) => [x.id, x.available]), [['main', true], ['syslog', true]]);
        const main = http(`/addons/kitdemo/kit/log.cgi?source=main&lines=50&sid=${encodeURIComponent(SID)}`);
        assert.ok(main.json.lines.some((l) => /kitdemo: started/.test(l)), main.body);
        const sys = http(`/addons/kitdemo/kit/log.cgi?source=syslog&sid=${encodeURIComponent(SID)}`);
        assert.ok(sys.json.lines.some((l) => /Starting kitdemo/.test(l)), sys.body);

        const v = http(`/addons/kitdemo/kit/version.cgi?sid=${encodeURIComponent(SID)}`);
        assert.equal(v.json.installed, VERSION);
        const uc = http('/addons/kitdemo/update_check.cgi?cmd=check_version&version=x');
        assert.equal(uc.status, 200);
        assert.ok(uc.body.length > 0 && !uc.body.includes('\n'));

        assert.equal(http(`/addons/kitdemo/api/hello.cgi?sid=${encodeURIComponent(SID)}`).json.greeting, 'Hallo von Kit Demo');
        const save = http(`/addons/kitdemo/api/hello.cgi?sid=${encodeURIComponent(SID)}`, {method: 'POST', data: 'greeting=Gr%C3%BC%C3%9F%20dich', headers: POST});
        assert.deepEqual(save.json, {ok: true, saved: true});
    });

    it('updates with exit 0, keeps the user data and runs again', () => {
        assert.equal(installAddon(), 0, sh('cat /tmp/update_script.log', {ok: false}).out);
        assert.equal(http(`/addons/kitdemo/api/hello.cgi?sid=${encodeURIComponent(SID)}`).json.greeting, 'Grüß dich');
        assert.equal(sh(`${RC} status`, {ok: false}).code, 0);
    });

    it('uninstalls without leftovers', () => {
        sh(`${RC} uninstall >/dev/null`);
        sh(`rm -f ${RC}`);
        const left = sh(`for p in ${ADDON} ${RC} /usr/local/etc/config/addons/www/kitdemo /var/log/kitdemo.log /tmp/kit-kitdemo-release.json; do [ -e "$p" ] || [ -L "$p" ] && echo "$p"; done; grep -c kitdemo /usr/local/etc/config/hm_addons.cfg; pidof kitdemo; true`, {ok: false}).out;
        assert.equal(left.trim(), '0');
        assert.equal(sh('ps | grep -v grep | grep -c "bin/kitdemo"', {ok: false}).out.trim(), '0');
    });
});
