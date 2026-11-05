import ConfirmDialog from './ui/ConfirmDialog.svelte';
import PathPicker from './ui/PathPicker.svelte';
import { openDialog } from './state.svelte';

export function confirm(opts: { title: string; text: string; confirm?: string; danger?: boolean }): Promise<boolean> {
  return openDialog<boolean>(ConfirmDialog, opts).then((r) => r === true);
}

export function pickPath(opts: {
  title?: string;
  confirm?: string;
  start?: string;
  create?: boolean;
  files?: boolean;
}): Promise<string | null>;
export function pickPath(opts: {
  title?: string;
  confirm?: string;
  start?: string;
  create?: boolean;
  files?: boolean;
  multiple: true;
}): Promise<string[] | null>;
export function pickPath(opts: Record<string, any>): Promise<any> {
  return openDialog(PathPicker, opts).then((r) => r ?? null);
}
