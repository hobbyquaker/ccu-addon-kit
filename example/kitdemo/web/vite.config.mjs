// The settings page of kitdemo: the kit's frame with one tab of its own (ccu-addon-kit, MIT).
// `vite build --config web/vite.config.mjs` writes web/dist/, which build.sh hands to
// `ccu-addon-kit vendor --frame web/dist`.
import {fileURLToPath} from 'node:url';
import {defineConfig} from 'vite';
import {kitVite} from 'ccu-addon-kit/vite';

export default defineConfig({
    root: fileURLToPath(new URL('.', import.meta.url)),
    plugins: [kitVite()],
});
