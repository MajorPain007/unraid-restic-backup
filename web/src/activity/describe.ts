import type { Op } from '../lib/types';
import { bytes, plural } from '../lib/format';

export const KIND: Record<string, string> = {
  backup: 'Backup',
  forget: 'Retention',
  prune: 'Prune',
  check: 'Check',
  restore: 'Restore'
};

export const STATUS: Record<string, [string, string]> = {
  success: ['ok', 'Succeeded'],
  warning: ['warn', 'Warnings'],
  error: ['err', 'Failed'],
  cancelled: ['idle', 'Stopped']
};

export function statusOf(status: string): [string, string] {
  return STATUS[status] || (status === 'running' ? ['run', 'Running'] : ['idle', status]);
}

export function what(r: Partial<Op>): string {
  const kind = r.kind ?? '';
  if (kind === 'backup' || kind === 'forget') return (KIND[kind] || kind) + ' · ' + (r.job_name || r.job);
  return (KIND[kind] || kind) + ' · ' + (r.repo_name || r.repo);
}

function n(v: unknown): string {
  return Number(v || 0).toLocaleString('en-GB');
}

export function result(r: Op): string {
  const s = r.summary || {};
  if (r.status === 'error' || r.status === 'cancelled') return r.error || '';
  if (r.kind === 'backup' && s.data_added_packed !== undefined) {
    let t = bytes(s.data_added_packed) + ' added, ' + n(s.files_new) + ' new and ' + n(s.files_changed) + ' changed files';
    if (s.forget_removed) t += '; ' + plural(s.forget_removed, 'old snapshot') + ' removed';
    return r.error ? t + ' - ' + r.error : t;
  }
  if (r.kind === 'restore' && s.files_restored !== undefined) {
    return plural(s.files_restored, 'file') + ' (' + bytes(s.bytes_restored) + ') restored to ' + (s.target || '');
  }
  if (r.kind === 'forget' && s.forget_removed !== undefined) {
    return plural(s.forget_removed, 'snapshot') + ' removed, ' + s.forget_kept + ' kept';
  }
  if (r.kind === 'prune' && s.total_size !== undefined) return 'Repository now ' + bytes(s.total_size);
  if (r.kind === 'check' && s.num_errors !== undefined) {
    return s.num_errors ? plural(s.num_errors, 'problem') + ' found' : 'No problems found';
  }
  return r.error || '';
}
