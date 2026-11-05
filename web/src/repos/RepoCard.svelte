<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import RepoProgress from './RepoProgress.svelte';
  import Sparkline from './Sparkline.svelte';
  import { api } from '../lib/api';
  import { actions } from '../lib/actions';
  import { app, openMenu, show } from '../lib/state.svelte';
  import { bytes, dateTime, relative } from '../lib/format';
  import type { Repo, RepoInfo } from '../lib/types';
  import { TYPE_LABEL, opLabel, repoIcon } from './util';
  import { connect, deleteRepo, keysDialog, measure, runMaintenance, showPassword, sshKeyDialog, unlock } from './flows';

  let { repo, info }: { repo: Repo; info: RepoInfo | null } = $props();

  let stats = $derived(info?.stats ?? null);
  let check = $derived(info?.check);
  let prune = $derived(info?.prune);
  let running = $derived(info?.running ?? null);
  let jobs = $derived((app.overview?.config.jobs ?? []).filter((j) => j.repo === repo.id));
  let history = $state<number[][]>([]);
  let measuredAt = $derived(info?.stats?.at ?? 0);

  let nextMaint = $derived.by(() => {
    const list: [string, number][] = [];
    if (repo.maintenance.prune.enabled && !repo.append_only && prune?.next) list.push(['Prune', prune.next]);
    if (repo.maintenance.check.enabled && check?.next) list.push(['Check', check.next]);
    list.sort((a, b) => a[1] - b[1]);
    return list[0] ?? null;
  });

  let stored = $derived(
    stats && stats.total_uncompressed_size > 0 ? Math.max(0, Math.min(1, stats.total_size / stats.total_uncompressed_size)) : null
  );

  $effect(() => {
    const id = repo.id;
    if (!measuredAt) return;
    let gone = false;
    api<{ repo: { points: number[][] } }>('size_history', { repo: id }).then(
      (d) => {
        if (!gone) history = (d.repo.points ?? []).slice().sort((a, b) => a[0] - b[0]).slice(-60);
      },
      () => {}
    );
    return () => {
      gone = true;
    };
  });

  function menu(anchor: Element) {
    const busy = !!running;
    openMenu(anchor, [
      { label: 'Edit', icon: 'edit', onclick: () => actions.editRepo(repo.id) },
      { label: 'Test connection', icon: 'refresh', onclick: () => connect(repo) },
      repo.type === 'sftp' && { label: 'SSH key', icon: 'key', onclick: () => sshKeyDialog(repo, false).then((go) => {
            if (go) connect(repo);
          }) },
      { label: 'Show password', icon: 'eye', onclick: () => showPassword(repo) },
      '-',
      { label: 'Check now', icon: 'shield', disabled: busy, onclick: () => runMaintenance(repo, 'check') },
      { label: 'Prune now', icon: 'trash', disabled: busy || repo.append_only, onclick: () => runMaintenance(repo, 'prune') },
      { label: 'Measure size', icon: 'repo', onclick: () => measure(repo) },
      { label: 'Size history', icon: 'activity', onclick: () => actions.sizeHistory({ repo: repo.id }) },
      { label: 'Keys', icon: 'key', onclick: () => keysDialog(repo) },
      { label: 'Remove stale locks', icon: 'unlock', disabled: busy, onclick: () => unlock(repo) },
      '-',
      { label: 'Delete', icon: 'trash', danger: true, onclick: () => deleteRepo(repo) }
    ]);
  }
</script>

<div class="rb-card" class:is-running={!!running}>
  <div class="rb-card-head">
    <span class="rb-tile"><Icon name={repoIcon(repo.type)} /></span>
    <div class="rb-card-title">
      <h3 title={repo.name}>{repo.name}</h3>
      <div class="rb-meta">{TYPE_LABEL[repo.type]}</div>
      <div class="rb-meta rb-mono rb-repo-url" title={info?.url}>{info?.url ?? ''}</div>
    </div>
    <div class="rb-card-actions">
      <Button icon="layers" small onclick={() => show('snapshots', { repo: repo.id })}>Snapshots</Button>
      <Button icon="more" small kind="ghost" title="More" onclick={(e) => menu(e.currentTarget as Element)} />
    </div>
  </div>

  {#if running}
    <div class="rb-meta" style="margin-top:14px">
      <Icon name="loader" class="rb-spin" />
      {running.kind === 'backup' ? 'Backup "' + running.job_name + '"' : opLabel(running.kind)}
    </div>
    <RepoProgress op={running} />
  {/if}

  <div class="rb-facts">
    <div class="rb-fact">
      <div class="rb-label">Size</div>
      <div class="rb-value">
        {#if stats}
          <span title={stats.at ? 'Measured ' + relative(stats.at) : null}>
            {bytes(stats.total_size)}
            {#if Math.round(stats.compression_space_saving || 0) >= 1}<small> · {Math.round(stats.compression_space_saving)}% saved by compression</small>{/if}
          </span>
        {:else}
          <span class="rb-dim">Not measured yet</span>
        {/if}
      </div>
    </div>
    <div class="rb-fact">
      <div class="rb-label">Snapshots</div>
      <div class="rb-value">{stats ? String(stats.snapshots_count) : '-'}</div>
    </div>
    <div class="rb-fact">
      <div class="rb-label">Last check</div>
      <div class="rb-value">
        {#if check?.last}
          <span class:rb-err-text={check.last.status === 'error'} title={dateTime(check.last.ended)}>
            {check.last.status === 'success'
              ? 'Passed ' + relative(check.last.ended)
              : check.last.status === 'error'
                ? 'Failed ' + relative(check.last.ended)
                : relative(check.last.ended)}
          </span>
        {:else}
          {repo.maintenance.check.enabled ? 'Not yet' : 'Off'}
        {/if}
      </div>
    </div>
    <div class="rb-fact">
      <div class="rb-label">Next maintenance</div>
      <div class="rb-value">
        {#if nextMaint}
          <span title={dateTime(nextMaint[1])}>{nextMaint[0] + ' ' + relative(nextMaint[1])}</span>
        {:else}
          None scheduled
        {/if}
      </div>
    </div>
  </div>

  {#if stats && stored !== null && stats.total_uncompressed_size > stats.total_size && Math.round(stats.compression_space_saving || 0) >= 1}
    <div
      class="rb-storage"
      title={bytes(stats.total_uncompressed_size) + ' of data, ' + bytes(stats.total_size) + ' on disk'}
    >
      <div class="rb-storage-track"><div class="rb-storage-bar" style:width="{(stored * 100).toFixed(1)}%"></div></div>
      <div class="rb-storage-text">
        <span>{bytes(stats.total_size)} on disk</span>
        <span>{bytes(stats.total_uncompressed_size)} of data</span>
      </div>
    </div>
  {/if}

  {#if history.length > 1}
    <Sparkline points={history} onclick={() => actions.sizeHistory({ repo: repo.id })} />
  {/if}

  {#if !repo.has_password}
    <div class="rb-card-note err"><Icon name="key" /><div>No password is stored for this repository.</div></div>
  {/if}
  {#if check?.last && check.last.status === 'error' && check.last.error}
    {@const id = check.last.id}
    <div class="rb-card-note err">
      <Icon name="alert" />
      <div>
        {check.last.error}
        <button type="button" class="rb-link" onclick={() => actions.showLog(id)}>Show log</button>
      </div>
    </div>
  {/if}
  {#if prune?.last && prune.last.status === 'error' && prune.last.error}
    {@const id = prune.last.id}
    <div class="rb-card-note warn">
      <Icon name="alert" />
      <div>
        Prune: {prune.last.error}
        <button type="button" class="rb-link" onclick={() => actions.showLog(id)}>Show log</button>
      </div>
    </div>
  {/if}
  {#if repo.append_only}
    <div class="rb-card-note info"><Icon name="lock" /><div>Append-only: old snapshots are removed on the server, not from here.</div></div>
  {/if}

  <div class="rb-card-foot">
    <span class="rb-meta">
      <Icon name="archive" />
      {jobs.length ? 'Used by ' + jobs.map((j) => j.name).join(', ') : 'No job uses it yet'}
    </span>
  </div>
</div>

<style>
  :global(#rb-app) .rb-repo-url {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-top: 2px;
  }
  .rb-storage {
    margin-top: 12px;
  }
  .rb-storage-track {
    height: 6px;
    border-radius: 3px;
    background: var(--rb-surface-3);
    overflow: hidden;
  }
  .rb-storage-bar {
    height: 100%;
    border-radius: 3px;
    background: var(--rb-accent);
    transition: width 0.6s ease;
  }
  .rb-storage-text {
    display: flex;
    justify-content: space-between;
    gap: 10px;
    margin-top: 5px;
    font-size: 12px;
    color: var(--rb-dim);
    font-variant-numeric: tabular-nums;
  }
</style>
