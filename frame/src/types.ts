// The frame's public types (docs/tab-contract.md).

import type {Component} from 'svelte';
import type {IconData} from 'occulite-ui/icons';

export type Lang = 'de' | 'en';
export type Level = 'admin' | 'user' | 'guest' | 'unknown';
export type RequestClass = 'read' | 'write' | 'service' | 'log';
export type PlatformKind = 'ccu3' | 'openccu' | 'openccu-lite';
export type ServiceCommand = 'start' | 'stop' | 'restart' | 'reload';
export type ServiceView = 'running' | 'stopped' | 'failed' | 'busy' | 'unknown';
export type NotifyKind = 'info' | 'success' | 'warning' | 'error';

/** A text in both languages of the UI. */
export interface I18nText {
    readonly de: string;
    readonly en: string;
}

/** The props every tab component gets. */
export interface KitTabProps {
    /** true while the tab is shown: start and stop polling with it. */
    active: boolean;
    /** The sub-route after the tab id: `#/devices/JEQ0123456` -> `JEQ0123456`. */
    path: string;
}

export type TabComponent = Component<KitTabProps>;
export type LazyTabComponent = () => Promise<{default: TabComponent}>;

/** An addon's tab, registered with `defineTabs()` and `mountFrame()`. */
export interface KitTab {
    /** `[a-z0-9][a-z0-9-]*`, not starting with `kit`; must be listed in kit.json's `tabs`. */
    readonly id: string;
    readonly label: I18nText;
    /** Icon data from `occulite-ui/icons` (or an app's own), drawn before the label. */
    readonly icon?: IconData;
    /** The component, or `() => import('./Tab.svelte')` to load it when the tab is first opened. */
    readonly component: TabComponent | LazyTabComponent;
    /** Hidden below this level (the CGIs still check). */
    readonly level?: 'user' | 'admin';
    /** A reactive badge after the label (`() => inbox.count`); `null` or `''` shows none. */
    readonly badge?: () => number | string | null;
}

/** The frame's view of kit.json (`www/frame/addon.json`, written by `vendor`). */
export interface AddonConfig {
    readonly schema: 1;
    readonly id: string;
    /** The frame's directory, `/addons/<www>/` (the id unless the addon's own app lives at `/addons/<id>/`). */
    readonly www?: string;
    readonly name: I18nText;
    readonly icon: string | null;
    readonly kit: string;
    readonly service: {readonly commands: readonly ServiceCommand[]; readonly pollSeconds: number};
    readonly logs: readonly {readonly id: string; readonly label: I18nText; readonly kind: 'file' | 'syslog'}[];
    readonly update: {readonly github: string} | null;
    readonly tabs: readonly string[];
    readonly proxied: {readonly path: string} | null;
    /** The addon's own web application, served by its backend; the frame links it prominently. */
    readonly app?: {readonly path: string; readonly label: I18nText} | null;
    readonly links: readonly {readonly label: I18nText; readonly url: string}[];
}

/** The status line of the rc.d script (service convention v1), plus what service.cgi adds. */
export interface ServiceStatus {
    readonly schema: number;
    readonly running: boolean;
    readonly state?: 'running' | 'stopped' | 'failed' | 'starting' | 'stopping';
    readonly pid?: number;
    readonly pids?: readonly number[];
    readonly rss_kb?: number;
    readonly since?: number | null;
    readonly version?: string;
    readonly cpu_pct?: number;
    readonly lastError?: string;
    readonly url?: string;
    readonly components?: Readonly<Record<string, string>>;
    /** The script's output when it was not a JSON line (`schema: 0`). */
    readonly raw?: string;
    /** `X-Kit-Exit`: the status command's exit code. */
    readonly exit: number | null;
    /** `X-Kit-Mem-Total-Kb`, `X-Kit-Mem-Available-Kb`. */
    readonly memTotalKb: number | null;
    readonly memAvailableKb: number | null;
}

export interface CommandResult {
    readonly ok: boolean;
    readonly cmd: ServiceCommand;
    readonly exit: number | null;
    readonly output: string;
    /** The error code when it failed (`failed`, `timeout`, `busy`, ...). */
    readonly code?: string;
    /** false when the user said no to the question before stop/restart. */
    readonly confirmed: boolean;
}

export interface ApiOptions {
    /** Default GET; POST adds the request header `X-CCU-Addon-Request: 1`. */
    readonly method?: 'GET' | 'POST';
    readonly query?: Readonly<Record<string, string | number | boolean>>;
    /** The POST body, `application/x-www-form-urlencoded` (what Tcl 8.2 parses). Implies POST. */
    readonly form?: Readonly<Record<string, string | number | boolean>>;
    /** Default `json`; `raw` resolves to {status, headers, json, text}. */
    readonly as?: 'json' | 'text' | 'raw';
    /** Default 30000 ms. */
    readonly timeoutMs?: number;
    readonly signal?: AbortSignal;
}

export interface AskOptions {
    readonly title: string;
    readonly message: string;
    readonly confirm: string;
    readonly cancel?: string;
    readonly danger?: boolean;
}

export interface AskTextOptions {
    readonly title: string;
    readonly message: string;
    readonly value?: string;
    readonly confirm: string;
    readonly cancel?: string;
}

export interface KitSession {
    readonly valid: boolean;
    readonly expired: boolean;
    readonly level: Level;
    readonly user: string | null;
    can(cls: RequestClass): boolean;
}

export interface KitService {
    readonly status: ServiceStatus | null;
    readonly view: ServiceView;
    /** The command in flight, or null. */
    readonly pending: ServiceCommand | null;
    /** The last action's result, for the Service tab's disclosure. */
    readonly last: CommandResult | null;
    readonly commands: readonly ServiceCommand[];
    refresh(): Promise<void>;
    /** Asks first for stop and restart. */
    run(cmd: ServiceCommand): Promise<CommandResult>;
}

export interface KitRoute {
    readonly tab: string;
    readonly path: string;
    go(tab: string, path?: string): void;
}

/** The context every tab reaches with `getKit()`. Every field is reactive. */
export interface Kit {
    readonly addon: {readonly id: string; readonly name: string; readonly version: string | null; readonly kitVersion: string};
    readonly config: AddonConfig;
    readonly platform: {readonly kind: PlatformKind; readonly firmware: string; readonly arch: string; readonly tcl: string};
    readonly session: KitSession;
    readonly service: KitService;
    /**
     * The addon's own web application (kit.json `app`): its URL with the session handed over as
     * the WebUI hands it to the settings page (`?sid=@...@` on a CCU; openccu-lite's gate needs
     * nothing), and its label in the page's language. null without one.
     */
    readonly app: {readonly url: string; readonly label: string} | null;
    /** `/addons/<www>/`, the frame's directory; `api()` paths are relative to it. */
    readonly base: string;
    api<T = unknown>(cgi: string, opts?: ApiOptions): Promise<T>;
    /** The addon's strings first, then the frame's; a missing key shows the key. */
    t(key: string, params?: Record<string, string | number>): string;
    /** A `{de, en}` text in the page's language. */
    text(t: I18nText): string;
    readonly lang: Lang;
    readonly theme: 'light' | 'dark';
    /** Inside openccu-lite's shell (a same-origin frame that follows the shell). */
    readonly embedded: boolean;
    notify(kind: NotifyKind, text: string, opts?: {timeoutMs?: number}): void;
    ask(o: AskOptions): Promise<boolean>;
    askText(o: AskTextOptions): Promise<string | null>;
    readonly route: KitRoute;
    /** The frame asks before leaving the tab or the page with unsaved input. */
    setDirty(dirty: boolean): void;
}
