<script>
  // Minimal canvas sparkline for the HUD. uPlot would bring axes, legend and
  // cursor machinery that an always-on-top overlay has no room for.
  let {
    values = [],
    color = '#e66767',
    height = 42,
    unit = '',
    invert = false, // draw larger values lower (frame time: spikes point down)
    digits = 1,
  } = $props()

  let canvas
  let box

  function draw() {
    if (!canvas || !box) return
    const dpr = window.devicePixelRatio || 1
    const w = box.clientWidth
    if (!w) return

    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(height * dpr)) {
      canvas.width = Math.round(w * dpr)
      canvas.height = Math.round(height * dpr)
      canvas.style.width = w + 'px'
      canvas.style.height = height + 'px'
    }

    const ctx = canvas.getContext('2d')
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, w, height)

    const pts = values.filter((v) => v != null && isFinite(v))
    if (pts.length < 2) return

    // Scale to the data's own range, with a little headroom so the line never
    // rides the border.
    let lo = Math.min(...pts)
    let hi = Math.max(...pts)
    if (hi - lo < 1e-6) {
      hi = lo + 1
    }
    const pad = (hi - lo) * 0.15
    lo -= pad
    hi += pad

    const n = values.length
    const stepX = w / Math.max(1, n - 1)
    const yOf = (v) => {
      const t = (v - lo) / (hi - lo)
      return invert ? t * (height - 2) + 1 : height - 1 - t * (height - 2)
    }

    // Filled area under the curve, then the curve itself.
    ctx.beginPath()
    let started = false
    for (let i = 0; i < n; i++) {
      const v = values[i]
      if (v == null || !isFinite(v)) continue
      const x = i * stepX
      const y = yOf(v)
      if (!started) {
        ctx.moveTo(x, y)
        started = true
      } else {
        ctx.lineTo(x, y)
      }
    }
    ctx.strokeStyle = color
    ctx.lineWidth = 1.5
    ctx.lineJoin = 'round'
    ctx.stroke()

    ctx.lineTo((n - 1) * stepX, invert ? 0 : height)
    ctx.lineTo(0, invert ? 0 : height)
    ctx.closePath()
    ctx.fillStyle = color + '25'
    ctx.fill()
  }

  $effect(() => {
    void values
    draw()
  })

  $effect(() => {
    const ro = new ResizeObserver(draw)
    if (box) ro.observe(box)
    return () => ro.disconnect()
  })

  const latest = $derived.by(() => {
    for (let i = values.length - 1; i >= 0; i--) {
      const v = values[i]
      if (v != null && isFinite(v)) return v
    }
    return null
  })
</script>

<div class="relative w-full" bind:this={box} style="height:{height}px">
  <canvas bind:this={canvas} class="block"></canvas>
  <span
    class="pointer-events-none absolute bottom-0 left-0 text-[9px] leading-none"
    style="color:{color}; text-shadow: 0 1px 2px rgba(0,0,0,.9)"
  >
    {latest == null ? '—' : latest.toFixed(digits)}{unit}
  </span>
</div>
