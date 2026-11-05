import { api } from '../lib/api';
import { app, fail, jobConfig, pref } from '../lib/state.svelte';
import type { Entry, Snapshot } from '../lib/types';

export const NO_JOB = ':none';

export type SortKey = '' | 'name' | 'size' | 'mtime';

export interface Item {
  path: string;
  type: string;
  name: string;
}

export interface FindResult {
  snap: Snapshot;
  path: string;
  focus: string;
}

export const S = $state({
  repo: null as string | null,
  job: '',
  list: null as Snapshot[] | null,
  names: {} as Record<string, string>,
  loading: false,
  error: null as string | null,
  snap: null as Snapshot | null,
  path: '/',
  entries: null as Entry[] | null,
  listing: false,
  opening: false,
  checked: {} as Record<string, Entry>,
  filter: '',
  cursor: -1,
  focus: '',
  sort: { key: '' as SortKey, desc: false },
  scroll: { list: 0, browser: 0 }
});

export function jobLabel(id: string): string {
  if (!id) return 'Not from a job';
  return S.names[id] || jobConfig(id)?.name || 'Job ' + id;
}

export function jobName(s: Snapshot): string {
  return s.job ? S.names[s.job] || 'Job ' + s.job : s.tags.length ? s.tags.join(', ') : 'Not from a job';
}

export function hostSuffix(s: Snapshot): string {
  return s.hostname !== app.overview?.system.server ? ' · ' + s.hostname : '';
}

export function matchesJob(s: Snapshot, job: string): boolean {
  if (!job) return true;
  if (job === NO_JOB) return !s.job;
  return s.job === job;
}

export function selectRepo(id: string, job: string): void {
  if (S.repo !== id) {
    S.repo = id;
    S.list = null;
    S.snap = null;
    S.entries = null;
    S.checked = {};
    S.error = null;
    S.filter = '';
    S.scroll = { list: 0, browser: 0 };
  }
  S.job = job || '';
  pref('snapRepo', id);
}

export async function loadList(refresh: boolean): Promise<void> {
  S.loading = true;
  S.error = null;
  const repo = S.repo;
  try {
    const r = await api<{ snapshots: Snapshot[]; job_names: Record<string, string> }>('snapshots', {
      repo,
      refresh: refresh ? 1 : 0
    });
    if (repo !== S.repo) return;
    S.list = r.snapshots;
    S.names = r.job_names || {};
    S.loading = false;
    if (S.snap && !S.list.some((s) => s.id === S.snap?.id)) {
      S.snap = null;
      S.entries = null;
      S.checked = {};
    } else if (S.snap) {
      S.snap = S.list.find((s) => s.id === S.snap?.id) ?? S.snap;
    }
  } catch (e) {
    if (repo !== S.repo) return;
    S.loading = false;
    S.error = (e as Error).message;
  }
}

export function commonParent(paths: string[]): string {
  if (!paths.length) return '/';
  if (paths.length === 1) return paths[0];
  const parts = paths.map((p) => p.split('/'));
  const out: string[] = [];
  for (let i = 0; i < parts[0].length; i++) {
    const seg = parts[0][i];
    if (parts.every((p) => p[i] === seg)) out.push(seg);
    else break;
  }
  return out.join('/') || '/';
}

export function parentOf(path: string): string {
  return path.replace(/\/[^/]+$/, '') || '/';
}

export function openSnapshot(s: Snapshot, path?: string, focus = ''): void {
  S.snap = s;
  S.checked = {};
  navigate(path || commonParent(s.paths), focus);
}

export function navigate(path: string, focus = ''): void {
  S.path = path;
  S.listing = true;
  S.opening = false;
  S.checked = {};
  S.filter = '';
  S.cursor = -1;
  S.focus = focus;
  S.scroll.browser = 0;
  const snap = S.snap;
  const repo = S.repo;
  if (!snap) return;
  const stale = () => S.snap?.id !== snap.id || S.path !== path || S.repo !== repo;
  const ask = (): void => {
    api<{ pending?: boolean; entries: Entry[] }>('ls', { repo, snapshot: snap.id, path })
      .then((r) => {
        if (stale()) return;
        if (r.pending) {
          S.opening = true;
          setTimeout(ask, 600);
          return;
        }
        S.entries = r.entries || [];
        S.listing = false;
        S.opening = false;
      })
      .catch((e) => {
        if (stale()) return;
        S.listing = false;
        S.opening = false;
        S.entries = [];
        fail(e);
      });
  };
  ask();
}

export function goUp(): void {
  if (S.path === '/') return;
  const from = S.path;
  navigate(parentOf(S.path), from);
}

export function checkedItems(): Entry[] {
  return Object.values(S.checked);
}
