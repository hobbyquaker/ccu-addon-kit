// mountFrame(): the entry point of an addon's settings page (docs/tab-contract.md).

import {mount, unmount} from 'svelte';
import {addMessages, initI18n, t, type Catalogue} from 'occulite-ui/i18n';
import {initTheme} from 'occulite-ui/theme';

import Frame from './Frame.svelte';
import {KitState} from './kit.svelte.js';
import {FRAME_MESSAGES} from './messages.js';
import {takeSid} from './sid.js';
import type {AddonConfig, KitTab} from './types.js';

/** An addon's strings: `{de: {key: text}, en: {key: text}}`, or a catalogue `{key: {de, en}}`. */
export type AddonStrings = Readonly<Record<'de' | 'en', Readonly<Record<string, string>>>> | Catalogue;

export interface MountOptions {
    readonly tabs?: readonly KitTab[];
    readonly strings?: AddonStrings;
    /** Where kit.json's frame view is; `frame/addon.json` next to settings.cgi by default. */
    readonly configUrl?: string;
}

/**
 * The tab list, written so the build can read the ids: `kitVite()` collects the `id` string
 * literals inside `defineTabs([...])` into `frame-manifest.json`, which `vendor` checks
 * kit.json's `tabs` against. Returns its argument.
 */
export function defineTabs<const T extends readonly KitTab[]>(tabs: T): T {
    return tabs;
}

function isPerLanguage(s: AddonStrings): s is Readonly<Record<'de' | 'en', Readonly<Record<string, string>>>> {
    const keys = Object.keys(s);
    return keys.length > 0 && keys.every((k) => k === 'de' || k === 'en') && Object.values(s).every((v) => typeof v === 'object' && v !== null && !('de' in v || 'en' in v));
}

/** `{de: {k: a}, en: {k: b}}` -> `{k: {de: a, en: b}}` */
export function toCatalogue(s: AddonStrings): Catalogue {
    if (!isPerLanguage(s)) return s as Catalogue;
    const out: Record<string, Record<string, string>> = {};
    for (const lang of ['de', 'en'] as const) {
        for (const [key, text] of Object.entries(s[lang] ?? {})) {
            if (key.startsWith('kit.')) {
                console.warn(`ccu-addon-kit: the addon's string "${key}" uses the frame's prefix "kit."`);
            }
            (out[key] ??= {})[lang] = text;
        }
    }
    return out;
}

function framedBySameOrigin(win: Window): boolean {
    try {
        return win.self !== win.top && win.parent.location.origin === win.location.origin;
    } catch {
        return false;
    }
}

function showPlain(el: HTMLElement, text: string, testId: string): void {
    el.textContent = '';
    const p = document.createElement('p');
    p.className = 'kit-boot';
    p.dataset['testid'] = testId;
    p.textContent = text;
    el.appendChild(p);
}

/**
 * Mounts the frame into `el`: reads frame/addon.json, takes the sid from the address, sets up the
 * theme and the language (following openccu-lite's shell inside it, the browser and the page's
 * own toggles on a CCU), and renders the header, the tabs and the frame's own tabs.
 */
export async function mountFrame(el: HTMLElement, options: MountOptions = {}): Promise<{kit: KitState; destroy: () => void}> {
    addMessages(FRAME_MESSAGES);
    if (options.strings) addMessages(toCatalogue(options.strings));
    // inside openccu-lite's shell the page follows it; anywhere else (a CCU's WebUI opens the page
    // in a tab of its own) the browser's preference and the page's own toggles decide
    const follow = framedBySameOrigin(window) ? 'shell' : 'none';
    initTheme({storageKey: 'kit:theme', follow});
    initI18n({storageKey: 'kit:lang', follow});

    showPlain(el, t('kit.loading'), 'kit-boot');
    let config: AddonConfig;
    try {
        const r = await fetch(options.configUrl ?? new URL('frame/addon.json', location.href).pathname, {cache: 'no-store', credentials: 'same-origin'});
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        config = (await r.json()) as AddonConfig;
        if (config.schema !== 1 || typeof config.id !== 'string') throw new Error('not a kit addon.json');
    } catch (e) {
        console.error('ccu-addon-kit: frame/addon.json:', e);
        showPlain(el, t('kit.noConfig'), 'kit-noconfig');
        throw e;
    }

    const kit = new KitState({config, sid: takeSid(config.id)});
    el.textContent = '';
    const app = mount(Frame, {target: el, props: {kit, tabs: options.tabs ?? []}});
    return {
        kit,
        destroy: () => {
            void unmount(app);
        },
    };
}
