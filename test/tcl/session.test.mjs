// session.tcl through kit/session.cgi and the permission and CSRF rules of kit_require.
import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {CSRF, SID, regaSessions, setup} from './harness.mjs';

const LIVE = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const OLD = 'abcdefgh12';

describe('kit/session.cgi on a CCU3/OpenCCU', () => {
    let t;
    before(async () => { t = await setup(); regaSessions(t); t.write('profiles/Admin.lang', '2\n'); });
    after(() => t.close());

    it('answers valid:false as a 200 without a sid', async () => {
        const r = await t.cgi('www/kit/session.cgi');
        assert.equal(r.status, 200);
        assert.equal(r.json.ok, true);
        assert.deepEqual(r.json.session, {valid: false});
        assert.equal(r.json.platform.kind, 'openccu');
        assert.equal(r.json.platform.firmware, '3.89.8.20260719');
        assert.match(r.json.platform.tcl, /^8\.[0-9]/);
        assert.deepEqual(r.json.kit, {version: '0.1.0-test', requestHeader: 'X-CCU-Addon-Request'});
        assert.equal(r.headers['cache-control'], 'no-store');
    });

    it('reads the level from the UPL and the language from the user profile', async () => {
        const r = await t.cgi('www/kit/session.cgi', {query: {sid: SID.admin}});
        assert.deepEqual(r.json.session, {valid: true, level: 'admin', user: 'Admin', via: 'sid', lang: 'en'});
        assert.deepEqual(r.json.permissions, {read: true, service: true, log: true, write: true});
        for (const [sid, level, user] of [[SID.user, 'user', 'anna'], [SID.guest, 'guest', 'gast'], [SID.shim, 'unknown', 'anna']]) {
            const s = await t.cgi('www/kit/session.cgi', {query: {sid}});
            assert.equal(s.json.session.level, level, sid);
            assert.equal(s.json.session.user, user, sid);
            assert.equal(s.json.session.lang, undefined);
        }
    });

    it('maps the profile language index 0/1/2 to auto/de/en', async () => {
        for (const [v, lang] of [['0', 'auto'], ['1', 'de'], ['2', 'en']]) {
            t.write('profiles/anna.lang', v);
            const s = await t.cgi('www/kit/session.cgi', {query: {sid: SID.user}});
            assert.equal(s.json.session.lang, lang);
        }
    });

    it('sends nothing but the ten checked characters to ReGa', async () => {
        t.write('rega-log', '');
        for (const sid of ["@abc'));x@", 'Adm1nSess1', '@Adm1nSess1', '@Adm1nSess12@', '@Adm1n Ses1@', '@Adm1nSess1@@', "@x'+y'%0a@"]) {
            const r = await t.cgi('www/kit/session.cgi', {query: {sid}});
            assert.equal(r.json.session.valid, false, sid);
        }
        assert.equal(t.read('rega-log'), '');
        await t.cgi('www/kit/session.cgi', {query: 'sid=%40Adm1nSess1%40'});
        assert.equal(t.read('rega-log'), "Write(system.GetSessionVarStr('Adm1nSess1'));\n");
    });

    it('treats an expired session (empty answer) as invalid', async () => {
        const r = await t.cgi('www/kit/session.cgi', {query: {sid: '@Expired001@'}});
        assert.equal(r.json.session.valid, false);
    });

    it('ignores the openccu-lite header on a CCU and never asks /state', async () => {
        t.states[LIVE] = {authenticated: true, sid: LIVE, user: 'anna', level: 'administer'};
        t.stateLog.length = 0;
        const r = await t.cgi('www/kit/session.cgi', {headers: {'X-Occulite-Session': LIVE}});
        assert.equal(r.json.session.valid, false);
        assert.equal(t.stateLog.length, 0);
    });

    it('says ccu3 where S00InstallAddon exists', async () => {
        t.write('S00', '');
        const r = await t.cgi('www/kit/session.cgi', {env: {KIT_S00: t.file('S00')}});
        assert.equal(r.json.platform.kind, 'ccu3');
    });
});

describe('kit/session.cgi on openccu-lite', () => {
    let t;
    before(async () => {
        t = await setup();
        regaSessions(t);
        t.lite();
        Object.assign(t.states, {
            [LIVE]: {setup_required: false, authenticated: true, user: 'anna', level: 'administer', role: 'admin', sid: LIVE, legacy_sid: OLD},
            USERSESSIONUSERSESSIONUS22: {authenticated: true, user: 'bob', level: 'operate', role: 'user', sid: 'USERSESSIONUSERSESSIONUS22'},
            READSESSIONREADSESSIONRE22: {authenticated: true, user: 'carl', level: 'read', role: 'user', sid: 'READSESSIONREADSESSIONRE22'},
            ROLEONLYROLEONLYROLEONLY22: {authenticated: true, user: 'dora', role: 'admin', sid: 'ROLEONLYROLEONLYROLEONLY22'},
            NOSIDNOSIDNOSIDNOSIDNOSI22: {authenticated: true, user: 'anna', level: 'administer'},
            OTHERSESSIONOTHERSESSION22: {authenticated: true, user: 'anna', level: 'administer', sid: LIVE},
            BROKENBROKENBROKENBROKEN22: {status: 500, body: `authenticated: true sid: BROKENBROKENBROKENBROKEN22`},
            FAKEROLEFAKEROLEFAKEROLE22: '{"authenticated":true,"user":"eve\\",\\"level\\":\\"administer","level":"read","sid":"FAKEROLEFAKEROLEFAKEROLE22"}',
            [OLD]: {authenticated: true, user: 'anna', level: 'configure', sid: OLD},
        });
    });
    after(() => t.close());

    it('detects openccu-lite by occulited or a LITE= line in /VERSION', async () => {
        let r = await t.cgi('www/kit/session.cgi');
        assert.equal(r.json.platform.kind, 'openccu-lite');
        t.write('VERSION-lite', 'VERSION=1.0.0-rc.3\nLITE=1.0.0-rc.3\n');
        r = await t.cgi('www/kit/session.cgi', {env: {KIT_OCCULITED: t.file('none'), KIT_VERSION_FILE: t.file('VERSION-lite')}});
        assert.equal(r.json.platform.kind, 'openccu-lite');
    });

    it('confirms the header at /state and maps level', async () => {
        const cases = [[LIVE, 'admin', 'anna'], ['USERSESSIONUSERSESSIONUS22', 'user', 'bob'], ['READSESSIONREADSESSIONRE22', 'guest', 'carl'],
            ['ROLEONLYROLEONLYROLEONLY22', 'admin', 'dora'], ['FAKEROLEFAKEROLEFAKEROLE22', 'guest', '']];
        for (const [id, level, user] of cases) {
            t.stateLog.length = 0;
            const r = await t.cgi('www/kit/session.cgi', {headers: {'X-Occulite-Session': id}});
            assert.equal(r.json.session.valid, true, id);
            assert.equal(r.json.session.level, level, id);
            assert.equal(r.json.session.via, 'header');
            assert.equal(r.json.session.user ?? '', user, id);
            assert.deepEqual(t.stateLog, [{method: 'GET', url: '/api/auth/v1/state', auth: `Bearer ${id}`}]);
        }
    });

    it('refuses tokens, foreign answers, errors and odd shapes', async () => {
        for (const id of ['NOSIDNOSIDNOSIDNOSIDNOSI22', 'OTHERSESSIONOTHERSESSION22', 'BROKENBROKENBROKENBROKEN22', 'UNKNOWNUNKNOWNUNKNOWNUNK22']) {
            const r = await t.cgi('www/kit/session.cgi', {headers: {'X-Occulite-Session': id}});
            assert.equal(r.json.session.valid, false, id);
        }
        t.stateLog.length = 0;
        for (const id of ['olt_0123456789abcdef0123456789abcdef', 'abc', `${LIVE}\nX`, 'abcdefghijklmnopqrstuvwxyz']) {
            const r = await t.cgi('www/kit/session.cgi', {headers: {'X-Occulite-Session': id}});
            assert.equal(r.json.session.valid, false, id);
        }
        assert.equal(t.stateLog.length, 0, 'nothing of another shape is sent to /state');
    });

    it('falls back to ?sid=: the session at /state, then the shim with level unknown', async () => {
        let r = await t.cgi('www/kit/session.cgi', {query: {sid: `@${OLD}@`}, headers: {'X-Occulite-Session': 'UNKNOWNUNKNOWNUNKNOWNUNK22'}});
        assert.deepEqual(r.json.session, {valid: true, level: 'user', user: 'anna', via: 'sid'});
        r = await t.cgi('www/kit/session.cgi', {query: {sid: SID.shim}});
        assert.deepEqual(r.json.session, {valid: true, level: 'unknown', user: 'anna', via: 'sid'});
    });

    it('lets a confirmed header decide alone', async () => {
        const r = await t.cgi('www/kit/session.cgi', {query: {sid: SID.admin}, headers: {'X-Occulite-Session': 'READSESSIONREADSESSIONRE22'}});
        assert.equal(r.json.session.level, 'guest');
        assert.equal(r.json.session.via, 'header');
    });

    it('answers a syslog source with the system log link', async () => {
        const r = await t.cgi('www/kit/log.cgi', {headers: {'X-Occulite-Session': LIVE}});
        const sys = r.json.sources.find((s) => s.id === 'syslog');
        assert.equal(sys.available, false);
        assert.equal(sys.link, '/system/log?unit=addon-kitdemo');
        const l = await t.cgi('www/kit/log.cgi', {query: {source: 'syslog'}, headers: {'X-Occulite-Session': LIVE}});
        assert.deepEqual(l.json, {ok: true, source: 'syslog', available: false, link: '/system/log?unit=addon-kitdemo', lines: []});
    });
});

describe('permissions', () => {
    let t;
    let lv;
    before(async () => {
        t = await setup();
        regaSessions(t);
        lv = await setup({permissions: 'levels'});
        regaSessions(lv);
        for (const x of [t, lv]) { x.write('rc-status-out', '{"schema":1,"running":false}'); }
    });
    after(async () => { await t.close(); await lv.close(); });

    it('lets any valid session do everything by default', async () => {
        for (const sid of [SID.guest, SID.shim, SID.user]) {
            const r = await t.cgi('www/kit/service.cgi', {method: 'POST', query: {sid}, headers: CSRF, body: {cmd: 'start'}});
            assert.equal(r.status, 200, sid);
            const s = await t.cgi('www/kit/session.cgi', {query: {sid}});
            assert.deepEqual(s.json.permissions, {read: true, service: true, log: true, write: true});
        }
    });

    it('refuses everything without a session', async () => {
        const r = await t.cgi('www/kit/service.cgi', {query: {cmd: 'status'}});
        assert.equal(r.status, 401);
        assert.equal(r.json.error.code, 'session-invalid');
        const p = await t.cgi('www/kit/service.cgi', {method: 'POST', headers: CSRF, body: {cmd: 'start'}});
        assert.equal(p.status, 401);
    });

    it('applies the levels when the addon opts in', async () => {
        const read = async (sid) => (await lv.cgi('www/kit/service.cgi', {query: {sid, cmd: 'status'}})).status;
        const act = async (sid) => lv.cgi('www/kit/service.cgi', {method: 'POST', query: {sid}, headers: CSRF, body: {cmd: 'stop'}});
        assert.equal(await read(SID.admin), 200);
        assert.equal(await read(SID.user), 200);
        assert.equal(await read(SID.shim), 200, 'unknown may read');
        assert.equal(await read(SID.guest), 403);
        assert.equal((await act(SID.admin)).status, 200);
        const u = await act(SID.user);
        assert.equal(u.status, 403);
        assert.equal(u.json.error.code, 'forbidden');
        const k = await act(SID.shim);
        assert.equal(k.status, 403);
        assert.equal(k.json.error.message, 'the level of this session could not be confirmed');
        const s = await lv.cgi('www/kit/session.cgi', {query: {sid: SID.user}});
        assert.deepEqual(s.json.permissions, {read: true, service: false, log: false, write: false});
    });
});

describe('CSRF and methods', () => {
    let t;
    before(async () => { t = await setup(); regaSessions(t); });
    after(() => t.close());
    const post = (headers) => t.cgi('www/kit/service.cgi', {method: 'POST', query: {sid: SID.admin}, headers, body: {cmd: 'restart'}});

    it('takes a POST with the header and this host as Origin', async () => {
        assert.equal((await post(CSRF)).status, 200);
        assert.equal((await post({'X-CCU-Addon-Request': '1'})).status, 200, 'without Origin and Fetch Metadata');
        assert.equal((await post({'X-CCU-Addon-Request': '1', Origin: 'https://CCU.example:8080', 'Sec-Fetch-Site': 'none'})).status, 200);
    });

    it('refuses a missing header, a foreign or null Origin, a cross-site fetch', async () => {
        const cases = [
            [{Origin: 'http://ccu.example:8080'}, 'header'],
            [{'X-CCU-Addon-Request': 'yes'}, 'header'],
            [{...CSRF, Origin: 'http://evil.example'}, 'origin'],
            [{...CSRF, Origin: 'http://ccu.example'}, 'origin'],
            [{...CSRF, Origin: 'null'}, 'origin'],
            [{...CSRF, 'Sec-Fetch-Site': 'cross-site'}, 'fetch-site'],
            [{...CSRF, 'Sec-Fetch-Site': 'same-site'}, 'fetch-site'],
        ];
        t.write('logger', '');
        for (const [headers, reason] of cases) {
            const r = await post(headers);
            assert.equal(r.status, 403, JSON.stringify(headers));
            assert.equal(r.json.error.code, 'csrf');
            assert.equal(r.json.error.message, `request refused: ${reason}`);
        }
        assert.match(t.read('logger'), /-t kitdemo-kit -p daemon.warning csrf refused: header from 192.0.2.10/);
        assert.equal(t.read('rc-calls').includes('restart\nrestart\nrestart\nrestart'), false);
    });

    it('without Host (openccu-lite\'s CGI runner): X-Forwarded-Host, else SERVER_NAME and its port', async () => {
        const lite = (headers, env) => t.cgi('www/kit/service.cgi', {method: 'POST', query: {sid: SID.admin},
            headers: {Host: null, 'X-CCU-Addon-Request': '1', ...headers}, env, body: {cmd: 'restart'}});
        assert.equal((await lite({Origin: 'https://172.16.0.5'}, {SERVER_NAME: '172.16.0.5', SERVER_PORT: '443'})).status, 200);
        assert.equal((await lite({Origin: 'http://ccu.local:8080'}, {SERVER_NAME: 'ccu.local', SERVER_PORT: '8080'})).status, 200);
        assert.equal((await lite({Origin: 'http://ccu.local'}, {SERVER_NAME: 'ccu.local', SERVER_PORT: '8080'})).status, 403);
        assert.equal((await lite({Origin: 'https://evil.example'}, {SERVER_NAME: '172.16.0.5', SERVER_PORT: '443'})).status, 403);
        assert.equal((await lite({Origin: 'https://ccu.example', 'X-Forwarded-Host': 'ccu.example, proxy'}, {})).status, 200);
        assert.equal((await lite({Origin: 'https://ccu.example'}, {})).json.error.message, 'request refused: origin');
    });

    it('answers 405 to a write as GET and to other methods', async () => {
        const g = await t.cgi('www/kit/service.cgi', {query: {sid: SID.admin, cmd: 'stop'}});
        assert.equal(g.status, 405);
        assert.equal(g.json.error.code, 'method');
        for (const method of ['PUT', 'DELETE', 'OPTIONS']) {
            const r = await t.cgi('www/kit/service.cgi', {method, query: {sid: SID.admin}});
            assert.equal(r.status, 405, method);
            assert.equal(r.headers.allow, 'GET, POST');
            assert.equal(r.headers['access-control-allow-origin'], undefined);
        }
    });
});
