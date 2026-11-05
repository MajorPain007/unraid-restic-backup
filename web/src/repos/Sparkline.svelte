<script lang="ts">
  import { bytes, shortDate } from '../lib/format';

  let { points, onclick }: { points: number[][]; onclick: () => void } = $props();

  const W = 300;
  const H = 34;

  let first = $derived(points[0]);
  let last = $derived(points[points.length - 1]);
  let change = $derived(last[1] - first[1]);

  let path = $derived.by(() => {
    const t0 = first[0];
    const t1 = Math.max(last[0], t0 + 1);
    const vs = points.map((p) => p[1]);
    const lo = Math.min(...vs);
    const hi = Math.max(...vs);
    const span = hi - lo || Math.max(hi, 1);
    const xy = points.map((p) => [((p[0] - t0) / (t1 - t0)) * W, H - 3 - ((p[1] - lo) / span) * (H - 6)]);
    const line = xy.map(([x, y], i) => (i ? 'L' : 'M') + x.toFixed(1) + ' ' + y.toFixed(1)).join(' ');
    return { line, area: line + ' L' + W + ' ' + H + ' L0 ' + H + ' Z' };
  });

  let text = $derived(
    (change === 0 ? 'Unchanged' : (change > 0 ? '+' : '-') + bytes(Math.abs(change))) + ' since ' + shortDate(first[0])
  );
</script>

<button type="button" class="rb-spark" title="Size history" {onclick}>
  <svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" aria-hidden="true">
    <path d={path.area} class="area" />
    <path d={path.line} class="line" vector-effect="non-scaling-stroke" />
  </svg>
  <span class="rb-spark-text">{text}</span>
</button>

<style>
  :global(#rb-app) .rb-spark {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    margin: 12px 0 0;
    padding: 6px 8px;
    border: 1px solid var(--rb-border);
    border-radius: var(--rb-radius-sm);
    background: transparent;
    color: var(--rb-dim);
    font: inherit;
    font-size: 12px;
    cursor: pointer;
    text-align: left;
    transition: border-color 0.15s;
  }
  :global(#rb-app) .rb-spark:hover {
    border-color: var(--rb-border-strong);
    color: var(--rb-text);
  }
  :global(#rb-app) svg {
    flex: 1 1 auto;
    min-width: 0;
    height: 26px;
    display: block;
  }
  :global(#rb-app) .area {
    fill: var(--rb-select);
  }
  :global(#rb-app) .line {
    fill: none;
    stroke: var(--rb-accent);
    stroke-width: 1.6;
    stroke-linejoin: round;
  }
  :global(#rb-app) .rb-spark-text {
    flex: none;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
</style>
