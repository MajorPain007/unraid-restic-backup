import { api } from '../lib/api';
import { confirm } from '../lib/dialogs';
import { busy, fail, openDialog, openMenu, refresh, toast, copyText } from '../lib/state.svelte';
import { dateTime } from '../lib/format';
import type { Op, Snapshot } from '../lib/types';
import CompareDialog from './CompareDialog.svelte';
import FindDialog from './FindDialog.svelte';
import RestoreDialog from './RestoreDialog.svelte';
import { S, jobName, loadList, openSnapshot, type FindResult, type Item } from './store.svelte';

export async function restoreItems(items: Item[]): Promise<void> {
  const snap = S.snap;
  const repo = S.repo;
  if (!snap || !repo) return;
  const started = await openDialog<boolean>(RestoreDialog, { repo, snap, items });
  if (!started) return;
  S.checked = {};
  toast('Restore started - follow it at the top of this page.');
  setTimeout(refresh, 800);
}

export async function deleteSnapshot(s: Snapshot): Promise<void> {
  const yes = await confirm({
    title: 'Delete this snapshot?',
    text:
      'The snapshot of ' +
      dateTime(s.time) +
      ' (' +
      jobName(s) +
      ') is removed for good. ' +
      'Its data is freed by the next prune, unless other snapshots share it.',
    confirm: 'Delete snapshot',
    danger: true
  });
  if (!yes) return;
  try {
    await api('snapshot_forget', { repo: S.repo, ids: [s.id] });
    toast('Snapshot deleted.');
    if (S.snap?.id === s.id) {
      S.snap = null;
      S.entries = null;
      S.checked = {};
    }
    loadList(true);
  } catch (e) {
    fail(e);
  }
}

export function snapMenu(s: Snapshot, anchor: Element): void {
  openMenu(anchor, [
    { label: 'Copy snapshot ID', icon: 'copy', onclick: () => copyText(s.id) },
    {
      label: 'Restore everything...',
      icon: 'restore',
      onclick: () => restoreItems(s.paths.map((p) => ({ path: p, type: 'dir', name: p })))
    },
    '-',
    { label: 'Delete snapshot', icon: 'trash', danger: true, onclick: () => deleteSnapshot(s) }
  ]);
}

function reveal(r: FindResult | undefined): void {
  if (r) openSnapshot(r.snap, r.path, r.focus);
}

export async function find(pattern: string): Promise<void> {
  if (!S.repo) return;
  reveal(await openDialog<FindResult | undefined>(FindDialog, { repo: S.repo, pattern, snapshots: S.list || [] }));
}

export async function compare(s: Snapshot): Promise<void> {
  const list = (S.list || []).filter((x) => x.id !== s.id);
  if (!list.length) {
    toast('There is no other snapshot to compare with.', 'info');
    return;
  }
  reveal(await openDialog<FindResult | undefined>(CompareDialog, { repo: S.repo, snap: s, list }));
}

export async function stopOp(op: Op, btn: { busy: boolean }): Promise<void> {
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
    await busy(btn, api('op_stop', { id: op.id }));
    toast('Stopping...', 'info');
    setTimeout(refresh, 1500);
  } catch (e) {
    fail(e);
  }
}
