import { api } from '../lib/api';
import { confirm } from '../lib/dialogs';
import { app, fail, refresh, toast } from '../lib/state.svelte';
import type { Job, JobInfo, Op, Repo } from '../lib/types';

export const STATUS: Record<string, [string, string]> = {
  success: ['ok', 'Succeeded'],
  warning: ['warn', 'Warnings'],
  error: ['err', 'Failed'],
  cancelled: ['idle', 'Stopped']
};

export function statusOf(status: string): [string, string] {
  return STATUS[status] || (status === 'running' ? ['run', 'Running'] : ['idle', status]);
}

export function jobStatus(info: Partial<JobInfo>, cfg: Job): [string, string] {
  if (info.running) return ['run', 'Running'];
  if (!cfg.enabled) return ['idle', 'Disabled'];
  if (info.queued) return ['run', 'Waiting'];
  if (!info.last) return ['idle', 'Not run yet'];
  return STATUS[info.last.status] || ['idle', info.last.status];
}

export function repoIcon(type: Repo['type'] | string): string {
  return ({ sftp: 'server', local: 'hdd', rest: 'cloud' } as Record<string, string>)[type] || 'repo';
}

export const REPO_TYPE: Record<string, string> = { sftp: 'SFTP', local: 'local', rest: 'REST' };

export function lines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
}

export function blankJob(): Job {
  const repos = app.overview?.config.repos ?? [];
  return {
    id: '',
    name: '',
    enabled: true,
    repo: repos.length === 1 ? repos[0].id : '',
    sources: [],
    excludes: [],
    iexcludes: [],
    exclude_caches: true,
    exclude_larger_than: '',
    one_file_system: false,
    tags: [],
    schedule: { mode: 'cron', cron: '0 3 * * *', every: 24, unit: 'hours', catch_up: true },
    retention: { enabled: true, last: 0, hourly: 0, daily: 7, weekly: 4, monthly: 12, yearly: 0, within: '' },
    consistency: { mode: 'none', containers: [], vms: [], skip: [], timeout: 120 },
    hooks: { before: '', after: '' },
    notify: { success: false, warning: true, error: true, healthcheck: '' },
    advanced: { host: '', read_concurrency: 0, skip_if_unchanged: false, extra_flags: [] }
  };
}

export function normalizeJob(job: Job): Job {
  const b = blankJob();
  job.sources ??= [];
  job.excludes ??= [];
  job.iexcludes ??= [];
  job.tags ??= [];
  job.schedule = { ...b.schedule, ...(job.schedule ?? {}) };
  job.retention = { ...b.retention, ...(job.retention ?? {}) };
  job.consistency = { ...b.consistency, ...(job.consistency ?? {}) };
  job.consistency.containers ??= [];
  job.consistency.vms ??= [];
  job.consistency.skip ??= [];
  job.hooks = { ...b.hooks, ...(job.hooks ?? {}) };
  job.notify = { ...b.notify, ...(job.notify ?? {}) };
  job.advanced = { ...b.advanced, ...(job.advanced ?? {}) };
  job.advanced.extra_flags ??= [];
  return job;
}

function num(v: unknown): number {
  const n = Number(v);
  return v === null || v === '' || isNaN(n) ? 0 : n;
}

export function forSave(job: Job): Job {
  const j = JSON.parse(JSON.stringify(job)) as Job;
  for (const k of ['last', 'hourly', 'daily', 'weekly', 'monthly', 'yearly'] as const) j.retention[k] = num(j.retention[k]);
  j.consistency.timeout = num(j.consistency.timeout);
  j.advanced.read_concurrency = num(j.advanced.read_concurrency);
  return j;
}

export async function runJob(cfg: Job): Promise<void> {
  try {
    const r = await api<{ queued?: boolean }>('job_run', { id: cfg.id });
    toast(r.queued ? `"${cfg.name}" waits for its repository to be free.` : `Backup "${cfg.name}" started.`);
    setTimeout(refresh, 700);
  } catch (e) {
    fail(e);
  }
}

export async function stopOp(op: Op): Promise<void> {
  const yes = await confirm({
    title: 'Stop this operation?',
    text:
      op.kind === 'backup'
        ? 'The backup ends without a snapshot. Containers or VMs it stopped are started again.'
        : 'It ends as soon as restic has cleaned up.',
    confirm: 'Stop',
    danger: true
  });
  if (!yes) return;
  try {
    await api('op_stop', { id: op.id });
    toast('Stopping...', 'info');
    setTimeout(refresh, 1500);
  } catch (e) {
    fail(e);
  }
}

export async function deleteJob(cfg: Job): Promise<boolean> {
  const yes = await confirm({
    title: `Delete "${cfg.name}"?`,
    text:
      'The job and its schedule are removed. Its snapshots stay in the repository - ' +
      'they can still be browsed and restored, and removed there by hand.',
    confirm: 'Delete job',
    danger: true
  });
  if (!yes) return false;
  try {
    await api('job_delete', { id: cfg.id });
    toast('Job deleted.');
    refresh();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

export async function setEnabled(cfg: Job, enabled: boolean): Promise<void> {
  const copy = JSON.parse(JSON.stringify(cfg)) as Job;
  copy.enabled = enabled;
  try {
    await api('job_save', { job: copy });
    await refresh();
  } catch (e) {
    fail(e);
  }
}

export async function applyRetention(cfg: Job): Promise<void> {
  try {
    await api('job_forget', { id: cfg.id });
    toast(`Removing old snapshots of "${cfg.name}"...`);
    setTimeout(refresh, 700);
  } catch (e) {
    fail(e);
  }
}
