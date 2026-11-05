<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import FileIcon from './FileIcon.svelte';
  import { download } from '../lib/api';
  import { bytes, dateTime, plural } from '../lib/format';
  import type { Entry } from '../lib/types';
  import { S, goUp, jobName, navigate, type SortKey } from './store.svelte';
  import { compare, restoreItems, snapMenu } from './ops';

  let grid: HTMLElement | undefined = $state();
  let filterEl: HTMLInputElement | undefined = $state();

  let parts = $derived(S.path.split('/').filter(Boolean));

  let visible = $derived.by(() => {
    const q = S.filter.trim().toLowerCase();
    let list = (S.entries || []).filter((e) => !q || e.name.toLowerCase().includes(q));
    const { key, desc } = S.sort;
    if (key) {
      const dir = desc ? -1 : 1;
      list = [...list].sort((a, b) => {
        if (key === 'name') {
          const d = Number(b.type === 'dir') - Number(a.type === 'dir');
          return d || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }) * dir;
        }
        const av = key === 'size' ? (a.type === 'dir' ? -1 : a.size ?? 0) : a.mtime ?? 0;
        const bv = key === 'size' ? (b.type === 'dir' ? -1 : b.size ?? 0) : b.mtime ?? 0;
        return (av - bv) * dir || a.name.localeCompare(b.name);
      });
    }
    return list;
  });

  let items = $derived(Object.values(S.checked));
  let allChecked = $derived(visible.length > 0 && visible.every((e) => S.checked[e.path]));
  let someChecked = $derived(!allChecked && visible.some((e) => S.checked[e.path]));
  let hiddenChecked = $derived.by(() => {
    const seen = new Set(visible.map((e) => e.path));
    return items.filter((e) => !seen.has(e.path)).length;
  });
  let folderSize = $derived((S.entries || []).reduce((n, e) => n + (e.type === 'dir' ? 0 : e.size || 0), 0));

  $effect(() => {
    if (S.listing || !S.focus || !S.entries) return;
    const i = visible.findIndex((e) => e.path === S.focus);
    S.focus = '';
    if (i >= 0) {
      S.cursor = i;
      grid?.focus({ preventScroll: true });
      reveal();
    }
  });

  onMount(() => {
    tick().then(() => {
      if (grid) grid.scrollTop = S.scroll.browser;
    });
  });

  function reveal() {
    tick().then(() => grid?.querySelector('tr.cursor')?.scrollIntoView({ block: 'nearest' }));
  }

  function toggle(e: Entry, on = !S.checked[e.path]) {
    if (on) S.checked[e.path] = e;
    else delete S.checked[e.path];
  }

  function selectAll(on: boolean) {
    S.checked = {};
    if (on) for (const e of visible) S.checked[e.path] = e;
  }

  function activate(e: Entry) {
    if (e.type === 'dir') navigate(e.path);
    else toggle(e);
  }

  function rowClick(e: Entry, i: number) {
    S.cursor = i;
    activate(e);
  }

  function sortBy(key: SortKey) {
    if (S.sort.key === key) {
      if (S.sort.desc) S.sort = { key: '', desc: false };
      else S.sort.desc = true;
    } else S.sort = { key, desc: false };
  }

  function sortMark(key: SortKey): string {
    return S.sort.key === key ? (S.sort.desc ? ' ↓' : ' ↑') : '';
  }

  function ariaSort(key: SortKey): 'ascending' | 'descending' | 'none' {
    return S.sort.key === key ? (S.sort.desc ? 'descending' : 'ascending') : 'none';
  }

  function move(to: number) {
    if (!visible.length) return;
    S.cursor = Math.max(0, Math.min(visible.length - 1, to));
    reveal();
  }

  function keydown(ev: KeyboardEvent) {
    if (ev.target !== grid) return;
    const cur = visible[S.cursor];
    const mod = ev.ctrlKey || ev.metaKey;
    switch (ev.key) {
      case 'ArrowDown':
        move(S.cursor < 0 ? 0 : S.cursor + 1);
        break;
      case 'ArrowUp':
        move(S.cursor < 0 ? 0 : S.cursor - 1);
        break;
      case 'PageDown':
        move(S.cursor + 10);
        break;
      case 'PageUp':
        move(S.cursor - 10);
        break;
      case 'Home':
        move(0);
        break;
      case 'End':
        move(visible.length - 1);
        break;
      case 'Enter':
        if (!cur) return;
        activate(cur);
        break;
      case 'ArrowRight':
        if (!cur || cur.type !== 'dir') return;
        navigate(cur.path);
        break;
      case ' ':
        if (!cur) return;
        toggle(cur);
        break;
      case 'Backspace':
      case 'ArrowLeft':
        if (S.path === '/') return;
        goUp();
        break;
      case 'Escape':
        if (!items.length) return;
        S.checked = {};
        break;
      case '/':
        filterEl?.focus();
        break;
      case 'a':
      case 'A':
        if (!mod) return;
        selectAll(!allChecked);
        break;
      default:
        return;
    }
    ev.preventDefault();
    ev.stopPropagation();
  }

  function filterKey(ev: KeyboardEvent) {
    if (ev.key === 'ArrowDown' || ev.key === 'Enter') {
      ev.preventDefault();
      grid?.focus();
      if (visible.length) move(Math.max(0, S.cursor));
    } else if (ev.key === 'Escape' && S.filter) {
      ev.stopPropagation();
      S.filter = '';
    }
  }
</script>

{#if !S.snap}
  <div class="rb-browser">
    <div class="rb-empty none">
      <Icon name="layers" class="big" />
      <h3>Choose a snapshot</h3>
      <p>Its files appear here, to look through, restore or download.</p>
    </div>
  </div>
{:else}
  {@const s = S.snap}
  {@const sum = s.summary}
  <div class="rb-browser">
    <div class="rb-snap-info">
      <span><b>{dateTime(s.time)}</b></span>
      <span>{jobName(s)}</span>
      {#if sum}
        <span>Size <b>{bytes(sum.size)}</b></span>
        <span>Added <b>{bytes(sum.added)}</b></span>
        <span><b>{(sum.files || 0).toLocaleString('en-GB')}</b> files</span>
      {/if}
      <span class="rb-mono">{s.short_id}</span>
      <span class="grow"></span>
      <span class="info-actions">
        <Button icon="compare" small kind="ghost" onclick={() => compare(s)}>Compare</Button>
        <Button icon="more" small kind="ghost" title="More" onclick={(e) => snapMenu(s, e.currentTarget as Element)} />
      </span>
    </div>

    <div class="rb-browser-head">
      <div class="rb-crumbs">
        <button type="button" title="/" aria-label="/" onclick={() => navigate('/')}><Icon name="home" /></button>
        {#each parts as p, i (i)}
          <span class="rb-sep">/</span>
          <button type="button" onclick={() => navigate('/' + parts.slice(0, i + 1).join('/'))}>{p}</button>
        {/each}
      </div>
      {#if S.path !== '/'}
        <Button icon="up" small kind="ghost" title="Up" onclick={goUp} />
      {/if}
      {#if ((S.entries && S.entries.length > 8) || S.filter) && !S.listing}
        <input
          bind:this={filterEl}
          class="quick"
          type="search"
          placeholder="Filter this folder"
          aria-label="Filter this folder"
          bind:value={S.filter}
          oninput={() => (S.cursor = -1)}
          onkeydown={filterKey}
        />
      {/if}
    </div>

    <div
      class="rb-browser-scroll grid"
      bind:this={grid}
      role="grid"
      tabindex="0"
      aria-label="Folder contents"
      aria-busy={S.listing}
      aria-multiselectable="true"
      aria-activedescendant={S.cursor >= 0 && visible[S.cursor] ? 'rb-snaprow-' + S.cursor : undefined}
      onkeydown={keydown}
      onscroll={() => grid && (S.scroll.browser = grid.scrollTop)}
    >
      {#if S.listing}
        <div class="rb-hint pad">
          <Icon name="loader" class="rb-spin" />
          {S.opening
            ? ' Opening the repository for browsing. This takes a moment once; after that, folders open right away.'
            : ' Reading the folder...'}
        </div>
      {:else}
        <table class="rb-table">
          <thead>
            <tr>
              <th class="shrink">
                <input
                  type="checkbox"
                  aria-label="Select all"
                  tabindex="-1"
                  checked={allChecked}
                  {@attach (n: HTMLInputElement) => {
                    n.indeterminate = someChecked;
                  }}
                  onchange={(ev) => selectAll(ev.currentTarget.checked)}
                />
              </th>
              <th aria-sort={ariaSort('name')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('name')}>Name{sortMark('name')}</button></th>
              <th class="num rb-hide-narrow" aria-sort={ariaSort('size')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('size')}>Size{sortMark('size')}</button></th>
              <th class="rb-hide-narrow" aria-sort={ariaSort('mtime')}><button type="button" tabindex="-1" class="sort" onclick={() => sortBy('mtime')}>Modified{sortMark('mtime')}</button></th>
            </tr>
          </thead>
          <tbody>
            {#each visible as e, i (e.path)}
              {@const isDir = e.type === 'dir'}
              <tr
                id={'rb-snaprow-' + i}
                class="clickable"
                class:cursor={i === S.cursor}
                class:checked={!!S.checked[e.path]}
                aria-selected={!!S.checked[e.path]}
                onclick={() => rowClick(e, i)}
              >
                <td class="shrink">
                  <input
                    type="checkbox"
                    tabindex="-1"
                    aria-label={'Select ' + e.name}
                    checked={!!S.checked[e.path]}
                    onclick={(ev) => ev.stopPropagation()}
                    onchange={(ev) => toggle(e, ev.currentTarget.checked)}
                  />
                </td>
                <td class="name"><FileIcon type={e.type} name={e.name} /><span title={e.path}>{e.name}</span></td>
                <td class="num rb-hide-narrow">{isDir ? '' : bytes(e.size)}</td>
                <td class="dim rb-hide-narrow">{e.mtime ? dateTime(e.mtime) : ''}</td>
              </tr>
            {:else}
              <tr>
                <td colspan="4" class="dim">
                  {#if S.filter && S.entries?.length}
                    Nothing here matches "{S.filter}". <button type="button" class="rb-link" onclick={() => (S.filter = '')}>Show all</button>
                  {:else}
                    Empty folder.
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>

    <div class="rb-selbar">
      {#if !items.length}
        <span>Select files and folders, or:</span>
        {#if S.entries?.length && !S.listing}
          <span class="count rb-hide-narrow">{plural(S.entries.length, 'item')}{folderSize ? ' · ' + bytes(folderSize) + ' in files' : ''}</span>
        {/if}
        <div class="rb-spacer"></div>
        <Button
          icon="download"
          small
          disabled={S.path === '/'}
          title={S.path === '/' ? '' : 'Downloads the folder as a zip file'}
          onclick={() => S.snap && S.repo && download({ repo: S.repo, snapshot: S.snap.id, path: S.path })}>Download folder</Button
        >
        <Button
          icon="restore"
          small
          kind="primary"
          disabled={S.path === '/'}
          onclick={() => restoreItems([{ path: S.path, type: 'dir', name: S.path }])}>Restore this folder...</Button
        >
      {:else}
        <b>{plural(items.length, 'item') + ' selected'}</b>
        {#if hiddenChecked}
          <span class="count">{hiddenChecked} hidden by the filter</span>
        {/if}
        <button type="button" class="rb-link" onclick={() => (S.checked = {})}>Clear</button>
        <div class="rb-spacer"></div>
        <Button
          icon="download"
          small
          disabled={items.length !== 1}
          title={items.length !== 1 ? 'One item at a time' : items[0].type === 'dir' ? 'Downloads the folder as a zip file' : ''}
          onclick={() => S.snap && S.repo && download({ repo: S.repo, snapshot: S.snap.id, path: items[0].path })}
          >Download</Button
        >
        <Button icon="restore" small kind="primary" onclick={() => restoreItems(items.map((e) => ({ path: e.path, type: e.type, name: e.name })))}
          >Restore...</Button
        >
      {/if}
    </div>
  </div>
{/if}

<style>
  :global(#rb-app) .none {
    border: 0;
  }
  .grow {
    flex: 1;
  }
  .info-actions {
    display: inline-flex;
    gap: 4px;
    margin: -4px 0;
  }
  :global(#rb-app) .rb-snap-info {
    align-items: center;
  }
  :global(#rb-app) input.quick {
    width: 200px;
    height: 30px;
    flex: 0 1 200px;
  }
  :global(#rb-app) .pad {
    padding: 16px;
  }
  .pad :global(.rb-spin) {
    animation: rb-spin 0.9s linear infinite;
  }
  .grid:focus {
    outline: none;
  }
  .grid:focus-visible {
    box-shadow: inset 0 0 0 1px var(--rb-accent);
  }
  :global(#rb-app) .grid table tbody tr.checked td {
    background: var(--rb-select-soft);
  }
  :global(#rb-app) .grid:focus table tbody tr.cursor td {
    background: var(--rb-select);
  }
  :global(#rb-app) .grid:focus table tbody tr.cursor td:first-child {
    box-shadow: inset 3px 0 0 var(--rb-accent);
  }
  :global(#rb-app) button.sort {
    font: inherit;
    color: inherit;
    letter-spacing: inherit;
    text-transform: inherit;
    cursor: pointer;
  }
  :global(#rb-app) button.sort:hover {
    color: var(--rb-text);
  }
  :global(#rb-app) button.rb-link {
    font-size: inherit;
  }
  .count {
    color: var(--rb-dim);
  }
  @media (max-width: 560px) {
    :global(#rb-app) input.quick {
      flex: 1 1 100%;
      width: 100%;
    }
  }
</style>
