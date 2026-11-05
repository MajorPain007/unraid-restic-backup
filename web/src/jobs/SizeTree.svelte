<script lang="ts" module>
  export interface SizeNode {
    name: string;
    bytes: number;
    files: number;
    children?: SizeNode[];
    more?: { count: number; bytes: number } | null;
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import Self from './SizeTree.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { bytes, count } from '../lib/format';

  let { node, total, depth = 0, title = '' }: { node: SizeNode; total: number; depth?: number; title?: string } = $props();

  let kids = $derived(node.children ?? []);
  let opens = $derived(kids.length > 0 || !!node.more);
  let share = $derived(total > 0 ? Math.max(0.5, (node.bytes / total) * 100) : 0);
  let open = $state(untrack(() => depth === 0));

  function files(n: number): string {
    return count(n) + (Number(n) === 1 ? ' file' : ' files');
  }
</script>

{#snippet cells()}
  <span class="rb-tree-name" style:padding-left="{depth * 18}px">
    {#if opens}<Icon name="right" class="rb-tree-caret" />{:else}<span class="rb-tree-caret"></span>{/if}
    <Icon name="folder" class="dir" /><span>{node.name}</span>
  </span>
  <span class="rb-tree-bar"><i style:width="{share}%"></i></span>
  <span class="num">{bytes(node.bytes)}</span>
  <span class="num files">{files(node.files)}</span>
{/snippet}

<div class="rb-tree-node" class:open>
  {#if opens}
    <button type="button" class="rb-tree-row" title={title || null} aria-expanded={open} onclick={() => (open = !open)}>{@render cells()}</button>
  {:else}
    <div class="rb-tree-row" title={title || null}>{@render cells()}</div>
  {/if}
  {#if opens && open}
    <div class="rb-tree-children">
      {#each kids as k, i (i)}
        <Self node={k} {total} depth={depth + 1} />
      {/each}
      {#if node.more}
        <div class="rb-tree-row more">
          <span class="rb-tree-name dim" style:padding-left="{(depth + 1) * 18 + 20}px">{count(node.more.count)} more {node.more.count === 1 ? 'folder' : 'folders'}</span>
          <span></span>
          <span class="num">{bytes(node.more.bytes)}</span>
          <span></span>
        </div>
      {/if}
    </div>
  {/if}
</div>
