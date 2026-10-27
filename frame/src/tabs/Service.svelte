<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    import {getKit} from '../context.js';
    import type {KitState} from '../kit.svelte.js';
    import {ENABLED, formatDuration, formatKb} from '../status.js';
    import Pill from '../components/Pill.svelte';
    import KitError from '../components/KitError.svelte';
    import type {KitTabProps, ServiceCommand} from '../types.js';

    let {active}: KitTabProps = $props();
    const kit = getKit() as KitState;

    const s = $derived(kit.status);
    const view = $derived(kit.service.view);
    const allowed = $derived(kit.session.can('service'));
    const enabled = $derived(ENABLED[view]);
    const pids = $derived(s?.pids?.length ? s.pids.join(', ') : s?.pid ? String(s.pid) : '');
    const share = $derived(
        s?.rss_kb && s.memTotalKb ? Math.max(0.1, Math.round((s.rss_kb / s.memTotalKb) * 1000) / 10).toLocaleString(kit.lang) : '',
    );
    // re-rendered every poll, so the uptime moves with it
    const uptime = $derived(s?.running && typeof s.since === 'number' ? formatDuration(Date.now() / 1000 - s.since) : '');

    $effect(() => {
        if (active) void kit.service.refresh();
    });

    function run(cmd: ServiceCommand): void {
        void kit.service.run(cmd);
    }
</script>

<section class="kit-section" data-testid="kit-tab-service">
    <div class="kit-service-head">
        <Pill {view} status={s} pending={kit.pending} lang={kit.lang} large />
        <div class="kit-buttons" role="group">
            {#each kit.service.commands as cmd (cmd)}
                <button
                    type="button"
                    class="hmm-button kit-cmd"
                    class:kit-danger={cmd === 'stop'}
                    disabled={!allowed || !enabled.includes(cmd)}
                    onclick={() => run(cmd)}
                    data-testid="kit-cmd-{cmd}"
                >
                    <span class="kit-cmd-spin" class:kit-cmd-spin-on={kit.pending === cmd} aria-hidden="true"></span>
                    {t(`kit.cmd.${cmd}`)}
                </button>
            {/each}
        </div>
    </div>
    {#if kit.app}
        <p class="kit-service-app">
            <a class="hmm-button kit-primary kit-a" href={kit.app.url} target="_blank" rel="noopener" data-testid="kit-app-service">{kit.app.label}</a>
            {#if view !== 'running'}<span class="kit-muted">{t('kit.app.needsService')}</span>{/if}
        </p>
    {/if}
    {#if !allowed && kit.session.valid}
        <p class="kit-muted">{t('kit.service.noRights')}</p>
    {/if}

    {#if kit.statusError}
        <KitError error={kit.statusError} retry={() => kit.service.refresh()} />
    {/if}

    {#if view === 'failed' && s?.lastError}
        <div class="kit-notice kit-notice-error" data-testid="kit-last-error">
            <strong>{t('kit.service.lastError')}:</strong>
            {s.lastError}
            {#if kit.config.tabs.includes('kit:log')}
                <button type="button" class="kit-link" onclick={() => kit.route.go('kit:log')}>{t('kit.service.toLog')}</button>
            {/if}
        </div>
    {/if}
    {#if view === 'unknown' && s && !kit.statusError}
        <div class="kit-notice" data-testid="kit-status-unreadable">
            {t('kit.status.unreadable')}
            {#if s.raw}
                <details>
                    <summary>{t('kit.status.raw')}</summary>
                    <pre class="hmm-mono">{s.raw}</pre>
                </details>
            {/if}
        </div>
    {/if}

    <dl class="kit-dl">
        <dt>{t('kit.service.pid')}</dt>
        <dd data-testid="kit-pid">{pids || '–'}</dd>
        <dt>{t('kit.service.ram')}</dt>
        <dd data-testid="kit-ram">
            {#if s?.running && typeof s.rss_kb === 'number'}
                {formatKb(s.rss_kb, kit.lang)}{#if share}&nbsp;({t('kit.service.share', {pct: share})}){/if}
            {:else}–{/if}
        </dd>
        <dt>{t('kit.service.uptime')}</dt>
        <dd>{uptime || '–'}</dd>
        <dt>{t('kit.service.version')}</dt>
        <dd>{s?.version ?? '–'}</dd>
    </dl>

    {#if kit.lastResult && kit.lastResult.confirmed}
        <details class="kit-output" open={!kit.lastResult.ok} data-testid="kit-output">
            <summary>{t('kit.cmd.output')} ({t(`kit.cmd.${kit.lastResult.cmd}`)}{kit.lastResult.exit !== null ? `, exit ${kit.lastResult.exit}` : ''})</summary>
            <pre class="hmm-mono">{kit.lastResult.output || '–'}</pre>
        </details>
    {/if}
</section>

<style>
    .kit-service-head {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 12px;
        margin-bottom: 14px;
    }

    .kit-buttons {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
    }

    .kit-cmd {
        display: inline-flex;
        align-items: center;
        gap: 6px;
    }

    /* the spinner's room is always there, so a button keeps its width while it runs */
    .kit-cmd-spin {
        width: 10px;
        height: 10px;
        border-radius: 50%;
        border: 2px solid transparent;
    }

    .kit-cmd-spin-on {
        border-color: currentColor;
        border-top-color: transparent;
        animation: kit-cmd-spin 0.8s linear infinite;
    }

    @keyframes kit-cmd-spin {
        to {
            transform: rotate(360deg);
        }
    }

    .kit-service-app {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 10px;
        margin: 0 0 14px;
    }

    .kit-output pre {
        max-height: 240px;
        overflow: auto;
        white-space: pre-wrap;
    }
</style>
