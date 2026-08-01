<script>
  // The project's README, rendered in-app. The file is inlined at build time
  // (Vite ?raw import), so the tab works offline and always matches the build.
  import {marked} from 'marked'
  import {BrowserOpenURL} from '../../wailsjs/runtime/runtime.js'
  import {live, api} from './state.svelte.js'
  import {t, lang} from './i18n.svelte.js'
  import Logo from './Logo.svelte'
  // Both READMEs are inlined. They are a couple of kilobytes each, and a static
  // import is what keeps this working offline — the alternative, fetching the
  // right one at runtime, is a network call inside a desktop app that makes a
  // point of not making any.
  import readmeEn from '../../../README.md?raw'
  import readmeRu from '../../../README.ru.md?raw'
  // The version comes from wails.json because that is what already stamps the
  // executable's file properties and the installer's pages — one place to bump,
  // three that agree. This tab is where a bug report starts, so the build has
  // to be nameable from inside the app.
  import wailsConfig from '../../../wails.json'

  const VERSION = wailsConfig.info.productVersion
  const REPO_URL = 'https://github.com/alexdedyura/open-monitoring'

  // idle → checking → (uptodate | available) → downloading → ready | error.
  // The background check may land at any point; it only ever moves idle
  // states forward, never interrupts a download.
  let phase = $state(live.update?.available ? 'available' : 'idle')
  let error = $state('')

  $effect(() => {
    if (live.update?.available && (phase === 'idle' || phase === 'uptodate')) phase = 'available'
  })

  async function check() {
    phase = 'checking'
    error = ''
    try {
      const info = await api.CheckForUpdate()
      live.update = info
      phase = info.available ? 'available' : 'uptodate'
    } catch (e) {
      error = String(e)
      phase = 'error'
    }
  }

  async function download() {
    phase = 'downloading'
    error = ''
    try {
      await api.ApplyUpdate()
      phase = 'ready'
    } catch (e) {
      error = String(e)
      phase = 'error'
    }
  }

  // Sections that only make sense outside the app: the screenshots are for
  // GitHub — their repo-relative paths cannot resolve inside the WebView, and
  // showing the app to itself is pointless — and Download explains how to
  // obtain the program the reader is already running.
  //
  // \r?\n matters: the README is checked out with CRLF on Windows, and a
  // \n-only pattern matches nothing at all. That is how the screenshots used to
  // survive this and render as captions with no images under them.
  // Per language, because the headings are what the regex matches on.
  const README = {
    en: {text: readmeEn, dropped: ['Screenshots', 'Download']},
    ru: {text: readmeRu, dropped: ['Скриншоты', 'Загрузка']},
  }

  const html = $derived.by(() => {
    const {text, dropped} = README[lang.code] ?? README.en
    return marked.parse(
      dropped.reduce(
        (md, heading) =>
          md.replace(new RegExp(String.raw`^## ${heading}\r?\n[\s\S]*?(?=^## )`, 'm'), ''),
        text,
      ),
    )
  })

  // All links leave the WebView for the system browser — the app is not a
  // web browser and has no navigation chrome to come back with.
  function onClick(e) {
    const a = e.target.closest('a')
    if (!a) return
    e.preventDefault()
    const href = a.getAttribute('href')
    if (href?.startsWith('http')) BrowserOpenURL(href)
  }
</script>

<div class="space-y-3 p-4">
  <div class="flex items-center gap-3 rounded-lg border border-line bg-card px-4 py-3">
    <Logo size={34} />
    <div class="min-w-0 grow">
      <div class="text-sm text-ink">Open Monitoring</div>
      <div class="font-mono text-[10px] text-mut">v{VERSION} · GPL-3.0 · {t('misc.about.openSource')}</div>
    </div>
    <button
      class="flex items-center gap-1.5 rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2"
      onclick={() => BrowserOpenURL(REPO_URL)}
      title={t('misc.about.starHint')}
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
        <path d="m12 2 2.9 6.26 6.85.63-5.17 4.54 1.53 6.72L12 16.63l-6.11 3.52 1.53-6.72L2.25 8.9l6.85-.64Z" />
      </svg>
      {t('misc.about.star')}
    </button>
  </div>

  <!-- updates: manual check, one-click install, restart -->
  <div class="flex items-center gap-3 rounded-lg border border-line bg-card px-4 py-3">
    <div class="min-w-0 grow">
      <div class="text-sm text-ink">{t('misc.about.updates')}</div>
      <div class="font-mono text-[10px] text-mut">
        {#if phase === 'checking'}
          {t('misc.about.phase.checking')}
        {:else if phase === 'uptodate'}
          {t('misc.about.phase.uptodate', {ver: VERSION})}
        {:else if phase === 'available'}
          {t('misc.about.phase.available', {ver: live.update.latest})}
          <button class="underline hover:text-ink" onclick={() => BrowserOpenURL(live.update.url)}>
            {t('misc.about.releaseNotes')}
          </button>
        {:else if phase === 'downloading'}
          {t('misc.about.phase.downloading')}
        {:else if phase === 'ready'}
          {t('misc.about.phase.ready')}
        {:else if phase === 'error'}
          {error}
        {:else}
          {t('misc.about.phase.idle')}
        {/if}
      </div>
    </div>
    {#if phase === 'available'}
      <button
        class="shrink-0 rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2"
        onclick={download}
      >
        {t('misc.about.updateTo', {ver: live.update.latest})}
      </button>
    {:else if phase === 'ready'}
      <button
        class="shrink-0 rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2"
        onclick={() => api.RestartApp()}
      >
        {t('misc.about.restart')}
      </button>
    {:else if phase === 'downloading'}
      <span class="shrink-0 font-mono text-xs text-mut">…</span>
    {:else}
      <button
        class="shrink-0 rounded-md border border-line px-3 py-1.5 font-mono text-xs text-ink2 hover:bg-card2"
        onclick={check}
        disabled={phase === 'checking'}
      >
        {t('misc.about.check')}
      </button>
    {/if}
  </div>

  <!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
  <div class="readme rounded-lg border border-line bg-card p-6" onclick={onClick}>
    {@html html}
  </div>
</div>

<style>
  /* Minimal markdown chrome over the app's tokens; :global because the HTML
     comes from {@html} and is invisible to Svelte's scoping. */
  .readme :global(h1) { font-size: 1.25rem; color: var(--color-ink); margin-bottom: 0.75rem; }
  .readme :global(h2) { font-size: 1rem; color: var(--color-ink); margin: 1.5rem 0 0.5rem; border-bottom: 1px solid var(--color-line); padding-bottom: 0.35rem; }
  .readme :global(h3) { font-size: 0.9rem; color: var(--color-ink); margin: 1.1rem 0 0.4rem; }
  .readme :global(p) { font-size: 0.85rem; line-height: 1.6; color: var(--color-ink2); margin-bottom: 0.6rem; }
  .readme :global(li) { font-size: 0.85rem; line-height: 1.55; color: var(--color-ink2); margin: 0.2rem 0 0.2rem 1.1rem; list-style: disc; }
  .readme :global(a) { color: var(--color-cpu); text-decoration: underline; }
  .readme :global(code) { font-family: var(--font-mono); font-size: 0.78rem; background: var(--color-card2); padding: 0.1rem 0.3rem; border-radius: 0.25rem; }
  .readme :global(pre) { background: var(--color-card2); border: 1px solid var(--color-line); border-radius: 0.5rem; padding: 0.75rem; overflow-x: auto; margin: 0.6rem 0; }
  .readme :global(pre code) { background: none; padding: 0; }
  .readme :global(table) { border-collapse: collapse; margin: 0.6rem 0; font-size: 0.8rem; }
  .readme :global(th), .readme :global(td) { border: 1px solid var(--color-line); padding: 0.35rem 0.6rem; color: var(--color-ink2); text-align: left; }
  .readme :global(strong) { color: var(--color-ink); }
  /* Repo-relative images (the header logo) have no base to resolve against
     in the WebView — the header above already shows the logo. */
  .readme :global(img) { display: none; }
</style>
