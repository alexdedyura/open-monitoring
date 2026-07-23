<script>
  import {onMount} from 'svelte'
  import uPlot from 'uplot'
  import {buf, live} from './state.svelte.js'
  import {makeOpts} from './uplotOpts.js'

  // series: [{key, label, color, digits?, fill?}]
  let {title, unit = '', series = [], yMax = null, height = 170} = $props()

  let el
  let plot

  function data() {
    const n = buf.t.length
    if (!n) return [[], ...series.map(() => [])]
    const cut = buf.t[n - 1] - live.range
    let i = n - 1
    while (i > 0 && buf.t[i - 1] >= cut) i--
    return [buf.t.slice(i), ...series.map((s) => buf[s.key].slice(i))]
  }

  onMount(() => {
    plot = new uPlot(makeOpts({width: el.clientWidth || 600, height, series, yMax, unit}), data(), el)
    const ro = new ResizeObserver(() => {
      if (el.clientWidth) plot.setSize({width: el.clientWidth, height})
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
      plot.destroy()
    }
  })

  $effect(() => {
    void live.tick
    void live.range
    plot?.setData(data())
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
