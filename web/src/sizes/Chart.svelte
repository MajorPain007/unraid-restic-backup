<script lang="ts" module>
  const DAY = 86400;

  function niceStep(raw: number): number {
    let unit = 1;
    while (raw / unit >= 1024) unit *= 1024;
    const m = raw / unit;
    const pow = Math.pow(10, Math.floor(Math.log10(m)));
    const step = [1, 2, 2.5, 5, 10].map((k) => k * pow).find((t) => t >= m) ?? 10 * pow;
    return step >= 1000 ? 1024 * unit : step * unit;
  }

  function axis(lo: number, hi: number, fromZero: boolean): number[] {
    hi = Math.max(hi, 1024);
    const base = fromZero || lo <= hi * 0.5 ? 0 : lo;
    const step = niceStep(Math.max(hi - base, hi * 0.02) / 4);
    const start = Math.floor(base / step) * step;
    const ticks: number[] = [];
    for (let k = 0; k === 0 || start + (k - 1) * step < hi; k++) ticks.push(start + k * step);
    return ticks;
  }

  let seq = 0;

  function reducedMotion(): boolean {
    try {
      return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    } catch {
      return false;
    }
  }
</script>

<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import { bytes, dateTime, shortDate } from '../lib/format';

  type Point = number[];

  let {
    points,
    from,
    to,
    kind = 'line',
    height = 170,
    label = '',
    value,
    detail,
    empty = 'Nothing recorded in this time yet.',
    delta = false
  }: {
    points: Point[];
    from: number;
    to: number;
    kind?: 'line' | 'bars';
    height?: number;
    label?: string;
    value: (p: Point) => number;
    detail?: (p: Point) => string;
    empty?: string;
    delta?: boolean;
  } = $props();

  const W = 720;
  const L = 62;
  const R = 12;
  const T = 12;
  const B = 26;
  const clip = 'rb-chart-clip-' + ++seq;

  let svg: SVGSVGElement | undefined = $state();
  let hover = $state<Point | null>(null);

  let sorted = $derived([...points].sort((a, b) => a[0] - b[0]));
  let pts = $derived(sorted.filter((p) => p[0] >= from && p[0] <= to));
  let values = $derived(pts.map((p) => value(p)));
  let ticks = $derived(
    pts.length ? axis(Math.min(...values), Math.max(...values), kind === 'bars' || pts.length < 2) : [0, 1024]
  );

  const dom = Tween.of(() => ({ from, min: ticks[0], max: ticks[ticks.length - 1] }), {
    duration: reducedMotion() ? 0 : 420,
    easing: cubicOut
  });

  let d = $derived(dom.current);
  let x = $derived((t: number) => L + ((t - d.from) / Math.max(1, to - d.from)) * (W - L - R));
  let y = $derived((v: number) => T + (1 - (v - d.min) / (d.max - d.min || 1)) * (height - T - B));
  let drawn = $derived(sorted.filter((p) => p[0] >= Math.min(d.from, from) && p[0] <= to));
  let spanDays = $derived((to - from) / DAY);

  let line = $derived(drawn.map((p, n) => (n ? 'L' : 'M') + x(p[0]).toFixed(1) + ' ' + y(value(p)).toFixed(1)).join(' '));
  let area = $derived(
    drawn.length
      ? line + ' L' + x(drawn[drawn.length - 1][0]).toFixed(1) + ' ' + (height - B) + ' L' + x(drawn[0][0]).toFixed(1) + ' ' + (height - B) + ' Z'
      : ''
  );

  let barWidth = $derived.by(() => {
    let gap = to - from;
    for (let j = 1; j < pts.length; j++) gap = Math.min(gap, pts[j][0] - pts[j - 1][0]);
    return Math.max(1.5, Math.min(18, (gap / Math.max(1, to - from)) * (W - L - R) * 0.7));
  });

  let xLabels = $derived(
    [0, 1, 2, 3, 4].map((k) => {
      const t = from + ((to - from) * k) / 4;
      return { t, x: L + ((W - L - R) * k) / 4, anchor: k === 0 ? 'start' : k === 4 ? 'end' : 'middle' };
    })
  );

  let previous = $derived.by(() => {
    if (!hover || !delta) return null;
    const i = pts.indexOf(hover);
    return i > 0 ? pts[i - 1] : null;
  });

  let change = $derived(hover && previous ? value(hover) - value(previous) : null);

  function move(e: PointerEvent) {
    if (!svg || !pts.length) return;
    const box = svg.getBoundingClientRect();
    const at = from + (((e.clientX - box.left) / box.width) * W - L) / (W - L - R) * (to - from);
    hover = pts.reduce((a, p) => (Math.abs(p[0] - at) < Math.abs(a[0] - at) ? p : a), pts[0]);
  }

  let tipLeft = $derived.by(() => {
    if (!hover || !svg) return 0;
    const w = svg.getBoundingClientRect().width;
    return Math.min(Math.max((x(hover[0]) / W) * w, 70), w - 70);
  });

  let tipTop = $derived.by(() => {
    if (!hover || !svg) return 0;
    return (y(value(hover)) / height) * svg.getBoundingClientRect().height;
  });

  function signed(n: number): string {
    if (n === 0) return 'no change';
    return (n > 0 ? '+' : '-') + bytes(Math.abs(n));
  }
</script>

<div class="rb-chart">
  {#if !pts.length}
    <div class="rb-chart-empty">{empty}</div>
  {:else}
    <svg
      bind:this={svg}
      viewBox="0 0 {W} {height}"
      class="rb-chart-svg"
      role="img"
      aria-label={label}
      onpointermove={move}
      onpointerdown={move}
      onpointerleave={() => (hover = null)}
    >
      <defs>
        <clipPath id={clip}><rect x={L} y={0} width={W - L - R} height={height} /></clipPath>
      </defs>
      {#each ticks as v (v)}
        {@const ty = y(v)}
        {#if ty >= T - 1 && ty <= height - B + 1}
          <line x1={L} x2={W - R} y1={ty} y2={ty} class="rb-chart-grid" />
          <text x={L - 8} y={ty + 4} text-anchor="end" class="rb-chart-label">{v === 0 ? '0' : bytes(v)}</text>
        {/if}
      {/each}
      {#each xLabels as l (l.x)}
        <text x={l.x} y={height - 7} text-anchor={l.anchor} class="rb-chart-label">{shortDate(l.t, spanDays > 330)}</text>
      {/each}
      <g clip-path="url(#{clip})">
        {#if kind === 'bars'}
          {#each drawn as p, i (i)}
            {@const top = y(value(p))}
            <rect
              x={x(p[0]) - barWidth / 2}
              y={Math.min(top, height - B - 1)}
              width={barWidth}
              height={Math.max(1, height - B - top)}
              rx={Math.min(2, barWidth / 3)}
              class="rb-chart-bar"
              class:active={hover === p}
            />
          {/each}
        {:else}
          <path d={area} class="rb-chart-area" />
          <path d={line} class="rb-chart-line" />
          {#if pts.length < 60}
            {#each drawn as p, i (i)}
              <circle cx={x(p[0])} cy={y(value(p))} r="2.6" class="rb-chart-dot" />
            {/each}
          {/if}
        {/if}
      </g>
      {#if hover}
        <line x1={x(hover[0])} x2={x(hover[0])} y1={T} y2={height - B} class="rb-chart-guide" />
        {#if kind === 'line'}
          <circle cx={x(hover[0])} cy={y(value(hover))} r="4" class="rb-chart-marker" />
        {/if}
      {/if}
    </svg>
    {#if hover}
      <div class="rb-chart-tip" style:display="block" style:left="{tipLeft}px" style:top="{tipTop}px">
        <div>
          <b>{bytes(value(hover))}</b><span>&nbsp;&nbsp;{dateTime(hover[0])}</span>
          {#if detail}<div class="rb-hint">{detail(hover)}</div>{/if}
          {#if change !== null}
            <div class="rb-hint" class:up={change > 0}>{signed(change)} since the previous measurement</div>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .rb-chart-svg {
    touch-action: pan-y;
  }
  .rb-chart-bar {
    transition: opacity 0.15s;
  }
  :global(#rb-app) .rb-chart-bar.active {
    opacity: 1;
    fill: var(--rb-accent);
  }
  :global(#rb-app) .up {
    color: var(--rb-accent-text);
  }
</style>
