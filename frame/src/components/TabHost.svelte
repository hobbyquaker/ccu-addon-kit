<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    import type {LazyTabComponent, TabComponent} from '../types.js';
    import KitLoading from './KitLoading.svelte';

    /**
     * One tab: mounted on its first opening and kept (so its input survives a tab switch), loaded
     * lazily when its component is `() => import(...)`, inside an error boundary that keeps the
     * frame and the other tabs working when this one throws.
     */
    let {
        component,
        active,
        path,
    }: {component: TabComponent | LazyTabComponent; active: boolean; path: string} = $props();

    // a Svelte 5 component takes (anchor, props); a lazy loader takes nothing
    const lazy = $derived(component.length === 0);
    let loaded = $state.raw<TabComponent | null>(null);
    let loadError = $state.raw<unknown>(null);
    let loading = false;

    function load(): void {
        if (loading || loaded) return;
        loading = true;
        loadError = null;
        (component as LazyTabComponent)()
            .then((m) => (loaded = m.default))
            .catch((e: unknown) => (loadError = e))
            .finally(() => (loading = false));
    }

    $effect(() => {
        if (lazy && active) load();
    });

    const Tab = $derived(lazy ? loaded : (component as TabComponent));
</script>

{#snippet failure(error: unknown, reset: () => void)}
    <div class="kit-boundary" role="alert" data-testid="kit-boundary">
        <p>{t('kit.boundary')}</p>
        <button type="button" class="hmm-button" onclick={reset}>{t('kit.retry')}</button>
        <details>
            <summary>{t('kit.details')}</summary>
            <pre class="hmm-mono">{error instanceof Error ? (error.stack ?? error.message) : String(error)}</pre>
        </details>
    </div>
{/snippet}

{#if loadError}
    {@render failure(loadError, load)}
{:else if Tab}
    <svelte:boundary onerror={(e) => console.error('ccu-addon-kit: a tab failed:', e)}>
        <Tab {active} {path} />
        {#snippet failed(error, reset)}
            {@render failure(error, reset)}
        {/snippet}
    </svelte:boundary>
{:else}
    <KitLoading />
{/if}

<style>
    .kit-boundary {
        margin: 12px 0;
        padding: 10px 12px;
        border: 1px solid var(--hmm-error);
        border-radius: var(--hmm-radius);
    }

    .kit-boundary p {
        margin: 0 0 8px;
        color: var(--hmm-error);
    }

    .kit-boundary pre {
        white-space: pre-wrap;
        font-size: var(--hmm-font-size-small);
        max-height: 200px;
        overflow: auto;
    }
</style>
