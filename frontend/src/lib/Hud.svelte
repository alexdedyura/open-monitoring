<script>
  import {live, exitHud} from './state.svelte.js'
  import {METRICS, HUD_GROUPS} from './metricDefs.js'

  let hover = $state(false)

  // Sections: HUD_GROUPS order, rows filtered by the user's enabled metric set.
  const sections = $derived.by(() => {
    const enabled = live.cfg?.hud.metrics ?? []
    return HUD_GROUPS.map((g) => ({
      ...g,
      rows: g.keys.filter((k) => enabled.includes(k)).map((k) => ({key: k, ...METRICS[k]})),
    })).filter((g) => g.rows.length > 0)
  })

  const opacity = $derived(live.cfg?.hud.opacity ?? 0.85)
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="drag h-full w-full"
  onmouseenter={() => (hover = true)}
  onmouseleave={() => (hover = false)}
>
  <div
    class="relative flex h-full flex-col overflow-hidden rounded-xl border border-white/10 px-3.5 py-2.5 font-mono"
    style="background: rgba(8, 10, 14, {opacity})"
  >
    <!-- header -->
    <div class="mb-1 flex items-center gap-2">
      <div class="grid grid-cols-2 gap-[2px]">
        <span class="h-[4px] w-[4px] rounded-[1px] bg-[#3987e5]"></span>
        <span class="h-[4px] w-[4px] rounded-[1px] bg-[#d95926]"></span>
        <span class="h-[4px] w-[4px] rounded-[1px] bg-[#199e70]"></span>
        <span class="h-[4px] w-[4px] rounded-[1px] bg-[#c98500]"></span>
      </div>
      <span class="text-[9px] uppercase tracking-[0.16em] text-white/50">Open Monitoring</span>
      {#if live.rec.active}
        <span class="rec-dot ml-auto h-1.5 w-1.5 rounded-full bg-[#e34948]" title="Recording"></span>
      {/if}
    </div>

    {#if hover}
      <button
        class="nodrag absolute right-1.5 top-1.5 z-10 rounded px-1.5 py-0.5 text-[11px] text-white/60 hover:bg-white/10 hover:text-white"
        onclick={exitHud}
        title="Back to dashboard"
      >
        ✕
      </button>
    {/if}

    {#if live.sample}
      <div class="min-h-0 flex-1 space-y-2 overflow-hidden pt-1">
        {#each sections as g (g.id)}
          {#if g.id === 'fps'}
            <div class="border-t border-white/10 pt-1.5">
              {#if g.rows.some((r) => r.key === 'fps')}
                <div class="flex items-baseline justify-between gap-4">
                  <span class="text-[15px] font-bold text-white" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">FPS</span>
                  <span class="text-[22px] font-bold leading-6" style="color:{METRICS.fps.color}; text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {METRICS.fps.value(live.sample)}
                  </span>
                </div>
              {/if}
              {#each g.rows.filter((r) => r.key !== 'fps') as m (m.key)}
                <div class="flex items-baseline justify-between gap-4 leading-[17px]">
                  <span class="text-[11px] text-white/60" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">{m.row}</span>
                  <span class="whitespace-nowrap text-[12px] font-semibold text-white" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {m.value(live.sample, live.info)}
                  </span>
                </div>
              {/each}
              {#if live.sample.fps?.process}
                <div class="truncate text-[9px] text-white/40" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                  {live.sample.fps.process}
                </div>
              {/if}
            </div>
          {:else}
            <div>
              <div
                class="mb-0.5 truncate text-[12px] font-bold leading-5"
                style="color:{METRICS[g.keys[0]]?.color}; text-shadow: 0 1px 2px rgba(0,0,0,.9)"
                title={g.header(live.info)}
              >
                {g.header(live.info)}
              </div>
              {#each g.rows as m (m.key)}
                <div class="flex items-baseline justify-between gap-4 leading-[17px]">
                  <span class="text-[11px]" style="color:{m.color}; opacity:.85; text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {m.row}
                  </span>
                  <span class="whitespace-nowrap text-[12px] font-semibold text-white" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {m.value(live.sample, live.info)}
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        {/each}
      </div>
    {:else}
      <div class="flex flex-1 items-center justify-center text-[11px] text-white/40">…</div>
    {/if}
  </div>
</div>
