<script>
  import {onMount} from 'svelte'
  import uPlot from 'uplot'
  import {buf, live} from './state.svelte.js'
  import {makeOpts} from './uplotOpts.js'
  import {onFrame} from './chartClock.js'

  // series: [{key, label, color, digits?, fill?}]
  let {title, unit = '', series = [], yMax = null, height = 170, theme = 'dark'} = $props()

  let el
  let plot

  // The chart's right edge is *now*, not the newest sample. Anchoring it to the
  // sample meant the whole plot stood still for a second and then jumped a
  // pixel or two sideways when one arrived — the data is a step function, but
  // time is not, and it was time the x axis was drawing. So the scroll runs on
  // the animation clock and samples simply land at the right edge as they come.
  const nowSec = () => Date.now() / 1000

  function data() {
    const n = buf.t.length
    if (!n) return [[], ...series.map(() => [])]
    const cut = nowSec() - live.range
    let i = n - 1
    while (i > 0 && buf.t[i - 1] >= cut) i--
    // Keep the sample just outside the window too, so the line is drawn off the
    // left edge and clipped there rather than ending a step short of it.
    if (i > 0) i--
    return [buf.t.slice(i), ...series.map((s) => buf[s.key].slice(i))]
  }

  // Redraw only once the view has actually moved a fraction of a pixel. The
  // 30-minute range crawls across the plot at a third of a pixel per second,
  // and repainting ten charts sixty times a second to move them by a hundredth
  // of one is pure heat — this settles at a couple of frames a second there and
  // in the thirties on the 1-minute range. The step stays well under a pixel
  // either way, which is the part that reads as smooth.
  const MIN_SHIFT_PX = 0.25
  let drawnAt = 0

  function scroll() {
    if (!plot) return
    const now = nowSec()
    const pxPerSec = (plot.over?.clientWidth || 1) / live.range
    if ((now - drawnAt) * pxPerSec < MIN_SHIFT_PX) return

    drawnAt = now
    // Setting the x scale explicitly re-ranges the auto y scale over the
    // visible window and commits the redraw, which is why setData below can
    // skip its own.
    plot.setScale('x', {min: now - live.range, max: now})
  }

  onMount(() => {
    plot = new uPlot(makeOpts({width: el.clientWidth || 600, height, series, yMax, unit, theme, smooth: true}), data(), el)
    const ro = new ResizeObserver(() => {
      if (el.clientWidth) plot.setSize({width: el.clientWidth, height})
    })
    ro.observe(el)
    const stopScroll = onFrame(scroll)
    return () => {
      stopScroll()
      ro.disconnect()
      plot.destroy()
    }
  })

  $effect(() => {
    void live.tick
    void live.range
    // `false`: the x scale belongs to the scroll loop, and letting new data
    // reset it to the sample extent is exactly the jump this avoids. That also
    // means setData draws nothing on its own — clearing the throttle hands the
    // repaint to the next frame rather than to whenever the view next moves a
    // quarter of a pixel, which on a 30-minute range is seconds away.
    plot?.setData(data(), false)
    drawnAt = 0
  })

  const latest = $derived.by(() => {
    void live.tick
    return series.map((s) => {
      const v = buf[s.key].at(-1)
      return v == null ? '—' : v.toFixed(s.digits ?? 0)
    })
  })
</script>

<div class="rounded-lg border border-line bg-card p-3">
  <div class="mb-2 flex items-baseline justify-between">
    <span class="font-mono text-[10px] uppercase tracking-[0.14em] text-mut">{title}</span>
    <span class="font-mono text-xs text-ink2">
      {#each series as s, i (s.key)}
        {#if i > 0}<span class="text-mut">&nbsp;·&nbsp;</span>{/if}
        <span style="color:{s.color}">{latest[i]}</span>
      {/each}
      {#if unit}<span class="text-mut">&nbsp;{unit}</span>{/if}
    </span>
  </div>
  <div bind:this={el}></div>
</div>
