<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    import {busyKind, formatDuration, formatKb} from '../status.js';
    import type {ServiceCommand, ServiceStatus, ServiceView} from '../types.js';

    /**
     * The service state. One fixed box for every state (the house rule: nothing resizes when the
     * state changes): the text is cut with an ellipsis, the whole of it is in the tooltip.
     */
    let {
        view,
        status,
        pending = null,
        lang,
        large = false,
    }: {view: ServiceView; status: ServiceStatus | null; pending?: ServiceCommand | null; lang: string; large?: boolean} = $props();

    const label = $derived(view === 'busy' ? t(`kit.status.${busyKind(status, pending)}`) : t(`kit.status.${view}`));
    const ram = $derived(view === 'running' && typeof status?.rss_kb === 'number' ? formatKb(status.rss_kb, lang) : '');
    const tooltip = $derived.by(() => {
        const parts = [label];
        if (ram && status?.rss_kb !== undefined) {
            parts.push(status.memTotalKb ? t('kit.status.ramOf', {ram, total: formatKb(status.memTotalKb, lang)}) : ram);
        }
        if (view === 'running' && typeof status?.since === 'number') {
            parts.push(t('kit.status.since', {duration: formatDuration(Date.now() / 1000 - status.since)}));
        }
        return parts.join(' · ');
    });
</script>

<span class="kit-pill kit-pill-{view}" class:kit-pill-large={large} title={tooltip} data-testid="kit-pill" data-view={view}>
    <span class="kit-pill-mark" class:kit-pill-spin={view === 'busy'} aria-hidden="true"></span>
    <span class="kit-pill-text">{label}{#if ram}<span class="kit-pill-ram">{` · ${t('kit.status.ram', {ram})}`}</span>{/if}</span>
</span>

<style>
    .kit-pill {
        --c: var(--hmm-fg-faint);
        display: inline-flex;
        align-items: center;
        gap: 6px;
        width: 15em;
        height: 22px;
        padding: 0 10px;
        border: 1px solid var(--c);
        border-radius: 11px;
        color: var(--hmm-fg);
        background: var(--hmm-bg);
        font-size: var(--hmm-font-size-small);
        white-space: nowrap;
        overflow: hidden;
        flex: none;
    }

    .kit-pill-large {
        width: 19em;
        height: 28px;
        border-radius: 14px;
        font-size: var(--hmm-font-size);
    }

    .kit-pill-running {
        --c: var(--hmm-ok);
    }

    .kit-pill-failed {
        --c: var(--hmm-error);
    }

    .kit-pill-busy {
        --c: var(--hmm-warn);
    }

    .kit-pill-mark {
        flex: none;
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: var(--c);
    }

    .kit-pill-spin {
        background: none;
        border: 2px solid var(--c);
        border-top-color: transparent;
        animation: kit-pill-spin 0.8s linear infinite;
    }

    .kit-pill-text {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .kit-pill-ram {
        color: var(--hmm-fg-muted);
    }

    @keyframes kit-pill-spin {
        to {
            transform: rotate(360deg);
        }
    }
</style>
