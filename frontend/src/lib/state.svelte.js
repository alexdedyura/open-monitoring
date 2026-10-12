import {EventsOn} from '../../wailsjs/runtime/runtime.js'
import * as api from '../../wailsjs/go/app/App.js'
import {newBuffers, appendSample} from './chartDefs.js'
import {setLang} from './i18n.svelte.js'

// Client-side history window: up to 30 min of samples feed the live charts.
const CAP_SECONDS = 1800

// Ring buffers are plain (non-reactive) arrays read directly by uPlot;
// `live.tick` is the reactive signal that a new sample arrived.
export const buf = newBuffers()

// The HUD's frame-rate readout and its two graphs run on their own event,
// which the backend emits several times per sample — a game's frame rate
// changes far faster than the sampling interval. Same non-reactive deal as
// `buf`, with `live.fpsTick` as the signal.
export const FPS_POINTS = 150 // ~15 s at the backend's FPS cadence
export const fpsBuf = {fps: [], frameMs: []}

export const live = $state({
  ready: false, // backend handshake done (splash gate)
  tick: 0,
  sample: null,
  fps: null, // freshest frame-rate metrics, ahead of sample.fps
  fpsTick: 0,
  info: null,
  cfg: null,
  rec: {active: false},
  stress: {running: false, jobs: []},
  view: 'monitor', // monitor | sessions | stress | settings | about
  hud: false,
  range: 300, // seconds shown in live charts
  update: null, // UpdateInfo once a newer release is known
  alerts: [], // active threshold-alert toasts, newest last
  // The last game seen in true exclusive fullscreen — {process, fsoOff, exe}
  // — where the HUD cannot be drawn. Kept after the game stops presenting:
  // alt-tabbing out of exclusive mode minimises the game, and the notice is
  // only readable once the player is back on the desktop.
  exclusive: null,
  exclusiveDismissed: [], // process names whose notice was closed this run
})

function pushSample(s) {
  appendSample(buf, s)
  if (buf.t.length > CAP_SECONDS) {
    for (const k in buf) buf[k].shift()
  }
}

// A null reading means nothing is presenting any more; the graphs are cleared
// rather than left holding the last game's frame times.
function pushFPS(m) {
  if (!m) {
    fpsBuf.fps.length = 0
    fpsBuf.frameMs.length = 0
    return
  }
  fpsBuf.fps.push(m.cur || null)
  fpsBuf.frameMs.push(m.frameMs || null)
  if (fpsBuf.fps.length > FPS_POINTS) {
    fpsBuf.fps.shift()
    fpsBuf.frameMs.shift()
  }
}

let started = false

export async function init() {
  if (started) return
  started = true
  const [cfg, info, rec, hist] = await Promise.all([
    api.GetConfig(),
    api.GetStaticInfo(),
    api.GetRecStatus(),
    api.GetHistory(CAP_SECONDS),
  ])
  live.cfg = cfg
  live.info = info
  live.rec = rec
  // Before the first render: the splash is already on screen in whatever
  // language the catalogue defaults to, and switching under the user is worse
  // than a beat of nothing.
  setLang(cfg.lang, info?.osLang)
  for (const s of hist ?? []) pushSample(s)
  if (hist?.length) {
    live.sample = hist[hist.length - 1]
    live.tick++
  }
  EventsOn('sample', (s) => {
    pushSample(s)
    live.sample = s
    live.tick++
  })
  EventsOn('fps', (m) => {
    live.fps = m
    pushFPS(m)
    live.fpsTick++
    noteExclusive(m)
    // Frame capture can come up long after the sensor poll has finished —
    // behind a UAC prompt the user answers in their own time — so the first
    // frames are what tells Settings it is running now.
    if (m && !live.info?.fpsOk) refreshInfoOnce()
  })
  // The HUD hotkey is handled by the backend, because it fires while a game
  // has focus; this is how the view finds out it was pressed.
  EventsOn('hud', (on) => {
    live.hud = on
  })
  EventsOn('recording', (r) => {
    live.rec = r
  })
  // The stress runner only emits while something is loaded, so the panel is
  // seeded once here — a run started before this window opened stays visible.
  EventsOn('stress', (s) => {
    live.stress = s
  })
  // The backend's daily check found a newer release; About offers it and the
  // header shows a dot until the user looks.
  EventsOn('update', (u) => {
    live.update = u
  })
  // The click-through hotkey is handled by the backend (it fires while a game
  // has focus); mirror the new state into the config the UI renders from.
  EventsOn('clickthrough', (on) => {
    if (live.cfg) live.cfg.hud.clickThrough = on
  })
  // Threshold alerts, shown as auto-expiring toasts on top of every view.
  EventsOn('alert', (a) => {
    live.alerts.push(a)
    setTimeout(() => {
      const i = live.alerts.indexOf(a)
      if (i >= 0) live.alerts.splice(i, 1)
    }, 10000)
  })
  try {
    live.stress = await api.GetStressStatus()
  } catch {
    // the panel renders its idle state from the default above
  }
  live.ready = true
  pollInfoUntilSensorsReady()
}

// The sensor helper takes a few seconds to start and produce its first
// reading, and until it does the backend cannot know the GPU model or whether
// the PawnIO driver is present. Without this the HUD would keep a generic
// "GPU" header and Settings would claim the driver is missing on machines
// where it is installed. Give up quietly — a helper that never reports is a
// state the UI already renders correctly.
async function pollInfoUntilSensorsReady() {
  for (let attempt = 0; attempt < 10 && !live.info?.sensorsOk; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 2000))
    try {
      live.info = await api.GetStaticInfo()
    } catch {
      return // backend went away; nothing left to refresh
    }
  }
}

// refreshInfo re-reads the machine description on demand, for views that show
// source status the user may have just changed outside the app.
export async function refreshInfo() {
  live.info = await api.GetStaticInfo()
}

// refreshInfoOnce is refreshInfo for a caller that fires ten times a second:
// one request in flight at a time.
let infoInFlight = false
async function refreshInfoOnce() {
  if (infoInFlight) return
  infoInFlight = true
  try {
    await refreshInfo()
  } catch {
    // the next frame will ask again
  } finally {
    infoInFlight = false
  }
}

// noteExclusive tracks the exclusive-fullscreen notice. A game that is seen
// presenting normally again — switched to borderless — clears its own notice.
function noteExclusive(m) {
  if (!m?.process) return
  if (m.exclusive) {
    const same = live.exclusive?.process === m.process
    // Restored, the setting only takes effect when the game restarts; until
    // then the backend still reports it off, and offering the fix again would
    // read as if it had not worked.
    if (same && live.exclusive.restored) return
    if (!same || live.exclusive.fsoOff !== (m.fsoOff || '')) {
      live.exclusive = {process: m.process, fsoOff: m.fsoOff || '', exe: m.exe || ''}
    }
  } else if (live.exclusive?.process === m.process) {
    live.exclusive = null
  }
}

export function dismissExclusive() {
  if (live.exclusive) live.exclusiveDismissed.push(live.exclusive.process)
}

// enableFPS raises the UAC prompt for frame capture again, then watches for
// the outcome: the prompt is answered whenever the user gets to it.
export async function enableFPS() {
  await api.EnableFPSCapture()
  for (let attempt = 0; attempt < 20; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 1500))
    try {
      await refreshInfo()
    } catch {
      return
    }
    if (live.info?.fpsOk || live.info?.fpsCanElevate) return
  }
}

// restoreFSO switches fullscreen optimisations back on for the game in the
// notice ("app") or for every game ("global"). Resolves to "" or an i18n key.
export async function restoreFSO() {
  const ex = live.exclusive
  if (!ex?.fsoOff) return ''
  const err = await api.RestoreFullscreenOptimizations(ex.fsoOff, ex.exe)
  if (!err) live.exclusive = {...ex, fsoOff: '', restored: true}
  return err
}

// installPawnIO runs the driver install and waits for the result to become
// visible. It is offered from two places — the startup notice and Settings —
// so the flow lives here rather than in either component.
//
// The wait is necessary: the backend relaunches the sensor helper, and the
// driver only shows as present once the replacement has produced a reading.
export async function installPawnIO() {
  const outcome = await api.InstallPawnIO()

  if (outcome.started) {
    for (let attempt = 0; attempt < 15 && !live.info?.pawnIo; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 1500))
      try {
        await refreshInfo()
      } catch {
        break
      }
    }
  }
  return {message: outcome.message, ok: outcome.started || outcome.openedSite}
}

// The stress panel drives the runner through these two: both return the fresh
// status so the UI reacts to a click without waiting for the next event tick.
export async function startStress(opts) {
  live.stress = await api.StartStress(opts)
}

export async function stopStress(targets = []) {
  live.stress = await api.StopStress(targets)
}

export async function saveConfig(cfg) {
  await api.SaveConfig(cfg)
  live.cfg = cfg
  // The language applies on save rather than on selection, so a user who picks
  // one and then cancels out of Settings does not get a half-applied switch.
  setLang(cfg.lang, live.info?.osLang)
}

export async function toggleTheme() {
  const cfg = $state.snapshot(live.cfg)
  if (!cfg) return
  cfg.theme = cfg.theme === 'light' ? 'dark' : 'light'
  await saveConfig(cfg)
}

export async function enterHud() {
  await api.SetHudMode(true)
  live.hud = true
}

export async function exitHud() {
  await api.SetHudMode(false)
  live.hud = false
}

export {api}
