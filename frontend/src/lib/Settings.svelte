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
  let hotkeys = $state({toggle: true, reset: true})

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

    <div class="flex items-center justify-between gap-4 rounded-md border border-line bg-card2/40 px-3 py-2">
      <span class="text-sm text-ink2">
        Sensor driver —
        <button class="underline hover:text-ink" onclick={() => api.OpenPawnIOSite()}>PawnIO</button>
        {live.info?.pawnIoVersion ?? ''}
      </span>
      <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] text-ram">installed</span>
    </div>
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
    </div>

    <p class="text-xs leading-relaxed text-mut">
      The HUD is a compact always-on-top overlay: drag it anywhere, resize by the
      edges — position and size are remembered. It stays above windowed and
      borderless-fullscreen games (exclusive fullscreen hides any overlay). The
      shortcuts work while a game has focus: click one and press the combination
      you want. Windows gives a combination to whichever application asks for it
      first, so if one is reported as held elsewhere, pick another.
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
