<script>
  import {onMount} from 'svelte'
  import {live, api} from './state.svelte.js'
  import {palette} from './metricDefs.js'
  import BrandIcon from './BrandIcon.svelte'

  let drives = $state([])

  onMount(() => {
    const load = async () => {
      try {
        drives = (await api.GetDiskHealth()) ?? []
      } catch {
        /* keep last known */
      }
    }
    load()
    const t = setInterval(load, 15000)
    return () => clearInterval(t)
  })

  const pal = $derived(palette(live.cfg?.theme))
  const vols = $derived(live.sample?.disks ?? [])
  const mb = (bps) => (bps / 2 ** 20).toFixed(1)

  const smartMissing = $derived(
    drives.length > 0 && drives.every((d) => !d.tempC) && !live.info?.isAdmin
  )
</script>

<div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
  <!-- physical drives + SMART -->
  <div class="rounded-lg border border-line bg-card p-3">
    <div class="mb-2 flex items-baseline justify-between">
      <span class="font-mono text-[10px] uppercase tracking-[0.14em] text-mut">Drives · SMART</span>
      {#if smartMissing}
        <span class="font-mono text-[10px] text-mut" title="SMART pass-through needs elevation">
          run as admin for temp/wear
        </span>
      {/if}
    </div>
    <div class="space-y-2">
      {#each drives as d (d.model)}
        <div class="flex items-center gap-3 rounded-md bg-card2/50 px-2.5 py-2">
          <span class="text-ink2"><BrandIcon brand={d.media === 'HDD' ? 'disk-hdd' : 'disk-ssd'} size={18} /></span>
          <div class="min-w-0 grow">
            <div class="truncate text-xs text-ink">{d.model}</div>
            <div class="font-mono text-[10px] text-mut">
              {[d.bus, d.media, d.sizeGb ? d.sizeGb.toFixed(0) + ' GB' : ''].filter(Boolean).join(' · ')}
            </div>
          </div>
          {#if d.tempC}
            <span class="font-mono text-xs text-ink2">{d.tempC.toFixed(0)}°C</span>
          {/if}
          {#if d.lifePercent}
            <span class="font-mono text-xs text-ink2" title="Remaining life">{d.lifePercent.toFixed(0)}%</span>
          {/if}
          {#if d.health}
            <span
              class="rounded-full px-2 py-0.5 font-mono text-[10px] {d.health === 'Healthy'
                ? 'bg-ram/15 text-ram'
                : 'bg-rec/15 text-rec'}"
            >
              {d.health}
            </span>
          {/if}
        </div>
      {:else}
        <div class="py-4 text-center font-mono text-xs text-mut">No drive info available</div>
      {/each}
    </div>
  </div>

  <!-- per-volume live throughput -->
  <div class="rounded-lg border border-line bg-card p-3">
    <div class="mb-2">
      <span class="font-mono text-[10px] uppercase tracking-[0.14em] text-mut">Volumes · live I/O</span>
    </div>
    <div class="space-y-2">
      {#each vols as v (v.name)}
        <div class="rounded-md bg-card2/50 px-2.5 py-2">
          <div class="flex items-baseline justify-between">
            <span class="font-mono text-xs text-ink">{v.name}</span>
            <span class="font-mono text-[11px]">
              <span style="color:{pal.diskR}">Read {mb(v.readBps)}</span>
              <span class="text-mut">·</span>
              <span style="color:{pal.diskW}">Write {mb(v.writeBps)}</span>
              <span class="text-mut"> MB/s</span>
            </span>
          </div>
          {#if v.usedPercent}
            <div class="mt-1.5 flex items-center gap-2">
              <div class="h-[3px] grow overflow-hidden rounded-full bg-card2">
                <div class="h-full rounded-full bg-mut" style="width:{Math.min(100, v.usedPercent)}%"></div>
              </div>
              <span class="font-mono text-[10px] text-mut">{v.usedPercent.toFixed(0)}% used</span>
            </div>
          {/if}
        </div>
      {:else}
        <div class="py-4 text-center font-mono text-xs text-mut">Waiting for samples…</div>
      {/each}
    </div>
  </div>
</div>
