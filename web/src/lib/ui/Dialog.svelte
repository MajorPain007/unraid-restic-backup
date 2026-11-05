<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Button from './Button.svelte';

  let {
    title,
    icon = '',
    wide = false,
    drawer = false,
    sticky = false,
    onclose,
    children,
    actions
  }: {
    title: string;
    icon?: string;
    wide?: boolean;
    drawer?: boolean;
    sticky?: boolean;
    onclose: () => void;
    children: Snippet;
    actions?: Snippet;
  } = $props();

  let panel: HTMLElement;

  $effect(() => {
    const t = setTimeout(() => {
      const f = panel?.querySelector<HTMLElement>(
        'input:not([type=hidden]):not([type=checkbox]):not([type=radio]), select, textarea'
      );
      (f ?? panel?.querySelector<HTMLElement>('.rb-panel-head button'))?.focus();
    }, 30);
    return () => clearTimeout(t);
  });

  function backdrop(e: MouseEvent) {
    if (e.target === e.currentTarget && !sticky) onclose();
  }
</script>

<div class="rb-overlay" class:center={!drawer} role="presentation" onmousedown={backdrop}>
  <div bind:this={panel} class={drawer ? 'rb-drawer' : 'rb-modal'} class:wide role="dialog" aria-modal="true" aria-label={title}>
    <div class="rb-panel-head">
      {#if icon}<Icon name={icon} />{/if}
      <h2>{title}</h2>
      <Button icon="x" kind="ghost" title="Close" onclick={onclose} />
    </div>
    <div class="rb-panel-body">
      {@render children()}
    </div>
    {#if actions}
      <div class="rb-panel-foot">{@render actions()}</div>
    {/if}
  </div>
</div>
