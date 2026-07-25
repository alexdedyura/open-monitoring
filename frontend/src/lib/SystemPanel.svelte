<script>
  import {live} from './state.svelte.js'
  import {cleanModel} from './metricDefs.js'
  import BrandIcon from './BrandIcon.svelte'

  let {disks = []} = $props()

  function vendorOf(name) {
    const n = (name ?? '').toLowerCase()
    if (n.includes('intel')) return 'intel'
    if (n.includes('amd') || n.includes('ryzen') || n.includes('radeon')) return 'amd'
    if (n.includes('nvidia') || n.includes('geforce') || n.includes('rtx') || n.includes('gtx')) return 'nvidia'
    return null
  }

  const info = $derived(live.info)
  const gb = (v) => (v / 2 ** 30).toFixed(1)

  // Uptime from the boot timestamp; live.tick keeps it advancing on screen.
  const uptime = $derived.by(() => {
    void live.tick
    if (!info?.bootTime) return ''
    const s = Math.max(0, Math.floor(Date.now() / 1000 - info.bootTime))
    const d = Math.floor(s / 86400)
    const h = Math.floor(s / 3600) % 24
    const m = Math.floor(s / 60) % 60
    return d ? `up ${d} d ${h} h` : `up ${h} h ${m} min`
  })

  const cards = $derived.by(() => {
    if (!info) return []
    const out = [
      {
        icon: vendorOf(info.cpuModel) ?? 'cpu',
        label: 'Processor',
        name: cleanModel(info.cpuModel),
        sub: [
          `${info.cpuCores} cores · ${info.cpuThreads} threads`,
          info.cpuBaseMhz ? `base ${(info.cpuBaseMhz / 1000).toFixed(1)} GHz` : '',
        ].filter(Boolean).join(' · '),
      },
      {
        icon: vendorOf(info.gpuName) ?? 'gpu',
        label: 'Graphics',
        name: cleanModel(info.gpuName) || 'Not detected',
        sub: live.sample?.gpu?.memTotalMb
          ? `${(live.sample.gpu.memTotalMb / 1024).toFixed(0)} GB VRAM`
          : '',
      },
      {
        icon: 'ram',
        label: 'Memory',
        name: `${gb(info.ramTotal)} GB`,
        sub: info.ram?.modules
          ? `${info.ram.modules} × ${info.ram.moduleGb.toFixed(0)} GB ${info.ram.type || ''}-${info.ram.speedMt} · ${info.ram.vendor}`
          : '',
      },
      {
        icon: 'board',
        label: 'Motherboard',
        name: info.board || 'Unknown',
        sub: '',
      },
      {
        icon: 'windows',
        label: 'System',
        name: info.os,
        sub: [
          info.hostname,
          uptime,
          info.isAdmin ? 'administrator' : 'standard privileges',
        ].filter(Boolean).join(' · '),
      },
    ]
    for (const d of disks) {
      out.push({
        icon: d.media === 'HDD' ? 'disk-hdd' : 'disk-ssd',
        label: [d.bus, d.media].filter(Boolean).join(' ') || 'Drive',
        name: d.model,
        sub: [
          d.sizeGb ? `${d.sizeGb.toFixed(0)} GB` : '',
          d.health,
          d.tempC ? `${d.tempC.toFixed(0)}°C` : '',
        ].filter(Boolean).join(' · '),
        health: d.health,
      })
    }
    return out
  })
</script>

<div class="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3">
  {#each cards as c (c.label + c.name)}
    <div class="flex items-center gap-3 rounded-lg border border-line bg-card px-3.5 py-3">
      <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-card2 text-ink2">
        <BrandIcon brand={c.icon} size={22} />
      </div>
      <div class="min-w-0">
        <div class="font-mono text-[9px] uppercase tracking-[0.14em] text-mut">{c.label}</div>
        <div class="truncate text-sm text-ink" title={c.name}>{c.name}</div>
        {#if c.sub}
          <div class="truncate font-mono text-[10px] text-ink2">
            {#if c.health && c.health !== 'Healthy'}
              <span class="text-rec">{c.sub}</span>
            {:else}
              {c.sub}
            {/if}
          </div>
        {/if}
      </div>
    </div>
  {/each}
</div>
