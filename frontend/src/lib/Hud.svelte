<script>
  import {live, exitHud, fpsBuf, api, FPS_POINTS} from './state.svelte.js'
  import {METRICS, HUD_GROUPS} from './metricDefs.js'
  import Logo from './Logo.svelte'
  import Sparkline from './Sparkline.svelte'

  let hover = $state(false)
  let shell = $state()

  // The FPS graphs run off the fast frame-rate event, not the sample stream —
  // copied per tick so Svelte sees a new array (the buffers are deliberately
  // non-reactive).
  const series = $derived.by(() => {
    void live.fpsTick
    return {fps: [...fpsBuf.fps], frameMs: [...fpsBuf.frameMs]}
  })

  // Every row reads from the sample, but the frame-rate part of it is stale by
  // up to a full sampling interval — overlay the fresher numbers on top.
  const shown = $derived.by(() => {
    void live.tick, live.fpsTick
    if (!live.sample) return null
    return live.fps ? {...live.sample, fps: live.fps} : live.sample
  })

  // Sections: HUD_GROUPS order, rows filtered by the user's enabled metric set.
  const sections = $derived.by(() => {
    const enabled = live.cfg?.hud.metrics ?? []
    return HUD_GROUPS.map((g) => ({
      ...g,
      rows: g.keys.filter((k) => enabled.includes(k)).map((k) => ({key: k, ...METRICS[k]})),
    })).filter((g) => g.rows.length > 0)
  })

  const opacity = $derived(live.cfg?.hud.opacity ?? 0.85)

  // Anchored to a corner means anchored: the window is put there on every
  // resize, so a drag could only fight it. Dropping the drag region also makes
  // it obvious the overlay is fixed rather than mysteriously snapping back.
  const anchor = $derived(live.cfg?.hud.anchor ?? 'free')
  const movable = $derived(anchor === 'free' || anchor === '')

  // The window is fitted to the overlay, not the overlay to the window: how
  // many rows and graphs there are depends on the user's metric selection, and
  // the window no longer has resize handles to make up the difference. `shell`
  // is laid out at its natural height, so measuring it cannot feed back into
  // the resize it triggers.
  $effect(() => {
    if (!shell) return

    let queued = 0
    const report = () => {
      queued = 0
      const h = Math.ceil(shell.getBoundingClientRect().height)
      if (h > 0) api.FitHudSize(h, window.innerHeight)
    }
    // Coalesced to one report per frame: a config change can move every row at
    // once, and each intermediate height would be a window resize of its own.
    const ro = new ResizeObserver(() => {
      if (!queued) queued = requestAnimationFrame(report)
    })
    ro.observe(shell)
    return () => {
      cancelAnimationFrame(queued)
      ro.disconnect()
    }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  bind:this={shell}
  class="w-full {movable ? 'drag' : 'nodrag'}"
  onmouseenter={() => (hover = true)}
  onmouseleave={() => (hover = false)}
>
  <div
    class="relative flex flex-col overflow-hidden rounded-xl border border-white/10 px-3.5 py-2.5 font-mono"
    style="background: rgba(8, 10, 14, {opacity})"
  >
    <!-- header -->
    <div class="mb-1 flex items-center gap-2">
      <Logo size={14} />
      <span class="text-[9px] uppercase tracking-[0.16em] text-white/50">Open Monitoring</span>
      <span class="ml-auto flex items-center gap-1.5">
        {#if live.cfg?.hud.clickThrough}
          <span class="text-[10px] leading-none text-white/40" title="Click-through: the mouse passes into the game">⊘</span>
        {/if}
        {#if live.rec.active}
          <span class="rec-dot h-1.5 w-1.5 rounded-full bg-rec" title="Recording"></span>
        {/if}
      </span>
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

    {#if shown}
      <div class="space-y-2 pt-1">
        {#each sections as g (g.id)}
          {#if g.id === 'fps'}
            <div class="border-t border-white/10 pt-1.5">
              {#if g.rows.some((r) => r.key === 'fps')}
                <div class="flex items-baseline justify-between gap-4">
                  <span class="text-[15px] font-bold text-white" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">FPS</span>
                  <span class="text-[22px] font-bold leading-6" style="color:{METRICS.fps.color}; text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {METRICS.fps.value(shown)}
                  </span>
                </div>
              {/if}
              {#each g.rows.filter((r) => r.key !== 'fps' && !r.graph) as m (m.key)}
                <div class="flex items-baseline justify-between gap-4 leading-[17px]">
                  <span class="text-[11px] text-white/60" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">{m.row}</span>
                  <span class="whitespace-nowrap text-[12px] font-semibold text-white" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {m.value(shown, live.info)}
                  </span>
                </div>
              {/each}

              {#each g.rows.filter((r) => r.graph) as m (m.key)}
                <div class="mt-1.5">
                  <div class="mb-0.5 text-[9px] uppercase tracking-[0.1em] text-white/40" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                    {m.row}
                  </div>
                  <div class="rounded border border-white/10 bg-black/40 px-1 pt-1">
                    <Sparkline
                      values={series[m.graph.key]}
                      color={m.color}
                      unit={m.graph.unit}
                      digits={m.graph.digits}
                      invert={m.graph.invert ?? false}
                      capacity={FPS_POINTS}
                      height={38}
                    />
                  </div>
                </div>
              {/each}
              {#if shown.fps?.process}
                <div class="truncate text-[9px] text-white/40" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
                  {shown.fps.process}
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
                    {m.value(shown, live.info)}
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        {/each}
      </div>
    {:else}
      <!-- Fixed height: the window is sized from this, and a zero-height
           placeholder would collapse the overlay before the first sample. -->
      <div class="flex h-16 items-center justify-center text-[11px] text-white/40">…</div>
    {/if}
  </div>
</div>
