<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    disabled = false,
    onchange
  }: { checked?: boolean; label: string; disabled?: boolean; onchange?: (v: boolean) => void } = $props();
</script>

<label class="rb-switch">
  <input type="checkbox" bind:checked {disabled} onchange={() => onchange?.(checked)} />
  <span></span>
  {label}
</label>
