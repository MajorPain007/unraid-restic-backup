<script lang="ts">
  import { slide } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import ProgressBlock from '../lib/ui/ProgressBlock.svelte';
  import WhatGetsBackedUp from './WhatGetsBackedUp.svelte';
  import { actions } from '../lib/actions';
  import { app, openDialog, openMenu, repoConfig, show } from '../lib/state.svelte';
  import { bytes, count, dateTime, duration, now, relative } from '../lib/format';
  import { applyRetention, deleteJob, jobStatus, repoIcon, runJob, setEnabled, statusOf, stopOp } from './jobs';
  import type { Job, JobInfo } from '../lib/types';

  let { cfg, info }: { cfg: Job; info: Partial<JobInfo> } = $props();

  let repo = $derived(repoConfig(cfg.repo));
  let st = $derived(jobStatus(info, cfg));
  let last = $derived(info.last ?? null);
  let s = $derived(last?.summary ?? null);
  let running = $derived(info.running ?? null);
  let arrayStarted = $derived(!!app.overview?.system.array_started);
  let working = $state(false);

  let runs = $derived((info.recent ?? []).slice(0, 30).reverse());
  let okRuns = $derived(runs.filter((r) => r.status === 'success').length);

  async function run() {
    working = true;
    await runJob(cfg);
    working = false;
  }

  async function stop() {
    if (!running) return;
    working = true;
    await stopOp(running);
    working = false;
  }

  function menu(e: MouseEvent) {
    openMenu(e.currentTarget as Element, [
      { label: 'Edit', icon: 'edit', onclick: () => actions.editJob(cfg.id) },
      { label: 'Browse snapshots', icon: 'layers', onclick: () => show('snapshots', { repo: cfg.repo, job: cfg.id }) },
      { label: 'What gets backed up', icon: 'eye', onclick: () => openDialog(WhatGetsBackedUp, { job: $state.snapshot(cfg) }) },
      { label: 'Size history', icon: 'activity', onclick: () => actions.sizeHistory({ job: cfg.id }) },
      last ? { label: 'Last log', icon: 'list', onclick: () => actions.showLog(last.id) } : null,
      { label: 'Apply retention now', icon: 'trash', disabled: !cfg.retention.enabled, onclick: () => applyRetention(cfg) },
      '-',
      { label: cfg.enabled ? 'Disable' : 'Enable', icon: 'power', onclick: () => setEnabled(cfg, !cfg.enabled) },
      { label: 'Delete', icon: 'trash', danger: true, onclick: () => deleteJob(cfg) }
    ]);
  }

  let maxAdded = $derived(Math.max(0, ...runs.map((r) => r.added || 0)));

  function barHeight(added: number | null | undefined): string {
    if (!maxAdded || !added) return '6px';
    return (6 + 16 * Math.sqrt(added / maxAdded)).toFixed(1) + 'px';
  }

  function runTitle(r: { status: string; started: number; added: number | null | undefined }): string {
    return (
      dateTime(r.started) +
      ' - ' +
      statusOf(r.status)[1] +
      (r.added !== null && r.added !== undefined ? ' - ' + bytes(r.added) + ' added' : '')
    );
  }
</script>

<div
  class="rb-card"
  class:is-running={!!running}
  class:is-failed={last?.status === 'error' && !running}
  class:is-disabled={!cfg.enabled}
>
  <div class="rb-card-head">
    <span class="rb-dot {st[0]}" title={st[1]}></span>
    <div class="rb-card-title">
      <h3 title={cfg.name}>
        <button type="button" class="rb-jc-name" onclick={() => actions.editJob(cfg.id)}>{cfg.name}</button>
      </h3>
      <div class="rb-meta">
        {#if repo}
          <span class="rb-chip" title={repo.name}><Icon name={repoIcon(repo.type)} />{repo.name}</span>
        {:else}
          <span class="rb-chip"><Icon name="alert" />No repository</span>
        {/if}
        <span class="rb-sep">·</span>
        <span>{cfg.enabled ? info.schedule : 'Disabled'}</span>
      </div>
    </div>
    <div class="rb-card-actions">
      {#if running}
        <Button icon="stop" small busy={working} onclick={stop}>Stop</Button>
      {:else if info.queued}
        <Button icon="clock" small disabled title="Waits for the repository to be free">Queued</Button>
      {:else}
        <Button
          icon="play"
          small
          kind="primary"
          busy={working}
          disabled={!arrayStarted}
          title={arrayStarted ? '' : 'The array is stopped'}
          onclick={run}>Run now</Button
        >
      {/if}
      <Button icon="more" small kind="ghost" title="More" onclick={menu} />
    </div>
  </div>

  {#if running}
    <div transition:slide={{ duration: 180 }}>
      <ProgressBlock op={running} />
    </div>
  {:else}
    <div transition:slide={{ duration: 180 }}>
      <div class="rb-facts">
        <div class="rb-fact">
          <div class="rb-label">Last run</div>
          <div class="rb-value">
            {#if last}
              <span title={dateTime(last.ended)}>{relative(last.ended)} <small>· {st[1].toLowerCase()}</small></span>
            {:else}
              Never
            {/if}
          </div>
        </div>
        <div class="rb-fact">
          <div class="rb-label">Next run</div>
          <div class="rb-value">
            {#if cfg.enabled && info.next}
              <span title={dateTime(info.next)}>{info.next <= now() + 30 ? 'Due now' : relative(info.next)}</span>
            {:else}
              {cfg.enabled ? 'Only when started' : 'Disabled'}
            {/if}
          </div>
        </div>
        <div class="rb-fact">
          <div class="rb-label">Added</div>
          <div class="rb-value">
            {#if s && s.data_added_packed !== undefined}
              <span>{bytes(s.data_added_packed)} <small>· {count(s.files_new || 0)} new, {count(s.files_changed || 0)} changed</small></span>
            {:else}
              -
            {/if}
          </div>
        </div>
        <div class="rb-fact">
          <div class="rb-label">Duration</div>
          <div class="rb-value">{last ? duration(last.ended - last.started) : '-'}</div>
        </div>
      </div>
      {#if last && (last.status === 'error' || last.status === 'warning') && last.error}
        <div class="rb-card-note {last.status === 'error' ? 'err' : 'warn'}">
          <Icon name="alert" />
          <div>{last.error} <button type="button" class="rb-link" onclick={() => actions.showLog(last.id)}>Show log</button></div>
        </div>
      {/if}
    </div>
  {/if}

  <div class="rb-strip" role="group" aria-label="Recent runs{runs.length ? `: ${okRuns} of ${runs.length} succeeded` : ''}">
    {#each { length: 30 - runs.length } as _, i (i)}
      <span class="st-empty"></span>
    {/each}
    {#each runs as r (r.id)}
      <button
        type="button"
        class="st-{r.status}"
        style:height={barHeight(r.added)}
        title={runTitle(r)}
        aria-label={runTitle(r)}
        onclick={() => actions.showLog(r.id)}
      ></button>
    {/each}
  </div>
</div>

<style>
  :global(#rb-app) .rb-jc-name {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: inherit;
    color: inherit;
  }
  :global(#rb-app) .rb-jc-name:hover {
    color: var(--rb-accent-text);
  }
  :global(#rb-app) .rb-strip button {
    flex: 1 1 0;
    max-width: 10px;
    min-height: 6px;
    border-radius: 2px;
    background: var(--rb-ok);
    opacity: 0.85;
    transition: opacity 0.12s, transform 0.12s;
  }
  :global(#rb-app) .rb-strip button:hover,
  :global(#rb-app) .rb-strip button:focus-visible {
    opacity: 1;
    filter: brightness(1.15);
  }
  :global(#rb-app) .rb-strip button.st-warning {
    background: var(--rb-warn);
  }
  :global(#rb-app) .rb-strip button.st-error {
    background: var(--rb-err);
  }
  :global(#rb-app) .rb-strip button.st-cancelled {
    background: var(--rb-faint);
  }
  :global(#rb-app) .rb-strip button.st-running {
    background: var(--rb-info);
  }
</style>
