<script>
  // The app mark: an abstract CPU with a rising metric inside. Fixed colors
  // (entity palette on the dark chip) so it reads identically in both themes,
  // in the HUD, and in README — one source of truth is assets/logo.svg.
  let {size = 24} = $props()
</script>

<svg width={size} height={size} viewBox="0 0 64 64" fill="none" aria-hidden="true">
  <g stroke="#5f6873" stroke-width="3" stroke-linecap="round">
    <path d="M23 6v4M32 6v4M41 6v4M23 54v4M32 54v4M41 54v4M6 23h4M6 32h4M6 41h4M54 23h4M54 32h4M54 41h4"/>
  </g>
  <rect x="11" y="11" width="42" height="42" rx="9" fill="#15181d" stroke="#9aa3ad" stroke-width="3"/>
  <path d="M18 44L26 35L33 39L46 25V46H18Z" fill="#3987e5" opacity=".25"/>
  <path d="M18 44L26 35L33 39L46 25" stroke="#3987e5" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"/>
  <circle cx="46" cy="25" r="3.4" fill="#c98500"/>
</svg>
