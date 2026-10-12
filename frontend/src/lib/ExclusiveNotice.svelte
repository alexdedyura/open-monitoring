<script>
  // Why the HUD is not in the game: true exclusive fullscreen, as PresentMon
  // reports it. Shown on the dashboard, where the player lands after
  // alt-tabbing, and as one line in the HUD itself, which is visible on the
  // desktop even though it was not in the game.
  import {live, dismissExclusive, restoreFSO} from './state.svelte.js'
  import {t} from './i18n.svelte.js'

  let {compact = false} = $props()

  let busy = $state(false)
  let errKey = $state('')

  const ex = $derived(
    live.exclusive && !live.exclusiveDismissed.includes(live.exclusive.process) ? live.exclusive : null,
  )

  async function fix() {
    busy = true
    errKey = ''
    try {
      errKey = await restoreFSO()
    } catch {
      errKey = 'fso.restoreFailed'
    }
    busy = false
  }
</script>

{#if ex && compact}
  <div class="mt-1.5 border-t border-white/10 pt-1.5 text-[10px] leading-snug text-rec" style="text-shadow: 0 1px 2px rgba(0,0,0,.9)">
    ⚠ {t('fso.hudLine', {name: ex.process})}
  </div>
{:else if ex}
  <div class="mx-4 mt-3 flex items-start gap-3 rounded-lg border border-rec/30 bg-rec/5 px-4 py-3">
    <span class="mt-0.5 text-rec">⚠</span>
    <div class="min-w-0 grow space-y-1.5 text-sm leading-relaxed text-ink2">
      <p><span class="text-ink">{t('fso.title', {name: ex.process})}</span> {t('fso.why')}</p>
      <p>{t('fso.advice')}</p>
      {#if ex.restored}
        <p class="text-ink">{t('fso.restored')}</p>
      {:else if ex.fsoOff === 'app' || ex.fsoOff === 'global'}
        <p>{t(ex.fsoOff === 'app' ? 'fso.offApp' : 'fso.offGlobal')}</p>
        <button
          class="rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2 disabled:opacity-50"
          disabled={busy}
          onclick={fix}
        >
          {t(ex.fsoOff === 'app' ? 'fso.fixApp' : 'fso.fixGlobal')}
        </button>
      {:else if ex.fsoOff === 'machine'}
        <p>{t('fso.offMachine')}</p>
      {/if}
      {#if errKey}
        <p class="text-rec">{t(errKey)}</p>
      {/if}
    </div>
    <button class="shrink-0 text-mut hover:text-ink" onclick={dismissExclusive} aria-label={t('app.alert.dismiss')}>
      <svg width="9" height="9" viewBox="0 0 10 10"><path d="M0 0l10 10M10 0L0 10" stroke="currentColor" /></svg>
    </button>
  </div>
{/if}
