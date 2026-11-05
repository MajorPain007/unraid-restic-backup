<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import type { Repo } from '../lib/types';
  import { repoUrl } from './util';

  let { repo, close }: { repo: Repo; close: (r?: boolean) => void } = $props();
</script>

<Dialog title={'Remove "' + repo.name + '" from the plugin?'} icon="alert" onclose={() => close(false)}>
  <p>The plugin forgets this repository, its password and its SSH key.</p>
  <p>
    <b>The backups themselves are not deleted</b> - they stay at {repoUrl(repo)}. Without the password, nobody can read them; keep it if
    you may need them.
  </p>
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close(false)}>Cancel</Button>
    <Button kind="danger solid" onclick={() => close(true)}>Remove</Button>
  {/snippet}
</Dialog>
