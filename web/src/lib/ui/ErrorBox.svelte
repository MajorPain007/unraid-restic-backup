<script lang="ts">
  import { errorsOf } from '../state.svelte';

  let { error }: { error: unknown } = $props();
  let list = $derived(errorsOf(error));
</script>

{#if error}
  <div class="rb-errors" role="alert">
    <div><b>{(error as Error).message || 'Please check the form.'}</b></div>
    {#if list.length}
      <ul>
        {#each list as e, i (i)}<li>{e}</li>{/each}
      </ul>
    {/if}
  </div>
{/if}
