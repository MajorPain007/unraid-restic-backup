<script lang="ts">
  import Button from '../lib/ui/Button.svelte';
  import { copyText } from '../lib/state.svelte';

  let { text, spaced = false }: { text: string; spaced?: boolean } = $props();
</script>

<div class="rb-secret" style:margin-top={spaced ? '6px' : null}>
  <code>{text}</code>
  <Button icon="copy" small title="Copy" onclick={() => copyText(text)} />
</div>
