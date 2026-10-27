// The frame in a browser against the stub server (playwright.config.mjs): kitdemo's built frame
// and the kit's real Tcl CGIs.
import {expect, test} from '@playwright/test';

const SID = '@PwAdmin001@';
const PAGE = `/addons/kitdemo/settings.cgi?sid=${SID}`;

async function stub(request, action, data = {}) {
    const r = await request.post(`/__stub/${action}`, {data});
    expect(r.ok()).toBeTruthy();
    return r.json();
}

test.beforeEach(async ({request, page}) => {
    await stub(request, 'reset');
    page.on('pageerror', (e) => { throw e; });
});

test('opens with the WebUI sid, drops it from the address, survives a reload', async ({page}) => {
    const errors = [];
    page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
    await page.goto(PAGE);
    await expect(page.getByTestId('kit-name')).toHaveText('Kit Demo');
    await expect(page.getByTestId('kit-pill').first()).toHaveAttribute('data-view', 'running');
    await expect(page.getByTestId('kit-pill').first()).toContainText('48 MB');
    expect(page.url()).not.toContain('sid=');
    expect(new URL(page.url()).hash).toBe('#/hello');
    await page.reload();
    await expect(page.getByTestId('kit-pill').first()).toHaveAttribute('data-view', 'running');
    await expect(page.getByTestId('kit-session-overlay')).toHaveCount(0);
    expect(errors).toEqual([]);
});

test('without a session: the overlay, and To the WebUI goes to /', async ({page}) => {
    await page.goto('/addons/kitdemo/settings.cgi');
    await expect(page.getByTestId('kit-session-overlay')).toBeVisible();
    await page.getByTestId('kit-overlay-leave').click();
    await expect(page.getByTestId('webui')).toBeVisible();
});

test('the example tab: load, edit, save with the request header, reload of the service', async ({page, request}) => {
    await page.goto(PAGE);
    const input = page.getByTestId('hello-input');
    await expect(input).toHaveValue('Hallo Welt');
    await input.fill('Moin');
    const post = page.waitForRequest((r) => r.url().includes('api/hello.cgi') && r.method() === 'POST');
    await page.getByTestId('hello-save').click();
    const req = await post;
    expect(req.headers()['x-ccu-addon-request']).toBe('1');
    expect(req.postData()).toBe('greeting=Moin');
    expect(req.url()).toContain('sid=%40PwAdmin001%40');
    await expect(page.getByTestId('kit-notices')).toContainText('Gespeichert');
    await expect.poll(async () => (await stub(request, 'state')).greeting).toBe('Moin');
    await expect.poll(async () => (await stub(request, 'state')).rcCalls).toContain('reload');
});

test('service actions: stop asks first, the pill keeps its size in every state', async ({page, request}) => {
    await page.goto(`${PAGE}#/kit-service`);
    const pill = page.getByTestId('kit-tab-service').getByTestId('kit-pill');
    await expect(pill).toHaveAttribute('data-view', 'running');
    const size = await pill.boundingBox();
    const headerPill = page.getByTestId('kit-header').getByTestId('kit-pill');
    const headerSize = await headerPill.boundingBox();
    const stopButton = page.getByTestId('kit-cmd-stop');
    const buttonSize = await stopButton.boundingBox();

    await stopButton.click();
    await expect(page.getByTestId('kit-ask')).toBeVisible();
    await page.getByTestId('kit-ask-cancel').click();
    await expect(page.getByTestId('kit-ask')).toHaveCount(0);
    expect((await stub(request, 'state')).rcCalls).not.toContain('stop');

    await stopButton.click();
    await page.getByTestId('kit-ask-confirm').click();
    await expect(pill).toHaveAttribute('data-view', 'busy');
    expect((await pill.boundingBox()).width).toBe(size.width);
    expect((await stopButton.boundingBox()).width).toBe(buttonSize.width);
    await expect(pill).toHaveAttribute('data-view', 'stopped');
    expect((await pill.boundingBox()).width).toBe(size.width);
    expect((await headerPill.boundingBox()).width).toBe(headerSize.width);
    await expect(page.getByTestId('kit-output')).toContainText('Stopping kitdemo: OK');
    await expect(page.getByTestId('kit-cmd-start')).toBeEnabled();
    await expect(page.getByTestId('kit-cmd-stop')).toBeDisabled();

    await page.getByTestId('kit-cmd-start').click();
    await expect(pill).toHaveAttribute('data-view', 'running');

    await stub(request, 'rc', {status: 'failed'});
    await page.getByTestId('kit-tabs').getByRole('tab').nth(1).click(); // away and back: refresh
    await page.getByTestId('kit-tabs').getByRole('tab', {name: 'Dienst'}).click();
    await expect(pill).toHaveAttribute('data-view', 'failed');
    await expect(page.getByTestId('kit-last-error')).toContainText('config file missing');
    expect((await pill.boundingBox()).width).toBe(size.width);

    await stub(request, 'rc', {status: 'garbage'});
    await page.getByTestId('kit-tabs').getByRole('tab').nth(1).click();
    await page.getByTestId('kit-tabs').getByRole('tab', {name: 'Dienst'}).click();
    await expect(pill).toHaveAttribute('data-view', 'unknown');
    await expect(page.getByTestId('kit-status-unreadable')).toContainText('kitdemo is fine, probably');
});

test('a failed action: the error toast and the output', async ({page, request}) => {
    await page.goto(`${PAGE}#/kit-service`);
    await stub(request, 'rc', {actionExit: 1});
    await page.getByTestId('kit-cmd-reload').click();
    await expect(page.getByTestId('kit-notices')).toContainText('Neu laden fehlgeschlagen');
    await expect(page.getByTestId('kit-output')).toContainText('Reloading kitdemo: OK');
});

test('session expiry: the overlay keeps unsaved input, Retry brings the page back', async ({page, request}) => {
    await page.goto(PAGE);
    await page.getByTestId('hello-input').fill('nicht gespeichert');
    await stub(request, 'expire');
    await page.getByTestId('kit-tabs').getByRole('tab', {name: 'Protokoll'}).click();
    await page.getByTestId('kit-ask-confirm').click(); // leave the dirty tab
    await expect(page.getByTestId('kit-session-overlay')).toBeVisible();
    await expect(page.getByTestId('kit-session-overlay')).toContainText('Die Sitzung ist abgelaufen');
    await page.getByTestId('kit-overlay-retry').click();
    await expect(page.getByTestId('kit-session-overlay')).toBeVisible();
    await stub(request, 'revive');
    await page.getByTestId('kit-overlay-retry').click();
    await expect(page.getByTestId('kit-session-overlay')).toHaveCount(0);
    await page.getByTestId('kit-tabs').getByRole('tab', {name: 'Begrüßung'}).click();
    await expect(page.getByTestId('hello-input')).toHaveValue('nicht gespeichert');
});

test('a dirty tab asks before it is left; no keeps it', async ({page}) => {
    await page.goto(PAGE);
    await page.getByTestId('hello-input').fill('x');
    await page.getByTestId('kit-tabs').getByRole('tab', {name: 'Info'}).click();
    await expect(page.getByTestId('kit-ask')).toBeVisible();
    await page.getByTestId('kit-ask-cancel').click();
    await expect(page.getByTestId('kit-tabs').getByRole('tab', {name: 'Begrüßung'})).toHaveAttribute('aria-selected', 'true');
    expect(new URL(page.url()).hash).toBe('#/hello');
});

test('the log: the newest lines, the filter, the line count, download links with the sid', async ({page}) => {
    await page.goto(`${PAGE}#/kit-log`);
    const text = page.getByTestId('kit-log-text');
    await expect(text).toContainText('heartbeat 299');
    await expect(text).not.toContainText('heartbeat 99\n');
    await page.getByTestId('kit-log-lines').selectOption('500');
    await expect(text).toContainText('heartbeat 0\n');
    await page.getByTestId('kit-log-filter').fill('heartbeat 12');
    await expect(text).toHaveText(/heartbeat 12\n(.|\n)*heartbeat 129/);
    expect(await page.getByTestId('kit-log-download').getAttribute('href')).toBe('/addons/kitdemo/kit/log.cgi?source=main&download=1&sid=%40PwAdmin001%40');
    await page.getByTestId('kit-log-filter').fill('');
    await page.getByTestId('kit-log-source').selectOption('syslog');
    await expect(page.getByTestId('kit-log-text')).toHaveText('Oct 27 10:00:01 ccu user.info kitdemo[4242]: started');
});

test('about: versions, the update check, the links', async ({page}) => {
    await page.goto(`${PAGE}#/kit-about`);
    await expect(page.getByTestId('kit-about-installed')).toHaveText('0.4.0');
    await expect(page.getByTestId('kit-about-platform')).toContainText('OpenCCU 3.89.8');
    await expect(page.getByTestId('kit-about-result')).not.toBeEmpty();
    await page.getByTestId('kit-about-check').click();
    await expect(page.getByTestId('kit-about-result')).not.toBeEmpty();
    await expect(page.getByRole('link', {name: 'ccu-addon-kit auf GitHub'})).toHaveAttribute('href', 'https://github.com/hobbyquaker/ccu-addon-kit');
});

test('the app button hands the session over', async ({page}) => {
    await page.goto(PAGE);
    const app = page.getByTestId('kit-app');
    await expect(app).toHaveText('Anwendung öffnen');
    expect(await app.getAttribute('href')).toBe('/addons/kitdemo/app/?sid=%40PwAdmin001%40');
    const popup = page.waitForEvent('popup');
    await app.click();
    const p = await popup;
    await expect(p.locator('#text')).toHaveAttribute('data-sid', 'yes');
});

for (const scheme of ['light', 'dark']) {
    test(`theme follows the browser (${scheme}), the toggle overrides it`, async ({page}) => {
        await page.emulateMedia({colorScheme: scheme});
        await page.goto(PAGE);
        await expect(page.getByTestId('kit-frame')).toBeVisible();
        const bg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
        expect(await bg()).toBe(scheme === 'dark' ? 'rgb(27, 31, 38)' : 'rgb(255, 255, 255)');
        await page.getByTestId('kit-theme').click(); // system -> light
        await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
        await page.getByTestId('kit-theme').click(); // -> dark
        await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
        expect(await bg()).toBe('rgb(27, 31, 38)');
        await page.reload();
        await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    });
}

test('language: the WebUI user\'s, else the browser\'s; the toggle wins', async ({page, request, browser}) => {
    await stub(request, 'lang', {value: 2});
    await page.goto(PAGE);
    await expect(page.getByTestId('kit-tabs').getByRole('tab', {name: 'Service'})).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    await page.getByTestId('kit-lang').selectOption('de');
    await expect(page.getByTestId('kit-tabs').getByRole('tab', {name: 'Dienst'})).toBeVisible();
    await stub(request, 'lang', {value: 0});
    const ctx = await browser.newContext({locale: 'en-GB'});
    const p = await ctx.newPage();
    await p.goto(PAGE);
    await expect(p.getByTestId('kit-tabs').getByRole('tab', {name: 'Greeting'})).toBeVisible();
    await ctx.close();
});

test('keep-alive: Session.renew every 4 minutes on a CCU', async ({page, request}) => {
    await page.clock.install();
    await page.goto(PAGE);
    await expect(page.getByTestId('kit-pill').first()).toHaveAttribute('data-view', 'running');
    await page.clock.fastForward('04:05');
    await expect.poll(async () => (await stub(request, 'state')).renews).toEqual([
        {version: '1.1', method: 'Session.renew', params: {_session_id_: 'PwAdmin001'}},
    ]);
});

test('inside openccu-lite\'s shell: compact header, the shell\'s theme and language, the journal link, no keep-alive', async ({page, request}) => {
    await stub(request, 'lite', {on: true});
    await page.clock.install();
    await page.goto('/lite-shell.html');
    const f = page.frameLocator('#f');
    await expect(f.getByTestId('kit-frame')).toHaveClass(/kit-frame-embedded/);
    await expect(f.getByTestId('kit-header')).toHaveClass(/kit-header-compact/);
    await expect(f.locator('.kit-header-icon')).toHaveCount(0);
    await expect(f.getByTestId('kit-theme')).toHaveCount(0);
    await expect(f.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(f.getByTestId('kit-tabs').getByRole('tab', {name: 'Greeting'})).toBeVisible();
    expect(await f.getByTestId('kit-app').getAttribute('href')).toBe('/addons/kitdemo/app/');
    await page.evaluate(() => window.shell('light', 'de'));
    await expect(f.locator('html')).toHaveAttribute('data-theme', 'light');
    await expect(f.getByTestId('kit-tabs').getByRole('tab', {name: 'Begrüßung'})).toBeVisible();
    await f.getByTestId('kit-tabs').getByRole('tab', {name: 'Protokoll'}).click();
    await f.getByTestId('kit-log-source').selectOption('syslog');
    await expect(f.getByTestId('kit-log-journal')).toHaveAttribute('href', '/system/log?unit=addon-kitdemo');
    await page.clock.fastForward('05:00');
    expect((await stub(request, 'state')).renews).toEqual([]);
});
