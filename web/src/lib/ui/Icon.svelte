<script lang="ts">
  import { ICONS } from '../icons';

  let { name, class: cls = '' }: { name: string; class?: string } = $props();
</script>

<svg viewBox="0 0 24 24" class="rb-i {cls}" aria-hidden="true">{@html ICONS[name] ?? ''}</svg>
