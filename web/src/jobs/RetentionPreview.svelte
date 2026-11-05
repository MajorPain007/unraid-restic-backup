<script lang="ts" module>
  export interface RetentionItem {
    id: string;
    time: string;
    reasons?: string[];
  }
</script>

<script lang="ts">
  import Dialog from '../lib/ui/Dialog.svelte';
  import { dateTime, plural } from '../lib/format';

  let { keep, remove, close }: { keep: RetentionItem[]; remove: RetentionItem[]; close: (r?: unknown) => void } = $props();
</script>

{#snippet list(items: RetentionItem[], kept: boolean)}
  {#if !items.length}
    <div class="rb-hint">{kept ? 'None.' : 'Nothing - every snapshot stays.'}</div>
  {:else}
    <div class="rb-table-wrap">
      <table class="rb-table">
        <tbody>
          {#each items as s (s.id)}
            <tr class:rb-rp-gone={!kept}>
              <td class="rb-mono">{s.id.slice(0, 8)}</td>
              <td>{dateTime(Date.parse(s.time) / 1000)}</td>
              <td class="dim">{(s.reasons || []).join(', ')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/snippet}

<Dialog title="What this policy would do" icon="eye" wide onclose={() => close()}>
  <p>
    <b>{plural(keep.length, 'snapshot')} kept</b>, <b class:rb-err-text={remove.length > 0}>{remove.length} removed</b>. Nothing has been
    removed yet - this applies with the next backup after saving.
  </p>
  <h4 class="rb-rp-head">Kept</h4>
  {@render list(keep, true)}
  <h4 class="rb-rp-head">Removed</h4>
  {@render list(remove, false)}
</Dialog>

<style>
  .rb-rp-head {
    margin: 16px 0 8px;
  }
  .rb-rp-gone td:nth-child(2) {
    text-decoration: line-through;
    text-decoration-color: var(--rb-err);
  }
</style>
