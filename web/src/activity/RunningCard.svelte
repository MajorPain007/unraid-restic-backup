<script lang="ts">
  import Button from '../lib/ui/Button.svelte';
  import ProgressBlock from '../lib/ui/ProgressBlock.svelte';
  import { api } from '../lib/api';
  import { confirm } from '../lib/dialogs';
  import { actions } from '../lib/actions';
  import { busy, fail, refresh, toast } from '../lib/state.svelte';
  import { relative } from '../lib/format';
  import type { Op } from '../lib/types';
  import { what } from './describe';

  let { op }: { op: Op } = $props();
  let stopping = $state({ busy: false });

  async function stop() {
    const yes = await confirm({
      title: 'Stop this operation?',
      text:
        op.kind === 'backup'
          ? 'The backup ends without a snapshot. Containers or VMs it stopped are started again.'
          : 'It ends as soon as restic has cleaned up.',
      confirm: 'Stop',
      danger: true
    });
    if (!yes) return;
    try {
      await busy(stopping, api('op_stop', { id: op.id }));
      toast('Stopping...', 'info');
      setTimeout(refresh, 1500);
    } catch (e) {
      fail(e);
    }
  }
</script>

<div class="rb-card is-running">
  <div class="rb-card-head">
    <span class="rb-dot run"></span>
    <div class="rb-card-title">
      <h3>{what(op)}</h3>
      <div class="rb-meta">Started {relative(op.started)}{op.trigger === 'schedule' ? ' by the schedule' : ''}</div>
    </div>
    <div class="rb-card-actions">
      <Button icon="list" small onclick={() => actions.showLog(op.id)}>Log</Button>
      <Button icon="stop" small busy={stopping.busy} onclick={stop}>Stop</Button>
    </div>
  </div>
  <ProgressBlock {op} />
</div>
