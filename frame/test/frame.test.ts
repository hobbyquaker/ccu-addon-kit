// Unit tests of the frame's pure parts and the stores (vitest, jsdom).

import {describe, expect, it, vi} from 'vitest';
import {flushSync} from 'svelte';
import {addMessages, i18n, t} from 'occulite-ui/i18n';

import {apiUrl, createApi, KitRequestError, REQUEST_HEADER} from '../src/api.js';
import {hashOf, KitState, parseHash, routeOf, tabOfRoute, visibleTabs} from '../src/kit.svelte.js';
import {FRAME_MESSAGES} from '../src/messages.js';
import {toCatalogue} from '../src/mountFrame.js';
import {bareSid, takeSid} from '../src/sid.js';
import {formatDuration, formatKb, parseStatus, statusView} from '../src/status.js';
import type {AddonConfig, KitTab, ServiceStatus} from '../src/types.js';
import {scanTabIds} from '../../vite/index.mjs';

const CONFIG: AddonConfig = {
    schema: 1,
    id: 'kitdemo',
    name: {de: 'Kit Demo DE', en: 'Kit Demo'},
    icon: null,
    kit: '0.1.0',
    service: {commands: ['start', 'stop', 'restart', 'reload'], pollSeconds: 10},
    logs: [],
    update: null,
    tabs: ['hello', 'admin-only', 'kit:service', 'kit:log', 'kit:about'],
    proxied: null,
    links: [],
};

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
    return new Response(JSON.stringify(body), {status, headers: {'Content-Type': 'application/json; charset=utf-8', ...headers}});
}

function st(o: Partial<ServiceStatus>): ServiceStatus {
    return {schema: 1, running: false, exit: 3, memTotalKb: null, memAvailableKb: null, ...o};
}

describe('api()', () => {
    it('adds the sid to the query, the base to a relative path', () => {
        const setup = {base: '/addons/kitdemo/', sid: () => '@AbCdEfGhIj@'};
        expect(apiUrl(setup, 'api/hello.cgi', {room: 'eg', n: 2, on: true})).toBe('/addons/kitdemo/api/hello.cgi?room=eg&n=2&on=1&sid=%40AbCdEfGhIj%40');
        expect(apiUrl({base: '/addons/x/', sid: () => null}, 'kit/session.cgi')).toBe('/addons/x/kit/session.cgi');
        expect(apiUrl({base: '/addons/x/', sid: () => null}, '/addons/x/app/v1')).toBe('/addons/x/app/v1');
    });

    it('POSTs a form with the request header; GETs without it', async () => {
        const calls: [string, RequestInit][] = [];
        const api = createApi({base: '/addons/k/', sid: () => '@AbCdEfGhIj@', fetch: async (u, init) => {
            calls.push([String(u), init ?? {}]);
            return json({ok: true, saved: true});
        }});
        await api('api/hello.cgi', {form: {greeting: 'hi & bye', n: 1}});
        await api('api/hello.cgi');
        const [url, init] = calls[0]!;
        expect(url).toBe('/addons/k/api/hello.cgi?sid=%40AbCdEfGhIj%40');
        expect(init.method).toBe('POST');
        expect((init.headers as Record<string, string>)[REQUEST_HEADER]).toBe('1');
        expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/x-www-form-urlencoded');
        expect(init.body).toBe('greeting=hi+%26+bye&n=1');
        expect(calls[1]![1].method).toBe('GET');
        expect((calls[1]![1].headers as Record<string, string>)[REQUEST_HEADER]).toBeUndefined();
    });

    it('throws the error envelope as KitRequestError; session-invalid calls onExpired', async () => {
        const onExpired = vi.fn();
        const api = createApi({base: '/a/', sid: () => null, onExpired, fetch: async () =>
            json({ok: false, error: {code: 'session-invalid', message: 'no session'}}, 401)});
        const e = await api('kit/service.cgi').catch((x: unknown) => x);
        expect(e).toBeInstanceOf(KitRequestError);
        expect((e as KitRequestError).code).toBe('session-invalid');
        expect((e as KitRequestError).status).toBe(401);
        expect(onExpired).toHaveBeenCalledOnce();
    });

    it('a 200 with ok:false is an error too; a non-JSON 500 is http-500', async () => {
        const a = createApi({base: '/a/', sid: () => null, fetch: async () => json({ok: false, error: {code: 'busy', message: 'x'}})});
        await expect(a('x')).rejects.toMatchObject({code: 'busy'});
        const b = createApi({base: '/a/', sid: () => null, fetch: async () => new Response('<html>', {status: 500, headers: {'Content-Type': 'text/html'}})});
        await expect(b('x')).rejects.toMatchObject({code: 'http-500', status: 500});
    });

    it('a network failure is code network, a timeout is code timeout', async () => {
        const a = createApi({base: '/a/', sid: () => null, fetch: async () => { throw new TypeError('Failed to fetch'); }});
        await expect(a('x')).rejects.toMatchObject({code: 'network'});
        const b = createApi({base: '/a/', sid: () => null, fetch: (_u, init) => new Promise((_r, rej) => {
            init?.signal?.addEventListener('abort', () => rej(new DOMException('aborted', 'AbortError')));
        })});
        await expect(b('x', {timeoutMs: 20})).rejects.toMatchObject({code: 'timeout'});
    });

    it('raw gives the headers', async () => {
        const a = createApi({base: '/a/', sid: () => null, fetch: async () => json({schema: 1, running: true, pid: 4, rss_kb: 2048}, 200, {'X-Kit-Exit': '0'})});
        const r = await a<{headers: Headers; json: unknown}>('kit/service.cgi', {as: 'raw'});
        expect(r.headers.get('X-Kit-Exit')).toBe('0');
        expect(parseStatus(r.json, r.headers)).toMatchObject({running: true, exit: 0, memTotalKb: null});
    });
});

describe('the status view (service convention v1)', () => {
    it('maps the status line and the exit code', () => {
        expect(statusView(st({running: true, exit: 0}))).toBe('running');
        expect(statusView(st({}))).toBe('stopped');
        expect(statusView(st({exit: 1}))).toBe('stopped'); // exit 1 without a state: stopped
        expect(statusView(st({exit: 1, state: 'failed'}))).toBe('failed');
        expect(statusView(st({state: 'starting'}))).toBe('busy');
        expect(statusView(st({running: true, state: 'stopping'}))).toBe('busy');
        expect(statusView(st({exit: 4}))).toBe('unknown');
        expect(statusView(st({schema: 0, exit: 3}))).toBe('stopped');
        expect(statusView(st({schema: 0, exit: 4}))).toBe('unknown');
        expect(statusView(st({schema: 2}))).toBe('unknown');
        expect(statusView(null)).toBe('unknown');
        expect(statusView(st({running: true}), 'restart')).toBe('busy');
    });

    it('parses only a status object; reads the memory headers', () => {
        const h = new Headers({'X-Kit-Exit': '3', 'X-Kit-Mem-Total-Kb': '1012340', 'X-Kit-Mem-Available-Kb': ''});
        expect(parseStatus({schema: 1, running: false}, h)).toMatchObject({exit: 3, memTotalKb: 1012340, memAvailableKb: null});
        expect(parseStatus({ok: true}, h)).toBeNull();
        expect(parseStatus([1], h)).toBeNull();
    });

    it('formats memory and durations', () => {
        expect(formatKb(49459, 'en')).toBe('48 MB');
        expect(formatKb(9830, 'en')).toBe('9.6 MB');
        expect(formatKb(9830, 'de')).toBe('9,6 MB');
        expect(formatKb(1012340, 'en')).toBe('989 MB');
        expect(formatKb(2097152, 'en')).toBe('2 GB');
        expect(formatDuration(45)).toBe('45 s');
        expect(formatDuration(3 * 3600 + 12 * 60)).toBe('3 h 12 min');
        expect(formatDuration(2 * 86400 + 4 * 3600)).toBe('2 d 4 h');
    });
});

describe('routes', () => {
    it('round-trips tab ids and sub-paths through the hash', () => {
        expect(routeOf('kit:service')).toBe('kit-service');
        expect(tabOfRoute('kit-log')).toBe('kit:log');
        expect(tabOfRoute('devices')).toBe('devices');
        expect(parseHash('#/devices/JEQ0123456:1')).toEqual({tab: 'devices', path: 'JEQ0123456:1'});
        expect(parseHash('#/kit-about')).toEqual({tab: 'kit:about', path: ''});
        expect(parseHash('')).toEqual({tab: '', path: ''});
        expect(hashOf('kit:log')).toBe('#/kit-log');
        expect(parseHash(hashOf('devices', 'a b/c'))).toEqual({tab: 'devices', path: 'a b/c'});
    });

    it('lists kit.json\'s tabs that are registered and allowed', () => {
        const C = () => null as never;
        const tabs: KitTab[] = [
            {id: 'hello', label: {de: 'a', en: 'a'}, component: C},
            {id: 'admin-only', label: {de: 'b', en: 'b'}, component: C, level: 'admin'},
        ];
        expect(visibleTabs(CONFIG, tabs, 'admin')).toEqual(['hello', 'admin-only', 'kit:service', 'kit:log', 'kit:about']);
        expect(visibleTabs(CONFIG, tabs, 'user')).toEqual(['hello', 'kit:service', 'kit:log', 'kit:about']);
        expect(visibleTabs(CONFIG, tabs, 'unknown')).toEqual(['hello', 'kit:service', 'kit:log', 'kit:about']);
        expect(visibleTabs(CONFIG, tabs.slice(1), 'admin')).toEqual(['admin-only', 'kit:service', 'kit:log', 'kit:about']);
    });

    it('follows the hash; an unknown tab goes to the first', () => {
        history.replaceState(null, '', '/addons/kitdemo/settings.cgi#/nope');
        const kit = new KitState({config: CONFIG, sid: null, fetch: async () => json({})});
        kit.setTabs(['hello', 'kit:service']);
        expect(kit.tab).toBe('hello');
        expect(location.hash).toBe('#/hello');
        history.replaceState(null, '', '#/kit-service/x');
        kit.setTabs(['hello', 'kit:service']);
        expect([kit.tab, kit.path]).toEqual(['kit:service', 'x']);
    });

    it('asks before leaving a dirty tab', async () => {
        history.replaceState(null, '', '/addons/kitdemo/settings.cgi#/hello');
        const kit = new KitState({config: CONFIG, sid: null, fetch: async () => json({})});
        kit.setTabs(['hello', 'kit:service']);
        kit.setDirty(true);
        const going = kit.go('kit:service');
        expect(kit.question?.options.title).toBeTruthy();
        kit.answer(false);
        await going;
        expect(kit.tab).toBe('hello');
        expect(kit.nonce).toBe(1);
        const again = kit.go('kit:service');
        kit.answer(true);
        await again;
        expect(kit.tab).toBe('kit:service');
        expect(kit.anyDirty).toBe(false);
    });
});

describe('the session and the service store', () => {
    function kitWith(handler: (url: string, init: RequestInit) => Response) {
        const calls: string[] = [];
        const kit = new KitState({config: CONFIG, sid: '@AbCdEfGhIj@', fetch: async (u, init) => {
            calls.push(`${init?.method ?? 'GET'} ${String(u)}`);
            return handler(String(u), init ?? {});
        }});
        return {kit, calls};
    }
    const SESSION = {ok: true, session: {valid: true, level: 'admin', user: 'Admin', via: 'sid', lang: 'en'},
        platform: {kind: 'openccu', firmware: '3.89.8', arch: 'x86_64', tcl: '8.6'}, kit: {version: '0.1.0', requestHeader: REQUEST_HEADER},
        permissions: {read: true, service: true, log: true, write: true}};

    it('takes the level and permissions from session.cgi and its language when nobody chose one', async () => {
        i18n.language = 'de';
        const {kit} = kitWith(() => json(SESSION));
        expect(await kit.checkSession()).toBe(true);
        expect(kit.session.level).toBe('admin');
        expect(kit.session.can('service')).toBe(true);
        expect(kit.lang).toBe('en');
        expect(document.documentElement.lang).toBe('en');
    });

    it('a 401 anywhere sets expired; Retry clears it when the session is back', async () => {
        let valid = true;
        const {kit} = kitWith((u) => {
            if (u.includes('session.cgi')) return json({...SESSION, session: {...SESSION.session, valid}});
            return json({ok: false, error: {code: 'session-invalid', message: ''}}, 401);
        });
        await kit.checkSession();
        await kit.refreshStatus();
        expect(kit.expired).toBe(true);
        expect(kit.session.valid).toBe(false);
        valid = false;
        expect(await kit.retrySession()).toBe(false);
        expect(kit.expired).toBe(true);
        valid = true;
        expect(await kit.retrySession()).toBe(true);
        expect(kit.expired).toBe(false);
        kit.stop();
    });

    it('stop asks first; no means no request', async () => {
        const {kit, calls} = kitWith((u, init) => {
            if (u.includes('session.cgi')) return json(SESSION);
            if (init.method === 'POST') return json({ok: true, cmd: 'stop', exit: 0, output: 'Stopping: OK'});
            return json({schema: 1, running: true, pid: 1, rss_kb: 1}, 200, {'X-Kit-Exit': '0'});
        });
        await kit.checkSession();
        const no = kit.service.run('stop');
        expect(kit.question?.options).toMatchObject({danger: true});
        kit.answer(false);
        expect(await no).toMatchObject({confirmed: false});
        expect(calls.filter((c) => c.startsWith('POST'))).toEqual([]);
        const yes = kit.service.run('stop');
        kit.answer(true);
        const r = await yes;
        expect(r).toMatchObject({ok: true, exit: 0, output: 'Stopping: OK'});
        expect(calls.filter((c) => c.startsWith('POST'))).toEqual(['POST /addons/kitdemo/kit/service.cgi?sid=%40AbCdEfGhIj%40']);
        expect(kit.lastResult?.cmd).toBe('stop');
        kit.stop();
    });

    it('a failed action keeps its output and notifies', async () => {
        const {kit} = kitWith((u, init) => {
            if (u.includes('session.cgi')) return json(SESSION);
            if (init.method === 'POST') return json({ok: false, error: {code: 'failed', message: 'exit 1'}, cmd: 'start', exit: 1, output: 'no config'}, 500);
            return json({schema: 1, running: false}, 200, {'X-Kit-Exit': '3'});
        });
        await kit.checkSession();
        const r = await kit.service.run('start');
        expect(r).toMatchObject({ok: false, code: 'failed', exit: 1, output: 'no config'});
        expect(kit.notices.items.at(-1)?.level).toBe('error');
        kit.stop();
    });

    it('the keep-alive renews the WebUI session on a CCU, never on openccu-lite', async () => {
        const bodies: string[] = [];
        const {kit} = kitWith((u, init) => {
            if (u === '/api/homematic.cgi') { bodies.push(String(init.body)); return json({result: true}); }
            return json(SESSION);
        });
        await kit.checkSession();
        await kit.keepAlive();
        expect(JSON.parse(bodies[0]!)).toEqual({version: '1.1', method: 'Session.renew', params: {_session_id_: 'AbCdEfGhIj'}});
        const lite = kitWith(() => json({...SESSION, platform: {...SESSION.platform, kind: 'openccu-lite'}}));
        await lite.kit.checkSession();
        await lite.kit.keepAlive();
        expect(lite.calls.some((c) => c.includes('homematic.cgi'))).toBe(false);
    });
});

describe('i18n and the sid', () => {
    it('converts per-language strings; the addon wins over the frame; missing keys show the key', () => {
        addMessages(FRAME_MESSAGES);
        addMessages(toCatalogue({de: {'hello.x': 'Hallo', 'kit.retry': 'Nochmal'}, en: {'hello.x': 'Hello'}}));
        i18n.language = 'de';
        flushSync();
        expect(t('hello.x')).toBe('Hallo');
        expect(t('kit.retry')).toBe('Nochmal');
        expect(t('kit.cancel')).toBe('Abbrechen');
        expect(t('no.such.key')).toBe('no.such.key');
        i18n.language = 'en';
        expect(t('kit.retry')).toBe('Retry');
        expect(toCatalogue({k: {de: 'a', en: 'b'}})).toEqual({k: {de: 'a', en: 'b'}});
    });

    it('every frame string has German and English', () => {
        for (const [k, v] of Object.entries(FRAME_MESSAGES)) {
            expect(typeof v['de'], k).toBe('string');
            expect(typeof v['en'], k).toBe('string');
            expect(String(v['de']) + String(v['en']), k).not.toMatch(/\bbox\b/i);
        }
    });

    it('takes the sid from the address once and removes it there', () => {
        sessionStorage.clear();
        history.replaceState(null, '', '/addons/kitdemo/settings.cgi?sid=@AbCdEfGhIj@&x=1#/hello');
        expect(takeSid('kitdemo')).toBe('@AbCdEfGhIj@');
        expect(location.search).toBe('?x=1');
        expect(location.hash).toBe('#/hello');
        expect(takeSid('kitdemo')).toBe('@AbCdEfGhIj@'); // a reload: from sessionStorage
        history.replaceState(null, '', '/addons/kitdemo/settings.cgi?sid=AbCdEfGhI2');
        expect(takeSid('kitdemo')).toBe('@AbCdEfGhI2@');
        history.replaceState(null, '', '/addons/kitdemo/settings.cgi?sid=%3Cscript%3E');
        expect(takeSid('kitdemo')).toBe('@AbCdEfGhI2@');
        expect(bareSid('@AbCdEfGhIj@')).toBe('AbCdEfGhIj');
    });
});

describe('kitVite: the tab ids of defineTabs()', () => {
    it('collects the id literals inside the call only', () => {
        const code = `
            const other = {id: 'not-a-tab'};
            const tabs = defineTabs([
                {id: 'overview', label: {de: 'Ü (1)', en: 'O)'}, component: O}, // a ) in a string
                {"id": "devices", component: () => import('./D.svelte')},
                /* {id: 'commented'} is still inside the call */
            ]);`;
        expect(scanTabIds(code)).toEqual(['overview', 'devices', 'commented']);
    });
});
