<script>
  import {onMount} from 'svelte'
  import {live, api, startStress, stopStress} from './state.svelte.js'
  import {palette, cleanModel, f0, f1} from './metricDefs.js'
  import {CHARTS, resolveSeries} from './chartDefs.js'
  import StreamChart from './StreamChart.svelte'

  // The stress panel is the deliberate counterpart to the dashboard: the same
  // sensors, but with the machine pushed to where its limits actually show. It
  // owns the run options; everything about a run in progress comes from the
  // backend through live.stress, so closing and reopening the tab — or the
  // window — never loses a running test.

  const DURATIONS = [
    {sec: 60, label: '1 min'},
    {sec: 300, label: '5 min'},
    {sec: 600, label: '10 min'},
    {sec: 1800, label: '30 min'},
    {sec: 3600, label: '1 hour'},
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
    ...(tempDir ? [{value: tempDir, label: 'Temp folder'}] : []),
  ])

  const theme = $derived(live.cfg?.theme ?? 'dark')
  const pal = $derived(palette(theme))
  const jobs = $derived(Object.fromEntries((live.stress.jobs ?? []).map((j) => [j.target, j])))
  const running = $derived(live.stress.running)
  const seconds = $derived(custom ? Math.max(0, Math.round(customMinutes)) * 60 : opts.seconds)

  const NODES = [
    {
      id: 'cpu',
      title: 'Processor',
      pkey: 'cpu',
      subtitle: (info) => cleanModel(info?.cpuModel) || 'CPU',
      blurb:
        'Rotates every thread through wide FMA (AVX-512 or AVX2), AES-NI, the SHA extensions, ' +
        'a 1024-bit modular exponentiation, a random cache walk and the transcendental library, ' +
        'so no execution port is left idle.',
    },
    {
      id: 'ram',
      title: 'Memory',
      pkey: 'ram',
      subtitle: (info) =>
        info?.ram?.speedMt ? `${info.ram.type || 'RAM'} · ${f0(info.ram.speedMt)} MT/s` : 'System memory',
      blurb:
        'Claims the free memory, writes an address-derived checkerboard pattern, streams the whole ' +
        'region through the controller and verifies it twice per pass. Mismatches are counted as faults.',
    },
    {
      id: 'gpu',
      title: 'Graphics',
      pkey: 'gpu',
      subtitle: (info) => cleanModel(info?.gpuName) || 'GPU',
      blurb:
        'Runs an OpenCL compute kernel on the shader array, fills VRAM to the chosen share and streams ' +
        'all of it through the memory interface — the part that an unstable memory clock trips over — ' +
        'verifying the contents on every pass.',
    },
    {
      id: 'disk',
      title: 'Storage',
      pkey: 'diskR',
      subtitle: () => 'Sequential and random I/O',
      blurb:
        'Writes an unbuffered test file to bypass the cache, then alternates large sequential transfers ' +
        'with 4 KB random requests at queue depth 16. The file is deleted when the run ends.',
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
    {key: 'cpuLoad', label: 'CPU load', unit: '%', pkey: 'cpu'},
    {key: 'cpuTemp', label: 'CPU temp', unit: '°C', pkey: 'cpu'},
    {key: 'cpuPow', label: 'CPU power', unit: 'W', pkey: 'cpu', digits: 1},
    {key: 'cpuClock', label: 'CPU clock', unit: 'MHz', pkey: 'cpu'},
    {key: 'gpuLoad', label: 'GPU load', unit: '%', pkey: 'gpu'},
    {key: 'gpuTemp', label: 'GPU temp', unit: '°C', pkey: 'gpu'},
    {key: 'gpuHotspot', label: 'GPU hot spot', unit: '°C', pkey: 'gpu'},
    {key: 'gpuPow', label: 'GPU power', unit: 'W', pkey: 'gpu', digits: 1},
    {key: 'gpuClock', label: 'GPU clock', unit: 'MHz', pkey: 'gpu'},
    {key: 'gpuMemClock', label: 'GPU mem clock', unit: 'MHz', pkey: 'vram'},
    {key: 'gpuFan', label: 'GPU fan', unit: '%', pkey: 'gpu'},
    {key: 'ram', label: 'RAM used', unit: 'GB', pkey: 'ram', digits: 1},
  ]

  const hasPeaks = $derived(PEAK_ROWS.some((r) => peaks[r.key]))

  // The two charts worth having on this tab: everything else is on the
  // dashboard, and these are the ones a burn-in is actually judged by.
  const stressCharts = $derived(CHARTS.filter((c) => c.title === 'Temperature' || c.title === 'Power draw'))
</script>

<!-- Run bar: duration, what to run, and the countdown. -->
<div class="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b border-line bg-page/90 px-4 py-2 backdrop-blur">
  <h2 class="mr-1 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Stress test</h2>

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
        {d.label}
      </button>
    {/each}
    <button
      class="px-2.5 py-1.5 font-mono text-[11px] {custom ? 'bg-card2 text-ink' : 'text-mut hover:text-ink2'}"
      onclick={() => (custom = true)}
    >
      Custom
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
      min {customMinutes > 0 ? '' : '— until stopped'}
    </label>
  {/if}

  <div class="grow"></div>

  {#if remaining}
    <span class="font-mono text-xs text-ink2">{remaining} left</span>
  {/if}

  {#if running}
    <button
      class="rounded-md border border-rec/50 bg-rec/10 px-4 py-1.5 font-mono text-xs text-ink hover:bg-rec/20"
      onclick={() => stopStress()}
    >
      Stop all
    </button>
  {:else}
    <button
      class="rounded-md border px-4 py-1.5 font-mono text-xs {anySelected
        ? 'border-ink2 bg-card2 text-ink hover:border-ink'
        : 'border-line text-mut'}"
      onclick={runSelected}
      disabled={!anySelected}
    >
      Run selected
    </button>
  {/if}
</div>

<div class="space-y-3 p-4">
  <p class="rounded-lg border border-vram/40 bg-vram/5 px-3 py-2 text-xs leading-relaxed text-ink2">
    <span class="font-mono text-vram">Warning</span> — a stress test drives the machine well past
    any normal workload: expect maximum fan noise, a hot case and full power draw. Stop the run if
    temperatures keep climbing, and be aware that the storage test writes several gigabytes per
    round, which counts against an SSD's endurance.
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
          <span class="font-mono text-[11px] uppercase tracking-[0.14em] text-ink">{node.title}</span>
          <span class="truncate font-mono text-[10px] text-mut">{node.subtitle(live.info)}</span>

          <div class="grow"></div>

          {#if job}
            <span class="font-mono text-[10px] uppercase tracking-[0.1em] {STATE_STYLE[job.state] ?? 'text-mut'}">
              {job.state}
            </span>
          {/if}

          <label class="flex items-center gap-1.5 font-mono text-[10px] text-mut" title="Include in “Run selected”">
            <input type="checkbox" class="accent-white" bind:checked={selected[node.id]} />
            select
          </label>

          {#if busy}
            <button
              class="rounded-md border border-rec/50 bg-rec/10 px-2.5 py-1 font-mono text-[11px] text-ink hover:bg-rec/20"
              onclick={() => stopStress([node.id])}
            >
              Stop
            </button>
          {:else}
            <button
              class="rounded-md border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:border-ink2 hover:text-ink"
              onclick={() => run([node.id])}
            >
              Start
            </button>
          {/if}
        </div>

        <p class="mt-2 text-xs leading-relaxed text-mut">{node.blurb}</p>

        <!-- per-node tuning -->
        <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
          {#if node.id === 'cpu'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              Threads
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
              Use AVX-512 where present
            </label>
          {:else if node.id === 'ram'}
            <label class="flex items-center gap-2 text-xs text-ink2">
              Share of free memory
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
              Share of VRAM
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
              Drive
              <select
                disabled={busy}
                class="rounded-md border border-line bg-card2 px-2 py-1 font-mono text-xs text-ink focus:outline-none disabled:text-mut"
                bind:value={opts.diskPath}
              >
                {#each diskChoices as c (c.value)}<option value={c.value}>{c.label}</option>{/each}
              </select>
            </label>
            <label class="flex items-center gap-2 text-xs text-ink2">
              Test file
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
                      {s.value >= 1000 ? f0(s.value) : f1(s.value)}
                      <span class="text-mut">{s.unit}</span>
                    </span>
                  </div>
                {/each}
              </div>
            {/if}

            {#if job.faults > 0}
              <div class="font-mono text-[11px] text-rec">
                {job.faults} data mismatches — this memory is not stable under load
              </div>
            {:else if job.state === 'done' && (node.id === 'ram' || node.id === 'gpu')}
              <div class="font-mono text-[11px] text-ram">No data mismatches</div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <!-- what the load is doing to the machine -->
  {#key theme}
    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      {#each stressCharts as c (c.title)}
        <StreamChart title={c.title} unit={c.unit} series={resolveSeries(c.series, theme)} {theme} />
      {/each}
    </div>
  {/key}

  {#if hasPeaks}
    <div class="rounded-lg border border-line bg-card p-3.5">
      <h2 class="mb-2.5 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Peaks during this run</h2>
      <div class="grid grid-cols-2 gap-x-5 gap-y-1.5 md:grid-cols-3 xl:grid-cols-5">
        {#each PEAK_ROWS as r (r.key)}
          {#if peaks[r.key]}
            <div class="flex items-baseline justify-between gap-2">
              <span class="truncate font-mono text-[10px] text-mut">{r.label}</span>
              <span class="shrink-0 font-mono text-xs" style="color:{pal[r.pkey]}">
                {r.digits === 1 ? f1(peaks[r.key]) : f0(peaks[r.key])}
                <span class="text-mut">{r.unit}</span>
              </span>
            </div>
          {/if}
        {/each}
      </div>
    </div>
  {/if}
</div>
