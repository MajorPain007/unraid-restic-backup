<script lang="ts">
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import { api } from '../lib/api';
  import type { Job } from '../lib/types';

  interface Hit {
    name: string;
    running: boolean | null;
    path: string | null;
  }

  interface Detection {
    docker: boolean;
    libvirt: boolean;
    containers: Hit[];
    broad: Hit[];
    vms: Hit[];
    zfs: { ok: boolean; error: string; datasets: string[]; live: { path: string; why: string }[] };
  }

  let { sources, consistency = $bindable() }: { sources: string[]; consistency: Job['consistency'] } = $props();

  const c = consistency;
  c.containers ??= [];
  c.vms ??= [];
  c.skip ??= [];
  let firstTime = !c.containers.length && !c.vms.length && !c.skip.length;

  let d = $state<Detection | null>(null);
  let error = $state('');
  let loading = $state(false);
  let seq = 0;

  async function load() {
    const my = ++seq;
    loading = true;
    error = '';
    try {
      const r = await api<Detection>('consistency_detect', { sources: untrack(() => $state.snapshot(sources)) });
      if (my !== seq) return;
      if (firstTime && (consistency.mode === 'zfs-stop' || consistency.mode === 'stop')) {
        for (const x of r.containers) if (!consistency.skip.includes(x.name)) setChoice(x.name, 'container', true);
        if (r.libvirt || consistency.vms.length) {
          for (const x of r.vms) if (!consistency.skip.includes(x.name)) setChoice(x.name, 'vm', true);
        }
      }
      firstTime = false;
      d = r;
    } catch (e) {
      if (my === seq) error = (e as Error).message;
    } finally {
      if (my === seq) loading = false;
    }
  }

  let sourcesKey = $derived(JSON.stringify(sources));
  let loadedKey = '';

  $effect(() => {
    const k = sourcesKey;
    if (k === loadedKey) return;
    const delay = loadedKey === '' ? 0 : 600;
    const t = setTimeout(() => {
      loadedKey = k;
      load();
    }, delay);
    return () => clearTimeout(t);
  });

  function setChoice(name: string, kind: 'vm' | 'container', on: boolean) {
    const list = kind === 'vm' ? consistency.vms : consistency.containers;
    const i = list.indexOf(name);
    if (on && i === -1) list.push(name);
    else if (!on && i !== -1) list.splice(i, 1);
    const s = consistency.skip.indexOf(name);
    if (on && s !== -1) consistency.skip.splice(s, 1);
    else if (!on && s === -1) consistency.skip.push(name);
  }

  function setBroad(name: string, on: boolean) {
    const i = consistency.containers.indexOf(name);
    if (on && i === -1) consistency.containers.push(name);
    else if (!on && i !== -1) consistency.containers.splice(i, 1);
  }

  function withListed(items: Hit[], listed: string[]): Hit[] {
    const names = items.map((x) => x.name);
    return [...items, ...listed.filter((n) => !names.includes(n)).map((n) => ({ name: n, running: null, path: null }))];
  }

  function datasetSummary(names: string[]): string {
    return names
      .filter((n) => !names.some((o) => n.startsWith(o + '/')))
      .map((top) => {
        const inside = names.filter((n) => n.startsWith(top + '/')).length;
        return top + (inside ? ' (+' + inside + ' inside)' : '');
      })
      .join(', ');
  }

  let mode = $derived(consistency.mode);
  let stops = $derived(mode === 'zfs-stop' || mode === 'stop');
</script>

{#snippet item(x: Hit, kind: 'vm' | 'container', checked: boolean, onchange: (on: boolean) => void)}
  <label class="rb-pathlist-item rb-cp-item">
    <input type="checkbox" {checked} onchange={(e) => onchange(e.currentTarget.checked)} />
    <Icon name={kind === 'vm' ? 'server' : 'archive'} />
    <span class="rb-cp-name">
      <b>{x.name}</b>
      <span class="rb-hint">&nbsp; {x.path ? x.path : 'no longer uses these folders'}</span>
    </span>
    {#if x.running !== null}
      <span class="rb-pill" class:ok={x.running}>{x.running ? 'running' : 'stopped'}</span>
    {/if}
  </label>
{/snippet}

{#snippet itemList(items: Hit[], kind: 'vm' | 'container')}
  {@const listed = kind === 'vm' ? consistency.vms : consistency.containers}
  {@const all = withListed(items, listed)}
  {#if !all.length}
    <div class="rb-hint">{kind === 'vm' ? 'No VM has a disk in these folders.' : 'No container uses these folders.'}</div>
  {:else}
    <div class="rb-pathlist">
      {#each all as x (x.name)}
        {@render item(x, kind, listed.includes(x.name), (on) => setChoice(x.name, kind, on))}
      {/each}
    </div>
  {/if}
{/snippet}

<div>
  {#if error}
    <div class="rb-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if !d}
    <div class="rb-hint"><Icon name="loader" class="rb-spin" /> Looking at what uses these folders...</div>
  {:else}
    <div class:rb-cp-stale={loading} transition:slide={{ duration: 150 }}>
      {#if mode === 'zfs' || mode === 'zfs-stop'}
        {#if d.zfs.ok}
          <div class="rb-callout rb-cp-callout">
            <Icon name="check" />
            <div>
              Each backup snapshots <b>{datasetSummary(d.zfs.datasets)}</b> first and reads from the snapshot. The snapshot is removed afterwards.
              {#if d.zfs.live?.length}
                <div class="rb-cp-live">Not on ZFS, so read as they are: {d.zfs.live.map((l) => l.path + ' (' + l.why + ')').join(', ')}.</div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="rb-callout warn rb-cp-callout"><Icon name="alert" /><div>{d.zfs.error}</div></div>
        {/if}
      {/if}
      {#if stops}
        <div class="rb-field-label rb-cp-label">{mode === 'stop' ? 'Stopped while the backup runs' : 'Stopped for the snapshot'}</div>
        {#if !d.docker}
          <div class="rb-hint rb-cp-gap">Docker is not running, so containers could not be looked at.</div>
        {/if}
        {@render itemList(d.containers, 'container')}
        {#if d.broad?.length}
          <div class="rb-field-label rb-cp-label2">Also have access, through a parent folder</div>
          <div class="rb-hint rb-cp-gap">File managers and the like, with all of /mnt or a whole share mounted. They keep running unless you tick them.</div>
          <div class="rb-pathlist">
            {#each d.broad as x (x.name)}
              {@render item(x, 'container', consistency.containers.includes(x.name), (on) => setBroad(x.name, on))}
            {/each}
          </div>
        {/if}
        {#if d.libvirt || consistency.vms.length}
          <div class="rb-cp-spacer"></div>
          {@render itemList(d.vms, 'vm')}
        {/if}
        {#if d.vms.length || consistency.vms.length}
          <div class="rb-fields rb-cp-fields">
            <Field label="Wait for VMs to shut down" hint="If a VM is not off by then, the backup is skipped and everything is started again.">
              <div class="rb-inline">
                <input type="number" min="10" max="3600" bind:value={consistency.timeout} />seconds
              </div>
            </Field>
          </div>
        {/if}
        <div class="rb-hint rb-cp-note">
          Only what is running is stopped, and started again in Unraid's autostart order. Containers get as long to stop as set in Settings &gt;
          Docker, as when the array stops.
        </div>
      {/if}
    </div>
  {/if}
  {#if d || error}
    <div class="rb-cp-again">
      <Button icon="refresh" small kind="ghost" busy={loading} onclick={load}>Look again</Button>
    </div>
  {/if}
</div>

<style>
  .rb-cp-item {
    cursor: pointer;
  }
  .rb-cp-name {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .rb-cp-callout {
    margin-bottom: 14px;
  }
  .rb-cp-live {
    margin-top: 6px;
  }
  .rb-cp-label {
    margin: 4px 0 8px;
  }
  .rb-cp-label2 {
    margin: 14px 0 4px;
  }
  .rb-cp-gap {
    margin-bottom: 8px;
  }
  .rb-cp-spacer {
    height: 8px;
  }
  .rb-cp-fields {
    margin-top: 12px;
  }
  .rb-cp-note,
  .rb-cp-again {
    margin-top: 10px;
  }
  .rb-cp-stale {
    opacity: 0.6;
    transition: opacity 0.15s;
  }
</style>
