// The lighttpd fragment `vendor` writes from kit.json's "proxied" is the one go/addonkit/fragment
// renders: both are checked against the same fixtures in go/addonkit/fragment/testdata
// (<name>.json: the kit.json and the Go Fragment; <name>.conf: the one text).
import {describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {readdirSync, readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {lighttpdConf, normalizeKit} from '../../cli/lib/kitconfig.mjs';

const dir = join(dirname(fileURLToPath(import.meta.url)), '../../go/addonkit/fragment/testdata');
const fixtures = readdirSync(dir).filter((f) => f.endsWith('.json')).sort();

describe('the lighttpd fragment (shared with the Go side)', () => {
    it('has fixtures', () => { assert.ok(fixtures.length >= 3); });
    for (const file of fixtures) {
        it(file.replace(/\.json$/, ''), () => {
            const fx = JSON.parse(readFileSync(join(dir, file), 'utf8'));
            const want = readFileSync(join(dir, file.replace(/\.json$/, '.conf')), 'utf8');
            assert.equal(lighttpdConf(normalizeKit(fx.kit), '0.0.0-fixture'), want);
        });
    }
});
