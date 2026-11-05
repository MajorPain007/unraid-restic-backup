<script lang="ts" module>
  import { provide } from '../lib/actions';
  import { openDialog } from '../lib/state.svelte';
  import { editRepo } from '../repos/flows';
  import SizeHistory from '../sizes/SizeHistory.svelte';

  provide({
    editRepo: (id) => {
      editRepo(id);
    },
    sizeHistory: (which) => {
      openDialog(SizeHistory, { which });
    }
  });
</script>

<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import RepoCard from '../repos/RepoCard.svelte';
  import { actions } from '../lib/actions';
  import { app, repoInfo } from '../lib/state.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let repos = $derived(app.overview?.config.repos ?? []);
</script>

{#if !repos.length}
  <div class="rb-empty">
    <Icon name="repo" class="big" />
    <h3>No repository yet</h3>
    <p>A repository is where restic keeps the backups: a folder on another disk, a second server reached over SSH, or a restic REST server.</p>
    <Button icon="plus" kind="primary" onclick={() => actions.editRepo(null)}>Add repository</Button>
  </div>
{:else}
  <div class="rb-grid">
    {#each repos as repo (repo.id)}
      <RepoCard {repo} info={repoInfo(repo.id)} />
    {/each}
    <button type="button" class="rb-card add" onclick={() => actions.editRepo(null)}><Icon name="plus" />Add repository</button>
  </div>
{/if}
