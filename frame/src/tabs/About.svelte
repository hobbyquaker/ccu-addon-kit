<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    import {getKit} from '../context.js';
    import type {KitState} from '../kit.svelte.js';
    import KitError from '../components/KitError.svelte';
    import KitLoading from '../components/KitLoading.svelte';
    import type {KitTabProps} from '../types.js';

    interface Version {
        installed: string | null;
        components: Record<string, string>;
        kit: string;
        platform: {kind: string; firmware: string; arch: string; tcl?: string};
    }
    interface Check {
        latest: string | null;
        newer: boolean;
        url?: string;
        error?: string;
        checkedAt?: number;
    }

    let {active}: KitTabProps = $props();
    const kit = getKit() as KitState;

    let version = $state.raw<Version | null>(null);
    let versionError = $state.raw<unknown>(null);
    let check = $state.raw<Check | null>(null);
    let checking = $state(false);
    let checkedOnce = false;

    const PLATFORM: Record<string, string> = {ccu3: 'CCU3', openccu: 'OpenCCU', 'openccu-lite': 'openccu-lite'};

    async function load(): Promise<void> {
        versionError = null;
        try {
            version = await kit.api<Version>('kit/version.cgi');
        } catch (e) {
            versionError = e;
        }
    }

    async function runCheck(force: boolean): Promise<void> {
        checking = true;
        try {
            check = force
                ? await kit.api<Check>('kit/version.cgi', {form: {cmd: 'check', force: 1}, timeoutMs: 30000})
                : await kit.api<Check>('kit/version.cgi', {query: {cmd: 'check'}, timeoutMs: 30000});
        } catch (e) {
            check = {latest: null, newer: false, error: e instanceof Error && 'code' in e ? String((e as {code: string}).code) : 'internal'};
        } finally {
            checking = false;
        }
    }

    $effect(() => {
        if (!active) return;
        if (version === null && !versionError) void load();
        // a CCU checks once when the tab opens (from the cache when it is fresh); openccu-lite
        // contacts the internet only when the user asks
        if (!checkedOnce && kit.config.update && !kit.isLite) {
            checkedOnce = true;
            void runCheck(false);
        }
    });

    const result = $derived.by(() => {
        if (!check) return '';
        if (check.error === 'offline') return t('kit.about.offline');
        if (check.error === 'rate-limited') return t('kit.about.rateLimited');
        if (check.error === 'no-release') return t('kit.about.noRelease');
        if (check.error === 'not-configured') return t('kit.about.notConfigured');
        if (check.error) return t('kit.about.checkFailed', {code: check.error});
        if (check.newer && check.latest) return t('kit.about.newer', {version: check.latest});
        return t('kit.about.current');
    });
</script>

<section class="kit-section" data-testid="kit-tab-about">
    {#if versionError}
        <KitError error={versionError} retry={load} />
    {:else if version === null}
        <KitLoading />
    {:else}
        <dl class="kit-dl">
            <dt>{t('kit.about.installed')}</dt>
            <dd data-testid="kit-about-installed">{version.installed ?? '–'}</dd>
            {#each Object.entries(version.components) as [name, v] (name)}
                <dt>{name}</dt>
                <dd>{v}</dd>
            {/each}
            <dt>{t('kit.about.kit')}</dt>
            <dd>{version.kit}</dd>
            <dt>{t('kit.about.platform')}</dt>
            <dd data-testid="kit-about-platform">
                {PLATFORM[version.platform.kind] ?? version.platform.kind}
                {version.platform.firmware}
                ({version.platform.arch}{version.platform.tcl ? `, Tcl ${version.platform.tcl}` : ''})
            </dd>
            {#if kit.session.user}
                <dt>{t('kit.about.user')}</dt>
                <dd>{kit.session.user}{kit.session.level !== 'unknown' ? ` (${kit.session.level})` : ''}</dd>
            {/if}
        </dl>

        {#if kit.config.update}
            <div class="kit-about-check">
                <button type="button" class="hmm-button kit-cmd" disabled={checking} onclick={() => runCheck(true)} data-testid="kit-about-check">
                    <span class="kit-cmd-spin" class:kit-cmd-spin-on={checking} aria-hidden="true"></span>
                    {t('kit.about.check')}
                </button>
                <span class="kit-about-result" class:kit-muted={check?.error === 'offline'} data-testid="kit-about-result">
                    {result}
                    {#if check?.url && check.newer}
                        <a href={check.url} target="_blank" rel="noopener noreferrer">{t('kit.about.release')}</a>
                    {/if}
                </span>
            </div>
        {/if}

        {#if kit.config.links.length}
            <h3 class="kit-h3">{t('kit.about.links')}</h3>
            <ul class="kit-links">
                {#each kit.config.links as l (l.url)}
                    <li><a href={l.url} target="_blank" rel="noopener noreferrer">{kit.text(l.label)}</a></li>
                {/each}
            </ul>
        {/if}
    {/if}
</section>

<style>
    .kit-about-check {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 10px;
        margin-top: 14px;
    }

    .kit-cmd {
        display: inline-flex;
        align-items: center;
        gap: 6px;
    }

    .kit-cmd-spin {
        width: 10px;
        height: 10px;
        border-radius: 50%;
        border: 2px solid transparent;
    }

    .kit-cmd-spin-on {
        border-color: currentColor;
        border-top-color: transparent;
        animation: kit-about-spin 0.8s linear infinite;
    }

    @keyframes kit-about-spin {
        to {
            transform: rotate(360deg);
        }
    }

    .kit-h3 {
        margin: 18px 0 6px;
        font-size: var(--hmm-font-size);
    }

    .kit-links {
        margin: 0;
        padding-left: 18px;
    }
</style>
