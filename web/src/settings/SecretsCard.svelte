<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Field from '../lib/ui/Field.svelte';
  import ErrorBox from '../lib/ui/ErrorBox.svelte';
  import OptionCards from '../lib/ui/OptionCards.svelte';
  import { api } from '../lib/api';
  import { busy, fail, refresh, toast } from '../lib/state.svelte';
  import type { Settings } from '../lib/types';

  type Mode = Settings['secrets'];

  interface SecretsStatus {
    mode: Mode;
    locked: boolean;
    problem: string;
    key_file: string;
    key_possible: boolean;
    key_impossible: string;
  }

  const OPTIONS: { value: Mode; title: string; text: string; icon: string }[] = [
    {
      value: 'plain',
      title: 'Readable',
      icon: 'unlock',
      text: 'As restic needs them. Right for servers without disk encryption, where encrypting them would protect nothing.'
    },
    {
      value: 'key',
      title: 'Encrypted, unlocked with the array',
      icon: 'shield',
      text: 'The key lies on the encrypted pool. Unlocking the array unlocks the passwords too.'
    },
    {
      value: 'master',
      title: 'Encrypted with a master password',
      icon: 'lock',
      text: 'Entered after every start of the server. Works without disk encryption.'
    }
  ];

  const NOW: Record<Mode, [string, string]> = {
    plain: ['', 'Readable now'],
    key: ['ok', 'Encrypted now'],
    master: ['ok', 'Encrypted now']
  };

  let st = $state<SecretsStatus | null>(null);
  let loadError = $state<unknown>(null);
  let choice = $state<Mode>('plain');
  let pw = $state({ password: '', password_repeat: '', current_password: '' });
  let error = $state<unknown>(null);
  let applying = $state({ busy: false });
  let locking = $state({ busy: false });

  let mismatch = $derived(choice === 'master' && !!pw.password_repeat && pw.password !== pw.password_repeat);
  let needCurrent = $derived(!!st && st.mode === 'master' && (choice !== 'master' || !!pw.password));
  let badge = $derived(st ? (st.locked ? (['warn', 'Locked'] as [string, string]) : NOW[st.mode]) : null);

  async function load() {
    try {
      const r = await api<SecretsStatus>('secrets_status');
      st = r;
      choice = r.mode;
      pw = { password: '', password_repeat: '', current_password: '' };
      error = null;
      loadError = null;
    } catch (e) {
      loadError = e;
    }
  }

  onMount(load);

  async function apply() {
    try {
      await busy(applying, api('secrets_mode', { mode: choice, ...pw }));
      toast('The passwords are kept the new way.');
      st = null;
      load();
      refresh();
    } catch (e) {
      error = e;
    }
  }

  async function lock() {
    try {
      await busy(locking, api('secrets_lock'));
      st = null;
      load();
      refresh();
    } catch (e) {
      fail(e);
    }
  }
</script>

<div class="rb-card rb-settings-card">
  <div class="rb-form-section">
    <h3>
      <Icon name="lock" />Passwords on the flash drive
      {#if badge}<span class="rb-pill {badge[0]} rb-now">{badge[1]}</span>{/if}
    </h3>
    <div class="rb-hint" style="margin-bottom:14px">
      The repository passwords and SSH keys are kept on the flash drive. Encrypted, someone who has only the flash drive - or the whole server
      switched off - cannot read them.
    </div>
    {#if loadError}
      <ErrorBox error={loadError} />
    {:else if !st}
      <div class="rb-hint"><Icon name="loader" class="rb-spin" /> Loading...</div>
    {:else}
      <div>
        <ErrorBox {error} />
        <OptionCards bind:value={choice} options={OPTIONS} />
        <div class="rb-secrets-detail">
          {#if choice === 'key' && !st.key_possible}
            <div class="rb-card-note warn" style="margin-top:0">
              <Icon name="alert" />
              <div>
                The data folder is not on encrypted storage ({st.key_impossible}). A key kept there would protect nothing: anyone with the disks
                could read it. Encrypt the pool, or use a master password.
              </div>
            </div>
          {:else if choice === 'key'}
            <div class="rb-hint">
              The key is kept hidden in the data folder ({st.key_file}), on encrypted storage. Unlocking the array unlocks the passwords; nothing
              else to enter. Keep the repository passwords somewhere safe as well: without the key they cannot be read from the flash drive.
            </div>
          {/if}
          {#if choice === 'master'}
            <div class="rb-fields">
              <Field
                label="Master password"
                hint="At least 8 characters. Entered after every start of the server; until then backups wait and a notification asks for it."
              >
                <input type="password" autocomplete="new-password" bind:value={pw.password} />
              </Field>
              <Field label="Repeat it">
                <input type="password" autocomplete="new-password" class:rb-invalid={mismatch} bind:value={pw.password_repeat} />
                {#if mismatch}<div class="rb-error">The two master passwords differ.</div>{/if}
              </Field>
            </div>
          {/if}
          {#if needCurrent}
            <div class="rb-fields" style="margin-top:10px">
              <Field label="Current master password">
                <input type="password" autocomplete="current-password" bind:value={pw.current_password} />
              </Field>
            </div>
          {/if}
        </div>
        <div class="rb-inline" style="margin-top:14px">
          <Button kind="primary" icon="check" busy={applying.busy} onclick={apply}>Apply</Button>
          {#if st.mode === 'master' && !st.locked}
            <Button icon="lock" busy={locking.busy} onclick={lock}>Lock now</Button>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .rb-secrets-detail {
    margin-top: 14px;
  }
  .rb-now {
    margin-left: auto;
  }
  .rb-invalid {
    border-color: var(--rb-err);
  }
</style>
