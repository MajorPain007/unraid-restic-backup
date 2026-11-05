export interface RuleOut {
  kind: string;
  label: string;
  via?: string;
}

export interface TreeEntry {
  name: string;
  path: string;
  type: 'dir' | 'file' | 'symlink' | 'missing' | string;
  size: number | null;
  open: boolean;
  out?: RuleOut | null;
  mount?: boolean;
}

export interface TreeAction {
  title: string;
  run: () => void;
}

const OUT_LABEL: Record<string, string> = {
  zfs: 'ZFS snapshots',
  plugin: "the plugin's own files",
  caches: 'cache folder',
  otherfs: 'other file system',
  size: 'larger than '
};

export function exact(path: string): string {
  return path.replace(/[\\*?[\]]/g, '\\$&');
}

export function outText(out: RuleOut): string {
  if (out.kind === 'pattern' || out.kind === 'ipattern') return out.label + (out.kind === 'ipattern' ? ' (any case)' : '');
  return (OUT_LABEL[out.kind] ?? out.kind) + (out.kind === 'size' ? out.label : '');
}

export function removeAll(list: string[], pattern: string): void {
  for (let i = list.length - 1; i >= 0; i--) if (list[i] === pattern) list.splice(i, 1);
}
