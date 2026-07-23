<script>
  import {live, exitHud} from './state.svelte.js'
  import {METRICS} from './metricDefs.js'

  let hover = $state(false)

  const items = $derived((live.cfg?.hud.metrics ?? []).map((k) => METRICS[k]).filter(Boolean))
  const opacity = $derived(live.cfg?.hud.opacity ?? 0.85)
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="drag h-full w-full p-1"
  onmouseenter={() => (hover = true)}
  onmouseleave={() => (hover = false)}
>
  <div
    class="relative flex h-full flex-col justify-evenly overflow-hidden rounded-xl border border-line px-3 py-2 font-mono"
    style="background: rgba(10, 12, 16, {opacity})"
  >
    {#if live.rec.active}
      <div class="rec-dot absolute left-2 top-2 h-1.5 w-1.5 rounded-full bg-rec" title="Recording"></div>
    {/if}

    {#if hover}
      <button
        class="nodrag absolute right-1 top-1 z-10 rounded px-1.5 py-0.5 text-[11px] text-ink2 hover:bg-white/10 hover:text-ink"
        onclick={exitHud}
        title="Back to dashboard"
      >
        ✕
      </button>
    {/if}

    {#if live.sample}
      {#each items as m (m.label)}
        <div class="flex items-baseline justify-between gap-4 leading-tight">
          <span class="text-[11px] font-semibold tracking-wide" style="color:{m.color}; text-shadow: 0 1px 2px rgba(0,0,0,.9)">
            {m.label}
          </span>
          <span class="text-[13px] text-ink" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
            {m.value(live.sample)}
          </span>
        </div>
      {/each}
    {:else}
      <div class="text-center text-[11px] text-mut">…</div>
    {/if}
  </div>
</div>
