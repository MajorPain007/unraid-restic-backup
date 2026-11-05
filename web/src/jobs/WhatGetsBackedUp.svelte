<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import RuleTree from './RuleTree.svelte';
  import PreviewDialog from './PreviewDialog.svelte';
  import { openDialog } from '../lib/state.svelte';
  import type { Job } from '../lib/types';

  let { job, close }: { job: Job; close: (r?: unknown) => void } = $props();

  let data = $state(untrack(() => job));
</script>

<Dialog title={`What "${job.name}" backs up`} icon="eye" wide onclose={() => close()}>
  <p class="rb-hint rb-wgb-intro">Open the folders to see what the backup takes and what its rules leave out. To change the rules, edit the job.</p>
  <RuleTree bind:job={data} />
  {#snippet actions()}
    <Button icon="activity" onclick={() => openDialog(PreviewDialog, { job: $state.snapshot(data) })}>Measure sizes...</Button>
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .rb-wgb-intro {
    margin-top: 0;
  }
</style>
