// The frame's state: session, service, route, notices, questions - the object getKit() hands to
// every tab (docs/tab-contract.md). Svelte 5 runes; every field a tab reads is reactive.

import {i18n, languageChoice, setLanguage, t as kitT} from 'occulite-ui/i18n';
import {NoticesStore} from 'occulite-ui/notices';
import {theme} from 'occulite-ui/theme';
import {shellEmbedding} from 'occulite-ui/embedding';

import {apiUrl, createApi, isKitError, KitRequestError, type Api, type ApiRaw} from './api.js';
import {bareSid} from './sid.js';
import {parseStatus, statusView} from './status.js';
import type {
    AddonConfig,
    AskOptions,
    AskTextOptions,
    CommandResult,
    I18nText,
    Kit,
    KitRoute,
    KitService,
    KitSession,
    KitTab,
    Lang,
    Level,
    NotifyKind,
    PlatformKind,
    RequestClass,
    ServiceCommand,
    ServiceStatus,
    ServiceView,
} from './types.js';

/** session.cgi's answer. */
export interface SessionAnswer {
    readonly ok: true;
    readonly session: {valid: boolean; level?: Level; user?: string; via?: string; lang?: string};
    readonly platform: {kind: PlatformKind; firmware: string; arch: string; tcl: string};
    readonly kit: {version: string; requestHeader: string};
    readonly permissions: Partial<Record<RequestClass, boolean>>;
}

/** A question on screen: the frame's dialog renders it (occulite-ui 0.1 has no `ask()` yet). */
export interface PendingQuestion {
    readonly kind: 'confirm' | 'text';
    readonly options: AskOptions | AskTextOptions;
    value: string;
    readonly resolve: (answer: boolean | string | null) => void;
}

/** The route segment of a tab id: `kit:service` -> `kit-service`. */
export function routeOf(id: string): string {
    return id.replace(':', '-');
}

/** The tab id of a route segment. */
export function tabOfRoute(seg: string): string {
    return /^kit-(service|log|about)$/.test(seg) ? seg.replace('-', ':') : seg;
}

/** `#/devices/JEQ0123456` -> {tab: 'devices', path: 'JEQ0123456'} (the route segment's tab id). */
export function parseHash(hash: string): {tab: string; path: string} {
    const m = /^#\/([^/]*)(?:\/(.*))?$/.exec(hash);
    if (!m) return {tab: '', path: ''};
    let path = m[2] ?? '';
    try {
        path = decodeURIComponent(path);
    } catch {
        // keep it as it came
    }
    return {tab: tabOfRoute(m[1] ?? ''), path};
}

export function hashOf(tab: string, path = ''): string {
    return `#/${routeOf(tab)}${path ? `/${encodeURIComponent(path).replace(/%2F/gi, '/')}` : ''}`;
}

const KEEPALIVE_MS = 4 * 60 * 1000;
const FAST_POLL_MS = 2000;
const FAST_FOR_MS = 30000;

export interface KitStateSetup {
    readonly config: AddonConfig;
    readonly sid: string | null;
    readonly win?: Window;
    readonly fetch?: typeof fetch;
}

export class KitState implements Kit {
    readonly config: AddonConfig;
    readonly base: string;
    readonly sid: string | null;
    readonly notices = new NoticesStore();
    readonly api: Api;

    // session.cgi
    sessionAnswer = $state.raw<SessionAnswer | null>(null);
    expired = $state(false);
    // service
    status = $state.raw<ServiceStatus | null>(null);
    statusError = $state.raw<KitRequestError | null>(null);
    pending = $state<ServiceCommand | null>(null);
    lastResult = $state.raw<CommandResult | null>(null);
    // route
    tab = $state('');
    path = $state('');
    // dirty tabs (by id)
    dirty = $state<Record<string, boolean>>({});
    question = $state<PendingQuestion | null>(null);
    /** Bumped when a tab switch was cancelled, so the tab bar shows the current tab again. */
    nonce = $state(0);
    /** Calls of Session.renew so far (the keep-alive; for the tests and the About tab). */
    keepAlives = $state(0);

    readonly #win: Window;
    readonly #fetch: typeof fetch;
    #pollTimer: ReturnType<typeof setTimeout> | undefined;
    #fastUntil = 0;
    #keepAliveTimer: ReturnType<typeof setInterval> | undefined;
    #tabs: readonly string[] = [];
    #stopped = false;

    constructor(setup: KitStateSetup) {
        this.config = setup.config;
        this.base = `/addons/${setup.config.www ?? setup.config.id}/`;
        this.sid = setup.sid;
        this.#win = setup.win ?? window;
        this.#fetch = setup.fetch ?? ((i, o) => fetch(i, o));
        this.api = createApi({
            base: this.base,
            sid: () => this.sid,
            onExpired: () => this.#expire(),
            fetch: this.#fetch,
        });
    }

    // ---------------------------------------------------------------- the Kit interface

    get addon(): Kit['addon'] {
        return {
            id: this.config.id,
            name: this.text(this.config.name),
            version: this.status?.version ?? null,
            kitVersion: this.sessionAnswer?.kit.version ?? this.config.kit,
        };
    }

    get app(): Kit['app'] {
        const app = this.config.app;
        if (!app) return null;
        const sid = this.isLite ? null : this.sid;
        const url = sid ? `${app.path}${app.path.includes('?') ? '&' : '?'}sid=${encodeURIComponent(sid)}` : app.path;
        return {url, label: this.text(app.label)};
    }

    get platform(): Kit['platform'] {
        return this.sessionAnswer?.platform ?? {kind: 'openccu', firmware: '', arch: '', tcl: ''};
    }

    get isLite(): boolean {
        return this.platform.kind === 'openccu-lite';
    }

    readonly session: KitSession = (() => {
        // eslint-disable-next-line @typescript-eslint/no-this-alias -- the getters read the state
        const self = this;
        return {
            get valid() {
                return self.sessionAnswer?.session.valid === true && !self.expired;
            },
            get expired() {
                return self.expired;
            },
            get level() {
                return self.sessionAnswer?.session.level ?? 'unknown';
            },
            get user() {
                return self.sessionAnswer?.session.user || null;
            },
            can(cls: RequestClass) {
                return this.valid && self.sessionAnswer?.permissions[cls] === true;
            },
        };
    })();

    readonly service: KitService = (() => {
        // eslint-disable-next-line @typescript-eslint/no-this-alias -- the getters read the state
        const self = this;
        return {
            get status() {
                return self.status;
            },
            get view(): ServiceView {
                return statusView(self.status, self.pending);
            },
            get pending() {
                return self.pending;
            },
            get last() {
                return self.lastResult;
            },
            get commands() {
                return self.config.service.commands;
            },
            refresh: () => this.refreshStatus(),
            run: (cmd: ServiceCommand) => this.runCommand(cmd),
        };
    })();

    readonly route: KitRoute = (() => {
        // eslint-disable-next-line @typescript-eslint/no-this-alias -- the getters read the state
        const self = this;
        return {
            get tab() {
                return self.tab;
            },
            get path() {
                return self.path;
            },
            go: (tab: string, path = '') => {
                void this.go(tab, path);
            },
        };
    })();

    get lang(): Lang {
        return i18n.language === 'de' ? 'de' : 'en';
    }

    get theme(): 'light' | 'dark' {
        return theme.resolved;
    }

    get embedded(): boolean {
        return shellEmbedding().embedded;
    }

    readonly t = (key: string, params: Record<string, string | number> = {}): string => {
        return kitT(key, params);
    };

    /** The URL of a CGI with the query and the sid, for a link (a download). */
    readonly url = (cgi: string, query?: Record<string, string | number | boolean>): string =>
        apiUrl({base: this.base, sid: () => this.sid}, cgi, query);

    readonly text = (t: I18nText): string => (this.lang === 'de' ? t.de : t.en);

    readonly notify = (kind: NotifyKind, text: string, opts: {timeoutMs?: number} = {}): void => {
        const level = kind === 'error' ? 'error' : kind === 'warning' ? 'warn' : 'info';
        const id = this.notices.push(level, text);
        if (opts.timeoutMs !== undefined && opts.timeoutMs > 0) {
            setTimeout(() => this.notices.dismiss(id), opts.timeoutMs);
        }
    };

    readonly ask = (options: AskOptions): Promise<boolean> =>
        new Promise((resolve) => {
            this.question = {kind: 'confirm', options, value: '', resolve: (a) => resolve(a === true)};
        });

    readonly askText = (options: AskTextOptions): Promise<string | null> =>
        new Promise((resolve) => {
            this.question = {
                kind: 'text',
                options,
                value: options.value ?? '',
                resolve: (a) => resolve(typeof a === 'string' ? a : null),
            };
        });

    /** The dialog's answer. */
    answer(a: boolean | string | null): void {
        const q = this.question;
        this.question = null;
        q?.resolve(a);
    }

    readonly setDirty = (dirty: boolean): void => {
        if (this.tab) {
            this.dirty = {...this.dirty, [this.tab]: dirty};
        }
    };

    get anyDirty(): boolean {
        return Object.values(this.dirty).some(Boolean);
    }

    // ---------------------------------------------------------------- session

    /** Asks session.cgi; true when the session is valid. */
    async checkSession(): Promise<boolean> {
        const answer = await this.api<SessionAnswer>('kit/session.cgi');
        this.sessionAnswer = answer;
        this.expired = false;
        this.#applySessionLang();
        return answer.session.valid;
    }

    /** The WebUI user's language, unless the user chose one here or the shell decides. */
    #applySessionLang(): void {
        const lang = this.sessionAnswer?.session.lang;
        if ((lang === 'de' || lang === 'en') && languageChoice.value === 'auto' && !shellEmbedding().embedded) {
            i18n.language = lang;
            this.#win.document.documentElement.lang = lang;
        }
    }

    /** The overlay's Retry: true when the session is back. */
    async retrySession(): Promise<boolean> {
        try {
            const ok = await this.checkSession();
            if (ok) {
                this.start(this.#tabs);
            } else {
                this.expired = true;
            }
            return ok;
        } catch (e) {
            if (!(isKitError(e) && e.code === 'session-invalid')) {
                this.expired = true;
            }
            return false;
        }
    }

    #expire(): void {
        if (!this.expired) {
            this.expired = true;
            this.#stopTimers();
        }
    }

    // ---------------------------------------------------------------- service

    async refreshStatus(): Promise<void> {
        if (!this.session.can('read')) {
            return;
        }
        try {
            const raw = await this.api<ApiRaw>('kit/service.cgi', {query: {cmd: 'status'}, as: 'raw', timeoutMs: 10000});
            this.status = parseStatus(raw.json, raw.headers) ?? {
                schema: 0,
                running: false,
                raw: raw.text,
                exit: Number(raw.headers.get('X-Kit-Exit') ?? 4),
                memTotalKb: null,
                memAvailableKb: null,
            };
            this.statusError = null;
        } catch (e) {
            this.status = null;
            this.statusError = isKitError(e) ? e : new KitRequestError('internal', 0, String(e));
        }
    }

    async runCommand(cmd: ServiceCommand): Promise<CommandResult> {
        if (cmd === 'stop' || cmd === 'restart') {
            const yes = await this.ask({
                title: this.t(`kit.ask.${cmd}.title`),
                message: this.t(`kit.ask.${cmd}.message`, {name: this.addon.name}),
                confirm: this.t(`kit.cmd.${cmd}`),
                cancel: this.t('kit.cancel'),
                danger: cmd === 'stop',
            });
            if (!yes) {
                return {ok: false, cmd, exit: null, output: '', confirmed: false};
            }
        }
        this.pending = cmd;
        let result: CommandResult;
        try {
            const r = await this.api<{exit?: number; output?: string}>('kit/service.cgi', {
                form: {cmd},
                timeoutMs: cmd === 'restart' ? 190000 : 100000,
            });
            result = {ok: true, cmd, exit: r.exit ?? 0, output: r.output ?? '', confirmed: true};
        } catch (e) {
            const body = (isKitError(e) ? e.body : null) as {exit?: number; output?: string; error?: {message?: string}} | null;
            result = {
                ok: false,
                cmd,
                exit: body?.exit ?? null,
                output: body?.output ?? body?.error?.message ?? (e instanceof Error ? e.message : String(e)),
                code: isKitError(e) ? e.code : 'internal',
                confirmed: true,
            };
            if (!(isKitError(e) && e.code === 'session-invalid')) {
                this.notify('error', this.t('kit.cmd.failed', {cmd: this.t(`kit.cmd.${cmd}`)}));
            }
        }
        this.pending = null;
        this.lastResult = result;
        this.#fastUntil = Date.now() + FAST_FOR_MS;
        await this.refreshStatus();
        this.#schedulePoll();
        return result;
    }

    #schedulePoll(): void {
        clearTimeout(this.#pollTimer);
        if (this.#stopped || this.expired) {
            return;
        }
        const ms = Date.now() < this.#fastUntil ? FAST_POLL_MS : this.config.service.pollSeconds * 1000;
        this.#pollTimer = setTimeout(() => {
            void this.#poll();
        }, ms);
    }

    async #poll(): Promise<void> {
        if (this.#win.document.visibilityState !== 'hidden' && this.pending === null) {
            await this.refreshStatus();
        }
        this.#schedulePoll();
    }

    readonly #onVisible = (): void => {
        if (this.#win.document.visibilityState === 'visible' && !this.expired && !this.#stopped) {
            void this.refreshStatus();
            this.#schedulePoll();
        }
    };

    // ---------------------------------------------------------------- keep-alive (CCU only)

    /** Session.renew on the WebUI's JSON API, so the ReGa session does not run out under the page. */
    async keepAlive(): Promise<void> {
        if (this.isLite || this.sid === null || this.expired) {
            return;
        }
        if (this.#win.document.visibilityState === 'hidden') {
            return;
        }
        this.keepAlives += 1;
        try {
            await this.#fetch('/api/homematic.cgi', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                credentials: 'same-origin',
                body: JSON.stringify({version: '1.1', method: 'Session.renew', params: {_session_id_: bareSid(this.sid)}}),
            });
        } catch {
            // the next data request says whether the session is still there
        }
    }

    // ---------------------------------------------------------------- route

    /** Sets the tab list (the ids the frame shows) and follows the hash. */
    setTabs(ids: readonly string[]): void {
        this.#tabs = ids;
        this.#followHash();
    }

    #followHash(): void {
        const {tab, path} = parseHash(this.#win.location.hash);
        const first = this.#tabs[0] ?? '';
        if (this.#tabs.includes(tab)) {
            this.tab = tab;
            this.path = path;
        } else {
            this.tab = first;
            this.path = '';
            if (first) {
                this.#win.history.replaceState(this.#win.history.state, '', hashOf(first));
            }
        }
    }

    readonly #onHash = (): void => {
        const {tab, path} = parseHash(this.#win.location.hash);
        if (tab !== this.tab && this.dirty[this.tab]) {
            // back to where we were; go() asks
            const target = {tab, path};
            this.#win.history.replaceState(this.#win.history.state, '', hashOf(this.tab, this.path));
            void this.go(target.tab, target.path);
            return;
        }
        this.#followHash();
    };

    /** Switches the tab, asking first when the current one has unsaved input. */
    async go(tab: string, path = ''): Promise<void> {
        if (tab === this.tab && path === this.path) {
            return;
        }
        if (tab !== this.tab && this.dirty[this.tab]) {
            const leave = await this.ask({
                title: this.t('kit.dirty.title'),
                message: this.t('kit.dirty.message'),
                confirm: this.t('kit.dirty.leave'),
                cancel: this.t('kit.cancel'),
                danger: true,
            });
            if (!leave) {
                this.nonce += 1;
                return;
            }
            this.dirty = {...this.dirty, [this.tab]: false};
        }
        this.#win.history.pushState(this.#win.history.state, '', hashOf(tab, path));
        this.#followHash();
    }

    readonly #onBeforeUnload = (e: BeforeUnloadEvent): void => {
        if (this.anyDirty) {
            e.preventDefault();
        }
    };

    // ---------------------------------------------------------------- lifecycle

    #listening = false;

    /** Starts polling, the keep-alive and the listeners once the session is valid. */
    start(tabs: readonly string[]): void {
        this.#stopped = false;
        this.#tabs = tabs;
        if (!this.#listening) {
            this.#listening = true;
            this.#win.addEventListener('hashchange', this.#onHash);
            this.#win.addEventListener('beforeunload', this.#onBeforeUnload);
            this.#win.document.addEventListener('visibilitychange', this.#onVisible);
        }
        void this.refreshStatus().then(() => this.#schedulePoll());
        clearInterval(this.#keepAliveTimer);
        if (!this.isLite && this.sid !== null) {
            this.#keepAliveTimer = setInterval(() => void this.keepAlive(), KEEPALIVE_MS);
        }
    }

    #stopTimers(): void {
        clearTimeout(this.#pollTimer);
        clearInterval(this.#keepAliveTimer);
    }

    stop(): void {
        this.#stopped = true;
        this.#stopTimers();
        this.#win.removeEventListener('hashchange', this.#onHash);
        this.#win.removeEventListener('beforeunload', this.#onBeforeUnload);
        this.#win.document.removeEventListener('visibilitychange', this.#onVisible);
        this.#listening = false;
        this.notices.dispose();
    }
}

/** Tab ids the frame shows, in kit.json's order: registered, allowed for the session's level. */
export function visibleTabs(config: AddonConfig, registered: readonly KitTab[], level: Level): string[] {
    const byId = new Map(registered.map((t) => [t.id, t]));
    const rank = (l: Level) => (l === 'admin' ? 2 : l === 'user' || l === 'unknown' ? 1 : 0);
    const out: string[] = [];
    for (const id of config.tabs) {
        if (id.startsWith('kit:')) {
            out.push(id);
            continue;
        }
        const tab = byId.get(id);
        if (!tab) {
            continue;
        }
        if (tab.level === 'admin' && rank(level) < 2) continue;
        if (tab.level === 'user' && rank(level) < 1) continue;
        out.push(id);
    }
    return out;
}

export {setLanguage};
