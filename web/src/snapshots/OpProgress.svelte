<script lang="ts" module>
  const rates = new Map<string, { bytes: number; at: number; rate: number | null }>();
</script>

<script lang="ts">
  import type { Op } from '../lib/types';
  import { bytes, duration, now, plural } from '../lib/format';

  let { op }: { op: Op } = $props();

  let p = $derived(op.progress);
  let determinate = $derived(!!p && !!(p.total_bytes || p.percent));
  let pct = $derived(determinate ? Math.max(0, Math.min(1, p?.percent || 0)) : 0);

  let detail = $derived.by(() => {
    if (!p || !determinate) return '';
    const parts: string[] = [];
    if (p.total_bytes) parts.push(bytes(p.bytes_done) + ' of ' + bytes(p.total_bytes));
    if (p.total_files) parts.push(Number(p.files_done || 0).toLocaleString('en-GB') + ' of ' + Number(p.total_files).toLocaleString('en-GB') + ' files');
    return '  ' + (op.phase || '') + (parts.length ? ' · ' + parts.join(' · ') : '');
  });

  let right = $derived.by(() => {
    if (!p || !determinate) return duration(now() - op.started);
    let r = rates.get(op.id);
    const t = now();
    if (r && p.bytes_done >= r.bytes && t > r.at) {
      const inst = (p.bytes_done - r.bytes) / (t - r.at);
      r.rate = r.rate === null ? inst : r.rate * 0.7 + inst * 0.3;
      r.bytes = p.bytes_done;
      r.at = t;
    } else {
      r = { bytes: p.bytes_done || 0, at: t, rate: null };
      rates.set(op.id, r);
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
  <div class="rb-progress-track" class:indeterminate={!determinate}>
    <div class="rb-progress-bar" style:width={determinate ? (pct * 100).toFixed(1) + '%' : null}></div>
  </div>
  <div class="rb-progress-text">
    <span>
      {#if determinate}<b>{Math.floor(pct * 100)}%</b>{detail}{:else}<b>{op.phase || 'Working'}</b>{/if}
    </span>
    <span>{right}</span>
  </div>
  <div class="rb-current" title={determinate ? p?.current || '' : null}>{determinate ? p?.current || '' : ''}</div>
</div>
