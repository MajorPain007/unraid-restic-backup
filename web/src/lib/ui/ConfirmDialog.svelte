<script lang="ts">
  import Dialog from './Dialog.svelte';
  import Button from './Button.svelte';

  let {
    title,
    text,
    confirm = 'OK',
    danger = false,
    close
  }: { title: string; text: string; confirm?: string; danger?: boolean; close: (r: boolean) => void } = $props();

  let yes: HTMLElement;
  $effect(() => {
    const t = setTimeout(() => yes?.querySelector('button')?.focus(), 40);
    return () => clearTimeout(t);
  });
</script>

<Dialog {title} icon={danger ? 'alert' : ''} onclose={() => close(false)}>
  <p>{text}</p>
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close(false)}>Cancel</Button>
    <span bind:this={yes} style="display:contents">
      <Button kind={danger ? 'danger solid' : 'primary'} onclick={() => close(true)}>{confirm}</Button>
    </span>
  {/snippet}
</Dialog>
