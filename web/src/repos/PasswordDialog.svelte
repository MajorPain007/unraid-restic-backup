<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Secret from './Secret.svelte';
  import type { Repo } from '../lib/types';
  import { recoveryFileName, recoveryText, saveText } from './util';

  let {
    repo,
    password,
    firstTime = false,
    close
  }: { repo: Repo; password: string; firstTime?: boolean; close: (r?: boolean) => void } = $props();

  let stored = $state(false);
  let downloaded = $state(false);

  $effect(() => {
    if (!firstTime) return;
    const guard = (e: KeyboardEvent) => {
      if (e.key === 'Escape') e.stopPropagation();
    };
    window.addEventListener('keydown', guard, true);
    return () => window.removeEventListener('keydown', guard, true);
  });

  function download() {
    saveText(recoveryFileName(repo), recoveryText(repo, password));
    downloaded = true;
  }
</script>

<Dialog
  title={firstTime ? 'Store this password' : 'Password of "' + repo.name + '"'}
  icon="key"
  sticky={firstTime}
  onclose={() => {
    if (!firstTime) close();
  }}
>
  {#if firstTime}
    <div class="rb-callout warn" style="margin-bottom:14px">
      <Icon name="alert" />
      <div>
        <b>Without this password the backups cannot be restored. </b>
        The plugin keeps a copy on the flash drive, but if the server is lost, that copy is lost with it. Store it in your password
        manager, or download the recovery file and keep it elsewhere.
      </div>
    </div>
  {/if}
  <Secret text={password} />
  <div class="rb-inline" style="margin-top:12px">
    <Button icon={downloaded ? 'check' : 'download'} small onclick={download}>Download recovery file</Button>
  </div>
  {#if firstTime}
    <div style="margin-top:16px">
      <Toggle bind:checked={stored} label="I have stored the password somewhere safe" />
    </div>
  {/if}
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button kind="primary" disabled={firstTime && !stored} onclick={() => close(true)}>{firstTime ? 'Continue' : 'Close'}</Button>
  {/snippet}
</Dialog>
