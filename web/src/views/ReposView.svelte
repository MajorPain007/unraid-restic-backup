<script lang="ts">
  let { opts = {} }: { opts?: Record<string, any> } = $props();
</script>

<div class="rb-empty"><h3>Repos</h3><p>{JSON.stringify(opts)}</p></div>
