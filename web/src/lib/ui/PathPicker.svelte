<script lang="ts">
  import Dialog from './Dialog.svelte';
  import Button from './Button.svelte';
  import Field from './Field.svelte';
  import Icon from './Icon.svelte';
  import { onMount } from 'svelte';
  import { api } from '../api';
  import { fail } from '../state.svelte';

  interface BrowseEntry {
    name: string;
    path: string;
    dir: boolean;
  }

  let {
    title = 'Choose a folder',
    confirm = 'Choose',
    start = '/mnt',
    create = false,
    multiple = false,
    files = false,
    close
  }: {
    title?: string;
    confirm?: string;
    start?: string;
    create?: boolean;
    multiple?: boolean;
    files?: boolean;
    close: (r: string | string[] | null) => void;
  } = $props();

  let current = $state('');
  let typed = $state('');
  let entries = $state<BrowseEntry[]>([]);
  let picked = $state<string[]>([]);
  let loading = $state(false);
  let newName = $state<string | null>(null);
  let creating = $state(false);
  let first = true;

  let parts = $derived(current.split('/').filter(Boolean));
  let label = $derived(picked.length > 1 ? `${confirm} ${picked.length} folders` : confirm);

  async function load(path: string) {
    loading = true;
    try {
      const r = await api<{ path: string; entries: BrowseEntry[] }>('browse', { path, files: files ? 1 : 0 });
      current = r.path;
      if (!first || r.path === path) typed = current === '/' ? '' : current;
      first = false;
      entries = r.entries;
    } catch (e) {
      fail(e);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    current = typed = start;
    load(start);
  });

  function tick(path: string, on: boolean) {
    const i = picked.indexOf(path);
    if (on && i === -1) picked.push(path);
    else if (!on && i !== -1) picked.splice(i, 1);
  }

  async function makeFolder() {
    const n = (newName ?? '').trim();
    if (!n) return;
    creating = true;
    try {
      const r = await api<{ path: string }>('mkdir', { path: current, name: n });
      newName = null;
      await load(r.path);
    } catch (e) {
      fail(e);
    } finally {
      creating = false;
    }
  }

  function choose() {
    const t = typed.trim() || current;
    close(multiple ? (picked.length ? [...picked] : [t]) : t);
  }
</script>

<Dialog {title} icon="folder" wide={multiple} onclose={() => close(null)}>
  <div class="rb-browser-head" style="padding:0 0 10px;border:0">
    <div class="rb-crumbs">
      <button type="button" onclick={() => load('/')}><Icon name="home" /></button>
      {#each parts as p, i (i)}
        <span class="rb-sep">/</span>
        <button type="button" onclick={() => load('/' + parts.slice(0, i + 1).join('/'))}>{p}</button>
      {/each}
    </div>
    {#if create}
      <Button icon="folderPlus" small kind="ghost" disabled={!current.startsWith('/mnt/')} onclick={() => (newName = '')}>New folder</Button>
    {/if}
  </div>
  {#if newName !== null}
    <div class="rb-newfolder">
      <Icon name="folderPlus" />
      <input
        type="text"
        placeholder="Name of the new folder"
        autocomplete="off"
        spellcheck="false"
        bind:value={newName}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            makeFolder();
          } else if (e.key === 'Escape') {
            e.stopPropagation();
            newName = null;
          }
        }}
      />
      <Button small kind="primary" busy={creating} onclick={makeFolder}>Create</Button>
      <Button small kind="ghost" onclick={() => (newName = null)}>Cancel</Button>
    </div>
  {/if}
  <div class="rb-table-wrap rb-browser-scroll rb-browser" style="max-height:50vh">
    <table class="rb-table">
      <tbody>
        {#each entries as e (e.path)}
          <tr class="clickable" onclick={() => (e.dir ? load(e.path) : (typed = e.path))}>
            {#if multiple}
              <td
                class="pick"
                onclick={(ev) => {
                  ev.stopPropagation();
                  if (current !== '/' && (e.dir || files)) tick(e.path, !picked.includes(e.path));
                }}
              >
                {#if current !== '/' && (e.dir || files)}
                  <input type="checkbox" checked={picked.includes(e.path)} onclick={(ev) => ev.stopPropagation()} onchange={(ev) => tick(e.path, ev.currentTarget.checked)} />
                {/if}
              </td>
            {/if}
            <td class="name"><Icon name={e.dir ? 'folder' : 'file'} class={e.dir ? 'dir' : 'file'} /><span>{e.name}</span></td>
          </tr>
        {:else}
          <tr><td class="dim" colspan="2">{loading ? 'Loading...' : 'No folders here.'}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if multiple}
    <div class="rb-picked">
      {#each picked as p (p)}
        <span class="rb-picked-item" title={p}>
          <span class="rb-mono">{p}</span>
          <button type="button" title="Remove" onclick={() => tick(p, false)}><Icon name="x" /></button>
        </span>
      {:else}
        <span class="rb-hint">Tick folders to add several at once, from different places too.</span>
      {/each}
    </div>
  {/if}
  <div style="margin-top:12px">
    <Field label="Path">
      <input type="text" class="rb-mono" autocomplete="off" spellcheck="false" bind:value={typed} />
    </Field>
  </div>
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close(null)}>Cancel</Button>
    <Button kind="primary" onclick={choose}>{label}</Button>
  {/snippet}
</Dialog>
