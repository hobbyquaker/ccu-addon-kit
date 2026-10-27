import {readFileSync} from 'node:fs';
import {dirname, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

export const KIT_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

export function kitVersion() {
    return JSON.parse(readFileSync(resolve(KIT_ROOT, 'package.json'), 'utf8')).version;
}
