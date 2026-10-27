#!/usr/bin/env node
// ccu-addon-kit: the command line (MIT). See README.md.

import {parseArgs} from 'node:util';
import {checkRc, printTable} from './check-rc.mjs';
import {scaffold} from './new.mjs';
import {vendor} from './vendor.mjs';
import {kitVersion} from './lib/kitroot.mjs';

const USAGE = `usage: ccu-addon-kit <command> [options]

  vendor   --config kit.json --out addon_files/<id> [--frame web/dist] [--force] [--check]
           copy the kit's Tcl library, CGIs and kit-run into the addon's tree and generate
           lib/kit/addon.tcl, www/frame/addon.json, the CGI stubs and etc/lighttpd.conf from
           kit.json; --frame copies the frame built with kitVite() into www/frame/
  check-rc --rc <rc.d script> [--exec "<prefix>"] [--log <file>] [--legacy] [--no-reload] [--json]
           the conformance test of the service convention v1 against a running system
  new      <id> [--name "<name>"] [--dir <parent>]
           scaffold an addon from the kitdemo example
  version  print the kit's version`;

async function main(argv) {
    const [cmd, ...rest] = argv;
    if (cmd === 'vendor') {
        const {values} = parseArgs({args: rest, options: {config: {type: 'string', default: 'kit.json'}, out: {type: 'string'}, frame: {type: 'string'},
            force: {type: 'boolean'}, check: {type: 'boolean'}}});
        if (!values.out) { throw new Error('vendor: --out is required'); }
        vendor({config: values.config, out: values.out, frame: values.frame, force: values.force, check: values.check});
        return 0;
    }
    if (cmd === 'check-rc') {
        const {values} = parseArgs({args: rest, options: {exec: {type: 'string'}, rc: {type: 'string'}, log: {type: 'string'},
            legacy: {type: 'boolean'}, 'no-reload': {type: 'boolean'}, json: {type: 'boolean'}}});
        if (!values.rc) { throw new Error('check-rc: --rc is required'); }
        const report = await checkRc({exec: values.exec, rc: values.rc, log: values.log, legacy: values.legacy, noReload: values['no-reload']});
        if (values.json) { console.log(JSON.stringify(report, null, 2)); } else { printTable(report); }
        return report.ok ? 0 : 1;
    }
    if (cmd === 'new') {
        const {values, positionals} = parseArgs({args: rest, allowPositionals: true, options: {name: {type: 'string'}, dir: {type: 'string', default: '.'}}});
        if (positionals.length !== 1) { throw new Error('new: one addon id is required'); }
        const r = scaffold({id: positionals[0], name: values.name, dir: values.dir});
        console.log(`new: ${r.written.length} files in ${r.dir}`);
        return 0;
    }
    if (cmd === 'version' || cmd === '--version') { console.log(kitVersion()); return 0; }
    console.error(USAGE);
    return cmd === undefined || cmd === 'help' || cmd === '--help' ? 0 : 2;
}

main(process.argv.slice(2)).then((code) => { process.exitCode = code; }, (err) => {
    console.error(`ccu-addon-kit: ${err.message}`);
    process.exitCode = 1;
});
