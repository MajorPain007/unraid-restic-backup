<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import SnapshotList from '../snapshots/SnapshotList.svelte';
  import Browser from '../snapshots/Browser.svelte';
  import RestoreCard from '../snapshots/RestoreCard.svelte';
  import { app, busy, pref, repoConfig } from '../lib/state.svelte';
  import { NO_JOB, S, jobLabel, loadList, selectRepo } from '../snapshots/store.svelte';
  import { find } from '../snapshots/ops';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let repos = $derived(app.overview?.config.repos ?? []);
  let search = $state('');
  let reloading = $state({ busy: false });

  let restores = $derived((app.overview?.running ?? []).filter((op) => op.kind === 'restore' && op.repo === S.repo));

  let jobOptions = $derived.by(() => {
    const ids = new Set<string>();
    for (const s of S.list || []) ids.add(s.job);
    if (S.job && S.job !== NO_JOB) ids.add(S.job);
    return [...ids].map((id) => [id ? id : NO_JOB, jobLabel(id)] as [string, string]);
  });

  untrack(() => {
    if (!repos.length) return;
    if (opts.repo && opts.repo !== S.repo) selectRepo(opts.repo, opts.job || '');
    else if (opts.job !== undefined && opts.job !== S.job) S.job = opts.job;
    if (!S.list && !S.loading) S.error = null;
  });

  $effect(() => {
    if (!repos.length) return;
    if (S.repo && repoConfig(S.repo)) return;
    const saved = pref('snapRepo');
    untrack(() => selectRepo(saved && repoConfig(saved) ? saved : repos[0].id, ''));
  });

  $effect(() => {
    if (S.repo && repoConfig(S.repo) && !S.list && !S.loading && !S.error) untrack(() => loadList(false));
  });

  function runFind() {
    const q = search.trim();
    if (q) find(q);
  }
</script>

{#if !repos.length}
  <div class="rb-empty">
    <Icon name="layers" class="big" />
    <h3>No repository yet</h3>
    <p>Snapshots appear here once a repository is set up.</p>
  </div>
{:else}
  <div class="rb-toolbar">
    <select aria-label="Repository" value={S.repo} onchange={(e) => selectRepo(e.currentTarget.value, '')}>
      {#each repos as r (r.id)}
        <option value={r.id}>{r.name}</option>
      {/each}
    </select>
    <select aria-label="Job" bind:value={S.job}>
      <option value="">All jobs</option>
      {#each jobOptions as [id, label] (id)}
        <option value={id}>{label}</option>
      {/each}
    </select>
    <Button icon="refresh" title="Reload the snapshot list" busy={reloading.busy} onclick={() => busy(reloading, loadList(true))} />
    <div class="rb-spacer"></div>
    <input
      type="search"
      placeholder="Find files, e.g. *.jpg"
      aria-label="Find files"
      bind:value={search}
      onkeydown={(e) => {
        if (e.key === 'Enter') runFind();
      }}
    />
    <Button icon="search" onclick={runFind}>Find</Button>
  </div>

  {#each restores as op (op.id)}
    <RestoreCard {op} />
  {/each}

  <div class="rb-split">
    <SnapshotList />
    <Browser />
  </div>
{/if}
