<script module lang="ts">
  const rates = new Map<string, { bytes: number; at: number; rate: number | null }>();
</script>

<script lang="ts">
  import type { Op } from '../lib/types';
  import { bytes, count, duration, now, plural } from '../lib/format';

  let { op }: { op: Op } = $props();

  let rate = $state<number | null>(null);

  let p = $derived(op.progress);
  let known = $derived(!!p && !!(p.total_bytes || p.percent));
  let pct = $derived(known ? Math.max(0, Math.min(1, p?.percent || 0)) : 0);

  $effect(() => {
    if (!p || !known) return;
    const t = now();
    let r = rates.get(op.id);
    if (r && p.bytes_done >= r.bytes && t > r.at) {
      const inst = (p.bytes_done - r.bytes) / (t - r.at);
      r.rate = r.rate === null ? inst : r.rate * 0.7 + inst * 0.3;
      r.bytes = p.bytes_done;
      r.at = t;
    } else {
      r = { bytes: p.bytes_done || 0, at: t, rate: null };
      rates.set(op.id, r);
    }
    if (rates.size > 50) {
      const first = rates.keys().next().value;
      if (first !== undefined && first !== op.id) rates.delete(first);
    }
    rate = r.rate;
  });

  let detail = $derived.by(() => {
    if (!p) return '';
    const parts: string[] = [];
    if (p.total_bytes) parts.push(bytes(p.bytes_done) + ' of ' + bytes(p.total_bytes));
    if (p.total_files) parts.push(count(p.files_done) + ' of ' + count(p.total_files) + ' files');
    return '  ' + op.phase + (parts.length ? ' · ' + parts.join(' · ') : '');
  });

  let right = $derived.by(() => {
    if (!known || !p) return duration(now() - op.started);
    const bits: string[] = [];
    if (rate) bits.push(bytes(rate) + '/s');
    const eta = p.eta !== null && p.eta !== undefined ? p.eta : rate && p.total_bytes ? (p.total_bytes - p.bytes_done) / rate : null;
    if (eta !== null && pct > 0.01) bits.push(duration(eta) + ' left');
    if (p.errors) bits.push(plural(p.errors, 'error'));
    return bits.join(' · ');
  });
</script>

<div class="rb-progress">
  <div class="rb-progress-track" class:indeterminate={!known}>
    <div class="rb-progress-bar" style:width={known ? (pct * 100).toFixed(1) + '%' : null}></div>
  </div>
  <div class="rb-progress-text">
    {#if known}
      <span><b>{Math.floor(pct * 100)}%</b>{detail}</span>
    {:else}
      <span><b>{op.phase || 'Working'}</b></span>
    {/if}
    <span>{right}</span>
  </div>
  <div class="rb-current" title={known ? p?.current || '' : null}>{known ? p?.current || '' : ''}</div>
</div>
