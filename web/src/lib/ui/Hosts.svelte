<script lang="ts">
  import Icon from './Icon.svelte';
  import { closeDialog, closeMenu, closeTopDialog, dialogs, menu, toasts } from '../state.svelte';

  let menuEl: HTMLElement | undefined = $state();
  let pos = $state({ top: 0, left: 0 });

  $effect(() => {
    if (!menu.open || !menuEl || !menu.anchor) return;
    const r = menu.anchor;
    const w = menuEl.offsetWidth;
    let top = r.bottom + 4;
    if (top + menuEl.offsetHeight > window.innerHeight - 8) top = Math.max(8, r.top - menuEl.offsetHeight - 4);
    pos = { top, left: Math.max(8, Math.min(r.right - w, window.innerWidth - w - 8)) };
    menuEl.querySelector<HTMLElement>('button:not(:disabled)')?.focus();
  });

  function keydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    if (menu.open) {
      closeMenu();
      return;
    }
    if (dialogs.length) closeTopDialog();
  }

  function mousedown(e: MouseEvent) {
    if (menu.open && menuEl && !menuEl.contains(e.target as Node)) closeMenu();
  }
</script>

<svelte:window onkeydown={keydown} onmousedown={mousedown} onresize={closeMenu} />
<svelte:document onscrollcapture={closeMenu} />

{#each dialogs as d (d.id)}
  <d.component {...d.props} close={(r?: unknown) => closeDialog(d.id, r)} />
{/each}

{#if menu.open}
  <div bind:this={menuEl} class="rb-menu" role="menu" style="top:{pos.top}px;left:{pos.left}px">
    {#each menu.items as it, i (i)}
      {#if it === '-'}
        <hr />
      {:else}
        <button
          type="button"
          role="menuitem"
          class:danger={it.danger}
          disabled={it.disabled}
          onclick={() => {
            closeMenu();
            it.onclick();
          }}><Icon name={it.icon || 'right'} />{it.label}</button
        >
      {/if}
    {/each}
  </div>
{/if}

{#if toasts.length}
  <div class="rb-toasts" role="status" aria-live="polite">
    {#each toasts as t (t.id)}
      <div class="rb-toast {t.kind}">
        <Icon name={t.kind === 'err' ? 'alert' : t.kind === 'info' ? 'info' : 'check'} />
        <div>{t.message}</div>
      </div>
    {/each}
  </div>
{/if}
