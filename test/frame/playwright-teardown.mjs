// Removes the stub server's container, however the run ended.
import {spawnSync} from 'node:child_process';

export default function teardown() {
    spawnSync('docker', ['rm', '-f', `kit-pw-${process.env.PW_PORT || 18080}`], {stdio: 'ignore'});
}
