<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import type { Repo } from '../lib/types';

  let { repo, message, close }: { repo: Repo; message: string; close: (r?: 'key' | 'edit') => void } = $props();
</script>

<Dialog title={'Could not open "' + repo.name + '"'} icon="alert" onclose={() => close()}>
  <p>{message}</p>
  {#if repo.type === 'sftp'}
    <p class="rb-hint">Check host, port, user and path, and that the server accepts the plugin's SSH key.</p>
  {/if}
  {#snippet actions()}
    {#if repo.type === 'sftp'}
      <Button icon="key" onclick={() => close('key')}>Show SSH key</Button>
    {/if}
    <div class="rb-spacer"></div>
    <Button icon="edit" onclick={() => close('edit')}>Edit repository</Button>
  {/snippet}
</Dialog>
