<script>
  import {onMount} from 'svelte'
  import {api, live} from './state.svelte.js'
  import {CHARTS, buildBuffers, resolveSeries} from './chartDefs.js'
  import StaticChart from './StaticChart.svelte'

  let {session, onback, onexport} = $props()

  let bufs = $state(null)
  let error = $state('')
  const theme = $derived(live.cfg?.theme ?? 'dark')

  onMount(async () => {
    try {
      const samples = await api.GetSessionSamples(session.id)
      bufs = buildBuffers(samples ?? [])
    } catch (e) {
      error = String(e)
    }
  })
</script>

<div class="space-y-3 p-4">
  <div class="flex items-center gap-3">
    <button
      class="rounded-md border border-line bg-card px-2.5 py-1.5 font-mono text-[11px] text-ink2 hover:bg-card2"
      onclick={onback}
    >
      ← Back
    </button>
    <div class="min-w-0">
      <div class="truncate text-sm text-ink">{session.name}</div>
      <div class="font-mono text-[11px] text-mut">
        {new Date(session.startedAt).toLocaleString()} · {session.samples} samples
      </div>
    </div>
    <div class="grow"></div>
    <span class="font-mono text-[10px] text-mut">drag to zoom · double-click to reset</span>
    <button
      class="rounded-md border border-line bg-card px-2.5 py-1.5 font-mono text-[11px] text-ink2 hover:bg-card2"
      onclick={onexport}
    >
      Export PNG
    </button>
  </div>

  {#if error}
    <div class="rounded-lg border border-rec/40 bg-card p-4 font-mono text-xs text-ink2">{error}</div>
  {:else if !bufs}
    <div class="flex h-40 items-center justify-center font-mono text-sm text-mut">Loading…</div>
  {:else}
    {#key theme}
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {#each CHARTS as c (c.id)}
          <StaticChart
            title={c.title}
            unit={c.unit}
            series={resolveSeries(c.series, theme)}
            yMax={c.yMax ?? null}
            data={[bufs.t, ...c.series.map((sd) => bufs[sd.key])]}
            height={200}
            {theme}
          />
        {/each}
      </div>
    {/key}
  {/if}
</div>
