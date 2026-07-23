<script>
  import {onMount} from 'svelte'
  import {fade} from 'svelte/transition'
  import {live, init, enterHud, toggleTheme, saveConfig} from './lib/state.svelte.js'
  import {WindowMinimise, WindowToggleMaximise, Quit} from '../wailsjs/runtime/runtime.js'
  import Splash from './lib/Splash.svelte'
  import Dashboard from './lib/Dashboard.svelte'
  import Sessions from './lib/Sessions.svelte'
  import Settings from './lib/Settings.svelte'
  import Hud from './lib/Hud.svelte'

  let splashMinDone = $state(false)
  let hudAlert = $state(false)
  let hudAlertMute = $state(false)

  onMount(() => {
    init()
    setTimeout(() => (splashMinDone = true), 1500)
  })

  const booting = $derived(!live.ready || !splashMinDone)

  // UI scale applies to the main app only; the HUD overlay stays 1:1.
  $effect(() => {
    document.documentElement.dataset.theme = live.cfg?.theme ?? 'dark'
    document.documentElement.style.zoom = live.hud ? '' : String(live.cfg?.uiScale ?? 1)
  })

  function onHudClick() {
    if (live.cfg?.hud.fsAlertDismissed) {
      enterHud()
    } else {
      hudAlertMute = false
      hudAlert = true
    }
  }

  async function confirmHudAlert() {
    hudAlert = false
    if (hudAlertMute) {
      const cfg = $state.snapshot(live.cfg)
      cfg.hud.fsAlertDismissed = true
      await saveConfig(cfg)
    }
    enterHud()
  }

  const tabs = [
    {id: 'monitor', label: 'Monitor'},
    {id: 'sessions', label: 'Sessions'},
    {id: 'settings', label: 'Settings'},
  ]
</script>

{#if live.hud}
  <Hud />
{:else if booting}
  <Splash />
{:else}
  <div class="flex h-full flex-col overflow-hidden rounded-lg bg-page" in:fade={{duration: 250}}>
    <header class="drag flex h-10 shrink-0 items-center border-b border-line pl-4">
      <div class="flex items-center gap-2.5">
        <div class="grid grid-cols-2 gap-[3px]">
          <span class="h-[5px] w-[5px] rounded-[1px] bg-cpu"></span>
          <span class="h-[5px] w-[5px] rounded-[1px] bg-gpu"></span>
          <span class="h-[5px] w-[5px] rounded-[1px] bg-ram"></span>
          <span class="h-[5px] w-[5px] rounded-[1px] bg-vram"></span>
        </div>
        <span class="font-mono text-[11px] uppercase tracking-[0.18em] text-ink2">Open Monitoring</span>
      </div>

      <nav class="nodrag ml-8 flex h-full items-stretch">
        {#each tabs as t (t.id)}
          <button
            class="border-b-2 px-3 font-mono text-[11px] uppercase tracking-[0.1em] {live.view === t.id
              ? 'border-ink text-ink'
              : 'border-transparent text-mut hover:text-ink2'}"
            onclick={() => (live.view = t.id)}
          >
            {t.label}
          </button>
        {/each}
      </nav>

      <div class="grow"></div>

      {#if live.rec.active && live.view !== 'monitor'}
        <span class="rec-dot mr-3 h-2 w-2 rounded-full bg-rec" title="Recording"></span>
      {/if}

      <button
        class="nodrag mr-1.5 flex h-7 w-7 items-center justify-center rounded-md border border-line text-ink2 hover:bg-card2 hover:text-ink"
        onclick={toggleTheme}
        title="Toggle light / dark theme"
        aria-label="Toggle theme"
      >
        {#if (live.cfg?.theme ?? 'dark') === 'dark'}
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
          </svg>
        {:else}
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8Z" />
          </svg>
        {/if}
      </button>

      <button
        class="nodrag mr-1 rounded-md border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:bg-card2 hover:text-ink"
        onclick={onHudClick}
        title="Switch to overlay"
      >
        HUD
      </button>

      <button
        class="nodrag flex h-10 w-11 items-center justify-center text-mut hover:bg-card2 hover:text-ink"
        onclick={WindowMinimise}
        aria-label="Minimise"
      >
        <svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 5h10" stroke="currentColor" /></svg>
      </button>
      <button
        class="nodrag flex h-10 w-11 items-center justify-center text-mut hover:bg-card2 hover:text-ink"
        onclick={WindowToggleMaximise}
        aria-label="Maximise"
      >
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none"><rect x="0.5" y="0.5" width="9" height="9" stroke="currentColor" /></svg>
      </button>
      <button
        class="nodrag flex h-10 w-11 items-center justify-center text-mut hover:bg-rec hover:text-white"
        onclick={Quit}
        aria-label="Close"
      >
        <svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 0l10 10M10 0L0 10" stroke="currentColor" /></svg>
      </button>
    </header>

    <main class="min-h-0 flex-1 overflow-y-auto">
      {#if live.view === 'monitor'}
        <Dashboard />
      {:else if live.view === 'sessions'}
        <Sessions />
      {:else if live.view === 'settings' && live.cfg}
        <Settings />
      {/if}
    </main>
  </div>

  {#if hudAlert}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" transition:fade={{duration: 150}}>
      <div class="w-[420px] rounded-xl border border-line bg-card p-5 shadow-2xl">
        <h3 class="mb-2 font-mono text-[11px] uppercase tracking-[0.14em] text-mut">HUD overlay</h3>
        <p class="text-sm leading-relaxed text-ink2">
          The overlay stays on top of windowed and
          <span class="text-ink">borderless-fullscreen</span> apps and games.
          <span class="text-ink">Exclusive fullscreen</span> bypasses the desktop, so the
          HUD will not be visible there — in the game's video settings choose
          <span class="text-ink">Windowed Fullscreen (Borderless)</span>.
        </p>
        <label class="mt-4 flex items-center gap-2 text-xs text-ink2">
          <input type="checkbox" bind:checked={hudAlertMute} class="accent-white" />
          Don't show this again
        </label>
        <div class="mt-4 flex justify-end gap-2">
          <button
            class="rounded-md border border-line px-3 py-1.5 font-mono text-xs text-ink2 hover:bg-card2"
            onclick={() => (hudAlert = false)}
          >
            Cancel
          </button>
          <button
            class="rounded-md border border-line bg-card2 px-3 py-1.5 font-mono text-xs text-ink hover:border-ink2"
            onclick={confirmHudAlert}
          >
            Switch to HUD
          </button>
        </div>
      </div>
    </div>
  {/if}
{/if}
