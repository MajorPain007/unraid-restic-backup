<script lang="ts" module>
  import { provide } from '../lib/actions';
  import { jobConfig, openDialog } from '../lib/state.svelte';
  import JobEditor from '../jobs/JobEditor.svelte';
  import PreviewDialog from '../jobs/PreviewDialog.svelte';

  provide({
    editJob: (id: string | null) => {
      openDialog(JobEditor, { id });
    },
    previewJob: (jobId: string) => {
      const job = jobConfig(jobId);
      if (job) openDialog(PreviewDialog, { job: JSON.parse(JSON.stringify(job)) });
    }
  });
</script>

<script lang="ts">
  import { flip } from 'svelte/animate';
  import { fade } from 'svelte/transition';
  import Icon from '../lib/ui/Icon.svelte';
  import Button from '../lib/ui/Button.svelte';
  import JobCard from '../jobs/JobCard.svelte';
  import { actions } from '../lib/actions';
  import { app, jobInfo } from '../lib/state.svelte';

  let { opts = {} }: { opts?: Record<string, any> } = $props();

  let o = $derived(app.overview);
  let jobs = $derived(o?.config.jobs ?? []);
</script>

{#if o && !o.config.repos.length}
  <div class="rb-empty">
    <Icon name="archive" class="big" />
    <h3>Back up your server with restic</h3>
    <p>Backups are encrypted and deduplicated. They can go to another disk, to a second server over SSH, or to a restic REST server.</p>
    <div class="rb-steps">
      <span><b>1</b>Add a repository - where backups go</span>
      <span><b>2</b>Create a job - what to back up, and when</span>
      <span><b>3</b>Done - it runs on its own</span>
    </div>
    <Button icon="plus" kind="primary" onclick={() => actions.editRepo(null)}>Add repository</Button>
  </div>
{:else if !jobs.length}
  <div class="rb-empty">
    <Icon name="archive" class="big" />
    <h3>Create your first backup job</h3>
    <p>A job says which folders go into which repository, when it runs, and how many old snapshots to keep.</p>
    <Button icon="plus" kind="primary" onclick={() => actions.editJob(null)}>New job</Button>
  </div>
{:else}
  <div class="rb-grid">
    {#each jobs as cfg (cfg.id)}
      <div class="rb-jv-cell" animate:flip={{ duration: 200 }} in:fade={{ duration: 150 }}>
        <JobCard {cfg} info={jobInfo(cfg.id) ?? {}} />
      </div>
    {/each}
    <button type="button" class="rb-card add" onclick={() => actions.editJob(null)}><Icon name="plus" />New job</button>
  </div>
{/if}

<style>
  .rb-jv-cell {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .rb-jv-cell > :global(.rb-card) {
    flex: 1;
  }
</style>
