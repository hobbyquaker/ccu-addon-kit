// The WebUI's sid in the browser (docs/session.md): read once from `?sid=`, kept in
// sessionStorage per tab, removed from the address bar; a reload reads it back.

const SID = /^@[0-9A-Za-z]{10}@$/;

function storageKey(id: string): string {
    return `kit:${id}:sid`;
}

function sessionGet(key: string): string | null {
    try {
        return sessionStorage.getItem(key);
    } catch {
        return null;
    }
}

function sessionSet(key: string, value: string | null): void {
    try {
        if (value === null) {
            sessionStorage.removeItem(key);
        } else {
            sessionStorage.setItem(key, value);
        }
    } catch {
        // a page with blocked site data keeps the sid in memory only
    }
}

/**
 * Takes the sid from the address (and removes it there), else from sessionStorage. Accepts the
 * WebUI's `@xxxxxxxxxx@` and the bare ten characters.
 */
export function takeSid(id: string, win: Window = window): string | null {
    const url = new URL(win.location.href);
    const fromUrl = url.searchParams.get('sid');
    if (fromUrl !== null) {
        url.searchParams.delete('sid');
        win.history.replaceState(win.history.state, '', url.pathname + url.search + url.hash);
        const sid = /^[0-9A-Za-z]{10}$/.test(fromUrl) ? `@${fromUrl}@` : fromUrl;
        if (SID.test(sid)) {
            sessionSet(storageKey(id), sid);
            return sid;
        }
    }
    const stored = sessionGet(storageKey(id));
    return stored !== null && SID.test(stored) ? stored : null;
}

/** The bare ten characters for `Session.renew`. */
export function bareSid(sid: string): string {
    return sid.replace(/^@|@$/g, '');
}
