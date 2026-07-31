<script>
  import {live, saveConfig, refreshInfo, api} from './state.svelte.js'
  import {METRICS, HUD_GROUPS} from './metricDefs.js'
  import SystemPanel from './SystemPanel.svelte'
  import HotkeyInput from './HotkeyInput.svelte'
  import {onMount} from 'svelte'

  let cfg = $state(structuredClone($state.snapshot(live.cfg)))
  let saved = $state(false)
  let disks = $state([])

  // Whether Windows actually handed us each shortcut. It describes the saved
  // combinations, so it says nothing about one still being edited.
  let hotkeys = $state({toggle: true, reset: true, clickThrough: true})

  // The save bar reacts to edits: comparing against the applied config is
  // cheap at this size and spares tracking every field by hand.
  const dirty = $derived(
    JSON.stringify($state.snapshot(cfg)) !== JSON.stringify($state.snapshot(live.cfg)),
  )

  onMount(async () => {
    try {
      disks = (await api.GetDiskHealth()) ?? []
    } catch {
      disks = []
    }
    // The driver can be installed while the app is running, so re-read the
    // source status every time this panel is opened.
    try {
      await refreshInfo()
    } catch {
      // keep whatever was already known
    }
    await refreshHotkeys()
  })

  async function refreshHotkeys() {
    try {
      hotkeys = await api.GetHotkeyStatus()
    } catch {
      // keep whatever was already known
    }
  }

  function toggleMetric(k) {
    const i = cfg.hud.metrics.indexOf(k)
    if (i >= 0) cfg.hud.metrics.splice(i, 1)
    else cfg.hud.metrics.push(k)
  }

  async function save() {
    await saveConfig($state.snapshot(cfg))
    // The backend re-registers the shortcuts as part of the save, so the status
    // now describes the combinations that were just stored.
    await refreshHotkeys()
    saved = true
    setTimeout(() => (saved = false), 2500)
  }
</script>

<!-- Sticky action bar: the Save button is always in view, and lights up as
     soon as there is something to save. -->
<div class="sticky top-0 z-10 flex items-center gap-3 border-b border-line bg-page/90 px-4 py-2 backdrop-blur">
  <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Settings</h2>
  <div class="grow"></div>
  {#if saved}
    <span class="font-mono text-xs text-ram">Saved ✓</span>
  {:else if dirty}
    <span class="font-mono text-[11px] text-vram">unsaved changes</span>
  {/if}
  <button
    class="rounded-md border px-4 py-1.5 font-mono text-xs {dirty
      ? 'border-ink2 bg-card2 text-ink hover:border-ink'
      : 'border-line text-mut'}"
    onclick={save}
    disabled={!dirty}
  >
    Save changes
  </button>
</div>

<div class="grid grid-cols-1 gap-3 p-4 lg:grid-cols-2">
  <!-- sampling & recording -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5">
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

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">Delete recordings older than</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.keepSessionsDays}
      >
        <option value={0}>Keep forever</option>
        <option value={30}>30 days</option>
        <option value={90}>90 days</option>
        <option value={365}>1 year</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">Check for updates daily</span>
      <input type="checkbox" class="accent-white" bind:checked={cfg.updateCheck} />
    </label>

    <div class="flex items-center justify-between gap-4 rounded-md border border-line bg-card2/40 px-3 py-2">
      <span class="text-sm text-ink2">
        Sensor driver —
        <button class="underline hover:text-ink" onclick={() => api.OpenPawnIOSite()}>PawnIO</button>
        {live.info?.pawnIoVersion ?? ''}
      </span>
      <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] text-ram">installed</span>
    </div>

    <!-- data-source diagnostics: a dead helper otherwise renders as silent
         dashes, indistinguishable from a bug -->
    <div class="space-y-1.5 rounded-md border border-line bg-card2/40 px-3 py-2">
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-ink2">Hardware sensors</span>
        <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] {live.info?.sensorsOk ? 'text-ram' : 'text-rec'}">
          {live.info?.sensorsOk ? 'running' : 'not running'}
        </span>
      </div>
      {#if !live.info?.sensorsOk && live.info?.sensorsError}
        <p class="text-xs leading-relaxed text-mut">{live.info.sensorsError}</p>
      {/if}
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-ink2">Frame capture (PresentMon)</span>
        <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] {live.info?.fpsOk ? 'text-ram' : 'text-rec'}">
          {live.info?.fpsOk ? 'running' : 'not running'}
        </span>
      </div>
      {#if !live.info?.fpsOk && live.info?.fpsError}
        <p class="text-xs leading-relaxed text-mut">{live.info.fpsError}</p>
      {/if}
    </div>
  </div>

  <!-- threshold alerts -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5">
    <div class="flex items-center justify-between">
      <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Alerts</h2>
      <input type="checkbox" class="accent-white" bind:checked={cfg.alerts.enabled} />
    </div>

    <p class="text-xs leading-relaxed text-mut">
      A Windows notification and an in-app toast when a value crosses its limit.
      Each alert fires once per incident and re-arms after the value settles;
      set a limit to 0 to switch that rule off.
    </p>

    {#each [
      ['cpuTempC', 'CPU temperature', '°C', 40, 110],
      ['gpuTempC', 'GPU temperature', '°C', 40, 110],
      ['ramPercent', 'RAM usage', '%', 50, 100],
      ['diskPercent', 'Drive fill level', '%', 50, 100],
    ] as [key, label, unit, min, max] (key)}
      <label class="flex items-center justify-between gap-4 {cfg.alerts.enabled ? '' : 'opacity-40'}">
        <span class="text-sm text-ink2">{label}</span>
        <span class="flex items-center gap-1.5">
          <input
            type="number"
            {min}
            {max}
            step="1"
            class="w-20 rounded-md border border-line bg-card2 px-2 py-1.5 text-right font-mono text-xs text-ink focus:outline-none"
            bind:value={cfg.alerts[key]}
            disabled={!cfg.alerts.enabled}
          />
          <span class="w-6 font-mono text-xs text-mut">{unit}</span>
        </span>
      </label>
    {/each}
  </div>

  <!-- HUD: position & opacity -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">HUD overlay</h2>

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

    <!-- Width only: the overlay measures its own rows and sizes the window to
         them, so height is not the user's to set. -->
    <label class="block">
      <div class="mb-1 flex justify-between text-sm text-ink2">
        <span>Width</span>
        <span class="font-mono text-xs text-mut">{cfg.hud.w} px</span>
      </div>
      <input type="range" min="240" max="520" step="10" class="w-full accent-white" bind:value={cfg.hud.w} />
    </label>

    <label class="block">
      <div class="mb-1 flex justify-between text-sm text-ink2">
        <span>Background opacity</span>
        <span class="font-mono text-xs text-mut">{Math.round(cfg.hud.opacity * 100)}%</span>
      </div>
      <input type="range" min="0.2" max="1" step="0.05" class="w-full accent-white" bind:value={cfg.hud.opacity} />
    </label>

    <div class="space-y-2">
      <div class="text-sm text-ink2">Shortcuts</div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">Show / hide the overlay</span>
        <div class="text-right">
          <HotkeyInput
            value={cfg.hud.hotkeyToggle}
            live={dirty || hotkeys.toggle}
            onchange={(v) => (cfg.hud.hotkeyToggle = v)}
          />
        </div>
      </div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">Restart average and lows</span>
        <div class="text-right">
          <HotkeyInput
            value={cfg.hud.hotkeyReset}
            live={dirty || hotkeys.reset}
            onchange={(v) => (cfg.hud.hotkeyReset = v)}
          />
        </div>
      </div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">Toggle click-through</span>
        <div class="text-right">
          <HotkeyInput
            value={cfg.hud.hotkeyClickThrough}
            live={dirty || hotkeys.clickThrough}
            onchange={(v) => (cfg.hud.hotkeyClickThrough = v)}
          />
        </div>
      </div>
    </div>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">
        Click-through
        <span class="block text-xs text-mut">Mouse clicks pass through the overlay into the game</span>
      </span>
      <input type="checkbox" class="accent-white" bind:checked={cfg.hud.clickThrough} />
    </label>

    <p class="text-xs leading-relaxed text-mut">
      The HUD is a compact always-on-top overlay. Its height follows the metrics
      you enable below — the window cannot be resized by hand. On
      <span class="text-ink2">Free</span> it is draggable and its position is
      remembered; on a corner it stays pinned there. It stays above windowed and
      borderless-fullscreen games (exclusive fullscreen hides any overlay). The
      shortcuts work while a game has focus: click one and press the combination
      you want. Windows gives a combination to whichever application asks for it
      first, so if one is reported as held elsewhere, pick another. A
      click-through overlay cannot be clicked or dragged — use its shortcut to
      make it interactive again.
    </p>
  </div>

  <!-- HUD: sections and rows -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5 lg:col-span-2">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">HUD sections and rows</h2>
    <div class="grid grid-cols-1 gap-x-6 gap-y-3 md:grid-cols-2 xl:grid-cols-3">
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

  <!-- system info -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5 lg:col-span-2">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">System</h2>
    <SystemPanel {disks} />
  </div>
</div>
