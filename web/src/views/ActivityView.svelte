<script module lang="ts">
  import LogDialog from '../activity/LogDialog.svelte';
  import { provide } from '../lib/actions';
  import { openDialog } from '../lib/state.svelte';

  provide({
    showLog: (opId: string) => {
      openDialog(LogDialog, { id: opId });
    }
  });
</script>

<script lang="ts">
  import RunningCard from '../activity/RunningCard.svelte';
  import History from '../activity/History.svelte';
  import { app } from '../lib/state.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let running = $derived(app.overview?.running ?? []);

  $effect(() => {
    if (opts.log) openDialog(LogDialog, { id: String(opts.log) });
  });
</script>

{#if running.length}
  <div class="rb-section-head"><h2>Running</h2></div>
  <div class="rb-grid rb-running">
    {#each running as op (op.id)}
      <RunningCard {op} />
    {/each}
  </div>
{/if}

<History />

<style>
  .rb-running {
    margin-bottom: 8px;
  }
</style>
