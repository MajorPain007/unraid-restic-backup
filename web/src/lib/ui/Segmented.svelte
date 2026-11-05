<script lang="ts" generics="T extends string">
  let {
    value = $bindable(),
    options,
    onchange
  }: { value: T; options: [T, string][]; onchange?: (v: T) => void } = $props();
</script>

<div class="rb-seg" role="radiogroup">
  {#each options as [v, label] (v)}
    <button
      type="button"
      role="radio"
      class:active={v === value}
      aria-checked={v === value}
      onclick={() => {
        value = v;
        onchange?.(v);
      }}>{label}</button
    >
  {/each}
</div>
