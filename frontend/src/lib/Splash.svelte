<script>
  // Boot splash: the logo scales in and pulses, the wordmark fades in.
  // Shown until the backend handshake finishes (plus a short minimum).
  import Logo from './Logo.svelte'
  import {t} from './i18n.svelte.js'
</script>

<div class="splash flex h-full w-full flex-col items-center justify-center bg-page">
  <div class="mark">
    <Logo size={72} />
  </div>
  <div class="word mt-5 font-mono text-[13px] uppercase tracking-[0.35em] text-ink2">
    Open&nbsp;Monitoring
  </div>
  <div class="word mt-2 font-mono text-[10px] tracking-[0.1em] text-mut" style="animation-delay:.5s">
    {t('app.splash.starting')}
  </div>
</div>

<style>
  .mark {
    opacity: 0;
    transform: scale(0.6);
    animation: mark-in 0.5s cubic-bezier(0.2, 0.9, 0.3, 1.2) forwards,
      mark-pulse 1.8s ease-in-out 0.9s infinite;
  }

  .word {
    opacity: 0;
    animation: word-in 0.6s ease-out 0.35s forwards;
  }

  @keyframes mark-in {
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  @keyframes mark-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }

  @keyframes word-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .mark, .word {
      animation: none;
      opacity: 1;
      transform: none;
    }
  }
</style>
