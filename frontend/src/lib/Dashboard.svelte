<script>
  import {live, api} from './state.svelte.js'
  import {palette, diskTotals} from './metricDefs.js'
  import {CHARTS, resolveSeries} from './chartDefs.js'
  import {t, fmtNum, lang} from './i18n.svelte.js'
  import StatTile from './StatTile.svelte'
  import StreamChart from './StreamChart.svelte'
  import StoragePanel from './StoragePanel.svelte'

  // Script scope, so these hold KEYS — a t() call here would freeze in the
  // language that was active at mount. The template translates them.
  const RANGES = [
    {sec: 60, labelKey: 'dash.range.1m'},
    {sec: 300, labelKey: 'dash.range.5m'},
    {sec: 900, labelKey: 'dash.range.15m'},
    {sec: 1800, labelKey: 'dash.range.30m'},
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

  const theme = $derived(live.cfg?.theme ?? 'dark')
  const pal = $derived(palette(theme))
  const s = $derived(live.sample)
  const dsk = $derived(s ? diskTotals(s) : [0, 0])

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
        {t('dash.rec.stop')} · {elapsed}
      </button>
      <span class="truncate font-mono text-xs text-ink2">{live.rec.name}</span>
      <span class="font-mono text-[10px] text-mut">{t('dash.rec.autostop', {n: live.rec.maxMinutes})}</span>
    {:else}
      <button
        class="flex items-center gap-2 rounded-md border border-line bg-card px-3 py-1.5 font-mono text-xs text-ink hover:border-rec/60 hover:bg-card2"
        onclick={toggleRec}
      >
        <span class="h-2 w-2 rounded-full bg-rec"></span>
        {t('dash.rec.start')}
      </button>
      <input
        class="w-56 rounded-md border border-line bg-card px-2.5 py-1.5 font-mono text-xs text-ink placeholder:text-mut focus:border-ink2 focus:outline-none"
        placeholder={t('dash.rec.name.placeholder')}
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
          {t(r.labelKey)}
        </button>
      {/each}
    </div>
  </div>

  {#if s}
    <!-- current values -->
    <div class="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-6">
      <!-- CPU/GPU/RAM/VRAM stay Latin in every language (see the glossary). -->
      <StatTile
        label="CPU"
        color={pal.cpu}
        value={fmtNum(s.cpu.usage, 0) + '%'}
        sub={s.cpu.tempC ? `${fmtNum(s.cpu.tempC, 0)}° · ${fmtNum(s.cpu.clockMhz, 0)} MHz` : ''}
        bars={s.cpu.perCore}
      />
      <StatTile
        label="GPU"
        color={pal.gpu}
        value={s.gpu ? fmtNum(s.gpu.usage, 0) + '%' : '—'}
        sub={s.gpu ? `${fmtNum(s.gpu.tempC, 0)}° · ${fmtNum(s.gpu.powerW, 0)} W` : t('dash.tile.nodata')}
        pct={s.gpu?.usage ?? 0}
      />
      <StatTile
        label="RAM"
        color={pal.ram}
        value={fmtNum(s.mem.used / 2 ** 30, 1) + ' GB'}
        sub={t('dash.tile.ram.sub', {
          pct: fmtNum(s.mem.usedPercent, 0),
          total: fmtNum(s.mem.total / 2 ** 30, 0),
        })}
        pct={s.mem.usedPercent}
      />
      <StatTile
        label="VRAM"
        color={pal.vram}
        value={s.gpu?.memUsedMb ? fmtNum(s.gpu.memUsedMb / 1024, 1) + ' GB' : '—'}
        sub={s.gpu?.memTotalMb
          ? t('dash.tile.vram.sub', {total: fmtNum(s.gpu.memTotalMb / 1024, 1)})
          : ''}
        pct={s.gpu?.memTotalMb ? (s.gpu.memUsedMb / s.gpu.memTotalMb) * 100 : 0}
      />
      <StatTile
        label={t('dash.tile.disk')}
        color={pal.diskR}
        value={`${fmtNum(dsk[0], 1)} / ${fmtNum(dsk[1], 1)}`}
        sub={t('dash.tile.disk.sub')}
      />
      <StatTile
        label={t('dash.tile.net')}
        color={pal.netU}
        value={`↓${fmtNum(s.net.downBps * 8 / 1e6, 1)} ↑${fmtNum(s.net.upBps * 8 / 1e6, 1)}`}
        sub="Mbit/s"
      />
    </div>

    <!-- charts (recreated on theme switch so uPlot picks up new colors, and on
         a language switch so it picks up the new series labels — it copies the
         legend at construction) -->
    {#key theme + lang.code}
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {#each CHARTS as c (c.id)}
          <StreamChart
            title={t(c.titleKey ?? c.title)}
            unit={c.unit}
            series={resolveSeries(c.series, theme)}
            yMax={c.yMaxKey ? yMaxByKey[c.yMaxKey] : (c.yMax ?? null)}
            {theme}
          />
        {/each}
      </div>
    {/key}

    <!-- storage details -->
    <StoragePanel />
  {:else}
    <div class="flex h-64 items-center justify-center font-mono text-sm text-mut">
      {t('dash.collecting')}
    </div>
  {/if}
</div>
