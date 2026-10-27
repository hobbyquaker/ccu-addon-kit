// api(): every request of the frame and its tabs (docs/tab-contract.md). Adds the WebUI's sid as
// the query parameter `sid=@...@`, the request header on a POST, a form-encoded body, a timeout;
// turns a non-2xx answer or {"ok": false} into a KitRequestError.

import type {ApiOptions} from './types.js';

export const REQUEST_HEADER = 'X-CCU-Addon-Request';

/** An answer of the kit's error envelope, a network failure (`network`) or a timeout (`timeout`). */
export class KitRequestError extends Error {
    readonly code: string;
    readonly status: number;
    /** The answer's body when it was JSON (a failed service action carries `output` and `exit`). */
    readonly body: unknown;

    constructor(code: string, status: number, message: string, body?: unknown) {
        super(message);
        this.name = 'KitRequestError';
        this.code = code;
        this.status = status;
        this.body = body;
    }
}

/** What `api(cgi, {as: 'raw'})` resolves to: the answer with its headers. */
export interface ApiRaw {
    readonly status: number;
    readonly headers: Headers;
    readonly json: unknown;
    readonly text: string;
}

export function isKitError(e: unknown): e is KitRequestError {
    return e instanceof KitRequestError;
}

export interface ApiSetup {
    /** `/addons/<www>/` */
    readonly base: string;
    /** The sid (`@xxxxxxxxxx@`) or null. Read on every request. */
    readonly sid: () => string | null;
    /** Called for a `session-invalid` answer. */
    readonly onExpired?: () => void;
    readonly fetch?: typeof fetch;
}

function encode(values: Readonly<Record<string, string | number | boolean>> | undefined, into: URLSearchParams): void {
    if (!values) {
        return;
    }
    for (const [k, v] of Object.entries(values)) {
        into.set(k, typeof v === 'boolean' ? (v ? '1' : '0') : String(v));
    }
}

/** The URL of a CGI below the addon's base, with the query and the sid. */
export function apiUrl(setup: Pick<ApiSetup, 'base' | 'sid'>, cgi: string, query?: ApiOptions['query']): string {
    const q = new URLSearchParams();
    encode(query, q);
    const sid = setup.sid();
    if (sid) {
        q.set('sid', sid);
    }
    const qs = q.toString();
    const path = /^(\/|https?:)/.test(cgi) ? cgi : setup.base + cgi.replace(/^\.\//, '');
    return path + (qs ? (path.includes('?') ? '&' : '?') + qs : '');
}

export function createApi(setup: ApiSetup) {
    const doFetch = setup.fetch ?? ((input: RequestInfo | URL, init?: RequestInit) => fetch(input, init));
    return async function api<T = unknown>(cgi: string, opts: ApiOptions = {}): Promise<T> {
        const method = opts.form ? 'POST' : (opts.method ?? 'GET');
        const headers: Record<string, string> = {Accept: opts.as === 'text' ? 'text/plain, */*' : 'application/json'};
        let body: string | undefined;
        if (method === 'POST') {
            headers[REQUEST_HEADER] = '1';
            headers['Content-Type'] = 'application/x-www-form-urlencoded';
            const f = new URLSearchParams();
            encode(opts.form, f);
            body = f.toString();
        }
        const ctrl = new AbortController();
        const timeoutMs = opts.timeoutMs ?? 30000;
        let timedOut = false;
        const timer = setTimeout(() => {
            timedOut = true;
            ctrl.abort();
        }, timeoutMs);
        const onAbort = () => ctrl.abort();
        opts.signal?.addEventListener('abort', onAbort);
        let res: Response;
        try {
            const init: RequestInit = {method, headers, credentials: 'same-origin', cache: 'no-store', signal: ctrl.signal};
            if (body !== undefined) {
                init.body = body;
            }
            res = await doFetch(apiUrl(setup, cgi, opts.query), init);
        } catch (e) {
            clearTimeout(timer);
            opts.signal?.removeEventListener('abort', onAbort);
            if (timedOut) {
                throw new KitRequestError('timeout', 0, `no answer within ${timeoutMs} ms`);
            }
            if (opts.signal?.aborted) {
                throw e;
            }
            throw new KitRequestError('network', 0, e instanceof Error ? e.message : String(e));
        }
        clearTimeout(timer);
        opts.signal?.removeEventListener('abort', onAbort);
        const type = res.headers.get('Content-Type') ?? '';
        let json: unknown;
        let text: string | undefined;
        if (type.includes('json')) {
            text = await res.text();
            try {
                json = JSON.parse(text);
            } catch {
                json = undefined;
            }
        }
        const envelope = json as {ok?: unknown; error?: {code?: unknown; message?: unknown}} | undefined;
        if (!res.ok || (envelope && envelope.ok === false)) {
            const code = typeof envelope?.error?.code === 'string' ? envelope.error.code : `http-${res.status}`;
            const message = typeof envelope?.error?.message === 'string' ? envelope.error.message : res.statusText;
            if (code === 'session-invalid') {
                setup.onExpired?.();
            } else if (code === 'csrf' || code === 'internal') {
                console.error(`ccu-addon-kit: ${cgi}: ${code}: ${message}`);
            }
            throw new KitRequestError(code, res.status, message, json);
        }
        if (opts.as === 'raw') {
            const raw: ApiRaw = {status: res.status, headers: res.headers, json, text: text ?? (await res.text())};
            return raw as unknown as T;
        }
        if (opts.as === 'text') {
            return (text ?? (await res.text())) as unknown as T;
        }
        if (json === undefined) {
            throw new KitRequestError('bad-answer', res.status, `${cgi}: the answer is not JSON`);
        }
        return json as T;
    };
}

export type Api = ReturnType<typeof createApi>;
