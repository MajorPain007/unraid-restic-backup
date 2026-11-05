<script lang="ts">
  import { statusOf } from './describe';

  let { status }: { status: string } = $props();
  let s = $derived(statusOf(status));
</script>

<span class="rb-pill {s[0]}">{s[1]}</span>
