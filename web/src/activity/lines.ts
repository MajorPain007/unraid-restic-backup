export type LineKind = '' | 'phase' | 'cmd' | 'bad' | 'warn' | 'note';

export interface Line {
  i: number;
  t: string;
  c: LineKind;
}

export function classify(line: string): LineKind {
  if (/\s==\s/.test(line)) return 'phase';
  if (/\s\$ restic /.test(line)) return 'cmd';
  if (/error|failed|fatal|refused|denied/i.test(line)) return 'bad';
  if (/warning|\bwarn\b/i.test(line)) return 'warn';
  return '';
}

export function pieces(text: string, query: string): [string, boolean][] {
  if (!query) return [[text, false]];
  const out: [string, boolean][] = [];
  const low = text.toLowerCase();
  let from = 0;
  for (let at = low.indexOf(query, from); at !== -1; at = low.indexOf(query, from)) {
    if (at > from) out.push([text.slice(from, at), false]);
    out.push([text.slice(at, at + query.length), true]);
    from = at + query.length;
  }
  if (from < text.length) out.push([text.slice(from), false]);
  return out;
}

export function fileName(op: Record<string, any>): string {
  const d = new Date((op.started || Date.now() / 1000) * 1000);
  const p = (v: number) => String(v).padStart(2, '0');
  const stamp = `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}_${p(d.getHours())}-${p(d.getMinutes())}`;
  const name = String(op.kind === 'backup' || op.kind === 'forget' ? op.job_name || op.job || '' : op.repo_name || op.repo || '');
  const safe = (op.kind + '-' + name).replace(/[^A-Za-z0-9._-]+/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '');
  return `restic-${safe || 'log'}-${stamp}.log`;
}

export function saveText(name: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    a.remove();
    URL.revokeObjectURL(url);
  }, 1000);
}
