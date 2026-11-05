<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    icon = '',
    kind = '',
    small = false,
    title = '',
    disabled = false,
    busy = false,
    type = 'button',
    onclick,
    children
  }: {
    icon?: string;
    kind?: string;
    small?: boolean;
    title?: string;
    disabled?: boolean;
    busy?: boolean;
    type?: 'button' | 'submit';
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button
  {type}
  class="rb-btn {kind}"
  class:sm={small}
  class:icon={!children}
  title={title || null}
  aria-label={!children ? title || null : null}
  disabled={disabled || busy}
  {onclick}
>
  {#if busy}
    <Icon name="loader" class="rb-spin" />
  {:else if icon}
    <Icon name={icon} />
  {/if}
  {@render children?.()}
</button>
