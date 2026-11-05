<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import Chart from './Chart.svelte';
  import { api } from '../lib/api';
  import { pref } from '../lib/state.svelte';
  import { bytes, count, plural } from '../lib/format';

  interface Series {
    id: string;
    name: string;
    points: number[][];
  }

  interface History {
    repo: Series;
    jobs: Series[];
  }

  type Range = '30' | '90' | '365' | 'all';

  const DAY = 86400;
  const RANGES: [Range, string][] = [
    ['30', '30 days'],
    ['90', '90 days'],
    ['365', '1 year'],
    ['all', 'All']
  ];

  let { which, close }: { which: { repo?: string; job?: string }; close: () => void } = $props();

  let data = $state<History | null>(null);
  let error = $state('');
  let range = $state<Range>(initialRange());
  const now = Math.floor(Date.now() / 1000);

  function initialRange(): Range {
    const v = pref('sizeRange');
    return RANGES.some(([r]) => r === v) ? (v as Range) : '90';
  }

  onMount(() => {
    api<History>('size_history', which).then(
      (d) => {
        data = {
          repo: { ...d.repo, points: d.repo.points ?? [] },
          jobs: (d.jobs ?? []).map((j) => ({ ...j, points: j.points ?? [] }))
        };
      },
      (e: Error) => (error = e.message)
    );
  });

  let from = $derived.by(() => {
    if (!data) return now - 90 * DAY;
    const all = [...data.repo.points, ...data.jobs.flatMap((j) => j.points)];
    const oldest = all.length ? Math.min(...all.map((p) => p[0])) - DAY : now - 30 * DAY;
    const f = range === 'all' ? oldest : Math.max(oldest, now - Number(range) * DAY);
    return Math.min(f, now - DAY);
  });

  function inRange(points: number[][]): number[][] {
    return points.filter((p) => p[0] >= from && p[0] <= now).sort((a, b) => a[0] - b[0]);
  }

  function trend(points: number[][], idx: number): { last: number; change: number | null } | null {
    const p = inRange(points);
    if (!p.length) return null;
    const last = p[p.length - 1][idx];
    return { last, change: p.length > 1 ? last - p[0][idx] : null };
  }

  function signed(n: number): string {
    if (n === 0) return 'unchanged';
    return (n > 0 ? '+' : '-') + bytes(Math.abs(n));
  }

  function rangeText(): string {
    return range === 'all' ? 'since the start' : 'in ' + (RANGES.find(([r]) => r === range)?.[1] ?? '');
  }

  function sumAdded(points: number[][]): number {
    return inRange(points).reduce((s, p) => s + (Number(p[2]) || 0), 0);
  }
</script>

{#snippet summary(t: { last: number; change: number | null } | null, extra: string)}
  {#if t}
    <span class="rb-size-sum">
      <b>{bytes(t.last)}</b>
      {#if t.change !== null}<span class:up={t.change > 0}>{signed(t.change)} {rangeText()}</span>{/if}
      {#if extra}<span>{extra}</span>{/if}
    </span>
  {/if}
{/snippet}

<Dialog title="Size history" icon="activity" wide onclose={() => close()}>
  {#if error}
    <div class="rb-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if !data}
    <div class="rb-hint"><Icon name="loader" class="rb-spin" /> Loading...</div>
  {:else}
    <div style="margin-bottom:14px">
      <Segmented bind:value={range} options={RANGES} onchange={(v) => pref('sizeRange', v)} />
    </div>
    <div class="rb-chart-block">
      <div class="rb-chart-title">
        <Icon name="repo" /><b>{data.repo.name}</b><span class="rb-hint">&nbsp;&nbsp;on disk</span>
        {@render summary(trend(data.repo.points, 1), '')}
      </div>
      <Chart
        points={data.repo.points}
        {from}
        to={now}
        label="Repository size"
        value={(p) => p[1]}
        detail={(p) => plural(p[2], 'snapshot')}
        delta
        empty="Measured after the next backup - or now with Measure size on the repository."
      />
    </div>
    {#each data.jobs as j (j.id)}
      {@const added = sumAdded(j.points)}
      <div class="rb-chart-block">
        <div class="rb-chart-title">
          <Icon name="archive" /><b>{j.name}</b><span class="rb-hint">&nbsp;&nbsp;backed up per run</span>
          {@render summary(trend(j.points, 1), added ? bytes(added) + ' added' : '')}
        </div>
        <Chart
          points={j.points}
          {from}
          to={now}
          height={150}
          label="Backed up"
          value={(p) => p[1]}
          detail={(p) => count(Number(p[3])) + ' files'}
          delta
          empty="Recorded with the next backup."
        />
        {#if inRange(j.points).length}
          <div class="rb-chart-sub">Added to the repository per run</div>
          <Chart points={j.points} {from} to={now} kind="bars" height={110} label="Added" value={(p) => p[2]} empty=" " />
        {/if}
      </div>
    {/each}
  {/if}
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .rb-size-sum {
    margin-left: auto;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 4px 10px;
    font-size: 12.5px;
    color: var(--rb-dim);
    font-variant-numeric: tabular-nums;
  }
  .rb-size-sum b {
    color: var(--rb-text);
    font-weight: 600;
  }
  .rb-size-sum .up {
    color: var(--rb-accent-text);
  }
  :global(#rb-app) .rb-chart-title {
    flex-wrap: wrap;
  }
</style>
