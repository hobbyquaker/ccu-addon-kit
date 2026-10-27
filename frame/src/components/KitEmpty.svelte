<script lang="ts">
    let {text}: {text: string} = $props();
</script>

<p class="kit-empty" data-testid="kit-empty">{text}</p>

<style>
    .kit-empty {
        margin: 24px 0;
        color: var(--hmm-fg-muted);
        text-align: center;
    }
</style>
