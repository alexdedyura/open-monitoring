<script>
  import {onMount} from 'svelte'
  import {api, live} from './state.svelte.js'
  import {exportSessionPNG} from './exportPng.js'
  import SessionView from './SessionView.svelte'

  let sessions = $state([])
  let open = $state(null) // SessionInfo being viewed
  let confirmDel = $state(0) // id awaiting delete confirmation
  let exportedTo = $state('')

  async function reload() {
    sessions = (await api.ListSessions()) ?? []
  }

  onMount(reload)

  function fmtDate(ms) {
    return new Date(ms).toLocaleString(undefined, {
      day: '2-digit', month: '2-digit', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  }

  function fmtDur(s) {
    if (!s.endedAt) return 'recording…'
    const sec = Math.round((s.endedAt - s.startedAt) / 1000)
    const h = Math.floor(sec / 3600)
    const m = Math.floor(sec / 60) % 60
    return h ? `${h} h ${m} min` : `${m} min ${sec % 60} s`
  }

  async function del(id) {
    if (confirmDel !== id) {
      confirmDel = id
      setTimeout(() => (confirmDel = 0), 6000)
      return
    }
    confirmDel = 0
    await api.DeleteSession(id)
    await reload()
  }

  let exporting = $state(0) // session id being rendered

  async function exportPng(sess) {
    if (exporting) return
    exporting = sess.id
    try {
      const path = await exportSessionPNG(sess, live.cfg?.theme ?? 'dark')
      if (path) {
        exportedTo = path
        setTimeout(() => (exportedTo = ''), 5000)
      }
    } finally {
      exporting = 0
    }
  }
</script>

{#if open}
  <SessionView session={open} onback={() => { open = null; reload() }} onexport={() => exportPng(open)} />
{:else}
  <div class="space-y-3 p-4">
    <div class="flex items-baseline justify-between">
      <h2 class="font-mono text-[11px] uppercase tracking-[0.14em] text-mut">Recorded sessions</h2>
      {#if exportedTo}
        <span class="font-mono text-[11px] text-ink2">Saved to {exportedTo}</span>
      {/if}
    </div>

    {#if sessions.length === 0}
      <div class="rounded-lg border border-line bg-card p-8 text-center font-mono text-sm text-mut">
        No recordings yet. Start one with the Record button on the Monitor tab —
        it captures every metric for up to 4 hours.
      </div>
    {:else}
      <div class="divide-y divide-line overflow-hidden rounded-lg border border-line bg-card">
        {#each sessions as sess (sess.id)}
          <div class="flex items-center gap-4 px-4 py-3 hover:bg-card2">
            <button class="min-w-0 grow text-left" onclick={() => (open = sess)}>
              <div class="truncate text-sm text-ink">{sess.name}</div>
              <div class="mt-0.5 font-mono text-[11px] text-mut">
                {fmtDate(sess.startedAt)} · {fmtDur(sess)} · {sess.samples} samples · every {sess.intervalMs} ms
              </div>
            </button>
            <button
              class="rounded-md border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:bg-page"
              onclick={() => (open = sess)}
            >
              Open
            </button>
            <button
              class="rounded-md border border-line px-2.5 py-1 font-mono text-[11px] text-ink2 hover:bg-page disabled:opacity-50"
              onclick={() => exportPng(sess)}
              disabled={exporting !== 0}
            >
              {exporting === sess.id ? 'Rendering…' : 'PNG'}
            </button>
            <button
              class="rounded-md border px-2.5 py-1 font-mono text-[11px] {confirmDel === sess.id
                ? 'border-rec bg-rec/15 text-ink'
                : 'border-line text-ink2 hover:bg-page'}"
              onclick={() => del(sess.id)}
            >
              {confirmDel === sess.id ? 'Sure?' : 'Delete'}
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}
