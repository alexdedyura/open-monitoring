<script>
  import {onMount} from 'svelte'
  import {live, api, startStress, stopStress} from './state.svelte.js'
  import {palette, cleanModel} from './metricDefs.js'
  import {CHARTS, resolveSeries} from './chartDefs.js'
  import {t, fmtNum, lang} from './i18n.svelte.js'
  import StreamChart from './StreamChart.svelte'

  // The stress panel is the deliberate counterpart to the dashboard: the same
  // sensors, but with the machine pushed to where its limits actually show. It
  // owns the run options; everything about a run in progress comes from the
  // backend through live.stress, so closing and reopening the tab — or the
  // window — never loses a running test.
  //
  // Every table of labels below holds KEYS, never t() results: a constant
  // evaluated at script scope freezes in the language that was active at mount.
  // The templates translate at the point of use. The one exception that only
  // looks like one is a `subtitle` arrow — its body runs when the template calls
  // it, which is inside tracked code.

  const DURATIONS = [
    {sec: 60, labelKey: 'stress.duration.1m'},
    {sec: 300, labelKey: 'stress.duration.5m'},
    {sec: 600, labelKey: 'stress.duration.10m'},
    {sec: 1800, labelKey: 'stress.duration.30m'},
    {sec: 3600, labelKey: 'stress.duration.1h'},
  ]

  // Per-node options, plus which of them "Run selected" covers.
  let opts = $state({
    seconds: 300,
    cpuThreads: 0,
    cpuAvx512: true,
    ramPercent: 70,
    gpuPercent: 80,
    diskPath: '',
    diskMb: 2048,
  })
  let selected = $state({cpu: true, ram: true, gpu: true, disk: false})
  let custom = $state(false)
  let customMinutes = $state(3)
  let error = $state('')
  let tempDir = $state('')

  // The backend knows this machine's core count and temp directory; the panel
  // would otherwise have to guess both.
  onMount(async () => {
    const d = await api.GetStressDefaults()
    tempDir = d.diskPath
    if (!opts.cpuThreads) opts.cpuThreads = d.cpuThreads
    if (!opts.diskPath) opts.diskPath = d.diskPath
  })

  // Drive letters come from the machine description; the temp folder is always
  // offered because it is the one location guaranteed to be writable.
  const diskChoices = $derived([
    ...(live.info?.disks ?? []).map((d) => ({value: d, label: d})),
    ...(tempDir ? [{value: tempDir, label: t('stress.disk.tempFolder')}] : []),
  ])

  const theme = $derived(live.cfg?.theme ?? 'dark')
  const pal = $derived(palette(theme))
  const jobs = $derived(Object.fromEntries((live.stress.jobs ?? []).map((j) => [j.target, j])))
  const running = $derived(live.stress.running)
  const seconds = $derived(custom ? Math.max(0, Math.round(customMinutes)) * 60 : opts.seconds)

  // `subtitle` is a function, so the t() inside it is evaluated where the
  // template calls it — the only reason a translated string may live in this
  // table at all. Titles and blurbs are plain values and therefore keys.
  const NODES = [
    {
      id: 'cpu',
      titleKey: 'stress.node.cpu.title',
      pkey: 'cpu',
      subtitle: (info) => cleanModel(info?.cpuModel) || 'CPU',
      blurbKey: 'stress.node.cpu.blurb',
    },
    {
      id: 'ram',
      titleKey: 'stress.node.ram.title',
      pkey: 'ram',
      subtitle: (info) =>
        info?.ram?.speedMt
          ? `${info.ram.type || 'RAM'} · ${fmtNum(info.ram.speedMt)} MT/s`
          : t('stress.node.ram.subtitle'),
      blurbKey: 'stress.node.ram.blurb',
    },
    {
      id: 'gpu',
      titleKey: 'stress.node.gpu.title',
      pkey: 'gpu',
      subtitle: (info) => cleanModel(info?.gpuName) || 'GPU',
      blurbKey: 'stress.node.gpu.blurb',
    },
    {
      id: 'disk',
      titleKey: 'stress.node.disk.title',
      pkey: 'diskR',
      subtitle: () => t('stress.node.disk.subtitle'),
      blurbKey: 'stress.node.disk.blurb',
    },
  ]

  const STATE_STYLE = {
    running: 'text-ram',
    done: 'text-ram',
    stopped: 'text-mut',
    starting: 'text-vram',
    failed: 'text-rec',
  }

  async function run(targets) {
    error = ''
    try {
      await startStress({...$state.snapshot(opts), seconds, targets})
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  const runSelected = () => run(NODES.map((n) => n.id).filter((id) => selected[id]))
  const anySelected = $derived(NODES.some((n) => selected[n.id]))

  // Countdown, driven by the sample tick so it stays in step with the charts.
  const remaining = $derived.by(() => {
    void live.tick
    const ends = (live.stress.jobs ?? []).filter((j) => j.state === 'running' && j.endsAt).map((j) => j.endsAt)
    if (!ends.length) return ''
    const s = Math.max(0, Math.round((Math.max(...ends) - Date.now()) / 1000))
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  })

  // Peaks are the reason to watch a stress run at all: the sustained figure a
  // machine settles at after the boost clocks fade. Kept in a plain object so
  // updating it cannot re-trigger the effect that fills it.
  let peaks = $state({})
  let acc = {}
  let wasRunning = false

  $effect(() => {
    void live.tick
    const s = live.sample
    if (!s) return

    if (running && !wasRunning) acc = {} // a fresh run starts from nothing
    wasRunning = running
    if (!running) return

    const bump = (k, v) => {
      if (v) acc[k] = Math.max(acc[k] ?? 0, v)
    }
    bump('cpuTemp', s.cpu.tempC)
    bump('cpuPow', s.cpu.powerW)
    bump('cpuClock', s.cpu.clockMhz)
    bump('cpuLoad', s.cpu.usage)
    bump('gpuLoad', s.gpu?.usage)
    bump('gpuTemp', s.gpu?.tempC)
    bump('gpuHotspot', s.gpu?.hotspotC)
    bump('gpuPow', s.gpu?.powerW)
    bump('gpuClock', s.gpu?.coreMhz)
    bump('gpuMemClock', s.gpu?.memMhz)
    bump('gpuFan', s.gpu?.fanPercent)
    bump('ram', s.mem.used / 2 ** 30)
    peaks = {...acc}
  })

  const PEAK_ROWS = [
    {key: 'cpuLoad', labelKey: 'stress.peak.cpuLoad', unit: '%', pkey: 'cpu'},
    {key: 'cpuTemp', labelKey: 'stress.peak.cpuTemp', unit: '°C', pkey: 'cpu'},
    {key: 'cpuPow', labelKey: 'stress.peak.cpuPow', unit: 'W', pkey: 'cpu', digits: 1},
    {key: 'cpuClock', labelKey: 'stress.peak.cpuClock', unit: 'MHz', pkey: 'cpu'},
    {key: 'gpuLoad', labelKey: 'stress.peak.gpuLoad', unit: '%', pkey: 'gpu'},
    {key: 'gpuTemp', labelKey: 'stress.peak.gpuTemp', unit: '°C', pkey: 'gpu'},
    {key: 'gpuHotspot', labelKey: 'stress.peak.gpuHotspot', unit: '°C', pkey: 'gpu'},
    {key: 'gpuPow', labelKey: 'stress.peak.gpuPow', unit: 'W', pkey: 'gpu', digits: 1},
    {key: 'gpuClock', labelKey: 'stress.peak.gpuClock', unit: 'MHz', pkey: 'gpu'},
    {key: 'gpuMemClock', labelKey: 'stress.peak.gpuMemClock', unit: 'MHz', pkey: 'vram'},
    {key: 'gpuFan', labelKey: 'stress.peak.gpuFan', unit: '%', pkey: 'gpu'},
    {key: 'ram', labelKey: 'stress.peak.ram', unit: 'GB', pkey: 'ram', digits: 1},
  ]

  const hasPeaks = $derived(PEAK_ROWS.some((r) => peaks[r.key]))

  // The two charts worth having on this tab: everything else is on the
  // dashboard, and these are the ones a burn-in is actually judged by.
  const stressCharts = $derived(CHARTS.filter((c) => c.id === 'temperature' || c.id === 'power-draw'))
</script>

<!-- Run bar: duration, what to run, and the countdown. -->
<div class="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b border-line bg-page/90 px-4 py-2 backdrop-blur">
  <h2 class="mr-1 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('stress.title')}</h2>

  <div class="flex overflow-hidden rounded-md border border-line">
    {#each DURATIONS as d (d.sec)}
      <button
        class="px-2.5 py-1.5 font-mono text-[11px] {!custom && opts.seconds === d.sec
          ? 'bg-card2 text-ink'
          : 'text-mut hover:text-ink2'}"
        onclick={() => {
          custom = false
          opts.seconds = d.sec
        }}
      >
        {t(d.labelKey)}
      </button>
    {/each}
    <button
      class="px-2.5 py-1.5 font-mono text-[11px] {custom ? 'bg-card2 text-ink' : 'text-mut hover:text-ink2'}"
      onclick={() => (custom = true)}
    >
      {t('stress.duration.custom')}
    </button>
  </div>

  {#if custom}
    <label class="flex items-center gap-1.5 font-mono text-[11px] text-mut">
      <input
        type="number"
        min="0"
        max="1440"
        class="w-16 rounded-md border border-line bg-card px-2 py-1 text-right font-mono text-xs text-ink focus:border-ink2 focus:outline-none"
        bind:value={customMinutes}
      />
      {t('stress.duration.minutes')}
      {customMinutes > 0 ? '' : t('stress.duration.untilStopped')}
    </label>
  {/if}

  <div class="grow"></div>

  {#if remaining}
    <span class="font-mono text-xs text-ink2">{t('stress.timeLeft', {time: remaining})}</span>
  {/if}

  {#if running}
    <button
      class="rounded-md border border-rec/50 bg-rec/10 px-4 py-1.5 font-mono text-xs text-ink hover:bg-rec/20"
      onclick={() => stopStress()}
    >
      {t('stress.stopAll')}
    </button>
  {:else}
    <button
      class="rounded-md border px-4 py-1.5 font-mono text-xs {anySelected
        ? 'border-ink2 bg-card2 text-ink hover:border-ink'
        : 'border-line text-mut'}"
      onclick={runSelected}
      disabled={!anySelected}
    >
      {t('stress.runSelected')}
    </button>
  {/if}
</div>

<div class="space-y-3 p-4">
  <p class="rounded-lg border border-vram/40 bg-vram/5 px-3 py-2 text-xs leading-relaxed text-ink2">
    <span class="font-mono text-vram">{t('stress.warning.label')}</span>
    {t('stress.warning.body')}
  </p>

  {#if error}
    <p class="rounded-lg border border-rec/40 bg-rec/5 px-3 py-2 font-mono text-xs text-rec">{error}</p>
  {/if}

  <!-- one card per node -->
  <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
    {#each NODES as node (node.id)}
      {@const job = jobs[node.id]}
      {@const busy = job?.state === 'running' || job?.state === 'starting'}
      <div
        class="flex flex-col rounded-lg border bg-card p-3.5 {busy ? '' : 'border-line'}"
        style={busy ? `border-color:${pal[node.pkey]}` : ''}
      >
        <div class="flex items-center gap-2">
          <span class="h-2 w-2 shrink-0 rounded-sm" style="background:{pal[node.pkey]}"></span>
          <span class="font-mono text-[11px] uppercase tracking-[0.14em] text-ink">{t(node.titleKey)}</span>
          <span class="truncate font-mono text-[10px] text-mut">{node.subtitle(live.info)}</span>

          <div class="grow"></div>

          {#if job}
            <span class="font-mono text-[10px] uppercase tracking-[0.1em] {STATE_STYLE[job.state] ?? 'text-mut'}">
              {t(`stress.state.${job.state}`)}
            </span>
          {/if}

          <label class="flex items-center gap-1.5 font-mono text-[10px] text-mut" title={t('stress.selectHint')}>
            <input type="checkbox" class="accent-white" bind:checked={selected[node.id]} />
            {t('stress.select')}
          </label>

          {#if busy}
            <button
              class="rounded-md border border-rec/50 bg-rec/10 px-2.5 py-1 font-mono text-[11px] text-ink hover:bg-rec/20"
              onclick={() => stopStress([node.id])}
            >
              {t('stress.stop')}
            </button>
          {:else}
            <button
              class="rounded-md border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:border-ink2 hover:text-ink"
              onclick={() => run([node.id])}
            >
              {t('stress.start')}
            </button>
          {/if}
        </div>

        <p class="mt-2 text-xs leading-relaxed text-mut">{t(node.blurbKey)}</p>

        <!-- per-node tuning -->
        <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
          {#if node.id === 'cpu'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              {t('stress.cpu.threads')}
              <input
                type="number"
                min="1"
                max="256"
                disabled={busy}
                class="w-16 rounded-md border border-line bg-card2 px-2 py-1 text-right font-mono text-xs text-ink focus:border-ink2 focus:outline-none disabled:text-mut"
                bind:value={opts.cpuThreads}
              />
            </label>
            <label class="flex items-center gap-2 text-xs text-ink2">
              <input type="checkbox" class="accent-white" disabled={busy} bind:checked={opts.cpuAvx512} />
              {t('stress.cpu.avx512')}
            </label>
          {:else if node.id === 'ram'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              {t('stress.ram.share')}
              <select
                disabled={busy}
                class="rounded-md border border-line bg-card2 px-2 py-1 font-mono text-xs text-ink focus:outline-none disabled:text-mut"
                bind:value={opts.ramPercent}
              >
                {#each [25, 50, 70, 90] as p (p)}<option value={p}>{p}%</option>{/each}
              </select>
            </label>
          {:else if node.id === 'gpu'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              {t('stress.gpu.share')}
              <select
                disabled={busy}
                class="rounded-md border border-line bg-card2 px-2 py-1 font-mono text-xs text-ink focus:outline-none disabled:text-mut"
                bind:value={opts.gpuPercent}
              >
                {#each [40, 60, 80, 95] as p (p)}<option value={p}>{p}%</option>{/each}
              </select>
            </label>
          {:else if node.id === 'disk'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              {t('stress.disk.drive')}
              <select
                disabled={busy}
                class="rounded-md border border-line bg-card2 px-2 py-1 font-mono text-xs text-ink focus:outline-none disabled:text-mut"
                bind:value={opts.diskPath}
              >
                {#each diskChoices as c (c.value)}<option value={c.value}>{c.label}</option>{/each}
              </select>
            </label>
            <label class="flex items-center gap-2 text-xs text-ink2">
              {t('stress.disk.file')}
              <select
                disabled={busy}
                class="rounded-md border border-line bg-card2 px-2 py-1 font-mono text-xs text-ink focus:outline-none disabled:text-mut"
                bind:value={opts.diskMb}
              >
                {#each [1024, 2048, 4096, 8192, 16384] as mb (mb)}
                  <option value={mb}>{mb / 1024} GB</option>
                {/each}
              </select>
            </label>
          {/if}
        </div>

        <!-- live results -->
        {#if job}
          <div class="mt-3 space-y-2 border-t border-line pt-3">
            {#if job.detail}
              <div class="font-mono text-[10px] leading-relaxed text-mut">{job.detail}</div>
            {/if}
            {#if job.error}
              <div class="font-mono text-[11px] text-rec">{job.error}</div>
            {:else if job.phase}
              <div class="font-mono text-[11px] text-ink2">{job.phase}</div>
            {/if}

            {#if job.stats?.length}
              <div class="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
                {#each job.stats as s (s.label)}
                  <div class="flex items-baseline justify-between gap-2">
                    <span class="truncate font-mono text-[10px] text-mut">{s.label}</span>
                    <span class="shrink-0 font-mono text-xs text-ink">
                      {fmtNum(s.value, s.value >= 1000 ? 0 : 1)}
                      <span class="text-mut">{s.unit}</span>
                    </span>
                  </div>
                {/each}
              </div>
            {/if}

            {#if job.faults > 0}
              <div class="font-mono text-[11px] text-rec">
                {t('stress.faults.count', {n: job.faults})}
              </div>
            {:else if job.state === 'done' && (node.id === 'ram' || node.id === 'gpu')}
              <div class="font-mono text-[11px] text-ram">{t('stress.faults.none')}</div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <!-- what the load is doing to the machine; re-keyed on the language too,
       because uPlot copies the series labels into its legend at construction -->
  {#key theme + lang.code}
    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      {#each stressCharts as c (c.id)}
        <!-- the chart header is chartDefs' string to own; translate it if that
             table carries a key, otherwise print what it gives us -->
        <StreamChart
          title={c.titleKey ? t(c.titleKey) : c.title}
          unit={c.unit}
          series={resolveSeries(c.series, theme)}
          {theme}
        />
      {/each}
    </div>
  {/key}

  {#if hasPeaks}
    <div class="rounded-lg border border-line bg-card p-3.5">
      <h2 class="mb-2.5 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('stress.peaks.title')}</h2>
      <div class="grid grid-cols-2 gap-x-5 gap-y-1.5 md:grid-cols-3 xl:grid-cols-5">
        {#each PEAK_ROWS as r (r.key)}
          {#if peaks[r.key]}
            <div class="flex items-baseline justify-between gap-2">
              <span class="truncate font-mono text-[10px] text-mut">{t(r.labelKey)}</span>
              <span class="shrink-0 font-mono text-xs" style="color:{pal[r.pkey]}">
                {fmtNum(peaks[r.key], r.digits ?? 0)}
                <span class="text-mut">{r.unit}</span>
              </span>
            </div>
          {/if}
        {/each}
      </div>
    </div>
  {/if}
</div>
