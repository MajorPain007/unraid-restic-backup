<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { bytes, dateTime } from '../lib/format';
  import type { Snapshot } from '../lib/types';
  import { jobName, parentOf, type FindResult } from './store.svelte';

  interface Change {
    modifier: string;
    path: string;
  }

  interface Diff {
    statistics?: {
      added?: { files?: number; bytes?: number };
      removed?: { files?: number; bytes?: number };
      changed_files?: number;
    };
    changes: Change[];
    total: number;
  }

  type Kind = 'add' | 'del' | 'mod';

  let {
    repo,
    snap,
    list,
    close
  }: { repo: string; snap: Snapshot; list: Snapshot[]; close: (r?: FindResult) => void } = $props();

  let other = $state('');
  let filter = $state('');
  let data = $state<Diff | null>(null);
  let error = $state('');
  let loading = $state(false);
  let kinds = $state<Record<Kind, boolean>>({ add: true, del: true, mod: true });
  let seq = 0;

  let pair = $derived.by(() => {
    const o = list.find((x) => x.id === other);
    if (!o) return null;
    const from = o.time < snap.time ? o : snap;
    return { from, to: from === snap ? o : snap };
  });

  let rows = $derived.by(() => {
    if (!data) return [];
    const q = filter.toLowerCase();
    return data.changes.filter((c) => kinds[kindOf(c)] && (!q || c.path.toLowerCase().includes(q)));
  });

  onMount(() => {
    const older = list.filter((x) => x.time < snap.time && x.job === snap.job);
    other = (older.length ? older[0] : list[0]).id;
    load();
  });

  function kindOf(c: Change): Kind {
    return c.modifier === '+' ? 'add' : c.modifier === '-' ? 'del' : 'mod';
  }

  async function load() {
    if (!pair) return;
    const my = ++seq;
    loading = true;
    error = '';
    data = null;
    try {
      const r = await api<Diff>('diff', { repo, from: pair.from.id, to: pair.to.id });
      if (my !== seq) return;
      data = r;
    } catch (e) {
      if (my !== seq) return;
      error = (e as Error).message;
    } finally {
      if (my === seq) loading = false;
    }
  }

  function toggle(k: Kind) {
    const on = Object.values(kinds).filter(Boolean).length;
    if (kinds[k] && on === 3) kinds = { add: false, del: false, mod: false, [k]: true };
    else if (kinds[k] && on === 1) kinds = { add: true, del: true, mod: true };
    else kinds[k] = !kinds[k];
  }

  function reveal(c: Change) {
    if (!pair) return;
    const s = kindOf(c) === 'del' ? pair.from : pair.to;
    const dir = c.path.endsWith('/');
    const path = dir ? c.path.replace(/\/+$/, '') || '/' : c.path;
    close({ snap: s, path: dir ? path : parentOf(path), focus: dir ? '' : path });
  }
</script>

<Dialog title="Changes between snapshots" icon="compare" wide onclose={() => close()}>
  <div class="rb-fields">
    <Field label={'Compare ' + dateTime(snap.time) + ' with'}>
      <select bind:value={other} onchange={load}>
        {#each list as x (x.id)}
          <option value={x.id}>{dateTime(x.time) + ' · ' + jobName(x)}</option>
        {/each}
      </select>
    </Field>
    <Field label="Filter">
      <input type="search" placeholder="Filter paths" bind:value={filter} />
    </Field>
  </div>
  {#if loading}
    <div class="rb-hint loading"><Icon name="loader" class="rb-spin" /> Comparing...</div>
  {:else if error}
    <div class="rb-card-note err note"><Icon name="alert" /><div>{error}</div></div>
  {:else if data}
    {@const st = data.statistics || {}}
    <div class="rb-inline stats">
      <button type="button" class="rb-pill ok" class:off={!kinds.add} aria-pressed={kinds.add} title="Show only added files, click again for all" onclick={() => toggle('add')}>
        +{st.added?.files || 0} files · {bytes(st.added?.bytes || 0)}
      </button>
      <button type="button" class="rb-pill err" class:off={!kinds.del} aria-pressed={kinds.del} title="Show only removed files, click again for all" onclick={() => toggle('del')}>
        -{st.removed?.files || 0} files · {bytes(st.removed?.bytes || 0)}
      </button>
      <button type="button" class="rb-pill warn" class:off={!kinds.mod} aria-pressed={kinds.mod} title="Show only changed files, click again for all" onclick={() => toggle('mod')}>
        {st.changed_files || 0} changed
      </button>
      {#if pair}
        <span class="rb-hint">{dateTime(pair.from.time)} → {dateTime(pair.to.time)}</span>
      {/if}
    </div>
    <div class="rb-table-wrap diff">
      {#each rows.slice(0, 3000) as c, i (i)}
        <button type="button" class="rb-diff-line {kindOf(c)}" title="Show it in the snapshot" onclick={() => reveal(c)}>
          <b>{c.modifier}</b><span>{c.path}</span>
        </button>
      {:else}
        <div class="rb-hint empty">No differences.</div>
      {/each}
    </div>
    {#if data.total > data.changes.length}
      <p class="rb-hint">Showing {data.changes.length} of {data.total} changes.</p>
    {/if}
  {/if}
</Dialog>

<style>
  :global(#rb-app) .loading {
    margin-top: 14px;
  }
  .loading :global(.rb-spin) {
    animation: rb-spin 0.9s linear infinite;
  }
  :global(#rb-app) .note {
    margin-top: 14px;
  }
  :global(#rb-app) .stats {
    margin: 14px 0;
  }
  :global(#rb-app) .stats button {
    cursor: pointer;
  }
  :global(#rb-app) .stats button.off {
    opacity: 0.45;
  }
  :global(#rb-app) .diff {
    max-height: 50vh;
    overflow-y: auto;
  }
  :global(#rb-app) .diff button {
    width: 100%;
    text-align: left;
    cursor: pointer;
  }
  :global(#rb-app) .diff button:hover {
    background: var(--rb-surface-2);
  }
  :global(#rb-app) .diff span {
    overflow-wrap: anywhere;
  }
  :global(#rb-app) .empty {
    padding: 12px;
  }
</style>
