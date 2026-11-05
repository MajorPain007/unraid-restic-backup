<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import { api } from '../lib/api';
  import { confirm, pickPath } from '../lib/dialogs';
  import { fail } from '../lib/state.svelte';
  import { dateTime } from '../lib/format';
  import type { Snapshot } from '../lib/types';
  import type { Item } from './store.svelte';

  type Target = 'original' | 'path';

  let {
    repo,
    snap,
    items,
    close
  }: { repo: string; snap: Snapshot; items: Item[]; close: (started?: boolean) => void } = $props();

  const OVERWRITE: [string, string][] = [
    ['if-changed', 'Replace files that differ from the snapshot'],
    ['if-newer', "Replace files only where the snapshot's copy is newer"],
    ['never', 'Keep files that are there; only add missing ones'],
    ['always', 'Replace every file']
  ];

  let target = $state<Target>('original');
  let targetPath = $state('/mnt/user/restore');
  let overwrite = $state('if-changed');
  let del = $state(false);
  let working = $state(false);

  function setTarget(v: Target) {
    target = v;
    overwrite = v === 'original' ? 'if-changed' : 'always';
  }

  async function browse() {
    const path = await pickPath({ title: 'Restore into', create: true, start: '/mnt/user' });
    if (path) targetPath = path;
  }

  async function go() {
    working = true;
    try {
      await api('restore_start', {
        repo,
        snapshot: snap.id,
        items: items.map((i) => i.path),
        target,
        target_path: targetPath,
        overwrite,
        delete: target === 'original' && del ? 1 : 0
      });
      close(true);
    } catch (e) {
      fail(e);
    } finally {
      working = false;
    }
  }

  async function restore() {
    if (target === 'original' && del) {
      const yes = await confirm({
        title: 'Delete files not in the snapshot?',
        text: 'Anything in these folders that is newer than the snapshot is deleted.',
        confirm: 'Restore and delete',
        danger: true
      });
      if (!yes) return;
    }
    go();
  }
</script>

<Dialog title={'Restore from ' + dateTime(snap.time)} icon="restore" wide onclose={() => close()}>
  <div class="rb-pathlist">
    {#each items.slice(0, 6) as it (it.path)}
      <div class="rb-pathlist-item">
        <Icon name={it.type === 'dir' ? 'folder' : 'file'} />
        <span class="rb-mono" title={it.path}>{it.path}</span>
      </div>
    {/each}
    {#if items.length > 6}
      <div class="rb-hint">and {items.length - 6} more</div>
    {/if}
  </div>
  <div class="gap-lg">
    <OptionCards
      value={target}
      onchange={setTarget}
      options={[
        { value: 'original', title: 'Where they came from', icon: 'restore', text: 'Puts the files back in place.' },
        { value: 'path', title: 'Into another folder', icon: 'folder', text: 'Leaves what is there untouched, to compare first.' }
      ]}
    />
  </div>
  <div class="gap">
    {#if target === 'path'}
      <Field label="Restore into" hint="Each item lands directly in this folder, which is created if needed.">
        <div class="rb-input-group">
          <input type="text" class="rb-mono" autocomplete="off" spellcheck="false" bind:value={targetPath} />
          <Button icon="folder" onclick={browse}>Browse...</Button>
        </div>
      </Field>
    {/if}
    <div class="gap-sm">
      <Field label="Files that already exist">
        <select bind:value={overwrite}>
          {#each OVERWRITE as [v, label] (v)}
            <option value={v}>{label}</option>
          {/each}
        </select>
      </Field>
    </div>
    {#if target === 'original'}
      <div class="gap">
        <Toggle bind:checked={del} label="Also delete files that are not in the snapshot" />
        <div class="rb-hint del-hint">Makes the folders exactly as they were. Files created since are lost.</div>
      </div>
    {/if}
  </div>
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="restore" busy={working} onclick={restore}>Restore</Button>
  {/snippet}
</Dialog>

<style>
  .gap-lg {
    margin-top: 18px;
  }
  .gap {
    margin-top: 14px;
  }
  .gap-sm {
    margin-top: 12px;
  }
  .del-hint {
    margin: 4px 0 0 44px;
  }
</style>
