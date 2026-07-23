<script>
  import {siIntel, siAmd, siNvidia} from 'simple-icons'

  // brand: intel | amd | nvidia | windows | cpu | gpu | ram | board | disk-ssd | disk-hdd
  let {brand, size = 22} = $props()

  const SI = {intel: siIntel, amd: siAmd, nvidia: siNvidia}

  // Simple generic glyphs (24x24 viewBox) for parts without a brand mark.
  const GLYPHS = {
    windows: 'M3 5.5 10.5 4.4V11.5H3V5.5ZM3 12.5H10.5V19.6L3 18.5V12.5ZM11.5 4.25 21 3V11.5H11.5V4.25ZM11.5 12.5H21V21L11.5 19.75V12.5Z',
    cpu: 'M9 2h2v3h2V2h2v3h1a2 2 0 0 1 2 2v1h3v2h-3v2h3v2h-3v2a2 2 0 0 1-2 2h-1v3h-2v-3h-2v3H9v-3H8a2 2 0 0 1-2-2v-2H3v-2h3v-2H3V8h3V7a2 2 0 0 1 2-2h1V2Zm-1 5v10h8V7H8Zm2 2h4v6h-4V9Z',
    gpu: 'M2 6h20v10h-3v3h-2v-3H7v3H5v-3H2V6Zm2 2v6h16V8H4Zm4 1a2 2 0 1 1 0 4 2 2 0 0 1 0-4Zm8 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4Z',
    ram: 'M2 7h20v7h-2v3h-3v-3h-3v3h-4v-3H7v3H4v-3H2V7Zm3 2v3h3V9H5Zm5 0v3h4V9h-4Zm6 0v3h3V9h-3Z',
    board: 'M3 3h18v18H3V3Zm2 2v14h14V5H5Zm2 2h6v6H7V7Zm8 0h4v2h-4V7Zm0 4h4v2h-4v-2ZM7 15h4v2H7v-2Zm6 0h6v2h-6v-2Z',
    'disk-ssd': 'M4 5h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Zm0 2v10h16V7H4Zm2 2h2v6H6V9Zm4 0h2v6h-2V9Zm4 0h2v6h-2V9Z',
    'disk-hdd': 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18Zm0 2a7 7 0 1 0 4.9 12L13 13.1A3 3 0 1 1 15 10h2.9A7 7 0 0 0 12 5Zm0 5.5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3Z',
  }

  const si = $derived(SI[brand])
  const glyph = $derived(GLYPHS[brand] ?? GLYPHS.cpu)
  const tint = $derived(
    si ? '#' + si.hex
      : brand === 'windows' ? '#0078d4'
      : 'currentColor'
  )
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill={tint} aria-hidden="true">
  {#if si}
    <path d={si.path} />
  {:else}
    <path d={glyph} fill-rule="evenodd" />
  {/if}
</svg>
