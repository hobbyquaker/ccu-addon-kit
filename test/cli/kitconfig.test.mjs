import {describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {KitConfigError, addonJson, addonTcl, normalizeKit, tclWord} from '../../cli/lib/kitconfig.mjs';
import {KIT_ROOT} from '../../cli/lib/kitroot.mjs';

const MIN = {schema: 1, id: 'battery', name: {de: 'Batterien', en: 'Batteries'}, tabs: ['kit:service']};

describe('kit.json', () => {
    it('fills the defaults, with any valid session allowed everything', () => {
        const k = normalizeKit(MIN);
        assert.equal(k.addonDir, '/usr/local/addons/battery');
        assert.equal(k.rc, '/usr/local/etc/config/rc.d/battery');
        assert.equal(k.versionFile, '/usr/local/addons/battery/versions');
        assert.deepEqual(k.service, {commands: ['start', 'stop', 'restart', 'reload'], pollSeconds: 10});
        assert.deepEqual(k.logs, [{id: 'syslog', label: {de: 'Systemprotokoll', en: 'System log'}, kind: 'syslog', tag: 'battery'}]);
        assert.deepEqual(k.permissions, {read: 'any', service: 'any', log: 'any', write: 'any'});
        assert.equal(k.update, null);
    });

    it('takes the levels as an opt-in, as a preset or per class', () => {
        assert.deepEqual(normalizeKit({...MIN, permissions: 'levels'}).permissions, {read: 'user', service: 'admin', log: 'admin', write: 'admin'});
        assert.deepEqual(normalizeKit({...MIN, permissions: {write: 'admin'}}).permissions, {read: 'any', service: 'any', log: 'any', write: 'admin'});
    });

    it('accepts the example addon and the spec example', () => {
        normalizeKit(JSON.parse(readFileSync(join(KIT_ROOT, 'example/kitdemo/kit.json'), 'utf8')));
        normalizeKit({...MIN, icon: 'icon.svg', logs: [{id: 'main', label: {de: 'Dienst', en: 'Service'}, file: '/var/log/battery.log', rotated: '/var/log/battery.log.0'},
            {id: 'syslog', syslogTag: 'battery'}], update: {github: 'hobbyquaker/ccu-addon-battery', assetHint: 'x-{version}.tgz'},
            permissions: {read: 'user', service: 'admin', log: 'admin', write: 'admin'},
            tabs: ['overview', 'devices', 'kit:service', 'kit:log', 'kit:about'], proxied: {path: '/addons/battery/app/', errorPage: true}});
    });

    it('lists every problem at once', () => {
        const bad = {schema: 2, id: 'Bad Id', name: {de: 'x'}, extra: 1, service: {commands: ['start', 'boot'], pollSeconds: 1},
            logs: [{id: 'a', file: 'relative.log'}, {id: 'a', syslogTag: 'x', file: '/x'}], permissions: {read: 'root', admin: 'any'},
            tabs: ['kit:nope', 'kitchen', 'x', 'x'], proxied: {path: '/elsewhere/'}, update: {github: 'nope'}, addonDir: '/usr/../etc'};
        try {
            normalizeKit(bad);
            assert.fail('no error');
        } catch (e) {
            assert.ok(e instanceof KitConfigError);
            for (const part of ['unknown key "extra"', '"schema" must be 1', '"id" must match', '"name" must be', 'service.commands', 'pollSeconds',
                'logs[0].file', 'logs[1].id "a" is used twice', 'exactly one of', '"permissions.read"', 'unknown key "permissions.admin"',
                'tab "kit:nope"', 'tab "kitchen"', 'lists a tab twice', 'proxied.path', 'update.github', '"addonDir" must be an absolute path']) {
                assert.ok(e.errors.some((m) => m.includes(part)), `${part} in\n${e.errors.join('\n')}`);
            }
        }
    });
});

describe('Tcl generation', () => {
    it('quotes every special character and writes non-ASCII as \\u', () => {
        assert.equal(tclWord(''), '{}');
        assert.equal(tclWord('a b'), 'a\\ b');
        assert.equal(tclWord('[exec id] $x {y} "q" \\ ;#'), '\\[exec\\ id\\]\\ \\$x\\ \\{y\\}\\ \\"q\\"\\ \\\\\\ \\;\\#');
        assert.equal(tclWord('Ä€\n'), '\\u00c4\\u20ac\\n');
    });

    it('writes addon.tcl in ASCII and addon.json without paths', () => {
        const k = normalizeKit({...MIN, name: {de: 'Batterien „Ä“', en: 'Batteries'}, logs: [{id: 'main', file: '/var/log/b.log'}]});
        const tcl = addonTcl(k, '0.1.0');
        assert.equal(/[^\x00-\x7f]/.test(tcl), false);
        assert.match(tcl, /^set KIT\(perm,write\) any$/m);
        const json = JSON.parse(addonJson(k, '0.1.0'));
        assert.deepEqual(json.logs, [{id: 'main', label: {de: 'main', en: 'main'}, kind: 'file'}]);
        assert.equal(JSON.stringify(json).includes('/var/log'), false);
        assert.equal(json.kit, '0.1.0');
    });
});

describe('schema/kit.schema.json', () => {
    it('is valid JSON and names the same top-level keys', () => {
        const schema = JSON.parse(readFileSync(join(KIT_ROOT, 'schema/kit.schema.json'), 'utf8'));
        assert.deepEqual(Object.keys(schema.properties).sort(), ['$schema', 'addonDir', 'icon', 'id', 'links', 'logs', 'name', 'permissions', 'proxied', 'rc',
            'schema', 'service', 'tabs', 'update', 'versionFile']);
        JSON.parse(readFileSync(join(KIT_ROOT, 'schema/service-status.schema.json'), 'utf8'));
    });
});
