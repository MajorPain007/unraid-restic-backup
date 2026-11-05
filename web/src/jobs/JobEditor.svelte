<script lang="ts" module>
  const EXCLUDE_PRESETS: [string, string[]][] = [
    ['Recycle bins', ['**/.Recycle.Bin', '**/.Trash-*', '**/$RECYCLE.BIN']],
    ['Temporary files', ['*.tmp', '*.temp', '*.part', '*.partial', '**/incomplete']],
    ['macOS clutter', ['.DS_Store', '._*', '**/.Spotlight-V100', '**/.fseventsd']],
    [
      'Plex caches',
      [
        '**/Plex Media Server/Cache',
        '**/Plex Media Server/Logs',
        '**/Plex Media Server/Crash Reports',
        '**/Plex Media Server/Media/localhost/**/*.bif'
      ]
    ],
    ['Jellyfin caches', ['**/jellyfin/cache', '**/jellyfin/log', '**/transcodes']],
    ['Logs', ['*.log', '**/logs/*.log.*']]
  ];

  const SOURCE_PRESETS: [string, string][] = [
    ['appdata', '/mnt/user/appdata'],
    ['Flash drive', '/boot'],
    ['domains (VMs)', '/mnt/user/domains'],
    ['system', '/mnt/user/system']
  ];

  const SECTIONS: [string, string][] = [
    ['basics', 'Basics'],
    ['what', 'What'],
    ['schedule', 'Schedule'],
    ['keep', 'Keep'],
    ['consistency', 'Consistency'],
    ['notify', 'Notifications'],
    ['commands', 'Commands'],
    ['advanced', 'Advanced']
  ];
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import ScheduleEditor from '../lib/ui/ScheduleEditor.svelte';
  import RuleTree from './RuleTree.svelte';
  import ConsistencyPicker from './ConsistencyPicker.svelte';
  import PreviewDialog from './PreviewDialog.svelte';
  import RetentionPreview, { type RetentionItem } from './RetentionPreview.svelte';
  import { api } from '../lib/api';
  import { pickPath } from '../lib/dialogs';
  import { app, fail, jobConfig, openDialog, refresh, toast } from '../lib/state.svelte';
  import { blankJob, deleteJob, forSave, lines, normalizeJob, REPO_TYPE } from './jobs';
  import type { Job } from '../lib/types';

  let { id = null, close }: { id?: string | null; close: (r?: unknown) => void } = $props();

  const saved = untrack(() => (id ? jobConfig(id) : null));
  const isNew = !saved;
  const savedName = saved?.name ?? '';
  const initial = normalizeJob(saved ? ($state.snapshot(saved) as Job) : blankJob());
  let job = $state<Job>(initial);

  let exclText = $state(initial.excludes.join('\n'));
  let iexclText = $state(initial.iexcludes.join('\n'));
  let flagsText = $state(initial.advanced.extra_flags.join('\n'));
  let tagsText = $state(initial.tags.join(' '));

  let error = $state<unknown>(null);
  let saving = $state(false);
  let previewing = $state(false);
  let errorEl: HTMLElement | undefined = $state();
  let root: HTMLElement | undefined = $state();
  let advancedEl: HTMLDetailsElement | undefined = $state();
  const saveKeys = /Mac|iPhone|iPad/.test(navigator.platform) ? '\u2318S' : 'Ctrl+S';
  const uid = 'rbje' + Math.random().toString(36).slice(2, 8);

  let repos = $derived(app.overview?.config.repos ?? []);
  let server = $derived(app.overview?.system.server ?? '');

  function syncExcl() {
    exclText = job.excludes.join('\n');
  }

  function addSource(path: string) {
    const p = (path || '').trim().replace(/\/+$/, '');
    if (p && !job.sources.includes(p)) job.sources.push(p);
  }

  function insideOf(src: string): string | null {
    return job.sources.find((o) => o !== src && (o === '/' || src.startsWith(o + '/'))) ?? null;
  }

  async function addFolders() {
    const paths = await pickPath({ title: 'Add folders to back up', confirm: 'Add', multiple: true, start: job.sources[0] || '/mnt/user' });
    (paths || []).forEach(addSource);
  }

  function addPreset(patterns: string[]) {
    for (const pat of patterns) if (!job.excludes.includes(pat)) job.excludes.push(pat);
    syncExcl();
  }

  async function previewRetention() {
    previewing = true;
    try {
      const r = await api<{ keep: RetentionItem[]; remove: RetentionItem[] }>('retention_preview', {
        job: { id: job.id, retention: forSave(job).retention }
      });
      openDialog(RetentionPreview, { keep: r.keep, remove: r.remove });
    } catch (e) {
      fail(e);
    } finally {
      previewing = false;
    }
  }

  async function save() {
    if (saving) return;
    saving = true;
    try {
      const res = await api<{ job: Job }>('job_save', { job: forSave(job) });
      close('saved');
      toast(isNew ? `Job "${res.job.name}" created.` : 'Saved.');
      refresh();
    } catch (e) {
      error = e;
      setTimeout(() => errorEl?.scrollIntoView({ block: 'start', behavior: 'smooth' }), 0);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (saved && (await deleteJob(saved))) close('deleted');
  }

  function keydown(e: KeyboardEvent) {
    if (!root || !root.contains(e.target as Node)) return;
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'Enter')) {
      e.preventDefault();
      save();
    }
  }

  function jump(name: string) {
    if (name === 'advanced' && advancedEl) advancedEl.open = true;
    document.getElementById(uid + '-' + name)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }
</script>

<svelte:window onkeydown={keydown} />

{#snippet head(name: string, title: string, icon: string)}
  <h3 id="{uid}-{name}"><Icon name={icon} />{title}</h3>
{/snippet}

<Dialog drawer title={isNew ? 'New backup job' : `Edit "${savedName}"`} icon="archive" onclose={() => close()}>
  <div bind:this={root} class="rb-je">
    <nav class="rb-je-nav" aria-label="Sections">
      {#each SECTIONS as [name, label] (name)}
        <button type="button" onclick={() => jump(name)}>{label}</button>
      {/each}
    </nav>

    <div bind:this={errorEl} class="rb-je-error">
      {#if error}<div transition:slide={{ duration: 150 }}><ErrorBox {error} /></div>{/if}
    </div>

    <div class="rb-form-section">
      {@render head('basics', 'Basics', 'archive')}
      <div class="rb-fields">
        <Field label="Name">
          <input type="text" placeholder="e.g. Appdata to offsite" autocomplete="off" spellcheck="false" data-lpignore="true" data-1p-ignore="true" bind:value={job.name} />
        </Field>
        <Field label="Repository" hint="Where the snapshots go.">
          <select bind:value={job.repo}>
            <option value="">Choose a repository...</option>
            {#each repos as r (r.id)}
              <option value={r.id}>{r.name} ({REPO_TYPE[r.type] ?? r.type})</option>
            {/each}
          </select>
        </Field>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('what', 'What to back up', 'folder')}
      <div class="rb-pathlist">
        {#each job.sources as src, i (src)}
          {@const parent = insideOf(src)}
          <div class="rb-pathlist-item" transition:slide={{ duration: 150 }}>
            <Icon name="folder" />
            <span class="rb-mono" title={src}>{src}</span>
            {#if parent}
              <span class="rb-pill warn" title="Already backed up as part of {parent}">inside {parent}</span>
            {/if}
            <Button icon="x" small kind="ghost" title="Remove" onclick={() => job.sources.splice(i, 1)} />
          </div>
        {:else}
          <div class="rb-hint">No folders yet.</div>
        {/each}
      </div>
      <div class="rb-inline rb-je-gap">
        <Button icon="plus" small onclick={addFolders}>Add folders...</Button>
        <span class="rb-hint">Quick add:</span>
        {#each SOURCE_PRESETS as [label, path] (path)}
          <Button small kind="ghost" disabled={job.sources.includes(path)} title={path} onclick={() => addSource(path)}>{label}</Button>
        {/each}
      </div>

      <div class="rb-je-excludes">
        <Field label="Leave out">
          <textarea
            rows="5"
            spellcheck="false"
            placeholder={'*.tmp\n**/cache\n/mnt/user/appdata/plex/Library/Application Support/Plex Media Server/Cache'}
            bind:value={exclText}
            oninput={() => (job.excludes = lines(exclText))}
          ></textarea>
          {#snippet hintContent()}
            One pattern per line. <code>*</code> matches within a name, <code>**</code> across folders; a pattern starting with <code>/</code> is an
            absolute path.
          {/snippet}
        </Field>
        <div class="rb-presets rb-je-presets">
          {#each EXCLUDE_PRESETS as [label, patterns] (label)}
            {@const have = patterns.every((p) => job.excludes.includes(p))}
            <Button icon={have ? 'check' : 'plus'} small kind="ghost" disabled={have} title={patterns.join('\n')} onclick={() => addPreset(patterns)}
              >{label}</Button
            >
          {/each}
        </div>
        <div class="rb-fields rb-je-gap14">
          <div class="rb-field">
            <Toggle bind:checked={job.exclude_caches} label="Skip cache folders" />
            <div class="rb-hint">Folders marked with a CACHEDIR.TAG file, as many apps do for theirs.</div>
          </div>
          <Field label="Skip files larger than" optional>
            <input type="text" placeholder="e.g. 10G" autocomplete="off" spellcheck="false" bind:value={job.exclude_larger_than} />
          </Field>
        </div>
        <div class="rb-field rb-je-gap16">
          <span class="rb-field-label">What gets backed up</span>
          <RuleTree bind:job edit onchange={syncExcl} />
          <div class="rb-hint">
            Open the folders: <span class="rb-rtree-mark in rb-je-mark"><Icon name="check" /></span> is backed up,
            <span class="rb-rtree-mark out rb-je-mark"><Icon name="x" /></span> is left out. Click the mark to change it; the rules above follow.
          </div>
        </div>
        <div class="rb-inline rb-je-gap">
          <Button icon="activity" small onclick={() => openDialog(PreviewDialog, { job: forSave(job) })}>Measure sizes...</Button>
          <span class="rb-hint">Adds up what these rules back up and leave out, folder by folder. Goes through every file, so large shares take minutes.</span>
        </div>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('schedule', 'Schedule', 'calendar')}
      <ScheduleEditor bind:schedule={job.schedule} />
    </div>

    <div class="rb-form-section">
      {@render head('keep', 'Keep snapshots', 'layers')}
      <div class="rb-hint rb-je-hint">
        Each number keeps that many snapshots, one per hour, day, week and so on - the newest of each. The latest snapshot is always kept.
      </div>
      <Toggle bind:checked={job.retention.enabled} label="Remove old snapshots after each backup" />
      {#if job.retention.enabled}
        <div class="rb-je-gap14" transition:slide={{ duration: 150 }}>
          <div class="rb-fields">
            <Field label="Last"><input type="number" min="0" bind:value={job.retention.last} /></Field>
            <Field label="Hourly"><input type="number" min="0" bind:value={job.retention.hourly} /></Field>
            <Field label="Daily"><input type="number" min="0" bind:value={job.retention.daily} /></Field>
            <Field label="Weekly"><input type="number" min="0" bind:value={job.retention.weekly} /></Field>
            <Field label="Monthly"><input type="number" min="0" bind:value={job.retention.monthly} /></Field>
            <Field label="Yearly"><input type="number" min="0" bind:value={job.retention.yearly} /></Field>
            <Field label="Everything from the last" optional>
              <input type="text" placeholder="e.g. 14d, 6m, 1y" autocomplete="off" spellcheck="false" bind:value={job.retention.within} />
            </Field>
          </div>
        </div>
      {/if}
      <div class="rb-inline rb-je-gap">
        <Button icon="eye" small busy={previewing} disabled={isNew} title={isNew ? 'Save the job and run it first' : ''} onclick={previewRetention}>Preview</Button>
        <span class="rb-hint">Shows which snapshots this policy keeps and removes, without removing any.</span>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('consistency', 'Consistency', 'shield')}
      <div class="rb-hint rb-je-hint">For data that is written to while it is being backed up.</div>
      <OptionCards
        two
        bind:value={job.consistency.mode}
        options={[
          {
            value: 'none',
            title: 'Back up as it is',
            icon: 'file',
            text: 'Fine for media and documents. Files that change during the backup, such as databases, may be caught halfway.'
          },
          {
            value: 'zfs',
            title: 'ZFS snapshot',
            icon: 'layers',
            text: 'Backs up a snapshot taken at the start. Nothing stops; databases see it like a power cut, which they recover from.'
          },
          {
            value: 'zfs-stop',
            title: 'ZFS snapshot, brief stop',
            icon: 'shield',
            text: 'Stops the containers and VMs using the data for the seconds the snapshot takes. Consistent, nearly no downtime.'
          },
          {
            value: 'stop',
            title: 'Stop while backing up',
            icon: 'stop',
            text: 'For data not on ZFS: containers and VMs using it are stopped for the whole backup and started again after.'
          }
        ]}
      />
      {#if job.consistency.mode !== 'none'}
        <div class="rb-je-gap">
          {#key job.consistency.mode}
            <ConsistencyPicker sources={job.sources} bind:consistency={job.consistency} />
          {/key}
        </div>
      {/if}
    </div>

    <div class="rb-form-section">
      {@render head('notify', 'Notifications', 'bell')}
      <div class="rb-je-toggles">
        <Toggle bind:checked={job.notify.error} label="When a backup fails" />
        <Toggle bind:checked={job.notify.warning} label="When some files could not be read" />
        <Toggle bind:checked={job.notify.success} label="When a backup succeeds" />
      </div>
      <div class="rb-hint rb-je-gap8">
        Sent as Unraid notifications - to the browser, e-mail or an agent like Discord or Telegram, as set in Settings &gt; Notifications.
      </div>
      <div class="rb-je-gap14">
        <Field
          label="Healthcheck URL"
          optional
          hint="Pinged at the start and end of each backup (Healthchecks.io, Uptime Kuma, ...). It alerts you also when a backup does not run at all."
        >
          <input type="url" placeholder="https://hc-ping.com/your-uuid" autocomplete="off" spellcheck="false" bind:value={job.notify.healthcheck} />
        </Field>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('commands', 'Commands', 'terminal')}
      <div class="rb-fields">
        <Field label="Before the backup" wide optional hint="Runs as root with bash. If it fails, nothing is backed up.">
          <textarea
            rows="3"
            spellcheck="false"
            placeholder={'# e.g. dump a database first\ndocker exec mariadb mariadb-dump --all-databases > /mnt/user/appdata/mariadb/dump.sql'}
            bind:value={job.hooks.before}
          ></textarea>
        </Field>
        <Field label="After the backup" wide optional hint="Also after a failed backup. Variables: RB_JOB_NAME, RB_STATUS, RB_SNAPSHOT, RB_LOG.">
          <textarea
            rows="3"
            spellcheck="false"
            placeholder="# runs after the backup; $RB_STATUS is success, warning, error or cancelled"
            bind:value={job.hooks.after}
          ></textarea>
        </Field>
      </div>
    </div>

    <div class="rb-form-section" id="{uid}-advanced">
      <details class="rb-more" bind:this={advancedEl}>
        <summary>Advanced</summary>
        <div class="rb-fields">
          <Field label="Tags" optional hint="Added to every snapshot of this job.">
            <input
              type="text"
              placeholder="e.g. weekly important"
              autocomplete="off"
              spellcheck="false"
              bind:value={tagsText}
              oninput={() => (job.tags = tagsText.split(/[\s,]+/).filter(Boolean))}
            />
          </Field>
          <Field label="Host name" optional hint="Recorded with each snapshot. Leave empty for the server name.">
            <input type="text" placeholder={server} autocomplete="off" spellcheck="false" bind:value={job.advanced.host} />
          </Field>
          <Field label="Files read at once" hint="0 = restic's default (2). More helps on fast SSDs.">
            <input type="number" min="0" bind:value={job.advanced.read_concurrency} />
          </Field>
          <Field label="Leave out, ignoring case" wide optional>
            <textarea rows="3" spellcheck="false" bind:value={iexclText} oninput={() => (job.iexcludes = lines(iexclText))}></textarea>
          </Field>
          <Field label="Extra restic options" wide optional hint="One per line, as --name=value.">
            <textarea
              rows="2"
              spellcheck="false"
              placeholder="--exclude-if-present=.nobackup"
              bind:value={flagsText}
              oninput={() => (job.advanced.extra_flags = lines(flagsText))}
            ></textarea>
          </Field>
          <div class="rb-field"><Toggle bind:checked={job.one_file_system} label="Stay on one file system" /></div>
          <div class="rb-field"><Toggle bind:checked={job.advanced.skip_if_unchanged} label="No snapshot if nothing changed" /></div>
        </div>
      </details>
    </div>
  </div>

  {#snippet actions()}
    {#if !isNew}
      <Button icon="trash" kind="danger" onclick={remove}>Delete</Button>
    {/if}
    <div class="rb-spacer"></div>
    <span class="rb-hint rb-je-keys rb-hide-narrow">{saveKeys} saves</span>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" busy={saving} onclick={save}>{isNew ? 'Create job' : 'Save'}</Button>
  {/snippet}
</Dialog>

<style>
  .rb-je-nav {
    position: sticky;
    top: -18px;
    z-index: 2;
    display: flex;
    gap: 4px;
    margin: -18px -20px 0;
    padding: 8px 20px;
    overflow-x: auto;
    scrollbar-width: none;
    background: var(--rb-panel);
    border-bottom: 1px solid var(--rb-border);
  }
  :global(#rb-app) .rb-je-nav button {
    flex: none;
    padding: 3px 10px;
    border-radius: 20px;
    font-size: 12.5px;
    color: var(--rb-dim);
    white-space: nowrap;
  }
  :global(#rb-app) .rb-je-nav button:hover {
    color: var(--rb-text);
    background: var(--rb-surface-2);
  }
  .rb-je-error {
    scroll-margin-top: 48px;
  }
  .rb-je-error > div {
    padding-top: 14px;
  }
  .rb-je .rb-form-section > h3 {
    scroll-margin-top: 48px;
  }
  .rb-je-gap {
    margin-top: 12px;
  }
  .rb-je-gap8 {
    margin-top: 8px;
  }
  .rb-je-gap14 {
    margin-top: 14px;
  }
  .rb-je-gap16 {
    margin-top: 16px;
  }
  .rb-je-excludes {
    margin-top: 18px;
  }
  .rb-je-presets {
    margin-top: 8px;
  }
  .rb-je-hint {
    margin-bottom: 14px;
  }
  .rb-je-mark {
    vertical-align: middle;
  }
  .rb-je-toggles {
    display: grid;
    gap: 10px;
  }
  .rb-je-keys {
    font-size: 12px;
    margin-right: 4px;
  }
  @media (max-width: 560px) {
    .rb-je-nav {
      margin: -18px -14px 0;
      padding: 8px 14px;
    }
  }
</style>
