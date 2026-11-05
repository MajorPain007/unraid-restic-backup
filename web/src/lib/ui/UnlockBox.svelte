<script lang="ts">
  import Icon from './Icon.svelte';
  import Button from './Button.svelte';
  import { api } from '../api';
  import { fail, refresh, toast } from '../state.svelte';

  let password = $state('');
  let working = $state(false);

  async function unlock() {
    working = true;
    try {
      await api('secrets_unlock', { password });
      password = '';
      toast('Unlocked. Scheduled backups run again.');
      await refresh();
    } catch (e) {
      fail(e);
    } finally {
      working = false;
    }
  }
</script>

<div class="rb-card-note warn rb-unlock">
  <Icon name="lock" />
  <div class="rb-unlock-body">
    <div>The repository passwords are locked since the server started. Backups wait until you enter the master password.</div>
    <form
      class="rb-inline"
      onsubmit={(e) => {
        e.preventDefault();
        unlock();
      }}
    >
      <input type="password" placeholder="Master password" autocomplete="current-password" bind:value={password} />
      <Button type="submit" kind="primary" small icon="unlock" busy={working}>Unlock</Button>
    </form>
  </div>
</div>

<style>
  .rb-unlock {
    margin: 0 0 14px;
  }
  .rb-unlock-body {
    flex: 1;
  }
  .rb-unlock form {
    margin-top: 8px;
  }
  .rb-unlock input {
    width: 220px;
    margin: 0;
  }
</style>
