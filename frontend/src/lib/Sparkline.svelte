<script>
  // Minimal canvas sparkline for the HUD. uPlot would bring axes, legend and
  // cursor machinery that an always-on-top overlay has no room for.
  //
  // Points arrive in discrete steps — one per frame-rate event — but the line
  // scrolls continuously: redrawing only on arrival made it jump a whole point
  // at a time, which next to a game running at 100+ fps reads as stutter. Each
  // animation frame draws the same data shifted by how far along we are to the
  // next point, so the newest sample slides in from the right edge instead of
  // appearing on it.
  let {
    values = [],
    color = '#e66767',
    height = 42,
    unit = '',
    invert = false, // draw larger values lower (frame time: spikes point down)
    digits = 1,
    // How many points the graph is scaled for. Deriving the spacing from
    // values.length instead would squeeze the curve horizontally on every
    // arrival until the ring buffer is full — a visible crawl for the first
    // seconds after a game starts.
    capacity = 0,
  } = $props()

  let canvas
  let box

  // When the newest point arrived, and how far apart points have been coming.
  // The interval is measured rather than assumed: it is the backend's emit
  // cadence, and a wrong guess would make the scroll drift ahead or lag behind.
  let lastAt = 0
  let interval = 100
  let frame = 0

  // The drawn vertical range, eased towards the data's own. Rescaling straight
  // to it made the whole curve jump the instant a peak scrolled off the left
  // edge — the line had not moved, the axis had. Easing spreads that over a few
  // frames, which reads as the graph settling rather than snapping.
  let lo = 0
  let hi = 1
  let ranged = false

  function draw(progress) {
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
    if (pts.length < 2) {
      ranged = false // nothing is presenting; the next run starts from its own range
      return
    }

    // Scale to the data's own range, with a little headroom so the line never
    // rides the border.
    let want0 = Math.min(...pts)
    let want1 = Math.max(...pts)
    if (want1 - want0 < 1e-6) {
      want1 = want0 + 1
    }
    const pad = (want1 - want0) * 0.15
    want0 -= pad
    want1 += pad

    // Grow to fit immediately — a spike outside the drawn range is a spike
    // clipped off the canvas — but shrink back gradually.
    if (!ranged) {
      lo = want0
      hi = want1
      ranged = true
    } else {
      lo = want0 < lo ? want0 : lo + (want0 - lo) * 0.12
      hi = want1 > hi ? want1 : hi + (want1 - hi) * 0.12
    }

    const n = values.length
    const stepX = w / Math.max(1, (capacity || n) - 1)
    // A point lands on the right edge the instant it arrives and drifts one
    // step left over the interval, so when the next one takes its place the
    // series is already exactly where the new indices put it — nothing moves at
    // the seam. Running this the other way round (`1 - progress`) carried the
    // whole series *rightwards* for an interval and then snapped it two steps
    // back on arrival: an average drift that was right, and a motion that was a
    // sawtooth ten times a second. That was the shake.
    const xOf = (i) => w - (n - 1 - i + progress) * stepX
    const yOf = (v) => {
      const t = (v - lo) / (hi - lo)
      return invert ? t * (height - 2) + 1 : height - 1 - t * (height - 2)
    }

    // Filled area under the curve, then the curve itself.
    ctx.beginPath()
    let started = false
    let firstX = 0
    let lastX = 0
    for (let i = 0; i < n; i++) {
      const v = values[i]
      if (v == null || !isFinite(v)) continue
      const x = xOf(i)
      const y = yOf(v)
      if (!started) {
        ctx.moveTo(x, y)
        firstX = x
        started = true
      } else {
        ctx.lineTo(x, y)
      }
      lastX = x
    }
    ctx.strokeStyle = color
    ctx.lineWidth = 1.5
    ctx.lineJoin = 'round'
    ctx.lineCap = 'round'
    ctx.stroke()

    ctx.lineTo(lastX, invert ? 0 : height)
    ctx.lineTo(firstX, invert ? 0 : height)
    ctx.closePath()
    ctx.fillStyle = color + '25'
    ctx.fill()
  }

  // One animation frame per display refresh while data is flowing, then idle.
  // An overlay that keeps a rAF loop alive over a paused game would be burning
  // a slice of the frame budget it exists to measure.
  //
  // `at` is the frame's own timestamp rather than performance.now(): it shares
  // the same origin, but it is when the frame will be shown instead of whenever
  // this callback happened to be reached, so the scroll is paced by the display
  // and not by however long the rest of the frame took.
  function tick(at) {
    const since = at - lastAt
    // Clamped low as well as high: a point can land after the frame's timestamp
    // was taken but before this callback runs, and a negative progress would
    // push the series a step to the right of where it belongs.
    draw(Math.min(Math.max(since / interval, 0), 1))

    if (since < interval * 2 + 200) {
      frame = requestAnimationFrame(tick)
    } else {
      frame = 0
    }
  }

  $effect(() => {
    void values // a new array means a new point landed

    const now = performance.now()
    if (lastAt) {
      // Smoothed, and bounded either side of anything the backend plausibly
      // emits, so one late event cannot stretch the scroll for seconds.
      const dt = Math.min(Math.max(now - lastAt, 30), 1000)
      interval = interval * 0.7 + dt * 0.3
    }
    lastAt = now

    if (!frame) frame = requestAnimationFrame(tick)
  })

  $effect(() => {
    const ro = new ResizeObserver(() => draw(1))
    if (box) ro.observe(box)
    return () => ro.disconnect()
  })

  $effect(() => () => cancelAnimationFrame(frame))

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
