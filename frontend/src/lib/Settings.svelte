<script>
  import {live, saveConfig, refreshInfo, api} from './state.svelte.js'
  import {METRICS, HUD_GROUPS} from './metricDefs.js'
  import {t, fmtNum} from './i18n.svelte.js'
  import SystemPanel from './SystemPanel.svelte'
  import HotkeyInput from './HotkeyInput.svelte'
  import {onMount} from 'svelte'

  let cfg = $state(structuredClone($state.snapshot(live.cfg)))
  let saved = $state(false)
  let disks = $state([])

  // Whether Windows actually handed us each shortcut. It describes the saved
  // combinations, so it says nothing about one still being edited.
  let hotkeys = $state({toggle: true, reset: true, clickThrough: true})

  // The three tables below hold KEYS, never translated text: a t() at script
  // scope runs once and freezes in whatever language was active at mount. The
  // template translates them at the point of use, where Svelte tracks the read.
  const ALERT_ROWS = [
    {key: 'cpuTempC', labelKey: 'settings.alerts.cpuTemp', unit: '°C', min: 40, max: 110},
    {key: 'gpuTempC', labelKey: 'settings.alerts.gpuTemp', unit: '°C', min: 40, max: 110},
    {key: 'ramPercent', labelKey: 'settings.alerts.ramUsage', unit: '%', min: 50, max: 100},
    {key: 'diskPercent', labelKey: 'settings.alerts.diskFill', unit: '%', min: 50, max: 100},
  ]

  const HUD_ANCHORS = [
    {val: 'free', labelKey: 'settings.hud.anchor.free'},
    {val: 'tl', labelKey: 'settings.hud.anchor.tl'},
    {val: 'tr', labelKey: 'settings.hud.anchor.tr'},
    {val: 'bl', labelKey: 'settings.hud.anchor.bl'},
    {val: 'br', labelKey: 'settings.hud.anchor.br'},
  ]

  // The two language names are deliberately not translated: a language picker
  // has to stay readable in the language you are switching away from. Only the
  // "follow Windows" option is a catalogue string.
  const LANGS = [
    {val: 'auto', labelKey: 'settings.lang.auto'},
    {val: 'en', label: 'English'},
    {val: 'ru', label: 'Русский'},
  ]

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
  <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.title')}</h2>
  <div class="grow"></div>
  {#if saved}
    <span class="font-mono text-xs text-ram">{t('settings.saved')}</span>
  {:else if dirty}
    <span class="font-mono text-[11px] text-vram">{t('settings.unsaved')}</span>
  {/if}
  <button
    class="rounded-md border px-4 py-1.5 font-mono text-xs {dirty
      ? 'border-ink2 bg-card2 text-ink hover:border-ink'
      : 'border-line text-mut'}"
    onclick={save}
    disabled={!dirty}
  >
    {t('settings.save')}
  </button>
</div>

<div class="grid grid-cols-1 gap-3 p-4 lg:grid-cols-2">
  <!-- sampling & recording -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.sampling.title')}</h2>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">{t('settings.sampling.interval')}</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.sampleIntervalMs}
      >
        <option value={500}>{t('settings.interval.ms', {n: 500})}</option>
        <option value={1000}>{t('settings.interval.s', {n: 1})}</option>
        <option value={2000}>{t('settings.interval.s', {n: 2})}</option>
        <option value={5000}>{t('settings.interval.s', {n: 5})}</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">{t('settings.recording.autoStop')}</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.maxRecordMinutes}
      >
        <option value={60}>{t('settings.recording.hours', {n: 1})}</option>
        <option value={120}>{t('settings.recording.hours', {n: 2})}</option>
        <option value={180}>{t('settings.recording.hours', {n: 3})}</option>
        <option value={240}>{t('settings.recording.hours', {n: 4})}</option>
      </select>
    </label>

    <!-- The language applies on save, like everything else on this page. -->
    <div class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">{t('settings.lang.label')}</span>
      <div class="flex flex-wrap justify-end gap-1.5">
        {#each LANGS as l (l.val)}
          <button
            class="rounded-full border px-2.5 py-1 font-mono text-[11px] {cfg.lang === l.val
              ? 'border-ink2 bg-card2 text-ink'
              : 'border-line text-mut hover:text-ink2'}"
            onclick={() => (cfg.lang = l.val)}
          >
            {l.label ?? t(l.labelKey)}
          </button>
        {/each}
      </div>
    </div>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">
        {t('settings.uiScale.label')}
        {#if !cfg.uiScale && live.info?.osScale}
          <span class="ml-1 font-mono text-[10px] text-mut">
            {t('settings.uiScale.current', {n: fmtNum(live.info.osScale * 100)})}
          </span>
        {/if}
      </span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.uiScale}
      >
        <option value={0}>{t('settings.uiScale.auto')}</option>
        <option value={1}>100%</option>
        <option value={1.25}>125%</option>
        <option value={1.5}>150%</option>
        <option value={2}>200%</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">{t('settings.retention.label')}</span>
      <select
        class="rounded-md border border-line bg-card2 px-2 py-1.5 font-mono text-xs text-ink focus:outline-none"
        bind:value={cfg.keepSessionsDays}
      >
        <option value={0}>{t('settings.retention.forever')}</option>
        <option value={30}>{t('settings.retention.days', {n: 30})}</option>
        <option value={90}>{t('settings.retention.days', {n: 90})}</option>
        <option value={365}>{t('settings.retention.year1')}</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-ink2">{t('settings.updateCheck')}</span>
      <input type="checkbox" class="accent-white" bind:checked={cfg.updateCheck} />
    </label>

    <div class="flex items-center justify-between gap-4 rounded-md border border-line bg-card2/40 px-3 py-2">
      <span class="text-sm text-ink2">
        {t('settings.driver.label')}
        <button class="underline hover:text-ink" onclick={() => api.OpenPawnIOSite()}>PawnIO</button>
        {live.info?.pawnIoVersion ?? ''}
      </span>
      <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] text-ram">
        {t('settings.driver.installed')}
      </span>
    </div>

    <!-- data-source diagnostics: a dead helper otherwise renders as silent
         dashes, indistinguishable from a bug -->
    <div class="space-y-1.5 rounded-md border border-line bg-card2/40 px-3 py-2">
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-ink2">{t('settings.sources.sensors')}</span>
        <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] {live.info?.sensorsOk ? 'text-ram' : 'text-rec'}">
          {live.info?.sensorsOk ? t('settings.sources.running') : t('settings.sources.stopped')}
        </span>
      </div>
      {#if !live.info?.sensorsOk && live.info?.sensorsError}
        <p class="text-xs leading-relaxed text-mut">{live.info.sensorsError}</p>
      {/if}
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-ink2">{t('settings.sources.fps')}</span>
        <span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.1em] {live.info?.fpsOk ? 'text-ram' : 'text-rec'}">
          {live.info?.fpsOk ? t('settings.sources.running') : t('settings.sources.stopped')}
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
      <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.alerts.title')}</h2>
      <input type="checkbox" class="accent-white" bind:checked={cfg.alerts.enabled} />
    </div>

    <p class="text-xs leading-relaxed text-mut">{t('settings.alerts.about')}</p>

    {#each ALERT_ROWS as row (row.key)}
      <label class="flex items-center justify-between gap-4 {cfg.alerts.enabled ? '' : 'opacity-40'}">
        <span class="text-sm text-ink2">{t(row.labelKey)}</span>
        <span class="flex items-center gap-1.5">
          <input
            type="number"
            min={row.min}
            max={row.max}
            step="1"
            class="w-20 rounded-md border border-line bg-card2 px-2 py-1.5 text-right font-mono text-xs text-ink focus:outline-none"
            bind:value={cfg.alerts[row.key]}
            disabled={!cfg.alerts.enabled}
          />
          <span class="w-6 font-mono text-xs text-mut">{row.unit}</span>
        </span>
      </label>
    {/each}
  </div>

  <!-- HUD: position & opacity -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.hud.title')}</h2>

    <div>
      <div class="mb-1.5 text-sm text-ink2">{t('settings.hud.position')}</div>
      <div class="flex flex-wrap gap-1.5">
        {#each HUD_ANCHORS as a (a.val)}
          <button
            class="rounded-full border px-2.5 py-1 font-mono text-[11px] {cfg.hud.anchor === a.val
              ? 'border-ink2 bg-card2 text-ink'
              : 'border-line text-mut hover:text-ink2'}"
            onclick={() => (cfg.hud.anchor = a.val)}
          >
            {t(a.labelKey)}
          </button>
        {/each}
      </div>
    </div>

    <!-- Width only: the overlay measures its own rows and sizes the window to
         them, so height is not the user's to set. -->
    <label class="block">
      <div class="mb-1 flex justify-between text-sm text-ink2">
        <span>{t('settings.hud.width')}</span>
        <span class="font-mono text-xs text-mut">{fmtNum(cfg.hud.w)} px</span>
      </div>
      <input type="range" min="240" max="520" step="10" class="w-full accent-white" bind:value={cfg.hud.w} />
    </label>

    <label class="block">
      <div class="mb-1 flex justify-between text-sm text-ink2">
        <span>{t('settings.hud.opacity')}</span>
        <span class="font-mono text-xs text-mut">{fmtNum(cfg.hud.opacity * 100)}%</span>
      </div>
      <input type="range" min="0.2" max="1" step="0.05" class="w-full accent-white" bind:value={cfg.hud.opacity} />
    </label>

    <div class="space-y-2">
      <div class="text-sm text-ink2">{t('settings.hud.shortcuts')}</div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">{t('settings.hud.hotkeyToggle')}</span>
        <div class="text-right">
          <HotkeyInput
            value={cfg.hud.hotkeyToggle}
            live={dirty || hotkeys.toggle}
            onchange={(v) => (cfg.hud.hotkeyToggle = v)}
          />
        </div>
      </div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">{t('settings.hud.hotkeyReset')}</span>
        <div class="text-right">
          <HotkeyInput
            value={cfg.hud.hotkeyReset}
            live={dirty || hotkeys.reset}
            onchange={(v) => (cfg.hud.hotkeyReset = v)}
          />
        </div>
      </div>
      <div class="flex items-start justify-between gap-4">
        <span class="pt-1.5 text-xs text-ink2">{t('settings.hud.hotkeyClickThrough')}</span>
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
        {t('settings.hud.clickThrough')}
        <span class="block text-xs text-mut">{t('settings.hud.clickThrough.hint')}</span>
      </span>
      <input type="checkbox" class="accent-white" bind:checked={cfg.hud.clickThrough} />
    </label>

    <!-- Split around the highlighted anchor name: the emphasis is part of the
         sentence, and each half is its own key so a translation can restructure
         inside it. -->
    <p class="text-xs leading-relaxed text-mut">
      {t('settings.hud.about.size')}
      {t('settings.hud.about.anchorPre')}
      <span class="text-ink2">{t('settings.hud.anchor.freeName')}</span>
      {t('settings.hud.about.anchorPost')}
      {t('settings.hud.about.fullscreen')}
      {t('settings.hud.about.shortcuts')}
      {t('settings.hud.about.clickThrough')}
    </p>
  </div>

  <!-- HUD: sections and rows -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5 lg:col-span-2">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.hudRows.title')}</h2>
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
                {t(METRICS[k].rowKey)}
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- system info -->
  <div class="space-y-3 rounded-lg border border-line bg-card p-3.5 lg:col-span-2">
    <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('settings.system.title')}</h2>
    <SystemPanel {disks} />
  </div>
</div>
