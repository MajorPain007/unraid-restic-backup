export class ApiError extends Error {
  errors: string[];

  constructor(message: string, errors: string[] = []) {
    super(message);
    this.errors = errors;
  }
}

function root(): HTMLElement {
  return document.getElementById('rb-app') as HTMLElement;
}

function endpoint(): string {
  return root().dataset.api || '';
}

function csrf(): string {
  const w = window as unknown as { csrf_token?: string };
  return w.csrf_token || root().dataset.csrf || '';
}

export type Params = Record<string, unknown>;

export async function api<T = Record<string, any>>(action: string, params: Params = {}): Promise<T> {
  const body = new URLSearchParams();
  body.append('action', action);
  body.append('csrf_token', csrf());
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null) continue;
    body.append(k, typeof v === 'object' ? JSON.stringify(v) : String(v));
  }
  const res = await fetch(endpoint(), { method: 'POST', body, credentials: 'same-origin' });
  const text = await res.text();
  let data: any;
  try {
    data = JSON.parse(text);
  } catch {
    throw new ApiError(
      text === ''
        ? `The server sent an empty answer (HTTP ${res.status}). Reload the page - the security token may have expired.`
        : `The server sent something unexpected (HTTP ${res.status}).`
    );
  }
  if (!data.ok) {
    throw new ApiError(data.error || 'Something went wrong.', data.errors || []);
  }
  return data as T;
}

export function download(params: Record<string, string>): void {
  let frame = document.getElementById('rb-download-frame') as HTMLIFrameElement | null;
  if (!frame) {
    frame = document.createElement('iframe');
    frame.id = frame.name = 'rb-download-frame';
    frame.style.display = 'none';
    document.body.appendChild(frame);
  }
  const form = document.createElement('form');
  form.method = 'POST';
  form.action = endpoint();
  form.target = 'rb-download-frame';
  form.style.display = 'none';
  const all: Record<string, string> = { action: 'download', csrf_token: csrf(), ...params };
  for (const [k, v] of Object.entries(all)) {
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = k;
    input.value = v;
    form.appendChild(input);
  }
  document.body.appendChild(form);
  form.submit();
  setTimeout(() => form.remove(), 1000);
}
