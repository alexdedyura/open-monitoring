<script>
  import {live, saveConfig, api} from './state.svelte.js'
  import {METRICS} from './metricDefs.js'
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

  const enabled = $derived(cfg.hud.metrics)
  const available = $derived(Object.keys(METRICS).filter((k) => !cfg.hud.metrics.includes(k)))

  function move(i, dir) {
    const j = i + dir
    if (j < 0 || j >= cfg.hud.metrics.length) return
    const m = cfg.hud.metrics
    ;[m[i], m[j]] = [m[j], m[i]]
  }

  function remove(i) {
    cfg.hud.metrics.splice(i, 1)
  }

  function add(k) {
    cfg.hud.metrics.push(k)
  }

  async function save() {
    await saveConfig($state.snapshot(cfg))
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

    <div class="space-y-1.5">
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-ink2">Sensor engine</span>
        {#if live.info?.lhmMode === 'bridge' && live.info?.lhmConnected}
          <span class="font-mono text-[11px] text-ram">● embedded (lhm-bridge)</span>
        {:else if live.info?.lhmMode === 'http' && live.info?.lhmConnected}
          <span class="font-mono text-[11px] text-ram">● external LHM (HTTP)</span>
        {:else}
          <span class="font-mono text-[11px] text-mut">○ starting…</span>
        {/if}
      </div>
      <input
        class="w-full rounded-md border border-line bg-card2 px-2.5 py-1.5 font-mono text-xs text-ink focus:border-ink2 focus:outline-none"
        bind:value={cfg.lhmUrl}
        title="Fallback URL of an external LibreHardwareMonitor web server"
      />
      <p class="text-xs leading-relaxed text-mut">
        Sensors run on an embedded
        <button class="underline hover:text-ink2" onclick={() => BrowserOpenURL('https://github.com/LibreHardwareMonitor/LibreHardwareMonitor')}>LibreHardwareMonitor</button>
        engine (lhm-bridge.exe, spawned automatically). The URL above is only a
        fallback to an external LHM instance if the bridge binary is missing.
        Run the app <span class="text-ink2">as administrator</span> to unlock CPU
        temperature, package power and drive SMART data.
      </p>
    </div>
  </div>

  <!-- HUD -->
  <div class="space-y-4 rounded-lg border border-line bg-card p-4">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">HUD overlay</h2>

    <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
      <div>
        <div class="mb-2 text-sm text-ink2">Rows, in order</div>
        <div class="space-y-1">
          {#each enabled as k, i (k)}
            <div class="flex items-center gap-2 rounded-md border border-line bg-card2 px-2.5 py-1.5">
              <span class="w-4 font-mono text-[10px] text-mut">{i + 1}</span>
              <span class="h-2 w-2 rounded-sm" style="background:{METRICS[k]?.color}"></span>
              <span class="grow font-mono text-xs text-ink">{METRICS[k]?.label ?? k}</span>
              <button class="px-1 font-mono text-xs text-mut hover:text-ink" onclick={() => move(i, -1)}>↑</button>
              <button class="px-1 font-mono text-xs text-mut hover:text-ink" onclick={() => move(i, 1)}>↓</button>
              <button class="px-1 font-mono text-xs text-mut hover:text-rec" onclick={() => remove(i)}>✕</button>
            </div>
          {/each}
        </div>
        {#if available.length}
          <div class="mt-3 flex flex-wrap gap-1.5">
            {#each available as k (k)}
              <button
                class="rounded-full border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:bg-card2"
                onclick={() => add(k)}
              >
                + {METRICS[k].label}
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <div class="space-y-4">
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
  </div>
</div>
