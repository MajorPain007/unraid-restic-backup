<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import { api } from '../lib/api';
  import { fail } from '../lib/state.svelte';
  import type { Repo } from '../lib/types';
  import type { HostKeyInfo } from './flows';

  let { repo, hostkey, close }: { repo: Repo; hostkey: HostKeyInfo; close: (trusted?: boolean) => void } = $props();

  let changed = $derived(hostkey.status === 'changed');
  let working = $state(false);

  async function trust() {
    working = true;
    try {
      await api('repo_hostkey_trust', { id: repo.id, fingerprints: hostkey.scanned.map((k) => k.fingerprint) });
      close(true);
    } catch (e) {
      fail(e);
    } finally {
      working = false;
    }
  }
</script>

<Dialog
  title={changed ? "The server's key has changed" : 'Is this the right server?'}
  icon={changed ? 'alert' : 'shield'}
  onclose={() => close(false)}
>
  {#if changed}
    <div class="rb-callout warn" style="margin-bottom:14px">
      <Icon name="alert" />
      <div>
        The server now shows a different key than the one confirmed before. That is expected after it was reinstalled. Otherwise
        someone may be between this server and that one - do not accept it then.
      </div>
    </div>
  {:else}
    <p>
      This is the first connection to {repo.sftp.host}. It identifies itself with the key below. If you can, compare it with the
      server's own (ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub there).
    </p>
  {/if}
  <div class="rb-table-wrap">
    <table class="rb-table">
      <tbody>
        {#each hostkey.scanned as k, i (i)}
          <tr><td class="shrink">{k.type}</td><td class="rb-mono">{k.fingerprint}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if changed && hostkey.stored.length}
    <div class="rb-hint" style="margin-top:10px">
      Confirmed before: {hostkey.stored.map((k) => k.type + ' ' + k.fingerprint).join(', ')}
    </div>
  {/if}
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close(false)}>Cancel</Button>
    <Button kind={changed ? 'danger solid' : 'primary'} busy={working} onclick={trust}>
      {changed ? 'Accept the new key' : 'Yes, trust it'}
    </Button>
  {/snippet}
</Dialog>
