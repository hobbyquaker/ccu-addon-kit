// kit.json: validation, defaults, and the two generated files (lib/kit/addon.tcl for the
// CGIs, www/frame/addon.json for the frame). schema/kit.schema.json describes the same rules
// for editors; this is the check the vendor step relies on (no dependencies).

export const KIT_TABS = ['kit:service', 'kit:log', 'kit:about'];
export const COMMANDS = ['start', 'stop', 'restart', 'reload'];
export const CLASSES = ['read', 'service', 'log', 'write'];
export const LEVELS = ['any', 'user', 'admin'];
// the opt-in levels table (docs/session.md): reading for user, everything else for admin
export const LEVELS_PRESET = {read: 'user', service: 'admin', log: 'admin', write: 'admin'};

const ID = /^[a-z0-9][a-z0-9-]{0,31}$/;
const TAB = /^[a-z0-9][a-z0-9-]*$/;
const KEYS = ['$schema', 'schema', 'id', 'name', 'icon', 'addonDir', 'rc', 'versionFile', 'service', 'logs',
    'update', 'permissions', 'tabs', 'proxied', 'links'];

function isObj(v) { return v !== null && typeof v === 'object' && !Array.isArray(v); }
function isI18n(v) { return isObj(v) && typeof v.de === 'string' && typeof v.en === 'string' && v.de && v.en; }
function absPath(v) { return typeof v === 'string' && /^\/[^\0\n\r]*$/.test(v) && !v.split('/').includes('..'); }

export class KitConfigError extends Error {
    constructor(errors) {
        super(`kit.json is not valid:\n  - ${errors.join('\n  - ')}`);
        this.errors = errors;
    }
}

// -> the normalized configuration; throws KitConfigError with every problem found
export function normalizeKit(raw) {
    const e = [];
    if (!isObj(raw)) { throw new KitConfigError(['the file must hold one JSON object']); }
    for (const k of Object.keys(raw)) { if (!KEYS.includes(k)) { e.push(`unknown key "${k}"`); } }
    if (raw.schema !== 1) { e.push('"schema" must be 1'); }
    const id = raw.id;
    if (typeof id !== 'string' || !ID.test(id)) { e.push('"id" must match [a-z0-9][a-z0-9-]{0,31}'); }
    if (!isI18n(raw.name)) { e.push('"name" must be {"de": "...", "en": "..."}'); }
    if (raw.icon !== undefined && (typeof raw.icon !== 'string' || !/^[A-Za-z0-9._-]+(\/[A-Za-z0-9._-]+)*$/.test(raw.icon) || raw.icon.includes('..'))) {
        e.push('"icon" must be a relative path inside www/');
    }
    const addonDir = raw.addonDir ?? `/usr/local/addons/${id}`;
    const rc = raw.rc ?? `/usr/local/etc/config/rc.d/${id}`;
    const versionFile = raw.versionFile ?? `${addonDir}/versions`;
    for (const [k, v] of [['addonDir', addonDir], ['rc', rc], ['versionFile', versionFile]]) {
        if (!absPath(v)) { e.push(`"${k}" must be an absolute path`); }
    }

    const service = {commands: [...COMMANDS], pollSeconds: 10};
    if (raw.service !== undefined) {
        if (!isObj(raw.service)) { e.push('"service" must be an object'); } else {
            for (const k of Object.keys(raw.service)) { if (!['commands', 'pollSeconds'].includes(k)) { e.push(`unknown key "service.${k}"`); } }
            if (raw.service.commands !== undefined) {
                const c = raw.service.commands;
                if (!Array.isArray(c) || c.some((x) => !COMMANDS.includes(x)) || new Set(c).size !== c.length) {
                    e.push(`"service.commands" must be a list of distinct ${COMMANDS.join('|')}`);
                } else { service.commands = COMMANDS.filter((x) => c.includes(x)); }
            }
            if (raw.service.pollSeconds !== undefined) {
                const p = raw.service.pollSeconds;
                if (!Number.isInteger(p) || p < 2 || p > 300) { e.push('"service.pollSeconds" must be an integer 2..300'); } else { service.pollSeconds = p; }
            }
        }
    }

    let logs = [{id: 'syslog', label: {de: 'Systemprotokoll', en: 'System log'}, kind: 'syslog', tag: id}];
    if (raw.logs !== undefined) {
        if (!Array.isArray(raw.logs)) { e.push('"logs" must be a list'); } else {
            logs = [];
            const seen = new Set();
            raw.logs.forEach((l, i) => {
                const at = `logs[${i}]`;
                if (!isObj(l)) { e.push(`${at} must be an object`); return; }
                for (const k of Object.keys(l)) { if (!['id', 'label', 'file', 'rotated', 'syslogTag'].includes(k)) { e.push(`unknown key "${at}.${k}"`); } }
                if (typeof l.id !== 'string' || !TAB.test(l.id)) { e.push(`${at}.id must match [a-z0-9][a-z0-9-]*`); return; }
                if (seen.has(l.id)) { e.push(`${at}.id "${l.id}" is used twice`); }
                seen.add(l.id);
                if (l.label !== undefined && !isI18n(l.label)) { e.push(`${at}.label must be {"de","en"}`); }
                const label = l.label ?? {de: l.id, en: l.id};
                if ((l.file === undefined) === (l.syslogTag === undefined)) { e.push(`${at} needs exactly one of "file" and "syslogTag"`); return; }
                if (l.file !== undefined) {
                    if (!absPath(l.file)) { e.push(`${at}.file must be an absolute path`); }
                    if (l.rotated !== undefined && !absPath(l.rotated)) { e.push(`${at}.rotated must be an absolute path`); }
                    logs.push({id: l.id, label, kind: 'file', file: l.file, ...(l.rotated ? {rotated: l.rotated} : {})});
                } else {
                    if (l.rotated !== undefined) { e.push(`${at}.rotated belongs to a file source`); }
                    if (typeof l.syslogTag !== 'string' || !/^[A-Za-z0-9._-]{1,32}$/.test(l.syslogTag)) { e.push(`${at}.syslogTag must be 1-32 of [A-Za-z0-9._-]`); }
                    logs.push({id: l.id, label, kind: 'syslog', tag: l.syslogTag});
                }
            });
        }
    }

    let update = null;
    if (raw.update !== undefined) {
        if (!isObj(raw.update)) { e.push('"update" must be an object'); } else {
            for (const k of Object.keys(raw.update)) { if (!['github', 'assetHint'].includes(k)) { e.push(`unknown key "update.${k}"`); } }
            if (typeof raw.update.github !== 'string' || !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(raw.update.github)) {
                e.push('"update.github" must be "owner/repo"');
            }
            if (raw.update.assetHint !== undefined && typeof raw.update.assetHint !== 'string') { e.push('"update.assetHint" must be a string'); }
            update = {github: raw.update.github, ...(raw.update.assetHint ? {assetHint: raw.update.assetHint} : {})};
        }
    }

    // default: any valid WebUI session may do everything (today's behaviour on a CCU);
    // levels only when the addon opts in
    let permissions = {read: 'any', service: 'any', log: 'any', write: 'any'};
    if (raw.permissions !== undefined) {
        if (raw.permissions === 'levels') { permissions = {...LEVELS_PRESET}; } else if (!isObj(raw.permissions)) {
            e.push('"permissions" must be "levels" or an object');
        } else {
            for (const [k, v] of Object.entries(raw.permissions)) {
                if (!CLASSES.includes(k)) { e.push(`unknown key "permissions.${k}"`); } else if (!LEVELS.includes(v)) {
                    e.push(`"permissions.${k}" must be one of ${LEVELS.join('|')}`);
                } else { permissions[k] = v; }
            }
        }
    }

    let tabs = [];
    if (!Array.isArray(raw.tabs) || raw.tabs.length === 0) { e.push('"tabs" must be a non-empty list'); } else {
        tabs = raw.tabs;
        for (const t of tabs) {
            if (typeof t !== 'string' || !(KIT_TABS.includes(t) || (TAB.test(t) && !t.startsWith('kit')))) {
                e.push(`tab "${t}" is neither one of ${KIT_TABS.join(', ')} nor an addon tab id ([a-z0-9-]+, not starting with "kit")`);
            }
        }
        if (new Set(tabs).size !== tabs.length) { e.push('"tabs" lists a tab twice'); }
    }

    let proxied = null;
    if (raw.proxied !== undefined) {
        if (!isObj(raw.proxied)) { e.push('"proxied" must be an object'); } else {
            for (const k of Object.keys(raw.proxied)) { if (!['path', 'errorPage'].includes(k)) { e.push(`unknown key "proxied.${k}"`); } }
            if (typeof raw.proxied.path !== 'string' || !raw.proxied.path.startsWith(`/addons/${id}/`)) {
                e.push(`"proxied.path" must start with /addons/${id}/`);
            }
            if (raw.proxied.errorPage !== undefined && typeof raw.proxied.errorPage !== 'boolean') { e.push('"proxied.errorPage" must be true or false'); }
            proxied = {path: raw.proxied.path, errorPage: raw.proxied.errorPage === true};
        }
    }

    let links = [];
    if (raw.links !== undefined) {
        if (!Array.isArray(raw.links) || raw.links.some((l) => !isObj(l) || !isI18n(l.label) || typeof l.url !== 'string' || !/^https?:\/\//.test(l.url))) {
            e.push('"links" must be a list of {"label": {"de","en"}, "url": "https://..."}');
        } else { links = raw.links.map((l) => ({label: l.label, url: l.url})); }
    }

    if (e.length) { throw new KitConfigError(e); }
    return {schema: 1, id, name: raw.name, icon: raw.icon ?? null, addonDir, rc, versionFile, service, logs, update,
        permissions, tabs, proxied, links};
}

// --- Tcl ---------------------------------------------------------------------------------

// One Tcl word that reads back as exactly `s`, in a script and as a list element, on Tcl 8.2:
// every special character backslash-escaped, everything outside ASCII as \uXXXX (a CCU3 sources
// files in its system encoding, iso8859-1, so raw UTF-8 would arrive garbled).
export function tclWord(s) {
    s = String(s);
    if (s === '') { return '{}'; }
    let out = '';
    for (const ch of s) {
        const c = ch.codePointAt(0);
        if (c > 0xffff) { throw new Error(`character U+${c.toString(16)} cannot be written for Tcl 8.2`); }
        if (c > 126 || c < 32) {
            if (ch === '\n') { out += '\\n'; } else if (ch === '\t') { out += '\\t'; } else { out += `\\u${c.toString(16).padStart(4, '0')}`; }
        } else if ('\\{}[]$";# '.includes(ch)) {
            out += `\\${ch}`;
        } else { out += ch; }
    }
    return out;
}

export function tclList(items) { return items.map(tclWord).join(' '); }

function i18nList(o) { return tclList(['de', o.de, 'en', o.en]); }

export function addonTcl(kit, kitVersion) {
    const lines = [
        `# lib/kit/addon.tcl - generated from kit.json by ccu-addon-kit ${kitVersion} (MIT); do not edit`,
        `set KIT(id) ${tclWord(kit.id)}`,
        `set KIT(name) ${tclWord(i18nList(kit.name))}`,
        `set KIT(addonDir) ${tclWord(kit.addonDir)}`,
        `set KIT(rc) ${tclWord(kit.rc)}`,
        `set KIT(versionFile) ${tclWord(kit.versionFile)}`,
        `set KIT(commands) ${tclWord(tclList(kit.service.commands))}`,
        `set KIT(pollSeconds) ${kit.service.pollSeconds}`,
        `set KIT(logs) ${tclWord(tclList(kit.logs.map((l) => {
            const kv = ['id', l.id, 'label', i18nList(l.label), 'kind', l.kind];
            if (l.kind === 'file') { kv.push('file', l.file); if (l.rotated) { kv.push('rotated', l.rotated); } } else { kv.push('tag', l.tag); }
            return tclList(kv);
        })))}`,
        `set KIT(github) ${tclWord(kit.update ? kit.update.github : '')}`,
        ...['read', 'service', 'log', 'write'].map((c) => `set KIT(perm,${c}) ${kit.permissions[c]}`),
        `set KIT(tabs) ${tclWord(tclList(kit.tabs))}`,
        `set KIT(proxiedPath) ${tclWord(kit.proxied ? kit.proxied.path : '')}`,
        '',
    ];
    return lines.join('\n');
}

// the frame's view of kit.json: public, so no paths
export function addonJson(kit, kitVersion) {
    return `${JSON.stringify({
        schema: 1,
        id: kit.id,
        name: kit.name,
        icon: kit.icon,
        kit: kitVersion,
        service: kit.service,
        logs: kit.logs.map((l) => ({id: l.id, label: l.label, kind: l.kind})),
        update: kit.update ? {github: kit.update.github} : null,
        permissions: kit.permissions,
        tabs: kit.tabs,
        proxied: kit.proxied ? {path: kit.proxied.path} : null,
        links: kit.links,
    }, null, 2)}\n`;
}
