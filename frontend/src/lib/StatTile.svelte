<script>
  // One glanceable current-value tile; entity color threads through the
  // capacity bar and optional per-core strip.
  let {label, value, sub = '', color, pct = null, bars = null} = $props()
</script>

<div class="rounded-lg border border-line bg-card px-3 pt-2.5 pb-2">
  <div class="flex items-baseline justify-between gap-2">
    <span class="font-mono text-[10px] uppercase tracking-[0.14em] text-mut">{label}</span>
    {#if sub}<span class="truncate font-mono text-[10px] text-ink2">{sub}</span>{/if}
  </div>
  <div class="mt-1 font-mono text-xl leading-none text-ink">{value}</div>
  {#if bars}
    <div class="mt-2 flex h-5 items-end gap-[2px]">
      {#each bars as b}
        <div class="w-full rounded-[1px]" style="height:{Math.max(6, b)}%; background:{color}; opacity:.8"></div>
      {/each}
    </div>
  {:else if pct != null}
    <div class="mt-2.5 h-[3px] w-full overflow-hidden rounded-full bg-card2">
      <div class="h-full rounded-full" style="width:{Math.min(100, pct)}%; background:{color}"></div>
    </div>
  {:else}
    <div class="mt-2.5 h-[3px] w-full"></div>
  {/if}
</div>
