<script lang="ts" module>
  const SHAPES: Record<string, string> = {
    image: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="m20.5 16-5-5-8.5 8.5"/>',
    video: '<rect x="3" y="5.5" width="13" height="13" rx="2"/><path d="m16 10 5-3v10l-5-3z"/>',
    audio: '<path d="M9 18V5.5l11-2V16"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="17.5" cy="16" r="2.5"/>',
    archive: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M11 5h1M11 8h1M11 11h1M10.5 14h2v3h-2z"/>',
    code: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="m10 12-2 2 2 2M14 12l2 2-2 2"/>',
    text: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h6"/>',
    data: '<ellipse cx="12" cy="6" rx="7" ry="2.8"/><path d="M5 6v12c0 1.5 3.1 2.8 7 2.8s7-1.3 7-2.8V6"/><path d="M5 12c0 1.5 3.1 2.8 7 2.8s7-1.3 7-2.8"/>',
    disk: '<rect x="3.5" y="3.5" width="17" height="17" rx="2"/><circle cx="12" cy="12" r="4"/><path d="M12 12h.01"/>'
  };

  const EXT: Record<string, string> = {};
  const groups: [string, string][] = [
    ['image', 'jpg jpeg png gif webp bmp tif tiff heic heif svg ico raw cr2 nef arw dng'],
    ['video', 'mp4 mkv avi mov wmv m4v webm mpg mpeg ts m2ts flv vob'],
    ['audio', 'mp3 flac wav aac ogg opus m4a wma alac aiff'],
    ['archive', 'zip tar gz tgz bz2 xz zst 7z rar txz lz4 deb rpm'],
    ['code', 'js ts mjs php py sh bash go rs c h cpp java rb pl lua html css json yaml yml toml xml ini conf cfg plg page svelte'],
    ['text', 'txt md log csv nfo srt pdf doc docx odt rtf xls xlsx ods'],
    ['data', 'db sqlite sqlite3 sql mdb dat bak'],
    ['disk', 'img iso qcow2 vdisk vmdk vhd vhdx']
  ];
  for (const [kind, list] of groups) for (const e of list.split(' ')) EXT[e] = kind;

  export function fileKind(name: string): string {
    const i = name.lastIndexOf('.');
    if (i <= 0) return '';
    return EXT[name.slice(i + 1).toLowerCase()] || '';
  }
</script>

<script lang="ts">
  import Icon from '../lib/ui/Icon.svelte';

  let { type, name }: { type: string; name: string } = $props();

  let kind = $derived(type === 'file' || type === '' ? fileKind(name) : '');
</script>

{#if type === 'dir'}
  <Icon name="folder" class="dir" />
{:else if type === 'symlink'}
  <Icon name="link" class="file" />
{:else if kind}
  <svg viewBox="0 0 24 24" class="rb-i file kind-{kind}" aria-hidden="true">{@html SHAPES[kind]}</svg>
{:else}
  <Icon name="file" class="file" />
{/if}
