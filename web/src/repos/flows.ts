import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { confirm } from '../lib/dialogs';
import { fail, openDialog, refresh, repoConfig, toast } from '../lib/state.svelte';
import type { Repo } from '../lib/types';
import ConnectFailDialog from './ConnectFailDialog.svelte';
import HostKeyDialog from './HostKeyDialog.svelte';
import KeysDialog from './KeysDialog.svelte';
import PasswordDialog from './PasswordDialog.svelte';
import RemoveRepoDialog from './RemoveRepoDialog.svelte';
import RepoEditor from './RepoEditor.svelte';
import SshKeyDialog from './SshKeyDialog.svelte';
import { repoUrl } from './util';

export interface HostKey {
  type: string;
  bits?: number;
  fingerprint: string;
}

export interface HostKeyInfo {
  status: string;
  scanned: HostKey[];
  stored: HostKey[];
}

export interface ResticKey {
  id: string;
  userName?: string;
  hostName?: string;
  created: string;
  current?: boolean;
}

export interface EditorResult {
  repo: Repo;
  generated: string;
  isNew: boolean;
}

function current(repo: Repo): Repo {
  return repoConfig(repo.id) ?? repo;
}

export async function editRepo(id: string | null): Promise<void> {
  const saved = id ? repoConfig(id) : null;
  if (id && !saved) return;
  const r = await openDialog<EditorResult | undefined>(RepoEditor, { saved });
  if (!r) return;
  await refresh();
  if (r.isNew) afterCreate(r.repo, r.generated);
  else toast('Saved.');
}

export async function afterCreate(repo: Repo, generated: string): Promise<void> {
  if (generated) await passwordDialog(repo, generated, true);
  if (repo.type === 'sftp') {
    const go = await sshKeyDialog(repo, true);
    if (go) connect(repo);
    else toast('Use "Test connection" in the repository\'s menu once the key is on the server.', 'info');
    return;
  }
  await connect(repo);
}

export function passwordDialog(repo: Repo, password: string, firstTime: boolean): Promise<unknown> {
  return openDialog(PasswordDialog, { repo, password, firstTime });
}

export async function showPassword(repo: Repo): Promise<void> {
  try {
    const r = await api<{ password: string }>('repo_password', { id: repo.id });
    await passwordDialog(repo, r.password, false);
  } catch (e) {
    fail(e);
  }
}

export function sshKeyDialog(repo: Repo, setup: boolean): Promise<boolean> {
  return openDialog<boolean | undefined>(SshKeyDialog, { repo: current(repo), setup }).then((go) => go === true);
}

export async function connect(repo: Repo): Promise<void> {
  toast('Connecting to "' + repo.name + '"...', 'info');
  let r: { status: string; hostkey?: HostKeyInfo };
  try {
    r = await api<{ status: string; hostkey?: HostKeyInfo }>('repo_test', { id: repo.id });
  } catch (e) {
    await connectFailed(repo, e);
    return;
  }
  if (r.status === 'hostkey' && r.hostkey) {
    const ok = await openDialog<boolean | undefined>(HostKeyDialog, { repo: current(repo), hostkey: r.hostkey });
    if (ok) await connect(repo);
    return;
  }
  if (r.status === 'empty') {
    const yes = await confirm({
      title: 'Nothing there yet',
      text: 'There is no restic repository at ' + repoUrl(repo) + '. Create one there now?',
      confirm: 'Initialize repository'
    });
    if (!yes) return;
    try {
      await api('repo_init', { id: repo.id });
      toast('Repository "' + repo.name + '" is ready.');
      refresh();
    } catch (e) {
      await connectFailed(repo, e);
    }
    return;
  }
  toast('Connected: "' + repo.name + '" opens with its password.');
  api('repo_stats', { id: repo.id })
    .then(() => refresh())
    .catch(() => {});
}

async function connectFailed(repo: Repo, e: unknown): Promise<void> {
  const choice = await openDialog<'key' | 'edit' | undefined>(ConnectFailDialog, {
    repo,
    message: (e as Error).message
  });
  if (choice === 'key') {
    if (await sshKeyDialog(repo, true)) connect(repo);
  } else if (choice === 'edit') {
    actions.editRepo(repo.id);
  }
}

export async function runMaintenance(repo: Repo, kind: 'check' | 'prune'): Promise<void> {
  try {
    await api('repo_run', { id: repo.id, kind });
    toast((kind === 'check' ? 'Check' : 'Prune') + ' of "' + repo.name + '" started.');
    setTimeout(refresh, 700);
  } catch (e) {
    fail(e);
  }
}

export async function measure(repo: Repo): Promise<void> {
  toast('Measuring "' + repo.name + '"...', 'info');
  try {
    await api('repo_stats', { id: repo.id });
    toast('"' + repo.name + '" measured.');
    refresh();
  } catch (e) {
    fail(e);
  }
}

export async function unlock(repo: Repo): Promise<void> {
  const yes = await confirm({
    title: 'Remove stale locks?',
    text:
      'Removes locks left behind by restic processes that no longer run - after a crash or a power cut. ' +
      'Locks of running operations, on this or another computer, stay.',
    confirm: 'Remove stale locks'
  });
  if (!yes) return;
  try {
    await api('repo_unlock', { id: repo.id });
    toast('Stale locks removed.');
  } catch (e) {
    fail(e);
  }
}

export async function deleteRepo(repo: Repo): Promise<boolean> {
  const yes = await openDialog<boolean | undefined>(RemoveRepoDialog, { repo });
  if (!yes) return false;
  try {
    await api('repo_delete', { id: repo.id });
    toast('Repository removed from the plugin.');
    refresh();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

export async function keysDialog(repo: Repo): Promise<void> {
  try {
    const r = await api<{ keys: ResticKey[] }>('repo_keys', { id: repo.id });
    await openDialog(KeysDialog, { repo, keys: Array.isArray(r.keys) ? r.keys : [] });
  } catch (e) {
    fail(e);
  }
}
