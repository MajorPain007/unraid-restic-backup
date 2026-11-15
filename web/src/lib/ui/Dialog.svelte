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
  const opener = document.activeElement as HTMLElement | null;

  $effect(() => () => {
    if (opener?.isConnected) opener.focus();
  });

  $effect(() => {
    const t = setTimeout(() => {
      const f = panel?.querySelector<HTMLElement>(
        'input:not([type=hidden]):not([type=checkbox]):not([type=radio]), select, textarea'
      );
      (f ?? panel?.querySelector<HTMLElement>('.rb-panel-head button'))?.focus();
    }, 30);
    return () => clearTimeout(t);
  });

  function trap(e: KeyboardEvent) {
    if (e.key !== 'Tab' || !panel) return;
    const items = [...panel.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, summary, [tabindex]:not([tabindex="-1"])')].filter(
      (el) => !el.hasAttribute('disabled') && el.getClientRects().length
    );
    if (!items.length) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (e.shiftKey && (document.activeElement === first || document.activeElement === panel)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function backdrop(e: MouseEvent) {
    if (e.target === e.currentTarget && !sticky) onclose();
  }
</script>

<div class="rb-overlay" class:center={!drawer} role="presentation" onmousedown={backdrop}>
  <div bind:this={panel} class={drawer ? 'rb-drawer' : 'rb-modal'} class:wide role="dialog" aria-modal="true" aria-label={title} data-sticky={sticky ? 'yes' : undefined} tabindex="-1" onkeydown={trap}>
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
