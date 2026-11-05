<script lang="ts">
  import Button from '../lib/ui/Button.svelte';
  import OpProgress from './OpProgress.svelte';
  import { relative } from '../lib/format';
  import type { Op } from '../lib/types';
  import { stopOp } from './ops';

  let { op }: { op: Op } = $props();

  let stop = $state({ busy: false });
</script>

<div class="rb-card is-running restore-card">
  <div class="rb-card-head">
    <span class="rb-dot run"></span>
    <div class="rb-card-title">
      <h3>Restore in progress</h3>
      <div class="rb-meta">Started {relative(op.started)}</div>
    </div>
    <Button icon="stop" small busy={stop.busy} onclick={() => stopOp(op, stop)}>Stop</Button>
  </div>
  <OpProgress {op} />
</div>

<style>
  .restore-card {
    margin-bottom: 14px;
  }
</style>
