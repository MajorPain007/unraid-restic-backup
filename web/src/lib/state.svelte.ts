import type { Component } from 'svelte';
import { api, ApiError } from './api';
import type { IconName } from './icons';
import type { Job, JobInfo, Op, Overview, Repo, RepoInfo } from './types';

export type Tab = 'jobs' | 'repos' | 'snapshots' | 'activity' | 'settings';

export function pref(key: string): string | null;
export function pref(key: string, value: string): string;
export function pref(key: string, value?: string): string | null {
  try {
    if (value === undefined) return localStorage.getItem('rb.' + key);
    localStorage.setItem('rb.' + key, value);
  } catch {
    return value ?? null;
  }
  return value;
}

const TABS: Tab[] = ['jobs', 'repos', 'snapshots', 'activity', 'settings'];

function initialTab(): Tab {
  const t = pref('tab') as Tab | null;
  return t && TABS.includes(t) ? t : 'jobs';
}

export const app = $state({
  overview: null as Overview | null,
  loadError: '' as string,
  tab: initialTab(),
  viewOpts: {} as Record<string, any>,
  viewKey: 0
});

export function show(tab: Tab, opts: Record<string, any> = {}): void {
  app.tab = TABS.includes(tab) ? tab : 'jobs';
  app.viewOpts = opts;
  app.viewKey++;
  pref('tab', app.tab);
}

let pollTimer: ReturnType<typeof setTimeout> | null = null;
let lastRunning = '';
const progressListeners = new Set<(ops: Op[]) => void>();

export function onProgress(fn: (ops: Op[]) => void): () => void {
  progressListeners.add(fn);
  return () => progressListeners.delete(fn);
}

export async function refresh(): Promise<Overview | null> {
  try {
    const data = await api<Overview>('overview');
    app.overview = data;
    app.loadError = '';
    lastRunning = runningKey(data.running);
    schedulePoll();
    return data;
  } catch (e) {
    if (!app.overview) app.loadError = (e as Error).message;
    else fail(e);
    schedulePoll();
    return null;
  }
}

function runningKey(ops: Op[]): string {
  return ops
    .map((o) => o.id)
    .sort()
    .join(',');
}

function schedulePoll(): void {
  if (pollTimer) clearTimeout(pollTimer);
  const running = app.overview?.running ?? [];
  pollTimer = setTimeout(running.length ? pollRunning : refresh, running.length ? 2000 : 20000);
}

async function pollRunning(): Promise<void> {
  try {
    const r = await api<{ running: Op[] }>('ops');
    const key = runningKey(r.running);
    if (key !== lastRunning) {
      lastRunning = key;
      await refresh();
      return;
    }
    if (app.overview) app.overview.running = r.running;
    progressListeners.forEach((fn) => fn(r.running));
  } catch {
    schedulePoll();
    return;
  }
  schedulePoll();
}

export function jobConfig(id: string): Job | null {
  return app.overview?.config.jobs.find((j) => j.id === id) ?? null;
}

export function repoConfig(id: string): Repo | null {
  return app.overview?.config.repos.find((r) => r.id === id) ?? null;
}

export function jobInfo(id: string): JobInfo | null {
  return app.overview?.jobs.find((j) => j.id === id) ?? null;
}

export function repoInfo(id: string): RepoInfo | null {
  return app.overview?.repos.find((r) => r.id === id) ?? null;
}

export function runningOp(match: (op: Op) => boolean): Op | null {
  return app.overview?.running.find(match) ?? null;
}

export type ToastKind = 'ok' | 'err' | 'info';

export const toasts = $state<{ id: number; message: string; kind: ToastKind }[]>([]);
let toastSeq = 0;

export function toast(message: string, kind: ToastKind = 'ok'): void {
  const id = ++toastSeq;
  toasts.push({ id, message, kind });
  setTimeout(
    () => {
      const i = toasts.findIndex((t) => t.id === id);
      if (i >= 0) toasts.splice(i, 1);
    },
    kind === 'err' ? 9000 : 4500
  );
}

export function fail(err: unknown): void {
  toast(err instanceof Error ? err.message : String(err), 'err');
}

export function errorsOf(err: unknown): string[] {
  return err instanceof ApiError ? err.errors : [];
}

export interface DialogEntry {
  id: number;
  component: Component<any>;
  props: Record<string, any>;
  resolve: (value: any) => void;
}

export const dialogs = $state<DialogEntry[]>([]);
let dialogSeq = 0;

export function openDialog<T = unknown>(component: Component<any>, props: Record<string, any> = {}): Promise<T> {
  closeMenu();
  return new Promise<T>((resolve) => {
    dialogs.push({ id: ++dialogSeq, component, props, resolve });
  });
}

export function closeDialog(id: number, result?: unknown): void {
  const i = dialogs.findIndex((d) => d.id === id);
  if (i < 0) return;
  const [d] = dialogs.splice(i, 1);
  d.resolve(result);
}

export function closeTopDialog(): void {
  const top = dialogs[dialogs.length - 1];
  if (top) closeDialog(top.id, undefined);
}

export interface MenuItem {
  label: string;
  icon?: IconName;
  danger?: boolean;
  disabled?: boolean;
  onclick: () => void;
}

export const menu = $state({
  open: false,
  x: 0,
  y: 0,
  anchor: null as DOMRect | null,
  items: [] as (MenuItem | '-')[]
});

export function openMenu(anchor: Element, items: (MenuItem | '-' | null | false | undefined)[]): void {
  menu.anchor = anchor.getBoundingClientRect();
  menu.items = items.filter((i): i is MenuItem | '-' => !!i);
  menu.open = true;
}

export function closeMenu(): void {
  menu.open = false;
}

export async function busy<T>(target: { busy: boolean }, promise: Promise<T>): Promise<T> {
  target.busy = true;
  try {
    return await promise;
  } finally {
    target.busy = false;
  }
}

export function copyText(text: string): void {
  const done = () => toast('Copied to the clipboard');
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(done, () => legacyCopy(text) && done());
  } else if (legacyCopy(text)) {
    done();
  } else {
    toast('Copying is blocked here - select the text and copy it by hand.', 'err');
  }
}

function legacyCopy(text: string): boolean {
  const ta = document.createElement('textarea');
  ta.style.position = 'fixed';
  ta.style.top = '-1000px';
  ta.value = text;
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand('copy');
  } catch {
    ok = false;
  }
  ta.remove();
  return ok;
}
