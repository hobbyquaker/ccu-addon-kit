<script lang="ts">
    import LanguageSwitch from 'occulite-ui/components/LanguageSwitch';
    import ThemeSwitch from 'occulite-ui/components/ThemeSwitch';
    import {languageChoice, setLanguage, t} from 'occulite-ui/i18n';
    import {cycleTheme, theme} from 'occulite-ui/theme';

    import type {KitState} from '../kit.svelte.js';
    import Pill from './Pill.svelte';

    let {kit}: {kit: KitState} = $props();

    const embedded = $derived(kit.embedded);
    const version = $derived(kit.status?.version ?? '');
    const url = $derived(kit.status?.running && kit.status.url ? kit.status.url : '');
</script>

<header class="kit-header" class:kit-header-compact={embedded} data-testid="kit-header">
    {#if !embedded && kit.config.icon}
        <img class="kit-header-icon" src={kit.base + kit.config.icon} alt="" width="28" height="28" />
    {/if}
    <span class="kit-header-name" data-testid="kit-name">{kit.addon.name}</span>
    <span class="kit-header-version" data-testid="kit-version">{version}</span>
    {#if kit.session.can('read')}
        <Pill view={kit.service.view} status={kit.status} pending={kit.pending} lang={kit.lang} />
    {/if}
    <span class="kit-header-spacer"></span>
    {#if kit.app && kit.session.valid}
        <a class="hmm-button kit-primary kit-a kit-header-app" href={kit.app.url} target="_blank" rel="noopener" data-testid="kit-app">{kit.app.label}</a>
    {/if}
    {#if url}
        <a class="hmm-button kit-header-open" href={url} target="_blank" rel="noopener noreferrer">{t('kit.open')}</a>
    {/if}
    {#if !theme.followsShell}
        <ThemeSwitch theme={theme.choice} onclick={cycleTheme} testId="kit-theme" />
        <LanguageSwitch language={languageChoice.value} onchange={(l) => setLanguage(l)} label={t('kit.header.lang')} testId="kit-lang" />
    {/if}
</header>

<style>
    .kit-header {
        display: flex;
        align-items: center;
        gap: 10px;
        min-height: 48px;
        padding: 6px 16px;
        background: var(--hmm-header-bg);
        border-bottom: 1px solid var(--hmm-border-muted);
        flex-wrap: wrap;
    }

    .kit-header-compact {
        min-height: 36px;
        padding: 4px 12px;
    }

    .kit-header-icon {
        flex: none;
    }

    .kit-header-name {
        font-size: 15px;
        font-weight: 600;
    }

    .kit-header-compact .kit-header-name {
        font-size: var(--hmm-font-size);
    }

    .kit-header-version {
        /* reserved, so the pill does not move when the version arrives */
        min-width: 4em;
        color: var(--hmm-fg-muted);
        font-size: var(--hmm-font-size-small);
    }

    .kit-header-spacer {
        flex: 1;
    }

    .kit-header-open {
        text-decoration: none;
        color: inherit;
    }
</style>
