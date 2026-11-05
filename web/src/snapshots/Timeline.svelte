<script lang="ts">
  import type { Snapshot } from '../lib/types';
  import { dayLabel, plural, shortDate } from '../lib/format';

  let {
    snaps,
    active = '',
    onpick
  }: { snaps: Snapshot[]; active?: string; onpick: (day: string) => void } = $props();

  const DAY = 86400000;

  let days = $derived.by(() => {
    if (!snaps.length) return [];
    const today = new Date();
    const end = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
    const counts = new Map<string, number>();
    let oldest = end;
    for (const s of snaps) {
      const d = new Date(s.time * 1000);
      const key = d.toDateString();
      counts.set(key, (counts.get(key) || 0) + 1);
      oldest = Math.min(oldest, new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime());
    }
    const span = Math.max(14, Math.min(60, Math.round((end - oldest) / DAY) + 1));
    const out: { key: string; ts: number; n: number }[] = [];
    for (let i = span - 1; i >= 0; i--) {
      const dt = new Date(end - i * DAY + DAY / 2);
      const key = dt.toDateString();
      out.push({ key, ts: dt.getTime() / 1000, n: counts.get(key) || 0 });
    }
    return out;
  });

  let max = $derived(Math.max(1, ...days.map((d) => d.n)));
  let shown = $derived(days.reduce((n, d) => n + d.n, 0));
  let gaps = $derived(days.filter((d) => !d.n).length);
</script>

{#if days.length}
  <div class="timeline">
    <div class="bars" role="group" aria-label="Snapshots per day">
      {#each days as d (d.key)}
        <button
          type="button"
          class="bar"
          class:empty={!d.n}
          class:active={d.key === active}
          disabled={!d.n}
          title={dayLabel(d.ts) + ': ' + (d.n ? plural(d.n, 'snapshot') : 'no snapshot')}
          aria-label={dayLabel(d.ts) + ': ' + (d.n ? plural(d.n, 'snapshot') : 'no snapshot')}
          onclick={() => onpick(d.key)}
        >
          <span style:height={d.n ? Math.max(18, Math.round((Math.sqrt(d.n) / Math.sqrt(max)) * 100)) + '%' : null}></span>
        </button>
      {/each}
    </div>
    <div class="legend">
      <span>{shortDate(days[0].ts)}</span>
      <span class="mid">{plural(shown, 'snapshot')} in {days.length} days{gaps ? ' · ' + plural(gaps, 'day') + ' without' : ''}</span>
      <span>Today</span>
    </div>
  </div>
{/if}

<style>
  .timeline {
    padding: 10px 12px 8px;
    margin-bottom: 10px;
    border: 1px solid var(--rb-border);
    border-radius: var(--rb-radius);
    background: var(--rb-surface);
  }
  .bars {
    display: flex;
    align-items: stretch;
    gap: 2px;
    height: 34px;
  }
  :global(#rb-app) .bar {
    flex: 1 1 0;
    min-width: 0;
    display: flex;
    align-items: flex-end;
    padding: 0;
    border-radius: 2px;
    cursor: pointer;
  }
  :global(#rb-app) .bar:hover:not(:disabled) {
    background: var(--rb-surface-2);
  }
  :global(#rb-app) .bar:disabled {
    cursor: default;
  }
  .bar span {
    display: block;
    width: 100%;
    border-radius: 2px 2px 0 0;
    background: var(--rb-accent);
    opacity: 0.55;
  }
  .bar:hover span,
  .bar.active span {
    opacity: 1;
  }
  .bar.empty span {
    height: 2px;
    border-radius: 1px;
    background: var(--rb-border-strong);
    opacity: 1;
  }
  .legend {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 5px;
    font-size: 11px;
    color: var(--rb-faint);
    font-variant-numeric: tabular-nums;
  }
  .legend .mid {
    color: var(--rb-dim);
    text-align: center;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
