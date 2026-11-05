<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    label = '',
    hint = '',
    optional = false,
    wide = false,
    children,
    hintContent
  }: {
    label?: string;
    hint?: string;
    optional?: boolean;
    wide?: boolean;
    children: Snippet;
    hintContent?: Snippet;
  } = $props();

  const id = 'rbf' + Math.random().toString(36).slice(2, 9);
  let box: HTMLElement;

  $effect(() => {
    const ctl = box?.querySelector<HTMLElement>('input:not([type=hidden]), select, textarea');
    if (ctl && !ctl.id) ctl.id = id;
  });
</script>

<div class="rb-field" class:wide bind:this={box}>
  {#if label}
    <label for={id}>{label}{#if optional}{' '}<span class="opt">(optional)</span>{/if}</label>
  {/if}
  {@render children()}
  {#if hintContent}
    <div class="rb-hint">{@render hintContent()}</div>
  {:else if hint}
    <div class="rb-hint">{hint}</div>
  {/if}
</div>
