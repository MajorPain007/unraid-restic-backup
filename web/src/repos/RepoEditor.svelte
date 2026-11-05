<script lang="ts">
  import { untrack } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Toggle from '../lib/ui/Toggle.svelte';
  import Segmented from '../lib/ui/Segmented.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import ScheduleEditor from '../lib/ui/ScheduleEditor.svelte';
  import { api } from '../lib/api';
  import { pickPath } from '../lib/dialogs';
  import type { Repo } from '../lib/types';
  import type { EditorResult } from './flows';
  import { deleteRepo } from './flows';
  import { KIND_ORDER, SERVER_KINDS, blankRepo, isStorageBoxHost, repoIcon, serverKind, serverName, type ServerKind } from './util';

  let { saved = null, close }: { saved?: Repo | null; close: (r?: EditorResult) => void } = $props();

  const original = untrack(() => $state.snapshot(saved)) as Repo | null;
  const isNew = !original;
  let repo = $state<Repo>(original ? structuredClone(original) : blankRepo());
  let kind = $state<ServerKind>(serverKind(repo));
  let pwMode = $state<'generate' | 'set' | 'keep'>(isNew ? 'generate' : 'keep');
  let pwValue = $state('');
  let pwChange = $state(false);
  let restMode = $state<'keep' | 'set'>('keep');
  let restValue = $state('');
  let flags = $state(repo.options.extra_flags.join('\n'));
  let error = $state<unknown>(null);
  let saving = $state(false);
  let top: HTMLElement | undefined = $state();

  const TYPES: { value: Repo['type']; title: string; icon: string; text: string }[] = [
    { value: 'sftp', title: 'SFTP', icon: 'server', text: 'Another server over SSH - a second Unraid, a NAS, a Storage Box, a VPS.' },
    { value: 'local', title: 'Local folder', icon: 'hdd', text: 'A disk in this server: an unassigned device or another pool.' },
    { value: 'rest', title: 'REST server', icon: 'cloud', text: "restic's own server, optionally append-only against ransomware." }
  ];

  function setKind(k: ServerKind) {
    kind = k;
    repo.sftp.port = SERVER_KINDS[k].port;
  }

  function hostTyped() {
    if (isStorageBoxHost(repo.sftp.host) && kind !== 'storagebox') setKind('storagebox');
  }

  async function browse() {
    const path = await pickPath({ title: 'Repository folder', create: true, start: repo.local.path || '/mnt' });
    if (path) repo.local.path = path;
  }

  function num(v: unknown): number {
    const n = Number(v);
    return v === null || v === '' || isNaN(n) ? 0 : n;
  }

  async function save() {
    const data = $state.snapshot(repo) as Repo;
    data.sftp.port = num(data.sftp.port);
    data.options.limit_upload = num(data.options.limit_upload);
    data.options.limit_download = num(data.options.limit_download);
    data.options.connections = num(data.options.connections);
    data.options.extra_flags = flags
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);
    const params: Record<string, unknown> = { repo: data, password_mode: pwMode };
    if (pwMode === 'set') params.password = pwValue;
    if (data.type === 'rest' && restMode === 'set') {
      params.rest_password_mode = 'set';
      params.rest_password = restValue;
    }
    saving = true;
    try {
      const res = await api<{ repo: Repo; generated_password?: string }>('repo_save', params);
      close({ repo: res.repo, generated: res.generated_password || '', isNew });
    } catch (e) {
      error = e;
      const body = top?.closest('.rb-panel-body');
      if (body) body.scrollTop = 0;
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (original && (await deleteRepo(original))) close();
  }
</script>

<Dialog drawer title={isNew ? 'Add repository' : 'Edit "' + original?.name + '"'} icon="repo" onclose={() => close()}>
  <div bind:this={top}><ErrorBox {error} /></div>

  {#if isNew}
    <div class="rb-form-section">
      <h3><Icon name="repo" />Type</h3>
      <OptionCards bind:value={repo.type} options={TYPES} />
    </div>
  {/if}

  <div class="rb-form-section">
    <h3><Icon name={repoIcon(repo.type)} />Location</h3>
    <div class="rb-fields" style="margin-bottom:14px">
      <Field label="Name">
        <input type="text" autocomplete="off" spellcheck="false" placeholder="e.g. Offsite" bind:value={repo.name} />
      </Field>
    </div>
    {#if repo.type === 'sftp'}
      <div class="rb-fields">
        <Field label="The server is" hint="Sets the usual port. Once saved, the plugin shows its SSH key and how to add it on the server.">
          <select value={kind} onchange={(e) => setKind(e.currentTarget.value as ServerKind)}>
            {#each KIND_ORDER as k (k)}<option value={k}>{SERVER_KINDS[k].label}</option>{/each}
          </select>
        </Field>
        <Field label="Host">
          <input
            type="text"
            autocomplete="off"
            spellcheck="false"
            placeholder="backup.example.com or 192.168.1.20"
            bind:value={repo.sftp.host}
            oninput={hostTyped}
          />
        </Field>
        <Field label="Port">
          <input type="number" min="1" max="65535" autocomplete="off" bind:value={repo.sftp.port} />
        </Field>
        <Field label="User">
          <input type="text" autocomplete="off" spellcheck="false" placeholder="root, or uXXXXXX for a Storage Box" bind:value={repo.sftp.user} />
        </Field>
        <Field
          label="Path on the server"
          wide
          hint={'Relative paths start in the user\'s home folder. Created by "Initialize" if it does not exist.'}
        >
          <input type="text" class="rb-mono" autocomplete="off" spellcheck="false" bind:value={repo.sftp.path} />
        </Field>
      </div>
    {:else if repo.type === 'local'}
      <div class="rb-fields">
        <Field label="Folder" wide hint="Best on a disk that is not part of what you back up: an unassigned device, or another pool.">
          <div class="rb-input-group">
            <input
              type="text"
              class="rb-mono"
              autocomplete="off"
              spellcheck="false"
              placeholder="/mnt/disks/backup/restic"
              bind:value={repo.local.path}
            />
            <Button icon="folder" onclick={browse}>Browse...</Button>
          </div>
        </Field>
      </div>
    {:else}
      <div class="rb-fields">
        <Field label="Server URL" wide>
          <input
            type="text"
            class="rb-mono"
            autocomplete="off"
            spellcheck="false"
            placeholder={'https://backup.lan:8000/' + serverName()}
            bind:value={repo.rest.url}
          />
        </Field>
        <Field label="User" optional>
          <input type="text" autocomplete="off" spellcheck="false" bind:value={repo.rest.user} />
        </Field>
        <Field label="Password" optional>
          <input
            type="password"
            autocomplete="new-password"
            placeholder={original?.has_rest_password ? 'unchanged' : ''}
            bind:value={restValue}
            oninput={() => (restMode = 'set')}
          />
        </Field>
        <Field label="CA certificate" optional wide hint="For a server with a certificate of its own making.">
          <input
            type="text"
            class="rb-mono"
            autocomplete="off"
            spellcheck="false"
            placeholder="/boot/config/plugins/restic.backup/ca.pem"
            bind:value={repo.rest.cacert}
          />
        </Field>
      </div>
    {/if}
  </div>

  <div class="rb-form-section">
    <h3><Icon name="key" />Password</h3>
    <div class="rb-hint" style="margin-bottom:14px">restic encrypts everything with it.</div>
    {#if isNew}
      <Segmented
        bind:value={pwMode}
        options={[
          ['generate', 'Make a new password'],
          ['set', 'Enter an existing one']
        ]}
      />
    {:else}
      <Toggle bind:checked={pwChange} label="Change the stored password" onchange={(on) => (pwMode = on ? 'set' : 'keep')} />
      <div class="rb-hint" style="margin-top:6px">
        This changes only what the plugin uses to open the repository. To change the repository's own password, use restic key passwd.
      </div>
    {/if}
    {#if pwMode === 'set'}
      <div style="margin-top:12px">
        <Field label={isNew ? 'Password of the existing repository' : 'New stored password'}>
          <input type="password" autocomplete="new-password" placeholder="The repository password" bind:value={pwValue} />
        </Field>
      </div>
    {:else if pwMode === 'generate'}
      <div class="rb-hint" style="margin-top:12px">A random 32-character password is made and shown once you save, for you to keep.</div>
    {/if}
  </div>

  <div class="rb-form-section">
    <h3><Icon name="shield" />Maintenance</h3>
    <Toggle bind:checked={repo.maintenance.prune.enabled} label="Remove unused data (prune)" />
    <div class="rb-hint" style="margin:4px 0 0 44px">Retention only drops snapshots; prune frees the space they used.</div>
    {#if repo.maintenance.prune.enabled}
      <div style="margin-top:12px">
        <ScheduleEditor bind:schedule={repo.maintenance.prune.schedule} />
        <div class="rb-fields" style="margin-top:12px">
          <Field label="Leave unused at most" hint="Repacking costs time and traffic; a little unused space saves both.">
            <input type="text" autocomplete="off" spellcheck="false" placeholder="10%" bind:value={repo.maintenance.prune.max_unused} />
          </Field>
        </div>
      </div>
    {/if}
    <div style="height:18px"></div>
    <Toggle bind:checked={repo.maintenance.check.enabled} label="Check the repository" />
    {#if repo.maintenance.check.enabled}
      <div style="margin-top:12px">
        <ScheduleEditor bind:schedule={repo.maintenance.check.schedule} />
        <div class="rb-fields" style="margin-top:12px">
          <Field label="Read" hint="Reading data downloads it - mind the traffic for remote repositories.">
            <select bind:value={repo.maintenance.check.read_data}>
              <option value="none">Structure only</option>
              <option value="subset">Structure and part of the data</option>
              <option value="all">Structure and all data</option>
            </select>
          </Field>
          <Field label="Share of data" hint="A different part each time.">
            <input type="text" autocomplete="off" spellcheck="false" placeholder="5%" bind:value={repo.maintenance.check.subset} />
          </Field>
        </div>
      </div>
    {/if}
  </div>

  <div class="rb-form-section">
    <details class="rb-more">
      <summary>Advanced</summary>
      <div class="rb-fields">
        <Field label="Compression">
          <select bind:value={repo.options.compression}>
            <option value="auto">Auto (recommended)</option>
            <option value="fastest">Fastest</option>
            <option value="better">Better</option>
            <option value="max">Maximum</option>
            <option value="off">Off</option>
          </select>
        </Field>
        <Field label="Upload limit (KiB/s)" hint="0 = unlimited. 1 MB/s = 1024.">
          <input type="number" min="0" autocomplete="off" bind:value={repo.options.limit_upload} />
        </Field>
        <Field label="Download limit (KiB/s)" hint="0 = unlimited.">
          <input type="number" min="0" autocomplete="off" bind:value={repo.options.limit_download} />
        </Field>
        <Field label="Connections" hint="0 = restic's default.">
          <input type="number" min="0" autocomplete="off" bind:value={repo.options.connections} />
        </Field>
        <Field label="Extra restic options" wide optional>
          <textarea rows="2" spellcheck="false" placeholder="--pack-size=64" bind:value={flags}></textarea>
        </Field>
        <div class="rb-field wide">
          <Toggle bind:checked={repo.unlock_stale} label="Remove stale locks automatically" />
          <div class="rb-hint">When a run finds the repository locked by a process that no longer exists - after a crash or power cut.</div>
        </div>
        <div class="rb-field wide">
          <Toggle bind:checked={repo.append_only} label="The server keeps this repository append-only" />
          <div class="rb-hint">Snapshots cannot be removed from here; retention and prune have to run on the server.</div>
        </div>
      </div>
    </details>
  </div>

  {#snippet actions()}
    {#if !isNew}
      <Button icon="trash" kind="danger" onclick={remove}>Delete</Button>
    {/if}
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Cancel</Button>
    <Button kind="primary" icon="check" busy={saving} onclick={save}>{isNew ? 'Add repository' : 'Save'}</Button>
  {/snippet}
</Dialog>
