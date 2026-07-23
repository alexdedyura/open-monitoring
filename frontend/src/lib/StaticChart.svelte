<script>
  import {onMount} from 'svelte'
  import uPlot from 'uplot'
  import {makeOpts} from './uplotOpts.js'

  // data: uPlot-shaped [[t...], [v...], ...]; drag to zoom, double-click resets
  let {title, unit = '', series = [], data, yMax = null, height = 200, theme = 'dark'} = $props()

  let el
  let plot

  onMount(() => {
    plot = new uPlot(
      makeOpts({width: el.clientWidth || 600, height, series, yMax, unit, zoom: true, theme}),
      data,
      el
    )
    const ro = new ResizeObserver(() => {
      if (el.clientWidth) plot.setSize({width: el.clientWidth, height})
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
      plot.destroy()
    }
  })
</script>

<div class="rounded-lg border border-line bg-card p-3">
  <div class="mb-2">
    <span class="font-mono text-[10px] uppercase tracking-[0.14em] text-mut">{title}</span>
  </div>
  <div bind:this={el}></div>
</div>
