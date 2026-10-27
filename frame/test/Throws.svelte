<script lang="ts">
    let {active}: {active: boolean; path: string} = $props();

    function boom(): string {
        if (active) throw new Error('the tab broke');
        return 'never';
    }
</script>

<p>{boom()}</p>
