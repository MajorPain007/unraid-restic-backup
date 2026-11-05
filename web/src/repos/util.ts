import { app, repoInfo } from '../lib/state.svelte';
import type { Repo } from '../lib/types';

export const TYPE_LABEL: Record<Repo['type'], string> = { sftp: 'SFTP', local: 'Local folder', rest: 'REST server' };

export type ServerKind = 'ssh' | 'storagebox';

export const SERVER_KINDS: Record<ServerKind, { label: string; port: number }> = {
  ssh: { label: 'SSH server: Unraid, NAS, Linux, VPS', port: 22 },
  storagebox: { label: 'Hetzner Storage Box', port: 23 }
};

export const KIND_ORDER: ServerKind[] = ['ssh', 'storagebox'];

export function isStorageBoxHost(host: string): boolean {
  return /\.your-storagebox\.de$/i.test(host.trim());
}

export function serverKind(repo: Repo): ServerKind {
  return isStorageBoxHost(repo.sftp.host || '') || +repo.sftp.port === 23 ? 'storagebox' : 'ssh';
}

export function repoIcon(type: string): string {
  return ({ sftp: 'server', local: 'hdd', rest: 'cloud' } as Record<string, string>)[type] || 'repo';
}

export function keyPath(repo: Repo): string {
  return '/boot/config/plugins/restic.backup/ssh/' + repo.id + '.pub';
}

export function serverName(): string {
  return app.overview?.system.server ?? '';
}

export function repoUrl(repo: Repo): string {
  return repoInfo(repo.id)?.url ?? '';
}

export function opLabel(kind: string): string {
  return (
    ({
      prune: 'Removing unused data',
      check: 'Checking the repository',
      restore: 'Restoring',
      forget: 'Applying retention'
    } as Record<string, string>)[kind] || kind
  );
}

export function recoveryText(repo: Repo, password: string): string {
  const url = repoUrl(repo);
  return [
    'restic repository: ' + repo.name,
    'Location: ' + url,
    'Password: ' + password,
    '',
    'Keep this file somewhere safe and away from the server: in a password manager,',
    'or printed. Without the password the backups cannot be read - by anyone.',
    '',
    'To get at the backups without the plugin, on any computer with restic:',
    '  restic -r ' +
      (repo.type === 'sftp'
        ? 'sftp:' + repo.sftp.user + '@' + repo.sftp.host + ':' + repo.sftp.path + ' -o sftp.args="-p ' + repo.sftp.port + '"'
        : url) +
      ' snapshots',
    ''
  ].join('\n');
}

export function saveText(name: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    URL.revokeObjectURL(url);
    a.remove();
  }, 1000);
}

export function recoveryFileName(repo: Repo): string {
  return 'restic-' + repo.name.replace(/[^A-Za-z0-9._-]+/g, '-') + '-recovery.txt';
}

export function blankRepo(): Repo {
  return {
    id: '',
    name: '',
    type: 'sftp',
    sftp: { user: '', host: '', port: 22, path: 'backups/' + serverName() },
    local: { path: '' },
    rest: { url: '', user: '', cacert: '' },
    options: { compression: 'auto', limit_upload: 0, limit_download: 0, connections: 0, extra_flags: [] },
    maintenance: {
      prune: {
        enabled: true,
        schedule: { mode: 'cron', cron: '0 4 * * 0', every: 24, unit: 'hours', catch_up: true },
        max_unused: '10%'
      },
      check: {
        enabled: true,
        schedule: { mode: 'cron', cron: '0 5 1 * *', every: 24, unit: 'hours', catch_up: true },
        read_data: 'subset',
        subset: '5%'
      }
    },
    unlock_stale: true,
    append_only: false
  };
}
