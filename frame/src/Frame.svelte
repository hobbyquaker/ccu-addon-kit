<script lang="ts">
    import Notices from 'occulite-ui/components/Notices';
    import Tabs from 'occulite-ui/components/Tabs';
    import {t} from 'occulite-ui/i18n';
    import {onDestroy, onMount} from 'svelte';

    import {isKitError} from './api.js';
    import {setKit} from './context.js';
    import {routeOf, tabOfRoute, visibleTabs, type KitState} from './kit.svelte.js';
    import AskDialog from './components/AskDialog.svelte';
    import Header from './components/Header.svelte';
    import KitError from './components/KitError.svelte';
    import KitLoading from './components/KitLoading.svelte';
    import SessionOverlay from './components/SessionOverlay.svelte';
    import TabHost from './components/TabHost.svelte';
    import About from './tabs/About.svelte';
    import Log from './tabs/Log.svelte';
    import Service from './tabs/Service.svelte';
    import type {KitTab, TabComponent} from './types.js';

    let {kit, tabs}: {kit: KitState; tabs: readonly KitTab[]} = $props();
    // svelte-ignore state_referenced_locally -- the frame's kit never changes
    setKit(kit);

    const FRAME_TABS: Record<string, TabComponent> = {'kit:service': Service, 'kit:log': Log, 'kit:about': About};

    let phase = $state<'loading' | 'ready' | 'error'>('loading');
    let bootError = $state.raw<unknown>(null);
    /** Tabs opened at least once: they stay mounted, so their input survives a switch. */
    let opened = $state<Record<string, boolean>>({});

    const ids = $derived(phase === 'ready' ? visibleTabs(kit.config, tabs, kit.session.level, kit.platform.kind) : []);
    const byId = $derived(new Map(tabs.map((tab) => [tab.id, tab])));
    const bar = $derived(
        ids.map((id) => {
            const tab = byId.get(id);
            const badge = tab?.badge?.();
            return {
                id: routeOf(id),
                label: id.startsWith('kit:') ? t(`kit.tab.${id.slice(4)}`) : tab ? kit.text(tab.label) : id,
                ...(badge !== undefined && badge !== null && badge !== '' ? {badge} : {}),
            };
        }),
    );

    $effect(() => {
        if (kit.tab) opened[kit.tab] = true;
    });

    $effect(() => {
        document.title = kit.addon.name;
    });

    async function boot(): Promise<void> {
        phase = 'loading';
        bootError = null;
        try {
            const valid = await kit.checkSession();
            phase = 'ready';
            const list = visibleTabs(kit.config, tabs, kit.session.level, kit.platform.kind);
            kit.setTabs(list);
            if (valid) {
                kit.start(list);
            }
        } catch (e) {
            if (isKitError(e) && e.code === 'session-invalid') {
                phase = 'ready';
                kit.setTabs(visibleTabs(kit.config, tabs, kit.session.level, kit.platform.kind));
                return;
            }
            bootError = e;
            phase = 'error';
        }
    }

    onMount(() => {
        // the registry and kit.json disagree: vendor checks it at build time, the frame says it once
        const registered = new Set(tabs.map((tab) => tab.id));
        for (const id of kit.config.tabs) {
            if (!id.startsWith('kit:') && !registered.has(id)) console.warn(`ccu-addon-kit: kit.json lists tab "${id}", the frame has no component for it`);
        }
        for (const id of registered) {
            if (!kit.config.tabs.includes(id)) console.warn(`ccu-addon-kit: tab "${id}" is registered but not in kit.json's tabs`);
        }
        void boot();
    });

    onDestroy(() => kit.stop());

    const overlay = $derived.by(() => {
        if (phase !== 'ready') return '';
        if (kit.expired) return kit.isLite ? t('kit.session.expiredLite') : t('kit.session.expired');
        if (kit.sessionAnswer && !kit.sessionAnswer.session.valid) return t('kit.session.none');
        return '';
    });
</script>

<div class="kit-frame" class:kit-frame-embedded={kit.embedded} data-testid="kit-frame" data-phase={phase}>
    <Header {kit} />
    {#if phase === 'loading'}
        <main class="kit-main"><KitLoading /></main>
    {:else if phase === 'error'}
        <main class="kit-main"><KitError error={bootError} retry={boot} /></main>
    {:else}
        <nav class="kit-tabbar">
            {#key kit.nonce}
                <Tabs tabs={bar} active={routeOf(kit.tab)} onselect={(id) => void kit.go(tabOfRoute(id))} testId="kit-tabs" />
            {/key}
        </nav>
        <main class="kit-main">
            {#each ids as id (id)}
                {#if opened[id]}
                    <div
                        class="kit-panel"
                        id="panel-{routeOf(id)}"
                        role="tabpanel"
                        aria-labelledby="tab-{routeOf(id)}"
                        hidden={kit.tab !== id}
                        data-testid="kit-panel-{routeOf(id)}"
                    >
                        {#if FRAME_TABS[id]}
                            {@const Tab = FRAME_TABS[id]}
                            <svelte:boundary>
                                <Tab active={kit.tab === id && !overlay} path={kit.tab === id ? kit.path : ''} />
                                {#snippet failed(error, reset)}
                                    <KitError {error} retry={reset} />
                                {/snippet}
                            </svelte:boundary>
                        {:else}
                            {@const tab = byId.get(id)}
                            {#if tab}
                                <TabHost component={tab.component} active={kit.tab === id && !overlay} path={kit.tab === id ? kit.path : ''} />
                            {/if}
                        {/if}
                    </div>
                {/if}
            {/each}
        </main>
    {/if}
    {#if overlay}
        <SessionOverlay text={overlay} lite={kit.isLite} retry={() => kit.retrySession()} />
    {/if}
    <AskDialog {kit} />
    <div class="kit-notices">
        <Notices notices={kit.notices.items} ondismiss={(id) => kit.notices.dismiss(id)} testId="kit-notices" />
    </div>
</div>

<style>
    .kit-frame {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-width: 0;
    }

    .kit-tabbar {
        display: flex;
        height: var(--hmm-header-height);
        padding: 0 12px;
        overflow-x: auto;
        overflow-y: hidden;
        border-bottom: 1px solid var(--hmm-border);
        flex: none;
    }

    .kit-main {
        flex: 1;
        min-height: 0;
        overflow: auto;
        padding: 14px 16px;
    }

    .kit-panel {
        height: 100%;
    }

    .kit-panel[hidden] {
        display: none;
    }

    .kit-notices {
        position: fixed;
        right: 12px;
        bottom: 12px;
        z-index: 40;
    }

    /* the frame's shared building blocks, also for addon tabs */
    :global(.kit-section) {
        max-width: 1100px;
    }

    :global(.kit-dl) {
        display: grid;
        grid-template-columns: max-content 1fr;
        gap: 4px 16px;
        margin: 0;
    }

    :global(.kit-dl dt) {
        color: var(--hmm-fg-muted);
    }

    :global(.kit-dl dd) {
        margin: 0;
    }

    :global(.kit-notice) {
        margin: 10px 0;
        padding: 8px 12px;
        border-left: 3px solid var(--hmm-accent);
        background: var(--hmm-bg-sunken);
    }

    :global(.kit-notice-error) {
        border-left-color: var(--hmm-error);
    }

    :global(.kit-notice pre) {
        white-space: pre-wrap;
        font-size: var(--hmm-font-size-small);
    }

    :global(.kit-muted) {
        color: var(--hmm-fg-muted);
    }

    :global(.hmm-button.kit-primary) {
        border-color: var(--hmm-accent);
        background: var(--hmm-accent);
        color: var(--hmm-accent-fg);
    }

    :global(.hmm-button.kit-danger:not(:disabled)) {
        border-color: var(--hmm-error);
        color: var(--hmm-error);
    }

    :global(.hmm-button.kit-primary.kit-danger) {
        background: var(--hmm-error);
        color: var(--hmm-bg);
    }

    :global(.kit-link) {
        margin-left: 8px;
        padding: 0;
        border: none;
        background: none;
        color: var(--hmm-link);
        cursor: pointer;
        text-decoration: underline;
    }

    :global(a.kit-a) {
        color: inherit;
        text-decoration: none;
        display: inline-flex;
        align-items: center;
    }

    :global(a) {
        color: var(--hmm-link);
    }
</style>
