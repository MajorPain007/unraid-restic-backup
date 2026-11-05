const LOCALE = 'en-GB';

export function bytes(n: number | null | undefined): string {
  if (n === null || n === undefined || isNaN(n)) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let i = 0;
  let v = Number(n);
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return (i === 0 ? String(v) : v.toFixed(v < 10 ? 1 : 0)) + ' ' + units[i];
}

export function duration(s: number | null | undefined): string {
  if (s === null || s === undefined || isNaN(s)) return '-';
  s = Math.max(0, Math.round(s));
  if (s < 60) return s + ' s';
  if (s < 3600) return Math.round(s / 60) + ' min';
  if (s < 86400) {
    const hh = Math.floor(s / 3600);
    const mm = Math.round((s % 3600) / 60);
    return hh + ' h' + (mm ? ' ' + mm + ' min' : '');
  }
  const d = Math.floor(s / 86400);
  const rh = Math.round((s % 86400) / 3600);
  return d + ' d' + (rh ? ' ' + rh + ' h' : '');
}

export function now(): number {
  return Date.now() / 1000;
}

function dayName(dt: Date): string {
  const today = new Date();
  const d0 = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
  const d1 = new Date(dt.getFullYear(), dt.getMonth(), dt.getDate()).getTime();
  const days = Math.round((d1 - d0) / 86400000);
  if (days === -1) return 'yesterday';
  if (days === 1) return 'tomorrow';
  return dt.toLocaleDateString(LOCALE, { weekday: 'short' });
}

export function clock(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString(LOCALE, { hour: '2-digit', minute: '2-digit' });
}

export function dateTime(ts: number | null | undefined): string {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString(LOCALE, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  });
}

export function relative(ts: number | null | undefined): string {
  if (!ts) return '-';
  const diff = ts - now();
  const abs = Math.abs(diff);
  if (abs < 45) return diff < 0 ? 'just now' : 'now';
  let text: string;
  if (abs < 3600) text = Math.round(abs / 60) + ' min';
  else if (abs < 86400) text = Math.round(abs / 3600) + ' h';
  else if (abs < 86400 * 7) return dayName(new Date(ts * 1000)) + ' ' + clock(ts);
  else return dateTime(ts);
  return diff < 0 ? text + ' ago' : 'in ' + text;
}

export function shortDate(ts: number, withYear = false): string {
  return new Date(ts * 1000).toLocaleDateString(
    LOCALE,
    withYear ? { day: 'numeric', month: 'short', year: '2-digit' } : { day: 'numeric', month: 'short' }
  );
}

export function dayLabel(ts: number): string {
  const dt = new Date(ts * 1000);
  const label = dayName(dt);
  if (label === 'yesterday' || label === 'tomorrow') return label.charAt(0).toUpperCase() + label.slice(1);
  const today = new Date();
  if (dt.toDateString() === today.toDateString()) return 'Today';
  return dt.toLocaleDateString(LOCALE, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: dt.getFullYear() === today.getFullYear() ? undefined : 'numeric'
  });
}

export function plural(n: number, one: string, many?: string): string {
  return n + ' ' + (n === 1 ? one : many || one + 's');
}

export function count(n: number): string {
  return Number(n).toLocaleString(LOCALE);
}
