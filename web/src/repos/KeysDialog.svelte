<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import type { Repo } from '../lib/types';
  import type { ResticKey } from './flows';

  let { repo, keys, close }: { repo: Repo; keys: ResticKey[]; close: () => void } = $props();
</script>

<Dialog title={'Keys of "' + repo.name + '"'} icon="key" wide onclose={() => close()}>
  <p class="rb-hint">Each key is a password that opens the repository. The plugin uses the one marked current.</p>
  <div class="rb-table-wrap">
    <table class="rb-table">
      <thead>
        <tr><th>ID</th><th>Made by</th><th>Created</th><th></th></tr>
      </thead>
      <tbody>
        {#each keys as k, i (i)}
          <tr>
            <td class="rb-mono">{String(k.id).slice(0, 8)}</td>
            <td>{(k.userName || '?') + '@' + (k.hostName || '?')}</td>
            <td class="dim">{k.created}</td>
            <td>{#if k.current}<span class="rb-pill accent">current</span>{/if}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</Dialog>
