<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import Dialog from '../lib/ui/Dialog.svelte';
  import Button from '../lib/ui/Button.svelte';
  import Icon from '../lib/ui/Icon.svelte';
  import StatusPill from './StatusPill.svelte';
  import ProgressBlock from '../lib/ui/ProgressBlock.svelte';
  import { api } from '../lib/api';
  import { app, copyText, fail } from '../lib/state.svelte';
  import { dateTime, plural } from '../lib/format';
  import { what } from './describe';
  import { classify, fileName, pieces, saveText, type Line } from './lines';

  interface LogAnswer {
    text: string;
    offset: number;
    size: number;
    status: string;
    op: Record<string, any>;
  }

  let { id, close }: { id: string; close: (r?: unknown) => void } = $props();

  let lines = $state<Line[]>([]);
  let op = $state<Record<string, any> | null>(null);
  let status = $state('');
  let error = $state('');
  let loaded = $state(false);
  let stick = $state(true);
  let query = $state('');
  let problemsOnly = $state(false);
  let current = $state(-1);
  let saving = $state({ busy: false });
  let pre: HTMLElement | undefined = $state();

  let offset = -1;
  let rest = '';
  let timer: ReturnType<typeof setTimeout> | null = null;
  let alive = true;

  let running = $derived(status === 'running');
  let live = $derived(app.overview?.running.find((o) => o.id === id) ?? null);
  let q = $derived(query.trim().toLowerCase());
  let counts = $derived({
    bad: lines.filter((l) => l.c === 'bad').length,
    warn: lines.filter((l) => l.c === 'warn').length
  });
  let visible = $derived(problemsOnly ? lines.filter((l) => l.c === 'bad' || l.c === 'warn' || l.c === 'note') : lines);
  let hits = $derived(q ? visible.filter((l) => l.t.toLowerCase().includes(q)).map((l) => l.i) : []);

  function add(text: string, final: boolean) {
    const all = rest + text;
    const parts = all.split('\n');
    rest = final ? '' : (parts.pop() ?? '');
    const next: Line[] = [];
    let i = lines.length;
    for (const t of parts) {
      if (t === '') continue;
      next.push({ i: i++, t, c: classify(t) });
    }
    if (next.length) lines.push(...next);
  }

  async function follow() {
    await tick();
    if (stick && pre) pre.scrollTop = pre.scrollHeight;
  }

  async function poll() {
    try {
      const r = await api<LogAnswer>('op_log', { id, offset });
      if (!alive) return;
      op = r.op || {};
      status = r.status;
      if (offset < 0 && r.offset - r.text.length > 0) lines.push({ i: lines.length, t: '(earlier lines left out)', c: 'note' });
      add(r.text, r.status !== 'running');
      offset = r.offset;
      loaded = true;
      follow();
      if (r.status === 'running') timer = setTimeout(poll, 1500);
    } catch (e) {
      if (alive) error = (e as Error).message;
    }
  }

  onMount(poll);
  onDestroy(() => {
    alive = false;
    if (timer) clearTimeout(timer);
  });

  function onScroll() {
    if (!pre) return;
    stick = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 20;
  }

  function setFollow(on: boolean) {
    stick = on;
    follow();
  }

  async function jump(step: number) {
    if (!hits.length) return;
    current = (current + step + hits.length) % hits.length;
    stick = false;
    await tick();
    const el = pre?.querySelector<HTMLElement>(`[data-i="${hits[current]}"]`);
    if (pre && el) pre.scrollTop = el.offsetTop - pre.clientHeight / 2;
  }

  function searchKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      jump(e.shiftKey ? -1 : 1);
    } else if (e.key === 'Escape' && query) {
      e.stopPropagation();
      query = '';
    }
  }

  function copy() {
    copyText(lines.map((l) => l.t + '\n').join('') + rest);
  }

  async function download() {
    saving.busy = true;
    try {
      let at = 0;
      let text = '';
      let info: Record<string, any> = op || {};
      for (let n = 0; n < 200; n++) {
        const r = await api<LogAnswer>('op_log', { id, offset: at });
        info = r.op || info;
        text += r.text;
        if (!r.text || r.offset >= r.size) break;
        at = r.offset;
      }
      saveText(fileName(info), text);
    } catch (e) {
      fail(e);
    } finally {
      saving.busy = false;
    }
  }

</script>

<Dialog title="Log" icon="list" wide onclose={() => close()}>
  <div class="rb-log-head">
    <div class="rb-log-title">
      {#if op}
        <span><b>{what(op)}</b> · {dateTime(op.started)}</span>
        <StatusPill {status} />
      {:else}
        <span>Log</span>
      {/if}
    </div>
    {#if loaded && (counts.bad || counts.warn)}
      <div class="rb-log-counts">
        {#if counts.bad}<span class="rb-err-text">{plural(counts.bad, 'error line')}</span>{/if}
        {#if counts.warn}<span class="rb-warn-text">{plural(counts.warn, 'warning line')}</span>{/if}
      </div>
    {/if}
  </div>

  {#if live}
    <div class="rb-log-progress"><ProgressBlock op={live} /></div>
  {/if}

  {#if loaded}
    <div class="rb-log-tools">
      <div class="rb-log-search">
        <Icon name="search" />
        <input type="search" placeholder="Search the log" aria-label="Search the log" autocomplete="off" spellcheck="false" bind:value={query} oninput={() => (current = -1)} onkeydown={searchKey} />
        {#if q}
          <span class="rb-log-hits">{hits.length ? (current < 0 ? plural(hits.length, 'match', 'matches') : current + 1 + ' of ' + hits.length) : 'No match'}</span>
          <Button icon="up" small kind="ghost" title="Previous match" disabled={!hits.length} onclick={() => jump(-1)} />
          <span class="rb-log-down"><Button icon="up" small kind="ghost" title="Next match" disabled={!hits.length} onclick={() => jump(1)} /></span>
        {/if}
      </div>
      <label class="rb-switch">
        <input type="checkbox" bind:checked={problemsOnly} onchange={() => (current = -1)} />
        <span></span>
        Errors and warnings only
      </label>
      {#if running}
        <label class="rb-switch">
          <input type="checkbox" checked={stick} onchange={(e) => setFollow(e.currentTarget.checked)} />
          <span></span>
          Follow
        </label>
      {/if}
    </div>
  {/if}

  {#if error}
    <pre class="rb-log">{error}</pre>
  {:else}
    <pre class="rb-log" aria-label="Log text" bind:this={pre} onscroll={onScroll}>{#if !loaded}<span class="rb-dim">Loading...</span>{/if}{#each visible as l (l.i)}<span class={l.c === 'note' ? 'rb-dim' : l.c || null} class:hit={q && hits[current] === l.i} data-i={l.i}>{#if q}{#each pieces(l.t, q) as [t, m], k (k)}{#if m}<mark>{t}</mark>{:else}{t}{/if}{/each}{:else}{l.t}{/if}{'\n'}</span>{/each}{#if loaded && problemsOnly && !visible.length}<span class="rb-dim">No errors or warnings in this log.</span>{/if}</pre>
  {/if}

  {#snippet actions()}
    <Button icon="copy" small disabled={!loaded} onclick={copy}>Copy</Button>
    <Button icon="download" small disabled={!loaded} busy={saving.busy} onclick={download}>Download</Button>
    <div class="rb-spacer"></div>
    <Button onclick={() => close()}>Close</Button>
  {/snippet}
</Dialog>

<style>
  .rb-log-head {
    display: flex;
    align-items: center;
    gap: 8px 14px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  .rb-log-title {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1 1 auto;
    min-width: 0;
  }
  .rb-log-counts {
    display: flex;
    gap: 12px;
    font-size: 12.5px;
  }
  .rb-log-progress {
    margin: -6px 0 12px;
  }
  .rb-log-tools {
    display: flex;
    align-items: center;
    gap: 8px 18px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  .rb-log-search {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 260px;
    min-width: 0;
    color: var(--rb-dim);
  }
  .rb-log-search input {
    flex: 1 1 auto;
    min-width: 0;
    margin: 0;
  }
  .rb-log-hits {
    font-size: 12px;
    white-space: nowrap;
  }
  .rb-log-down :global(svg.rb-i) {
    transform: rotate(180deg);
  }
  pre.rb-log {
    position: relative;
  }
  pre.rb-log .warn {
    color: var(--rb-warn);
  }
  pre.rb-log mark {
    background: var(--rb-accent-soft);
    color: inherit;
    border-radius: 2px;
    outline: 1px solid var(--rb-accent-soft);
  }
  pre.rb-log .hit mark {
    background: var(--rb-accent);
    color: var(--rb-panel);
  }
</style>
