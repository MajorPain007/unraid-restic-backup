<script lang="ts" module>
  const rates: Record<string, { bytes: number; at: number; rate: number | null }> = {};
</script>

<script lang="ts">
  import { onProgress } from '../lib/state.svelte';
  import { bytes, count, duration, now, plural } from '../lib/format';
  import type { Op } from '../lib/types';

  let { op }: { op: Op } = $props();

  let live = $state<Op | null>(null);
  let tick = $state(now());
  let cur = $derived(live && live.id === op.id ? live : op);

  $effect(() => {
    const id = op.id;
    const off = onProgress((ops) => {
      const found = ops.find((o) => o.id === id);
      if (found) live = found;
      tick = now();
    });
    const t = setInterval(() => (tick = now()), 1000);
    return () => {
      off();
      clearInterval(t);
    };
  });

  let p = $derived(cur.progress as Record<string, any> | null | undefined);
  let indeterminate = $derived(!p || (!p.total_bytes && !p.percent));
  let pct = $derived(p ? Math.max(0, Math.min(1, p.percent || 0)) : 0);

  let detail = $derived.by(() => {
    if (!p) return '';
    const parts: string[] = [];
    if (p.total_bytes) parts.push(bytes(p.bytes_done) + ' of ' + bytes(p.total_bytes));
    if (p.total_files) parts.push(count(p.files_done) + ' of ' + count(p.total_files) + ' files');
    return (cur.phase || '') + (parts.length ? ' · ' + parts.join(' · ') : '');
  });

  let right = $derived.by(() => {
    if (indeterminate || !p) return duration(tick - cur.started);
    const t = now();
    let r = rates[cur.id];
    if (r && p.bytes_done >= r.bytes && t > r.at) {
      const inst = (p.bytes_done - r.bytes) / (t - r.at);
      r.rate = r.rate === null ? inst : r.rate * 0.7 + inst * 0.3;
      r.bytes = p.bytes_done;
      r.at = t;
    } else {
      rates[cur.id] = r = { bytes: p.bytes_done || 0, at: t, rate: null };
    }
    const bits: string[] = [];
    if (r.rate) bits.push(bytes(r.rate) + '/s');
    const eta = p.eta !== null && p.eta !== undefined ? p.eta : r.rate && p.total_bytes ? (p.total_bytes - p.bytes_done) / r.rate : null;
    if (eta !== null && pct > 0.01) bits.push(duration(eta) + ' left');
    if (p.errors) bits.push(plural(p.errors, 'error'));
    return bits.join(' · ');
  });
</script>

<div class="rb-progress">
  <div class="rb-progress-track" class:indeterminate>
    <div class="rb-progress-bar" style:width={indeterminate ? null : (pct * 100).toFixed(1) + '%'}></div>
  </div>
  <div class="rb-progress-text">
    <span>
      {#if indeterminate}
        <b>{cur.phase || 'Working'}</b>
      {:else}
        <b>{Math.floor(pct * 100)}%</b>&nbsp;&nbsp;{detail}
      {/if}
    </span>
    <span>{right}</span>
  </div>
  <div class="rb-current" title={p?.current || ''}>{indeterminate ? '' : p?.current || ''}</div>
</div>
