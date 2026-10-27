// The CGI simulator for the kit's Tcl tests: a vendored addon tree in a temp directory, the
// stub tclrega.so, a stub /api/auth/v1/state, a fake curl, logger and rc.d script, and cgi()
// which runs one CGI with the environment lighttpd gives it and parses the answer.
//
// TCLSH names the interpreter (tclsh 8.6, or 8.2.3 in the tcl82 image); KIT_TCLREGA_DIR the
// directory of the stub tclrega.so built against it.

import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {plan} from '../../cli/vendor.mjs';
import {normalizeKit} from '../../cli/lib/kitconfig.mjs';

export const TCLSH = process.env.TCLSH || 'tclsh';
const REGA_DIR = process.env.KIT_TCLREGA_DIR || '/usr/local/lib/kit';

function sh(path, body) {
    writeFileSync(path, `#!/bin/sh\n${body}\n`);
    chmodSync(path, 0o755);
}

export async function setup(kitOverrides = {}, {frame} = {}) {
    const dir = mkdtempSync(join(tmpdir(), 'kit-test-'));
    const home = join(dir, 'addon');
    const fake = join(dir, 'fake');
    mkdirSync(join(home, 'etc'), {recursive: true});
    mkdirSync(fake, {recursive: true});
    mkdirSync(join(dir, 'tmp'));
    mkdirSync(join(dir, 'profiles'));
    const kit = normalizeKit({
        schema: 1,
        id: 'kitdemo',
        name: {de: 'Kit Demo „Ä“', en: 'Kit Demo'},
        addonDir: home,
        rc: join(fake, 'rc'),
        logs: [
            {id: 'main', label: {de: 'Dienst', en: 'Service'}, file: join(dir, 'kitdemo.log'), rotated: join(dir, 'kitdemo.log.0')},
            {id: 'syslog', label: {de: 'Systemprotokoll', en: 'System log'}, syslogTag: 'kitdemo'},
        ],
        update: {github: 'owner/repo'},
        tabs: ['hello', 'kit:service', 'kit:log', 'kit:about'],
        ...kitOverrides,
    });
    for (const [path, {data, mode}] of Object.entries(plan(kit, {out: home, frame, version: '0.1.0-test', commit: 'abc'}))) {
        mkdirSync(join(home, path, '..'), {recursive: true});
        writeFileSync(join(home, path), data);
        chmodSync(join(home, path), mode);
    }
    writeFileSync(join(home, 'versions'), 'VERSION_ADDON=0.4.0\nexport NODE_VERSION="22.20.0"\nVERSION_MOSQUITTO=\'2.1.2\'\n# comment\n');
    writeFileSync(join(dir, 'VERSION'), 'PRODUCT=raspmatic_ova\nVERSION=3.89.8.20260719\nPLATFORM=x86_64\n');
    writeFileSync(join(dir, 'meminfo'), 'MemTotal:        1012340 kB\nMemFree:          100000 kB\nMemAvailable:     612340 kB\n');
    writeFileSync(join(dir, 'rega-sids'), '');
    // the rc.d script: its answers come from files the test writes
    sh(join(fake, 'rc'), `D=${dir}
echo "$1" >> $D/rc-calls
case "$1" in
    status)
        [ -f $D/rc-status-sleep ] && sleep $(cat $D/rc-status-sleep)
        echo "status noise on stderr" >&2
        cat $D/rc-status-out 2>/dev/null
        exit $(cat $D/rc-status-code 2>/dev/null || echo 3) ;;
    start|stop|restart|reload)
        echo "$1 on stdout"
        echo "$1 on stderr" >&2
        exit $(cat $D/rc-action-code 2>/dev/null || echo 0) ;;
esac
exit 0`);
    sh(join(fake, 'curl'), `D=${dir}
echo "$@" >> $D/curl-calls
echo call >> $D/curl-count
[ -f $D/curl-exit ] && { echo "curl: (6) Could not resolve host" >&2; exit $(cat $D/curl-exit); }
cat $D/curl-body 2>/dev/null
printf '\\n%s' "$(cat $D/curl-code 2>/dev/null || echo 200)"`);
    sh(join(fake, 'logger'), `echo "$@" >> ${dir}/logger`);
    sh(join(fake, 'occulited'), 'exit 0');

    // the stub /state, answering by Bearer
    const stateLog = [];
    const states = {};
    const server = createServer((req, res) => {
        stateLog.push({method: req.method, url: req.url, auth: req.headers.authorization || '-'});
        const bearer = (req.headers.authorization || '').replace(/^Bearer /, '');
        const s = states[bearer];
        if (req.url !== '/api/auth/v1/state') { res.writeHead(404); res.end('{}'); return; }
        if (s && s.status) { res.writeHead(s.status, {'Content-Type': 'text/html'}); res.end(s.body || ''); return; }
        res.writeHead(200, {'Content-Type': 'application/json'});
        res.end(s ? (typeof s === 'string' ? s : JSON.stringify(s)) : '{"setup_required":false,"authenticated":false}');
    });
    await new Promise((r) => { server.listen(0, '127.0.0.1', r); });
    const port = server.address().port;

    const env = {
        PATH: `${fake}:${process.env.PATH}`,
        KIT_TEST: '1',
        KIT_VERSION_FILE: join(dir, 'VERSION'),
        KIT_OCCULITED: join(dir, 'no-occulited'),
        KIT_S00: join(dir, 'no-S00'),
        KIT_USERPROFILES: join(dir, 'profiles'),
        KIT_STATE_URL: `http://127.0.0.1:${port}/api/auth/v1/state`,
        KIT_TMP: join(dir, 'tmp'),
        KIT_MESSAGES: join(dir, 'messages'),
        KIT_MEMINFO: join(dir, 'meminfo'),
        KIT_LOGGER: join(fake, 'logger'),
        KIT_REGA_SIDS: join(dir, 'rega-sids'),
        KIT_REGA_LOG: join(dir, 'rega-log'),
        LD_LIBRARY_PATH: [REGA_DIR, process.env.LD_LIBRARY_PATH].filter(Boolean).join(':'),
        TMPDIR: join(dir, 'tmp'),
    };

    const t = {
        dir, home, fake, kit, env, port, states, stateLog,
        file: (name) => join(dir, name),
        write: (name, data) => writeFileSync(join(dir, name), data),
        read: (name) => { try { return readFileSync(join(dir, name), 'utf8'); } catch { return ''; } },
        rega: (lines) => writeFileSync(join(dir, 'rega-sids'), lines.map(([sid, answer]) => `${sid} ${answer}\n`).join('')),
        lite: (on = true) => { t.env.KIT_OCCULITED = on ? join(fake, 'occulited') : join(dir, 'no-occulited'); },
        cgi: (path, opts) => runCgi(join(home, path), {...opts, env: {...t.env, ...(opts && opts.env)}}),
        tcl: (script) => runTcl(script, t.env, home),
        async close() {
            await new Promise((r) => { server.close(r); });
            rmSync(dir, {recursive: true, force: true});
        },
    };
    return t;
}

// one CGI request: method, query (string or object), headers (name -> value, as the client
// sent them), body (string or object as a form)
export function runCgi(script, {method = 'GET', query = '', headers = {}, body, env = {}} = {}) {
    const q = typeof query === 'string' ? query : new URLSearchParams(query).toString();
    let data = Buffer.alloc(0);
    if (body !== undefined) {
        data = Buffer.from(typeof body === 'string' ? body : new URLSearchParams(body).toString());
    }
    const cgiEnv = {
        ...env,
        GATEWAY_INTERFACE: 'CGI/1.1',
        REQUEST_METHOD: method,
        QUERY_STRING: q,
        SCRIPT_FILENAME: script,
        REMOTE_ADDR: '192.0.2.10',
        HTTP_HOST: 'ccu.example:8080',
        SERVER_PORT: '8080',
    };
    if (body !== undefined) {
        cgiEnv.CONTENT_LENGTH = String(data.length);
        if (!Object.keys(headers).some((h) => h.toLowerCase() === 'content-type')) {
            cgiEnv.CONTENT_TYPE = 'application/x-www-form-urlencoded';
        }
    }
    for (const [k, v] of Object.entries(headers)) {
        const name = k.toUpperCase().replace(/-/g, '_');
        if (name === 'CONTENT_TYPE') { cgiEnv.CONTENT_TYPE = v; } else { cgiEnv[`HTTP_${name}`] = v; }
    }
    return new Promise((resolve, reject) => {
        const child = spawn(TCLSH, [script], {env: cgiEnv, cwd: join(script, '..')});
        const out = [];
        let err = '';
        child.stdout.on('data', (d) => out.push(d));
        child.stderr.on('data', (d) => { err += d; });
        child.on('error', reject);
        child.on('close', (code) => {
            const raw = Buffer.concat(out);
            const sep = raw.indexOf('\r\n\r\n');
            if (sep < 0) { reject(new Error(`no CGI header (exit ${code}): ${raw.toString().slice(0, 500)} ${err}`)); return; }
            const head = raw.subarray(0, sep).toString('latin1').split('\r\n');
            const hdrs = {};
            for (const l of head) {
                const i = l.indexOf(':');
                hdrs[l.slice(0, i).toLowerCase()] = l.slice(i + 1).trim();
            }
            const status = hdrs.status ? Number(hdrs.status.split(' ')[0]) : 200;
            const bodyBuf = raw.subarray(sep + 4);
            const text = bodyBuf.toString('utf8');
            let json;
            if ((hdrs['content-type'] || '').startsWith('application/json')) {
                try { json = JSON.parse(text); } catch (e) { reject(new Error(`invalid JSON body: ${e.message}: ${text.slice(0, 500)}`)); return; }
            }
            resolve({status, headers: hdrs, text, json, raw: bodyBuf, stderr: err, exit: code});
        });
        child.stdin.on('error', () => {});
        child.stdin.end(data);
    });
}

// a Tcl snippet with the kit's library loaded; returns stdout
export function runTcl(script, env, home) {
    const file = join(env.KIT_TMP, `snippet-${process.hrtime.bigint()}.tcl`);
    writeFileSync(file, `set KIT_HOME ${home}\nsource $KIT_HOME/lib/kit/cgi.tcl\n${script}\n`);
    return new Promise((resolve, reject) => {
        const child = spawn(TCLSH, [file], {env});
        let out = '';
        let err = '';
        child.stdout.on('data', (d) => { out += d; });
        child.stderr.on('data', (d) => { err += d; });
        child.on('close', (code) => { if (code === 0) { resolve(out); } else { reject(new Error(`tclsh exit ${code}: ${err}`)); } });
        child.stdin.end();
    });
}

// sessions for the tests: ten-character ReGa ids and their answers
export const SID = {admin: '@Adm1nSess1@', user: '@UserSess01@', guest: '@GuestSes01@', shim: '@LiteAlias1@'};
export function regaSessions(t) {
    t.rega([
        ['Adm1nSess1', '1001;8;Admin;;;'],
        ['UserSess01', '1002;2;anna;Anna;Muster;'],
        ['GuestSes01', '1003;1;gast;;;'],
        ['LiteAlias1', 'anna'],
    ]);
}
export const CSRF = {'X-CCU-Addon-Request': '1', Origin: 'http://ccu.example:8080', 'Sec-Fetch-Site': 'same-origin'};
