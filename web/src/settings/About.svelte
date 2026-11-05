<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import { app } from '../lib/state.svelte';

  let sys = $derived(app.overview?.system);
  let facts = $derived<[string, string][]>(
    sys
      ? [
          ['restic', sys.restic || 'not installed'],
          ['Server', sys.server],
          ['Time zone', sys.timezone],
          ['Array', sys.array_started ? 'Started' : 'Stopped']
        ]
      : []
  );
</script>

<div class="rb-card rb-settings-card">
  <div class="rb-form-section">
    <h3><Icon name="info" />About</h3>
    <div class="rb-facts">
      {#each facts as [label, value] (label)}
        <div class="rb-fact">
          <div class="rb-label">{label}</div>
          <div class="rb-value" title={value}>{value}</div>
        </div>
      {/each}
    </div>
  </div>
</div>
