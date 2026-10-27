// Playwright against the stub server (test/frame/stub-server.mjs) with the kit's real Tcl CGIs.
// The stub runs in the ccu-addon-kit-tcl86 image (tclsh and the stub tclrega.so); build the
// frame first: npm run build:kitdemo. PW_PORT picks the port (default 18080).
import {defineConfig, devices} from '@playwright/test';

const PORT = Number(process.env.PW_PORT || 18080);

export default defineConfig({
    testDir: '.',
    outputDir: 'test-results',
    testMatch: /.*\.spec\.mjs/,
    // the stub has one state: one test after another
    globalTeardown: './playwright-teardown.mjs',
    workers: 1,
    fullyParallel: false,
    retries: process.env.CI ? 1 : 0,
    reporter: process.env.CI ? [['list'], ['github']] : 'list',
    use: {baseURL: `http://127.0.0.1:${PORT}`, locale: 'de-DE', trace: 'retain-on-failure'},
    projects: [{name: 'chromium', use: {...devices['Desktop Chrome']}}],
    webServer: {
        command: `sh test/frame/stub.sh ${PORT}`,
        cwd: '../..',
        url: `http://127.0.0.1:${PORT}/__stub/state`,
        timeout: 60000,
        reuseExistingServer: false,
        stdout: 'pipe',
        gracefulShutdown: {signal: 'SIGTERM', timeout: 5000},
    },
});
