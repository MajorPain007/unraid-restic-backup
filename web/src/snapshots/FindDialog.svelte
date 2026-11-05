<script lang="ts">
  import { onMount } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import FileIcon from './FileIcon.svelte';
  import { api } from '../lib/api';
  import { bytes, dateTime, plural } from '../lib/format';
  import type { Snapshot } from '../lib/types';
  import { jobName, parentOf, type FindResult } from './store.svelte';

  interface Match {
    path: string;
    type: string;
    size: number | null;
  }

  interface Group {
    snapshot: string;
    matches: Match[];
  }

  let {
    repo,
    pattern,
    snapshots,
    close
  }: { repo: string; pattern: string; snapshots: Snapshot[]; close: (r?: FindResult) => void } = $props();

  let loading = $state(true);
  let error = $state('');
  let groups = $state<Group[]>([]);
  let truncated = $state(false);

  let byId = $derived(new Map(snapshots.map((s) => [s.id, s])));
  let found = $derived(groups.filter((g) => g.matches.length));
  let total = $derived(found.reduce((n, g) => n + g.matches.length, 0));

  onMount(() => {
    api<{ results: Group[]; truncated?: boolean }>('find', { repo, pattern })
      .then((r) => {
        groups = r.results || [];
        truncated = !!r.truncated;
      })
      .catch((e) => (error = (e as Error).message))
      .finally(() => (loading = false));
  });

  function pick(g: Group, m: Match) {
    const s = byId.get(g.snapshot);
    if (!s) {
      close();
      return;
    }
    close({ snap: s, path: m.type === 'dir' ? m.path : parentOf(m.path), focus: m.type === 'dir' ? '' : m.path });
  }

  function heading(g: Group): string {
    const s = byId.get(g.snapshot);
    const when = s?.time ? dateTime(s.time) : g.snapshot.slice(0, 8);
    return when + ' · ' + plural(g.matches.length, 'match', 'matches');
  }
</script>

<Dialog title={'Find "' + pattern + '"'} icon="search" wide onclose={() => close()}>
  {#if loading}
    <div class="rb-hint loading"><Icon name="loader" class="rb-spin" /> Searching every snapshot...</div>
  {:else if error}
    <div class="rb-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if !total}
    <p>Nothing found. Patterns match whole names: try *{pattern}*.</p>
  {:else}
    {#each found as g (g.snapshot)}
      {@const s = byId.get(g.snapshot)}
      <div class="rb-section-head group-head">
        <h2>{heading(g)}</h2>
        {#if s}<span class="rb-hint">{jobName(s)}</span>{/if}
      </div>
      <div class="rb-table-wrap">
        <table class="rb-table">
          <tbody>
            {#each g.matches.slice(0, 200) as m, i (i)}
              <tr class="clickable" title="Show it in the snapshot" onclick={() => pick(g, m)}>
                <td class="name">
                  <span class="cell"><FileIcon type={m.type} name={m.path.slice(m.path.lastIndexOf('/') + 1)} /><span class="rb-mono">{m.path}</span></span>
                </td>
                <td class="num">{m.type === 'dir' ? '' : bytes(m.size)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/each}
    {#if truncated}
      <p class="rb-hint">Only the first 2000 matches are shown.</p>
    {/if}
  {/if}
</Dialog>

<style>
  :global(#rb-app) .rb-section-head.group-head {
    margin: 16px 0 8px;
  }
  .cell {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 0;
  }
  .cell :global(svg.rb-i.dir) {
    color: var(--rb-accent);
  }
  .cell :global(svg.rb-i.file) {
    color: var(--rb-dim);
  }
  .loading :global(.rb-spin) {
    animation: rb-spin 0.9s linear infinite;
  }
</style>
