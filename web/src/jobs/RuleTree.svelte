<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import RuleTreeLevel from './RuleTreeLevel.svelte';
  import { exact, removeAll, type TreeAction, type TreeEntry } from './rules';
  import type { Job } from '../lib/types';

  let {
    job = $bindable(),
    edit = false,
    onchange
  }: {
    job: Job;
    edit?: boolean;
    onchange?: () => void;
  } = $props();

  const opened = new SvelteSet<string>();
  let gen = $state(0);
  let fetchedKey = '';

  let key = $derived(
    JSON.stringify([job.repo, job.sources, job.excludes, job.iexcludes, job.exclude_caches, job.exclude_larger_than, job.one_file_system])
  );

  function bump() {
    fetchedKey = untrack(() => key);
    gen++;
  }

  $effect(() => {
    if (key === fetchedKey) return;
    if (untrack(() => gen) === 0) {
      untrack(bump);
      return;
    }
    const t = setTimeout(bump, 350);
    return () => clearTimeout(t);
  });

  export function refresh() {
    bump();
  }

  function request() {
    return untrack(() => $state.snapshot(job));
  }

  function change(fn: () => void) {
    fn();
    onchange?.();
    bump();
  }

  function action(e: TreeEntry): TreeAction | null {
    if (!edit) return null;
    const p = exact(e.path);
    if (!e.out) {
      if (job.excludes.includes('!' + p)) return { title: 'Leave out again', run: () => change(() => removeAll(job.excludes, '!' + p)) };
      return { title: 'Leave out', run: () => change(() => job.excludes.push(p)) };
    }
    if (e.out.via) return null;
    if (e.out.kind === 'pattern' && e.out.label === p) return { title: 'Back up again', run: () => change(() => removeAll(job.excludes, p)) };
    if (e.out.kind === 'pattern') return { title: 'Back up anyway, despite ' + e.out.label, run: () => change(() => job.excludes.push('!' + p)) };
    return null;
  }
</script>

<div class="rb-rtree">
  <div class="rb-rtree-body">
    {#if gen > 0}
      <RuleTreeLevel path="" depth={0} {gen} {request} {opened} {action} />
    {/if}
  </div>
</div>
