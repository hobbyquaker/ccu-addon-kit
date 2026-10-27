<script lang="ts">
    import {t} from 'occulite-ui/i18n';

    /**
     * Lies over every tab when the session is gone. The tabs stay mounted underneath, so input
     * that was not saved yet survives a renewal in another browser tab and Retry.
     */
    let {
        text,
        lite,
        retry,
    }: {text: string; lite: boolean; retry: () => Promise<unknown>} = $props();

    let busy = $state(false);

    async function onRetry(): Promise<void> {
        busy = true;
        try {
            await retry();
        } finally {
            busy = false;
        }
    }

    function leave(): void {
        if (lite) {
            // the gate sends the browser to the login and back
            location.reload();
        } else {
            location.href = '/';
        }
    }
</script>

<div class="kit-overlay" role="alertdialog" aria-modal="true" aria-labelledby="kit-overlay-text" data-testid="kit-session-overlay">
    <div class="kit-overlay-box">
        <p id="kit-overlay-text">{text}</p>
        <div class="kit-overlay-buttons">
            <button type="button" class="hmm-button" disabled={busy} onclick={onRetry} data-testid="kit-overlay-retry">{t('kit.retry')}</button>
            <button type="button" class="hmm-button kit-primary" onclick={leave} data-testid="kit-overlay-leave">
                {lite ? t('kit.session.signin') : t('kit.session.home')}
            </button>
        </div>
    </div>
</div>

<style>
    .kit-overlay {
        position: fixed;
        inset: 0;
        z-index: 50;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 16px;
        background: var(--hmm-overlay);
    }

    .kit-overlay-box {
        max-width: 440px;
        padding: 16px 18px;
        border: 1px solid var(--hmm-border-strong);
        border-radius: var(--hmm-radius);
        background: var(--hmm-bg);
        box-shadow: var(--hmm-shadow-menu);
    }

    .kit-overlay-box p {
        margin: 0 0 14px;
    }

    .kit-overlay-buttons {
        display: flex;
        justify-content: flex-end;
        gap: 8px;
    }
</style>
