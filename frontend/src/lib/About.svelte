<script>
  // The project's README, rendered in-app. The file is inlined at build time
  // (Vite ?raw import), so the tab works offline and always matches the build.
  import {marked} from 'marked'
  import {BrowserOpenURL} from '../../wailsjs/runtime/runtime.js'
  import Logo from './Logo.svelte'
  import readme from '../../../README.md?raw'

  const REPO_URL = 'https://github.com/alexdedyura/open-monitoring'

  // The screenshots section is for GitHub: its relative image paths cannot
  // resolve inside the WebView, and showing the app to itself is pointless.
  const html = marked.parse(readme.replace(/^## Screenshots\n[\s\S]*?(?=^## )/m, ''))

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
      <div class="font-mono text-[10px] text-mut">GPL-3.0 · open source</div>
    </div>
    <button
      class="flex items-center gap-1.5 rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2"
      onclick={() => BrowserOpenURL(REPO_URL)}
      title="Open the repository and star it"
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
        <path d="m12 2 2.9 6.26 6.85.63-5.17 4.54 1.53 6.72L12 16.63l-6.11 3.52 1.53-6.72L2.25 8.9l6.85-.64Z" />
      </svg>
      Star on GitHub
    </button>
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
