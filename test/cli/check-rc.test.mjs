// check-rc against small local rc.d scripts: a legacy one (mosquitto's exit codes before v1)
// and a broken one. The reference script itself runs in the e2e container (test/e2e).
import {after, before, describe, it} from 'node:test';
import assert from 'node:assert/strict';
import {chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {checkRc} from '../../cli/check-rc.mjs';

function script(dir, name, {legacy = true, multiline = false} = {}) {
    const p = join(dir, name);
    writeFileSync(p, `#!/bin/sh
PIDFILE=${dir}/${name}.pid
pid() { [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null && cat $PIDFILE; }
case "$1" in
  status)
    p=$(pid)
    if [ -n "$p" ]; then
      rss=$(awk '/^VmRSS:/ {print $2}' /proc/$p/status)
      printf '{"schema":1,"running":true,"pid":%s,"rss_kb":%s,"version":"1.0"}\\n' $p $rss
      ${multiline ? 'echo extra' : ':'}
      exit 0
    fi
    printf '{"schema":1,"running":false,"version":"1.0"}\\n'; exit ${legacy ? 1 : 3} ;;
  start) [ -n "$(pid)" ] && exit ${legacy ? 1 : 0}; cd /; sleep 1000 >/dev/null 2>&1 </dev/null & echo $! > $PIDFILE ;;
  stop) p=$(pid); [ -z "$p" ] && exit ${legacy ? 1 : 0}; kill $p; rm -f $PIDFILE ;;
  restart) "$0" stop; "$0" start ;;
  reload) exit 3 ;;
  info) echo "Name: Test"; echo "Version: 1.0" ;;
  *) exit 0 ;;
esac
exit 0
`);
    chmodSync(p, 0o755);
    return p;
}

describe('check-rc', {timeout: 120000}, () => {
    let dir;
    before(() => { dir = mkdtempSync(join(tmpdir(), 'kit-rc-')); });
    after(() => {
        for (const n of ['legacy', 'modern', 'multi']) { try { process.kill(Number(readFileSync(join(dir, `${n}.pid`), 'utf8'))); } catch { /* gone */ } }
        rmSync(dir, {recursive: true, force: true});
    });

    it('fails the legacy exit codes, and warns about them with --legacy', async () => {
        const rc = script(dir, 'legacy');
        const strict = await checkRc({rc, noReload: true, statusRetryMs: 3000});
        assert.equal(strict.ok, false);
        const ids = (r, s) => r.results.filter((x) => x.status === s).map((x) => x.id);
        assert.deepEqual(ids(strict, 'fail'), ['C1', 'C4', 'C7', 'C8']);
        const legacy = await checkRc({rc, legacy: true, noReload: true, statusRetryMs: 3000});
        assert.equal(legacy.ok, true, JSON.stringify(legacy.results));
        assert.deepEqual(ids(legacy, 'warn'), ['C1', 'C4', 'C7', 'C8']);
    });

    it('passes a conforming script and fails reload without --no-reload', async () => {
        const rc = script(dir, 'modern', {legacy: false});
        const r = await checkRc({rc, noReload: true, statusRetryMs: 3000});
        assert.equal(r.ok, true, JSON.stringify(r.results));
        assert.equal(r.results.find((x) => x.id === 'C12').status, 'skip');
        const nr = await checkRc({rc, statusRetryMs: 3000});
        assert.equal(nr.results.find((x) => x.id === 'C5').status, 'fail');
    });

    it('fails a status with more than one line', async () => {
        const rc = script(dir, 'multi', {legacy: false, multiline: true});
        const r = await checkRc({rc, noReload: true, statusRetryMs: 2000});
        assert.equal(r.ok, false);
        assert.equal(r.results.find((x) => x.id === 'C11').status, 'fail');
        assert.equal(r.results.find((x) => x.id === 'C3').status, 'fail');
    });
});
