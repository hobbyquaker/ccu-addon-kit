<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    /** A skeleton of fixed height: nothing jumps when the content arrives. */
    let {height = '160px', text = undefined}: {height?: string; text?: string | undefined} = $props();
</script>

<div class="kit-loading" style:min-height={height} role="status" aria-live="polite" data-testid="kit-loading">
    <span class="kit-spinner" aria-hidden="true"></span>
    <span>{text ?? t('kit.loading')}</span>
</div>

<style>
    .kit-loading {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 8px;
        color: var(--hmm-fg-muted);
    }

    .kit-spinner {
        width: 14px;
        height: 14px;
        border: 2px solid var(--hmm-border);
        border-top-color: var(--hmm-accent);
        border-radius: 50%;
        animation: kit-spin 0.8s linear infinite;
    }

    @keyframes kit-spin {
        to {
            transform: rotate(360deg);
        }
    }
</style>
