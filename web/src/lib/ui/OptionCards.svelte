<script lang="ts" generics="T extends string">
  import Icon from './Icon.svelte';

  let {
    value = $bindable(),
    options,
    two = false,
    onchange
  }: {
    value: T;
    options: { value: T; title: string; text: string; icon?: string; disabled?: boolean }[];
    two?: boolean;
    onchange?: (v: T) => void;
  } = $props();
</script>

<div class="rb-options" class:two role="radiogroup">
  {#each options as o (o.value)}
    <button
      type="button"
      role="radio"
      class="rb-option"
      class:active={o.value === value}
      aria-checked={o.value === value}
      disabled={o.disabled}
      onclick={() => {
        value = o.value;
        onchange?.(o.value);
      }}
    >
      <b>{#if o.icon}<Icon name={o.icon} />{/if}{o.title}</b>
      <span>{o.text}</span>
    </button>
  {/each}
</div>
