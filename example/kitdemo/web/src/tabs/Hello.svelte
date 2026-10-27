<script lang="ts">
    // kitdemo's tab: reads and writes the greeting through www/api/hello.cgi
    import {getKit, KitError, KitLoading} from 'ccu-addon-kit/frame';

    let {active}: {active: boolean; path: string} = $props();
    const kit = getKit();

    let greeting = $state('');
    let loaded = $state(false);
    let error = $state<unknown>(null);
    let saving = $state(false);
    const tooLong = $derived(greeting.length > 80);

    async function load(): Promise<void> {
        error = null;
        try {
            const r = await kit.api<{greeting: string}>('api/hello.cgi');
            greeting = r.greeting;
            loaded = true;
        } catch (e) {
            error = e;
        }
    }

    async function save(): Promise<void> {
        saving = true;
        try {
            await kit.api('api/hello.cgi', {form: {greeting}});
            kit.setDirty(false);
            kit.notify('success', kit.t('hello.saved'));
            // the daemon re-reads etc/ on HUP: the convention's reload
            if (kit.service.view === 'running' && kit.service.commands.includes('reload')) await kit.service.run('reload');
        } catch {
            kit.notify('error', kit.t('hello.saveFailed'));
        } finally {
            saving = false;
        }
    }

    $effect(() => {
        if (active && !loaded && !error) void load();
    });
</script>

<section class="kit-section" data-testid="hello">
    {#if error}
        <KitError {error} retry={load} />
    {:else if !loaded}
        <KitLoading />
    {:else}
        <label class="hello-field">
            <span>{kit.t('hello.label')}</span>
            <input
                class="hmm-input"
                bind:value={greeting}
                oninput={() => kit.setDirty(true)}
                maxlength={80}
                data-testid="hello-input"
            />
        </label>
        <p class="kit-muted">{tooLong ? kit.t('hello.tooLong') : kit.t('hello.help')}</p>
        <button
            type="button"
            class="hmm-button kit-primary"
            onclick={save}
            disabled={saving || tooLong || !kit.session.can('write')}
            data-testid="hello-save"
        >
            {kit.t('hello.save')}
        </button>
    {/if}
</section>

<style>
    .hello-field {
        display: flex;
        align-items: center;
        gap: 10px;
    }

    .hello-field input {
        width: min(28em, 100%);
    }
</style>
