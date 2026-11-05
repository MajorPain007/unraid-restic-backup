<script lang="ts">
  import Self from './RuleTreeLevel.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { bytes, plural } from '../lib/format';
  import { exact, outText, type TreeAction, type TreeEntry } from './rules';
  import type { SvelteSet } from 'svelte/reactivity';

  let {
    path,
    depth,
    gen,
    request,
    opened,
    action
  }: {
    path: string;
    depth: number;
    gen: number;
    request: () => unknown;
    opened: SvelteSet<string>;
    action: (e: TreeEntry) => TreeAction | null;
  } = $props();

  let entries = $state<TreeEntry[] | null>(null);
  let more = $state(0);
  let moreOut = $state(0);
  let error = $state('');

  $effect(() => {
    const g = gen;
    let alive = true;
    api<{ entries: TreeEntry[]; more: number; more_out: number }>('job_tree', { job: request(), path })
      .then((r) => {
        if (!alive || g !== gen) return;
        entries = r.entries;
        more = r.more;
        moreOut = r.more_out;
        error = '';
      })
      .catch((e: Error) => {
        if (!alive || g !== gen) return;
        error = e.message;
      });
    return () => {
      alive = false;
    };
  });

  function why(e: TreeEntry): string {
    if (e.out) return e.out.via ? `Left out with ${e.out.via} (${outText(e.out)})` : 'Left out: ' + outText(e.out);
    return e.mount ? 'Backed up as an empty folder: another file system is mounted here' : 'Backed up';
  }

  function flip(e: TreeEntry) {
    if (opened.has(e.path)) opened.delete(e.path);
    else opened.add(e.path);
  }

  let pad = $derived(depth * 18 + 30);
</script>

{#snippet cells(e: TreeEntry, act: TreeAction | null)}
  <span class="rb-tree-name" style:padding-left="{depth * 18}px">
    {#if e.open}<Icon name="right" class="rb-tree-caret" />{:else}<span class="rb-tree-caret"></span>{/if}
    {#if e.type === 'missing'}
      <span class="rb-rtree-mark out" title="Does not exist"><Icon name="alert" /></span>
    {:else if act}
      <button
        type="button"
        class="rb-rtree-mark act {e.out ? 'out' : 'in'}"
        class:via={!!e.out?.via}
        title="{why(e)}. Click: {act.title.toLowerCase()}."
        aria-label="{act.title} {e.name}"
        onclick={(ev) => {
          ev.stopPropagation();
          act.run();
        }}><Icon name={e.out ? 'x' : 'check'} /></button
      >
    {:else}
      <span class="rb-rtree-mark {e.out ? 'out' : 'in'}" class:via={!!e.out?.via} title={why(e)}><Icon name={e.out ? 'x' : 'check'} /></span>
    {/if}
    <Icon name={e.type === 'dir' ? 'folder' : e.type === 'symlink' ? 'link' : 'file'} class={e.type === 'dir' ? 'dir' : ''} />
    <span class="rb-rtree-name" title={e.path}>{e.name}</span>
  </span>
  <span class="rb-rtree-rule">
    {#if e.type === 'missing'}
      <span class="rb-rtree-error">does not exist</span>
    {:else if e.out && !e.out.via}
      {#if e.out.kind === 'pattern' && e.out.label === exact(e.path)}
        <span title={e.out.label}>by its path</span>
      {:else if /pattern$/.test(e.out.kind)}
        <code title={outText(e.out)}>{outText(e.out)}</code>
      {:else}
        <span title={outText(e.out)}>{outText(e.out)}</span>
      {/if}
    {:else if e.mount}
      <span>mount point, kept empty</span>
    {/if}
  </span>
  <span class="num">{e.size !== null ? bytes(e.size) : ''}</span>
{/snippet}

{#if error}
  <div class="rb-hint rb-rtree-error" style:padding="4px 8px 4px {pad}px">{error}</div>
{:else if entries === null}
  <div class="rb-hint rb-rtree-wait" style:padding-left="{pad}px"><Icon name="loader" class="rb-spin" /> Reading...</div>
{:else}
  {#if !entries.length}
    <div class="rb-hint" style:padding="4px 8px 4px {pad}px">{path === '' ? 'No folders yet.' : 'Empty.'}</div>
  {/if}
  {#each entries as e (e.path)}
    {@const isOpen = e.open && opened.has(e.path)}
    {@const act = action(e)}
    <div class="rb-tree-node" class:open={isOpen}>
      {#if e.open}
        <div
          class="rb-rtree-row opens"
          class:out={!!e.out}
          class:via={!!e.out?.via}
          role="button"
          tabindex="0"
          aria-expanded={isOpen}
          onclick={() => flip(e)}
          onkeydown={(ev) => {
            if ((ev.key === 'Enter' || ev.key === ' ') && ev.target === ev.currentTarget) {
              ev.preventDefault();
              flip(e);
            }
          }}
        >
          {@render cells(e, act)}
        </div>
      {:else}
        <div class="rb-rtree-row" class:out={!!e.out} class:via={!!e.out?.via}>
          {@render cells(e, act)}
        </div>
      {/if}
      {#if isOpen}
        <div class="rb-tree-children">
          <Self path={e.path} depth={depth + 1} {gen} {request} {opened} {action} />
        </div>
      {/if}
    </div>
  {/each}
  {#if more}
    <div class="rb-hint" style:padding="4px 8px 4px {pad}px">
      and {plural(more, 'more entry', 'more entries')}{moreOut ? `, ${moreOut} of them left out` : ''}
    </div>
  {/if}
{/if}
