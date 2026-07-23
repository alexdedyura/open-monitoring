<script>
  // Boot splash: the four entity-color dots assemble, the wordmark fades in.
  // Shown until the backend handshake finishes (plus a short minimum).
  const DOTS = ['#3987e5', '#d95926', '#199e70', '#c98500']
</script>

<div class="splash flex h-full w-full flex-col items-center justify-center bg-page">
  <div class="grid grid-cols-2 gap-1.5">
    {#each DOTS as c, i (c)}
      <span class="dot h-4 w-4 rounded-[3px]" style="background:{c}; animation-delay:{i * 120}ms"></span>
    {/each}
  </div>
  <div class="word mt-5 font-mono text-[13px] uppercase tracking-[0.35em] text-ink2">
    Open&nbsp;Monitoring
  </div>
  <div class="word mt-2 font-mono text-[10px] tracking-[0.1em] text-mut" style="animation-delay:.5s">
    starting sensors…
  </div>
</div>

<style>
  .dot {
    opacity: 0;
    transform: scale(0.3);
    animation: dot-in 0.5s cubic-bezier(0.2, 0.9, 0.3, 1.2) forwards,
      dot-pulse 1.8s ease-in-out 0.9s infinite;
  }

  .word {
    opacity: 0;
    animation: word-in 0.6s ease-out 0.35s forwards;
  }

  @keyframes dot-in {
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  @keyframes dot-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.55; }
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
    .dot, .word {
      animation: none;
      opacity: 1;
      transform: none;
    }
  }
</style>
