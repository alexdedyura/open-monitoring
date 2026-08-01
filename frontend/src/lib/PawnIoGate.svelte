<script>
  // Blocking gate: the app requires the PawnIO driver and stays locked until
  // it is installed. Shown instead of the dashboard once the sensor helper has
  // reported that the driver is missing; disappears reactively the moment a
  // reading arrives with the driver present.
  import {fade} from 'svelte/transition'
  import {installPawnIO, refreshInfo, api} from './state.svelte.js'
  import {t} from './i18n.svelte.js'
  import Logo from './Logo.svelte'

  let busy = $state(false)
  let checking = $state(false)
  let result = $state(null) // {message, ok} after an attempt

  async function install() {
    busy = true
    result = null
    try {
      result = await installPawnIO()
    } catch (e) {
      result = {message: String(e), ok: false}
    } finally {
      busy = false
    }
  }

  // For users who install manually from pawnio.eu while the gate is up.
  async function checkAgain() {
    checking = true
    try {
      await refreshInfo()
    } finally {
      checking = false
    }
  }
</script>

<div class="drag flex h-full flex-col items-center justify-center gap-5 rounded-lg bg-page px-8" in:fade={{duration: 250}}>
  <Logo size={72} />

  <div class="text-center">
    <h1 class="font-mono text-[13px] uppercase tracking-[0.35em] text-ink">Open Monitoring</h1>
    <p class="mt-1 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('app.gate.subtitle')}</p>
  </div>

  <!-- The link sits mid-sentence, so the sentence is split around it rather
       than held in one string. Both halves are written so that "… through
       PawnIO — …" and «… через PawnIO — …» put the driver's name in the same
       place; a translation that needs it elsewhere has to move the button. -->
  <p class="max-w-md text-center text-sm leading-relaxed text-ink2">
    {t('app.gate.beforeLink')}
    <button class="nodrag underline hover:text-ink" onclick={() => api.OpenPawnIOSite()}>PawnIO</button>
    {t('app.gate.afterLink')}
  </p>

  <div class="nodrag flex items-center gap-2">
    <button
      class="rounded-md border border-line bg-card2 px-4 py-2 font-mono text-xs text-ink hover:border-ink2 disabled:opacity-50"
      onclick={install}
      disabled={busy || checking}
    >
      {busy ? t('app.gate.installing') : t('app.gate.install')}
    </button>
    <button
      class="rounded-md border border-line px-4 py-2 font-mono text-xs text-ink2 hover:bg-card2 disabled:opacity-50"
      onclick={checkAgain}
      disabled={busy || checking}
    >
      {checking ? t('app.gate.checking') : t('app.gate.check')}
    </button>
  </div>

  {#if result && !result.ok}
    <p class="max-w-md text-center font-mono text-[11px] text-rec">{result.message}</p>
  {:else if result}
    <p class="max-w-md text-center font-mono text-[11px] text-ram">{result.message}</p>
  {/if}
</div>
