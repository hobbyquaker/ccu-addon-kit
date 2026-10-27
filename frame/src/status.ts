// The status view mapping (docs/service-convention.md, "What the frame shows") and the numbers
// the header and the Service tab print. Pure: unit-tested without a browser.

import type {ServiceCommand, ServiceStatus, ServiceView} from './types.js';

/** What the pill shows for a status line and its exit code. */
export function statusView(s: ServiceStatus | null, pending: ServiceCommand | null = null): ServiceView {
    if (pending !== null) {
        return 'busy';
    }
    if (!s || typeof s.running !== 'boolean') {
        return 'unknown';
    }
    if (s.state === 'starting' || s.state === 'stopping') {
        return 'busy';
    }
    if (s.running) {
        return 'running';
    }
    if (s.state === 'failed') {
        return 'failed';
    }
    if (s.exit === 4) {
        return 'unknown';
    }
    // schema 0: the kit's own wrapper around a line that was not JSON, running from the exit code;
    // the convention's transition rule: exit 1 with running false and no state is stopped
    if (s.schema === 0) {
        return s.exit === 3 || s.exit === 1 || s.exit === 0 ? 'stopped' : 'unknown';
    }
    return s.schema === 1 ? 'stopped' : 'unknown';
}

/** The buttons a view enables, before the command list and the level are applied. */
export const ENABLED: Readonly<Record<ServiceView, readonly ServiceCommand[]>> = {
    running: ['stop', 'restart', 'reload'],
    stopped: ['start'],
    failed: ['start', 'restart'],
    busy: [],
    unknown: ['start', 'stop', 'restart'],
};

/** The busy label: what is in flight, else what the script says it is doing. */
export function busyKind(s: ServiceStatus | null, pending: ServiceCommand | null): 'starting' | 'stopping' | 'restarting' | 'reloading' | 'wait' {
    if (pending === 'start') return 'starting';
    if (pending === 'stop') return 'stopping';
    if (pending === 'restart') return 'restarting';
    if (pending === 'reload') return 'reloading';
    if (s?.state === 'starting') return 'starting';
    if (s?.state === 'stopping') return 'stopping';
    return 'wait';
}

/** Parses a service.cgi status answer: the body (verbatim line or schema 0) and the headers. */
export function parseStatus(json: unknown, headers: Headers): ServiceStatus | null {
    const num = (name: string): number | null => {
        const v = headers.get(name);
        if (v === null || v === '' || !/^-?\d+$/.test(v)) return null;
        return Number(v);
    };
    const extra = {exit: num('X-Kit-Exit'), memTotalKb: num('X-Kit-Mem-Total-Kb'), memAvailableKb: num('X-Kit-Mem-Available-Kb')};
    if (json === null || typeof json !== 'object' || Array.isArray(json)) {
        return null;
    }
    const o = json as Record<string, unknown>;
    if (typeof o['running'] !== 'boolean' || typeof o['schema'] !== 'number') {
        return null;
    }
    return {...(o as unknown as ServiceStatus), ...extra};
}

/** `48 MB`, `9.6 MB` (one decimal under 10 MB), `1.2 GB`, in the page's language. */
export function formatKb(kb: number, lang: string): string {
    const mb = kb / 1024;
    if (mb >= 1024) {
        return `${(mb / 1024).toLocaleString(lang, {maximumFractionDigits: 1})} GB`;
    }
    return `${mb.toLocaleString(lang, {maximumFractionDigits: mb < 10 ? 1 : 0, minimumFractionDigits: mb < 10 ? 1 : 0})} MB`;
}

/** `3 h 12 min`, `2 d 4 h`, `45 s`. */
export function formatDuration(seconds: number): string {
    const s = Math.max(0, Math.floor(seconds));
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) return `${d} d ${h} h`;
    if (h > 0) return `${h} h ${m} min`;
    if (m > 0) return `${m} min`;
    return `${s} s`;
}
