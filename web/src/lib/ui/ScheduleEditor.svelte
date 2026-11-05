<script lang="ts">
  import type { Schedule } from '../types';
  import Toggle from './Toggle.svelte';

  let { schedule = $bindable() }: { schedule: Schedule } = $props();
</script>

<div class="rb-inline">
  <select bind:value={schedule.mode}>
    <option value="cron">Cron</option>
    <option value="interval">Interval</option>
    <option value="off">Off</option>
  </select>
  {#if schedule.mode === 'cron'}
    <input type="text" class="rb-mono" autocomplete="off" spellcheck="false" bind:value={schedule.cron} />
  {:else if schedule.mode === 'interval'}
    <input type="number" min="1" bind:value={schedule.every} />
    <select bind:value={schedule.unit}>
      <option value="minutes">minutes</option>
      <option value="hours">hours</option>
      <option value="days">days</option>
    </select>
  {/if}
</div>
<div style="margin-top:12px">
  <Toggle bind:checked={schedule.catch_up} label="Run missed backups when the server is back on" />
</div>
