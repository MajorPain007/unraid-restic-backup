<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Secret from './Secret.svelte';
  import { api } from '../lib/api';
  import { confirm } from '../lib/dialogs';
  import { fail, refresh, toast } from '../lib/state.svelte';
  import type { Repo } from '../lib/types';
  import { KIND_ORDER, SERVER_KINDS, keyPath, serverKind, serverName, type ServerKind } from './util';

  let { repo, setup = false, close }: { repo: Repo; setup?: boolean; close: (go?: boolean) => void } = $props();

  let key = $state(untrack(() => repo.ssh_public_key || ''));
  let kind = $state<ServerKind>(untrack(() => serverKind(repo)));
  let renewing = $state(false);

  let port = $derived(repo.sftp.port || 22);
  let copyId = $derived(
    'ssh-copy-id -p ' + port + ' -i ' + keyPath(repo) + ' ' + (repo.sftp.user || 'user') + '@' + (repo.sftp.host || 'server')
  );
  let install = $derived(
    'cat ' +
      keyPath(repo) +
      ' | ssh -p 23 ' +
      (repo.sftp.user || 'uXXXXXX') +
      '@' +
      (repo.sftp.host || 'uXXXXXX.your-storagebox.de') +
      ' install-ssh-key'
  );

  async function newKey() {
    const yes = await confirm({
      title: 'Make a new key?',
      text: 'The old key stops working; the new one has to be added to the server again.',
      confirm: 'New key',
      danger: true
    });
    if (!yes) return;
    renewing = true;
    try {
      const r = await api<{ public_key: string }>('repo_keygen', { id: repo.id });
      key = r.public_key;
      toast('New key made - add it to the server.');
      await refresh();
    } catch (e) {
      fail(e);
    } finally {
      renewing = false;
    }
  }
</script>

<Dialog title={'Let the plugin log in to ' + (repo.sftp.host || 'the server')} icon="key" wide onclose={() => close(false)}>
  <p>
    The plugin made its own SSH key for this repository. The server has to accept it - add the public key below to the user
    {repo.sftp.user} on {repo.sftp.host}. No password is stored anywhere.
  </p>
  <Field label="Public key">
    <Secret text={key} />
  </Field>
  <div class="rb-inline" style="margin:16px 0 4px">
    <b>The server is</b>
    <select style="width:auto" bind:value={kind}>
      {#each KIND_ORDER as k (k)}<option value={k}>{SERVER_KINDS[k].label}</option>{/each}
    </select>
  </div>
  <ol class="rb-ssh-steps">
    {#if kind === 'ssh'}
      <li>
        Add the key to the user on the server, for example from the Unraid terminal:
        <Secret text={copyId} spaced />
      </li>
      <li>
        Another Unraid server takes it under Users, root, "SSH authorized keys" too. A Synology or QNAP needs SFTP switched on first
        (Control Panel &gt; File Services), and its paths start at the shared folder: /backups/{serverName()}, without /volume1.
      </li>
      <li>A relative path starts in the user's home folder.</li>
    {:else}
      <li>Enable SSH support for the Storage Box (or sub-account) in the Hetzner console.</li>
      <li>
        In the Unraid terminal, run this once and enter the Storage Box password when asked:
        <Secret text={install} spaced />
      </li>
      <li>Use port 23, and a relative path such as backups/{serverName()} - it is relative to the home folder.</li>
    {/if}
  </ol>
  {#snippet actions()}
    <Button icon="refresh" kind="ghost" small busy={renewing} onclick={newKey}>New key</Button>
    <div class="rb-spacer"></div>
    <Button onclick={() => close(false)}>{setup ? 'Later' : 'Close'}</Button>
    <Button kind="primary" icon="refresh" onclick={() => close(true)}>Test connection</Button>
  {/snippet}
</Dialog>

<style>
  .rb-ssh-steps {
    margin: 6px 0 0;
    padding-left: 20px;
    display: grid;
    gap: 8px;
  }
</style>
