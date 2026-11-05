<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import Timeline from './Timeline.svelte';
  import { bytes, clock, dayLabel } from '../lib/format';
  import type { Snapshot } from '../lib/types';
  import { S, hostSuffix, jobName, matchesJob, openSnapshot } from './store.svelte';

  let listEl: HTMLElement | undefined = $state();

  let shown = $derived((S.list || []).filter((s) => matchesJob(s, S.job)));

  let groups = $derived.by(() => {
    const out: { key: string; ts: number; snaps: Snapshot[] }[] = [];
    for (const s of shown) {
      const key = new Date(s.time * 1000).toDateString();
      const last = out[out.length - 1];
      if (last && last.key === key) last.snaps.push(s);
      else out.push({ key, ts: s.time, snaps: [s] });
    }
    return out;
  });

  let activeDay = $derived(S.snap ? new Date(S.snap.time * 1000).toDateString() : '');

  onMount(() => {
    tick().then(() => {
      if (listEl) listEl.scrollTop = S.scroll.list;
    });
  });

  function jumpTo(day: string) {
    if (!listEl) return;
    const head = listEl.querySelector<HTMLElement>(`[data-day="${CSS.escape(day)}"]`);
    const first = head?.nextElementSibling as HTMLElement | null;
    if (head && first) listEl.scrollTo({ top: first.offsetTop - head.offsetHeight, behavior: 'smooth' });
  }

  function keydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const btns = [...(listEl?.querySelectorAll<HTMLButtonElement>('button.rb-snap') ?? [])];
    const i = btns.indexOf(document.activeElement as HTMLButtonElement);
    if (i < 0) return;
    e.preventDefault();
    btns[Math.max(0, Math.min(btns.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))].focus();
  }
</script>

<div class="snapcol">
  {#if shown.length}
    <Timeline snaps={shown} active={activeDay} onpick={jumpTo} />
  {/if}
  <div class="rb-snaplist" bind:this={listEl} onscroll={() => listEl && (S.scroll.list = listEl.scrollTop)} onkeydown={keydown} role="presentation">
    {#if S.loading && !S.list}
      <div class="rb-hint pad"><Icon name="loader" class="rb-spin" /> Loading snapshots...</div>
    {:else if S.error}
      <div class="rb-card-note err note"><Icon name="alert" /><div>{S.error}</div></div>
    {:else if !shown.length}
      <div class="rb-hint pad">No snapshots yet.</div>
    {:else}
      {#each groups as g (g.key)}
        <div class="rb-snap-day" data-day={g.key}>{dayLabel(g.ts)}<span class="n">{g.snaps.length}</span></div>
        {#each g.snaps as s (s.id)}
          <button type="button" class="rb-snap" class:active={S.snap?.id === s.id} title={s.id} onclick={() => openSnapshot(s)}>
            <span class="rb-snap-time">{clock(s.time)}</span>
            <span class="rb-snap-main">{jobName(s) + hostSuffix(s)}</span>
            {#if s.summary}
              <span class="rb-snap-size" title="Added by this snapshot">+{bytes(s.summary.added)}</span>
            {/if}
          </button>
        {/each}
      {/each}
    {/if}
  </div>
</div>

<style>
  .snapcol {
    min-width: 0;
  }
  :global(#rb-app) .rb-snaplist {
    position: relative;
  }
  :global(#rb-app) .rb-snap-day {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .n {
    font-weight: 500;
    color: var(--rb-faint);
    letter-spacing: 0;
  }
  :global(#rb-app) .pad {
    padding: 16px;
  }
  :global(#rb-app) .note {
    margin: 12px;
  }
  .pad :global(.rb-spin) {
    animation: rb-spin 0.9s linear infinite;
  }
  :global(#rb-app) .rb-snaplist :global(button.rb-snap:focus-visible) {
    outline: 2px solid var(--rb-accent);
    outline-offset: -2px;
  }
</style>
