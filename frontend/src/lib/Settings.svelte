<script>
  import {live, saveConfig, api} from './state.svelte.js'
  import {METRICS, HUD_GROUPS} from './metricDefs.js'
  import {BrowserOpenURL} from '../../wailsjs/runtime/runtime.js'
  import SystemPanel from './SystemPanel.svelte'
  import {onMount} from 'svelte'

  let cfg = $state(structuredClone($state.snapshot(live.cfg)))
  let saved = $state(false)
  let disks = $state([])

  onMount(async () => {
    try {
      disks = (await api.GetDiskHealth()) ?? []
    } catch {
      disks = []
    }
  })

  function toggleMetric(k) {
    const i = cfg.hud.metrics.indexOf(k)
    if (i >= 0) cfg.hud.metrics.splice(i, 1)
    else cfg.hud.metrics.push(k)
  }

  let needsRestart = $state(false)

  async function save() {
    await saveConfig($state.snapshot(cfg))
    needsRestart = await api.RestartPending()
    saved = true
    setTimeout(() => (saved = false), 2500)
  }

</script>

<div class="grid grid-cols-1 gap-3 p-4 xl:grid-cols-2">
  <!-- sampling & recording -->
  <div class="space-y-4 rounded-lg border border-line bg-card p-4">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Sampling &amp; recording</h2>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">Sampling interval</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.sampleIntervalMs}
      >
        <option value={500}>500 ms</option>
        <option value={1000}>1 s</option>
        <option value={2000}>2 s</option>
        <option value={5000}>5 s</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">Recording auto-stop</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.maxRecordMinutes}
      >
        <option value={60}>1 hour</option>
        <option value={120}>2 hours</option>
        <option value={180}>3 hours</option>
        <option value={240}>4 hours</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">
        Interface scale
        {#if !cfg.uiScale && live.info?.osScale}
          <span class="ml-1 font-mono text-[10px] text-mut">
            now {Math.round(live.info.osScale * 100)}%
          </span>
        {/if}
      </span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.uiScale}
      >
        <option value={0}>Match Windows</option>
        <option value={1}>100%</option>
        <option value={1.25}>125%</option>
        <option value={1.5}>150%</option>
        <option value={2}>200%</option>
      </select>
    </label>

    <div class="space-y-2 rounded-md border border-line bg-card2/40 p-3">
      <label class="flex cursor-pointer items-start justify-between gap-4">
        <span>
          <span class="text-sm text-ink">CPU temperature and power</span>
          <span class="mt-0.5 block font-mono text-[10px] uppercase tracking-[0.1em] text-mut">
            loads a kernel driver
          </span>
        </span>
        <input type="checkbox" class="mt-1 accent-white" bind:checked={cfg.enableCpuSensors} />
      </label>
      <p class="text-xs leading-relaxed text-mut">
        Reading CPU package temperature and power needs ring-0 access, which the
        <button class="underline hover:text-ink2" onclick={() => BrowserOpenURL('https://github.com/LibreHardwareMonitor/LibreHardwareMonitor')}>LibreHardwareMonitor</button>
        engine gets through the WinRing0 driver. Microsoft Defender classifies
        that driver as vulnerable
        (<button class="underline hover:text-ink2" onclick={() => BrowserOpenURL('https://nvd.nist.gov/vuln/detail/CVE-2020-14979')}>CVE-2020-14979</button>)
        and quarantines it, so it stays <span class="text-ink2">off</span> by default.
        Everything else — CPU load, GPU, memory, drives with SMART, network and
        FPS — works without it.
      </p>
    </div>
  </div>

  <!-- HUD -->
  <div class="space-y-4 rounded-lg border border-line bg-card p-4">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">HUD overlay</h2>

    <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
      <div>
        <div class="mb-2 text-sm text-ink2">Sections and rows</div>
        <div class="space-y-3">
          {#each HUD_GROUPS as g (g.id)}
            <div>
              <div class="mb-1.5 flex items-center gap-2">
                <span class="h-2 w-2 rounded-sm" style="background:{METRICS[g.keys[0]]?.color}"></span>
                <span class="font-mono text-[10px] uppercase tracking-[0.12em] text-mut">{g.header(live.info)}</span>
              </div>
              <div class="flex flex-wrap gap-1.5">
                {#each g.keys as k (k)}
                  <button
                    class="rounded-full border px-2.5 py-1 font-mono text-[11px] {cfg.hud.metrics.includes(k)
                      ? 'border-ink2 bg-card2 text-ink'
                      : 'border-line text-mut hover:text-ink2'}"
                    onclick={() => toggleMetric(k)}
                  >
                    {METRICS[k].row}
                  </button>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      </div>

      <div class="space-y-4">
        <div>
          <div class="mb-1.5 text-sm text-ink2">Screen position</div>
          <div class="flex flex-wrap gap-1.5">
            {#each [
              ['free', 'Free (drag)'],
              ['tl', '↖ Top left'],
              ['tr', '↗ Top right'],
              ['bl', '↙ Bottom left'],
              ['br', '↘ Bottom right'],
            ] as [val, lbl] (val)}
              <button
                class="rounded-full border px-2.5 py-1 font-mono text-[11px] {cfg.hud.anchor === val
                  ? 'border-ink2 bg-card2 text-ink'
                  : 'border-line text-mut hover:text-ink2'}"
                onclick={() => (cfg.hud.anchor = val)}
              >
                {lbl}
              </button>
            {/each}
          </div>
        </div>

        <label class="block">
          <div class="mb-1 flex justify-between text-sm text-ink2">
            <span>Background opacity</span>
            <span class="font-mono text-xs text-mut">{Math.round(cfg.hud.opacity * 100)}%</span>
          </div>
          <input type="range" min="0.2" max="1" step="0.05" class="w-full accent-white" bind:value={cfg.hud.opacity} />
        </label>
        <p class="text-xs leading-relaxed text-mut">
          The HUD is a compact always-on-top overlay. Drag it anywhere, resize by
          the edges — position and size are remembered. It stays above windowed
          and borderless-fullscreen games (exclusive fullscreen hides any overlay).
        </p>
      </div>
    </div>
  </div>

  <!-- system info -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-4 xl:col-span-2">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">System</h2>
    <SystemPanel {disks} />
  </div>

  <div class="flex items-center gap-3 xl:col-span-2">
    <button
      class="rounded-md border border-line bg-card2 px-4 py-2 font-mono text-xs text-ink hover:border-ink2"
      onclick={save}
    >
      Save changes
    </button>
    {#if saved}
      <span class="font-mono text-xs text-ram">Saved ✓</span>
    {/if}
    {#if needsRestart}
      <span class="font-mono text-xs text-vram">Restart the app to apply the sensor change</span>
    {/if}
  </div>
</div>
