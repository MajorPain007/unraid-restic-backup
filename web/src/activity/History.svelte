<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import StatusPill from './StatusPill.svelte';
  import { api } from '../lib/api';
  import { actions } from '../lib/actions';
  import { app } from '../lib/state.svelte';
  import { clock, dateTime, dayLabel, duration, plural, relative } from '../lib/format';
  import type { Op } from '../lib/types';
  import { KIND, result, what } from './describe';
  import { filters } from './filters.svelte';

  const LIMIT = 300;
  const RESULTS: [string, string][] = [
    ['', 'Any result'],
    ['success', 'Succeeded'],
    ['warning', 'Warnings'],
    ['error', 'Failed'],
    ['cancelled', 'Stopped']
  ];

  let history = $state<Op[] | null>(null);
  let error = $state('');
  let loading = $state(false);
  let seq = 0;

  let o = $derived(app.overview);
  let filtered = $derived(!!(filters.job || filters.repo || filters.kind || filters.status));

  async function load(quiet: boolean) {
    const mine = ++seq;
    if (!quiet) {
      loading = true;
      history = null;
    }
    try {
      const r = await api<{ history: Op[] }>('history', { ...filters, limit: LIMIT });
      if (mine !== seq) return;
      history = r.history;
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      if (!quiet || !history) error = (e as Error).message;
    } finally {
      if (mine === seq) loading = false;
    }
  }

  let first = true;
  $effect(() => {
    app.overview?.now;
    untrack(() => {
      load(!first);
      first = false;
    });
  });

  function change(key: keyof typeof filters, value: string) {
    filters[key] = value;
    load(false);
  }

  function clear() {
    filters.job = filters.repo = filters.kind = filters.status = '';
    load(false);
  }

  function took(r: Op): number {
    return Math.max(0, (r.ended ?? r.started) - r.started);
  }

  let longest = $derived(Math.max(1, ...(history ?? []).map(took)));

  let groups = $derived.by(() => {
    const out: { label: string; items: Op[] }[] = [];
    for (const r of history ?? []) {
      const label = dayLabel(r.started);
      const last = out[out.length - 1];
      if (last && last.label === label) last.items.push(r);
      else out.push({ label, items: [r] });
    }
    return out;
  });

  let tally = $derived({
    failed: (history ?? []).filter((r) => r.status === 'error').length,
    warned: (history ?? []).filter((r) => r.status === 'warning').length
  });

  function textClass(status: string): string {
    return status === 'error' ? 'rb-err-text' : status === 'warning' ? 'rb-warn-text' : 'rb-dim';
  }
</script>

<div class="rb-section-head"><h2>History</h2></div>
{#if o}
  <div class="rb-filters">
    <select aria-label="Job" value={filters.job} onchange={(e) => change('job', e.currentTarget.value)}>
      <option value="">All jobs</option>
      {#each o.config.jobs as j (j.id)}<option value={j.id}>{j.name}</option>{/each}
    </select>
    <select aria-label="Repository" value={filters.repo} onchange={(e) => change('repo', e.currentTarget.value)}>
      <option value="">All repositories</option>
      {#each o.config.repos as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
    </select>
    <select aria-label="Kind" value={filters.kind} onchange={(e) => change('kind', e.currentTarget.value)}>
      <option value="">Everything</option>
      {#each Object.entries(KIND) as [k, label] (k)}<option value={k}>{label}</option>{/each}
    </select>
    <select aria-label="Result" value={filters.status} onchange={(e) => change('status', e.currentTarget.value)}>
      {#each RESULTS as [v, label] (v)}<option value={v}>{label}</option>{/each}
    </select>
    {#if filtered}
      <button type="button" class="rb-link rb-clear" onclick={clear}><Icon name="x" />Clear filters</button>
    {/if}
  </div>
{/if}

{#if error}
  <div class="rb-card-note err"><Icon name="alert" /><div>{error}</div></div>
{:else if loading || !history}
  <div class="rb-hint"><Icon name="loader" class="rb-spin" /> Loading...</div>
{:else if !history.length}
  <div class="rb-empty">
    <Icon name="activity" class="big" />
    <h3>Nothing yet</h3>
    <p>Backups, clean-ups, checks and restores appear here once they have run.</p>
  </div>
{:else}
  <div class="rb-hint rb-tally">
    {history.length >= LIMIT ? 'The latest ' + LIMIT + ' runs' : plural(history.length, 'run')}{#if tally.failed}<span class="rb-sep">·</span><span class="rb-err-text">{tally.failed} failed</span>{/if}{#if tally.warned}<span class="rb-sep">·</span><span class="rb-warn-text">{tally.warned} with warnings</span>{/if}
  </div>
  <div class="rb-table-wrap">
    <table class="rb-table rb-history">
      <thead>
        <tr>
          <th>Result</th>
          <th>What</th>
          <th class="rb-hide-narrow">Started</th>
          <th class="num rb-hide-narrow">Took</th>
          <th class="rb-hide-narrow">Details</th>
        </tr>
      </thead>
      <tbody>
        {#each groups as g (g.label + g.items[0].id)}
          <tr class="rb-day"><td colspan="5">{g.label}</td></tr>
          {#each g.items as rec (rec.id)}
            <tr
              class="clickable"
              title="Show the log"
              tabindex="0"
              onclick={() => actions.showLog(rec.id)}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  actions.showLog(rec.id);
                }
              }}
            >
              <td class="shrink"><StatusPill status={rec.status} /></td>
              <td>
                <div>{what(rec)}</div>
                <div class="rb-hint">{rec.trigger === 'schedule' ? 'Scheduled' : 'By hand'}<span class="rb-narrow-only">{" · " + clock(rec.started) + " · " + duration(took(rec))}</span></div>
              </td>
              <td class="dim rb-hide-narrow" title={dateTime(rec.started) + ' (' + relative(rec.started) + ')'}>{clock(rec.started)}</td>
              <td class="num rb-hide-narrow">
                <div>{duration(took(rec))}</div>
                <div class="rb-took" aria-hidden="true"><span style:width={Math.max(3, Math.sqrt(took(rec) / longest) * 100).toFixed(1) + '%'}></span></div>
              </td>
              <td class="rb-hide-narrow rb-details">
                <div class={textClass(rec.status)} title={result(rec)}>{result(rec)}</div>
              </td>
            </tr>
          {/each}
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<style>
  .rb-clear {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: 0;
    background: none;
    padding: 0 4px;
    font-size: 12.5px;
  }
  .rb-tally {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    margin: -4px 0 8px;
  }
  .rb-tally .rb-sep {
    opacity: 0.5;
  }
  :global(#rb-app) table.rb-history tr.rb-day td {
    padding-top: 9px;
    padding-bottom: 5px;
    background: var(--rb-surface-2);
    color: var(--rb-dim);
    font-size: 11.5px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    cursor: default;
  }
  .rb-took {
    width: 72px;
    height: 3px;
    margin: 4px 0 0 auto;
    border-radius: 2px;
    background: var(--rb-surface-3);
    overflow: hidden;
  }
  .rb-took span {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--rb-dim);
    opacity: 0.6;
  }
  .rb-details {
    max-width: 420px;
  }
  .rb-details div {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .rb-narrow-only {
    display: none;
  }
  tr.clickable:focus-visible {
    outline: 2px solid var(--rb-accent);
    outline-offset: -2px;
  }
  @media (max-width: 560px) {
    .rb-narrow-only {
      display: inline;
    }
  }
</style>
