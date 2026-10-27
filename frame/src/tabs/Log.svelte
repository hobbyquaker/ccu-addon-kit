<script lang="ts">
    import {t} from 'occulite-ui/i18n';
    import {tick} from 'svelte';

    import {getKit} from '../context.js';
    import type {KitState} from '../kit.svelte.js';
    import KitError from '../components/KitError.svelte';
    import KitLoading from '../components/KitLoading.svelte';
    import type {I18nText, KitTabProps} from '../types.js';

    interface Source {
        id: string;
        label?: I18nText;
        kind: 'file' | 'syslog';
        available: boolean;
        link?: string;
    }
    interface Tail {
        lines: string[];
        truncated?: boolean;
    }

    let {active}: KitTabProps = $props();
    const kit = getKit() as KitState;

    let sources = $state.raw<Source[] | null>(null);
    let sourcesError = $state.raw<unknown>(null);
    let source = $state('');
    let lines = $state(200);
    let filter = $state('');
    let auto = $state(false);
    let tail = $state.raw<Tail | null>(null);
    let tailError = $state.raw<unknown>(null);
    let loading = $state(false);
    let pre = $state<HTMLPreElement | undefined>(undefined);

    const current = $derived(sources?.find((s) => s.id === source));
    const shown = $derived.by(() => {
        if (!tail) return [];
        const f = filter.trim().toLowerCase();
        return f ? tail.lines.filter((l) => l.toLowerCase().includes(f)) : tail.lines;
    });

    async function loadSources(): Promise<void> {
        sourcesError = null;
        try {
            const r = await kit.api<{sources: Source[]}>('kit/log.cgi');
            sources = r.sources;
            if (!source || !r.sources.some((s) => s.id === source)) source = r.sources[0]?.id ?? '';
        } catch (e) {
            sourcesError = e;
        }
    }

    async function loadTail(): Promise<void> {
        if (!current || !current.available) {
            tail = null;
            return;
        }
        loading = true;
        try {
            const r = await kit.api<Tail>('kit/log.cgi', {query: {source, lines}});
            tail = r;
            tailError = null;
            await tick();
            if (pre) pre.scrollTop = pre.scrollHeight;
        } catch (e) {
            tailError = e;
        } finally {
            loading = false;
        }
    }

    $effect(() => {
        if (active && sources === null && !sourcesError) void loadSources();
    });

    $effect(() => {
        // reload when the source or the line count change, or the tab opens
        void source;
        void lines;
        if (active && current) void loadTail();
    });

    $effect(() => {
        if (!active || !auto) return;
        const timer = setInterval(() => {
            if (document.visibilityState !== 'hidden' && !loading) void loadTail();
        }, 5000);
        return () => clearInterval(timer);
    });
</script>

<section class="kit-section kit-log" data-testid="kit-tab-log">
    {#if sourcesError}
        <KitError error={sourcesError} retry={loadSources} />
    {:else if sources === null}
        <KitLoading />
    {:else}
        <div class="kit-log-bar">
            <label>
                {t('kit.log.source')}
                <select class="hmm-select" bind:value={source} data-testid="kit-log-source">
                    {#each sources as s (s.id)}
                        <option value={s.id}>{s.label ? kit.text(s.label) : s.id}</option>
                    {/each}
                </select>
            </label>
            <label>
                {t('kit.log.lines')}
                <select class="hmm-select" bind:value={lines} data-testid="kit-log-lines">
                    <option value={200}>200</option>
                    <option value={500}>500</option>
                    <option value={2000}>2000</option>
                </select>
            </label>
            <input class="hmm-input kit-log-filter" type="search" placeholder={t('kit.log.filter')} aria-label={t('kit.log.filter')} bind:value={filter} data-testid="kit-log-filter" />
            <label class="kit-log-auto"><input type="checkbox" bind:checked={auto} data-testid="kit-log-auto" /> {t('kit.log.auto')}</label>
            <span class="kit-log-spacer"></span>
            <button type="button" class="hmm-button" disabled={loading || !current?.available} onclick={loadTail} data-testid="kit-log-refresh">{t('kit.log.refresh')}</button>
            {#if current?.available}
                <a class="hmm-button kit-a" href={kit.url('kit/log.cgi', {source, download: 1})} download data-testid="kit-log-download">{t('kit.log.download')}</a>
            {/if}
            <a class="hmm-button kit-a" href={kit.url('kit/log.cgi', {bundle: 1})} download data-testid="kit-log-bundle">{t('kit.log.bundle')}</a>
        </div>

        {#if current && !current.available}
            <div class="kit-notice" data-testid="kit-log-unavailable">
                {#if current.link}
                    {t('kit.log.journal')}
                    <a href={current.link} target="_top" data-testid="kit-log-journal">{t('kit.log.journalLink')}</a>
                {:else}
                    {t('kit.log.unavailable')}
                {/if}
            </div>
        {:else if tailError}
            <KitError error={tailError} retry={loadTail} />
        {:else if tail === null}
            <KitLoading />
        {:else}
            <pre class="hmm-mono kit-log-text" bind:this={pre} data-testid="kit-log-text">{#if tail.lines.length === 0}{t('kit.log.empty')}{:else if shown.length === 0}{t('kit.log.noMatch')}{:else}{shown.join('\n')}{/if}</pre>
            {#if tail.truncated}
                <p class="kit-muted">{t('kit.log.truncated')}</p>
            {/if}
        {/if}
    {/if}
</section>

<style>
    .kit-log {
        display: flex;
        flex-direction: column;
        min-height: 0;
        height: 100%;
    }

    .kit-log-bar {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 8px;
        margin-bottom: 8px;
    }

    .kit-log-bar label {
        display: inline-flex;
        align-items: center;
        gap: 4px;
    }

    .kit-log-filter {
        width: 14em;
    }

    .kit-log-spacer {
        flex: 1;
    }

    .kit-log-text {
        flex: 1;
        min-height: 240px;
        margin: 0;
        padding: 8px;
        overflow: auto;
        border: 1px solid var(--hmm-border-muted);
        border-radius: var(--hmm-radius);
        background: var(--hmm-bg-sunken);
        font-size: var(--hmm-font-size-small);
        white-space: pre-wrap;
        word-break: break-all;
    }
</style>
