<script>
  import {live} from './state.svelte.js'
  import {cleanModel} from './metricDefs.js'
  import {t, fmtNum} from './i18n.svelte.js'
  import BrandIcon from './BrandIcon.svelte'

  let {disks = []} = $props()

  // Go reports SMART health as an English word; the card compares the raw
  // value, so only the display goes through the catalogue. Script scope, so
  // this table holds KEYS — healthLabel() is what resolves them, and it is
  // only ever called from inside the $derived below.
  const HEALTH_KEYS = {
    Healthy: 'dash.health.ok',
    Warning: 'dash.health.warning',
    Unhealthy: 'dash.health.bad',
  }
  const healthLabel = (h) => (HEALTH_KEYS[h] ? t(HEALTH_KEYS[h]) : h)

  function vendorOf(name) {
    const n = (name ?? '').toLowerCase()
    if (n.includes('intel')) return 'intel'
    if (n.includes('amd') || n.includes('ryzen') || n.includes('radeon')) return 'amd'
    if (n.includes('nvidia') || n.includes('geforce') || n.includes('rtx') || n.includes('gtx')) return 'nvidia'
    return null
  }

  const info = $derived(live.info)
  const gb = (v) => fmtNum(v / 2 ** 30, 1)

  // Uptime from the boot timestamp; live.tick keeps it advancing on screen.
  const uptime = $derived.by(() => {
    void live.tick
    if (!info?.bootTime) return ''
    const s = Math.max(0, Math.floor(Date.now() / 1000 - info.bootTime))
    const d = Math.floor(s / 86400)
    const h = Math.floor(s / 3600) % 24
    const m = Math.floor(s / 60) % 60
    // Abbreviated units on purpose: spelling out day/hour would drag four
    // Russian plural forms into a line that has to stay short.
    return d ? t('dash.sys.uptime.dh', {d, h}) : t('dash.sys.uptime.hm', {h, m})
  })

  const cards = $derived.by(() => {
    if (!info) return []
    const out = [
      {
        icon: vendorOf(info.cpuModel) ?? 'cpu',
        label: t('dash.sys.cpu'),
        name: cleanModel(info.cpuModel),
        sub: [
          info.cpuCores ? t('dash.sys.cpu.cores', {n: info.cpuCores}) : '',
          info.cpuThreads ? t('dash.sys.cpu.threads', {n: info.cpuThreads}) : '',
          info.cpuBaseMhz ? t('dash.sys.cpu.base', {v: fmtNum(info.cpuBaseMhz / 1000, 1)}) : '',
        ].filter(Boolean).join(' · '),
      },
      {
        icon: vendorOf(info.gpuName) ?? 'gpu',
        label: t('dash.sys.gpu'),
        name: cleanModel(info.gpuName) || t('dash.sys.gpu.none'),
        sub: live.sample?.gpu?.memTotalMb
          ? t('dash.sys.gpu.vram', {n: fmtNum(live.sample.gpu.memTotalMb / 1024, 0)})
          : '',
      },
      {
        icon: 'ram',
        label: t('dash.sys.ram'),
        name: `${gb(info.ramTotal)} GB`,
        // Type and speed form one designator (DDR5-6000): no locale digit
        // grouping in there, or it reads as two numbers.
        sub: info.ram?.modules
          ? `${info.ram.modules} × ${info.ram.moduleGb.toFixed(0)} GB ${info.ram.type || ''}-${info.ram.speedMt} · ${info.ram.vendor}`
          : '',
      },
      {
        icon: 'board',
        label: t('dash.sys.board'),
        name: info.board || t('dash.sys.unknown'),
        sub: '',
      },
      {
        icon: 'windows',
        label: t('dash.sys.os'),
        name: info.os,
        sub: [
          info.hostname,
          uptime,
          info.isAdmin ? t('dash.sys.admin') : t('dash.sys.standard'),
        ].filter(Boolean).join(' · '),
      },
    ]
    for (const d of disks) {
      out.push({
        icon: d.media === 'HDD' ? 'disk-hdd' : 'disk-ssd',
        label: [d.bus, d.media].filter(Boolean).join(' ') || t('dash.sys.drive'),
        name: d.model,
        sub: [
          d.sizeGb ? `${fmtNum(d.sizeGb, 0)} GB` : '',
          healthLabel(d.health),
          d.tempC ? `${fmtNum(d.tempC, 0)}°C` : '',
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
