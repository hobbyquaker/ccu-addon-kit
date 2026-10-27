// The stub server for the frame's Playwright tests (ccu-addon-kit, MIT): kitdemo's built frame
// and the kit's **real Tcl CGIs** (run by tclsh through the CGI simulator of test/tcl), a stub
// ReGa session store, a stub openccu-lite /state, a stateful fake rc.d script, the WebUI's
// Session.renew, and a page that frames the settings page the way openccu-lite's shell does.
//
// Runs where tclsh and the stub tclrega.so are (the ccu-addon-kit-tcl86 image):
//   node test/frame/stub-server.mjs [port]           (default 8080; FRAME=<built web/dist>)
//
// Control (for the tests): GET /__stub/state, POST /__stub/<action> with a JSON body:
//   reset, expire (the ReGa session goes away), revive, lite {on}, lang {value: 0|1|2},
//   rc {status: "running"|"stopped"|"failed"|"garbage", actionExit: n}

import {createServer} from 'node:http';
import {copyFileSync, existsSync, mkdirSync, readFileSync, statSync} from 'node:fs';
import {extname, join, normalize, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {runCgi, setup} from '../tcl/harness.mjs';

const KIT_ROOT = resolve(fileURLToPath(import.meta.url), '../../..');
const PORT = Number(process.argv[2] || process.env.PORT || 8080);
const FRAME = process.env.FRAME || join(KIT_ROOT, 'example/kitdemo/web/dist');
const KITDEMO = join(KIT_ROOT, 'example/kitdemo');
export const SID = '@PwAdmin001@';
const LITE_SID = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.slice(0, 26);

const kitJson = JSON.parse(readFileSync(join(KITDEMO, 'kit.json'), 'utf8'));
const t = await setup({name: kitJson.name, icon: kitJson.icon, tabs: kitJson.tabs, app: kitJson.app, links: kitJson.links, service: kitJson.service},
    {frame: FRAME});

// kitdemo's own files: the tab CGI, the stand-in app, the icon, the greeting
mkdirSync(join(t.home, 'www/api'), {recursive: true});
mkdirSync(join(t.home, 'www/app'), {recursive: true});
copyFileSync(join(KITDEMO, 'addon_files/kitdemo/www/api/hello.cgi'), join(t.home, 'www/api/hello.cgi'));
copyFileSync(join(KITDEMO, 'addon_files/kitdemo/www/app/index.html'), join(t.home, 'www/app/index.html'));
copyFileSync(join(KITDEMO, 'addon_files/kitdemo/www/icon.svg'), join(t.home, 'www/icon.svg'));
t.env.KIT_HOME = t.home;

const D = t.dir;
// a stateful rc.d script: status follows start/stop, the test can set it
t.write('rc-state', 'running');
const rc = `#!/bin/sh
D=${D}
echo "$1" >> $D/rc-calls
state=$(cat $D/rc-state)
case "$1" in
    status)
        case "$state" in
            running) echo '{"schema":1,"running":true,"state":"running","pid":4242,"rss_kb":49459,"since":'$(( $(date +%s) - 11520 ))',"version":"0.4.0"}'; exit 0 ;;
            failed) echo '{"schema":1,"running":false,"state":"failed","lastError":"config file missing"}'; exit 1 ;;
            garbage) echo 'kitdemo is fine, probably'; exit 4 ;;
            *) echo '{"schema":1,"running":false,"state":"stopped"}'; exit 3 ;;
        esac ;;
    start) sleep 1; echo "Starting kitdemo: OK"; echo running > $D/rc-state ;;
    stop) sleep 1; echo "Stopping kitdemo: OK"; echo stopped > $D/rc-state ;;
    restart) sleep 1; echo "Restarting kitdemo: OK"; echo running > $D/rc-state ;;
    reload) echo "Reloading kitdemo: OK" ;;
esac
exit $(cat $D/rc-action-exit 2>/dev/null || echo 0)
`;
t.write('fake/rc', rc);
t.write('kitdemo.log', Array.from({length: 300}, (_, i) => `2026-10-27 10:${String(Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')} kitdemo: heartbeat ${i}`).join('\n') + '\n');

t.write('messages', 'Oct 27 10:00:01 ccu user.info kitdemo[4242]: started\nOct 27 10:00:02 ccu user.info other[1]: noise\n');
const renews = [];
function reset() {
    t.rega([[SID.slice(1, -1), '1001;8;Admin;;;']]);
    t.write('rc-state', 'running');
    t.write('rc-action-exit', '0');
    t.lite(false);
    for (const k of Object.keys(t.states)) delete t.states[k];
    t.states[LITE_SID] = {authenticated: true, sid: LITE_SID, user: 'admin', level: 'administer', role: 'admin'};
    t.write('profiles/Admin.lang', '');
    renews.length = 0;
    t.write('addon/etc/greeting.txt', 'Hallo Welt');
}
reset();
let lite = false;

const TYPES = {'.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml'};

function send(res, status, body, type = 'text/plain; charset=utf-8', extra = {}) {
    res.writeHead(status, {'Content-Type': type, 'Cache-Control': 'no-store', ...extra});
    res.end(body);
}

async function body(req) {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    return Buffer.concat(chunks).toString('utf8');
}

const PAGES = {
    '/': '<!doctype html><title>WebUI</title><h1 data-testid="webui">WebUI start page (stub)</h1>',
    '/lite-shell.html': `<!doctype html><title>shell</title>
<body style="margin:0"><iframe id="f" style="width:100%;height:700px;border:0" src="/addons/kitdemo/settings.cgi?theme=dark&lang=en"></iframe>
<script>
window.shell = (theme, lang) => document.getElementById('f').contentWindow.postMessage({type: 'openccu-lite:theme', theme, lang}, location.origin);
</script></body>`,
    '/system/log': '<!doctype html><title>log</title><h1 data-testid="system-log">system log (stub)</h1>',
};

const server = createServer(async (req, res) => {
    try {
        const url = new URL(req.url, 'http://stub');
        const path = url.pathname;
        if (path.startsWith('/__stub/')) {
            const action = path.slice(8);
            const arg = req.method === 'POST' ? JSON.parse((await body(req)) || '{}') : {};
            if (action === 'reset') { reset(); lite = false; }
            if (action === 'expire') t.rega([]);
            if (action === 'revive') t.rega([[SID.slice(1, -1), '1001;8;Admin;;;']]);
            if (action === 'lite') { lite = arg.on !== false; t.lite(lite); }
            if (action === 'lang') t.write('profiles/Admin.lang', String(arg.value));
            if (action === 'rc') {
                if (arg.status) t.write('rc-state', arg.status);
                if (arg.actionExit !== undefined) t.write('rc-action-exit', String(arg.actionExit));
            }
            send(res, 200, JSON.stringify({ok: true, lite, renews, rcCalls: t.read('rc-calls').trim().split('\n').filter(Boolean),
                greeting: t.read('addon/etc/greeting.txt')}), 'application/json');
            return;
        }
        if (path === '/api/homematic.cgi') {
            renews.push(JSON.parse((await body(req)) || 'null'));
            send(res, 200, '{"version":"1.1","result":true,"error":null}', 'application/json');
            return;
        }
        if (PAGES[path] !== undefined) { send(res, 200, PAGES[path], 'text/html; charset=utf-8'); return; }
        if (!path.startsWith('/addons/kitdemo/')) { send(res, 404, 'not found'); return; }
        const rel = normalize(path.slice('/addons/kitdemo/'.length));
        if (rel.startsWith('..')) { send(res, 400, 'bad path'); return; }
        const file = join(t.home, 'www', rel.endsWith('/') || rel === '.' ? join(rel, 'index.html') : rel);
        if (file.endsWith('.cgi')) {
            if (!existsSync(file)) { send(res, 404, 'not found'); return; }
            const headers = {};
            for (const [k, v] of Object.entries(req.headers)) {
                if (['connection', 'content-length', 'x-occulite-session'].includes(k)) continue;
                headers[k] = Array.isArray(v) ? v.join(', ') : v;
            }
            // openccu-lite's lighttpd puts the session id on every request under /addons/
            if (lite) headers['X-Occulite-Session'] = LITE_SID;
            const data = req.method === 'POST' ? await body(req) : undefined;
            const r = await runCgi(file, {method: req.method, query: url.search.slice(1), headers, body: data, env: t.env});
            const out = {...r.headers};
            delete out.status;
            res.writeHead(r.status, out);
            res.end(r.raw);
            return;
        }
        if (!existsSync(file) || !statSync(file).isFile()) { send(res, 404, 'not found'); return; }
        send(res, 200, readFileSync(file), TYPES[extname(file)] || 'application/octet-stream');
    } catch (e) {
        send(res, 500, String(e && e.stack || e));
    }
});

server.listen(PORT, '0.0.0.0', () => console.log(`stub server on :${PORT} (addon tree ${t.home})`));
const stop = () => { server.close(); t.close().finally(() => process.exit(0)); };
process.on('SIGTERM', stop);
process.on('SIGINT', stop);
