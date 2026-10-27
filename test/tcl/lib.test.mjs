// json.tcl, form.tcl and the version comparison, as Tcl snippets.
import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {setup} from './harness.mjs';

describe('the Tcl library', () => {
    let t;
    before(async () => { t = await setup(); });
    after(() => t.close());

    it('kit_json_string escapes every control character', async () => {
        const out = await t.tcl(`
            set s ""
            for {set i 0} {$i < 128} {incr i} { append s [format %c $i] }
            append s "\\u00e4\\u20ac\\u2028"
            fconfigure stdout -encoding utf-8
            puts -nonewline [kit_json_string $s]`);
        let expected = '';
        for (let i = 0; i < 128; i++) { expected += String.fromCharCode(i); }
        expected += 'ä€\u2028';
        assert.equal(JSON.parse(out), expected);
        assert.equal(/[\x00-\x1f]/.test(out), false, 'no raw control character in the JSON text');
    });

    it('kit_json_obj writes the five types', async () => {
        const out = await t.tcl(`puts -nonewline [kit_json_obj {name s Alice count n 3 neg n -0.5 bad n 1e enabled b 1 off b 0 data j {[1,2]} note z "" note2 z x lead n 007}]`);
        assert.deepEqual(JSON.parse(out), {name: 'Alice', count: 3, neg: -0.5, bad: null, enabled: true, off: false, data: [1, 2], note: null, note2: 'x', lead: 7});
    });

    it('kit_urldecode decodes without subst', async () => {
        const out = await t.tcl(`
            fconfigure stdout -encoding utf-8
            puts [kit_urldecode {%40Adm1nSess1%40}]
            puts [kit_urldecode {a+b%2Bc}]
            puts [kit_urldecode {%C3%A4%E2%82%AC}]
            puts [kit_urldecode {[exec id]$env(PATH)}]
            puts [kit_urldecode {%zz%4}]`);
        assert.equal(out, '@Adm1nSess1@\na b+c\nä€\n[exec id]$env(PATH)\n%zz%4\n');
    });

    it('kit_parse_pairs keeps names and empty values', async () => {
        const out = await t.tcl(`kit_parse_pairs {a=1&b=&c&=x&a=2&sid=%40x%40} arr; foreach k [lsort [array names arr]] { puts "$k=$arr($k)" }`);
        assert.equal(out, '=x\na=2\nb=\nc=\nsid=@x@\n');
    });

    it('kit_version_compare orders releases, builds and prereleases', async () => {
        const pairs = [
            ['0.5.0', '0.4.0', 1], ['v0.4.0', '0.4.0', 0], ['0.10.0', '0.9.9', 1], ['1.0.0', '1.0.0-rc.3', 1],
            ['1.0.0-rc.10', '1.0.0-rc.9', 1], ['1.0.0-dev.2', '1.0.0-rc.1', -1], ['1.0.0-rc.1', '1.0.0-rc', 1],
            ['2.1.2+1', '2.1.2+0', 1], ['2.1.2+0', '2.1.2', 0], ['2.1.2', 'garbage', 1], ['garbage', 'junk', 0],
            ['1.0.0-alpha', '1.0.0-1', 1], ['3.89.8', '3.89.11', -1],
        ];
        const script = pairs.map(([a, b]) => `puts [kit_version_compare {${a}} {${b}}]`).join('\n');
        const out = (await t.tcl(script)).trim().split('\n').map(Number);
        pairs.forEach(([a, b, want], i) => assert.equal(out[i], want, `${a} vs ${b}`));
    });

    it('kit_versions reads export, quotes and comments', async () => {
        const out = await t.tcl('puts [kit_versions]');
        assert.equal(out.trim(), 'VERSION_ADDON 0.4.0 NODE_VERSION 22.20.0 VERSION_MOSQUITTO 2.1.2');
    });

    it('generated addon.tcl reads back the names, unicode included', async () => {
        const out = await t.tcl(`fconfigure stdout -encoding utf-8; array set n $KIT(name); puts $n(de); puts [llength $KIT(logs)]; array set l [lindex $KIT(logs) 0]; puts $l(kind):$l(id):[file tail $l(file)]; puts $KIT(perm,write)`);
        assert.equal(out, 'Kit Demo „Ä“\n2\nfile:main:kitdemo.log\nany\n');
    });

    it('kit_run returns the exit code, merged output and 124 on a timeout', async () => {
        const out = await t.tcl(`
            puts [kit_run 5 sh -c {echo out; echo err >&2; exit 7}]
            puts [kit_run_stdout 5 sh -c {echo out; echo err >&2}]
            puts [lindex [kit_run 1 sleep 5] 0]`);
        assert.equal(out, '7 {out\nerr}\n0 out\n124\n');
    });
});
