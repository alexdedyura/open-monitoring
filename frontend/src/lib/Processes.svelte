<script>
  import {onMount} from 'svelte'
  import {live, api} from './state.svelte.js'
  import {palette} from './metricDefs.js'
  import {t, fmtNum} from './i18n.svelte.js'

  // The backend rebuilds the table on a two-second clock of its own (see
  // metrics.processInterval), so this poll only ever collects a cache that is
  // at most one tick old — asking more often re-reads the same rows, asking
  // less often makes the tab look frozen.
  const POLL_MS = 2000

  // Three hundred rows that reshuffle every two seconds is not a list anyone
  // reads. The tail stays one click away instead of being paid for on every
  // render, which is also what keeps this virtualisation-free.
  const TOP = 50

  // The single row-level accent. Colouring more of them turns the table into
  // confetti and the colour stops meaning "look here".
  const HEAVY_CPU = 10

  // Below these a reading is real but not worth the ink, and it prints as an
  // em dash — the same convention the rest of the app uses for a metric it
  // could not read (metricDefs.js). An idle process genuinely uses no CPU, and
  // two hundred rows of "0.0" hide the five rows that matter.
  //
  // CPU has no floor of its own: the backend already rounds to one decimal, so
  // anything that survives the trip is at least 0.1 and the test is simply
  // "is it zero".
  const MEM_FLOOR = 2 ** 19 // half a MB; under it the column would print "0 MB"
  const IO_FLOOR = 1024 // B/s

  let snap = $state({at: 0, rows: [], threads: 0, error: ''})
  let query = $state('')
  let sort = $state('cpu') // cpu | mem | io | threads | name
  let grouped = $state(false)
  let showAll = $state(false)

  onMount(() => {
    const load = async () => {
      try {
        snap = await api.GetProcesses()
      } catch {
        /* keep the last table on screen rather than blanking the tab */
      }
    }
    load()
    const t = setInterval(load, POLL_MS)
    return () => clearInterval(t)
  })

  const pal = $derived(palette(live.cfg?.theme))
  const all = $derived(snap.rows ?? [])

  // An enumerator that stopped leaves a table indistinguishable from a machine
  // that went quiet, so say which one it is instead of showing plausible
  // numbers. Two missed ticks is noise; five is a source that is not coming
  // back.
  //
  // `void live.tick` is what makes this re-evaluate. The failure it exists to
  // report is the binding call throwing, and that path deliberately does not
  // reassign `snap` — so without a dependency that moves on its own, the one
  // time the banner is needed is the one time it would never appear. The
  // sample tick is the repo's clock for exactly this (Dashboard, Hud).
  const stale = $derived.by(() => {
    void live.tick
    return snap.at > 0 && Date.now() - snap.at > 5 * POLL_MS
  })

  const io = (r) => r.readBps + r.writeBps

  const rows = $derived.by(() => {
    let out = all

    // A browser is thirty processes, and reading them one at a time says
    // nothing about the browser. Grouping happens before filtering so the
    // totals are the whole image's, not the part of it that matched.
    if (grouped) {
      const by = new Map()
      for (const r of out) {
        const g = by.get(r.name)
        if (!g) {
          by.set(r.name, {...r, count: 1})
          continue
        }
        g.cpu += r.cpu
        g.memBytes += r.memBytes
        g.privBytes += r.privBytes
        g.readBps += r.readBps
        g.writeBps += r.writeBps
        g.threads += r.threads
        g.count++
      }
      out = [...by.values()]
    }

    const q = query.trim().toLowerCase()
    if (q) out = out.filter((r) => r.name.toLowerCase().includes(q) || String(r.pid) === q)

    // Every comparator falls back to the PID. Without it the thirty rows one
    // browser contributes tie on most columns and swap places on every poll,
    // which looks like a table that will not sit still.
    const cmp = {
      cpu: (a, b) => b.cpu - a.cpu || a.pid - b.pid,
      mem: (a, b) => b.memBytes - a.memBytes || a.pid - b.pid,
      io: (a, b) => io(b) - io(a) || a.pid - b.pid,
      threads: (a, b) => b.threads - a.threads || a.pid - b.pid,
      name: (a, b) => a.name.localeCompare(b.name) || a.pid - b.pid,
    }[sort]
    return [...out].sort(cmp)
  })

  const shown = $derived(showAll ? rows : rows.slice(0, TOP))

  // fmtMem and fmtIO return [value, unit] so the em dash can drop the unit with
  // it: "— MB" reads like a broken reading, "—" like nothing worth showing,
  // which is what most of this table is. CPU carries no unit and so needs no
  // pair — the column header holds the %.
  //
  // The digits go through fmtNum so the decimal mark follows the language: a
  // table of "12.3" in a Russian UI reads as untranslated. These are called from
  // the template, which is what keeps them reactive.
  const fmtCPU = (v) => (v > 0 ? fmtNum(v, 1) : '—')
  const fmtMem = (b) => (b >= MEM_FLOOR ? [fmtNum(b / 2 ** 20), ' MB'] : ['—', ''])
  const fmtIO = (bps) => {
    if (bps < IO_FLOOR) return ['—', '']
    if (bps < 2 ** 20) return [fmtNum(bps / 1024), ' KB/s']
    return [fmtNum(bps / 2 ** 20, 1), ' MB/s']
  }
  const join = ([v, unit]) => v + unit

  // The caret points the way the column sorts, and only the name sorts upwards
  // — the interesting end of every other column is the top.
  //
  // Headers hold KEYS, not words: a t() call out here would run once and freeze
  // in the language that was active when the tab mounted. The template
  // translates them at the point of use.
  const COLS = [
    {key: 'name', labelKey: 'misc.proc.col.name', align: 'text-left', caret: '▴'},
    {key: 'cpu', labelKey: 'misc.proc.col.cpu', align: 'text-right', caret: '▾'},
    {key: 'mem', labelKey: 'misc.proc.col.mem', align: 'text-right', caret: '▾'},
    {key: 'io', labelKey: 'misc.proc.col.io', align: 'text-right', caret: '▾'},
    {key: 'threads', labelKey: 'misc.proc.col.threads', align: 'text-right', caret: '▾'},
  ]

  // A grid rather than a table, since the app has no <table> anywhere. The
  // numeric tracks are fixed and the name takes the rest: sizing them to their
  // content instead would make every column twitch as the values change under
  // it, twice a second, down the whole page.
  const GRID = 'grid grid-cols-[minmax(0,1fr)_76px_100px_112px_68px] items-center gap-3'
</script>

<!-- Table bar: what the machine is running, and the two controls that shape the list. -->
<div class="sticky top-0 z-10 flex flex-wrap items-center gap-3 border-b border-line bg-page/90 px-4 py-2 backdrop-blur">
  <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">{t('misc.proc.title')}</h2>
  <span class="font-mono text-[11px] text-mut">
    {t('misc.proc.running', {n: all.length})} · {t('misc.proc.threads', {
      n: snap.threads ?? 0,
      count: fmtNum(snap.threads ?? 0),
    })}
  </span>
  {#if stale}
    <span class="font-mono text-[11px] text-vram">{t('misc.proc.stale')}</span>
  {/if}

  <div class="grow"></div>

  <input
    class="w-44 rounded-md border border-line bg-card px-2.5 py-1 font-mono text-xs text-ink placeholder:text-mut focus:border-ink2 focus:outline-none"
    placeholder={t('misc.proc.filter')}
    bind:value={query}
  />
  <button
    class="rounded-full border px-2.5 py-1 font-mono text-[11px] {grouped
      ? 'border-ink2 bg-card2 text-ink'
      : 'border-line text-mut hover:text-ink2'}"
    onclick={() => (grouped = !grouped)}
  >
    {t('misc.proc.group')}
  </button>
</div>

<div class="space-y-3 p-4">
  {#if snap.error}
    <p class="rounded-lg border border-rec/40 bg-rec/5 px-3 py-2 font-mono text-xs text-rec">{snap.error}</p>
  {/if}

  {#if snap.at === 0}
    <div class="flex h-64 items-center justify-center font-mono text-sm text-mut">
      {t('misc.proc.loading')}
    </div>
  {:else}
    <div class="divide-y divide-line overflow-hidden rounded-lg border border-line bg-card">
      <div class="{GRID} bg-card2/40 px-4 py-2">
        {#each COLS as c (c.key)}
          <button
            class="{c.align} font-mono text-[10px] uppercase tracking-[0.14em] {sort === c.key
              ? 'text-ink'
              : 'text-mut hover:text-ink2'}"
            onclick={() => (sort = c.key)}
          >
            {t(c.labelKey)}{sort === c.key ? ' ' + c.caret : ''}
          </button>
        {/each}
      </div>

      {#each shown as r (grouped ? r.name : r.pid)}
        {@const mem = fmtMem(r.memBytes)}
        {@const rate = fmtIO(io(r))}
        <div class="{GRID} px-4 py-1.5 hover:bg-card2">
          <div class="min-w-0">
            <div class="truncate text-xs text-ink">{r.name}</div>
            <div class="font-mono text-[10px] text-mut">
              {grouped ? t('misc.proc.count', {n: r.count}) : `pid ${r.pid}`}
            </div>
          </div>
          <span
            class="text-right font-mono text-xs {r.cpu > 0 ? 'text-ink2' : 'text-mut'}"
            style={r.cpu >= HEAVY_CPU ? `color:${pal.cpu}` : ''}
          >
            {fmtCPU(r.cpu)}
          </span>
          <span
            class="text-right font-mono text-xs {r.memBytes >= MEM_FLOOR ? 'text-ink2' : 'text-mut'}"
            title={r.privBytes ? t('misc.proc.private', {size: join(fmtMem(r.privBytes))}) : ''}
          >
            {mem[0]}<span class="text-mut">{mem[1]}</span>
          </span>
          <span
            class="text-right font-mono text-xs {io(r) >= IO_FLOOR ? 'text-ink2' : 'text-mut'}"
            title={t('misc.proc.rw', {
              read: join(fmtIO(r.readBps)),
              write: join(fmtIO(r.writeBps)),
            })}
          >
            {rate[0]}<span class="text-mut">{rate[1]}</span>
          </span>
          <span class="text-right font-mono text-xs text-ink2">{fmtNum(r.threads)}</span>
        </div>
      {:else}
        <div class="py-4 text-center font-mono text-xs text-mut">
          {query ? t('misc.proc.noMatch', {query}) : t('misc.proc.waiting')}
        </div>
      {/each}

      {#if rows.length > TOP}
        <button
          class="w-full px-4 py-2 text-center font-mono text-[11px] text-mut hover:text-ink2"
          onclick={() => (showAll = !showAll)}
        >
          {showAll
            ? t('misc.proc.showTop', {n: TOP})
            : t('misc.proc.showAll', {n: rows.length, count: fmtNum(rows.length)})}
        </button>
      {/if}
    </div>

    <p class="text-xs leading-relaxed text-mut">
      {t('misc.proc.note.cpu')}
      <span class="text-ink2">I/O</span>
      {t('misc.proc.note.io')}
    </p>
  {/if}
</div>
