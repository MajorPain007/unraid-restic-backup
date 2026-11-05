<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import SecretsCard from '../settings/SecretsCard.svelte';
  import About from '../settings/About.svelte';
  import { api } from '../lib/api';
  import { pickPath } from '../lib/dialogs';
  import { app, busy, refresh, toast } from '../lib/state.svelte';
  import type { Settings } from '../lib/types';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  function clone(v: Settings): Settings {
    return JSON.parse(JSON.stringify(v));
  }

  function clean(v: Settings): Settings {
    return {
      ...v,
      nice: Number(v.nice),
      start_delay: Number(v.start_delay ?? 0),
      history_days: Number(v.history_days ?? 0)
    };
  }

  function fingerprint(v: Settings): string {
    const c = clean(v);
    return JSON.stringify([c.data_dir, c.host, c.nice, c.io_priority, c.start_delay, c.parity_pause, c.history_days, c.show_in_menu]);
  }

  const initial = clone(app.overview!.config.settings);
  let s = $state<Settings>(clone(initial));
  let saved = $state<Settings>(clone(initial));
  let error = $state<unknown>(null);
  let saving = $state({ busy: false });

  let dirty = $derived(fingerprint(s) !== fingerprint(saved));
  let server = $derived(app.overview?.system.server ?? '');

  $effect(() => {
    const incoming = app.overview?.config.settings;
    if (!incoming) return;
    const copy = clone(incoming);
    untrack(() => {
      if (dirty || fingerprint(copy) === fingerprint(saved)) return;
      s = clone(copy);
      saved = copy;
    });
  });

  $effect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  });

  async function browse() {
    const p = await pickPath({ title: 'Data folder', create: true, start: s.data_dir });
    if (p) s.data_dir = p;
  }

  async function save() {
    try {
      const r = await busy(saving, api<{ settings?: Settings }>('settings_save', { settings: clean(s) }));
      error = null;
      toast('Settings saved.');
      saved = clone(r.settings ?? clean(s));
      s = clone(saved);
      refresh();
    } catch (e) {
      error = e;
    }
  }

  function discard() {
    s = clone(saved);
    error = null;
  }
</script>

{#snippet head(title: string, icon: string)}
  <h3><Icon name={icon} />{title}</h3>
{/snippet}

<div class="rb-settings">
  <div class="rb-card rb-settings-card">
    <ErrorBox {error} />

    <div class="rb-form-section">
      {@render head('Data folder', 'folder')}
      <div class="rb-hint" style="margin-bottom:14px">
        restic's cache, the history of all runs, their logs and the schedule's memory. It has to be on a disk: the cache can grow to gigabytes,
        and Unraid keeps /tmp in RAM. The system share on a cache pool is ideal: like docker.img, nothing in it needs a backup of its own.
      </div>
      <div class="rb-fields">
        <Field label="Folder" wide>
          <div class="rb-input-group">
            <input
              id="rb-set-data_dir"
              type="text"
              class="rb-mono"
              autocomplete="off"
              spellcheck="false"
              data-lpignore="true"
              data-1p-ignore="true"
              bind:value={s.data_dir}
            />
            <Button icon="folder" onclick={browse}>Browse...</Button>
          </div>
        </Field>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('Running', 'activity')}
      <div class="rb-fields">
        <Field label="CPU priority" hint="How restic shares the processor with everything else.">
          <select bind:value={s.nice}>
            <option value={0}>Normal</option>
            <option value={10}>Lower (recommended)</option>
            <option value={19}>Lowest</option>
          </select>
        </Field>
        <Field label="Disk priority" hint={'"Only when idle" can stall a backup on a busy server.'}>
          <select bind:value={s.io_priority}>
            <option value="normal">Normal</option>
            <option value="low">Lower (recommended)</option>
            <option value="idle">Only when idle</option>
          </select>
        </Field>
        <Field label="Wait after the array starts" hint="Before missed backups catch up, so containers can start first.">
          <div class="rb-inline">
            <input type="number" min="0" max="120" autocomplete="off" bind:value={s.start_delay} />
            minutes
          </div>
        </Field>
        <Field label="History" hint="How long finished runs and their logs are listed.">
          <div class="rb-inline">
            <input type="number" min="7" autocomplete="off" bind:value={s.history_days} />
            days
          </div>
        </Field>
        <div class="rb-field wide">
          <Toggle bind:checked={s.parity_pause} label="No scheduled runs during a parity check" />
          <div class="rb-hint">They wait and run once it is done. Runs started by hand are not held back.</div>
        </div>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('Menu', 'archive')}
      <div>
        <Toggle bind:checked={s.show_in_menu} label={'Show "Backup" in the top menu, next to Docker and VMs'} />
        <div class="rb-hint" style="margin:4px 0 0 44px">
          The page stays under Settings &gt; Utilities either way. Takes effect when the page is loaded again.
        </div>
      </div>
    </div>

    <div class="rb-form-section">
      {@render head('Snapshots', 'layers')}
      <div class="rb-fields">
        <Field
          label="Host name"
          optional
          hint="Recorded with every snapshot. Change it only if this server should continue the snapshots of another."
        >
          <input type="text" autocomplete="off" spellcheck="false" placeholder={server} bind:value={s.host} />
        </Field>
      </div>
    </div>

    <div class="rb-card-foot rb-save-bar" class:dirty>
      {#if dirty}
        <span class="rb-dot warn"></span>
        <span class="rb-unsaved">Unsaved changes</span>
        <Button small kind="ghost" onclick={discard}>Discard</Button>
      {/if}
      <div class="rb-spacer" style="flex:1"></div>
      <Button kind="primary" icon="check" busy={saving.busy} onclick={save}>Save settings</Button>
    </div>
  </div>

  <SecretsCard />
  <About />
</div>

<style>
  .rb-settings {
    max-width: 860px;
  }
  .rb-settings :global(.rb-settings-card + .rb-settings-card) {
    margin-top: 16px;
  }
  .rb-save-bar {
    position: sticky;
    bottom: 0;
    z-index: 5;
    margin: 0 -18px -16px;
    padding: 12px 18px;
    border-top: 1px solid transparent;
    border-radius: 0 0 var(--rb-radius) var(--rb-radius);
    transition:
      background-color 0.15s,
      border-color 0.15s;
  }
  .rb-save-bar.dirty {
    background: var(--rb-panel);
    border-top-color: var(--rb-border-strong);
  }
  .rb-save-bar .rb-dot {
    margin-top: 0;
  }
  .rb-unsaved {
    font-size: 13px;
    color: var(--rb-warn);
    font-weight: 600;
  }
  @media (max-width: 560px) {
    .rb-save-bar {
      flex-wrap: wrap;
    }
  }
</style>
