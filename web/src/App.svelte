<script lang="ts">
  import Icon from './lib/ui/Icon.svelte';
  import Button from './lib/ui/Button.svelte';
  import Hosts from './lib/ui/Hosts.svelte';
  import UnlockBox from './lib/ui/UnlockBox.svelte';
  import JobsView from './views/JobsView.svelte';
  import ReposView from './views/ReposView.svelte';
  import SnapshotsView from './views/SnapshotsView.svelte';
  import ActivityView from './views/ActivityView.svelte';
  import SettingsView from './views/SettingsView.svelte';
  import { app, refresh, show, type Tab } from './lib/state.svelte';
  import { actions } from './lib/actions';
  import { plural, relative } from './lib/format';
  import type { Overview } from './lib/types';

  const TABS: [Tab, string, string][] = [
    ['jobs', 'Jobs', 'archive'],
    ['repos', 'Repositories', 'repo'],
    ['snapshots', 'Snapshots', 'layers'],
    ['activity', 'Activity', 'activity'],
    ['settings', 'Settings', 'settings']
  ];

  refresh();

  $effect(() => {
    const onVisible = () => {
      if (!document.hidden) refresh();
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  });

  function overallStatus(o: Overview): [string, string, string] {
    const running = o.running.filter((r) => r.kind === 'backup').length;
    let failed = 0;
    let warned = 0;
    let never = 0;
    for (const j of o.jobs) {
      const job = o.config.jobs.find((c) => c.id === j.id);
      if (!job || !job.enabled) continue;
      if (!j.last) never++;
      else if (j.last.status === 'error') failed++;
      else if (j.last.status === 'warning') warned++;
    }
    if (!o.config.jobs.length) return ['idle', o.config.repos.length ? 'No backup jobs yet' : 'Not set up yet', ''];
    if (failed) return ['err', plural(failed, 'backup') + ' failed', 'Look at the cards marked red.'];
    if (running) return ['run', plural(running, 'backup') + ' running', ''];
    if (warned) return ['warn', plural(warned, 'backup') + ' finished with warnings', ''];
    if (never === o.config.jobs.length) return ['idle', 'Ready', 'No backup has run yet.'];
    return ['ok', 'All backups are fine', ''];
  }

  let o = $derived(app.overview);
  let status = $derived(o ? overallStatus(o) : ['idle', '', '']);
  let scheduled = $derived(!!o && o.config.jobs.some((j) => j.enabled && j.schedule.mode !== 'off'));
  let fresh = $derived(!!o && !o.system.scheduler_tick && !!o.system.installed && o.now - o.system.installed < 180);
  let stale = $derived(!!o && scheduled && !fresh && (!o.system.scheduler_tick || o.now - o.system.scheduler_tick > 180));
  let counts = $derived<Record<string, number>>({
    jobs: o?.config.jobs.length ?? 0,
    repos: o?.config.repos.length ?? 0,
    activity: o?.running.length ?? 0,
    failed: o?.jobs.some((j) => j.last?.status === 'error') ? 1 : 0
  });
</script>

{#if !o}
  {#if app.loadError}
    <div class="rb-empty">
      <Icon name="alert" class="big" />
      <h3>The plugin could not load its data</h3>
      <p>{app.loadError}</p>
      <Button icon="refresh" onclick={() => refresh()}>Try again</Button>
    </div>
  {:else}
    <div class="rb-boot">Loading&hellip;</div>
  {/if}
{:else}
  <div class="rb-header">
    <div class="rb-header-main">
      <div class="rb-status-line">
        <span class="rb-dot {status[0]}" style="margin-top:0"></span>
        <span>{status[1]}</span>
        {#if status[2]}<span class="rb-sub">{status[2]}</span>{/if}
      </div>
      <div class="rb-sub" style="margin-top:3px">
        {o.system.restic ? 'restic ' + o.system.restic : 'restic not installed'} &nbsp;·&nbsp; {o.system.server}
      </div>
    </div>
    <div class="rb-header-actions">
      {#if !o.config.repos.length}
        <Button icon="plus" kind="primary" onclick={() => actions.editRepo(null)}>Add repository</Button>
      {:else}
        <Button icon="plus" kind="primary" onclick={() => actions.editJob(null)}>New job</Button>
      {/if}
    </div>
  </div>

  {#if !o.system.array_started}
    <div class="rb-card-note warn rb-banner"><Icon name="power" /><div>The array is stopped. Backups wait until it is started; missed ones run then.</div></div>
  {/if}
  {#if !o.system.restic}
    <div class="rb-card-note err rb-banner"><Icon name="alert" /><div>restic is not installed. Reinstall the plugin, and check the plugin log for why the download failed.</div></div>
  {/if}
  {#if stale}
    <div class="rb-card-note err rb-banner"><Icon name="clock" /><div>
      {#if o.system.scheduler_tick}
        The scheduler last ran {relative(o.system.scheduler_tick)} - scheduled backups are not starting. Reinstalling the plugin registers it with cron again.
      {:else}
        The scheduler has not run yet. If this stays, scheduled backups will not start - reinstalling the plugin registers it with cron again.
      {/if}
    </div></div>
  {/if}
  {#if o.system.secrets?.locked}
    <UnlockBox />
  {:else if o.system.secrets?.problem}
    <div class="rb-card-note err rb-banner"><Icon name="lock" /><div>{o.system.secrets.problem}</div></div>
  {/if}
  {#if o.system.array_started && !o.system.data_dir_ok && o.config.jobs.length}
    <div class="rb-card-note warn rb-banner"><Icon name="folder" /><div>
      The data folder {o.config.settings.data_dir} does not exist yet. It is created with the first backup; if it is on a disk that is gone, choose another in Settings.
    </div></div>
  {/if}

  <div class="rb-tabs" role="tablist">
    {#each TABS as [id, label, icon] (id)}
      <button type="button" role="tab" class="rb-tab" class:active={app.tab === id} aria-selected={app.tab === id} onclick={() => show(id)}>
        <Icon name={icon} /><span class="rb-tab-label">{label}</span>
        {#if counts[id]}<span class="rb-count" class:err={id === 'jobs' && counts.failed}>{counts[id]}</span>{/if}
      </button>
    {/each}
  </div>

  <div class="rb-view">
    {#key app.viewKey}
      {#if app.tab === 'jobs'}
        <JobsView opts={app.viewOpts} />
      {:else if app.tab === 'repos'}
        <ReposView opts={app.viewOpts} />
      {:else if app.tab === 'snapshots'}
        <SnapshotsView opts={app.viewOpts} />
      {:else if app.tab === 'activity'}
        <ActivityView opts={app.viewOpts} />
      {:else}
        <SettingsView opts={app.viewOpts} />
      {/if}
    {/key}
  </div>
{/if}

<Hosts />

<style>
  .rb-banner {
    margin: 0 0 14px;
  }
</style>
