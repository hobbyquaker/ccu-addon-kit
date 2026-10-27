#!/usr/bin/env node
// The Tcl 8.2 lint (MIT): greps the kit's Tcl and the example's CGIs for constructs newer than
// the CCU3 firmware's Tcl 8.2.3 (the CCU addon handbook, chapter 04) and for non-ASCII bytes
// (a CCU3 sources files as iso8859-1). It is the cheap guard; the tests under the self-built
// Tcl 8.2.3 (job tcl82) are the real one.
//
//   node test/tcl/lint-tcl82.mjs [files...]     default: classic/tcl/**/*.tcl, the CGI stub,
//                                               example/*/addon_files/**/*.cgi, classic/sh/update_addon

import {readFileSync, readdirSync, statSync} from 'node:fs';
import {join, relative} from 'node:path';
import {fileURLToPath} from 'node:url';

const ROOT = join(fileURLToPath(import.meta.url), '../../..');

const RULES = [
    [/\bdict\s/, 'dict (8.5)'],
    [/\{\*\}/, '{*} (8.5)'],
    [/\blassign\b/, 'lassign (8.5)'],
    [/\blrepeat\b/, 'lrepeat (8.5)'],
    [/\blset\b/, 'lset (8.4)'],
    [/\bapply\b/, 'apply (8.5)'],
    [/^\s*try\b|\[try\b/, 'try (8.6)'],
    [/\bstring is\b/, 'string is (8.4)'],
    [/\bfile normalize\b/, 'file normalize (8.4)'],
    [/\bglob\s.*-(directory|types|tails|path)\b/, 'glob -directory/-types/-tails/-path (8.3)'],
    [/2>@/, 'exec 2>@ (use kit_run)'],
    [/\barray unset\b/, 'array unset (8.3)'],
    [/\bstring match\s+-nocase\b/, 'string match -nocase (8.3)'],
    [/\bregexp\s+(-\w+\s+)*-(all|inline|start|line|lineanchor|linestop|expanded)\b/, 'regexp -all/-inline/-start/-line/-expanded (8.3)'],
    [/\bregsub\s+(-\w+\s+)*-(start|line|expanded)\b/, 'regsub -start/-line/-expanded (8.3)'],
    [/\bclock\s+(clicks\s+-milli|milliseconds|microseconds)/, 'clock milliseconds (8.3+)'],
    [/\breturn\s+-(level|options)\b/, 'return -level/-options (8.5)'],
    [/\bcatch\s+\{[^}]*\}\s+\S+\s+\S+\s*\]/, 'catch with an options variable (8.5)'],
    [/\*\*/, '** (8.5)'],
    [/\bwide\(|\bentier\(/, 'wide()/entier() (8.4/8.5)'],
    [/\b(expr|if|elseif|while)\s*\{[^}]*(?<![$\w])(eq|ne|in|ni)\s/, 'eq/ne/in/ni in an expression (8.4/8.5)'],
    [/\blsearch\s+(-\w+\s+)*-(start|all|inline|index|subindices|nocase|bisect)\b/, 'lsearch -start/-all/-inline/-index (8.4+)'],
    [/\blsort\s+(-\w+\s+)*-(unique|nocase|indices|stride)\b/, 'lsort -unique/-nocase/-indices (8.3+)'],
    [/\bstring\s+(index|range)\s+\S+\s+\S*end-/, 'end-N string indices (verify on 8.2; use expr)'],
    [/\bbinary\s+(encode|decode)\b/, 'binary encode/decode (8.6)'],
    [/\bchan\s/, 'chan (8.5)'],
    [/\bzlib\b/, 'zlib (8.6)'],
    [/\bcoroutine\b|\byield\b/, 'coroutines (8.6)'],
    [/\btailcall\b/, 'tailcall (8.6)'],
    [/\bdict\b/, 'dict (8.5)'],
    [/\bstring\s+(cat|reverse|totitle\s|is\s)/, 'string cat/reverse/is (8.4+)'],
    [/\bfile\s+(tempfile|link)\b/, 'file tempfile/link (8.4+)'],
    [/\binfo\s+(coroutine|frame)\b/, 'info coroutine/frame (8.5+)'],
];

function walk(dir, pred) {
    const out = [];
    for (const name of readdirSync(dir)) {
        const p = join(dir, name);
        if (name === 'node_modules' || name === 'dist') { continue; }
        if (statSync(p).isDirectory()) { out.push(...walk(p, pred)); } else if (pred(p)) { out.push(p); }
    }
    return out;
}

// the words of every [cmd ...] on the line, nesting of [] {} "" respected
function commands(line, name) {
    const found = [];
    let at = 0;
    for (;;) {
        const i = line.indexOf(`[${name}`, at);
        if (i < 0) { return found; }
        at = i + 1;
        if (!/\s/.test(line[i + 1 + name.length] || '')) { continue; }
        const words = [];
        let cur = '';
        let depth = 0;
        let quote = false;
        let j = i + 1 + name.length;
        for (; j < line.length; j++) {
            const c = line[j];
            if (c === '\\') { cur += c + (line[j + 1] || ''); j++; continue; }
            if (depth === 0 && !quote && c === ']') { break; }
            if (c === '[' || c === '{') { depth++; } else if ((c === ']' || c === '}') && depth > 0) { depth--; } else if (c === '"' && depth === 0) { quote = !quote; }
            if (depth === 0 && !quote && /\s/.test(c)) { if (cur) { words.push(cur); } cur = ''; } else { cur += c; }
        }
        if (cur) { words.push(cur); }
        found.push(words.filter((w) => !/^-(nocase|all|expanded|line|start|--)$/.test(w) && w !== '--'));
    }
}

function nested(line) {
    const out = [];
    for (const [name, max, what] of [['lindex', 2, 'lindex with several indices (8.4)'], ['regsub', 3, 'regsub returning the value (8.4; pass a variable)'],
        ['scan', 2, 'scan returning the value (8.4+; pass a variable)']]) {
        for (const words of commands(line, name)) {
            if (words.length > max && name === 'lindex') { out.push(what); }
            if (words.length <= max && name !== 'lindex') { out.push(what); }
        }
    }
    return out;
}

export function lint(files) {
    const problems = [];
    for (const file of files) {
        const text = readFileSync(file);
        const rel = relative(ROOT, file);
        text.toString('latin1').split('\n').forEach((line, i) => {
            if (/[^\x00-\x7f]/.test(line)) { problems.push(`${rel}:${i + 1}: non-ASCII byte (a CCU3 sources Tcl as iso8859-1; use \\uXXXX)`); }
            const code = line.replace(/^\s*#.*$/, '').replace(/;\s*#.*$/, '');
            for (const [re, what] of RULES) {
                if (re.test(code)) { problems.push(`${rel}:${i + 1}: ${what}: ${line.trim()}`); break; }
            }
            for (const what of nested(code)) { problems.push(`${rel}:${i + 1}: ${what}: ${line.trim()}`); }
        });
    }
    return problems;
}

export function defaultFiles() {
    return [
        ...walk(join(ROOT, 'classic/tcl'), (p) => p.endsWith('.tcl')),
        join(ROOT, 'classic/cgi/stub.cgi.in'),
        join(ROOT, 'classic/sh/update_addon'),
        ...walk(join(ROOT, 'example'), (p) => p.endsWith('.cgi')),
    ];
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
    const files = process.argv.length > 2 ? process.argv.slice(2) : defaultFiles();
    const problems = lint(files);
    for (const p of problems) { console.log(p); }
    console.log(problems.length ? `lint-tcl82: ${problems.length} problems in ${files.length} files` : `lint-tcl82: ${files.length} files clean`);
    process.exitCode = problems.length ? 1 : 0;
}
