<script lang="ts">
    import Dialog from 'occulite-ui/components/Dialog';
    import {t} from 'occulite-ui/i18n';

    import type {KitState} from '../kit.svelte.js';

    /**
     * kit.ask() and kit.askText(): a modal question on occulite-ui's Dialog. (occulite-ui 0.1 has
     * no question dialog of its own; this goes once its `ask()` exists.)
     */
    let {kit}: {kit: KitState} = $props();

    let open = $state(false);
    let input = $state<HTMLInputElement | undefined>(undefined);
    let confirmButton = $state<HTMLButtonElement | undefined>(undefined);

    $effect(() => {
        open = kit.question !== null;
    });

    $effect(() => {
        if (open) {
            queueMicrotask(() => (kit.question?.kind === 'text' ? input?.select() : confirmButton?.focus()));
        }
    });

    function cancel(): void {
        kit.answer(kit.question?.kind === 'text' ? null : false);
    }

    function confirm(): void {
        const q = kit.question;
        kit.answer(q?.kind === 'text' ? q.value : true);
    }
</script>

{#if kit.question}
    {@const q = kit.question}
    <Dialog bind:open title={q.options.title} width="420px" movable={false} onclose={cancel} testId="kit-ask">
        <p class="kit-ask-message">{q.options.message}</p>
        {#if q.kind === 'text'}
            <input
                class="hmm-input kit-ask-input"
                bind:this={input}
                bind:value={q.value}
                onkeydown={(e) => {
                    if (e.key === 'Enter') confirm();
                }}
                data-testid="kit-ask-input"
            />
        {/if}
        {#snippet buttons()}
            <button type="button" class="hmm-button" onclick={cancel} data-testid="kit-ask-cancel">
                {q.options.cancel ?? t('kit.cancel')}
            </button>
            <button
                type="button"
                class="hmm-button kit-primary"
                class:kit-danger={q.kind === 'confirm' && 'danger' in q.options && q.options.danger}
                bind:this={confirmButton}
                onclick={confirm}
                data-testid="kit-ask-confirm"
            >
                {q.options.confirm}
            </button>
        {/snippet}
    </Dialog>
{/if}

<style>
    .kit-ask-message {
        margin: 0 0 10px;
        white-space: pre-line;
    }

    .kit-ask-input {
        width: 100%;
    }
</style>
