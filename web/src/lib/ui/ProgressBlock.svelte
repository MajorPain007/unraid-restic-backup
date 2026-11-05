<script lang="ts" module>
  const rates = new Map<string, { bytes: number; at: number; rate: number | null }>();
</script>

<script lang="ts">
  import { app } from '../state.svelte';
  import { bytes, count, duration, now, plural } from '../format';
  import type { Op } from '../types';

  let { op }: { op: Op } = $props();

  let live = $derived(app.overview?.running.find((r) => r.id === op.id) ?? op);
  let p = $derived(live.progress as Record<string, any> | null | undefined);
  let known = $derived(!!p && !!(p.total_bytes || p.percent));
  let pct = $derived(known ? Math.max(0, Math.min(1, p!.percent || 0)) : 0);
  let rate = $state<number | null>(null);

  $effect(() => {
    const prog = p;
    if (!known || !prog) return;
    const t = now();
    let r = rates.get(live.id);
    if (r && prog.bytes_done >= r.bytes && t > r.at) {
      const inst = (prog.bytes_done - r.bytes) / (t - r.at);
      r.rate = r.rate === null ? inst : r.rate * 0.7 + inst * 0.3;
      r.bytes = prog.bytes_done;
      r.at = t;
    } else if (!r || prog.bytes_done < r.bytes) {
      r = { bytes: prog.bytes_done || 0, at: t, rate: null };
      rates.set(live.id, r);
      if (rates.size > 50) rates.delete(rates.keys().next().value as string);
    }
    rate = r.rate;
  });

  let detail = $derived.by(() => {
    if (!known || !p) return '';
    const parts: string[] = [];
    if (p.total_bytes) parts.push(bytes(p.bytes_done) + ' of ' + bytes(p.total_bytes));
    if (p.total_files) parts.push(count(p.files_done) + ' of ' + count(p.total_files) + ' files');
    return '  ' + live.phase + (parts.length ? ' · ' + parts.join(' · ') : '');
  });

  let right = $derived.by(() => {
    if (!known || !p) return duration(now() - live.started);
    const bits: string[] = [];
    if (rate) bits.push(bytes(rate) + '/s');
    const eta = p.eta !== null && p.eta !== undefined ? p.eta : rate && p.total_bytes ? (p.total_bytes - p.bytes_done) / rate : null;
    if (eta !== null && pct > 0.01) bits.push(duration(eta) + ' left');
    if (p.errors) bits.push(plural(p.errors, 'error'));
    return bits.join(' · ');
  });
</script>

<div class="rb-progress">
  <div
    class="rb-progress-track"
    class:indeterminate={!known}
    role="progressbar"
    aria-valuemin="0"
    aria-valuemax="100"
    aria-valuenow={known ? Math.floor(pct * 100) : undefined}
    aria-label={live.phase || 'Working'}
  >
    <div class="rb-progress-bar" style:width={known ? (pct * 100).toFixed(1) + '%' : null}></div>
  </div>
  <div class="rb-progress-text">
    {#if known}
      <span><b>{Math.floor(pct * 100)}%</b>{detail}</span>
    {:else}
      <span><b>{live.phase || 'Working'}</b></span>
    {/if}
    <span>{right}</span>
  </div>
  <div class="rb-current" title={known ? p?.current || '' : null}>{known ? p?.current || '' : ''}</div>
</div>
