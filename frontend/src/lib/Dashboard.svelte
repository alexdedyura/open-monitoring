<script>
  import {live, api} from './state.svelte.js'
  import {COLORS, diskTotals} from './metricDefs.js'
  import {CHARTS} from './chartDefs.js'
  import StatTile from './StatTile.svelte'
  import StreamChart from './StreamChart.svelte'

  const RANGES = [
    {sec: 60, label: '1m'},
    {sec: 300, label: '5m'},
    {sec: 900, label: '15m'},
    {sec: 1800, label: '30m'},
  ]

  let recName = $state('')
  let busy = $state(false)

  async function toggleRec() {
    if (busy) return
    busy = true
    try {
      if (live.rec.active) {
        live.rec = await api.StopRecording()
      } else {
        live.rec = await api.StartRecording(recName.trim())
        recName = ''
      }
    } finally {
      busy = false
    }
  }

  const elapsed = $derived.by(() => {
    void live.tick
    if (!live.rec.active) return ''
    const s = Math.max(0, Math.floor((Date.now() - live.rec.startedAt) / 1000))
    const h = Math.floor(s / 3600)
    const m = String(Math.floor(s / 60) % 60).padStart(2, '0')
    return `${h}:${m}:${String(s % 60).padStart(2, '0')}`
  })

  const s = $derived(live.sample)
  const dsk = $derived(s ? diskTotals(s) : [0, 0])
  const f0 = (v) => (v == null ? '—' : v.toFixed(0))
  const f1 = (v) => (v == null ? '—' : v.toFixed(1))

  const yMaxByKey = $derived({
    ram: live.info ? live.info.ramTotal / 2 ** 30 : null,
    vram: s?.gpu?.memTotalMb ? s.gpu.memTotalMb / 1024 : null,
  })
</script>

<div class="space-y-3 p-4">
  <!-- record + range toolbar -->
  <div class="flex items-center gap-2">
    {#if live.rec.active}
      <button
        class="nodrag flex items-center gap-2 rounded-md border border-rec/50 bg-rec/10 px-3 py-1.5 font-mono text-xs text-ink hover:bg-rec/20"
        onclick={toggleRec}
      >
        <span class="rec-dot h-2 w-2 rounded-full bg-rec"></span>
        Stop · {elapsed}
      </button>
      <span class="truncate font-mono text-xs text-ink2">{live.rec.name}</span>
      <span class="font-mono text-[10px] text-mut">auto-stop {live.rec.maxMinutes} min</span>
    {:else}
      <button
        class="flex items-center gap-2 rounded-md border border-line bg-card px-3 py-1.5 font-mono text-xs text-ink hover:border-rec/60 hover:bg-card2"
        onclick={toggleRec}
      >
        <span class="h-2 w-2 rounded-full bg-rec"></span>
        Record
      </button>
      <input
        class="w-56 rounded-md border border-line bg-card px-2.5 py-1.5 font-mono text-xs text-ink placeholder:text-mut focus:border-ink2 focus:outline-none"
        placeholder="Session name (optional)"
        bind:value={recName}
        onkeydown={(e) => e.key === 'Enter' && toggleRec()}
      />
    {/if}
    <div class="grow"></div>
    <div class="flex overflow-hidden rounded-md border border-line">
      {#each RANGES as r (r.sec)}
        <button
          class="px-2.5 py-1.5 font-mono text-[11px] {live.range === r.sec
            ? 'bg-card2 text-ink'
            : 'text-mut hover:text-ink2'}"
          onclick={() => (live.range = r.sec)}
        >
          {r.label}
        </button>
      {/each}
    </div>
  </div>

  {#if s}
    <!-- current values -->
    <div class="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-6">
      <StatTile
        label="CPU"
        color={COLORS.cpu}
        value={f0(s.cpu.usage) + '%'}
        sub={s.cpu.tempC ? `${f0(s.cpu.tempC)}° · ${f0(s.cpu.clockMhz)} MHz` : ''}
        bars={s.cpu.perCore}
      />
      <StatTile
        label="GPU"
        color={COLORS.gpu}
        value={s.gpu ? f0(s.gpu.usage) + '%' : '—'}
        sub={s.gpu ? `${f0(s.gpu.tempC)}° · ${f0(s.gpu.powerW)} W` : 'no data'}
        pct={s.gpu?.usage ?? 0}
      />
      <StatTile
        label="RAM"
        color={COLORS.ram}
        value={f1(s.mem.used / 2 ** 30) + ' GB'}
        sub={`${f0(s.mem.usedPercent)}% of ${f0(s.mem.total / 2 ** 30)} GB`}
        pct={s.mem.usedPercent}
      />
      <StatTile
        label="VRAM"
        color={COLORS.vram}
        value={s.gpu?.memUsedMb ? f1(s.gpu.memUsedMb / 1024) + ' GB' : '—'}
        sub={s.gpu?.memTotalMb ? `of ${f1(s.gpu.memTotalMb / 1024)} GB` : ''}
        pct={s.gpu?.memTotalMb ? (s.gpu.memUsedMb / s.gpu.memTotalMb) * 100 : 0}
      />
      <StatTile
        label="Disk"
        color={COLORS.diskR}
        value={`R ${f0(dsk[0])} W ${f0(dsk[1])}`}
        sub="MB/s"
      />
      <StatTile
        label="Net"
        color={COLORS.netU}
        value={`↓${f1(s.net.downBps * 8 / 1e6)} ↑${f1(s.net.upBps * 8 / 1e6)}`}
        sub="Mbit/s"
      />
    </div>

    <!-- charts -->
    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      {#each CHARTS as c (c.title)}
        <StreamChart
          title={c.title}
          unit={c.unit}
          series={c.series}
          yMax={c.yMaxKey ? yMaxByKey[c.yMaxKey] : (c.yMax ?? null)}
        />
      {/each}
    </div>
  {:else}
    <div class="flex h-64 items-center justify-center font-mono text-sm text-mut">
      Collecting first samples…
    </div>
  {/if}
</div>
