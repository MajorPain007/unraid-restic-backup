<script lang="ts" module>
  import type { Schedule } from '../types';

  type Preset = 'manual' | 'hourly' | 'daily' | 'weekly' | 'monthly' | 'interval' | 'custom';

  const DAYS: [string, string][] = [
    ['1', 'Monday'],
    ['2', 'Tuesday'],
    ['3', 'Wednesday'],
    ['4', 'Thursday'],
    ['5', 'Friday'],
    ['6', 'Saturday'],
    ['0', 'Sunday']
  ];

  function isNum(s: string): boolean {
    return /^\d+$/.test(s);
  }

  function pad(n: string | number): string {
    return ('0' + n).slice(-2);
  }

  function presetOf(s: Schedule): { preset: Preset; minute?: number; time?: string; day?: string; dom?: number } {
    if (s.mode === 'off') return { preset: 'manual' };
    if (s.mode === 'interval') return { preset: 'interval' };
    const f = String(s.cron || '').trim().split(/\s+/);
    if (f.length === 5 && isNum(f[0]) && f[1] === '*' && f[2] === '*' && f[3] === '*' && f[4] === '*') {
      return { preset: 'hourly', minute: +f[0] };
    }
    if (f.length === 5 && isNum(f[0]) && isNum(f[1]) && f[3] === '*') {
      const time = pad(f[1]) + ':' + pad(f[0]);
      if (f[2] === '*' && f[4] === '*') return { preset: 'daily', time };
      if (f[2] === '*' && /^[0-7]$/.test(f[4])) return { preset: 'weekly', time, day: f[4] === '7' ? '0' : f[4] };
      if (isNum(f[2]) && +f[2] <= 28 && f[4] === '*') return { preset: 'monthly', time, dom: +f[2] };
    }
    return { preset: 'custom' };
  }
</script>

<script lang="ts">
  import Segmented from './Segmented.svelte';
  import Toggle from './Toggle.svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api';
  import { dateTime } from '../format';

  let {
    schedule = $bindable(),
    allowManual = true,
    manualLabel = 'Manual',
    manualText = 'Runs only when started by hand.',
    noCatchUp = false,
    onchange
  }: {
    schedule: Schedule;
    allowManual?: boolean;
    manualLabel?: string;
    manualText?: string;
    noCatchUp?: boolean;
    onchange?: (s: Schedule) => void;
  } = $props();

  const p = presetOf(schedule);
  let st = $state({
    preset: p.preset,
    time: p.time || '03:00',
    minute: (p.minute ?? 0) as number | null,
    day: p.day || '0',
    dom: (p.dom || 1) as number | null,
    every: (schedule.every || 6) as number | null,
    unit: schedule.unit || 'hours',
    cron: schedule.cron || '0 3 * * *'
  });

  let presets = $derived<[Preset, string][]>([
    ...(allowManual ? [['manual', manualLabel] as [Preset, string]] : []),
    ['hourly', 'Hourly'],
    ['daily', 'Daily'],
    ['weekly', 'Weekly'],
    ['monthly', 'Monthly'],
    ['interval', 'Interval'],
    ['custom', 'Custom']
  ]);

  let preview = $state<{ valid: boolean; error?: string; next: number[] } | null>(null);

  function apply() {
    const t = (st.time || '03:00').split(':');
    const hh = +t[0] || 0;
    const mm = +t[1] || 0;
    const minute = Math.max(0, Math.min(59, Number(st.minute) || 0));
    const dom = Math.max(1, Math.min(28, Number(st.dom) || 1));
    switch (st.preset) {
      case 'manual':
        schedule.mode = 'off';
        break;
      case 'hourly':
        schedule.mode = 'cron';
        schedule.cron = minute + ' * * * *';
        break;
      case 'daily':
        schedule.mode = 'cron';
        schedule.cron = mm + ' ' + hh + ' * * *';
        break;
      case 'weekly':
        schedule.mode = 'cron';
        schedule.cron = mm + ' ' + hh + ' * * ' + st.day;
        break;
      case 'monthly':
        schedule.mode = 'cron';
        schedule.cron = mm + ' ' + hh + ' ' + dom + ' * *';
        break;
      case 'interval':
        schedule.mode = 'interval';
        schedule.every = Math.max(1, Number(st.every) || 1);
        schedule.unit = st.unit;
        break;
      default:
        schedule.mode = 'cron';
        schedule.cron = st.cron;
    }
    onchange?.(schedule);
  }

  let key = $derived(JSON.stringify([schedule.mode, schedule.cron, schedule.every, schedule.unit]));

  $effect(() => {
    const k = key;
    if (schedule.mode === 'off') {
      preview = null;
      return;
    }
    let alive = true;
    const timer = setTimeout(() => {
      api<{ valid: boolean; error?: string; next: number[] }>('schedule_preview', { schedule: JSON.parse(JSON.stringify(schedule)) })
        .then((r) => {
          if (alive && k === key) preview = r;
        })
        .catch(() => {});
    }, 250);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  });

  function chipText(ts: number): string {
    return new Date(ts * 1000).toLocaleString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
  }
</script>

<div class="rb-schedule">
  <Segmented bind:value={st.preset} options={presets} onchange={apply} />
  <div class="rb-inline rb-schedule-detail">
    {#if st.preset === 'hourly'}
      At minute <input type="number" min="0" max="59" bind:value={st.minute} oninput={apply} aria-label="Minute" /> of every hour
    {:else if st.preset === 'daily'}
      Every day at <input type="time" bind:value={st.time} onchange={apply} aria-label="Time" />
    {:else if st.preset === 'weekly'}
      Every
      <select bind:value={st.day} onchange={apply} aria-label="Weekday">
        {#each DAYS as [v, label] (v)}<option value={v}>{label}</option>{/each}
      </select>
      at <input type="time" bind:value={st.time} onchange={apply} aria-label="Time" />
    {:else if st.preset === 'monthly'}
      On day <input type="number" min="1" max="28" bind:value={st.dom} oninput={apply} aria-label="Day of the month" /> of every month at
      <input type="time" bind:value={st.time} onchange={apply} aria-label="Time" />
    {:else if st.preset === 'interval'}
      Every <input type="number" min="1" bind:value={st.every} oninput={apply} aria-label="Interval" />
      <select bind:value={st.unit} onchange={apply} aria-label="Unit">
        <option value="hours">hours</option>
        <option value="days">days</option>
      </select>
      <span class="rb-hint">counted from the start of the last run</span>
    {:else if st.preset === 'custom'}
      <input
        type="text"
        class="rb-mono rb-schedule-cron"
        placeholder="0 3 * * *"
        spellcheck="false"
        autocomplete="off"
        bind:value={st.cron}
        oninput={apply}
        aria-label="Cron expression"
      />
      <span class="rb-hint">minute hour day-of-month month weekday</span>
    {/if}
  </div>
  <div class="rb-next-runs" aria-live="polite">
    {#if schedule.mode === 'off'}
      <span class="rb-hint">{manualText}</span>
    {:else if preview && !preview.valid}
      <span class="rb-error">{preview.error}</span>
    {:else if preview}
      <span class="rb-hint">Next: </span>
      {#each preview.next as ts (ts)}
        <span class="rb-chip" title={dateTime(ts)}><Icon name="clock" />{chipText(ts)}</span>
      {/each}
    {/if}
  </div>
  {#if !noCatchUp && schedule.mode === 'cron'}
    <div class="rb-schedule-catch">
      <Toggle bind:checked={schedule.catch_up} label="Run missed backups when the server is back on" />
      <div class="rb-hint">If the server was off at the scheduled time, the backup runs once when it is back.</div>
    </div>
  {/if}
</div>

<style>
  .rb-schedule-detail {
    margin-top: 12px;
  }
  .rb-schedule-catch {
    margin-top: 12px;
  }
  .rb-schedule-catch .rb-hint {
    margin-left: 44px;
  }
  :global(#rb-app) .rb-schedule-cron {
    width: 220px;
    max-width: 100%;
  }
</style>
