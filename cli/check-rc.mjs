// ccu-addon-kit check-rc: the conformance test of the service convention v1
// (docs/service-convention.md, steps C1-C12). It drives a real rc.d script through a command
// prefix, so the same test runs against a container and against a box:
//
//   ccu-addon-kit check-rc --exec "docker exec kitdemo-e2e" --rc /usr/local/etc/config/rc.d/kitdemo
//   ccu-addon-kit check-rc --exec "ssh root@ccu" --rc /usr/local/etc/config/rc.d/kitdemo [--legacy] [--no-reload]
//                          [--log /var/log/kitdemo.log] [--json]
//
// The box needs nothing but sh. The prefix is split on spaces (no shell quoting); without
// --exec the commands run here through sh.

import {spawn} from 'node:child_process';

function shq(s) { return `'${String(s).replace(/'/g, `'\\''`)}'`; }

export function makeRunner(exec) {
    const prefix = exec ? exec.trim().split(/\s+/) : [];
    return function run(script, {timeoutMs = 180000} = {}) {
        return new Promise((resolve) => {
            const argv = [...prefix, 'sh', '-c', prefix.length && /(^|\/)ssh$/.test(prefix[0]) ? shq(script) : script];
            const started = process.hrtime.bigint();
            const child = spawn(argv[0], argv.slice(1), {stdio: ['ignore', 'pipe', 'pipe']});
            let stdout = '';
            let stderr = '';
            let timedOut = false;
            child.stdout.on('data', (d) => { stdout += d; });
            child.stderr.on('data', (d) => { stderr += d; });
            const timer = setTimeout(() => { timedOut = true; child.kill('SIGKILL'); }, timeoutMs);
            child.on('close', (code) => {
                clearTimeout(timer);
                resolve({code: timedOut ? null : code, stdout, stderr, timedOut, ms: Number(process.hrtime.bigint() - started) / 1e6});
            });
            child.on('error', (err) => {
                clearTimeout(timer);
                resolve({code: null, stdout, stderr: String(err), timedOut: false, ms: 0});
            });
        });
    };
}

function parseStatus(stdout) {
    const lines = stdout.split('\n');
    if (lines[lines.length - 1] === '') { lines.pop(); }
    let json = null;
    let error = null;
    if (lines.length !== 1) {
        error = `stdout has ${lines.length} lines, expected exactly one`;
    } else {
        try { json = JSON.parse(lines[0]); } catch (e) { error = `not valid JSON: ${e.message}`; }
        if (json !== null && (typeof json !== 'object' || Array.isArray(json))) { error = 'not a JSON object'; json = null; }
    }
    return {json, error, lines};
}

const sleep = (ms) => new Promise((r) => { setTimeout(r, ms); });

export async function checkRc({exec, rc, legacy = false, noReload = false, log = null, run = makeRunner(exec), statusRetryMs = 30000}) {
    const results = [];
    const rec = (id, status, text) => { results.push({id, status, text}); };
    const pass = (id, text) => rec(id, 'pass', text);
    const fail = (id, text) => rec(id, 'fail', text);
    const warn = (id, text) => rec(id, 'warn', text);
    // legacy codes are warnings in --legacy mode, failures otherwise
    const legacyOr = (id, text) => (legacy ? warn(id, `${text} (legacy)`) : fail(id, text));
    const id = rc.split('/').pop();

    // the prefix's own cost (ssh, docker exec), subtracted from the status timings
    const base = [];
    for (let i = 0; i < 3; i++) { base.push((await run('true')).ms); }
    const overhead = Math.min(...base);

    const statusCalls = [];
    async function status() {
        const r = await run(`${shq(rc)} status`, {timeoutMs: 10000});
        const net = Math.max(0, r.ms - overhead);
        const p = parseStatus(r.stdout);
        statusCalls.push({ms: net, lines: p.lines.length, timedOut: r.timedOut || net >= 5000});
        return {...r, ...p, net};
    }
    function checkJson(s, step) {
        if (s.error) { fail(step, `status: ${s.error}: ${JSON.stringify(s.stdout.slice(0, 300))}`); return false; }
        if (s.json.schema !== 1) { fail(step, `status: "schema" is ${JSON.stringify(s.json.schema)}, expected 1`); return false; }
        if (typeof s.json.running !== 'boolean') { fail(step, 'status: "running" is not a boolean'); return false; }
        return true;
    }
    async function pidAlive(pid) { return (await run(`kill -0 ${Number(pid)} 2>/dev/null`)).code === 0; }
    async function children(pid) {
        const r = await run(`for f in /proc/[0-9]*/stat; do sed -e 's/^\\([0-9]*\\) (.*) [A-Za-z] \\([0-9]*\\) .*$/\\1 \\2/' "$f" 2>/dev/null; done | awk -v p=${Number(pid)} '$2==p {print $1}'`);
        return r.stdout.split(/\s+/).filter(Boolean).map(Number);
    }

    // C1
    await run(`${shq(rc)} stop >/dev/null 2>&1`, {timeoutMs: 90000});
    let s = await status();
    if (checkJson(s, 'C1')) {
        if (s.json.running !== false) { fail('C1', 'running:true after stop'); } else if (s.code === 3) { pass('C1', 'stopped: exit 3, valid JSON'); } else if (s.code === 1) {
            legacyOr('C1', 'stopped: exit 1, expected 3');
        } else { fail('C1', `stopped: exit ${s.code}, expected 3`); }
    }

    // C2
    let r = await run(`${shq(rc)} start`, {timeoutMs: 60000});
    if (r.timedOut) { fail('C2', 'start did not return within 60 s (daemon kept in the foreground?)'); } else if (r.code === 0) {
        pass('C2', `start: exit 0 in ${(r.ms / 1000).toFixed(1)} s`);
    } else { fail('C2', `start: exit ${r.code}: ${r.stdout.trim().slice(0, 200)}`); }

    // C3
    let pid = null;
    const until = Date.now() + statusRetryMs;
    do {
        s = await status();
        if (s.code === 0 && s.json && s.json.running === true) { break; }
        await sleep(1000);
    } while (Date.now() < until);
    if (checkJson(s, 'C3')) {
        const problems = [];
        if (s.code !== 0) { problems.push(`exit ${s.code}`); }
        if (s.json.running !== true) { problems.push('running is not true'); }
        if (!Number.isInteger(s.json.pid) || s.json.pid <= 0) { problems.push('no integer pid'); } else if (!(await pidAlive(s.json.pid))) { problems.push(`pid ${s.json.pid} does not exist`); }
        if (!Number.isInteger(s.json.rss_kb) || s.json.rss_kb <= 0) { problems.push('rss_kb is not a positive integer'); }
        if (s.json.since !== undefined && s.json.since !== null) {
            const now = Number((await run('date +%s')).stdout.trim());
            if (!Number.isInteger(s.json.since) || Math.abs(now - s.json.since) > 120) { problems.push(`since ${s.json.since} is not within the last 120 s (box time ${now})`); }
        }
        if (problems.length) { fail('C3', `running: ${problems.join('; ')}`); } else { pass('C3', `running: pid ${s.json.pid}, rss_kb ${s.json.rss_kb}`); pid = s.json.pid; }
    }

    // C4
    r = await run(`${shq(rc)} start`, {timeoutMs: 60000});
    s = await status();
    const samePid = pid !== null && s.json && s.json.pid === pid;
    if (r.code === 0 && samePid) { pass('C4', 'start when running: exit 0, same pid'); } else if (r.code === 1 && samePid) {
        legacyOr('C4', 'start when running: exit 1, expected 0');
    } else { fail('C4', `start when running: exit ${r.code}, pid ${s.json && s.json.pid} (was ${pid})`); }

    // C5
    r = await run(`${shq(rc)} reload`, {timeoutMs: 15000});
    s = await status();
    if (r.code === 0 && pid !== null && s.json && s.json.pid === pid && s.json.running) { pass('C5', 'reload: exit 0, same pid'); } else if (r.code === 3 && noReload) {
        pass('C5', 'reload: exit 3 (not supported, --no-reload)');
    } else if (r.code === 3) { fail('C5', 'reload: exit 3 (not supported); pass --no-reload and leave reload out of kit.json'); } else {
        fail('C5', `reload: exit ${r.code}, pid ${s.json && s.json.pid} (was ${pid})`);
    }

    // C6
    r = await run(`${shq(rc)} restart`, {timeoutMs: 120000});
    let newPid = null;
    if (r.timedOut) { fail('C6', 'restart did not return within 120 s'); } else {
        const until6 = Date.now() + statusRetryMs;
        do {
            s = await status();
            if (s.json && s.json.running === true) { break; }
            await sleep(1000);
        } while (Date.now() < until6);
        newPid = s.json ? s.json.pid : null;
        if (r.code === 0 && s.json && s.json.running === true && newPid !== pid) { pass('C6', `restart: exit 0, new pid ${newPid}`); } else {
            fail('C6', `restart: exit ${r.code}, running ${s.json && s.json.running}, pid ${newPid} (was ${pid})`);
        }
    }

    // C7
    const kids = newPid ? await children(newPid) : [];
    r = await run(`${shq(rc)} stop`, {timeoutMs: 60000});
    s = await status();
    const left = [];
    for (const p of [newPid, ...kids].filter(Boolean)) { if (await pidAlive(p)) { left.push(p); } }
    if (r.code !== 0 || r.timedOut) { fail('C7', `stop: exit ${r.code}${r.timedOut ? ' (timeout)' : ''}`); } else if (s.code !== 3 && !(legacy && s.code === 1)) {
        fail('C7', `status after stop: exit ${s.code}, expected 3`);
    } else if (left.length) { fail('C7', `still alive after stop: ${left.join(', ')}`); } else {
        if (s.code === 1) { legacyOr('C7', 'stopped: status exit 1'); } else { pass('C7', `stop: exit 0, status 3, pid ${newPid} and ${kids.length} children gone`); }
    }

    // C8
    r = await run(`${shq(rc)} stop`, {timeoutMs: 60000});
    if (r.code === 0) { pass('C8', 'stop when stopped: exit 0'); } else if (r.code === 1) { legacyOr('C8', 'stop when stopped: exit 1, expected 0'); } else {
        fail('C8', `stop when stopped: exit ${r.code}`);
    }

    // C9
    const quiet = [];
    for (const cmd of ['info.de', 'info.en', 'nonsense']) {
        const q = await run(`${shq(rc)} ${cmd}`, {timeoutMs: 10000});
        if (q.code !== 0 || q.stdout.trim() !== '') { quiet.push(`${cmd}: exit ${q.code}, stdout ${JSON.stringify(q.stdout.slice(0, 80))}`); }
    }
    if (quiet.length) { fail('C9', quiet.join('; ')); } else { pass('C9', 'info.de, info.en, nonsense: exit 0, no output'); }

    // C10
    r = await run(`${shq(rc)} info`, {timeoutMs: 10000});
    const name = /^Name: (.+)$/m.exec(r.stdout);
    const version = /^Version: (.*)$/m.exec(r.stdout);
    s = await status();
    if (!name) { fail('C10', 'info has no "Name:" line'); } else if (!version) { fail('C10', 'info has no "Version:" line'); } else if (s.json && s.json.version !== undefined && s.json.version !== version[1].trim()) {
        fail('C10', `info Version "${version[1].trim()}" differs from status version "${s.json.version}"`);
    } else { pass('C10', `info: Name "${name[1].trim()}", Version "${version[1].trim()}"`); }

    // C12 (before C11, whose verdict covers every status call)
    if (log) {
        const line = `kitcheck ${id}[1]: error: injected "quote" back\\slash\ttab`;
        await run(`printf '%s\\n' ${shq(line)} >> ${shq(log)}`);
        s = await status();
        if (s.error) { fail('C12', `status with an injected lastError: ${s.error}`); } else if (s.json && typeof s.json.lastError === 'string' && s.json.lastError.includes('injected')) {
            pass('C12', 'status with ", \\ and a tab in lastError: valid JSON');
        } else { warn('C12', 'status is valid JSON, but its lastError does not show the injected line'); }
    } else {
        rec('C12', 'skip', 'no --log given');
    }

    // C11
    const slow = statusCalls.filter((c) => c.ms > 2000);
    const tooSlow = statusCalls.filter((c) => c.timedOut);
    const multi = statusCalls.filter((c) => c.lines !== 1);
    const worst = Math.max(...statusCalls.map((c) => c.ms));
    if (tooSlow.length || multi.length) {
        fail('C11', `${tooSlow.length} status calls at or above 5 s, ${multi.length} not exactly one line`);
    } else if (slow.length) { warn('C11', `${slow.length} of ${statusCalls.length} status calls above 2 s (slowest ${Math.round(worst)} ms)`); } else {
        pass('C11', `${statusCalls.length} status calls, one line each, slowest ${Math.round(worst)} ms`);
    }

    results.sort((a, b) => Number(a.id.slice(1)) - Number(b.id.slice(1)));
    return {ok: !results.some((x) => x.status === 'fail'), results, overheadMs: Math.round(overhead)};
}

export function printTable(report, out = console.log) {
    const mark = {pass: 'ok  ', fail: 'FAIL', warn: 'warn', skip: 'skip'};
    for (const r of report.results) { out(`${mark[r.status]}  ${r.id.padEnd(4)} ${r.text}`); }
    out(report.ok ? 'check-rc: conforms to the service convention v1' : 'check-rc: FAILED');
}
