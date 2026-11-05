<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import SizeTree, { type SizeNode } from './SizeTree.svelte';
  import { api } from '../lib/api';
  import { fail } from '../lib/state.svelte';
  import { bytes, count, duration, plural } from '../lib/format';
  import type { Job } from '../lib/types';

  interface Rule {
    kind: string;
    label: string;
    measure: boolean;
    files: number;
    dirs: number;
    bytes: number;
    examples: string[];
  }

  interface Result {
    cancelled: boolean;
    seconds: number;
    total: { files: number; dirs: number; bytes: number };
    sources: { path: string; files: number; dirs: number; bytes: number; tree: SizeNode | null; missing: boolean }[];
    rules: Rule[];
    largest: { path: string; bytes: number }[];
    error_count: number;
    errors: string[];
  }

  const AUTO_RULES: Record<string, string> = {
    zfs: 'ZFS snapshot folders (.zfs)',
    plugin: 'Kept out by the plugin: ',
    caches: 'Cache folders (with a CACHEDIR.TAG)',
    otherfs: 'Other file systems',
    size: 'Files larger than '
  };

  let { job, close }: { job: Job; close: (r?: unknown) => void } = $props();

  let id: string | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let finished = false;
  let figure = $state('Starting...');
  let current = $state('');
  let error = $state('');
  let result = $state<Result | null>(null);

  function files(n: number): string {
    return count(n) + (Number(n) === 1 ? ' file' : ' files');
  }

  function failed(message: string) {
    finished = true;
    error = message;
  }

  async function poll() {
    try {
      const s = await api<{ status: string; error?: string; progress?: { bytes: number; files: number; current?: string }; result: Result }>(
        'job_preview_status',
        { id }
      );
      if (s.status === 'starting' || s.status === 'running') {
        if (s.progress) {
          figure = bytes(s.progress.bytes) + ' in ' + count(s.progress.files) + ' files so far';
          current = s.progress.current || '';
        }
        timer = setTimeout(poll, 1000);
        return;
      }
      if (s.status === 'error') {
        failed(s.error || 'The preview failed.');
        return;
      }
      finished = true;
      result = s.result;
    } catch (e) {
      failed((e as Error).message);
    }
  }

  onMount(() => {
    api<{ id: string }>('job_preview', { job })
      .then((r) => {
        id = r.id;
        poll();
      })
      .catch((e: Error) => failed(e.message));
  });

  onDestroy(() => {
    clearTimeout(timer);
    if (id && !finished) api('job_preview_stop', { id }).catch(() => {});
  });

  function stop() {
    if (id) api('job_preview_stop', { id }).catch(fail);
  }

  function ruleLabel(rule: Rule): string {
    return (AUTO_RULES[rule.kind] ?? rule.kind) + (rule.kind === 'size' || rule.kind === 'plugin' ? rule.label : '');
  }

  let unmatched = $derived(
    !!result && result.rules.some((r) => r.files === 0 && r.dirs === 0 && (r.kind === 'pattern' || r.kind === 'ipattern'))
  );
  let trees = $derived(result ? result.sources.filter((s) => s.tree && s.bytes > 0) : []);
</script>

<Dialog title={'Preview' + (job.name ? ` of "${job.name}"` : '')} icon="eye" wide onclose={() => close()}>
  {#if error}
    <div class="rb-card-note err"><Icon name="alert" /><div>{error}</div></div>
  {:else if !result}
    <div>
      <p>Going through the folders with the job's rules. Only names and sizes are read; nothing is backed up.</p>
      <div class="rb-progress">
        <div class="rb-progress-track indeterminate"><div class="rb-progress-bar"></div></div>
        <div class="rb-progress-text">
          <span><b>{figure}</b></span>
          <Button small kind="ghost" onclick={stop}>Stop</Button>
        </div>
        <div class="rb-current" title={current}>{current}</div>
      </div>
    </div>
  {:else}
    {@const r = result}
    <div class="rb-preview" in:fade={{ duration: 150 }}>
      {#if r.cancelled}
        <div class="rb-callout warn rb-pv-gap"><Icon name="alert" /><div>Stopped: the numbers cover only the part looked at so far.</div></div>
      {/if}
      <div class="rb-facts four">
        <div class="rb-fact"><div class="rb-label">Would be backed up</div><div class="rb-value">{bytes(r.total.bytes)}</div></div>
        <div class="rb-fact"><div class="rb-label">Files</div><div class="rb-value">{count(r.total.files)}</div></div>
        <div class="rb-fact"><div class="rb-label">Folders</div><div class="rb-value">{count(r.total.dirs)}</div></div>
        <div class="rb-fact"><div class="rb-label">Looked through in</div><div class="rb-value">{duration(Math.max(1, Math.round(r.seconds)))}</div></div>
      </div>
      {#each r.sources.filter((s) => s.missing) as s (s.path)}
        <div class="rb-card-note err"><Icon name="alert" /><div>{s.path} does not exist.</div></div>
      {/each}

      <h3 class="rb-preview-head">Left out</h3>
      {#if !r.rules.length}
        <div class="rb-hint">Nothing: the job has no exclude rules.</div>
      {:else}
        <div class="rb-table-wrap">
          <table class="rb-table">
            <tbody>
              {#each r.rules as rule, i (i)}
                {@const nothing = rule.files === 0 && rule.dirs === 0}
                <tr title={rule.examples?.length ? 'For example:\n' + rule.examples.join('\n') : ''}>
                  <td>
                    {#if rule.kind === 'pattern' || rule.kind === 'ipattern'}
                      <span><code>{rule.label}</code>{#if rule.kind === 'ipattern'}<span class="rb-hint">&nbsp; ignoring case</span>{/if}</span>
                    {:else}
                      <span>{ruleLabel(rule)}</span>
                    {/if}
                  </td>
                  <td class="num">
                    {#if nothing}
                      <span class="dim">matches nothing</span>
                    {:else if !rule.measure}
                      <span class="dim">{plural(rule.dirs, 'folder')}, not counted</span>
                    {:else}
                      <span><b>{bytes(rule.bytes)}</b>&nbsp; {files(rule.files)}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if unmatched}
          <div class="rb-hint rb-pv-note">A rule that matches nothing may be misspelled - or there is simply nothing of that kind.</div>
        {/if}
      {/if}

      {#if trees.length}
        <h3 class="rb-preview-head">Where the space is</h3>
        {#each trees as s (s.path)}
          <div class="rb-tree"><SizeTree node={s.tree!} total={s.bytes} title={s.path} /></div>
        {/each}
      {/if}

      {#if r.largest.length}
        <h3 class="rb-preview-head">Largest files</h3>
        <div class="rb-table-wrap">
          <table class="rb-table">
            <tbody>
              {#each r.largest as f, i (i)}
                <tr>
                  <td class="rb-mono rb-pv-path">{f.path}</td>
                  <td class="num">{bytes(f.bytes)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

      {#if r.error_count}
        <div class="rb-callout warn rb-pv-errors">
          <Icon name="alert" />
          <div>
            {plural(r.error_count, 'folder')} could not be read; a backup would warn about them too.
            <div class="rb-mono rb-pv-errlist">{r.errors.join('\n')}</div>
          </div>
        </div>
      {/if}
    </div>
  {/if}
  {#snippet actions()}
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .rb-pv-gap {
    margin-bottom: 14px;
  }
  .rb-card-note {
    margin-top: 12px;
  }
  .rb-pv-note {
    margin-top: 6px;
  }
  .rb-pv-path {
    word-break: break-all;
  }
  .rb-pv-errors {
    margin-top: 14px;
  }
  .rb-pv-errlist {
    margin-top: 6px;
    font-size: 12px;
    white-space: pre-wrap;
  }
</style>
