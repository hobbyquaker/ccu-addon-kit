<script lang="ts">
    import {has, t} from 'occulite-ui/i18n';

    import {isKitError} from '../api.js';

    /** An error of a tab: the code translated, Retry, the details in a disclosure. */
    let {error, retry = undefined}: {error: unknown; retry?: (() => void) | undefined} = $props();

    const code = $derived(isKitError(error) ? error.code : 'internal');
    const text = $derived(has(`kit.error.${code}`) ? t(`kit.error.${code}`) : t('kit.error.other', {code}));
    const detail = $derived(error instanceof Error ? error.message : String(error));
</script>

<div class="kit-error" role="alert" data-testid="kit-error" data-code={code}>
    <p>{text}</p>
    {#if retry}
        <button type="button" class="hmm-button" onclick={() => retry?.()}>{t('kit.retry')}</button>
    {/if}
    {#if detail}
        <details>
            <summary>{t('kit.details')}</summary>
            <pre class="hmm-mono">{detail}</pre>
        </details>
    {/if}
</div>

<style>
    .kit-error {
        margin: 12px 0;
        padding: 10px 12px;
        border: 1px solid var(--hmm-error);
        border-radius: var(--hmm-radius);
        background: var(--hmm-bg-sunken);
    }

    .kit-error p {
        margin: 0 0 8px;
        color: var(--hmm-error);
    }

    .kit-error pre {
        margin: 6px 0 0;
        white-space: pre-wrap;
        word-break: break-word;
        font-size: var(--hmm-font-size-small);
    }

    details {
        margin-top: 8px;
        color: var(--hmm-fg-muted);
    }
</style>
